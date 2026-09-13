package pvp_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyRoomRepository_New(t *testing.T) {
	// Nil client check
	repo, err := pvp.NewValkeyRoomRepository(nil)
	if !errors.Is(err, pvp.ErrInvalidDependencies) || repo != nil {
		t.Fatalf("expected ErrInvalidDependencies on nil client, got repo=%v, err=%v", repo, err)
	}

	// Valid client
	client := valkeytest.NewMockClient()
	repo, err = pvp.NewValkeyRoomRepository(client)
	if err != nil || repo == nil {
		t.Fatalf("expected valid repo, got repo=%v, err=%v", repo, err)
	}
}

func TestValkeyRoomRepository_SaveRoom(t *testing.T) {
	ctx := context.Background()

	room := pvp.ColosseumRoom{
		ID:                "room-pvp-100",
		Name:              "PvP Arena 100",
		LeaderCharacterID: "char-leader-1",
		LeaderName:        "LeaderHero",
		Speed:             3,
		Stage:             1,
		MaxMembers:        4,
		Bet:               1000,
		PrizePool:         4000,
		TargetWins:        2,
		Status:            pvp.StatusRecruiting,
		Round:             1,
		CreatedAt:         time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC),
	}
	members := []pvp.RoomMember{
		{
			CharacterID:   "char-leader-1",
			CharacterName: "LeaderHero",
			TeamColor:     "#FF0000",
			JoinedAt:      time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			CharacterID:   "char-challenger-2",
			CharacterName: "ChallengerHero",
			TeamColor:     "#0000FF",
			JoinedAt:      time.Date(2026, 9, 13, 12, 1, 0, 0, time.UTC),
		},
	}

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo, _ := pvp.NewValkeyRoomRepository(client)

	if err := repo.SaveRoom(ctx, room, members); err != nil {
		t.Fatalf("unexpected SaveRoom error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}

	// First command: SET room key with TTL
	if cmds[0][0] != "SET" || cmds[0][1] != pvp.DefaultRoomKeyPrefix+room.ID {
		t.Errorf("unexpected SET command: %v", cmds[0])
	}
	var savedDetail pvp.RoomDetail
	if err := json.Unmarshal([]byte(cmds[0][2]), &savedDetail); err != nil {
		t.Fatalf("saved room payload is invalid JSON: %v", err)
	}
	if savedDetail.Room.ID != room.ID || len(savedDetail.Members) != 2 {
		t.Errorf("saved room detail mismatch: %+v", savedDetail)
	}

	// Second command: ZADD to rooms index
	if cmds[1][0] != "ZADD" || cmds[1][1] != pvp.DefaultRoomsIndexKey || cmds[1][3] != room.ID {
		t.Errorf("unexpected ZADD command: %v", cmds[1])
	}

	// 2. SET error
	errSet := errors.New("valkey set error")
	clientSetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "SET" {
			return valkeytest.MakeErrorResult(errSet)
		}
		return valkeytest.MakeOKResult()
	}))
	repoSetErr, _ := pvp.NewValkeyRoomRepository(clientSetErr)
	if err := repoSetErr.SaveRoom(ctx, room, members); !errors.Is(err, errSet) {
		t.Fatalf("expected errSet, got %v", err)
	}

	// 3. ZADD error
	errZadd := errors.New("valkey zadd error")
	clientZaddErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "ZADD" {
			return valkeytest.MakeErrorResult(errZadd)
		}
		return valkeytest.MakeOKResult()
	}))
	repoZaddErr, _ := pvp.NewValkeyRoomRepository(clientZaddErr)
	if err := repoZaddErr.SaveRoom(ctx, room, members); !errors.Is(err, errZadd) {
		t.Fatalf("expected errZadd, got %v", err)
	}
}

func TestValkeyRoomRepository_GetRoom(t *testing.T) {
	ctx := context.Background()

	room := pvp.ColosseumRoom{
		ID:     "room-get-1",
		Name:   "Get Room",
		Status: pvp.StatusRecruiting,
	}
	members := []pvp.RoomMember{
		{CharacterID: "c1", CharacterName: "Hero1", TeamColor: "#FFFFFF"},
	}
	validJSON, _ := json.Marshal(pvp.RoomDetail{Room: room, Members: members})

	// 1. Success
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" && cmd.Commands()[1] == pvp.DefaultRoomKeyPrefix+"room-get-1" {
			return valkeytest.MakeStringResult(string(validJSON))
		}
		return valkeytest.MakeNilResult()
	}))
	repo, _ := pvp.NewValkeyRoomRepository(client)

	detail, err := repo.GetRoom(ctx, "room-get-1")
	if err != nil {
		t.Fatalf("unexpected GetRoom error: %v", err)
	}
	if detail.Room.ID != "room-get-1" || len(detail.Members) != 1 {
		t.Errorf("unexpected room detail: %+v", detail)
	}

	// 2. Room not found (nil result)
	_, err = repo.GetRoom(ctx, "nonexistent-room")
	if !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}

	// 3. Network / Valkey error
	errGet := errors.New("valkey read timeout")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errGet)
	}))
	repoErr, _ := pvp.NewValkeyRoomRepository(clientErr)
	if _, err := repoErr.GetRoom(ctx, "room-get-1"); !errors.Is(err, errGet) {
		t.Fatalf("expected errGet, got %v", err)
	}

	// 4. Corrupted JSON payload
	clientCorrupted := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("invalid{json")
	}))
	repoCorrupted, _ := pvp.NewValkeyRoomRepository(clientCorrupted)
	if _, err := repoCorrupted.GetRoom(ctx, "room-get-1"); err == nil {
		t.Fatal("expected error on malformed JSON, got nil")
	}
}

func TestValkeyRoomRepository_DeleteRoom(t *testing.T) {
	ctx := context.Background()

	// 1. Success
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo, _ := pvp.NewValkeyRoomRepository(client)

	if err := repo.DeleteRoom(ctx, "del-room-1"); err != nil {
		t.Fatalf("unexpected DeleteRoom error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0][0] != "DEL" || cmds[0][1] != pvp.DefaultRoomKeyPrefix+"del-room-1" {
		t.Errorf("unexpected DEL command: %v", cmds[0])
	}
	if cmds[1][0] != "ZREM" || cmds[1][1] != pvp.DefaultRoomsIndexKey || cmds[1][2] != "del-room-1" {
		t.Errorf("unexpected ZREM command: %v", cmds[1])
	}

	// 2. Error on ZREM
	errZrem := errors.New("zrem error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "ZREM" {
			return valkeytest.MakeErrorResult(errZrem)
		}
		return valkeytest.MakeOKResult()
	}))
	repoErr, _ := pvp.NewValkeyRoomRepository(clientErr)
	if err := repoErr.DeleteRoom(ctx, "del-room-1"); !errors.Is(err, errZrem) {
		t.Fatalf("expected errZrem, got %v", err)
	}
}

func TestValkeyRoomRepository_ListRooms(t *testing.T) {
	ctx := context.Background()

	r1 := pvp.RoomDetail{
		Room: pvp.ColosseumRoom{
			ID:                "room-1",
			Name:              "Room 1",
			LeaderCharacterID: "c1",
			LeaderName:        "Leader 1",
			Speed:             3,
			Stage:             1,
			MaxMembers:        4,
			Bet:               500,
			PrizePool:         2000,
			TargetWins:        1,
			Status:            pvp.StatusRecruiting,
			Round:             1,
			CreatedAt:         time.Now().UTC(),
		},
		Members: []pvp.RoomMember{{CharacterID: "c1"}},
	}
	r2InBattle := pvp.RoomDetail{
		Room: pvp.ColosseumRoom{
			ID:     "room-2",
			Status: pvp.StatusInProgress, // should be filtered out
		},
		Members: []pvp.RoomMember{{CharacterID: "c2"}},
	}
	r3 := pvp.RoomDetail{
		Room: pvp.ColosseumRoom{
			ID:                "room-3",
			Name:              "Room 3",
			LeaderCharacterID: "c3",
			LeaderName:        "Leader 3",
			Status:            pvp.StatusRecruiting,
			CreatedAt:         time.Now().UTC(),
		},
		Members: []pvp.RoomMember{{CharacterID: "c3"}},
	}

	r1JSON, _ := json.Marshal(r1)
	r2JSON, _ := json.Marshal(r2InBattle)
	r3JSON, _ := json.Marshal(r3)

	// 1. Success with filtering and eviction of expired rooms
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdParts := cmd.Commands()
		switch cmdParts[0] {
		case "ZREVRANGE":
			return valkeytest.MakeStringSliceResult([]string{"room-1", "room-2", "room-expired", "room-3"})
		case "GET":
			switch cmdParts[1] {
			case pvp.DefaultRoomKeyPrefix + "room-1":
				return valkeytest.MakeStringResult(string(r1JSON))
			case pvp.DefaultRoomKeyPrefix + "room-2":
				return valkeytest.MakeStringResult(string(r2JSON))
			case pvp.DefaultRoomKeyPrefix + "room-3":
				return valkeytest.MakeStringResult(string(r3JSON))
			default:
				return valkeytest.MakeNilResult() // expired / missing
			}
		case "ZREM":
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))
	repo, _ := pvp.NewValkeyRoomRepository(client)

	summaries, err := repo.ListRooms(ctx)
	if err != nil {
		t.Fatalf("unexpected ListRooms error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 active recruiting rooms, got %d", len(summaries))
	}
	if summaries[0].ID != "room-1" || summaries[1].ID != "room-3" {
		t.Errorf("unexpected summaries order or IDs: %+v", summaries)
	}

	// Verify that room-expired was purged via ZREM
	purged := false
	for _, recorded := range client.RecordedCommandStrings() {
		if recorded[0] == "ZREM" && recorded[1] == pvp.DefaultRoomsIndexKey && recorded[2] == "room-expired" {
			purged = true
			break
		}
	}
	if !purged {
		t.Error("expected expired room to be removed from index via ZREM")
	}

	// 2. Error on ZREVRANGE
	errZrev := errors.New("zrevrange error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errZrev)
	}))
	repoErr, _ := pvp.NewValkeyRoomRepository(clientErr)
	if _, err := repoErr.ListRooms(ctx); !errors.Is(err, errZrev) {
		t.Fatalf("expected errZrev, got %v", err)
	}
}

func TestValkeyRoomRepository_CharacterRoom(t *testing.T) {
	ctx := context.Background()

	// 1. SetCharacterRoom success & error
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo, _ := pvp.NewValkeyRoomRepository(client)

	if err := repo.SetCharacterRoom(ctx, "char-1", "room-1"); err != nil {
		t.Fatalf("unexpected SetCharacterRoom error: %v", err)
	}
	cmds := client.RecordedCommandStrings()
	if len(cmds) != 1 || cmds[0][0] != "SET" || !strings.Contains(cmds[0][1], "char-1") || cmds[0][2] != "room-1" {
		t.Errorf("unexpected SetCharacterRoom command: %v", cmds)
	}

	errSet := errors.New("set char error")
	clientSetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errSet)
	}))
	repoSetErr, _ := pvp.NewValkeyRoomRepository(clientSetErr)
	if err := repoSetErr.SetCharacterRoom(ctx, "char-1", "room-1"); !errors.Is(err, errSet) {
		t.Fatalf("expected errSet, got %v", err)
	}

	// 2. GetCharacterRoom success, not found, error
	clientGet := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[1] == pvp.DefaultCharacterKeyPrefix+"char-1" {
			return valkeytest.MakeStringResult("room-1")
		}
		return valkeytest.MakeNilResult()
	}))
	repoGet, _ := pvp.NewValkeyRoomRepository(clientGet)

	roomID, err := repoGet.GetCharacterRoom(ctx, "char-1")
	if err != nil || roomID != "room-1" {
		t.Fatalf("expected 'room-1', got %q, err=%v", roomID, err)
	}

	_, err = repoGet.GetCharacterRoom(ctx, "char-missing")
	if !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}

	clientGetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("conn error"))
	}))
	repoGetErr, _ := pvp.NewValkeyRoomRepository(clientGetErr)
	if _, err := repoGetErr.GetCharacterRoom(ctx, "char-1"); err == nil {
		t.Fatal("expected error on get char room failure, got nil")
	}

	// 3. DeleteCharacterRoom success & error
	clientDel := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoDel, _ := pvp.NewValkeyRoomRepository(clientDel)

	if err := repoDel.DeleteCharacterRoom(ctx, "char-1"); err != nil {
		t.Fatalf("unexpected DeleteCharacterRoom error: %v", err)
	}
	delCmds := clientDel.RecordedCommandStrings()
	if len(delCmds) != 1 || delCmds[0][0] != "DEL" || !strings.Contains(delCmds[0][1], "char-1") {
		t.Errorf("unexpected DeleteCharacterRoom command: %v", delCmds)
	}

	errDel := errors.New("del error")
	clientDelErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errDel)
	}))
	repoDelErr, _ := pvp.NewValkeyRoomRepository(clientDelErr)
	if err := repoDelErr.DeleteCharacterRoom(ctx, "char-1"); !errors.Is(err, errDel) {
		t.Fatalf("expected errDel, got %v", err)
	}
}
