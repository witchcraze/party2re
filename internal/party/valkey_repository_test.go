package party_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/party"
)

type dummyLogRepo struct {
	logs []party.PartyAdventureLog
}

func (d *dummyLogRepo) SaveAdventureLog(_ context.Context, log party.PartyAdventureLog) error {
	d.logs = append(d.logs, log)
	return nil
}

func TestValkeyRepository_InMemory_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo := party.NewValkeyRepository(nil)

	now := time.Now().UTC()
	p := party.Party{
		ID:                "party-1",
		LeaderCharacterID: "char-1",
		Name:              "勇敢なる旅人たち",
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

	// 1. SaveParty & GetParty
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	gotParty, err := repo.GetParty(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if gotParty.ID != p.ID || gotParty.Name != p.Name {
		t.Errorf("got party %+v, want %+v", gotParty, p)
	}

	// 2. AddMember (Leader)
	leaderMember := party.Member{
		PartyID:       p.ID,
		CharacterID:   "char-1",
		CharacterName: "勇者アリス",
		JobID:         "job-hero",
		Level:         15,
		HP:            100,
		MaxHP:         100,
		IsLeader:      true,
		ReadyState:    true,
		JoinedAt:      now,
	}
	if err := repo.AddMember(ctx, leaderMember); err != nil {
		t.Fatalf("AddMember leader failed: %v", err)
	}

	// 3. AddMember (Second member)
	subMember := party.Member{
		PartyID:       p.ID,
		CharacterID:   "char-2",
		CharacterName: "戦士ボブ",
		JobID:         "job-warrior",
		Level:         14,
		HP:            120,
		MaxHP:         120,
		IsLeader:      false,
		ReadyState:    false,
		JoinedAt:      now.Add(1 * time.Second),
	}
	if err := repo.AddMember(ctx, subMember); err != nil {
		t.Fatalf("AddMember member failed: %v", err)
	}

	// 4. CountMembers & GetMembers
	count, err := repo.CountMembers(ctx, p.ID)
	if err != nil {
		t.Fatalf("CountMembers failed: %v", err)
	}
	if count != 2 {
		t.Errorf("got member count %d, want 2", count)
	}

	members, err := repo.GetMembers(ctx, p.ID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}
	if !members[0].IsLeader || members[0].CharacterID != "char-1" {
		t.Errorf("expected leader to be first in member list, got %+v", members[0])
	}

	// 5. GetActivePartyByCharacter
	activeP, activeM, err := repo.GetActivePartyByCharacter(ctx, "char-2")
	if err != nil {
		t.Fatalf("GetActivePartyByCharacter failed: %v", err)
	}
	if activeP.ID != p.ID || activeM.CharacterID != "char-2" {
		t.Errorf("got active party %s member %s, want %s / char-2", activeP.ID, activeM.CharacterID, p.ID)
	}

	// 6. UpdateMemberReady
	if err := repo.UpdateMemberReady(ctx, p.ID, "char-2", true); err != nil {
		t.Fatalf("UpdateMemberReady failed: %v", err)
	}
	updatedM, err := repo.GetMember(ctx, p.ID, "char-2")
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if !updatedM.ReadyState {
		t.Errorf("expected ReadyState true, got false")
	}

	// 7. ListParties
	summaries, total, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if total != 1 || len(summaries) != 1 {
		t.Errorf("expected 1 summary, got total=%d len=%d", total, len(summaries))
	}
	if summaries[0].CurrentMembers != 2 || summaries[0].LeaderName != "勇者アリス" {
		t.Errorf("unexpected summary details: %+v", summaries[0])
	}

	// 8. RemoveMember
	if err := repo.RemoveMember(ctx, p.ID, "char-2"); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	count, _ = repo.CountMembers(ctx, p.ID)
	if count != 1 {
		t.Errorf("expected 1 member after remove, got %d", count)
	}
	_, _, err = repo.GetActivePartyByCharacter(ctx, "char-2")
	if err != party.ErrNotFound {
		t.Errorf("expected ErrNotFound for removed member, got %v", err)
	}

	// 9. DeleteParty
	if err := repo.DeleteParty(ctx, p.ID); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}
	_, err = repo.GetParty(ctx, p.ID)
	if err != party.ErrNotFound {
		t.Errorf("expected ErrNotFound after DeleteParty, got %v", err)
	}
}

func TestValkeyRepository_InMemory_ReadyCountdownExpiry(t *testing.T) {
	ctx := context.Background()
	// Set 50ms ready countdown TTL
	repo := party.NewValkeyRepository(nil, party.WithReadyTTL(50*time.Millisecond))

	p := party.Party{
		ID:        "party-countdown",
		Name:      "秒読みパーティ",
		Status:    party.StatusRecruiting,
		CreatedAt: time.Now().UTC(),
	}
	_ = repo.SaveParty(ctx, p)
	m := party.Member{
		PartyID:     p.ID,
		CharacterID: "char-countdown",
		ReadyState:  false,
	}
	_ = repo.AddMember(ctx, m)

	// Set ready = true
	if err := repo.UpdateMemberReady(ctx, p.ID, m.CharacterID, true); err != nil {
		t.Fatalf("UpdateMemberReady failed: %v", err)
	}

	// Immediately check: should be ready
	gotM, err := repo.GetMember(ctx, p.ID, m.CharacterID)
	if err != nil {
		t.Fatalf("GetMember failed: %v", err)
	}
	if !gotM.ReadyState {
		t.Errorf("expected ready state true before expiry")
	}

	// Wait 70ms for countdown to expire
	time.Sleep(70 * time.Millisecond)

	// Check again: ready state must auto-expire to false
	gotM, err = repo.GetMember(ctx, p.ID, m.CharacterID)
	if err != nil {
		t.Fatalf("GetMember after expiry failed: %v", err)
	}
	if gotM.ReadyState {
		t.Errorf("expected ready state false after countdown expiry, got true")
	}
}

func TestValkeyRepository_InMemory_LobbyExpiry(t *testing.T) {
	ctx := context.Background()
	// Set 50ms lobby TTL
	repo := party.NewValkeyRepository(nil, party.WithLobbyTTL(50*time.Millisecond))

	p := party.Party{
		ID:                "party-abandoned",
		LeaderCharacterID: "char-lead",
		Name:              "放置ロビー",
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	_ = repo.SaveParty(ctx, p)

	// Before expiry
	_, err := repo.GetParty(ctx, p.ID)
	if err != nil {
		t.Fatalf("expected party found before expiry, got %v", err)
	}

	// Wait 70ms for lobby to expire
	time.Sleep(70 * time.Millisecond)

	// After expiry
	_, err = repo.GetParty(ctx, p.ID)
	if err != party.ErrNotFound {
		t.Errorf("expected ErrNotFound for expired lobby, got %v", err)
	}

	// Listing should be empty
	summaries, total, err := repo.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if total != 0 || len(summaries) != 0 {
		t.Errorf("expected 0 parties after expiry, got total=%d len=%d", total, len(summaries))
	}
}

func TestValkeyRepository_DurableLogDelegation(t *testing.T) {
	ctx := context.Background()
	dummy := &dummyLogRepo{}
	repo := party.NewValkeyRepository(nil, party.WithDurableLogRepository(dummy))

	log := party.PartyAdventureLog{
		ID:                  "adv-123",
		PartyID:             "party-1",
		StageID:             "stage-1",
		Outcome:             "win",
		Turns:               5,
		TotalEXP:            100,
		TotalGold:           50,
		SynergyBonusPercent: 20,
		CreatedAt:           time.Now().UTC(),
	}

	if err := repo.SaveAdventureLog(ctx, log); err != nil {
		t.Fatalf("SaveAdventureLog failed: %v", err)
	}

	if len(dummy.logs) != 1 || dummy.logs[0].ID != "adv-123" {
		t.Errorf("expected log delegated to durable repository, got %+v", dummy.logs)
	}
}

func TestValkeyRepository_LiveValkey(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "localhost:6379"
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{valkeyAddr},
	})
	if err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Skipf("skipping live Valkey test (cannot ping Valkey at %s): %v", valkeyAddr, err)
	}

	testLobbyPrefix := "party2:test:party:lobby:"
	testReadyPrefix := "party2:test:party:ready:"
	testCharPrefix := "party2:test:party:character:"
	testIndexKey := "party2:test:party:lobbies"

	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(testLobbyPrefix),
		party.WithReadyKeyPrefix(testReadyPrefix),
		party.WithCharacterKeyPrefix(testCharPrefix),
		party.WithLobbiesIndexKey(testIndexKey),
		party.WithLobbyTTL(10*time.Minute),
		party.WithReadyTTL(1*time.Second),
	)

	partyID := "test-party-live-1"
	leaderID := "test-char-live-1"
	memberID := "test-char-live-2"

	// Cleanup before/after
	cleanup := func() {
		_ = client.Do(ctx, client.B().Del().Key(testLobbyPrefix+partyID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+leaderID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+memberID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testReadyPrefix+partyID+":"+leaderID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testReadyPrefix+partyID+":"+memberID).Build()).Error()
		_ = client.Do(ctx, client.B().Zrem().Key(testIndexKey).Member(partyID).Build()).Error()
	}
	cleanup()
	defer cleanup()

	now := time.Now().UTC()
	p := party.Party{
		ID:                partyID,
		LeaderCharacterID: leaderID,
		Name:              "実Valkeyテストパーティ",
		StageID:           "stage-1",
		Speed:             3,
		MaxMembers:        4,
		MinLevel:          1,
		MaxLevel:          99,
		MinHP:             10,
		Status:            party.StatusRecruiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	// 1. SaveParty
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	// Verify key exists in Valkey
	exists, err := client.Do(ctx, client.B().Exists().Key(testLobbyPrefix+partyID).Build()).AsInt64()
	if err != nil || exists != 1 {
		t.Fatalf("lobby key not found in Valkey: exists=%d err=%v", exists, err)
	}

	// 2. Add Leader and Member
	_ = repo.AddMember(ctx, party.Member{
		PartyID:       partyID,
		CharacterID:   leaderID,
		CharacterName: "リーダー",
		IsLeader:      true,
		ReadyState:    true,
		JoinedAt:      now,
	})
	_ = repo.AddMember(ctx, party.Member{
		PartyID:       partyID,
		CharacterID:   memberID,
		CharacterName: "メンバー",
		IsLeader:      false,
		ReadyState:    false,
		JoinedAt:      now.Add(time.Second),
	})

	// 3. Verify GetParty and GetMembers
	gotP, err := repo.GetParty(ctx, partyID)
	if err != nil {
		t.Fatalf("GetParty failed: %v", err)
	}
	if gotP.Name != p.Name {
		t.Errorf("got party name %q, want %q", gotP.Name, p.Name)
	}

	members, err := repo.GetMembers(ctx, partyID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if len(members) != 2 {
		t.Fatalf("got %d members, want 2", len(members))
	}

	// 4. Test Ready Check countdown in live Valkey
	if err := repo.UpdateMemberReady(ctx, partyID, memberID, true); err != nil {
		t.Fatalf("UpdateMemberReady failed: %v", err)
	}
	m, err := repo.GetMember(ctx, partyID, memberID)
	if err != nil || !m.ReadyState {
		t.Errorf("expected ReadyState true immediately, got %v (err=%v)", m.ReadyState, err)
	}

	// Wait 1.1s for ready countdown key to expire in Valkey
	time.Sleep(1100 * time.Millisecond)

	m, err = repo.GetMember(ctx, partyID, memberID)
	if err != nil {
		t.Fatalf("GetMember after ready expiry failed: %v", err)
	}
	if m.ReadyState {
		t.Errorf("expected ReadyState false after countdown expiry, got true")
	}

	// 5. DeleteParty cleans up all keys
	if err := repo.DeleteParty(ctx, partyID); err != nil {
		t.Fatalf("DeleteParty failed: %v", err)
	}

	exists, _ = client.Do(ctx, client.B().Exists().Key(testLobbyPrefix+partyID).Build()).AsInt64()
	if exists != 0 {
		t.Errorf("expected lobby key deleted, got exists=%d", exists)
	}
	charExists, _ := client.Do(ctx, client.B().Exists().Key(testCharPrefix+leaderID).Build()).AsInt64()
	if charExists != 0 {
		t.Errorf("expected character index deleted, got exists=%d", charExists)
	}
}
