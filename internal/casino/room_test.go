package casino_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
)

type mockMemoryRoomRepo struct {
	rooms   map[string]casino.Room
	members map[string]map[string]casino.RoomMember
}

func newMockMemoryRoomRepo() *mockMemoryRoomRepo {
	return &mockMemoryRoomRepo{
		rooms:   make(map[string]casino.Room),
		members: make(map[string]map[string]casino.RoomMember),
	}
}

func (m *mockMemoryRoomRepo) CreateRoom(ctx context.Context, room casino.Room, leader casino.RoomMember) error {
	m.rooms[room.ID] = room
	if m.members[room.ID] == nil {
		m.members[room.ID] = make(map[string]casino.RoomMember)
	}
	m.members[room.ID][leader.CharacterID] = leader
	return nil
}

func (m *mockMemoryRoomRepo) GetRoom(ctx context.Context, roomID string) (*casino.Room, error) {
	r, ok := m.rooms[roomID]
	if !ok {
		return nil, casino.ErrRoomNotFound
	}
	cpy := r
	return &cpy, nil
}

func (m *mockMemoryRoomRepo) GetRoomByName(ctx context.Context, name string) (*casino.Room, error) {
	for _, r := range m.rooms {
		if r.Name == name {
			cpy := r
			return &cpy, nil
		}
	}
	return nil, casino.ErrRoomNotFound
}

func (m *mockMemoryRoomRepo) GetRoomForUpdate(ctx context.Context, roomID string) (*casino.Room, error) {
	return m.GetRoom(ctx, roomID)
}

func (m *mockMemoryRoomRepo) ListActiveRooms(ctx context.Context) ([]casino.RoomDetail, error) {
	var list []casino.RoomDetail
	for _, r := range m.rooms {
		if r.Status != casino.RoomStatusDisbanded {
			mems, _ := m.ListMembers(ctx, r.ID)
			list = append(list, casino.RoomDetail{Room: r, Members: mems})
		}
	}
	return list, nil
}

func (m *mockMemoryRoomRepo) UpdateRoom(ctx context.Context, room casino.Room) error {
	m.rooms[room.ID] = room
	return nil
}

func (m *mockMemoryRoomRepo) AddMember(ctx context.Context, member casino.RoomMember) error {
	if m.members[member.RoomID] == nil {
		m.members[member.RoomID] = make(map[string]casino.RoomMember)
	}
	m.members[member.RoomID][member.CharacterID] = member
	return nil
}

func (m *mockMemoryRoomRepo) GetMember(ctx context.Context, roomID string, characterID string) (*casino.RoomMember, error) {
	mems, ok := m.members[roomID]
	if !ok {
		return nil, casino.ErrMemberNotFound
	}
	mem, ok := mems[characterID]
	if !ok {
		return nil, casino.ErrMemberNotFound
	}
	cpy := mem
	return &cpy, nil
}

func (m *mockMemoryRoomRepo) GetMemberForUpdate(ctx context.Context, roomID string, characterID string) (*casino.RoomMember, error) {
	return m.GetMember(ctx, roomID, characterID)
}

func (m *mockMemoryRoomRepo) ListMembers(ctx context.Context, roomID string) ([]casino.RoomMember, error) {
	mems, ok := m.members[roomID]
	if !ok {
		return nil, nil
	}
	var list []casino.RoomMember
	for _, v := range mems {
		list = append(list, v)
	}
	return list, nil
}

func (m *mockMemoryRoomRepo) ListMembersForUpdate(ctx context.Context, roomID string) ([]casino.RoomMember, error) {
	return m.ListMembers(ctx, roomID)
}

func (m *mockMemoryRoomRepo) UpdateMember(ctx context.Context, member casino.RoomMember) error {
	if m.members[member.RoomID] != nil {
		m.members[member.RoomID][member.CharacterID] = member
	}
	return nil
}

func (m *mockMemoryRoomRepo) RemoveMember(ctx context.Context, roomID string, characterID string) error {
	if m.members[roomID] != nil {
		delete(m.members[roomID], characterID)
	}
	return nil
}

func (m *mockMemoryRoomRepo) DeleteRoom(ctx context.Context, roomID string) error {
	if r, ok := m.rooms[roomID]; ok {
		r.Status = casino.RoomStatusDisbanded
		m.rooms[roomID] = r
	}
	return nil
}

func TestCasinoRoomLobby(t *testing.T) {
	ctx := context.Background()
	casinoRepo := newMockPrizeCasinoRepo()
	roomRepo := newMockMemoryRoomRepo()

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
