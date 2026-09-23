package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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

func TestRoom_LeaveRoomFatigue(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := casino.NewMemoryRoomRepository()
	charRepo := &inMemoryCharRepo{chars: make(map[string]corecharacter.Character)}

	p1 := "part-1"
	p2 := "part-2"
	spec := "spec-1"

	charRepo.chars[p1] = corecharacter.Character{ID: p1, Name: "P1", Tired: 10}
	charRepo.chars[p2] = corecharacter.Character{ID: p2, Name: "P2", Tired: 20}
	charRepo.chars[spec] = corecharacter.Character{ID: spec, Name: "Spec", Tired: 0}

	casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
	casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}
	casinoRepo.accounts[spec] = casino.Account{CharacterID: spec, Coins: 500}

	svc, err := casino.NewService(
		casinoRepo,
		casino.WithRoomRepository(roomRepo),
		casino.WithCharacterRepository(charRepo),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
		Name:            "FatigueRoom",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedFast,
		MaxPlayers:      4,
		Rate:            10,
		AllowSpectators: true,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}
	roomID := detail.Room.ID

	// P2 joins as participant
	if _, err := svc.JoinRoom(ctx, roomID, p2, "", 0); err != nil {
		t.Fatalf("P2 JoinRoom failed: %v", err)
	}

	// Spectator joins as spectator
	if _, err := svc.SpectateRoom(ctx, roomID, spec, ""); err != nil {
		t.Fatalf("Spec SpectateRoom failed: %v", err)
	}

	// Start game
	if _, err := svc.StartGame(ctx, roomID, p1); err != nil {
		t.Fatalf("StartGame failed: %v", err)
	}

	// 1. Spectator leaves active room -> fatigue must remain unchanged
	if err := svc.LeaveRoom(ctx, roomID, spec); err != nil {
		t.Fatalf("Spec LeaveRoom failed: %v", err)
	}
	specChar, _ := charRepo.FindByID(ctx, spec)
	if specChar.Tired != 0 {
		t.Errorf("expected spectator Tired=0, got %d", specChar.Tired)
	}

	// 2. Participant P2 leaves active room -> fatigue must increment by 1
	if err := svc.LeaveRoom(ctx, roomID, p2); err != nil {
		t.Fatalf("P2 LeaveRoom failed: %v", err)
	}
	p2Char, _ := charRepo.FindByID(ctx, p2)
	if p2Char.Tired != 21 {
		t.Errorf("expected participant P2 Tired=21, got %d", p2Char.Tired)
	}
}

func TestRoom_ShowdownIncrementsCasinoWins(t *testing.T) {
	ctx := context.Background()

	t.Run("Indian Poker showdown increments winner CasinoWins", func(t *testing.T) {
		casinoRepo := newMockPrizeCasinoRepo()
		roomRepo := casino.NewMemoryRoomRepository()
		charRepo := &inMemoryCharRepo{chars: make(map[string]corecharacter.Character)}

		p1 := "ip-p1"
		p2 := "ip-p2"
		charRepo.chars[p1] = corecharacter.Character{ID: p1, Name: "P1", CasinoWins: 0}
		charRepo.chars[p2] = corecharacter.Character{ID: p2, Name: "P2", CasinoWins: 3}
		casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
		casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}

		svc, _ := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(charRepo))
		room, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
			Name: "IPWins", GameType: casino.GameTypeIndian, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
		})
		_, _ = svc.JoinRoom(ctx, room.Room.ID, p2, "", 0)
		_, _ = svc.StartGame(ctx, room.Room.ID, p1)

		// Set cards so P2 wins (P1=2, P2=12)
		m1, _ := roomRepo.GetMember(ctx, room.Room.ID, p1)
		m1.Card = 2
		_ = roomRepo.UpdateMember(ctx, *m1)
		m2, _ := roomRepo.GetMember(ctx, room.Room.ID, p2)
		m2.Card = 12
		_ = roomRepo.UpdateMember(ctx, *m2)

		_, _ = svc.PlayIndianPokerAction(ctx, room.Room.ID, p1, casino.ActionShowdown)
		_, err := svc.PlayIndianPokerAction(ctx, room.Room.ID, p2, casino.ActionShowdown)
		if err != nil {
			t.Fatalf("Showdown failed: %v", err)
		}

		c1, _ := charRepo.FindByID(ctx, p1)
		c2, _ := charRepo.FindByID(ctx, p2)
		if c1.CasinoWins != 0 {
			t.Errorf("expected loser P1 CasinoWins=0, got %d", c1.CasinoWins)
		}
		if c2.CasinoWins != 4 {
			t.Errorf("expected winner P2 CasinoWins=4 (was 3), got %d", c2.CasinoWins)
		}
	})

	t.Run("High & Low showdown increments split winners CasinoWins", func(t *testing.T) {
		casinoRepo := newMockPrizeCasinoRepo()
		roomRepo := casino.NewMemoryRoomRepository()
		charRepo := &inMemoryCharRepo{chars: make(map[string]corecharacter.Character)}

		p1 := "hl-p1"
		p2 := "hl-p2"
		p3 := "hl-p3"
		charRepo.chars[p1] = corecharacter.Character{ID: p1, Name: "P1", CasinoWins: 0}
		charRepo.chars[p2] = corecharacter.Character{ID: p2, Name: "P2", CasinoWins: 5}
		charRepo.chars[p3] = corecharacter.Character{ID: p3, Name: "P3", CasinoWins: 2}
		casinoRepo.accounts[p1] = casino.Account{CharacterID: p1, Coins: 500}
		casinoRepo.accounts[p2] = casino.Account{CharacterID: p2, Coins: 500}
		casinoRepo.accounts[p3] = casino.Account{CharacterID: p3, Coins: 500}

		svc, _ := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(charRepo))
		room, _ := svc.CreateRoom(ctx, p1, casino.CreateRoomRequest{
			Name: "HLWins", GameType: casino.GameTypeHighLow, Speed: casino.SpeedFast, MaxPlayers: 3, Rate: 10,
		})
		_, _ = svc.JoinRoom(ctx, room.Room.ID, p2, "", 0)
		_, _ = svc.JoinRoom(ctx, room.Room.ID, p3, "", 0)
		_, _ = svc.StartGame(ctx, room.Room.ID, p1)

		// P1 gets high card 12, P2 gets low card 0, P3 gets middle card 5
		m1, _ := roomRepo.GetMember(ctx, room.Room.ID, p1)
		m1.Card = 12
		_ = roomRepo.UpdateMember(ctx, *m1)
		m2, _ := roomRepo.GetMember(ctx, room.Room.ID, p2)
		m2.Card = 0
		_ = roomRepo.UpdateMember(ctx, *m2)
		m3, _ := roomRepo.GetMember(ctx, room.Room.ID, p3)
		m3.Card = 5
		_ = roomRepo.UpdateMember(ctx, *m3)

		_, _ = svc.PlayHighLowAction(ctx, room.Room.ID, p1, casino.HighLowActionHigh)
		_, _ = svc.PlayHighLowAction(ctx, room.Room.ID, p2, casino.HighLowActionLow)
		_, err := svc.PlayHighLowAction(ctx, room.Room.ID, p3, casino.HighLowActionFold)
		if err != nil {
			t.Fatalf("PlayHighLowAction failed: %v", err)
		}

		c1, _ := charRepo.FindByID(ctx, p1)
		c2, _ := charRepo.FindByID(ctx, p2)
		c3, _ := charRepo.FindByID(ctx, p3)
		if c1.CasinoWins != 1 {
			t.Errorf("expected high winner P1 CasinoWins=1, got %d", c1.CasinoWins)
		}
		if c2.CasinoWins != 6 {
			t.Errorf("expected low winner P2 CasinoWins=6 (was 5), got %d", c2.CasinoWins)
		}
		if c3.CasinoWins != 2 {
			t.Errorf("expected folded P3 CasinoWins=2, got %d", c3.CasinoWins)
		}
	})

	t.Run("Doppelganger showdown increments winner CasinoWins", func(t *testing.T) {
		casinoRepo := newMockPrizeCasinoRepo()
		roomRepo := casino.NewMemoryRoomRepository()
		charRepo := &inMemoryCharRepo{chars: make(map[string]corecharacter.Character)}

		leader := "dp-leader"
		child := "dp-child"
		charRepo.chars[leader] = corecharacter.Character{ID: leader, Name: "Leader", CasinoWins: 1}
		charRepo.chars[child] = corecharacter.Character{ID: child, Name: "Child", CasinoWins: 0}
		casinoRepo.accounts[leader] = casino.Account{CharacterID: leader, Coins: 500}
		casinoRepo.accounts[child] = casino.Account{CharacterID: child, Coins: 500}

		svc, _ := casino.NewService(casinoRepo, casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(charRepo))
		room, _ := svc.CreateRoom(ctx, leader, casino.CreateRoomRequest{
			Name: "DPWins", GameType: casino.GameTypeDoppel, Speed: casino.SpeedFast, MaxPlayers: 2, Rate: 10,
		})
		_, _ = svc.JoinRoom(ctx, room.Room.ID, child, "", 0)
		_, _ = svc.StartGame(ctx, room.Room.ID, leader)

		// Leader chooses mark 0, child chooses mark 0 -> Child matches and wins!
		_, _ = svc.PlayDoppelAction(ctx, room.Room.ID, leader, 0)
		_, err := svc.PlayDoppelAction(ctx, room.Room.ID, child, 0)
		if err != nil {
			t.Fatalf("PlayDoppelAction failed: %v", err)
		}

		cLeader, _ := charRepo.FindByID(ctx, leader)
		cChild, _ := charRepo.FindByID(ctx, child)
		if cLeader.CasinoWins != 1 {
			t.Errorf("expected losing leader CasinoWins=1, got %d", cLeader.CasinoWins)
		}
		if cChild.CasinoWins != 1 {
			t.Errorf("expected winning child CasinoWins=1, got %d", cChild.CasinoWins)
		}
	})
}
