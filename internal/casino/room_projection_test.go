package casino_test

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestRoomProjections(t *testing.T) {
	ctx := context.Background()
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow, casino.GameTypeDoppel} {
		for _, phaseCase := range []struct {
			name  string
			round int
		}{{"waiting", 0}, {"started", 1}, {"round", 2}, {"finished", 0}} {
			phase := phaseCase.round
			for _, viewer := range []string{"self", "other", "spectator", "outsider", ""} {
				t.Run(string(game)+"/"+phaseCase.name+"/"+viewer, func(t *testing.T) {
					repo := casino.NewMemoryRoomRepository()
					room := casino.Room{ID: "room", Name: "room", GameType: game, Round: phase, Status: casino.RoomStatusWaiting, UpdatedAt: time.Now(), PasswordHash: "secret", HasPassword: true}
					if phase > 0 {
						room.Status = casino.RoomStatusInProgress
					}
					members := []casino.RoomMember{{RoomID: "room", CharacterID: "self", Card: 3, Action: "high"}, {RoomID: "room", CharacterID: "other", Card: 4, Action: "high"}, {RoomID: "room", CharacterID: "spectator", IsSpectator: true, Card: -1}}
					if game == casino.GameTypeDoppel {
						members[0].Action = "♪"
						members[1].Action = "■"
					}
					if err := repo.CreateRoom(ctx, room, members[0]); err != nil {
						t.Fatal(err)
					}
					for _, m := range members[1:] {
						if err := repo.AddMember(ctx, m); err != nil {
							t.Fatal(err)
						}
					}
					chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{}}
					for _, v := range []string{"self", "other", "spectator", "outsider"} {
						chars.chars[v] = corecharacter.Character{ID: v, PlayerID: "owner"}
					}
					svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
					if err != nil {
						t.Fatal(err)
					}
					before, err := repo.ListMembers(ctx, "room")
					if err != nil {
						t.Fatal(err)
					}
					view, err := svc.GetRoomView(ctx, "room", "owner", viewer)
					if viewer == "outsider" || viewer == "" {
						if !errors.Is(err, casino.ErrRoomViewForbidden) {
							t.Fatalf("outsider: %v", err)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					for _, m := range view.Members {
						if m.IsSpectator {
							continue
						}
						hidden := phase > 0 && ((game == casino.GameTypeIndian && m.CharacterID == viewer) || (game != casino.GameTypeIndian && m.CharacterID != viewer))
						if hidden && (m.Card != -1 || m.CardDisplay != "？") {
							t.Errorf("hidden card leaked: %+v", m)
						}
						if !hidden && m.Card != map[string]int{"self": 3, "other": 4}[m.CharacterID] {
							t.Errorf("visible card missing: %+v", m)
						}
						if hidden && game != casino.GameTypeIndian && m.Action != "？？？" {
							t.Errorf("hidden action leaked: %+v", m)
						}
					}
					after, err := repo.ListMembers(ctx, "room")
					if err != nil {
						t.Fatal(err)
					}
					gotRoom, err := repo.GetRoom(ctx, "room")
					if err != nil {
						t.Fatal(err)
					}
					sort.Slice(before, func(i, j int) bool { return before[i].CharacterID < before[j].CharacterID })
					sort.Slice(after, func(i, j int) bool { return after[i].CharacterID < after[j].CharacterID })
					if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(room, *gotRoom) {
						t.Fatal("read mutated live state")
					}
					if _, err := svc.GetRoomView(ctx, "room", "attacker", viewer); !errors.Is(err, casino.ErrRoomViewForbidden) {
						t.Fatalf("spoof allowed: %v", err)
					}
					lobby, err := svc.ListRooms(ctx)
					if err != nil {
						t.Fatal(err)
					}
					data, err := json.Marshal(lobby)
					if err != nil {
						t.Fatal(err)
					}
					for _, field := range []string{"card", "mark", "action", "members", "secret", "joined_at", "pot"} {
						if strings.Contains(string(data), field) {
							t.Errorf("lobby leaked %s: %s", field, data)
						}
					}
				})
			}
		}
	}
}

type failingProjectionRepo struct {
	casino.RoomRepository
	failure error
}

func (r failingProjectionRepo) PurgeIdleRooms(context.Context, time.Time) (int, error) {
	return 0, r.failure
}

func TestRoomProjectionExpiryFailure(t *testing.T) {
	failure := errors.New("expiry unavailable")
	svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(failingProjectionRepo{casino.NewMemoryRoomRepository(), failure}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ListRooms(context.Background()); !errors.Is(err, failure) {
		t.Fatalf("expiry error lost: %v", err)
	}
}

func TestGameVisibilityExceptions(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		game           casino.GameType
		viewer, action string
		card           int
		wantAction     string
	}{
		{casino.GameTypeIndian, "self", "fold", 3, "fold"},
		{casino.GameTypeIndian, "self", "おりる", 3, "おりる"},
		{casino.GameTypeIndian, "self", "待機中", 3, "待機中"},
		{casino.GameTypeHighLow, "other", "待機中", 3, "待機中"},
		{casino.GameTypeHighLow, "other", "call", -1, "call"},
		{casino.GameTypeHighLow, "other", "fold", -1, "？？？"},
		{casino.GameTypeDoppel, "other", "待機中", -1, "待機中"},
	} {
		t.Run(string(tc.game)+"/"+tc.action, func(t *testing.T) {
			repo := casino.NewMemoryRoomRepository()
			if err := repo.CreateRoom(ctx, casino.Room{ID: "room", GameType: tc.game, Round: 1}, casino.RoomMember{RoomID: "room", CharacterID: "self", Card: 3, Action: tc.action}); err != nil {
				t.Fatal(err)
			}
			if err := repo.AddMember(ctx, casino.RoomMember{RoomID: "room", CharacterID: "other", Card: -1}); err != nil {
				t.Fatal(err)
			}
			svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo))
			if err != nil {
				t.Fatal(err)
			}
			detail, err := casino.RoomSnapshotForTest(svc, ctx, "room", tc.viewer)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range detail.Members {
				if m.CharacterID == "self" && (m.Card != tc.card || m.Action != tc.wantAction) {
					t.Fatalf("incorrect visibility: %+v", m)
				}
			}
		})
	}
}

func TestRoomViewExpiredAndEmptyLobby(t *testing.T) {
	ctx := context.Background()
	repo := casino.NewMemoryRoomRepository()
	chars := &inMemoryCharRepo{chars: map[string]corecharacter.Character{"self": {ID: "self", PlayerID: "owner"}}}
	svc, err := casino.NewService(newMockPrizeCasinoRepo(), casino.WithRoomRepository(repo), casino.WithCharacterRepository(chars))
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.ListRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(list)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "[]" {
		t.Fatalf("empty lobby must be an array: %s", data)
	}
	if err := repo.CreateRoom(ctx, casino.Room{ID: "expired", UpdatedAt: time.Now().Add(-time.Hour)}, casino.RoomMember{RoomID: "expired", CharacterID: "self"}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetRoomView(ctx, "expired", "owner", "self"); !errors.Is(err, casino.ErrRoomNotFound) {
		t.Fatalf("expired detail: %v", err)
	}
}
