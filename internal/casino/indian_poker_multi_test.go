package casino_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestMultiplayerIndianPoker_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "char-p1"
	p2 := "char-p2"
	p3 := "char-p3"

	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}
	casinoRepo.accounts[p3] = casino.Account{CharacterID: p3, Coins: 500}

	// 1. P1 creates room
	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:            "HighRollers",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedNormal,
		MaxPlayers:      4,
		Rate:            10,
		AllowSpectators: true,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	// 2. P2 and P3 join
	_, err = svc.JoinRoom(ctx, roomID, p2, "", 0)
	if err != nil {
		t.Fatalf("P2 JoinRoom failed: %v", err)
	}
	_, err = svc.JoinRoom(ctx, roomID, p3, "", 0)
	if err != nil {
		t.Fatalf("P3 JoinRoom failed: %v", err)
	}

	// 3. P1 (leader) starts the game
	startedDetail, err := svc.StartIndianPoker(ctx, roomID, p1)
	if err != nil {
		t.Fatalf("StartIndianPoker failed: %v", err)
	}
	if startedDetail.Room.Round != 1 || startedDetail.Room.Status != casino.RoomStatusInProgress {
		t.Fatalf("unexpected room state: round=%d status=%s", startedDetail.Room.Round, startedDetail.Room.Status)
	}

	// 4. Card masking check (Forehead card rule: see others, not own!)
	p1View, err := svc.GetRoomDetail(ctx, roomID, p1)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range p1View.Members {
		if m.CharacterID == p1 {
			if m.Card != -1 || m.CardDisplay != "？" {
				t.Errorf("P1 should NOT see their own card while active, got card=%d, display=%s", m.Card, m.CardDisplay)
			}
		} else {
			if m.Card < 0 || m.CardDisplay == "？" {
				t.Errorf("P1 SHOULD see %s's card, got card=%d display=%s", m.CharacterID, m.Card, m.CardDisplay)
			}
		}
	}

	// 5. Explicitly inject fixed cards for deterministic test
	// P1: 10 (Card 9), P2: K (Card 12), P3: 2 (Card 1)
	m1, _ := roomRepo.GetMember(ctx, roomID, p1)
	m1.Card = 9
	_ = roomRepo.UpdateMember(ctx, *m1)

	m2, _ := roomRepo.GetMember(ctx, roomID, p2)
	m2.Card = 12
	_ = roomRepo.UpdateMember(ctx, *m2)

	m3, _ := roomRepo.GetMember(ctx, roomID, p3)
	m3.Card = 1
	_ = roomRepo.UpdateMember(ctx, *m3)

	// 6. Round 1: All players call
	// Current bet is 10.
	_, err = svc.PlayIndianPokerAction(ctx, roomID, p1, casino.ActionCall)
	if err != nil {
		t.Fatalf("P1 Call error: %v", err)
	}
	_, err = svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionCall)
	if err != nil {
		t.Fatalf("P2 Call error: %v", err)
	}
	r1Detail, err := svc.PlayIndianPokerAction(ctx, roomID, p3, casino.ActionCall)
	if err != nil {
		t.Fatalf("P3 Call error: %v", err)
	}

	// Verify Round 1 advanced to Round 2!
	// Pot should have 10 * 3 = 30 coins. Current bet should now be 10 + 10 = 20 coins.
	if r1Detail.Room.Round != 2 {
		t.Fatalf("expected Round 2, got %d", r1Detail.Room.Round)
	}
	if r1Detail.Room.CurrentBet != 20 {
		t.Errorf("expected current bet 20, got %d", r1Detail.Room.CurrentBet)
	}
	if r1Detail.Room.Pot != 30 {
		t.Errorf("expected pot 30, got %d", r1Detail.Room.Pot)
	}

	// P3 folds: pays 20 coins into pot (pot becomes 50)
	_, err = svc.PlayIndianPokerAction(ctx, roomID, p3, casino.ActionFold)
	if err != nil {
		t.Fatalf("P3 Fold error: %v", err)
	}
	// P3 folded: their own card should now be visible to them!
	p3View, err := svc.GetRoomDetail(ctx, roomID, p3)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range p3View.Members {
		if m.CharacterID == p3 && (m.Card == -1 || m.CardDisplay == "？") {
			t.Errorf("expected folded player P3 to see their own card, got card=%d display=%s", m.Card, m.CardDisplay)
		}
	}
	// P1 calls: pays 20 coins into pot (pot becomes 70)
	_, err = svc.PlayIndianPokerAction(ctx, roomID, p1, casino.ActionCall)
	if err != nil {
		t.Fatalf("P1 Call error: %v", err)
	}
	// P2 showdowns: pays 20 coins into pot (pot becomes 90) -> Showdown triggers!
	finalDetail, err := svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("P2 Showdown error: %v", err)
	}

	// 8. Verify Showdown outcome:
	// P2 had Card 12 (K), P1 had Card 9 (10), P3 folded.
	// Winner must be P2!
	if finalDetail.Room.Round != 0 {
		t.Errorf("expected round reset to 0 after showdown, got %d", finalDetail.Room.Round)
	}
	if finalDetail.Room.WinnerCharacterID == nil || *finalDetail.Room.WinnerCharacterID != p2 {
		t.Errorf("expected winner P2, got %v", finalDetail.Room.WinnerCharacterID)
	}

	// P2 paid: 10 (R1) + 20 (R2) = 30 coins. P2 gets 90 pot coins.
	// P2 balance: 500 - 30 + 90 = 560 coins!
	p2Acc, _ := casinoRepo.GetAccount(ctx, p2)
	if p2Acc.Coins != 560 {
		t.Errorf("expected P2 coins 560, got %d", p2Acc.Coins)
	}

	// P1 paid: 10 + 20 = 30. Balance: 470
	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	if p1Acc.Coins != 470 {
		t.Errorf("expected P1 coins 470, got %d", p1Acc.Coins)
	}

	// After showdown, all cards are revealed
	revealedView, _ := svc.GetRoomDetail(ctx, roomID, p1)
	for _, m := range revealedView.Members {
		if m.CardDisplay == "？" {
			t.Errorf("expected revealed cards after showdown for %s, got '？'", m.CharacterID)
		}
	}
}

func TestMultiplayerIndianPoker_AllFoldWinner(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "char-a"
	p2 := "char-b"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 100}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 100}

	detail, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "QuickHeadsUp",
		GameType:   casino.GameTypeIndian,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	roomID := detail.Room.ID
	_, _ = svc.JoinRoom(ctx, roomID, p2, "", 0)
	_, _ = svc.StartIndianPoker(ctx, roomID, p1)

	// P2 folds immediately in Round 1
	res, err := svc.PlayIndianPokerAction(ctx, roomID, p2, casino.ActionFold)
	if err != nil {
		t.Fatalf("P2 Fold error: %v", err)
	}

	// All players except P1 folded -> P1 wins remaining pot!
	if res.Room.WinnerCharacterID == nil || *res.Room.WinnerCharacterID != p1 {
		t.Errorf("expected P1 to win by fold, got %v", res.Room.WinnerCharacterID)
	}
	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	// P2 paid 10 on fold into pot, P1 gets 10 pot -> P1 coins = 100 + 10 = 110!
	if p1Acc.Coins != 110 {
		t.Errorf("expected P1 coins 110, got %d", p1Acc.Coins)
	}
}
