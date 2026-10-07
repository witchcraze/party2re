package casino_test

import (
	"context"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// faultValkeyClient wraps a real valkeygo.Client and allows injecting errors
// into specific Valkey commands while delegating other operations directly to the real server.
type faultValkeyClient struct {
	valkeygo.Client
	failZadd atomic.Bool
}

func (c *faultValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if c.failZadd.Load() {
		cmds := cmd.Commands()
		if len(cmds) > 0 && cmds[0] == "ZADD" {
			return valkeygo.NewErrorResult(errors.New("injected active-index ZADD failure"))
		}
	}
	return c.Client.Do(ctx, cmd)
}

func (c *faultValkeyClient) DoMulti(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
	results := make([]valkeygo.ValkeyResult, len(multi))
	for i, cmd := range multi {
		results[i] = c.Do(ctx, cmd)
	}
	return results
}

func openIsolatedTestValkey(t *testing.T, dbIndex int) valkeygo.Client {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}

	cfg := vk.ConfigFromEnvironment()
	cfg.DB = dbIndex
	client, err := vk.NewClientWithConfig(cfg)
	if err != nil {
		t.Fatalf("open valkey client (DB %d): %v", dbIndex, err)
	}
	return client
}

func cleanupTestDB(t *testing.T, client valkeygo.Client) {
	t.Helper()
	ctx := context.Background()
	_ = client.Do(ctx, client.B().Flushdb().Build())
}

// TestValkeyRoomRenewal_RealValkeyIndexFailureAndPartialEffect verifies the reproduction
// and acceptance criteria described in Issue #1070:
// 1. Seed a room/member with an older active-index score (> 30m ago); pot=50.
// 2. UpdateRoom with UpdatedAt=now and pot=123; inject active-index ZADD failure.
// 3. UpdateRoom propagates the error; payload SET is preserved in Valkey.
// 4. Stale index is pruned by ListRooms, but room payload is preserved by locked expiry recheck.
// 5. Healthy contrast: working UpdateRoom restores lobby discovery.
func TestValkeyRoomRenewal_RealValkeyIndexFailureAndPartialEffect(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 14)
	defer rawClient.Close()

	cleanupTestDB(t, rawClient)
	defer cleanupTestDB(t, rawClient)

	ctx := context.Background()
	faultClient := &faultValkeyClient{Client: rawClient}

	repo, err := casino.NewValkeyRoomRepository(faultClient)
	if err != nil {
		t.Fatalf("failed to create valkey room repo: %v", err)
	}

	casinoRepo := newMockPrizeCasinoRepo()
	charLeader := "char-lead-renewal"
	casinoRepo.accounts[charLeader] = casino.Account{CharacterID: charLeader, Coins: 1000}

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	// Score older than 30-minute cutoff (35 minutes ago)
	oldTime := now.Add(-35 * time.Minute)
	roomID := "room-renewal-real-1"

	room := casino.Room{
		ID:                roomID,
		Name:              "RenewalRealTable",
		GameType:          casino.GameTypeIndian,
		LeaderCharacterID: charLeader,
		Speed:             casino.SpeedNormal,
		MaxPlayers:        4,
		Rate:              10,
		Status:            casino.RoomStatusInProgress,
		Round:             1,
		CurrentBet:        10,
		MaxBet:            50,
		Pot:               50,
		CreatedAt:         oldTime,
		UpdatedAt:         oldTime,
	}
	member := casino.RoomMember{
		RoomID:        roomID,
		CharacterID:   charLeader,
		CharacterName: "LeaderHero",
		Action:        "待機中",
		Card:          7,
		JoinedAt:      oldTime,
		UpdatedAt:     oldTime,
	}

	// 1. Seed room/member and older active-index score
	if err := repo.CreateRoom(ctx, room, member); err != nil {
		t.Fatalf("failed to seed room: %v", err)
	}

	zscoreCmd := rawClient.B().Zscore().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build()
	score, err := rawClient.Do(ctx, zscoreCmd).AsFloat64()
	if err != nil || score != float64(oldTime.Unix()) {
		t.Fatalf("expected seeded score %v, got %v (err=%v)", float64(oldTime.Unix()), score, err)
	}

	// 2. UpdateRoom with UpdatedAt=now and pot=123; fail only its active-index ZADD
	faultClient.failZadd.Store(true)

	room.Pot = 123
	room.UpdatedAt = now
	updateErr := repo.UpdateRoom(ctx, room)

	// Acceptance criteria: Active-index renewal failures propagate through saveRoomDetail and callers
	if updateErr == nil {
		t.Fatal("expected UpdateRoom to propagate ZADD failure, got nil")
	}

	// Acceptance criteria: Persisted payload partial effect is preserved in Valkey
	renewedRoom, err := repo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("failed to get renewed room from valkey: %v", err)
	}
	if renewedRoom.Pot != 123 {
		t.Errorf("expected renewed pot 123 in payload, got %d", renewedRoom.Pot)
	}
	if !renewedRoom.UpdatedAt.Equal(now) {
		t.Errorf("expected renewed UpdatedAt %v, got %v", now, renewedRoom.UpdatedAt)
	}

	// Active-index score remained at old score because ZADD was rejected:
	checkScoreCmd := rawClient.B().Zscore().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build()
	staleScore, err := rawClient.Do(ctx, checkScoreCmd).AsFloat64()
	if err != nil || staleScore != float64(oldTime.Unix()) {
		t.Fatalf("expected stale index score %v, got %v (err=%v)", float64(oldTime.Unix()), staleScore, err)
	}

	// 3. Restore writes and call real CasinoService.ListRooms:
	// PurgeIdleRooms preserves the renewed room because room.UpdatedAt > cutoff.
	// ListActiveRooms prunes the stale index because its score <= cutoff.
	faultClient.failZadd.Store(false)

	lobbyRooms, err := svc.ListRooms(ctx)
	if err != nil {
		t.Fatalf("ListRooms failed: %v", err)
	}
	if len(lobbyRooms) != 0 {
		t.Fatalf("expected room to be pruned from lobby due to stale index, got %d rooms: %+v", len(lobbyRooms), lobbyRooms)
	}

	// But GetRoom still returns the preserved renewed payload:
	persistedRoom, err := repo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("expected GetRoom to find preserved room, got error: %v", err)
	}
	if persistedRoom.Pot != 123 {
		t.Errorf("expected preserved room to have pot 123, got %d", persistedRoom.Pot)
	}

	// 4. Healthy contrast: the same UpdateRoom with ZADD available restores room discovery
	if err := repo.UpdateRoom(ctx, room); err != nil {
		t.Fatalf("expected healthy UpdateRoom to succeed, got: %v", err)
	}

	restoredLobby, err := svc.ListRooms(ctx)
	if err != nil {
		t.Fatalf("ListRooms failed after healthy update: %v", err)
	}
	if len(restoredLobby) != 1 || restoredLobby[0].ID != roomID {
		t.Fatalf("expected room %s to be discovered in lobby, got: %+v", roomID, restoredLobby)
	}
}

// TestValkeyRoomRenewal_CommandCallerPreservesPartialEffectOnWriteFailure verifies that
// a normal gameplay command caller using the shared writer propagates the write error,
// and that its partial outcome in the payload is preserved rather than authorizing blind command retry.
func TestValkeyRoomRenewal_CommandCallerPreservesPartialEffectOnWriteFailure(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 15)
	defer rawClient.Close()

	cleanupTestDB(t, rawClient)
	defer cleanupTestDB(t, rawClient)

	ctx := context.Background()
	faultClient := &faultValkeyClient{Client: rawClient}

	repo, err := casino.NewValkeyRoomRepository(faultClient)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	casinoRepo := newMockPrizeCasinoRepo()
	charLeader := "char-lead-cmd"
	charGuest := "char-guest-cmd"
	casinoRepo.accounts[charLeader] = casino.Account{CharacterID: charLeader, Coins: 1000}
	casinoRepo.accounts[charGuest] = casino.Account{CharacterID: charGuest, Coins: 1000}

	chars := &inMemoryCharRepo{
		chars: map[string]corecharacter.Character{
			charLeader: {ID: charLeader, Name: "LeaderHero", PlayerID: "player-leader"},
			charGuest:  {ID: charGuest, Name: "GuestHero", PlayerID: "player-guest"},
			"outsider": {ID: "outsider", Name: "OutsiderHero", PlayerID: "player-outsider"},
		},
	}

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	created, err := svc.CreateRoom(ctx, charLeader, casino.CreateRoomRequest{
		Name:            "CommandWriterTable",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedNormal,
		MaxPlayers:      2,
		Rate:            10,
		AllowSpectators: false,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := created.Room.ID

	if _, err := svc.JoinRoom(ctx, roomID, charGuest, "", 0); err != nil {
		t.Fatalf("JoinRoom failed: %v", err)
	}

	if _, err := svc.StartIndianPoker(ctx, roomID, charLeader); err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}

	// Verify initial game state: Round 1, Pot 0
	initSnap, err := casino.RoomSnapshotForTest(svc, ctx, roomID, charLeader)
	if err != nil {
		t.Fatalf("snapshot failed: %v", err)
	}
	if initSnap.Room.Round != 1 || initSnap.Room.Pot != 0 {
		t.Fatalf("unexpected initial room state: round=%d pot=%d", initSnap.Room.Round, initSnap.Room.Pot)
	}

	// Inject ZADD failure when leader plays action
	faultClient.failZadd.Store(true)

	// PlayIndianPokerAction updates member action and pot, then calls UpdateRoom (which calls saveRoomDetail)
	_, actionErr := svc.PlayIndianPokerAction(ctx, roomID, charLeader, casino.ActionCall)
	if actionErr == nil {
		t.Fatal("expected PlayIndianPokerAction to return error when ZADD fails, got nil")
	}

	// Verify partial outcome is preserved in Valkey room payload:
	// The payload was written before ZADD failed.
	faultClient.failZadd.Store(false)
	persistedSnap, err := casino.RoomSnapshotForTest(svc, ctx, roomID, charLeader)
	if err != nil {
		t.Fatalf("failed to read persisted snapshot: %v", err)
	}

	// Leader's action was recorded as "つづける" / call in the payload
	var leaderMember *casino.RoomMember
	for _, m := range persistedSnap.Members {
		if m.CharacterID == charLeader {
			cpy := m
			leaderMember = &cpy
			break
		}
	}
	if leaderMember == nil {
		t.Fatal("leader member not found in snapshot")
	}
	if leaderMember.Action != string(casino.ActionCall) {
		t.Errorf("expected leader action '%s', got '%s'", casino.ActionCall, leaderMember.Action)
	}

	// Because the partial state was applied, blind command replay would fail with ErrAlreadyActed:
	_, replayErr := svc.PlayIndianPokerAction(ctx, roomID, charLeader, casino.ActionCall)
	if !errors.Is(replayErr, casino.ErrAlreadyActed) {
		t.Errorf("expected ErrAlreadyActed on blind replay, got %v", replayErr)
	}

	// Existing game visibility masking and admission remain intact:
	// 1. Participant forehead card masking: leader cannot see own card
	view, err := svc.GetRoomView(ctx, roomID, "player-leader", charLeader)
	if err != nil {
		t.Fatalf("GetRoomView failed: %v", err)
	}
	for _, m := range view.Members {
		if m.CharacterID == charLeader && m.Card != -1 {
			t.Errorf("expected forehead card masking (card=-1) for own card, got %d", m.Card)
		}
		if m.CharacterID == charGuest && m.Card == -1 {
			t.Errorf("expected opponent card visible to leader, got %d", m.Card)
		}
	}

	// 2. Outsider denied view
	_, err = svc.GetRoomView(ctx, roomID, "player-outsider", "outsider")
	if !errors.Is(err, casino.ErrRoomViewForbidden) {
		t.Errorf("expected ErrRoomViewForbidden for outsider, got %v", err)
	}

	// 3. Spoofed player denied view
	_, err = svc.GetRoomView(ctx, roomID, "wrong-player", charLeader)
	if !errors.Is(err, casino.ErrRoomViewForbidden) {
		t.Errorf("expected ErrRoomViewForbidden for spoofed player, got %v", err)
	}
}
