package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestParseDoppelMark(t *testing.T) {
	tests := []struct {
		input   string
		want    int
		wantErr bool
	}{
		{"★", 0, false},
		{"●", 1, false},
		{"◆", 2, false},
		{"♪", 3, false},
		{"■", 4, false},
		{"▲", 5, false},
		{"†", 6, false},
		{"▼", 7, false},
		{"0", 0, false},
		{"1", 1, false},
		{"7", 7, false},
		{"8", -1, true},
		{"invalid", -1, true},
		{"", -1, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := casino.ParseDoppelMark(tt.input)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDoppelMark(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("ParseDoppelMark(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestMultiplayerDoppel_ChildrenWin_LeaderTransfer(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	p1 := "dp-p1"
	p2 := "dp-p2"
	p3 := "dp-p3"

	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}
	casinoRepo.accounts[p3] = casino.Account{CharacterID: p3, Coins: 500}

	// 1. P1 creates Doppel room (Rate = 20)
	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:            "DoppelLobby",
		GameType:        casino.GameTypeDoppel,
		Speed:           casino.SpeedFast,
		MaxPlayers:      4,
		Rate:            20,
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
	if _, err := svc.StartDoppel(ctx, roomID, p2); !errors.Is(err, casino.ErrNotLeader) {
		t.Errorf("expected ErrNotLeader, got %v", err)
	}

	// 3. P1 (leader) starts the game
	startedDetail, err := svc.StartDoppel(ctx, roomID, p1)
	if err != nil {
		t.Fatalf("StartDoppel failed: %v", err)
	}
	if startedDetail.Room.Round != 1 || startedDetail.Room.Status != casino.RoomStatusInProgress {
		t.Fatalf("unexpected room state: round=%d status=%s", startedDetail.Room.Round, startedDetail.Room.Status)
	}

	// 4. P1 chooses mark index 0 ("★"). Coins deducted: 20, Pot = 20.
	if _, err := svc.PlayDoppelAction(ctx, roomID, p1, 0); err != nil {
		t.Fatalf("P1 action failed: %v", err)
	}
	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	if p1Acc.Coins != 480 {
		t.Errorf("expected P1 coins 480 after initial mark select, got %d", p1Acc.Coins)
	}

	// Masking check: P1 sees own mark "★", other members see "？" and action "？？？"
	p2View, _ := svc.GetRoomDetail(ctx, roomID, p2)
	for _, m := range p2View.Members {
		if m.CharacterID == p1 {
			if m.CardDisplay != "？" || m.Card != -1 {
				t.Errorf("P2 should see P1 card masked as '？', got %s (%d)", m.CardDisplay, m.Card)
			}
			if m.Action != "？？？" {
				t.Errorf("P2 should see P1 action masked as '？？？', got %s", m.Action)
			}
		}
	}

	// P1 changes mark to index 2 ("◆"): Changing mark does NOT deduct coins again! (party2/lib/casino_doppel.cgi:46-53)
	if _, err := svc.PlayDoppelAction(ctx, roomID, p1, 2); err != nil {
		t.Fatalf("P1 change mark failed: %v", err)
	}
	p1Acc, _ = casinoRepo.GetAccount(ctx, p1)
	if p1Acc.Coins != 480 {
		t.Errorf("expected P1 coins to remain 480 on card change, got %d", p1Acc.Coins)
	}

	// 5. P2 chooses mark index 2 ("◆"): matches leader's mark!
	if _, err := svc.PlayDoppelAction(ctx, roomID, p2, 2); err != nil {
		t.Fatalf("P2 action failed: %v", err)
	}

	// 6. P3 chooses mark index 1 ("●"): does NOT match leader's mark.
	// All 3 participants have now chosen a mark -> Showdown triggers!
	finalDetail, err := svc.PlayDoppelAction(ctx, roomID, p3, 1)
	if err != nil {
		t.Fatalf("P3 action failed: %v", err)
	}

	// 7. Verify Showdown Outcome:
	// Pot = 20 * 3 = 60 coins.
	// Leader P1 chose "◆". Matching child: P2 ("◆").
	// P2 wins entire 60 coin pot!
	// Leadership transfers to P2!
	if finalDetail.Room.Round != 0 {
		t.Errorf("expected round reset to 0, got %d", finalDetail.Room.Round)
	}
	if finalDetail.Room.WinnerCharacterID == nil || *finalDetail.Room.WinnerCharacterID != p2 {
		t.Errorf("expected winner P2, got %v", finalDetail.Room.WinnerCharacterID)
	}
	if finalDetail.Room.LeaderCharacterID != p2 {
		t.Errorf("expected new leader P2, got %s", finalDetail.Room.LeaderCharacterID)
	}

	p2Acc, _ := casinoRepo.GetAccount(ctx, p2)
	p3Acc, _ := casinoRepo.GetAccount(ctx, p3)

	// P2: 500 - 20 + 60 = 540 coins!
	if p2Acc.Coins != 540 {
		t.Errorf("P2 coins = %d, want 540", p2Acc.Coins)
	}
	// P3: 500 - 20 = 480 coins
	if p3Acc.Coins != 480 {
		t.Errorf("P3 coins = %d, want 480", p3Acc.Coins)
	}
}

func TestMultiplayerDoppel_ParentWins_LeaderKept(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	p1 := "dp-lead"
	p2 := "dp-c1"
	p3 := "dp-c2"

	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 100}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 100}
	casinoRepo.accounts[p3] = casino.Account{CharacterID: p3, Coins: 100}

	detail, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "ParentWinLobby",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 3,
		Rate:       10,
	})
	roomID := detail.Room.ID
	_, _ = svc.JoinRoom(ctx, roomID, p2, "", 0)
	_, _ = svc.JoinRoom(ctx, roomID, p3, "", 0)
	_, _ = svc.StartDoppel(ctx, roomID, p1)

	// P1 chooses "★" (0)
	_, _ = svc.PlayDoppelAction(ctx, roomID, p1, 0)
	// P2 chooses "●" (1)
	_, _ = svc.PlayDoppelAction(ctx, roomID, p2, 1)
	// P3 chooses "◆" (2) -> Showdown triggers!
	res, err := svc.PlayDoppelAction(ctx, roomID, p3, 2)
	if err != nil {
		t.Fatalf("P3 action failed: %v", err)
	}

	// No children matched leader's mark (0)
	// Leader P1 wins entire pot (30 coins)! Leader remains P1.
	if res.Room.WinnerCharacterID == nil || *res.Room.WinnerCharacterID != p1 {
		t.Errorf("expected leader P1 to win, got %v", res.Room.WinnerCharacterID)
	}
	if res.Room.LeaderCharacterID != p1 {
		t.Errorf("expected leader to remain P1, got %s", res.Room.LeaderCharacterID)
	}

	p1Acc, _ := casinoRepo.GetAccount(ctx, p1)
	// P1: 100 - 10 + 30 = 120 coins!
	if p1Acc.Coins != 120 {
		t.Errorf("P1 coins = %d, want 120", p1Acc.Coins)
	}
}

func TestMultiplayerDoppel_MarkOutOfRange(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	p1 := "dp-v1"
	p2 := "dp-v2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 100}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 100}

	detail, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "RangeCheckLobby",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	roomID := detail.Room.ID
	_, _ = svc.JoinRoom(ctx, roomID, p2, "", 0)
	_, _ = svc.StartDoppel(ctx, roomID, p1)

	// For party of 2, available marks are 0..min(2, 7) = 0..2 (indices 0, 1, 2)
	// Choosing index 3 should return ErrInvalidDoppelMarkIndex
	_, err = svc.PlayDoppelAction(ctx, roomID, p1, 3)
	if !errors.Is(err, casino.ErrInvalidDoppelMarkIndex) {
		t.Errorf("expected ErrInvalidDoppelMarkIndex, got %v", err)
	}
}
