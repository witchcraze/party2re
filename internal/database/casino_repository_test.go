package database

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
)

func TestCasinoRepositoryLifecycle(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	casinoRepo, err := NewCasinoRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Create test character with 10,000 gold
	char, err := CreateTestCharacter(ctx, db, "CasinoPlayer")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 10000, char.ID); err != nil {
		t.Fatal(err)
	}

	// 2. Initial casino account should be 0 coins
	initialAcc, err := casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if initialAcc.Coins != 0 {
		t.Errorf("initial coins = %d, want 0", initialAcc.Coins)
	}

	// 3. Credit 100 coins via AdjustCoins
	acc, err := casinoRepo.AdjustCoins(ctx, char.ID, 100)
	if err != nil {
		t.Fatalf("AdjustCoins failed: %v", err)
	}
	if acc.Coins != 100 {
		t.Errorf("coins = %d, want 100", acc.Coins)
	}

	// 4. Deduct 30 coins via AdjustCoins (remaining 70 coins)
	acc, err = casinoRepo.AdjustCoins(ctx, char.ID, -30)
	if err != nil {
		t.Fatalf("AdjustCoins failed: %v", err)
	}
	if acc.Coins != 70 {
		t.Errorf("coins = %d, want 70", acc.Coins)
	}

	// 5. Insufficient coins to deduct should fail
	if _, err := casinoRepo.AdjustCoins(ctx, char.ID, -500); !errors.Is(err, casino.ErrInsufficientCoins) {
		t.Errorf("insufficient coins err = %v, want %v", err, casino.ErrInsufficientCoins)
	}

	// 7. Adjust coins (deduct 20 -> 50 remaining)
	acc, err = casinoRepo.AdjustCoins(ctx, char.ID, -20)
	if err != nil {
		t.Fatalf("AdjustCoins deduct failed: %v", err)
	}
	if acc.Coins != 50 {
		t.Errorf("coins = %d, want 50", acc.Coins)
	}

	// 8. Adjust coins (add 150 -> 200 total)
	acc, err = casinoRepo.AdjustCoins(ctx, char.ID, 150)
	if err != nil {
		t.Fatalf("AdjustCoins add failed: %v", err)
	}
	if acc.Coins != 200 {
		t.Errorf("coins = %d, want 200", acc.Coins)
	}

	// 9. DeductBetAndCreditPayout: bet 50, payout 100 -> net +50 (250 total)
	acc, err = casinoRepo.DeductBetAndCreditPayout(ctx, char.ID, 50, 100)
	if err != nil {
		t.Fatalf("DeductBetAndCreditPayout failed: %v", err)
	}
	if acc.Coins != 250 {
		t.Errorf("coins = %d, want 250", acc.Coins)
	}

	// 10. DeductBetAndCreditPayout: bet 300 (exceeds balance 250), payout 1000 -> must fail with ErrInsufficientCoins
	if _, err := casinoRepo.DeductBetAndCreditPayout(ctx, char.ID, 300, 1000); !errors.Is(err, casino.ErrInsufficientCoins) {
		t.Errorf("expected ErrInsufficientCoins, got %v", err)
	}

	// Balance should remain unchanged at 250
	checkAcc, err := casinoRepo.GetAccount(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetAccount failed: %v", err)
	}
	if checkAcc.Coins != 250 {
		t.Errorf("balance after failed bet = %d, want 250", checkAcc.Coins)
	}
}

func TestCasinoRoomRepository_Lifecycle(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	roomRepo, err := NewCasinoRoomRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	char, err := CreateTestCharacter(ctx, db, "RoomRepoUser")
	if err != nil {
		t.Fatal(err)
	}

	roomID := fmt.Sprintf("test_room_%d", time.Now().UnixNano())
	roomName := fmt.Sprintf("Room-%d", time.Now().UnixNano()%1000000000)
	now := time.Now().UTC()

	r := casino.Room{
		ID:                roomID,
		Name:              roomName,
		GameType:          casino.GameTypeIndian,
		LeaderCharacterID: char.ID,
		Speed:             12,
		MaxPlayers:        4,
		Rate:              10,
		Status:            casino.RoomStatusWaiting,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	m := casino.RoomMember{
		RoomID:      roomID,
		CharacterID: char.ID,
		IsSpectator: false,
		Action:      "待機中",
		Card:        -1,
		JoinedAt:    now,
		UpdatedAt:   now,
	}

	// 1. Create room
	if err := roomRepo.CreateRoom(ctx, r, m); err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	// 2. GetRoom & GetRoomByName
	gotRoom, err := roomRepo.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}
	if gotRoom.Name != roomName || gotRoom.LeaderCharacterID != char.ID {
		t.Errorf("unexpected room: %+v", gotRoom)
	}

	gotByName, err := roomRepo.GetRoomByName(ctx, roomName)
	if err != nil || gotByName.ID != roomID {
		t.Errorf("GetRoomByName failed: %v, %+v", err, gotByName)
	}

	// 3. ListActiveRooms
	activeList, err := roomRepo.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("ListActiveRooms failed: %v", err)
	}
	found := false
	for _, d := range activeList {
		if d.Room.ID == roomID {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected room %s in active list", roomID)
	}

	// 4. UpdateRoom
	gotRoom.Round = 1
	gotRoom.Pot = 20
	if err := roomRepo.UpdateRoom(ctx, *gotRoom); err != nil {
		t.Fatalf("UpdateRoom failed: %v", err)
	}

	// 5. Members: ListMembers & UpdateMember
	members, err := roomRepo.ListMembers(ctx, roomID)
	if err != nil || len(members) != 1 {
		t.Fatalf("ListMembers failed: %v (count=%d)", err, len(members))
	}
	mem := members[0]
	mem.Action = "しょうぶ"
	mem.Card = 5
	if err := roomRepo.UpdateMember(ctx, mem); err != nil {
		t.Fatalf("UpdateMember failed: %v", err)
	}

	gotMem, err := roomRepo.GetMember(ctx, roomID, char.ID)
	if err != nil || gotMem.Card != 5 || gotMem.Action != "しょうぶ" {
		t.Errorf("unexpected updated member: %v, %+v", err, gotMem)
	}

	// 6. RemoveMember & DeleteRoom
	if err := roomRepo.RemoveMember(ctx, roomID, char.ID); err != nil {
		t.Fatalf("RemoveMember failed: %v", err)
	}
	if err := roomRepo.DeleteRoom(ctx, roomID); err != nil {
		t.Fatalf("DeleteRoom failed: %v", err)
	}

	_, err = roomRepo.GetRoom(ctx, roomID)
	if !errors.Is(err, casino.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound after deletion, got %v", err)
	}
}
