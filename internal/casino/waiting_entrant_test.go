package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

// waitingRoom starts a game for w1/w2, then w3 joins mid-game as 待機中
// (party2/lib/casino.cgi:677-681).
func waitingRoom(t *testing.T, game casino.GameType, coins map[string]int64) (*casino.Service, *mockPrizeCasinoRepo, *casino.MemoryRoomRepository, string) {
	t.Helper()
	return startedRoom(t, game, coins, true)
}

func startedRoom(t *testing.T, game casino.GameType, coins map[string]int64, lateEntrant bool) (*casino.Service, *mockPrizeCasinoRepo, *casino.MemoryRoomRepository, string) {
	t.Helper()
	ctx := context.Background()
	accounts, rooms := newMockPrizeCasinoRepo(), newMockMemoryRoomRepo()
	for _, id := range []string{"w1", "w2", "w3"} {
		accounts.accounts[id] = casino.Account{CharacterID: id, Coins: 500}
	}
	svc, err := casino.NewService(accounts, casino.WithRoomRepository(rooms))
	if err != nil {
		t.Fatal(err)
	}
	room, err := svc.CreateRoom(ctx, "w1", casino.CreateRoomRequest{Name: "WaitTable", GameType: game, Speed: casino.SpeedNormal, MaxPlayers: 4, Rate: 10, AllowSpectators: true})
	if err != nil {
		t.Fatal(err)
	}
	id := room.Room.ID
	if _, err := svc.JoinRoom(ctx, id, "w2", "", 0); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartGame(ctx, id, "w1"); err != nil {
		t.Fatal(err)
	}
	if lateEntrant {
		if _, err := svc.JoinRoom(ctx, id, "w3", "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if game != casino.GameTypeDoppel { // Doppel cards are the selected marks
		setCard(t, rooms, id, "w1", 11)
		setCard(t, rooms, id, "w2", 1)
	}
	for cid, c := range coins { // balances after admission, which needs rate*5 to create
		accounts.accounts[cid] = casino.Account{CharacterID: cid, Coins: c}
	}
	return svc, accounts, rooms, id
}

func setCard(t *testing.T, rooms *casino.MemoryRoomRepository, roomID, id string, card int) {
	t.Helper()
	m, err := rooms.GetMember(context.Background(), roomID, id)
	if err != nil {
		t.Fatal(err)
	}
	m.Card = card
	if err := rooms.UpdateMember(context.Background(), *m); err != nil {
		t.Fatal(err)
	}
}

func play(t *testing.T, svc *casino.Service, roomID, id, action string) *casino.RoomDetail {
	t.Helper()
	d, err := svc.PlayRoomAction(context.Background(), roomID, id, action)
	if err != nil {
		t.Fatalf("%s %s: %v", id, action, err)
	}
	return d
}

func memberAction(t *testing.T, rooms *casino.MemoryRoomRepository, roomID, id string) string {
	t.Helper()
	m, err := rooms.GetMember(context.Background(), roomID, id)
	if err != nil {
		t.Fatal(err)
	}
	return m.Action
}

func assertWaitingRejected(t *testing.T, svc *casino.Service, accounts *mockPrizeCasinoRepo, rooms *casino.MemoryRoomRepository, roomID, action string) {
	t.Helper()
	ctx := context.Background()
	before, _ := rooms.GetRoom(ctx, roomID)
	if _, err := svc.PlayRoomAction(ctx, roomID, "w3", action); !errors.Is(err, casino.ErrAlreadyActed) {
		t.Fatalf("waiting entrant %s: want ErrAlreadyActed, got %v", action, err)
	}
	after, _ := rooms.GetRoom(ctx, roomID)
	if acc, _ := accounts.GetAccount(ctx, "w3"); acc.Coins != 500 || after.Pot != before.Pot {
		t.Fatalf("rejected action changed coins %d or pot %d->%d", acc.Coins, before.Pot, after.Pot)
	}
}

// Legacy handlers reject any declared action including 待機中, rounds count
// 待機中 as acted, resets keep it and showdowns skip it
// (casino_indian.cgi:66-102,118-141,156).
func TestIndianWaitingEntrantSitsOut(t *testing.T) {
	svc, accounts, rooms, id := waitingRoom(t, casino.GameTypeIndian, nil)
	assertWaitingRejected(t, svc, accounts, rooms, id, "call")
	play(t, svc, id, "w1", "call")
	if d := play(t, svc, id, "w2", "call"); d.Room.Round != 2 {
		t.Fatalf("waiting entrant stalled the round: round=%d", d.Room.Round)
	}
	if got := memberAction(t, rooms, id, "w3"); got != "待機中" {
		t.Fatalf("next round reset the waiting entrant to %q", got)
	}
	play(t, svc, id, "w1", "showdown")
	d := play(t, svc, id, "w2", "showdown")
	if d.Room.Round != 0 || d.Room.WinnerCharacterID == nil || *d.Room.WinnerCharacterID != "w1" {
		t.Fatalf("showdown: round=%d winner=%v", d.Room.Round, d.Room.WinnerCharacterID)
	}
	if acc, _ := accounts.GetAccount(context.Background(), "w3"); acc.Coins != 500 {
		t.Fatalf("waiting entrant paid: %d", acc.Coins)
	}
	if _, err := svc.StartGame(context.Background(), id, "w1"); err != nil {
		t.Fatal(err)
	}
	play(t, svc, id, "w3", "call")
}

func TestHighLowWaitingEntrantSitsOut(t *testing.T) {
	svc, accounts, rooms, id := waitingRoom(t, casino.GameTypeHighLow, nil)
	assertWaitingRejected(t, svc, accounts, rooms, id, "high")
	play(t, svc, id, "w1", "call")
	if d := play(t, svc, id, "w2", "call"); d.Room.Round != 2 {
		t.Fatalf("waiting entrant stalled the round: round=%d", d.Room.Round)
	}
	if got := memberAction(t, rooms, id, "w3"); got != "待機中" {
		t.Fatalf("next round reset the waiting entrant to %q", got)
	}
	play(t, svc, id, "w1", "high")
	if d := play(t, svc, id, "w2", "low"); d.Room.Round != 0 {
		t.Fatalf("showdown did not settle: round=%d", d.Room.Round)
	}
	if acc, _ := accounts.GetAccount(context.Background(), "w3"); acc.Coins != 500 {
		t.Fatalf("waiting entrant paid: %d", acc.Coins)
	}
}

// Doppel registers no marks for a member with an action, and is_all_select
// and battle skip it (casino_doppel.cgi:11-12,41-42,93-111).
func TestDoppelWaitingEntrantSitsOut(t *testing.T) {
	svc, accounts, _, id := waitingRoom(t, casino.GameTypeDoppel, nil)
	ctx := context.Background()
	if _, err := svc.PlayDoppelAction(ctx, id, "w3", 0); !errors.Is(err, casino.ErrAlreadyActed) {
		t.Fatalf("waiting entrant mark: %v", err)
	}
	// Mark count stays at the starting two members + 1 (now_bet = @members at start).
	if _, err := svc.PlayDoppelAction(ctx, id, "w1", 3); !errors.Is(err, casino.ErrInvalidDoppelMarkIndex) {
		t.Fatalf("waiting entrant widened the mark range: %v", err)
	}
	if _, err := svc.PlayDoppelAction(ctx, id, "w1", 0); err != nil {
		t.Fatal(err)
	}
	d, err := svc.PlayDoppelAction(ctx, id, "w2", 1)
	if err != nil {
		t.Fatal(err)
	}
	if d.Room.Round != 0 || d.Room.WinnerCharacterID == nil || *d.Room.WinnerCharacterID != "w1" {
		t.Fatalf("waiting entrant held up the showdown: round=%d winner=%v", d.Room.Round, d.Room.WinnerCharacterID)
	}
	if acc, _ := accounts.GetAccount(ctx, "w3"); acc.Coins != 500 {
		t.Fatalf("waiting entrant paid: %d", acc.Coins)
	}
}

// つづける is registered only while now_bet < max_bet && coin >= now_bet
// (casino_highlow.cgi:13-16).
func TestHighLowCallEligibility(t *testing.T) {
	ctx := context.Background()
	t.Run("short of coins", func(t *testing.T) {
		svc, accounts, rooms, id := waitingRoom(t, casino.GameTypeHighLow, map[string]int64{"w1": 9})
		if _, err := svc.PlayRoomAction(ctx, id, "w1", "call"); !errors.Is(err, casino.ErrHighLowCallUnavailable) {
			t.Fatalf("want ErrHighLowCallUnavailable, got %v", err)
		}
		if acc, _ := accounts.GetAccount(ctx, "w1"); acc.Coins != 9 {
			t.Fatalf("rejected call paid: %d", acc.Coins)
		}
		if r, _ := rooms.GetRoom(ctx, id); r.Pot != 0 {
			t.Fatalf("rejected call changed pot: %d", r.Pot)
		}
		play(t, svc, id, "w1", "high")
	})
	t.Run("exact coins", func(t *testing.T) {
		svc, _, _, id := waitingRoom(t, casino.GameTypeHighLow, map[string]int64{"w1": 10})
		play(t, svc, id, "w1", "call")
	})
	t.Run("max bet", func(t *testing.T) {
		svc, _, rooms, id := waitingRoom(t, casino.GameTypeHighLow, nil)
		r, _ := rooms.GetRoom(ctx, id)
		r.CurrentBet = r.MaxBet
		if err := rooms.UpdateRoom(ctx, *r); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PlayRoomAction(ctx, id, "w1", "call"); !errors.Is(err, casino.ErrHighLowCallUnavailable) {
			t.Fatalf("want ErrHighLowCallUnavailable at max bet, got %v", err)
		}
	})
}

// The coin condition reads only the acting player's balance after paying
// ($m{coin} <= 0, casino_indian.cgi:128, casino_highlow.cgi:146).
func TestRoundCoinConditionUsesActor(t *testing.T) {
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow} {
		t.Run(string(game)+" earlier all-in continues", func(t *testing.T) {
			svc, _, _, id := waitingRoom(t, game, map[string]int64{"w1": 10})
			play(t, svc, id, "w1", "call")
			if d := play(t, svc, id, "w2", "call"); d.Room.Round != 2 {
				t.Fatalf("non-actor's empty balance forced a showdown: round=%d", d.Room.Round)
			}
		})
		t.Run(string(game)+" actor reaching zero settles", func(t *testing.T) {
			svc, _, _, id := waitingRoom(t, game, map[string]int64{"w2": 10})
			play(t, svc, id, "w1", "call")
			if d := play(t, svc, id, "w2", "call"); d.Room.Round != 0 {
				t.Fatalf("actor's empty balance did not settle: round=%d", d.Room.Round)
			}
		})
	}
}

// High & Low has no all-but-one-folded rule: a lone caller continues and can
// declare next round; only everyone folding voids the pot (casino_highlow.cgi:146,207-210).
func TestHighLowLoneCallerContinues(t *testing.T) {
	svc, accounts, _, id := startedRoom(t, casino.GameTypeHighLow, nil, false)
	play(t, svc, id, "w1", "call")
	if d := play(t, svc, id, "w2", "fold"); d.Room.Round != 2 {
		t.Fatalf("lone caller's round was settled: round=%d", d.Room.Round)
	}
	d := play(t, svc, id, "w1", "high")
	if d.Room.Round != 0 || d.Room.WinnerCharacterID == nil || *d.Room.WinnerCharacterID != "w1" {
		t.Fatalf("high did not win: round=%d winner=%v", d.Room.Round, d.Room.WinnerCharacterID)
	}
	// 500 - 10 - 20 + pot (10 + 10 + 20)
	if acc, _ := accounts.GetAccount(context.Background(), "w1"); acc.Coins != 510 {
		t.Fatalf("winner coins = %d, want 510", acc.Coins)
	}

	// A lone caller whose own balance runs out reaches battle with no
	// declaration, which voids the pot (お流れ, casino_highlow.cgi:207-210).
	svc, _, _, id = startedRoom(t, casino.GameTypeHighLow, map[string]int64{"w1": 10}, false)
	play(t, svc, id, "w2", "fold")
	if d := play(t, svc, id, "w1", "call"); d.Room.Round != 0 || d.Room.WinnerCharacterID != nil {
		t.Fatalf("undeclared pot was awarded: round=%d winner=%v", d.Room.Round, d.Room.WinnerCharacterID)
	}
}
