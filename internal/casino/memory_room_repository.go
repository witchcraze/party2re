package casino

import (
	"context"
	"sync"
	"time"
)

// MemoryRoomRepository is a thread-safe in-memory room repository for local development and unit tests.
type MemoryRoomRepository struct {
	mu        sync.RWMutex
	rooms     map[string]Room
	members   map[string]map[string]RoomMember
	charRooms map[string]string
	roomLocks map[string]*sync.Mutex
	lockMu    sync.Mutex
}

// NewMemoryRoomRepository creates a new in-memory room repository.
func NewMemoryRoomRepository() *MemoryRoomRepository {
	return &MemoryRoomRepository{
		rooms:     make(map[string]Room),
		members:   make(map[string]map[string]RoomMember),
		charRooms: make(map[string]string),
		roomLocks: make(map[string]*sync.Mutex),
	}
}

func (m *MemoryRoomRepository) CreateRoom(_ context.Context, room Room, leader RoomMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rooms[room.ID] = room
	if m.members[room.ID] == nil {
		m.members[room.ID] = make(map[string]RoomMember)
	}
	m.members[room.ID][leader.CharacterID] = leader
	m.charRooms[leader.CharacterID] = room.ID
	return nil
}

func (m *MemoryRoomRepository) GetRoom(_ context.Context, roomID string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	r, ok := m.rooms[roomID]
	if !ok {
		return nil, ErrRoomNotFound
	}
	cpy := r
	return &cpy, nil
}

func (m *MemoryRoomRepository) GetRoomByName(_ context.Context, name string) (*Room, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, r := range m.rooms {
		if r.Name == name && r.Status != RoomStatusDisbanded {
			cpy := r
			return &cpy, nil
		}
	}
	return nil, ErrRoomNotFound
}

func (m *MemoryRoomRepository) GetRoomForUpdate(ctx context.Context, roomID string) (*Room, error) {
	return m.GetRoom(ctx, roomID)
}

func (m *MemoryRoomRepository) ListActiveRooms(_ context.Context) ([]RoomDetail, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var list []RoomDetail
	for _, r := range m.rooms {
		if r.Status != RoomStatusDisbanded {
			memsMap := m.members[r.ID]
			var mems []RoomMember
			for _, mem := range memsMap {
				mems = append(mems, mem)
			}
			list = append(list, RoomDetail{Room: r, Members: mems})
		}
	}
	return list, nil
}

func (m *MemoryRoomRepository) UpdateRoom(_ context.Context, room Room) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rooms[room.ID] = room
	if room.Status == RoomStatusDisbanded {
		for charID, rID := range m.charRooms {
			if rID == room.ID {
				delete(m.charRooms, charID)
			}
		}
	}
	return nil
}

func (m *MemoryRoomRepository) DeleteRoom(_ context.Context, roomID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if r, ok := m.rooms[roomID]; ok {
		r.Status = RoomStatusDisbanded
		m.rooms[roomID] = r
	}
	delete(m.members, roomID)
	for charID, rID := range m.charRooms {
		if rID == roomID {
			delete(m.charRooms, charID)
		}
	}
	return nil
}

func (m *MemoryRoomRepository) PurgeIdleRooms(_ context.Context, cutoff time.Time) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	count := 0
	for id, r := range m.rooms {
		if r.UpdatedAt.Before(cutoff) {
			delete(m.rooms, id)
			delete(m.members, id)
			for charID, rID := range m.charRooms {
				if rID == id {
					delete(m.charRooms, charID)
				}
			}
			count++
		}
	}
	return count, nil
}

func (m *MemoryRoomRepository) AddMember(_ context.Context, member RoomMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.members[member.RoomID] == nil {
		m.members[member.RoomID] = make(map[string]RoomMember)
	}
	m.members[member.RoomID][member.CharacterID] = member
	m.charRooms[member.CharacterID] = member.RoomID
	return nil
}

func (m *MemoryRoomRepository) GetMember(_ context.Context, roomID string, characterID string) (*RoomMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mems, ok := m.members[roomID]
	if !ok {
		return nil, ErrMemberNotFound
	}
	mem, ok := mems[characterID]
	if !ok {
		return nil, ErrMemberNotFound
	}
	cpy := mem
	return &cpy, nil
}

func (m *MemoryRoomRepository) GetMemberForUpdate(ctx context.Context, roomID string, characterID string) (*RoomMember, error) {
	return m.GetMember(ctx, roomID, characterID)
}

func (m *MemoryRoomRepository) ListMembers(_ context.Context, roomID string) ([]RoomMember, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	mems, ok := m.members[roomID]
	if !ok {
		return nil, nil
	}
	var list []RoomMember
	for _, v := range mems {
		list = append(list, v)
	}
	return list, nil
}

func (m *MemoryRoomRepository) ListMembersForUpdate(ctx context.Context, roomID string) ([]RoomMember, error) {
	return m.ListMembers(ctx, roomID)
}

func (m *MemoryRoomRepository) UpdateMember(_ context.Context, member RoomMember) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.members[member.RoomID] != nil {
		m.members[member.RoomID][member.CharacterID] = member
	}
	return nil
}

func (m *MemoryRoomRepository) RemoveMember(_ context.Context, roomID string, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.members[roomID] != nil {
		delete(m.members[roomID], characterID)
	}
	delete(m.charRooms, characterID)
	return nil
}

func (m *MemoryRoomRepository) GetCharacterRoom(_ context.Context, characterID string) (string, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return m.charRooms[characterID], nil
}

// WithRoomLock executes fn inside an exclusive lock for roomID.
// It supports reentrant calls within the same context.
func (m *MemoryRoomRepository) WithRoomLock(ctx context.Context, roomID string, fn func(ctx context.Context) error) error {
	if roomID == "" {
		return fn(ctx)
	}

	if isRoomLocked(ctx, roomID) {
		return fn(ctx)
	}

	m.lockMu.Lock()
	l, ok := m.roomLocks[roomID]
	if !ok {
		l = &sync.Mutex{}
		m.roomLocks[roomID] = l
	}
	m.lockMu.Unlock()

	l.Lock()
	defer l.Unlock()

	lockedCtx := withRoomLockedContext(ctx, roomID)
	return fn(lockedCtx)
}
