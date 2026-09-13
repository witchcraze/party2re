package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestParseHighLowAction(t *testing.T) {
	tests := []struct {
		input   string
		want    casino.HighLowAction
		wantErr bool
	}{
		{"call", casino.HighLowActionCall, false},
		{"tsuzukeru", casino.HighLowActionCall, false},
		{"つづける", casino.HighLowActionCall, false},
		{"high", casino.HighLowActionHigh, false},
		{"ハイ", casino.HighLowActionHigh, false},
		{"low", casino.HighLowActionLow, false},
		{"ロウ", casino.HighLowActionLow, false},
		{"fold", casino.HighLowActionFold, false},
		{"oriru", casino.HighLowActionFold, false},
		{"おりる", casino.HighLowActionFold, false},
		{"invalid", "", true},
		{"", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := casino.ParseHighLowAction(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseHighLowAction(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseHighLowAction(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMultiplayerHighLow_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "hl-p1"
	p2 := "hl-p2"
	p3 := "hl-p3"

	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}
	casinoRepo.accounts[p3] = casino.Account{CharacterID: p3, Coins: 500}

	// 1. P1 creates High-Low room
	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:            "HighLowTable",
		GameType:        casino.GameTypeHighLow,
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
	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("P2 JoinRoom failed: %v", err)
	}
	if _, err := svc.JoinRoom(ctx, roomID, p3, "", 0); err != nil {
		t.Fatalf("P3 JoinRoom failed: %v", err)
	}

	// Non-leader cannot start
	if _, err := svc.StartHighLow(ctx, roomID, p2); !errors.Is(err, casino.ErrNotLeader) {
		t.Errorf("expected ErrNotLeader, got %v", err)
	}

	// 3. P1 (leader) starts the game
	startedDetail, err := svc.StartHighLow(ctx, roomID, p1)
	if err != nil {
		t.Fatalf("StartHighLow failed: %v", err)
	}
	if startedDetail.Room.Round != 1 || startedDetail.Room.Status != casino.RoomStatusInProgress {
		t.Fatalf("unexpected room state: round=%d status=%s", startedDetail.Room.Round, startedDetail.Room.Status)
	}

	// 4. Card & action masking check (party2/lib/casino_highlow.cgi:33-50)
	// In High-Low: player sees their OWN card; other players' cards are masked as "？"
	p1View, err := svc.GetRoomDetail(ctx, roomID, p1)
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range p1View.Members {
		if m.CharacterID == p1 {
			if m.Card < 0 || m.CardDisplay == "？" {
				t.Errorf("P1 SHOULD see their own card, got card=%d display=%s", m.Card, m.CardDisplay)
			}
		} else {
			if m.Card != -1 || m.CardDisplay != "？" {
				t.Errorf("P1 should NOT see %s's card, got card=%d display=%s", m.CharacterID, m.Card, m.CardDisplay)
			}
		}
	}

	// 5. Inject deterministic cards:
	// P1: Q (Card 11), P2: 2 (Card 1), P3: 7 (Card 6)
	m1, _ := roomRepo.GetMember(ctx, roomID, p1)
	m1.Card = 11
	_ = roomRepo.UpdateMember(ctx, *m1)

	m2, _ := roomRepo.GetMember(ctx, roomID, p2)
	m2.Card = 1
	_ = roomRepo.UpdateMember(ctx, *m2)

	m3, _ := roomRepo.GetMember(ctx, roomID, p3)
	m3.Card = 6
	_ = roomRepo.UpdateMember(ctx, *m3)

	// 6. Round 1: Everyone calls (tsuzukeru)
	if _, err := svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionCall); err != nil {
		t.Fatalf("P1 Call error: %v", err)
	}
	if _, err := svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionCall); err != nil {
		t.Fatalf("P2 Call error: %v", err)
	}
	r1Detail, err := svc.PlayHighLowAction(ctx, roomID, p3, casino.HighLowActionCall)
	if err != nil {
		t.Fatalf("P3 Call error: %v", err)
	}

	// Verify Round 1 advanced to Round 2!
	// Pot = 30, CurrentBet = 20
	if r1Detail.Room.Round != 2 {
		t.Fatalf("expected Round 2, got %d", r1Detail.Room.Round)
	}
	if r1Detail.Room.CurrentBet != 20 {
		t.Errorf("expected current bet 20, got %d", r1Detail.Room.CurrentBet)
	}
	if r1Detail.Room.Pot != 30 {
		t.Errorf("expected pot 30, got %d", r1Detail.Room.Pot)
	}

	// Action masking: P1 calls High, P2 calls Low.
	// When P2 views room, P1's action should be masked as "？？？" (not revealing High)
	if _, err := svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionHigh); err != nil {
		t.Fatalf("P1 High error: %v", err)
	}
	p2View, _ := svc.GetRoomDetail(ctx, roomID, p2)
	for _, m := range p2View.Members {
		if m.CharacterID == p1 && m.Action != "？？？" {
			t.Errorf("P2 should see P1's action masked as '？？？', got %s", m.Action)
		}
	}

	if _, err := svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionLow); err != nil {
		t.Fatalf("P2 Low error: %v", err)
	}

	// P3 calls. All 3 players have now acted in Round 2. Since 2 out of 3 chose High/Low (>= 50%), showdown triggers!
	finalDetail, err := svc.PlayHighLowAction(ctx, roomID, p3, casino.HighLowActionCall)
	if err != nil {
		t.Fatalf("P3 Call error: %v", err)
	}

	// 7. Verify Showdown & 50/50 Pot Split:
	// Pot: 30 (R1) + 20 (P1 R2) + 20 (P2 R2) + 20 (P3 R2) = 90 coins.
	// Highest card: P1 with Q (11). Lowest card: P2 with 2 (1).
	// Because participants > 2 and both higher and lower exist, pot is split: 90 / 2 = 45 each!
	if finalDetail.Room.Round != 0 {
		t.Errorf("expected round reset to 0, got %d", finalDetail.Room.Round)
	}
	if finalDetail.Room.WinnerCharacterID == nil || *finalDetail.Room.WinnerCharacterID != p1+","+p2 {
		t.Errorf("expected split winners %s,%s, got %v", p1, p2, finalDetail.Room.WinnerCharacterID)
	}

	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	p2Acc, _ := casinoRepo.GetAccount(ctx, p2)
	p3Acc, _ := casinoRepo.GetAccount(ctx, p3)

	// P1: 500 - 10 (R1) - 20 (R2) + 45 = 515
	if p1Acc.Coins != 515 {
		t.Errorf("P1 coins = %d, want 515", p1Acc.Coins)
	}
	// P2: 500 - 10 (R1) - 20 (R2) + 45 = 515
	if p2Acc.Coins != 515 {
		t.Errorf("P2 coins = %d, want 515", p2Acc.Coins)
	}
	// P3: 500 - 10 (R1) - 20 (R2) = 470
	if p3Acc.Coins != 470 {
		t.Errorf("P3 coins = %d, want 470", p3Acc.Coins)
	}
}

func TestMultiplayerHighLow_TwoPlayer_LowProhibited(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	p1 := "hl-2p1"
	p2 := "hl-2p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 100}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 100}

	detail, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "TwoPlayerTable",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	roomID := detail.Room.ID
	_, _ = svc.JoinRoom(ctx, roomID, p2, "", 0)
	_, _ = svc.StartHighLow(ctx, roomID, p1)

	// Low is prohibited when <= 2 players
	_, err = svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionLow)
	if !errors.Is(err, casino.ErrLowNotAllowedTwo) {
		t.Errorf("expected ErrLowNotAllowedTwo for 2-player Low action, got %v", err)
	}
}

func TestMultiplayerHighLow_SingleWinner_And_AllFold(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	p1 := "hl-s1"
	p2 := "hl-s2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 100}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 100}

	detail, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "HeadsUpTable",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	roomID := detail.Room.ID
	_, _ = svc.JoinRoom(ctx, roomID, p2, "", 0)
	_, _ = svc.StartHighLow(ctx, roomID, p1)

	// Inject cards: P1=10 (Card 9), P2=4 (Card 3)
	m1, _ := roomRepo.GetMember(ctx, roomID, p1)
	m1.Card = 9
	_ = roomRepo.UpdateMember(ctx, *m1)

	m2, _ := roomRepo.GetMember(ctx, roomID, p2)
	m2.Card = 3
	_ = roomRepo.UpdateMember(ctx, *m2)

	// Both choose High -> P1 (Card 9) wins full pot (20 coins)!
	_, _ = svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionHigh)
	res, err := svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionHigh)
	if err != nil {
		t.Fatalf("P2 action failed: %v", err)
	}

	if res.Room.WinnerCharacterID == nil || *res.Room.WinnerCharacterID != p1 {
		t.Errorf("expected P1 winner, got %v", res.Room.WinnerCharacterID)
	}

	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	if p1Acc.Coins != 110 {
		t.Errorf("P1 coins = %d, want 110", p1Acc.Coins)
	}

	// Now test all fold:
	_, _ = svc.StartHighLow(ctx, roomID, p1)
	_, _ = svc.PlayHighLowAction(ctx, roomID, p1, casino.HighLowActionFold)
	resFold, err := svc.PlayHighLowAction(ctx, roomID, p2, casino.HighLowActionFold)
	if err != nil {
		t.Fatalf("P2 fold failed: %v", err)
	}
	if resFold.Room.WinnerCharacterID != nil {
		t.Errorf("expected nil winner on all fold, got %v", *resFold.Room.WinnerCharacterID)
	}
}
