package party_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
)

type dummyLogRepo struct {
	logs []party.PartyAdventureLog
}

func (d *dummyLogRepo) SaveAdventureLog(_ context.Context, log party.PartyAdventureLog) error {
	d.logs = append(d.logs, log)
	return nil
}

func TestMemoryRepository_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()

	now := time.Now().UTC()
	p := party.Party{
		ID:                "party-1",
		LeaderCharacterID: "char-1",
		Name:              "勇敢なる旅人たち",
		StageID:           "stage-1",
		Speed:             18,
		MaxMembers:        4,
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

func TestMemoryRepository_LeaderHandover(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()
	now := time.Now().UTC()
	p := party.Party{ID: "party-handover", LeaderCharacterID: "lead", Status: party.StatusRecruiting, CreatedAt: now}
	_ = repo.SaveParty(ctx, p)
	for i, id := range []string{"lead", "first", "second"} {
		if err := repo.AddMember(ctx, party.Member{PartyID: p.ID, CharacterID: id, IsLeader: id == "lead", JoinedAt: now.Add(time.Duration(i) * time.Second)}); err != nil {
			t.Fatal(err)
		}
	}

	if err := repo.RemoveMember(ctx, p.ID, "lead"); err != nil {
		t.Fatalf("RemoveMember(leader) failed: %v", err)
	}
	got, err := repo.GetParty(ctx, p.ID)
	if err != nil || got.LeaderCharacterID != "first" {
		t.Fatalf("leader not handed to the earliest member: %+v (err=%v)", got, err)
	}
	members, _ := repo.GetMembers(ctx, p.ID)
	if len(members) != 2 || members[0].CharacterID != "first" || !members[0].IsLeader || members[1].IsLeader {
		t.Fatalf("unexpected members after handover: %+v", members)
	}

	// Removing a non-leader keeps the leader.
	if err := repo.RemoveMember(ctx, p.ID, "second"); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetParty(ctx, p.ID); got.LeaderCharacterID != "first" {
		t.Fatalf("leader changed on member removal: %+v", got)
	}
}

func TestMemoryRepository_LobbyExpiry(t *testing.T) {
	ctx := context.Background()
	// Set 50ms lobby TTL
	repo := party.NewMemoryRepository(party.WithLobbyTTL(50 * time.Millisecond))

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

func TestMemoryRepository_DurableLogDelegation(t *testing.T) {
	ctx := context.Background()
	dummy := &dummyLogRepo{}
	repo := party.NewMemoryRepository(party.WithDurableLogRepository(dummy))

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
	testCharPrefix := "party2:test:party:character:"
	testIndexKey := "party2:test:party:lobbies"

	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(testLobbyPrefix),
		party.WithCharacterKeyPrefix(testCharPrefix),
		party.WithLobbiesIndexKey(testIndexKey),
		party.WithLobbyTTL(10*time.Minute),
	)

	partyID := "test-party-live-1"
	leaderID := "test-char-live-1"
	memberID := "test-char-live-2"

	// Cleanup before/after
	cleanup := func() {
		_ = client.Do(ctx, client.B().Del().Key(testLobbyPrefix+partyID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+leaderID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+memberID).Build()).Error()
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
		JoinedAt:      now,
	})
	_ = repo.AddMember(ctx, party.Member{
		PartyID:       partyID,
		CharacterID:   memberID,
		CharacterName: "メンバー",
		IsLeader:      false,
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

	// 4. The leader's removal promotes the earliest remaining member atomically.
	if err := repo.RemoveMember(ctx, partyID, leaderID); err != nil {
		t.Fatalf("RemoveMember(leader) failed: %v", err)
	}
	gotP, err = repo.GetParty(ctx, partyID)
	if err != nil || gotP.LeaderCharacterID != memberID {
		t.Fatalf("leader not handed over: %+v (err=%v)", gotP, err)
	}
	members, err = repo.GetMembers(ctx, partyID)
	if err != nil || len(members) != 1 || members[0].CharacterID != memberID || !members[0].IsLeader {
		t.Fatalf("member not promoted: %+v (err=%v)", members, err)
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

func TestMemoryRepository_CapacityEnforcement(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()

	// Non-existent party
	err := repo.AddMember(ctx, party.Member{PartyID: "non-existent", CharacterID: "c1"})
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound for non-existent party, got %v", err)
	}

	p := party.Party{
		ID:                "cap-party-mem",
		LeaderCharacterID: "leader-1",
		Name:              "定員テストパーティ",
		MaxMembers:        2,
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	// Add leader (1/2)
	if err := repo.AddMember(ctx, party.Member{PartyID: p.ID, CharacterID: "leader-1", IsLeader: true}); err != nil {
		t.Fatalf("AddMember leader failed: %v", err)
	}

	// Add second member (2/2)
	if err := repo.AddMember(ctx, party.Member{PartyID: p.ID, CharacterID: "member-2"}); err != nil {
		t.Fatalf("AddMember 2 failed: %v", err)
	}

	// Add third member (3/2) -> Must fail with ErrPartyFull
	err = repo.AddMember(ctx, party.Member{PartyID: p.ID, CharacterID: "member-3"})
	if !errors.Is(err, party.ErrPartyFull) {
		t.Errorf("expected ErrPartyFull when exceeding MaxMembers, got %v", err)
	}

	// Updating existing member must still succeed
	if err := repo.AddMember(ctx, party.Member{PartyID: p.ID, CharacterID: "member-2"}); err != nil {
		t.Errorf("expected update of existing member to succeed, got %v", err)
	}

}

func TestValkeyRepository_CapacityEnforcement_LiveValkey(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "127.0.0.1:6379"
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
	testCharPrefix := "party2:test:party:character:"
	testIndexKey := "party2:test:party:lobbies"

	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(testLobbyPrefix),
		party.WithCharacterKeyPrefix(testCharPrefix),
		party.WithLobbiesIndexKey(testIndexKey),
	)

	partyID := "test-party-cap-live"
	cleanup := func() {
		_ = client.Do(ctx, client.B().Del().Key(testLobbyPrefix+partyID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+"lead").Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+"mem1").Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+"mem2").Build()).Error()
		_ = client.Do(ctx, client.B().Zrem().Key(testIndexKey).Member(partyID).Build()).Error()
	}
	cleanup()
	defer cleanup()

	// Non-existent party
	err = repo.AddMember(ctx, party.Member{PartyID: "non-existent", CharacterID: "c1"})
	if !errors.Is(err, party.ErrNotFound) {
		t.Errorf("expected ErrNotFound for non-existent party in live Valkey, got %v", err)
	}

	p := party.Party{
		ID:                partyID,
		LeaderCharacterID: "lead",
		Name:              "定員テストライブ",
		MaxMembers:        2,
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	// Add leader (1/2)
	if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "lead", IsLeader: true}); err != nil {
		t.Fatalf("AddMember leader failed: %v", err)
	}

	// Add member 1 (2/2)
	if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "mem1"}); err != nil {
		t.Fatalf("AddMember mem1 failed: %v", err)
	}

	// Add member 2 (3/2) -> Must fail with ErrPartyFull
	err = repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "mem2"})
	if !errors.Is(err, party.ErrPartyFull) {
		t.Errorf("expected ErrPartyFull in live Valkey, got %v", err)
	}

	// Update existing member must succeed
	if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "mem1"}); err != nil {
		t.Errorf("expected update of existing member in live Valkey to succeed, got %v", err)
	}

}

func TestMemoryRepository_ConcurrentJoinStress(t *testing.T) {
	ctx := context.Background()
	repo := party.NewMemoryRepository()

	partyID := "stress-party-mem"
	maxMembers := 4
	p := party.Party{
		ID:                partyID,
		LeaderCharacterID: "stress-lead",
		Name:              "並行ストレステスト(メモリ)",
		MaxMembers:        maxMembers,
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	// Add leader (1 member, 3 open slots)
	if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "stress-lead", IsLeader: true}); err != nil {
		t.Fatalf("AddMember leader failed: %v", err)
	}

	numContenders := 50
	var successCount int64
	var fullCount int64
	var otherErrCount int64

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 1; i <= numContenders; i++ {
		wg.Add(1)
		charID := fmt.Sprintf("contender-mem-%d", i)
		go func(cid string) {
			defer wg.Done()
			<-startSignal

			err := repo.AddMember(ctx, party.Member{
				PartyID:       partyID,
				CharacterID:   cid,
				CharacterName: cid,
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, party.ErrPartyFull) {
				atomic.AddInt64(&fullCount, 1)
			} else {
				atomic.AddInt64(&otherErrCount, 1)
			}
		}(charID)
	}

	close(startSignal)
	wg.Wait()

	expectedWins := int64(maxMembers - 1) // 3 available slots
	expectedFulls := int64(numContenders) - expectedWins

	if successCount != expectedWins {
		t.Errorf("expected exactly %d successful joins, got %d", expectedWins, successCount)
	}
	if fullCount != expectedFulls {
		t.Errorf("expected exactly %d ErrPartyFull errors, got %d", expectedFulls, fullCount)
	}
	if otherErrCount != 0 {
		t.Errorf("unexpected other errors count: %d", otherErrCount)
	}

	members, err := repo.GetMembers(ctx, partyID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if len(members) != maxMembers {
		t.Errorf("expected party to have exactly %d members, got %d", maxMembers, len(members))
	}
}

func TestValkeyRepository_ConcurrentJoinStress_LiveValkey(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "127.0.0.1:6379"
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{valkeyAddr},
	})
	if err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}

	testLobbyPrefix := "party2:test:party:lobby:"
	testCharPrefix := "party2:test:party:character:"
	testIndexKey := "party2:test:party:lobbies"

	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(testLobbyPrefix),
		party.WithCharacterKeyPrefix(testCharPrefix),
		party.WithLobbiesIndexKey(testIndexKey),
	)

	partyID := "stress-party-live"
	maxMembers := 4

	cleanup := func() {
		_ = client.Do(ctx, client.B().Del().Key(testLobbyPrefix+partyID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+"stress-live-lead").Build()).Error()
		for i := 1; i <= 60; i++ {
			_ = client.Do(ctx, client.B().Del().Key(fmt.Sprintf("%scontender-live-%d", testCharPrefix, i)).Build()).Error()
		}
		_ = client.Do(ctx, client.B().Zrem().Key(testIndexKey).Member(partyID).Build()).Error()
	}
	cleanup()
	defer cleanup()

	p := party.Party{
		ID:                partyID,
		LeaderCharacterID: "stress-live-lead",
		Name:              "並行ストレステスト(実Valkey)",
		MaxMembers:        maxMembers,
		Status:            party.StatusRecruiting,
		CreatedAt:         time.Now().UTC(),
	}
	if err := repo.SaveParty(ctx, p); err != nil {
		t.Fatalf("SaveParty failed: %v", err)
	}

	if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: "stress-live-lead", IsLeader: true}); err != nil {
		t.Fatalf("AddMember leader failed: %v", err)
	}

	numContenders := 50
	var successCount int64
	var fullCount int64
	var otherErrCount int64

	var wg sync.WaitGroup
	startSignal := make(chan struct{})

	for i := 1; i <= numContenders; i++ {
		wg.Add(1)
		charID := fmt.Sprintf("contender-live-%d", i)
		go func(cid string) {
			defer wg.Done()
			<-startSignal

			err := repo.AddMember(ctx, party.Member{
				PartyID:       partyID,
				CharacterID:   cid,
				CharacterName: cid,
			})
			if err == nil {
				atomic.AddInt64(&successCount, 1)
			} else if errors.Is(err, party.ErrPartyFull) {
				atomic.AddInt64(&fullCount, 1)
			} else {
				atomic.AddInt64(&otherErrCount, 1)
			}
		}(charID)
	}

	close(startSignal)
	wg.Wait()

	expectedWins := int64(maxMembers - 1) // 3 available slots
	expectedFulls := int64(numContenders) - expectedWins

	if successCount != expectedWins {
		t.Errorf("expected exactly %d successful joins in live Valkey, got %d", expectedWins, successCount)
	}
	if fullCount != expectedFulls {
		t.Errorf("expected exactly %d ErrPartyFull in live Valkey, got %d", expectedFulls, fullCount)
	}
	if otherErrCount != 0 {
		t.Errorf("unexpected other errors in live Valkey: %d", otherErrCount)
	}

	members, err := repo.GetMembers(ctx, partyID)
	if err != nil {
		t.Fatalf("GetMembers failed: %v", err)
	}
	if len(members) != maxMembers {
		t.Errorf("expected party to have exactly %d members, got %d", maxMembers, len(members))
	}

	// Verify all members are unique
	seen := make(map[string]bool)
	for _, m := range members {
		if seen[m.CharacterID] {
			t.Errorf("duplicate member %s found in party", m.CharacterID)
		}
		seen[m.CharacterID] = true
	}
}

type liveTestCharRepo struct {
	chars map[string]corecharacter.Character
}

func (r *liveTestCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := r.chars[id]
	if !ok {
		return corecharacter.Character{}, party.ErrNotFound
	}
	return c, nil
}

func (r *liveTestCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}

func (r *liveTestCharRepo) Update(_ context.Context, value corecharacter.Character) error {
	r.chars[value.ID] = value
	return nil
}

type liveTestStageProvider struct{}

func (liveTestStageProvider) FindByID(id string) (adventure.Stage, error) {
	return adventure.Stage{ID: id, Name: "草原"}, nil
}

func (liveTestStageProvider) CanAccessStage(corecharacter.Character, string) error {
	return nil
}

type liveTestMonsterProvider struct{}

func (liveTestMonsterProvider) FindByID(string) (adventure.Monster, error) {
	return adventure.Monster{}, nil
}

// Concurrent departures must leave exactly the remaining member as leader.
func TestValkeyRepository_ConcurrentLeaderLeaveStress_LiveValkey(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "127.0.0.1:6379"
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{valkeyAddr}})
	if err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}

	lobbyPrefix, charPrefix, indexKey := "party2:test:leave:lobby:", "party2:test:leave:character:", "party2:test:leave:lobbies"
	repo := party.NewValkeyRepository(client, party.WithLobbyKeyPrefix(lobbyPrefix), party.WithCharacterKeyPrefix(charPrefix), party.WithLobbiesIndexKey(indexKey))
	cfg := testutil.GetStressConfig()
	for round := 0; round < cfg.OpsPerWorker; round++ {
		partyID := fmt.Sprintf("leave-stress-%d-%d", time.Now().UnixNano(), round)
		ids := []string{partyID + "-lead", partyID + "-a", partyID + "-b", partyID + "-c", partyID + "-stay"}
		t.Cleanup(func() {
			_ = repo.DeleteParty(context.Background(), partyID)
			for _, id := range ids {
				_ = client.Do(context.Background(), client.B().Del().Key(charPrefix+id).Build()).Error()
			}
		})
		now := time.Now().UTC()
		if err := repo.SaveParty(ctx, party.Party{ID: partyID, LeaderCharacterID: ids[0], MaxMembers: 5, Status: party.StatusRecruiting, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		for i, id := range ids {
			if err := repo.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: id, IsLeader: i == 0, JoinedAt: now.Add(time.Duration(i) * time.Second)}); err != nil {
				t.Fatal(err)
			}
		}

		var leavers []func() error
		for _, id := range ids[:4] {
			leavers = append(leavers, func() error { return repo.RemoveMember(ctx, partyID, id) })
		}
		for _, err := range testutil.RunRace(leavers...) {
			if err != nil {
				t.Fatalf("round %d: concurrent leave failed: %v", round, err)
			}
		}

		got, err := repo.GetParty(ctx, partyID)
		if err != nil || got.LeaderCharacterID != ids[4] {
			t.Fatalf("round %d: leader = %q (err=%v), want the remaining member", round, got.LeaderCharacterID, err)
		}
		members, err := repo.GetMembers(ctx, partyID)
		if err != nil || len(members) != 1 || members[0].CharacterID != ids[4] || !members[0].IsLeader {
			t.Fatalf("round %d: roster not conserved: %+v (err=%v)", round, members, err)
		}
		for _, id := range ids[:4] {
			if _, _, err := repo.GetActivePartyByCharacter(ctx, id); !errors.Is(err, party.ErrNotFound) {
				t.Fatalf("round %d: %s still mapped: %v", round, id, err)
			}
		}
	}
}

func TestValkeyRepository_PasswordHash_LiveValkey(t *testing.T) {
	valkeyAddr := os.Getenv("PARTY2_VALKEY_ADDR")
	if valkeyAddr == "" {
		valkeyAddr = "127.0.0.1:6379"
	}

	client, err := valkey.NewClient(valkey.ClientOption{
		InitAddress: []string{valkeyAddr},
	})
	if err != nil {
		t.Skipf("skipping live Valkey test: %v", err)
	}
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Skipf("skipping live Valkey test (cannot ping Valkey at %s): %v", valkeyAddr, err)
	}

	testLobbyPrefix := "party2:test:pw:party:lobby:"
	testCharPrefix := "party2:test:pw:party:character:"
	testIndexKey := "party2:test:pw:party:lobbies"

	repo := party.NewValkeyRepository(client,
		party.WithLobbyKeyPrefix(testLobbyPrefix),
		party.WithCharacterKeyPrefix(testCharPrefix),
		party.WithLobbiesIndexKey(testIndexKey),
	)

	leaderID := "pw-live-leader"
	memberID := "pw-live-member"
	var createdPartyID string

	cleanup := func() {
		if createdPartyID != "" {
			_ = client.Do(ctx, client.B().Del().Key(testLobbyPrefix+createdPartyID).Build()).Error()
			_ = client.Do(ctx, client.B().Zrem().Key(testIndexKey).Member(createdPartyID).Build()).Error()
		}
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+leaderID).Build()).Error()
		_ = client.Do(ctx, client.B().Del().Key(testCharPrefix+memberID).Build()).Error()
	}
	cleanup()
	defer cleanup()

	charRepo := &liveTestCharRepo{
		chars: map[string]corecharacter.Character{
			leaderID: {
				ID:    leaderID,
				Name:  "リーダー",
				JobID: "job-01",
				Level: 10,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100},
			},
			memberID: {
				ID:    memberID,
				Name:  "メンバー",
				JobID: "job-02",
				Level: 10,
				Stats: corecharacter.Stats{HP: 100, MaxHP: 100},
			},
		},
	}

	svc, err := party.NewService(repo, charRepo, nil, liveTestStageProvider{}, liveTestMonsterProvider{}, nil)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Create password-protected party
	detail, err := svc.CreateParty(ctx, leaderID, party.CreatePartyRequest{
		Speed:      18,
		Name:       "合言葉パーティ",
		StageID:    "stage-1",
		Password:   "secret",
		MaxMembers: 4,
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	createdPartyID = detail.Party.ID
	if detail.Party.PasswordHash == "" {
		t.Fatal("expected created party to have non-empty PasswordHash")
	}
	wantHash := detail.Party.PasswordHash

	assertNoPasswordLeak := func(label string, v any) {
		t.Helper()
		raw, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("%s json.Marshal failed: %v", label, err)
		}
		s := string(raw)
		if strings.Contains(s, "password_hash") || strings.Contains(s, wantHash) || strings.Contains(s, `"secret"`) {
			t.Fatalf("%s exposed secret password data: %s", label, s)
		}
	}
	assertNoPasswordLeak("CreateParty", detail)

	// 2. ListParties reflects HasPassword = true without exposing hash
	page, err := svc.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil {
		t.Fatalf("ListParties failed: %v", err)
	}
	if len(page.Items) != 1 || !page.Items[0].HasPassword {
		t.Fatalf("expected HasPassword=true in ListParties, got %+v", page.Items)
	}
	assertNoPasswordLeak("ListParties", page)

	// 4. Wrong and empty passwords are rejected with ErrInvalidPassword
	if _, err := svc.JoinParty(ctx, createdPartyID, memberID, "wrong"); !errors.Is(err, party.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for wrong password, got %v", err)
	}
	if _, err := svc.JoinParty(ctx, createdPartyID, memberID, ""); !errors.Is(err, party.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword for empty password, got %v", err)
	}

	// 5. Correct password joins via add_member.lua and preserves PasswordHash
	joinedDetail, err := svc.JoinParty(ctx, createdPartyID, memberID, "secret")
	if err != nil {
		t.Fatalf("JoinParty with valid password failed: %v", err)
	}
	assertNoPasswordLeak("JoinParty", joinedDetail)
	gotParty, err := repo.GetParty(ctx, createdPartyID)
	if err != nil || gotParty.PasswordHash != wantHash {
		t.Fatalf("PasswordHash lost after JoinParty: got %q, want %q (err=%v)", gotParty.PasswordHash, wantHash, err)
	}

	// 6. LeaveParty via remove_member.lua preserves PasswordHash
	if _, err := svc.LeaveParty(ctx, createdPartyID, memberID); err != nil {
		t.Fatalf("LeaveParty failed: %v", err)
	}
	gotParty, err = repo.GetParty(ctx, createdPartyID)
	if err != nil || gotParty.PasswordHash != wantHash {
		t.Fatalf("PasswordHash lost after LeaveParty: got %q, want %q (err=%v)", gotParty.PasswordHash, wantHash, err)
	}

	// 7. UpdateParty via update_party.lua preserves PasswordHash and HasPassword
	gotParty.Status = party.StatusInProgress
	if err := repo.UpdateParty(ctx, gotParty); err != nil {
		t.Fatalf("UpdateParty(in_progress) failed: %v", err)
	}
	gotParty.Status = party.StatusRecruiting
	if err := repo.UpdateParty(ctx, gotParty); err != nil {
		t.Fatalf("UpdateParty(recruiting) failed: %v", err)
	}
	gotParty, err = repo.GetParty(ctx, createdPartyID)
	if err != nil || gotParty.PasswordHash != wantHash {
		t.Fatalf("PasswordHash lost after UpdateParty: got %q, want %q (err=%v)", gotParty.PasswordHash, wantHash, err)
	}
	page, err = svc.ListParties(ctx, party.StatusRecruiting, 10, 0)
	if err != nil || len(page.Items) != 1 || !page.Items[0].HasPassword {
		t.Fatalf("expected HasPassword=true after UpdateParty, got %+v (err=%v)", page.Items, err)
	}
	if _, err := svc.JoinParty(ctx, createdPartyID, memberID, "wrong"); !errors.Is(err, party.ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword after UpdateParty, got %v", err)
	}
	if _, err := svc.JoinParty(ctx, createdPartyID, memberID, "secret"); err != nil {
		t.Fatalf("expected JoinParty with valid password to succeed after UpdateParty, got %v", err)
	}
}
