package casino_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// faultValkeyClient wraps a real valkeygo.Client and allows injecting errors
// into specific Valkey commands while delegating other operations directly to the real server.
type faultValkeyClient struct {
	valkeygo.Client
	failZadd    atomic.Bool
	failCharDel atomic.Bool
	failCharSet atomic.Bool
}

func (c *faultValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	cmds := cmd.Commands()
	if c.failZadd.Load() && len(cmds) > 0 && cmds[0] == "ZADD" {
		return valkeygo.NewErrorResult(errors.New("injected active-index ZADD failure"))
	}
	if c.failCharDel.Load() && len(cmds) == 5 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && strings.HasPrefix(cmds[3], casino.DefaultCharacterKeyPrefix) {
		return valkeygo.NewErrorResult(errors.New("injected character mapping DEL failure"))
	}
	if c.failCharSet.Load() && len(cmds) >= 6 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && strings.HasPrefix(cmds[3], casino.DefaultCharacterKeyPrefix) {
		return valkeygo.NewErrorResult(errors.New("injected character mapping SET failure"))
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

type hookValkeyClient struct {
	valkeygo.Client
	beforeDo func(cmds []string) (override error)
}

func (c *hookValkeyClient) Do(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
	if c.beforeDo != nil {
		if err := c.beforeDo(cmd.Commands()); err != nil {
			return valkeygo.NewErrorResult(err)
		}
	}
	return c.Client.Do(ctx, cmd)
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

func registerCasinoValkeyCleanup(t *testing.T, client valkeygo.Client, roomIDs []string, charIDs []string) {
	t.Helper()
	t.Cleanup(func() {
		ctx := context.Background()
		for _, rID := range roomIDs {
			_ = client.Do(ctx, client.B().Del().Key(casino.DefaultRoomKeyPrefix+rID).Build())
			_ = client.Do(ctx, client.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(rID).Build())
		}
		for _, cID := range charIDs {
			_ = client.Do(ctx, client.B().Del().Key(casino.DefaultCharacterKeyPrefix+cID).Build())
		}
	})
}

func TestValkeyRoomRenewal_PreservesAdmissionDiscoveryAcrossOriginalDeadline(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 0)
	defer rawClient.Close()

	ctx := context.Background()
	prefix := "r1091-" + id.New()[:8]
	leader := prefix + "-leader"
	peer := prefix + "-peer"
	spec := prefix + "-spec"
	extra := prefix + "-extra"
	unrenewedLeader := prefix + "-unrenewed"

	chars := &inMemoryCharRepo{
		chars: map[string]corecharacter.Character{
			leader:          {ID: leader, Name: "Leader", PlayerID: "owner-leader"},
			peer:            {ID: peer, Name: "Peer", PlayerID: "owner-peer"},
			spec:            {ID: spec, Name: "Spectator", PlayerID: "owner-spec"},
			extra:           {ID: extra, Name: "Extra", PlayerID: "owner-extra"},
			unrenewedLeader: {ID: unrenewedLeader, Name: "IdleLeader", PlayerID: "owner-idle"},
		},
	}
	casinoRepo := newMockPrizeCasinoRepo()
	for _, actor := range []string{leader, peer, spec, extra, unrenewedLeader} {
		casinoRepo.accounts[actor] = casino.Account{CharacterID: actor, Coins: 1000}
	}

	repo, err := casino.NewValkeyRoomRepository(rawClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	created, err := svc.CreateRoom(ctx, leader, casino.CreateRoomRequest{
		Name:            "Renew-" + prefix,
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedFast,
		MaxPlayers:      4,
		Rate:            10,
		AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := created.Room.ID

	unrenewedCreated, err := svc.CreateRoom(ctx, unrenewedLeader, casino.CreateRoomRequest{
		Name:            "Idle-" + prefix,
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedFast,
		MaxPlayers:      2,
		Rate:            10,
		AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	unrenewedRoomID := unrenewedCreated.Room.ID
	registerCasinoValkeyCleanup(t, rawClient, []string{roomID, unrenewedRoomID}, []string{leader, peer, spec, extra, unrenewedLeader})

	if _, err := svc.JoinRoom(ctx, roomID, peer, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SpectateRoom(ctx, roomID, spec, ""); err != nil {
		t.Fatal(err)
	}

	shortenMappings := func(actors ...string) {
		t.Helper()
		for _, actor := range actors {
			if err := rawClient.Do(ctx, rawClient.B().Expire().Key(casino.DefaultCharacterKeyPrefix+actor).Seconds(60).Build()).Error(); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertRenewedBeyondOriginalDeadline := func(step string, actors ...string) {
		t.Helper()
		roomTTL, err := rawClient.Do(ctx, rawClient.B().Ttl().Key(casino.DefaultRoomKeyPrefix+roomID).Build()).AsInt64()
		if err != nil || roomTTL <= 60 {
			t.Fatalf("%s: room TTL=%d err=%v", step, roomTTL, err)
		}
		for _, actor := range actors {
			charKey := casino.DefaultCharacterKeyPrefix + actor
			charTTL, err := rawClient.Do(ctx, rawClient.B().Ttl().Key(charKey).Build()).AsInt64()
			if err != nil || charTTL <= 60 {
				t.Fatalf("%s: actor %s mapping TTL=%d err=%v (roomTTL=%d)", step, actor, charTTL, err, roomTTL)
			}
			// Advance past the original 60s admission deadline without waiting wall-clock minutes.
			if err := rawClient.Do(ctx, rawClient.B().Expire().Key(charKey).Seconds(charTTL-61).Build()).Error(); err != nil {
				t.Fatal(err)
			}
			owner := chars.chars[actor].PlayerID
			view, err := svc.GetCharacterRoomView(ctx, owner, actor)
			if err != nil || view == nil || view.Room.ID != roomID {
				t.Fatalf("%s: GetCharacterRoomView(%s) across original deadline = %+v, err=%v", step, actor, view, err)
			}
		}
	}

	// GET/refresh must not renew room or mapping lifetime.
	shortenMappings(leader, peer, spec)
	if err := rawClient.Do(ctx, rawClient.B().Expire().Key(casino.DefaultRoomKeyPrefix+roomID).Seconds(60).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	for _, actor := range []string{leader, peer, spec} {
		owner := chars.chars[actor].PlayerID
		if view, err := svc.GetCharacterRoomView(ctx, owner, actor); err != nil || view == nil {
			t.Fatalf("read view(%s): %+v %v", actor, view, err)
		}
	}
	if roomTTL, err := rawClient.Do(ctx, rawClient.B().Ttl().Key(casino.DefaultRoomKeyPrefix+roomID).Build()).AsInt64(); err != nil || roomTTL > 60 {
		t.Fatalf("GET renewed room TTL=%d err=%v", roomTTL, err)
	}
	for _, actor := range []string{leader, peer, spec} {
		if charTTL, err := rawClient.Do(ctx, rawClient.B().Ttl().Key(casino.DefaultCharacterKeyPrefix+actor).Build()).AsInt64(); err != nil || charTTL > 60 {
			t.Fatalf("GET renewed actor %s mapping TTL=%d err=%v", actor, charTTL, err)
		}
	}

	// 1. UpdateRoom renews all admitted participants and spectators.
	shortenMappings(leader, peer, spec)
	r, err := repo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	r.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateRoom(ctx, *r); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("UpdateRoom", leader, peer, spec)

	// 2. UpdateMember driven by peer renews leader, peer and spectator.
	shortenMappings(leader, peer, spec)
	peerMem, err := repo.GetMember(ctx, roomID, peer)
	if err != nil {
		t.Fatal(err)
	}
	peerMem.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateMember(ctx, *peerMem); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("UpdateMember", leader, peer, spec)

	// 3. AddMember (another participant joining) renews existing members and sets new member.
	shortenMappings(leader, peer, spec)
	if _, err := svc.JoinRoom(ctx, roomID, extra, "", 0); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("AddMember", leader, peer, spec, extra)

	// 4. RemoveMember (extra leaving) renews remaining members and removes extra's mapping.
	shortenMappings(leader, peer, spec)
	if err := svc.LeaveRoom(ctx, roomID, extra); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("RemoveMember", leader, peer, spec)
	if view, err := svc.GetCharacterRoomView(ctx, "owner-extra", extra); err != nil || view != nil {
		t.Fatalf("left member still discoverable: %+v %v", view, err)
	}

	// 5. UpdateRoomDetail via game start and peer turn action renews leader, peer and spectator.
	shortenMappings(leader, peer, spec)
	if _, err := svc.StartIndianPoker(ctx, roomID, leader); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("StartIndianPoker", leader, peer, spec)

	shortenMappings(leader, peer, spec)
	if _, err := svc.PlayIndianPokerAction(ctx, roomID, peer, casino.ActionCall); err != nil {
		t.Fatal(err)
	}
	assertRenewedBeyondOriginalDeadline("PlayIndianPokerAction(peer)", leader, peer, spec)

	// Contrast: unrenewed room/mapping expires at original admission deadline.
	if err := rawClient.Do(ctx, rawClient.B().Expire().Key(casino.DefaultCharacterKeyPrefix+unrenewedLeader).Seconds(0).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-idle", unrenewedLeader); err != nil || view != nil {
		t.Fatalf("expired mapping should return nil, nil, got view=%+v err=%v", view, err)
	}
}

func TestValkeyRoomRenewal_LifecycleCleanupAndCrossRoomIsolation(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 0)
	defer rawClient.Close()

	ctx := context.Background()
	prefix := "iso1091-" + id.New()[:8]
	leaderA := prefix + "-leaderA"
	peerA := prefix + "-peerA"
	specA := prefix + "-specA"
	leaderB := prefix + "-leaderB"
	allActors := []string{leaderA, peerA, specA, leaderB}

	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{}}
	casinoRepo := newMockPrizeCasinoRepo()
	for _, actor := range allActors {
		chars.chars[actor] = corecharacter.Character{ID: actor, Name: actor, PlayerID: "owner-" + actor}
		casinoRepo.accounts[actor] = casino.Account{CharacterID: actor, Coins: 500}
	}

	faultClient := &faultValkeyClient{Client: rawClient}
	repo, err := casino.NewValkeyRoomRepository(faultClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	roomADetail, err := svc.CreateRoom(ctx, leaderA, casino.CreateRoomRequest{
		Name: "RoomA-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomA := roomADetail.Room.ID

	roomBDetail, err := svc.CreateRoom(ctx, leaderB, casino.CreateRoomRequest{
		Name: "RoomB-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomB := roomBDetail.Room.ID
	registerCasinoValkeyCleanup(t, rawClient, []string{roomA, roomB}, allActors)

	if _, err := svc.JoinRoom(ctx, roomA, peerA, "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SpectateRoom(ctx, roomA, specA, ""); err != nil {
		t.Fatal(err)
	}

	// 1. KickMember removes spectator mapping; subsequent roomA renewal does not resurrect it.
	if err := svc.KickMember(ctx, roomA, leaderA, specA); err != nil {
		t.Fatal(err)
	}
	rA, err := repo.GetRoom(ctx, roomA)
	if err != nil {
		t.Fatal(err)
	}
	rA.UpdatedAt = time.Now().UTC()
	if err := repo.UpdateRoom(ctx, *rA); err != nil {
		t.Fatal(err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+specA, specA); err != nil || view != nil {
		t.Fatalf("kicked spectator resurrected: %+v %v", view, err)
	}

	// 2. If secondary DEL fails on LeaveRoom, error is ErrSecondarySync, GetCharacterRoomView
	// still reports nil, nil (not forbidden) because live roster no longer admits peerA,
	// and joining roomB overwrites the stale roomA mapping.
	faultClient.failCharDel.Store(true)
	leaveErr := svc.LeaveRoom(ctx, roomA, peerA)
	faultClient.failCharDel.Store(false)
	if !errors.Is(leaveErr, casino.ErrSecondarySync) {
		t.Fatalf("expected ErrSecondarySync on failed DEL, got %v", leaveErr)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomA {
		t.Fatalf("expected stale mapping to roomA before rejoin, got %q err=%v", mapped, err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+peerA, peerA); err != nil || view != nil {
		t.Fatalf("stale mapping after leave should resolve to nil, nil, got view=%+v err=%v", view, err)
	}
	if _, err := svc.JoinRoom(ctx, roomB, peerA, "", 0); err != nil {
		t.Fatalf("rejoin roomB after stale mapping failed: %v", err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("expected roomB mapping, got %q err=%v", mapped, err)
	}

	// 3. Admitted member in live roomB cannot create, join, spectate, or be added to another room,
	// and roomA's roster/mapping remain untouched.
	if _, err := svc.CreateRoom(ctx, peerA, casino.CreateRoomRequest{
		Name: "RoomC-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
	}); !errors.Is(err, casino.ErrAlreadyInRoom) {
		t.Fatalf("expected ErrAlreadyInRoom on CreateRoom while admitted in roomB, got %v", err)
	}
	if _, err := svc.JoinRoom(ctx, roomA, peerA, "", 0); !errors.Is(err, casino.ErrAlreadyInRoom) {
		t.Fatalf("expected ErrAlreadyInRoom on JoinRoom(roomA) while admitted in roomB, got %v", err)
	}
	if _, err := svc.SpectateRoom(ctx, roomA, peerA, ""); !errors.Is(err, casino.ErrAlreadyInRoom) {
		t.Fatalf("expected ErrAlreadyInRoom on SpectateRoom(roomA) while admitted in roomB, got %v", err)
	}
	if err := repo.AddMember(ctx, casino.RoomMember{RoomID: roomA, CharacterID: peerA, JoinedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); !errors.Is(err, casino.ErrAlreadyInRoom) {
		t.Fatalf("expected ErrAlreadyInRoom on repo.AddMember(roomA) while admitted in roomB, got %v", err)
	}
	if _, err := repo.GetMember(ctx, roomA, peerA); !errors.Is(err, casino.ErrMemberNotFound) {
		t.Fatalf("expected peerA not in roomA roster after rejected admission, got err=%v", err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("rejected admission on roomA altered live roomB mapping: %q err=%v", mapped, err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+peerA, peerA); err != nil || view == nil || view.Room.ID != roomB {
		t.Fatalf("expected GetCharacterRoomView to return roomB after rejected roomA admission, got %+v err=%v", view, err)
	}

	// Even if roomA has a stale roster snapshot containing peerA, UpdateRoom/RemoveMember/DeleteRoom on roomA
	// must never overwrite or delete peerA's live mapping to roomB.
	if err := repo.UpdateRoomDetail(ctx, casino.RoomDetail{
		Room: *rA,
		Members: []casino.RoomMember{
			{RoomID: roomA, CharacterID: leaderA, JoinedAt: rA.CreatedAt, UpdatedAt: rA.UpdatedAt},
			{RoomID: roomA, CharacterID: peerA, JoinedAt: rA.CreatedAt, UpdatedAt: rA.UpdatedAt},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("UpdateRoomDetail on roomA overwrote live roomB mapping: %q err=%v", mapped, err)
	}
	if err := repo.UpdateRoom(ctx, *rA); err != nil {
		t.Fatal(err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("UpdateRoom on roomA overwrote live roomB mapping: %q err=%v", mapped, err)
	}
	if err := repo.RemoveMember(ctx, roomA, peerA); err != nil {
		t.Fatal(err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("RemoveMember on roomA deleted live roomB mapping: %q err=%v", mapped, err)
	}
	if err := repo.DeleteRoom(ctx, roomA); err != nil {
		t.Fatal(err)
	}
	if mapped, err := repo.GetCharacterRoom(ctx, peerA); err != nil || mapped != roomB {
		t.Fatalf("DeleteRoom on roomA deleted live roomB mapping: %q err=%v", mapped, err)
	}

	// 4. Secondary SET failure on mapping renewal propagates ErrSecondarySync.
	faultClient.failCharSet.Store(true)
	rB, err := repo.GetRoom(ctx, roomB)
	if err != nil {
		t.Fatal(err)
	}
	rB.UpdatedAt = time.Now().UTC()
	setSyncErr := repo.UpdateRoom(ctx, *rB)
	faultClient.failCharSet.Store(false)
	if !errors.Is(setSyncErr, casino.ErrSecondarySync) {
		t.Fatalf("expected ErrSecondarySync on failed mapping renewal SET, got %v", setSyncErr)
	}

	// 5. Unreadable mapping (blank value or WRONGTYPE) is not absence.
	if err := rawClient.Do(ctx, rawClient.B().Set().Key(casino.DefaultCharacterKeyPrefix+specA).Value("   ").Ex(casino.DefaultLobbyTTL).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.GetCharacterRoom(ctx, specA); !errors.Is(err, casino.ErrInvalidRoomMapping) {
		t.Fatalf("expected ErrInvalidRoomMapping on blank key, got %v", err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+specA, specA); view != nil || !errors.Is(err, casino.ErrInvalidRoomMapping) {
		t.Fatalf("expected ErrInvalidRoomMapping from GetCharacterRoomView, got view=%+v err=%v", view, err)
	}
	_ = rawClient.Do(ctx, rawClient.B().Del().Key(casino.DefaultCharacterKeyPrefix+specA).Build())
	if err := rawClient.Do(ctx, rawClient.B().Hset().Key(casino.DefaultCharacterKeyPrefix+specA).FieldValue().FieldValue("bad", "type").Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+specA, specA); view != nil || err == nil || errors.Is(err, casino.ErrRoomNotFound) {
		t.Fatalf("expected non-absence error on WRONGTYPE mapping, got view=%+v err=%v", view, err)
	}
}

func TestValkeyRoomRenewal_ConcurrentMembershipAndRenewalSafety(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 0)
	defer rawClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cfg := testutil.GetStressConfig()
	prefix := "conc1091-" + id.New()[:8]
	roomA := prefix + "-roomA"
	roomB := prefix + "-roomB"
	leaderA := prefix + "-leaderA"
	leaderB := prefix + "-leaderB"

	movers := make([]string, cfg.Workers)
	allChars := []string{leaderA, leaderB}
	chars := &inMemoryCharRepo{
		chars: map[string]corecharacter.Character{
			leaderA: {ID: leaderA, Name: leaderA, PlayerID: "owner-" + leaderA},
			leaderB: {ID: leaderB, Name: leaderB, PlayerID: "owner-" + leaderB},
		},
	}
	for i := range movers {
		cID := fmt.Sprintf("%s-mover-%d", prefix, i)
		movers[i] = cID
		allChars = append(allChars, cID)
		chars.chars[cID] = corecharacter.Character{ID: cID, Name: cID, PlayerID: "owner-" + cID}
	}
	registerCasinoValkeyCleanup(t, rawClient, []string{roomA, roomB}, allChars)

	repo, err := casino.NewValkeyRoomRepository(rawClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	if err := repo.CreateRoom(ctx, casino.Room{
		ID: roomA, Name: "ConcA-" + prefix, GameType: casino.GameTypeIndian, LeaderCharacterID: leaderA,
		Speed: casino.SpeedFast, MaxPlayers: 64, Rate: 10, AllowSpectators: true, Status: casino.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now,
	}, casino.RoomMember{RoomID: roomA, CharacterID: leaderA, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := repo.CreateRoom(ctx, casino.Room{
		ID: roomB, Name: "ConcB-" + prefix, GameType: casino.GameTypeIndian, LeaderCharacterID: leaderB,
		Speed: casino.SpeedFast, MaxPlayers: 64, Rate: 10, AllowSpectators: true, Status: casino.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now,
	}, casino.RoomMember{RoomID: roomB, CharacterID: leaderB, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	for _, m := range movers {
		if err := repo.AddMember(ctx, casino.RoomMember{RoomID: roomA, CharacterID: m, IsSpectator: true, JoinedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
	}

	testutil.RunConcurrentStressTest(t, cfg, func(workerID, iter int) error {
		mover := movers[workerID]
		if iter == 0 {
			if err := repo.RemoveMember(ctx, roomA, mover); err != nil {
				return err
			}
			if err := repo.AddMember(ctx, casino.RoomMember{
				RoomID: roomB, CharacterID: mover, IsSpectator: true, JoinedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		if workerID%2 == 0 {
			if err := repo.UpdateMember(ctx, casino.RoomMember{
				RoomID: roomA, CharacterID: leaderA, Action: "待機中", UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		} else {
			if err := repo.UpdateMember(ctx, casino.RoomMember{
				RoomID: roomB, CharacterID: mover, IsSpectator: true, UpdatedAt: time.Now().UTC(),
			}); err != nil {
				return err
			}
		}
		view, err := svc.GetCharacterRoomView(ctx, "owner-"+mover, mover)
		if err != nil {
			return err
		}
		if view == nil || view.Room.ID != roomB {
			return fmt.Errorf("mover %s view=%+v, want roomB %s", mover, view, roomB)
		}
		return nil
	})
}

func TestValkeyRoomRenewal_CrossRoomConcurrentSameCharacterInvariants(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 0)
	defer rawClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	prefix := "samechar1091-" + id.New()[:8]
	leaderA := prefix + "-leaderA"
	leaderB := prefix + "-leaderB"
	mover := prefix + "-mover"
	allActors := []string{leaderA, leaderB, mover}

	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{}}
	casinoRepo := newMockPrizeCasinoRepo()
	for _, actor := range allActors {
		chars.chars[actor] = corecharacter.Character{ID: actor, Name: actor, PlayerID: "owner-" + actor}
		casinoRepo.accounts[actor] = casino.Account{CharacterID: actor, Coins: 5000}
	}

	repo, err := casino.NewValkeyRoomRepository(rawClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	roomADetail, err := svc.CreateRoom(ctx, leaderA, casino.CreateRoomRequest{
		Name: "SameA-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomA := roomADetail.Room.ID

	roomBDetail, err := svc.CreateRoom(ctx, leaderB, casino.CreateRoomRequest{
		Name: "SameB-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomB := roomBDetail.Room.ID
	registerCasinoValkeyCleanup(t, rawClient, []string{roomA, roomB}, allActors)

	assertRosterAndMappingInvariant := func() error {
		_, errA := repo.GetMember(ctx, roomA, mover)
		inA := errA == nil
		if errA != nil && !errors.Is(errA, casino.ErrMemberNotFound) {
			return errA
		}
		_, errB := repo.GetMember(ctx, roomB, mover)
		inB := errB == nil
		if errB != nil && !errors.Is(errB, casino.ErrMemberNotFound) {
			return errB
		}
		if inA && inB {
			return fmt.Errorf("invariant violated: %s admitted in both roomA and roomB rosters", mover)
		}
		mapped, mapErr := repo.GetCharacterRoom(ctx, mover)
		view, viewErr := svc.GetCharacterRoomView(ctx, "owner-"+mover, mover)
		if viewErr != nil {
			return viewErr
		}
		switch {
		case inA:
			if mapErr != nil || mapped != roomA {
				return fmt.Errorf("in roomA roster but mapping=%q err=%v", mapped, mapErr)
			}
			if view == nil || view.Room.ID != roomA {
				return fmt.Errorf("in roomA roster but view=%+v", view)
			}
		case inB:
			if mapErr != nil || mapped != roomB {
				return fmt.Errorf("in roomB roster but mapping=%q err=%v", mapped, mapErr)
			}
			if view == nil || view.Room.ID != roomB {
				return fmt.Errorf("in roomB roster but view=%+v", view)
			}
		default:
			if !errors.Is(mapErr, casino.ErrRoomNotFound) {
				return fmt.Errorf("in neither roster but mapping=%q err=%v", mapped, mapErr)
			}
			if view != nil {
				return fmt.Errorf("in neither roster but view=%+v", view)
			}
		}
		return nil
	}

	// Deterministic Interleaving A: Both JoinRoom(roomA) and JoinRoom(roomB) observe GET char == nil
	// before either writes its room payload; atomic claim_character_room.lua serializes the winner
	// and rolls back the loser's roster.
	var barrierPreClaim atomic.Bool
	preClaimArrived := make(chan struct{}, 2)
	preClaimRelease := make(chan struct{})
	var pauseDeleteOnRoomA atomic.Bool
	deletePaused := make(chan struct{})
	deleteRelease := make(chan struct{})

	hookClient := &hookValkeyClient{
		Client: rawClient,
		beforeDo: func(cmds []string) error {
			if barrierPreClaim.Load() && len(cmds) == 7 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && cmds[3] == casino.DefaultCharacterKeyPrefix+mover {
				preClaimArrived <- struct{}{}
				<-preClaimRelease
			}
			if pauseDeleteOnRoomA.Load() && len(cmds) == 5 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && cmds[3] == casino.DefaultCharacterKeyPrefix+mover && cmds[4] == roomA {
				if pauseDeleteOnRoomA.CompareAndSwap(true, false) {
					close(deletePaused)
					<-deleteRelease
				}
			}
			return nil
		},
	}
	hookRepo, err := casino.NewValkeyRoomRepository(hookClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	hookSvc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(hookRepo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	barrierPreClaim.Store(true)
	go func() {
		<-preClaimArrived
		<-preClaimArrived
		barrierPreClaim.Store(false)
		close(preClaimRelease)
	}()
	raceErrs := testutil.RunRace(
		func() error {
			_, err := hookSvc.JoinRoom(ctx, roomA, mover, "", 0)
			return err
		},
		func() error {
			_, err := hookSvc.JoinRoom(ctx, roomB, mover, "", 0)
			return err
		},
	)
	wins := 0
	for _, rErr := range raceErrs {
		if rErr == nil {
			wins++
		} else if !errors.Is(rErr, casino.ErrAlreadyInRoom) {
			t.Fatalf("unexpected error in deterministic pre-claim CAS race: %v", rErr)
		}
	}
	if wins != 1 {
		t.Fatalf("expected 1 winner in deterministic pre-claim CAS race, got %d (errs=%v)", wins, raceErrs)
	}
	if err := assertRosterAndMappingInvariant(); err != nil {
		t.Fatalf("after deterministic pre-claim CAS race: %v", err)
	}
	// Reset mover to roomA for Interleaving B.
	if mapped, _ := repo.GetCharacterRoom(ctx, mover); mapped == roomB {
		if err := svc.LeaveRoom(ctx, roomB, mover); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.JoinRoom(ctx, roomA, mover, "", 0); err != nil {
			t.Fatal(err)
		}
	}

	// Deterministic Interleaving B: LeaveRoom(roomA, mover) updates roomA payload, then pauses before
	// delete_character_room.lua while mover joins roomB; roomA's delayed delete must not remove roomB's mapping.
	pauseDeleteOnRoomA.Store(true)
	raceErrs = testutil.RunRace(
		func() error {
			return hookSvc.LeaveRoom(ctx, roomA, mover)
		},
		func() error {
			<-deletePaused
			_, err := hookSvc.JoinRoom(ctx, roomB, mover, "", 0)
			close(deleteRelease)
			return err
		},
	)
	for idx, rErr := range raceErrs {
		if rErr != nil {
			t.Fatalf("deterministic leave-vs-join race goroutine %d error: %v", idx, rErr)
		}
	}
	if mapped, err := repo.GetCharacterRoom(ctx, mover); err != nil || mapped != roomB {
		t.Fatalf("delayed roomA delete corrupted roomB mapping: got %q err=%v", mapped, err)
	}
	if err := assertRosterAndMappingInvariant(); err != nil {
		t.Fatalf("after deterministic leave-vs-join race: %v", err)
	}
	if err := svc.LeaveRoom(ctx, roomB, mover); err != nil {
		t.Fatal(err)
	}

	for i := range 10 {
		// 1. Concurrent join race on the same character into Room A vs Room B while both rooms renew.
		errs := testutil.RunRace(
			func() error {
				_, err := svc.JoinRoom(ctx, roomA, mover, "", 0)
				return err
			},
			func() error {
				_, err := svc.JoinRoom(ctx, roomB, mover, "", 0)
				return err
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomA, CharacterID: leaderA, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomB, CharacterID: leaderB, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
		)
		joinSuccesses := 0
		for idx, rErr := range errs[:2] {
			if rErr == nil {
				joinSuccesses++
			} else if !errors.Is(rErr, casino.ErrAlreadyInRoom) {
				t.Fatalf("iter %d goroutine %d unexpected join error: %v", i, idx, rErr)
			}
		}
		for idx, rErr := range errs[2:] {
			if rErr != nil {
				t.Fatalf("iter %d renewal %d error: %v", i, idx, rErr)
			}
		}
		if joinSuccesses != 1 {
			t.Fatalf("iter %d expected exactly 1 join success across roomA/roomB, got %d (errs=%v)", i, joinSuccesses, errs[:2])
		}
		if err := assertRosterAndMappingInvariant(); err != nil {
			t.Fatalf("iter %d after concurrent join race: %v", i, err)
		}

		currentRoom, err := repo.GetCharacterRoom(ctx, mover)
		if err != nil {
			t.Fatalf("iter %d GetCharacterRoom: %v", i, err)
		}
		otherRoom := roomB
		if currentRoom == roomB {
			otherRoom = roomA
		}

		// 2. Leave currentRoom and immediately join otherRoom while concurrent renewals run on both rooms.
		errs = testutil.RunRace(
			func() error {
				if err := svc.LeaveRoom(ctx, currentRoom, mover); err != nil {
					return err
				}
				_, err := svc.JoinRoom(ctx, otherRoom, mover, "", 0)
				return err
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomA, CharacterID: leaderA, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomB, CharacterID: leaderB, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
		)
		for idx, rErr := range errs {
			if rErr != nil {
				t.Fatalf("iter %d leave+join+renew goroutine %d error: %v", i, idx, rErr)
			}
		}
		if err := assertRosterAndMappingInvariant(); err != nil {
			t.Fatalf("iter %d after leave+join race: %v", i, err)
		}

		// 3. Leave otherRoom while concurrent renewals run on both rooms; mapping must not be resurrected.
		errs = testutil.RunRace(
			func() error {
				return svc.LeaveRoom(ctx, otherRoom, mover)
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomA, CharacterID: leaderA, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
			func() error {
				return repo.UpdateMember(ctx, casino.RoomMember{RoomID: roomB, CharacterID: leaderB, Action: "待機中", UpdatedAt: time.Now().UTC()})
			},
		)
		for idx, rErr := range errs {
			if rErr != nil {
				t.Fatalf("iter %d final leave+renew goroutine %d error: %v", i, idx, rErr)
			}
		}
		if err := assertRosterAndMappingInvariant(); err != nil {
			t.Fatalf("iter %d after final leave race: %v", i, err)
		}
	}
}

func TestValkeyRoomRenewal_ConcurrentCreateRoomRollbackBlocksJoinAndPropagatesErrors(t *testing.T) {
	rawClient := openIsolatedTestValkey(t, 0)
	defer rawClient.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	prefix := "createrace1091-" + id.New()[:8]
	creatorC := prefix + "-creatorC"
	joinerD := prefix + "-joinerD"
	leaderOther := prefix + "-leaderOther"
	allActors := []string{creatorC, joinerD, leaderOther}

	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{}}
	casinoRepo := newMockPrizeCasinoRepo()
	for _, actor := range allActors {
		chars.chars[actor] = corecharacter.Character{ID: actor, Name: actor, PlayerID: "owner-" + actor}
		casinoRepo.accounts[actor] = casino.Account{CharacterID: actor, Coins: 5000}
	}

	var createClaimCount atomic.Int32
	var winnerRoomID atomic.Value
	var loserRoomID atomic.Value
	bothPublished := make(chan struct{})
	loserHoldRelease := make(chan struct{})
	joinerAttemptedLock := make(chan struct{})
	var notifyJoinerLock atomic.Bool
	var injectRollbackErr atomic.Bool
	errCompensatoryFault := errors.New("injected compensatory rollback failure")

	hookClient := &hookValkeyClient{
		Client: rawClient,
		beforeDo: func(cmds []string) error {
			// Detect creatorC's claim_character_room.lua calls during concurrent CreateRoom.
			if len(cmds) == 7 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && cmds[3] == casino.DefaultCharacterKeyPrefix+creatorC {
				n := createClaimCount.Add(1)
				switch n {
				case 1:
					winnerRoomID.Store(cmds[4])
					<-bothPublished
				case 2:
					loserRoomID.Store(cmds[4])
					close(bothPublished)
					<-loserHoldRelease
				}
			}
			// Detect joinerD's room lock acquisition attempt on loserRoom while CreateRoom holds loserRoom's lock.
			if notifyJoinerLock.Load() && len(cmds) >= 3 && cmds[0] == "SET" {
				if lID, ok := loserRoomID.Load().(string); ok && lID != "" && cmds[1] == casino.DefaultRoomLockKeyPrefix+lID {
					if notifyJoinerLock.CompareAndSwap(true, false) {
						close(joinerAttemptedLock)
					}
				}
			}
			// Inject compensatory rollback failure when requested.
			if injectRollbackErr.Load() && len(cmds) >= 2 && cmds[0] == "DEL" && strings.HasPrefix(cmds[1], casino.DefaultRoomKeyPrefix) {
				return errCompensatoryFault
			}
			if injectRollbackErr.Load() && len(cmds) >= 2 && cmds[0] == "SET" && strings.HasPrefix(cmds[1], casino.DefaultRoomKeyPrefix) {
				return errCompensatoryFault
			}
			return nil
		},
	}

	repo, err := casino.NewValkeyRoomRepository(hookClient, casino.WithValkeyCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}

	var createdRooms []string
	t.Cleanup(func() {
		bg := context.Background()
		for _, rID := range createdRooms {
			_ = rawClient.Do(bg, rawClient.B().Del().Key(casino.DefaultRoomKeyPrefix+rID).Build())
			_ = rawClient.Do(bg, rawClient.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(rID).Build())
		}
		for _, cID := range allActors {
			_ = rawClient.Do(bg, rawClient.B().Del().Key(casino.DefaultCharacterKeyPrefix+cID).Build())
		}
	})

	// 1. Concurrent CreateRoom(Room 1) and CreateRoom(Room 2) by creatorC while joinerD attempts
	// JoinRoom(loserRoom) after loserRoom's payload and active index have been written to Valkey.
	joinDone := make(chan error, 1)
	go func() {
		<-bothPublished
		lID := loserRoomID.Load().(string)
		notifyJoinerLock.Store(true)
		go func() {
			_, jErr := svc.JoinRoom(ctx, lID, joinerD, "", 0)
			joinDone <- jErr
		}()
		<-joinerAttemptedLock
		close(loserHoldRelease)
	}()

	createErrs := testutil.RunRace(
		func() error {
			_, cErr := svc.CreateRoom(ctx, creatorC, casino.CreateRoomRequest{
				Name: "Create1-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
			})
			return cErr
		},
		func() error {
			_, cErr := svc.CreateRoom(ctx, creatorC, casino.CreateRoomRequest{
				Name: "Create2-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
			})
			return cErr
		},
	)

	wID := winnerRoomID.Load().(string)
	lID := loserRoomID.Load().(string)
	createdRooms = append(createdRooms, wID, lID)

	createWins := 0
	for idx, cErr := range createErrs {
		if cErr == nil {
			createWins++
		} else if !errors.Is(cErr, casino.ErrAlreadyInRoom) {
			t.Fatalf("CreateRoom goroutine %d unexpected error: %v", idx, cErr)
		}
	}
	if createWins != 1 {
		t.Fatalf("expected exactly 1 CreateRoom winner, got %d (errs=%v)", createWins, createErrs)
	}

	joinErr := <-joinDone
	if !errors.Is(joinErr, casino.ErrRoomNotFound) {
		t.Fatalf("expected joinerD JoinRoom(loserRoom) to fail with ErrRoomNotFound after locked rollback, got %v", joinErr)
	}

	// Verify loserRoom has no payload, no members, and no active index entry.
	if _, err := repo.GetRoom(ctx, lID); !errors.Is(err, casino.ErrRoomNotFound) {
		t.Fatalf("expected loserRoom payload deleted, got err=%v", err)
	}
	if mems, err := repo.ListMembers(ctx, lID); err != nil || len(mems) != 0 {
		t.Fatalf("expected loserRoom members empty, got %+v err=%v", mems, err)
	}
	zscoreErr := rawClient.Do(ctx, rawClient.B().Zscore().Key(casino.DefaultRoomsActiveIndexKey).Member(lID).Build()).Error()
	if !valkeygo.IsValkeyNil(zscoreErr) {
		t.Fatalf("expected loserRoom removed from active index, got zscore err=%v", zscoreErr)
	}

	// Verify joinerD has no orphaned character mapping and nil room view.
	if mapped, err := repo.GetCharacterRoom(ctx, joinerD); !errors.Is(err, casino.ErrRoomNotFound) {
		t.Fatalf("expected joinerD to have no character mapping, got %q err=%v", mapped, err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+joinerD, joinerD); err != nil || view != nil {
		t.Fatalf("expected joinerD room view nil, got %+v err=%v", view, err)
	}

	// Verify creatorC is mapped to winnerRoom and winnerRoom has only creatorC.
	if mapped, err := repo.GetCharacterRoom(ctx, creatorC); err != nil || mapped != wID {
		t.Fatalf("expected creatorC mapped to winnerRoom %s, got %q err=%v", wID, mapped, err)
	}
	if view, err := svc.GetCharacterRoomView(ctx, "owner-"+creatorC, creatorC); err != nil || view == nil || view.Room.ID != wID {
		t.Fatalf("expected creatorC view in winnerRoom %s, got %+v err=%v", wID, view, err)
	}

	// 2. Verify compensatory rollback failures in CreateRoom and AddMember are not silently discarded
	// and do not masquerade as ErrSecondarySync.
	otherRoomDetail, err := svc.CreateRoom(ctx, leaderOther, casino.CreateRoomRequest{
		Name: "Other-" + prefix, GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherRoomID := otherRoomDetail.Room.ID
	createdRooms = append(createdRooms, otherRoomID)

	// Force joinerD's pre-check to see nil, then set joinerD's mapping to wID before claim_character_room.lua,
	// and fail the compensatory DEL/SET during rollback.
	failRollbackRoomID := prefix + "-failrollback"
	createdRooms = append(createdRooms, failRollbackRoomID)
	hookClient.beforeDo = func(cmds []string) error {
		if len(cmds) == 7 && (cmds[0] == "EVAL" || cmds[0] == "EVALSHA") && cmds[3] == casino.DefaultCharacterKeyPrefix+joinerD {
			// Concurrent admission into wID won the race right before claim_character_room.lua:
			_ = rawClient.Do(ctx, rawClient.B().Set().Key(casino.DefaultCharacterKeyPrefix+joinerD).Value(wID).Ex(casino.DefaultLobbyTTL).Build()).Error()
			injectRollbackErr.Store(true)
		}
		if injectRollbackErr.Load() && len(cmds) >= 2 && cmds[0] == "DEL" && cmds[1] == casino.DefaultRoomKeyPrefix+failRollbackRoomID {
			return errCompensatoryFault
		}
		if injectRollbackErr.Load() && len(cmds) >= 2 && cmds[0] == "SET" && cmds[1] == casino.DefaultRoomKeyPrefix+otherRoomID {
			return errCompensatoryFault
		}
		return nil
	}

	now := time.Now().UTC()
	createRollbackErr := repo.CreateRoom(ctx, casino.Room{
		ID: failRollbackRoomID, Name: "FailRB-" + prefix, GameType: casino.GameTypeIndian, LeaderCharacterID: joinerD,
		Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10, Status: casino.RoomStatusWaiting, CreatedAt: now, UpdatedAt: now,
	}, casino.RoomMember{RoomID: failRollbackRoomID, CharacterID: joinerD, JoinedAt: now, UpdatedAt: now})
	injectRollbackErr.Store(false)
	if !errors.Is(createRollbackErr, casino.ErrAlreadyInRoom) || !errors.Is(createRollbackErr, errCompensatoryFault) {
		t.Fatalf("expected CreateRoom rollback error to join ErrAlreadyInRoom and compensatory fault, got %v", createRollbackErr)
	}
	if errors.Is(createRollbackErr, casino.ErrSecondarySync) {
		t.Fatalf("expected CreateRoom rollback error not to match ErrSecondarySync, got %v", createRollbackErr)
	}

	_ = rawClient.Do(ctx, rawClient.B().Del().Key(casino.DefaultCharacterKeyPrefix+joinerD).Build())
	addRollbackErr := repo.AddMember(ctx, casino.RoomMember{RoomID: otherRoomID, CharacterID: joinerD, JoinedAt: now, UpdatedAt: now})
	injectRollbackErr.Store(false)
	if !errors.Is(addRollbackErr, casino.ErrAlreadyInRoom) || !errors.Is(addRollbackErr, errCompensatoryFault) {
		t.Fatalf("expected AddMember rollback error to join ErrAlreadyInRoom and compensatory fault, got %v", addRollbackErr)
	}
	if errors.Is(addRollbackErr, casino.ErrSecondarySync) {
		t.Fatalf("expected AddMember rollback error not to match ErrSecondarySync, got %v", addRollbackErr)
	}
}
