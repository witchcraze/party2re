package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

func newMockMemoryRoomRepo() *casino.MemoryRoomRepository {
	return casino.NewMemoryRoomRepository()
}

func TestCasinoRoomLobby(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := casino.NewMemoryRoomRepository()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	charLeader := "char-leader"
	casinoRepo.accounts[charLeader] = casino.Account{CharacterID: charLeader, Coins: 500}

	// 1. Create room validation errors
	t.Run("Room Name Validation", func(t *testing.T) {
		_, err := svc.CreateRoom(ctx, charLeader, casino.CreateRoomRequest{
			Name:       "Bad Room Name", // space is illegal
			GameType:   casino.GameTypeIndian,
			Speed:      casino.SpeedNormal,
			MaxPlayers: 4,
			Rate:       10,
		})
		if !errors.Is(err, casino.ErrInvalidRoomName) {
			t.Errorf("expected ErrInvalidRoomName on spaces, got %v", err)
		}
	})

	t.Run("Insufficient Coins for Creator (needs rate * 5)", func(t *testing.T) {
		casinoRepo.accounts["char-poor"] = casino.Account{CharacterID: "char-poor", Coins: 40}
		_, err := svc.CreateRoom(ctx, "char-poor", casino.CreateRoomRequest{
			Name:       "PoorRoom",
			GameType:   casino.GameTypeIndian,
			Speed:      casino.SpeedNormal,
			MaxPlayers: 4,
			Rate:       10, // needs 50 coins
		})
		if !errors.Is(err, casino.ErrInsufficientCoinsForRate) {
			t.Errorf("expected ErrInsufficientCoinsForRate, got %v", err)
		}
	})

	// 2. Successful creation
	var createdRoomID string
	t.Run("Create Room Success", func(t *testing.T) {
		detail, err := svc.CreateRoom(ctx, charLeader, casino.CreateRoomRequest{
			Name:            "PokerBattle",
			GameType:        casino.GameTypeIndian,
			Speed:           casino.SpeedNormal,
			MaxPlayers:      4,
			Rate:            10,
			Password:        "secret",
			AllowSpectators: true,
		})
		if err != nil {
			t.Fatalf("CreateRoom error: %v", err)
		}
		if detail.Room.LeaderCharacterID != charLeader {
			t.Errorf("expected leader %s, got %s", charLeader, detail.Room.LeaderCharacterID)
		}
		if len(detail.Members) != 1 || detail.Members[0].CharacterID != charLeader {
			t.Errorf("expected leader in members list, got %+v", detail.Members)
		}
		createdRoomID = detail.Room.ID
	})

	// 3. Duplicate name
	t.Run("Duplicate Room Name Rejection", func(t *testing.T) {
		_, err := svc.CreateRoom(ctx, charLeader, casino.CreateRoomRequest{
			Name:       "PokerBattle",
			GameType:   casino.GameTypeIndian,
			Speed:      casino.SpeedNormal,
			MaxPlayers: 4,
			Rate:       10,
		})
		if !errors.Is(err, casino.ErrRoomNameTaken) {
			t.Errorf("expected ErrRoomNameTaken, got %v", err)
		}
	})

	// 4. Join room
	charP2 := "char-p2"
	casinoRepo.accounts[charP2] = casino.Account{CharacterID: charP2, Coins: 100}

	t.Run("Join Room with Wrong Password", func(t *testing.T) {
		_, err := svc.JoinRoom(ctx, createdRoomID, charP2, "wrong_pass", 0)
		if !errors.Is(err, casino.ErrInvalidPassword) {
			t.Errorf("expected ErrInvalidPassword, got %v", err)
		}
	})

	t.Run("Join Room Exhausted Character Rejected", func(t *testing.T) {
		_, err := svc.JoinRoom(ctx, createdRoomID, charP2, "secret", 100)
		if !errors.Is(err, casino.ErrCharacterExhausted) {
			t.Errorf("expected ErrCharacterExhausted, got %v", err)
		}
	})

	t.Run("Join Room Success", func(t *testing.T) {
		detail, err := svc.JoinRoom(ctx, createdRoomID, charP2, "secret", 50)
		if err != nil {
			t.Fatalf("JoinRoom error: %v", err)
		}
		if len(detail.Members) != 2 {
			t.Errorf("expected 2 members, got %d", len(detail.Members))
		}
	})

	t.Run("Cannot Join Twice", func(t *testing.T) {
		_, err := svc.JoinRoom(ctx, createdRoomID, charP2, "secret", 50)
		if !errors.Is(err, casino.ErrAlreadyInRoom) {
			t.Errorf("expected ErrAlreadyInRoom, got %v", err)
		}
	})

	// 5. Spectating
	charSpectator := "char-spec"
	t.Run("Spectate Room", func(t *testing.T) {
		detail, err := svc.SpectateRoom(ctx, createdRoomID, charSpectator, "secret")
		if err != nil {
			t.Fatalf("SpectateRoom error: %v", err)
		}
		found := false
		for _, m := range detail.Members {
			if m.CharacterID == charSpectator && m.IsSpectator {
				found = true
			}
		}
		if !found {
			t.Errorf("expected spectator member in list, got %+v", detail.Members)
		}
	})

	// 6. Kick
	t.Run("Non-leader cannot kick", func(t *testing.T) {
		err := svc.KickMember(ctx, createdRoomID, charP2, charSpectator)
		if !errors.Is(err, casino.ErrNotLeader) {
			t.Errorf("expected ErrNotLeader, got %v", err)
		}
	})

	t.Run("Leader cannot kick self", func(t *testing.T) {
		err := svc.KickMember(ctx, createdRoomID, charLeader, charLeader)
		if !errors.Is(err, casino.ErrCannotKickSelf) {
			t.Errorf("expected ErrCannotKickSelf, got %v", err)
		}
	})

	t.Run("Leader kicks member", func(t *testing.T) {
		err := svc.KickMember(ctx, createdRoomID, charLeader, charSpectator)
		if err != nil {
			t.Fatalf("KickMember error: %v", err)
		}
		m, err := roomRepo.GetMember(ctx, createdRoomID, charSpectator)
		if err == nil && m != nil {
			t.Errorf("expected kicked member to be gone")
		}
	})

	// 7. Leave room and leader transfer
	t.Run("Leader leaves room -> transfers leader", func(t *testing.T) {
		err := svc.LeaveRoom(ctx, createdRoomID, charLeader)
		if err != nil {
			t.Fatalf("LeaveRoom error: %v", err)
		}
		r, err := roomRepo.GetRoom(ctx, createdRoomID)
		if err != nil {
			t.Fatalf("GetRoom error: %v", err)
		}
		if r.LeaderCharacterID != charP2 {
			t.Errorf("expected leader to transfer to charP2, got %s", r.LeaderCharacterID)
		}
	})

	t.Run("Last member leaves room -> disbands room", func(t *testing.T) {
		err := svc.LeaveRoom(ctx, createdRoomID, charP2)
		if err != nil {
			t.Fatalf("LeaveRoom error: %v", err)
		}
		r, err := roomRepo.GetRoom(ctx, createdRoomID)
		if err != nil {
			t.Fatalf("GetRoom error: %v", err)
		}
		if r.Status != casino.RoomStatusDisbanded {
			t.Errorf("expected status disbanded, got %s", r.Status)
		}
	})
}

func TestRoom_UnifiedStartAndActionDispatch(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo))
	if err != nil {
		t.Fatal(err)
	}

	p1 := "u-p1"
	p2 := "u-p2"
	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

	// 1. High-Low via unified StartGame & PlayRoomAction
	hlRoom, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "UnifiedHL",
		GameType:   casino.GameTypeHighLow,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       10,
	})
	_, _ = svc.JoinRoom(ctx, hlRoom.Room.ID, p2, "", 0)

	startedHL, err := svc.StartGame(ctx, hlRoom.Room.ID, p1)
	if err != nil {
		t.Fatalf("StartGame(HL) failed: %v", err)
	}
	if startedHL.Room.Round != 1 {
		t.Errorf("expected round 1, got %d", startedHL.Room.Round)
	}

	_, err = svc.PlayRoomAction(ctx, hlRoom.Room.ID, p1, casino.RoomActionRequest{Action: "high"})
	if err != nil {
		t.Fatalf("PlayRoomAction(HL high) failed: %v", err)
	}

	// 2. Doppel via unified StartGame & PlayRoomAction
	dpRoom, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:       "UnifiedDP",
		GameType:   casino.GameTypeDoppel,
		Speed:      casino.SpeedFast,
		MaxPlayers: 2,
		Rate:       20,
	})
	_, _ = svc.JoinRoom(ctx, dpRoom.Room.ID, p2, "", 0)

	startedDP, err := svc.StartGame(ctx, dpRoom.Room.ID, p1)
	if err != nil {
		t.Fatalf("StartGame(DP) failed: %v", err)
	}
	if startedDP.Room.Round != 1 {
		t.Errorf("expected round 1, got %d", startedDP.Room.Round)
	}

	_, err = svc.PlayRoomAction(ctx, dpRoom.Room.ID, p1, casino.RoomActionRequest{Action: "mark", Mark: "★"})
	if err != nil {
		t.Fatalf("PlayRoomAction(DP mark) failed: %v", err)
	}
}
