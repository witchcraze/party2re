package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func setupThreePlayerDepartureRoom(t *testing.T, game casino.GameType, coins map[string]int64) (*casino.Service, *mockPrizeCasinoRepo, *casino.MemoryRoomRepository, *inMemoryCharRepo, string) {
	t.Helper()
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := casino.NewMemoryRoomRepository()
	charRepo := &inMemoryCharRepo{chars: make(map[string]corecharacter.Character)}
	for _, id := range []string{"p1", "p2", "p3", "spec"} {
		charRepo.chars[id] = corecharacter.Character{ID: id, PlayerID: "owner-" + id, Name: id}
		casinoRepo.accounts[id] = casino.Account{CharacterID: id, Coins: 500}
	}
	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithCharacterRepository(charRepo),
		casino.WithDepartureRepository(casino.NewMemoryDepartureRepository()),
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.CreateRoom(ctx, "p1", casino.CreateRoomRequest{
		Name: "Dep-" + string(game), GameType: game, Speed: casino.SpeedFast, MaxPlayers: 4, Rate: 10, AllowSpectators: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomID := detail.Room.ID
	for _, id := range []string{"p2", "p3"} {
		if _, err := svc.JoinRoom(ctx, roomID, id, "", 0); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := svc.StartGame(ctx, roomID, "p1"); err != nil {
		t.Fatal(err)
	}
	if game != casino.GameTypeDoppel {
		setCard(t, roomRepo, roomID, "p1", 12)
		setCard(t, roomRepo, roomID, "p2", 2)
		setCard(t, roomRepo, roomID, "p3", 6)
	}
	for id, c := range coins {
		casinoRepo.accounts[id] = casino.Account{CharacterID: id, Coins: c}
	}
	return svc, casinoRepo, roomRepo, charRepo, roomID
}

func TestLeaveRoom_IndianPokerRoundReevaluation(t *testing.T) {
	ctx := context.Background()

	t.Run("last unacted leaver with coins advances round", func(t *testing.T) {
		svc, _, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeIndian, nil)
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "call")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 2 || room.CurrentBet != 20 || room.Status != casino.RoomStatusInProgress {
			t.Fatalf("expected round 2 / bet 20, got %+v", room)
		}
		for _, id := range []string{"p1", "p2"} {
			if act := memberAction(t, roomRepo, roomID, id); act != "" {
				t.Fatalf("expected cleared action for %s, got %q", id, act)
			}
		}
		ch3, _ := charRepo.FindByID(ctx, "p3")
		if ch3.Tired != 1 {
			t.Fatalf("expected leaver tired=1, got %d", ch3.Tired)
		}
	})

	t.Run("last unacted leaver with zero coins settles showdown", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeIndian, map[string]int64{"p3": 0})
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "call")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.Status != casino.RoomStatusWaiting || room.WinnerCharacterID == nil || *room.WinnerCharacterID != "p1" {
			t.Fatalf("expected p1 showdown win, got %+v", room)
		}
		acc1, _ := casinoRepo.GetAccount(ctx, "p1")
		ch1, _ := charRepo.FindByID(ctx, "p1")
		if acc1.Coins != 510 || ch1.CasinoWins != 1 {
			t.Fatalf("expected p1 coins=510 wins=1, got coins=%d wins=%d", acc1.Coins, ch1.CasinoWins)
		}
	})

	t.Run("leaver leaves one non-folded remaining player -> showdown", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeIndian, nil)
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "fold")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.WinnerCharacterID == nil || *room.WinnerCharacterID != "p1" {
			t.Fatalf("expected p1 fold-out win, got %+v", room)
		}
		acc1, _ := casinoRepo.GetAccount(ctx, "p1")
		ch1, _ := charRepo.FindByID(ctx, "p1")
		if acc1.Coins != 510 || ch1.CasinoWins != 1 {
			t.Fatalf("expected p1 coins=510 wins=1, got coins=%d wins=%d", acc1.Coins, ch1.CasinoWins)
		}
	})
}

func TestLeaveRoom_HighLowRoundReevaluation(t *testing.T) {
	ctx := context.Background()

	t.Run("last unacted leaver with coins advances round", func(t *testing.T) {
		svc, _, roomRepo, _, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeHighLow, nil)
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "call")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 2 || room.CurrentBet != 20 {
			t.Fatalf("expected round 2 / bet 20, got %+v", room)
		}
	})

	t.Run("last unacted leaver with zero coins settles showdown", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeHighLow, map[string]int64{"p3": 0})
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "call")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.WinnerCharacterID != nil || room.Pot != 0 {
			t.Fatalf("expected voided showdown, got %+v", room)
		}
		acc1, _ := casinoRepo.GetAccount(ctx, "p1")
		ch1, _ := charRepo.FindByID(ctx, "p1")
		if acc1.Coins != 490 || ch1.CasinoWins != 0 {
			t.Fatalf("expected voided pot, got coins=%d wins=%d", acc1.Coins, ch1.CasinoWins)
		}
	})

	t.Run("shrunken roster to 2 does not split between high and low", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeHighLow, nil)
		play(t, svc, roomID, "p1", "high")
		play(t, svc, roomID, "p2", "low")
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.WinnerCharacterID == nil || *room.WinnerCharacterID != "p1" {
			t.Fatalf("expected high-only winner p1 on 2 remaining players, got %+v", room)
		}
		acc1, _ := casinoRepo.GetAccount(ctx, "p1")
		acc2, _ := casinoRepo.GetAccount(ctx, "p2")
		ch1, _ := charRepo.FindByID(ctx, "p1")
		ch2, _ := charRepo.FindByID(ctx, "p2")
		if acc1.Coins != 510 || acc2.Coins != 490 || ch1.CasinoWins != 1 || ch2.CasinoWins != 0 {
			t.Fatalf("unexpected payout/wins: p1=(%d,%d) p2=(%d,%d)", acc1.Coins, ch1.CasinoWins, acc2.Coins, ch2.CasinoWins)
		}
	})
}

func TestLeaveRoom_DoppelMarkRangeAndRoundReevaluation(t *testing.T) {
	ctx := context.Background()

	t.Run("mark range stays at starting count after departure and settles on last unacted leaver", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeDoppel, nil)
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		// Started with 3 players -> marks 0..3 stay valid after p3 leaves; index 4 is rejected.
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p1", 4); !errors.Is(err, casino.ErrInvalidDoppelMarkIndex) {
			t.Fatalf("expected ErrInvalidDoppelMarkIndex for mark 4, got %v", err)
		}
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p1", 3); err != nil {
			t.Fatalf("expected mark 3 to remain valid after departure, got %v", err)
		}
		view, err := svc.GetRoomView(ctx, roomID, "owner-p2", "p2")
		if err != nil {
			t.Fatal(err)
		}
		controls, err := view.Controls("p2", 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(controls.Actions) != 4 {
			t.Fatalf("expected 4 marks in controls after departure, got %v", controls.Actions)
		}
		res, err := svc.PlayDoppelAction(ctx, roomID, "p2", 3)
		if err != nil {
			t.Fatal(err)
		}
		if res.Room.Round != 0 || res.Room.WinnerCharacterID == nil || *res.Room.WinnerCharacterID != "p2" {
			t.Fatalf("expected p2 child win, got %+v", res.Room)
		}
		acc2, _ := casinoRepo.GetAccount(ctx, "p2")
		ch2, _ := charRepo.FindByID(ctx, "p2")
		if acc2.Coins != 510 || ch2.CasinoWins != 1 {
			t.Fatalf("expected p2 coins=510 wins=1, got coins=%d wins=%d", acc2.Coins, ch2.CasinoWins)
		}
		_ = roomRepo
	})

	t.Run("last unselected child leaves after leader and matching child -> child wins", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeDoppel, nil)
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p1", 2); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p2", 2); err != nil {
			t.Fatal(err)
		}
		if err := svc.LeaveRoom(ctx, roomID, "p3"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.LeaderCharacterID != "p2" || room.WinnerCharacterID == nil || *room.WinnerCharacterID != "p2" {
			t.Fatalf("expected p2 child win and leadership, got %+v", room)
		}
		acc2, _ := casinoRepo.GetAccount(ctx, "p2")
		ch2, _ := charRepo.FindByID(ctx, "p2")
		if acc2.Coins != 510 || ch2.CasinoWins != 1 {
			t.Fatalf("expected p2 coins=510 wins=1, got coins=%d wins=%d", acc2.Coins, ch2.CasinoWins)
		}
	})

	t.Run("unselected leader leaves after all children selected -> parent wins pot and leadership transfers", func(t *testing.T) {
		svc, casinoRepo, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeDoppel, nil)
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p2", 0); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.PlayDoppelAction(ctx, roomID, "p3", 1); err != nil {
			t.Fatal(err)
		}
		if err := svc.LeaveRoom(ctx, roomID, "p1"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		if room.Round != 0 || room.WinnerCharacterID == nil || *room.WinnerCharacterID != "p1" || room.LeaderCharacterID == "p1" {
			t.Fatalf("expected parent p1 win and leader handover, got %+v", room)
		}
		acc1, _ := casinoRepo.GetAccount(ctx, "p1")
		ch1, _ := charRepo.FindByID(ctx, "p1")
		if acc1.Coins != 520 || ch1.Tired != 1 || ch1.CasinoWins != 1 {
			t.Fatalf("expected p1 coins=520 tired=1 wins=1, got coins=%d tired=%d wins=%d", acc1.Coins, ch1.Tired, ch1.CasinoWins)
		}
	})

	t.Run("spectator and pre-start departures do not evaluate or add fatigue for spectator", func(t *testing.T) {
		svc, _, roomRepo, charRepo, roomID := setupThreePlayerDepartureRoom(t, casino.GameTypeIndian, nil)
		if _, err := svc.SpectateRoom(ctx, roomID, "spec", ""); err != nil {
			t.Fatal(err)
		}
		play(t, svc, roomID, "p1", "call")
		play(t, svc, roomID, "p2", "call")
		if err := svc.LeaveRoom(ctx, roomID, "spec"); err != nil {
			t.Fatal(err)
		}
		room, _ := roomRepo.GetRoom(ctx, roomID)
		chSpec, _ := charRepo.FindByID(ctx, "spec")
		if room.Round != 1 || chSpec.Tired != 0 {
			t.Fatalf("spectator departure affected round or fatigue: room=%+v tired=%d", room, chSpec.Tired)
		}
	})
}
