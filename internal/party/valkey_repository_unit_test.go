package party_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func sampleTestParty(id, leaderID, name string) party.Party {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	return party.Party{
		ID:                id,
		LeaderCharacterID: leaderID,
		Name:              name,
		StageID:           "stage-1",
		Speed:             18,
		MaxMembers:        4,
		Status:            party.StatusRecruiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
}

func sampleTestMembers(partyID, leaderID string) []party.Member {
	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	return []party.Member{
		{
			PartyID:       partyID,
			CharacterID:   leaderID,
			CharacterName: "LeaderHero",
			JobID:         "job-hero",
			Level:         15,
			HP:            100,
			MaxHP:         100,
			IsLeader:      true,
			JoinedAt:      now,
		},
		{
			PartyID:       partyID,
			CharacterID:   "char-ally-2",
			CharacterName: "AllyFighter",
			JobID:         "job-warrior",
			Level:         14,
			HP:            120,
			MaxHP:         120,
			IsLeader:      false,
			JoinedAt:      now.Add(2 * time.Second),
		},
	}
}

func TestValkeyRepository_RequiresClient(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("expected nil Valkey client to be rejected")
		}
	}()
	party.NewValkeyRepository(nil)
}

func TestValkeyRepository_SaveParty(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-save-1", "char-lead-1", "勇者隊")

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 1 || len(cmds[0]) != 13 || cmds[0][5] != "save" ||
		cmds[0][3] != party.DefaultLobbyKeyPrefix+p.ID || cmds[0][4] != party.DefaultLobbiesIndexKey {
		t.Fatalf("unexpected lobby publication: %v", cmds)
	}
	var state party.LobbyState
	if err := json.Unmarshal([]byte(cmds[0][12]), &state); err != nil {
		t.Fatalf("saved lobby payload is invalid JSON: %v", err)
	}
	if state.Party.ID != p.ID || state.Party.LeaderCharacterID != p.LeaderCharacterID {
		t.Errorf("unexpected saved state: %+v", state)
	}

	errIndex := errors.New("lobby index is unwritable")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(context.Context, valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errIndex)
	}))
	if err := party.NewValkeyRepository(clientErr).SaveParty(ctx, p); !errors.Is(err, errIndex) {
		t.Fatalf("SaveParty error = %v, want %v", err, errIndex)
	}
}

func TestValkeyRepository_GetPartyAndGetPartyForUpdate(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-get-1", "char-lead-1", "探索隊")
	state := party.LobbyState{
		Party:   p,
		Members: sampleTestMembers("party-get-1", "char-lead-1"),
	}
	validData, _ := json.Marshal(state)

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" && cmd.Commands()[1] == party.DefaultLobbyKeyPrefix+"party-get-1" {
			return valkeytest.MakeStringResult(string(validData))
		}
		return valkeytest.MakeNilResult()
	}))
	repo := party.NewValkeyRepository(client)

	gotParty, err := repo.GetParty(ctx, "party-get-1")
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if gotParty.ID != "party-get-1" || gotParty.Name != "探索隊" {
		t.Errorf("unexpected party: %+v", gotParty)
	}

	// GetPartyForUpdate delegates to GetParty
	gotPartyForUpdate, err := repo.GetPartyForUpdate(ctx, "party-get-1")
	if err != nil || gotPartyForUpdate.ID != "party-get-1" {
		t.Fatalf("GetPartyForUpdate failed: got %+v, err=%v", gotPartyForUpdate, err)
	}

	// 2. Not found: Valkey Nil
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoNil := party.NewValkeyRepository(clientNil)
	_, err = repoNil.GetParty(ctx, "party-not-found")
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound on nil, got %v", err)
	}

	// 4. Error case: Malformed JSON
	clientCorrupt := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("invalid json")
	}))
	repoCorrupt := party.NewValkeyRepository(clientCorrupt)
	if _, err := repoCorrupt.GetParty(ctx, "party-corrupt"); err == nil {
		t.Errorf("expected error on malformed JSON")
	}

	// 5. Error case: GET command failure
	errGet := errors.New("valkey get error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errGet)
	}))
	repoErr := party.NewValkeyRepository(clientErr)
	if _, err := repoErr.GetParty(ctx, "party-err"); !errors.Is(err, errGet) {
		t.Fatalf("expected errGet, got %v", err)
	}
}

func TestValkeyRepository_ListParties(t *testing.T) {
	ctx := context.Background()

	p1 := sampleTestParty("p1", "c1", "Party 1")
	p2 := sampleTestParty("p2", "c2", "Party 2")
	p2.Status = party.StatusInProgress // Different status

	data1, _ := json.Marshal(party.LobbyState{Party: p1, Members: sampleTestMembers("p1", "c1")})
	data2, _ := json.Marshal(party.LobbyState{Party: p2, Members: sampleTestMembers("p2", "c2")})

	// 1. Success case: ZREVRANGE returns IDs, GET returns states
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if c[0] == "ZREVRANGE" {
			return valkeytest.MakeStringSliceResult([]string{"p1", "p2", "stale_p3", "corrupt_p4"})
		}
		if c[0] == "GET" {
			switch c[1] {
			case party.DefaultLobbyKeyPrefix + "p1":
				return valkeytest.MakeStringResult(string(data1))
			case party.DefaultLobbyKeyPrefix + "p2":
				return valkeytest.MakeStringResult(string(data2))
			case party.DefaultLobbyKeyPrefix + "corrupt_p4":
				return valkeytest.MakeStringResult("invalid json")
			default:
				return valkeytest.MakeNilResult()
			}
		}
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	// Filter by StatusRecruiting
	summaries, total, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if total != 1 || len(summaries) != 1 {
		t.Fatalf("expected 1 recruiting party, got total=%d len=%d", total, len(summaries))
	}
	if summaries[0].ID != "p1" || summaries[0].LeaderName != "LeaderHero" {
		t.Errorf("unexpected summary: %+v", summaries[0])
	}

	// Test offset beyond total
	emptySummaries, _, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 5)
	if err != nil || len(emptySummaries) != 0 {
		t.Errorf("expected empty summaries for offset beyond total, got %v", emptySummaries)
	}

	// 2. Error case: ZREVRANGE fails
	errZrev := errors.New("zrevrange error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "ZREVRANGE" {
			return valkeytest.MakeErrorResult(errZrev)
		}
		return valkeytest.MakeOKResult()
	}))
	repoErr := party.NewValkeyRepository(clientErr)
	if _, _, err := repoErr.ListParties(ctx, "", 10, 0); !errors.Is(err, errZrev) {
		t.Fatalf("expected errZrev, got %v", err)
	}
}

func TestValkeyRepository_UpdateParty(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-upd-1", "char-lead-1", "更新パーティ")

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.UpdateParty(ctx, p); err != nil {
		t.Fatalf("UpdateParty failed: %v", err)
	}

	// 2. Error mapping: ERR_PARTY_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_PARTY_NOT_FOUND"))
	}))
	repoNotFound := party.NewValkeyRepository(clientNotFound)
	if err := repoNotFound.UpdateParty(ctx, p); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. Generic error
	genericErr := errors.New("lua update error")
	clientGeneric := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(genericErr)
	}))
	repoGeneric := party.NewValkeyRepository(clientGeneric)
	if err := repoGeneric.UpdateParty(ctx, p); !errors.Is(err, genericErr) {
		t.Fatalf("expected genericErr, got %v", err)
	}
}

func TestValkeyRepository_DeleteParty(t *testing.T) {
	ctx := context.Background()
	delErr := errors.New("valkey delete failure")
	for _, want := range []error{nil, delErr} {
		client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
			c := cmd.Commands()
			if len(c) < 7 || c[5] != "delete" || c[6] != "party-del-1" {
				t.Fatalf("unexpected delete command: %v", c)
			}
			if want != nil {
				return valkeytest.MakeErrorResult(want)
			}
			return valkeytest.MakeOKResult()
		}))
		if err := party.NewValkeyRepository(client).DeleteParty(ctx, "party-del-1"); !errors.Is(err, want) {
			t.Fatalf("DeleteParty error = %v, want %v", err, want)
		}
	}
}

func TestValkeyRepository_AddMember(t *testing.T) {
	ctx := context.Background()
	m := party.Member{
		PartyID:       "party-add-1",
		CharacterID:   "char-m1",
		CharacterName: "HeroM1",
	}

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)
	if err := repo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// 3. Error mapping: ERR_PARTY_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_PARTY_NOT_FOUND"))
	}))
	repoNotFound := party.NewValkeyRepository(clientNotFound)
	if err := repoNotFound.AddMember(ctx, m); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 4. Error mapping: ERR_PARTY_FULL
	clientFull := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_PARTY_FULL"))
	}))
	repoFull := party.NewValkeyRepository(clientFull)
	if err := repoFull.AddMember(ctx, m); !errors.Is(err, party.ErrPartyFull) {
		t.Fatalf("expected ErrPartyFull, got %v", err)
	}
}

func TestValkeyRepository_GetMembersAndGetMember(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-m-1", "char-lead-1", "メンバー確認隊")
	members := sampleTestMembers("party-m-1", "char-lead-1")
	state := party.LobbyState{Party: p, Members: members}
	validData, _ := json.Marshal(state)

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if c[0] == "GET" && c[1] == party.DefaultLobbyKeyPrefix+"party-m-1" {
			return valkeytest.MakeStringResult(string(validData))
		}
		return valkeytest.MakeNilResult()
	}))
	repo := party.NewValkeyRepository(client)

	gotMembers, err := repo.GetMembers(ctx, "party-m-1")
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if len(gotMembers) != 2 {
		t.Fatalf("expected 2 members, got %d", len(gotMembers))
	}
	if !gotMembers[0].IsLeader {
		t.Errorf("expected leader first, got %+v", gotMembers[0])
	}

	// Test GetMember
	m, err := repo.GetMember(ctx, "party-m-1", "char-lead-1")
	if err != nil || m.CharacterID != "char-lead-1" {
		t.Fatalf("GetMember failed: got %+v, err=%v", m, err)
	}

	// Test GetMember not in party
	_, err = repo.GetMember(ctx, "party-m-1", "nonexistent-char")
	if !errors.Is(err, party.ErrCharacterNotInParty) {
		t.Errorf("expected ErrCharacterNotInParty, got %v", err)
	}

	// 2. Not found / nil case
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoNil := party.NewValkeyRepository(clientNil)
	nilMembers, err := repoNil.GetMembers(ctx, "party-nonexistent")
	if err != nil || nilMembers != nil {
		t.Errorf("expected nil members for nonexistent party, got %v, err=%v", nilMembers, err)
	}

	// 3. Malformed JSON
	clientCorrupt := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("bad json")
	}))
	repoCorrupt := party.NewValkeyRepository(clientCorrupt)
	if _, err := repoCorrupt.GetMembers(ctx, "party-corrupt"); err == nil {
		t.Errorf("expected error on bad JSON")
	}
}

func TestValkeyRepository_GetActivePartyByCharacter(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-active-1", "char-lead-1", "アクティブ隊")
	members := sampleTestMembers("party-active-1", "char-lead-1")
	validData, _ := json.Marshal(party.LobbyState{Party: p, Members: members})

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if c[0] == "GET" && c[1] == party.DefaultCharacterKeyPrefix+"char-lead-1" {
			return valkeytest.MakeStringResult("party-active-1")
		}
		if c[0] == "GET" && c[1] == party.DefaultLobbyKeyPrefix+"party-active-1" {
			return valkeytest.MakeStringResult(string(validData))
		}
		if c[0] == "EXISTS" {
			return valkeytest.MakeIntResult(1)
		}
		return valkeytest.MakeNilResult()
	}))
	repo := party.NewValkeyRepository(client)

	gotParty, gotMember, err := repo.GetActivePartyByCharacter(ctx, "char-lead-1")
	if err != nil {
		t.Fatalf("GetActivePartyByCharacter failed: %v", err)
	}
	if gotParty.ID != "party-active-1" || gotMember.CharacterID != "char-lead-1" {
		t.Errorf("unexpected active party/member: party=%+v, member=%+v", gotParty, gotMember)
	}

	// 2. Character not in any party (character key nil)
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeNilResult()
	}))
	repoNil := party.NewValkeyRepository(clientNil)
	_, _, err = repoNil.GetActivePartyByCharacter(ctx, "char-solo")
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound for solo character, got %v", err)
	}

	// 3. Stale character index: party not found
	clientStale := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if c[0] == "GET" && c[1] == party.DefaultCharacterKeyPrefix+"char-stale" {
			return valkeytest.MakeStringResult("party-dead")
		}
		return valkeytest.MakeNilResult()
	}))
	repoStale := party.NewValkeyRepository(clientStale)
	_, _, err = repoStale.GetActivePartyByCharacter(ctx, "char-stale")
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound for stale character party, got %v", err)
	}

	// 4. Character key command error
	errKey := errors.New("valkey character key error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errKey)
	}))
	repoErr := party.NewValkeyRepository(clientErr)
	if _, _, err := repoErr.GetActivePartyByCharacter(ctx, "char-err"); !errors.Is(err, errKey) {
		t.Fatalf("expected errKey, got %v", err)
	}
	for _, failRead := range []int{1, 2} {
		reads := 0
		client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(_ context.Context, cmd valkey.Completed) valkey.ValkeyResult {
			c := cmd.Commands()
			if c[0] != "GET" && c[0] != "EXISTS" {
				t.Fatalf("observation mutated storage: %v", c)
			}
			if c[0] == "EXISTS" {
				return valkeytest.MakeIntResult(1)
			}
			reads++
			if reads == failRead {
				return valkeytest.MakeErrorResult(errKey)
			}
			if reads == 1 {
				return valkeytest.MakeStringResult("party-active-1")
			}
			return valkeytest.MakeStringResult(string(validData))
		}))
		if _, _, err := party.NewValkeyRepository(client).GetActivePartyByCharacter(ctx, "char-lead-1"); !errors.Is(err, errKey) {
			t.Fatalf("read %d error suppressed: %v", failRead, err)
		}
	}
}

func TestValkeyRepository_RemoveMember(t *testing.T) {
	ctx := context.Background()

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.RemoveMember(ctx, "party-1", "char-1"); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}

	// 2. Error mapping: ERR_PARTY_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_PARTY_NOT_FOUND"))
	}))
	repoNotFound := party.NewValkeyRepository(clientNotFound)
	if err := repoNotFound.RemoveMember(ctx, "party-1", "char-1"); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. Generic error
	genericErr := errors.New("lua remove error")
	clientGeneric := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(genericErr)
	}))
	repoGeneric := party.NewValkeyRepository(clientGeneric)
	if err := repoGeneric.RemoveMember(ctx, "party-1", "char-1"); !errors.Is(err, genericErr) {
		t.Fatalf("expected genericErr, got %v", err)
	}
}

func TestLobbyState_UnmarshalJSON(t *testing.T) {
	// Case 1: normal array
	jsonNormal := `{"party":{"id":"p1"},"members":[{"character_id":"c1"}]}`
	var sNormal party.LobbyState
	if err := json.Unmarshal([]byte(jsonNormal), &sNormal); err != nil || len(sNormal.Members) != 1 {
		t.Errorf("failed normal unmarshal: %+v, err=%v", sNormal, err)
	}

	// Case 2: empty Lua table "{}"
	jsonLuaEmpty := `{"party":{"id":"p2"},"members":{}}`
	var sLuaEmpty party.LobbyState
	if err := json.Unmarshal([]byte(jsonLuaEmpty), &sLuaEmpty); err != nil || len(sLuaEmpty.Members) != 0 {
		t.Errorf("failed empty lua table unmarshal: %+v, err=%v", sLuaEmpty, err)
	}

	// Case 3: null members
	jsonNull := `{"party":{"id":"p3"},"members":null}`
	var sNull party.LobbyState
	if err := json.Unmarshal([]byte(jsonNull), &sNull); err != nil || len(sNull.Members) != 0 {
		t.Errorf("failed null unmarshal: %+v, err=%v", sNull, err)
	}

	// Case 4: empty string
	jsonEmpty := `{"party":{"id":"p4"},"members":""}`
	var sEmpty party.LobbyState
	if err := json.Unmarshal([]byte(jsonEmpty), &sEmpty); err == nil {
		t.Errorf("expected error on empty string members")
	}

	// Case 5: corrupted top-level JSON
	var sCorrupt party.LobbyState
	if err := json.Unmarshal([]byte("bad json"), &sCorrupt); err == nil {
		t.Errorf("expected error on bad JSON")
	}
}

func TestMemoryRepository_BattleActorRoundTripAndPublicJSON(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()
	now := time.Now().UTC()
	p := sampleTestParty("party-actor-1", "char-lead-1", "戦闘行テスト")
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	held := "item-113"
	actor := &corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID:                "char-lead-1",
			Name:              "LeaderHero",
			HP:                100,
			MaxHP:             200,
			MP:                20,
			MaxMP:             120,
			Attack:            55,
			Defense:           33,
			Agility:           22,
			Gear:              &corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01"},
			ItemDefinitionIDs: []string{"weapon-01", "armor-01"},
		},
		HeldToolID: &held,
		Tension:    1,
		Hit:        95,
	}
	m := party.Member{
		PartyID:       p.ID,
		CharacterID:   "char-lead-1",
		CharacterName: "LeaderHero",
		JobID:         "job-01",
		Level:         10,
		HP:            100,
		MaxHP:         200,
		IsLeader:      true,
		JoinedAt:      now,
		BattleActor:   actor,
	}
	if err := repo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// Mutating caller pointer must not mutate stored snapshot.
	actor.MaxHP = 999
	actor.Gear.WeaponID = "changed"

	members, err := repo.GetMembers(ctx, p.ID)
	if err != nil || len(members) != 1 || members[0].BattleActor == nil {
		t.Fatalf("GetMembers failed: members=%+v err=%v", members, err)
	}
	got := members[0].BattleActor
	if got.MaxHP != 200 || got.MaxMP != 120 || got.Gear == nil || got.Gear.WeaponID != "weapon-01" || got.HeldToolID == nil || *got.HeldToolID != "item-113" {
		t.Fatalf("stored BattleActor mutated or lost fields: %+v", got)
	}

	// Public PartyDetail JSON must never expose BattleActor or internal battle_actor_json.
	publicJSON, err := json.Marshal(party.PartyDetail{Party: p, Members: members})
	if err != nil {
		t.Fatalf("Marshal PartyDetail failed: %v", err)
	}
	if strings.Contains(string(publicJSON), "battle_actor") || strings.Contains(string(publicJSON), "held_tool_id") || strings.Contains(string(publicJSON), "weapon-01") {
		t.Fatalf("public lobby JSON exposed private battle row fields: %s", string(publicJSON))
	}
}

func TestValkeyRepository_PasswordHashRoundTripAndPublicJSON(t *testing.T) {
	ctx := context.Background()
	p := sampleTestParty("party-pw-1", "char-lead-1", "合言葉隊")
	const wantHash = "deadbeef1234567890abcdef"
	p.PasswordHash = wantHash
	members := sampleTestMembers(p.ID, p.LeaderCharacterID)

	var storedPayload string
	var updatePartyArg string

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		switch c[0] {
		case "GET":
			if c[1] == party.DefaultLobbyKeyPrefix+p.ID {
				return valkeytest.MakeStringResult(storedPayload)
			}
			if c[1] == party.DefaultCharacterKeyPrefix+p.LeaderCharacterID {
				return valkeytest.MakeStringResult(p.ID)
			}
			return valkeytest.MakeNilResult()
		case "ZREVRANGE":
			return valkeytest.MakeStringSliceResult([]string{p.ID})
		case "EXISTS":
			return valkeytest.MakeIntResult(1)
		case "EVAL", "EVALSHA":
			if len(c) == 13 && c[3] == party.DefaultLobbyKeyPrefix+p.ID && c[4] == party.DefaultLobbiesIndexKey {
				switch c[5] {
				case "save":
					storedPayload = c[12]
				case "update":
					updatePartyArg = c[12]
				}
			}
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.SaveParty(ctx, p, members[0]); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}
	if !strings.Contains(storedPayload, `"password_hash":"`+wantHash+`"`) {
		t.Fatalf("expected stored Valkey payload to contain password_hash, got: %s", storedPayload)
	}
	gotParty, err := repo.GetParty(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if gotParty.PasswordHash != wantHash {
		t.Fatalf("GetParty PasswordHash = %q, want %q", gotParty.PasswordHash, wantHash)
	}

	gotForUpdate, err := repo.GetPartyForUpdate(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetPartyForUpdate failed: %v", err)
	}
	if gotForUpdate.PasswordHash != wantHash {
		t.Fatalf("GetPartyForUpdate PasswordHash = %q, want %q", gotForUpdate.PasswordHash, wantHash)
	}

	summaries, total, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if total != 1 || len(summaries) != 1 || !summaries[0].HasPassword {
		t.Fatalf("expected HasPassword=true in ListParties summary, got total=%d summaries=%+v", total, summaries)
	}

	activeParty, _, err := repo.GetActivePartyByCharacter(ctx, p.LeaderCharacterID)
	if err != nil {
		t.Fatalf("GetActivePartyByCharacter failed: %v", err)
	}
	if activeParty.PasswordHash != wantHash {
		t.Fatalf("GetActivePartyByCharacter PasswordHash = %q, want %q", activeParty.PasswordHash, wantHash)
	}

	if err := repo.UpdateParty(ctx, gotParty); err != nil {
		t.Fatalf("UpdateParty failed: %v", err)
	}
	if !strings.Contains(updatePartyArg, `"password_hash":"`+wantHash+`"`) {
		t.Fatalf("expected UpdateParty Lua payload to contain password_hash, got: %s", updatePartyArg)
	}

	publicDetailJSON, err := json.Marshal(party.PartyDetail{Party: gotParty, Members: members})
	if err != nil {
		t.Fatalf("Marshal PartyDetail failed: %v", err)
	}
	if strings.Contains(string(publicDetailJSON), "password_hash") || strings.Contains(string(publicDetailJSON), wantHash) {
		t.Fatalf("public PartyDetail JSON exposed password hash: %s", string(publicDetailJSON))
	}

	publicListJSON, err := json.Marshal(summaries)
	if err != nil {
		t.Fatalf("Marshal PartySummary failed: %v", err)
	}
	if strings.Contains(string(publicListJSON), "password_hash") || strings.Contains(string(publicListJSON), wantHash) {
		t.Fatalf("public PartySummary JSON exposed password hash: %s", string(publicListJSON))
	}
}
