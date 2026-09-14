package casino_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyRoomRepository_New(t *testing.T) {
	repo, err := casino.NewValkeyRoomRepository(nil)
	if !errors.Is(err, casino.ErrNilValkeyClient) || repo != nil {
		t.Fatalf("expected ErrNilValkeyClient on nil client, got repo=%v, err=%v", repo, err)
	}

	client := valkeytest.NewMockClient()
	repo, err = casino.NewValkeyRoomRepository(client)
	if err != nil || repo == nil {
		t.Fatalf("expected valid repo, got repo=%v, err=%v", repo, err)
	}
}

func TestValkeyRoomRepository_CreateRoom(t *testing.T) {
	ctx := context.Background()

	room := casino.Room{
		ID:                "casino-room-1",
		Name:              "HighRollerTable",
		GameType:          casino.GameTypeIndian,
		LeaderCharacterID: "char-101",
		Speed:             casino.SpeedNormal,
		MaxPlayers:        4,
		Rate:              100,
		PasswordHash:      "secret-sha256-hash",
		HasPassword:       true,
		AllowSpectators:   true,
		Status:            casino.RoomStatusWaiting,
		Round:             0,
		CurrentBet:        100,
		MaxBet:            500,
		Pot:               0,
		CreatedAt:         time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:         time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}
	leader := casino.RoomMember{
		RoomID:        "casino-room-1",
		CharacterID:   "char-101",
		CharacterName: "LeaderHero",
		IsSpectator:   false,
		Action:        "待機中",
		Card:          -1,
		JoinedAt:      time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
		UpdatedAt:     time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC),
	}

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("failed to create repo: %v", err)
	}

	if err := repo.CreateRoom(ctx, room, leader); err != nil {
		t.Fatalf("unexpected CreateRoom error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 3 {
		t.Fatalf("expected 3 commands (SET room, ZADD index, SET char), got %d: %v", len(cmds), cmds)
	}

	// 1. SET room key
	if cmds[0][0] != "SET" || cmds[0][1] != casino.DefaultRoomKeyPrefix+room.ID {
		t.Errorf("unexpected SET room command: %v", cmds[0])
	}
	// Verify JSON contains password hash
	var data map[string]any
	if err := json.Unmarshal([]byte(cmds[0][2]), &data); err != nil {
		t.Fatalf("invalid json payload: %v", err)
	}
	if data["password_hash"] != "secret-sha256-hash" {
		t.Errorf("expected password_hash in stored json, got: %v", data["password_hash"])
	}

	// 2. ZADD to active rooms index
	if cmds[1][0] != "ZADD" || cmds[1][1] != casino.DefaultRoomsActiveIndexKey || cmds[1][3] != room.ID {
		t.Errorf("unexpected ZADD command: %v", cmds[1])
	}

	// 3. SET character to room mapping
	if cmds[2][0] != "SET" || cmds[2][1] != casino.DefaultCharacterKeyPrefix+leader.CharacterID || cmds[2][2] != room.ID {
		t.Errorf("unexpected SET char command: %v", cmds[2])
	}
}

func TestValkeyRoomRepository_GetRoom(t *testing.T) {
	ctx := context.Background()

	room := casino.Room{
		ID:                "casino-room-2",
		Name:              "GetRoomTest",
		GameType:          casino.GameTypeHighLow,
		LeaderCharacterID: "char-202",
		Speed:             casino.SpeedFast,
		Status:            casino.RoomStatusWaiting,
		PasswordHash:      "my-pass-hash",
		CreatedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}
	leader := casino.RoomMember{
		RoomID:      "casino-room-2",
		CharacterID: "char-202",
	}

	type dto struct {
		Room         casino.Room         `json:"room"`
		PasswordHash string              `json:"password_hash"`
		Members      []casino.RoomMember `json:"members"`
	}
	rawJSON, _ := json.Marshal(dto{
		Room:         room,
		PasswordHash: "my-pass-hash",
		Members:      []casino.RoomMember{leader},
	})

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStr := cmd.Commands()
		if cmdStr[0] == "GET" && cmdStr[1] == casino.DefaultRoomKeyPrefix+"casino-room-2" {
			return valkeytest.MakeStringResult(string(rawJSON))
		}
		if cmdStr[0] == "GET" && cmdStr[1] == casino.DefaultRoomKeyPrefix+"missing-room" {
			return valkeytest.MakeNilResult()
		}
		return valkeytest.MakeOKResult()
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)

	// Found
	got, err := repo.GetRoom(ctx, "casino-room-2")
	if err != nil {
		t.Fatalf("unexpected GetRoom error: %v", err)
	}
	if got.ID != room.ID || got.Name != room.Name || got.PasswordHash != "my-pass-hash" {
		t.Errorf("GetRoom result mismatch: %+v", got)
	}

	// Update variant
	gotUpdate, err := repo.GetRoomForUpdate(ctx, "casino-room-2")
	if err != nil || gotUpdate.ID != room.ID {
		t.Errorf("GetRoomForUpdate failed: %v", err)
	}

	// Not found
	_, err = repo.GetRoom(ctx, "missing-room")
	if !errors.Is(err, casino.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound, got %v", err)
	}
}

func TestValkeyRoomRepository_UpdateRoom(t *testing.T) {
	ctx := context.Background()

	room := casino.Room{
		ID:                "casino-room-3",
		Name:              "UpdateTest",
		LeaderCharacterID: "c1",
		Status:            casino.RoomStatusInProgress,
		UpdatedAt:         time.Now().UTC(),
	}
	leader := casino.RoomMember{
		RoomID:      "casino-room-3",
		CharacterID: "c1",
	}

	type dto struct {
		Room    casino.Room         `json:"room"`
		Members []casino.RoomMember `json:"members"`
	}
	rawJSON, _ := json.Marshal(dto{
		Room:    room,
		Members: []casino.RoomMember{leader},
	})

	var lastSetPayload string
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStr := cmd.Commands()
		if cmdStr[0] == "GET" {
			return valkeytest.MakeStringResult(string(rawJSON))
		}
		if cmdStr[0] == "SET" && cmdStr[1] == casino.DefaultRoomKeyPrefix+"casino-room-3" {
			lastSetPayload = cmdStr[2]
			return valkeytest.MakeOKResult()
		}
		return valkeytest.MakeOKResult()
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)

	room.Status = casino.RoomStatusDisbanded
	if err := repo.UpdateRoom(ctx, room); err != nil {
		t.Fatalf("unexpected UpdateRoom error: %v", err)
	}

	if lastSetPayload == "" {
		t.Fatal("expected SET room payload to be called")
	}

	var saved dto
	if err := json.Unmarshal([]byte(lastSetPayload), &saved); err != nil {
		t.Fatalf("invalid payload json: %v", err)
	}
	if saved.Room.Status != casino.RoomStatusDisbanded {
		t.Errorf("expected status disbanded, got %v", saved.Room.Status)
	}
}

func TestValkeyRoomRepository_DeleteRoom(t *testing.T) {
	ctx := context.Background()

	room := casino.Room{ID: "casino-del-1"}
	member := casino.RoomMember{RoomID: "casino-del-1", CharacterID: "char-del-1"}

	type dto struct {
		Room    casino.Room         `json:"room"`
		Members []casino.RoomMember `json:"members"`
	}
	rawJSON, _ := json.Marshal(dto{Room: room, Members: []casino.RoomMember{member}})

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" {
			return valkeytest.MakeStringResult(string(rawJSON))
		}
		return valkeytest.MakeOKResult()
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)
	if err := repo.DeleteRoom(ctx, "casino-del-1"); err != nil {
		t.Fatalf("unexpected DeleteRoom error: %v", err)
	}

	deletedChar := false
	deletedRoom := false
	zremRoom := false
	for _, cmd := range client.RecordedCommandStrings() {
		if cmd[0] == "DEL" && cmd[1] == casino.DefaultCharacterKeyPrefix+"char-del-1" {
			deletedChar = true
		}
		if cmd[0] == "DEL" && cmd[1] == casino.DefaultRoomKeyPrefix+"casino-del-1" {
			deletedRoom = true
		}
		if cmd[0] == "ZREM" && cmd[1] == casino.DefaultRoomsActiveIndexKey && cmd[2] == "casino-del-1" {
			zremRoom = true
		}
	}

	if !deletedChar || !deletedRoom || !zremRoom {
		t.Errorf("DeleteRoom did not delete all keys: deletedChar=%v, deletedRoom=%v, zremRoom=%v",
			deletedChar, deletedRoom, zremRoom)
	}
}

func TestValkeyRoomRepository_ListActiveRoomsAndGetByName(t *testing.T) {
	ctx := context.Background()

	room1 := casino.Room{
		ID:     "room-active-1",
		Name:   "ActiveRoomAlpha",
		Status: casino.RoomStatusWaiting,
	}
	room2 := casino.Room{
		ID:     "room-active-2",
		Name:   "ActiveRoomBeta",
		Status: casino.RoomStatusDisbanded,
	}

	type dto struct {
		Room    casino.Room         `json:"room"`
		Members []casino.RoomMember `json:"members"`
	}
	json1, _ := json.Marshal(dto{Room: room1, Members: []casino.RoomMember{{CharacterID: "c1"}}})
	json2, _ := json.Marshal(dto{Room: room2, Members: []casino.RoomMember{{CharacterID: "c2"}}})

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStr := cmd.Commands()
		switch cmdStr[0] {
		case "ZREVRANGE":
			return valkeytest.MakeStringSliceResult([]string{"room-active-1", "room-active-2", "room-stale"})
		case "GET":
			switch cmdStr[1] {
			case casino.DefaultRoomKeyPrefix + "room-active-1":
				return valkeytest.MakeStringResult(string(json1))
			case casino.DefaultRoomKeyPrefix + "room-active-2":
				return valkeytest.MakeStringResult(string(json2))
			default:
				return valkeytest.MakeNilResult()
			}
		default:
			return valkeytest.MakeOKResult()
		}
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)

	// ListActiveRooms should return only room1 (not disbanded, not stale)
	list, err := repo.ListActiveRooms(ctx)
	if err != nil {
		t.Fatalf("unexpected ListActiveRooms error: %v", err)
	}
	if len(list) != 1 || list[0].Room.ID != "room-active-1" {
		t.Fatalf("expected 1 active room (room-active-1), got: %+v", list)
	}

	// GetRoomByName: found
	found, err := repo.GetRoomByName(ctx, "ActiveRoomAlpha")
	if err != nil || found.ID != "room-active-1" {
		t.Errorf("GetRoomByName found mismatch: %v, %v", found, err)
	}

	// GetRoomByName: not found (disbanded or non-existent)
	_, err = repo.GetRoomByName(ctx, "ActiveRoomBeta")
	if !errors.Is(err, casino.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound for disbanded room, got: %v", err)
	}
}

func TestValkeyRoomRepository_MemberLifecycle(t *testing.T) {
	ctx := context.Background()

	room := casino.Room{
		ID:        "room-mems-1",
		Status:    casino.RoomStatusWaiting,
		UpdatedAt: time.Now().UTC(),
	}
	leader := casino.RoomMember{
		RoomID:      "room-mems-1",
		CharacterID: "char-leader",
		Action:      "待機中",
	}

	type dto struct {
		Room    casino.Room         `json:"room"`
		Members []casino.RoomMember `json:"members"`
	}

	currentDTO := dto{
		Room:    room,
		Members: []casino.RoomMember{leader},
	}

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStr := cmd.Commands()
		switch cmdStr[0] {
		case "GET":
			if cmdStr[1] == casino.DefaultRoomKeyPrefix+"room-mems-1" {
				data, _ := json.Marshal(currentDTO)
				return valkeytest.MakeStringResult(string(data))
			}
			if cmdStr[1] == casino.DefaultCharacterKeyPrefix+"char-joiner" {
				return valkeytest.MakeStringResult("room-mems-1")
			}
			return valkeytest.MakeNilResult()
		case "SET":
			if cmdStr[1] == casino.DefaultRoomKeyPrefix+"room-mems-1" {
				_ = json.Unmarshal([]byte(cmdStr[2]), &currentDTO)
			}
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)

	// 1. AddMember
	joiner := casino.RoomMember{
		RoomID:      "room-mems-1",
		CharacterID: "char-joiner",
		Action:      "待機中",
		Card:        -1,
	}
	if err := repo.AddMember(ctx, joiner); err != nil {
		t.Fatalf("unexpected AddMember error: %v", err)
	}

	// 2. ListMembers
	mems, err := repo.ListMembers(ctx, "room-mems-1")
	if err != nil || len(mems) != 2 {
		t.Fatalf("expected 2 members, got %d (err=%v)", len(mems), err)
	}

	// 3. GetMember
	gotJoiner, err := repo.GetMember(ctx, "room-mems-1", "char-joiner")
	if err != nil || gotJoiner.CharacterID != "char-joiner" {
		t.Fatalf("expected char-joiner, got %v (err=%v)", gotJoiner, err)
	}

	// 4. UpdateMember
	gotJoiner.Action = "しょうぶ"
	if err := repo.UpdateMember(ctx, *gotJoiner); err != nil {
		t.Fatalf("unexpected UpdateMember error: %v", err)
	}
	updated, _ := repo.GetMember(ctx, "room-mems-1", "char-joiner")
	if updated.Action != "しょうぶ" {
		t.Errorf("expected updated action 'しょうぶ', got %s", updated.Action)
	}

	// 5. GetCharacterRoom
	charRoom, err := repo.GetCharacterRoom(ctx, "char-joiner")
	if err != nil || charRoom != "room-mems-1" {
		t.Errorf("expected room-mems-1, got %s (err=%v)", charRoom, err)
	}

	// 6. RemoveMember
	if err := repo.RemoveMember(ctx, "room-mems-1", "char-joiner"); err != nil {
		t.Fatalf("unexpected RemoveMember error: %v", err)
	}
	remaining, _ := repo.ListMembers(ctx, "room-mems-1")
	if len(remaining) != 1 || remaining[0].CharacterID != "char-leader" {
		t.Errorf("expected 1 remaining member, got %d", len(remaining))
	}
}

func TestValkeyRoomRepository_PurgeIdleRooms(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdStr := cmd.Commands()
		if cmdStr[0] == "ZRANGEBYSCORE" {
			return valkeytest.MakeStringSliceResult([]string{"idle-1", "idle-2"})
		}
		return valkeytest.MakeOKResult()
	}))

	repo, _ := casino.NewValkeyRoomRepository(client)
	count, err := repo.PurgeIdleRooms(ctx, time.Now().UTC().Add(-30*time.Minute))
	if err != nil {
		t.Fatalf("unexpected PurgeIdleRooms error: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2 purged rooms, got %d", count)
	}
}
