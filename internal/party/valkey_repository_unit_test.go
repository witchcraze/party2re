package party_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
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
		Speed:             party.DefaultSpeed,
		MaxMembers:        4,
		MinLevel:          1,
		MaxLevel:          99,
		MinHP:             10,
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
			ReadyState:    true,
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
			ReadyState:    false,
			JoinedAt:      now.Add(2 * time.Second),
		},
	}
}

func TestValkeyRepository_NewWithOptions(t *testing.T) {
	client := valkeytest.NewMockClient()
	dummy := &dummyLogRepo{}

	repo := party.NewValkeyRepository(
		client,
		party.WithLobbyKeyPrefix("test:lobby:"),
		party.WithReadyKeyPrefix("test:ready:"),
		party.WithCharacterKeyPrefix("test:char:"),
		party.WithLobbiesIndexKey("test:lobbies"),
		party.WithLobbyTTL(10*time.Minute),
		party.WithReadyTTL(30*time.Second),
		party.WithDurableLogRepository(dummy),
	)
	if repo == nil {
		t.Fatalf("expected non-nil ValkeyRepository")
	}

	// Test SaveAdventureLog delegation
	log := party.PartyAdventureLog{
		ID:       "adv-test-1",
		PartyID:  "party-1",
		Outcome:  "win",
		TotalEXP: 100,
	}
	if err := repo.SaveAdventureLog(context.Background(), log); err != nil {
		t.Fatalf("SaveAdventureLog failed: %v", err)
	}
	if len(dummy.logs) != 1 || dummy.logs[0].ID != "adv-test-1" {
		t.Errorf("expected log delegated to dummy repo: %+v", dummy.logs)
	}
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
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands, got %d: %v", len(cmds), cmds)
	}

	// First command: SET lobbyKey
	if cmds[0][0] != "SET" || cmds[0][1] != party.DefaultLobbyKeyPrefix+"party-save-1" {
		t.Errorf("unexpected SET lobby command: %v", cmds[0])
	}
	var state party.LobbyState
	if err := json.Unmarshal([]byte(cmds[0][2]), &state); err != nil {
		t.Fatalf("saved lobby payload is invalid JSON: %v", err)
	}
	if state.Party.ID != "party-save-1" || state.Party.LeaderCharacterID != "char-lead-1" {
		t.Errorf("unexpected saved state: %+v", state)
	}

	// Second command: ZADD lobbiesIndexKey
	if cmds[1][0] != "ZADD" || cmds[1][1] != party.DefaultLobbiesIndexKey || cmds[1][3] != "party-save-1" {
		t.Errorf("unexpected ZADD command: %v", cmds[1])
	}

	// Third command: SET characterKey for leader
	if cmds[2][0] != "SET" || cmds[2][1] != party.DefaultCharacterKeyPrefix+"char-lead-1" || cmds[2][2] != "party-save-1" {
		t.Errorf("unexpected SET character command: %v", cmds[2])
	}

	// 2. Error case: SET returns error
	errSet := errors.New("valkey set error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "SET" {
			return valkeytest.MakeErrorResult(errSet)
		}
		return valkeytest.MakeOKResult()
	}))
	repoErr := party.NewValkeyRepository(clientErr)
	if err := repoErr.SaveParty(ctx, p); !errors.Is(err, errSet) {
		t.Fatalf("expected errSet, got %v", err)
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

	// 3. Not found: Disbanded status
	disbandedState := state
	disbandedState.Party.Status = party.StatusDisbanded
	disbandedData, _ := json.Marshal(disbandedState)
	clientDisbanded := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult(string(disbandedData))
	}))
	repoDisbanded := party.NewValkeyRepository(clientDisbanded)
	_, err = repoDisbanded.GetParty(ctx, "party-disbanded")
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound on disbanded party, got %v", err)
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
	p := sampleTestParty("party-del-1", "char-lead-1", "解散パーティ")
	state := party.LobbyState{
		Party:   p,
		Members: sampleTestMembers("party-del-1", "char-lead-1"),
	}
	validData, _ := json.Marshal(state)

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" {
			return valkeytest.MakeStringResult(string(validData))
		}
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.DeleteParty(ctx, "party-del-1"); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	// GET, DEL char1, DEL ready1, DEL char2, DEL ready2, DEL leader char1, DEL lobbyKey, ZREM
	if len(cmds) < 4 {
		t.Fatalf("expected cleanup commands, got %d: %v", len(cmds), cmds)
	}
}

func TestValkeyRepository_AddMember(t *testing.T) {
	ctx := context.Background()
	m := party.Member{
		PartyID:       "party-add-1",
		CharacterID:   "char-m1",
		CharacterName: "HeroM1",
		ReadyState:    true,
	}

	// 1. Success case (ReadyState = true)
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)
	if err := repo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// 2. Success case (ReadyState = false)
	mNotReady := m
	mNotReady.ReadyState = false
	if err := repo.AddMember(ctx, mNotReady); err != nil {
		t.Fatalf("AddMember (not ready) failed: %v", err)
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

	// 1. Success case: EXISTS returns 1 for leader, 0 for ally
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		c := cmd.Commands()
		if c[0] == "GET" && c[1] == party.DefaultLobbyKeyPrefix+"party-m-1" {
			return valkeytest.MakeStringResult(string(validData))
		}
		if c[0] == "EXISTS" {
			if strings.Contains(c[1], "char-lead-1") {
				return valkeytest.MakeIntResult(1)
			}
			return valkeytest.MakeIntResult(0)
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
	if !gotMembers[0].IsLeader || !gotMembers[0].ReadyState {
		t.Errorf("expected leader first and ready=true, got %+v", gotMembers[0])
	}
	if gotMembers[1].ReadyState {
		t.Errorf("expected ally ready=false, got true")
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

	// Test CountMembers
	count, err := repo.CountMembers(ctx, "party-m-1")
	if err != nil || count != 2 {
		t.Errorf("expected count 2, got %d, err=%v", count, err)
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

func TestValkeyRepository_UpdateMemberReady(t *testing.T) {
	ctx := context.Background()

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo := party.NewValkeyRepository(client)

	if err := repo.UpdateMemberReady(ctx, "party-1", "char-1", true); err != nil {
		t.Fatalf("UpdateMemberReady (true) failed: %v", err)
	}
	if err := repo.UpdateMemberReady(ctx, "party-1", "char-1", false); err != nil {
		t.Fatalf("UpdateMemberReady (false) failed: %v", err)
	}

	// 2. Error mapping: ERR_PARTY_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_PARTY_NOT_FOUND"))
	}))
	repoNotFound := party.NewValkeyRepository(clientNotFound)
	if err := repoNotFound.UpdateMemberReady(ctx, "party-1", "char-1", true); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// 3. Error mapping: ERR_CHAR_NOT_IN_PARTY
	clientNotInParty := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_CHAR_NOT_IN_PARTY"))
	}))
	repoNotInParty := party.NewValkeyRepository(clientNotInParty)
	if err := repoNotInParty.UpdateMemberReady(ctx, "party-1", "char-1", true); !errors.Is(err, party.ErrCharacterNotInParty) {
		t.Fatalf("expected ErrCharacterNotInParty, got %v", err)
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
