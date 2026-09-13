package gvg_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyRoomRepository_New(t *testing.T) {
	// Nil client check
	repo, err := gvg.NewValkeyRoomRepository(nil)
	if !errors.Is(err, gvg.ErrInvalidDependencies) || repo != nil {
		t.Fatalf("expected ErrInvalidDependencies on nil client, got repo=%v, err=%v", repo, err)
	}

	// Valid client
	client := valkeytest.NewMockClient()
	repo, err = gvg.NewValkeyRoomRepository(client)
	if err != nil || repo == nil {
		t.Fatalf("expected valid repo, got repo=%v, err=%v", repo, err)
	}
}

func TestValkeyRoomRepository_SaveRoom(t *testing.T) {
	ctx := context.Background()

	room := gvg.GvGRoom{
		ID:                "room-gvg-100",
		Name:              "GvG Battle 100",
		LeaderCharacterID: "char-guild-lead-1",
		LeaderName:        "GuildLeader",
		Speed:             3,
		Stage:             1,
		MaxMembers:        4,
		TargetWins:        2,
		PrizePool:         10, // 2 GP initial + 8 GP from joiners
		Status:            gvg.StatusRecruiting,
		Round:             1,
		GuildScores:       map[string]int{"guild-1": 1, "guild-2": 0},
		CreatedAt:         time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC),
	}
	members := []gvg.GvGMember{
		{
			RoomID:        "room-gvg-100",
			CharacterID:   "char-guild-lead-1",
			CharacterName: "GuildLeader",
			GuildID:       "guild-1",
			GuildName:     "Red Knights",
			GuildColor:    "#FF3333",
			IsLeader:      true,
			JoinedAt:      time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC),
		},
		{
			RoomID:        "room-gvg-100",
			CharacterID:   "char-guild-mem-2",
			CharacterName: "BlueKnight",
			GuildID:       "guild-2",
			GuildName:     "Blue Falcons",
			GuildColor:    "#6666FF",
			IsLeader:      false,
			JoinedAt:      time.Date(2026, 9, 13, 12, 1, 0, 0, time.UTC),
		},
	}

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repo, _ := gvg.NewValkeyRoomRepository(client)

	if err := repo.SaveRoom(ctx, room, members); err != nil {
		t.Fatalf("unexpected SaveRoom error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}

	// First command: SET room key with TTL
	if cmds[0][0] != "SET" || cmds[0][1] != gvg.DefaultRoomKeyPrefix+room.ID {
		t.Errorf("unexpected SET command: %v", cmds[0])
	}
	var savedDetail gvg.RoomDetail
	if err := json.Unmarshal([]byte(cmds[0][2]), &savedDetail); err != nil {
		t.Fatalf("saved room payload is invalid JSON: %v", err)
	}
	if savedDetail.Room.ID != room.ID || len(savedDetail.Members) != 2 {
		t.Errorf("saved room detail mismatch: %+v", savedDetail)
	}
	if savedDetail.Room.PrizePool != 10 || savedDetail.Members[0].GuildName != "Red Knights" {
		t.Errorf("saved room payload content mismatch: %+v", savedDetail)
	}

	// Second command: ZADD to rooms index
	if cmds[1][0] != "ZADD" || cmds[1][1] != gvg.DefaultRoomsIndexKey || cmds[1][3] != room.ID {
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
	repoSetErr, _ := gvg.NewValkeyRoomRepository(clientSetErr)
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
	repoZaddErr, _ := gvg.NewValkeyRoomRepository(clientZaddErr)
	if err := repoZaddErr.SaveRoom(ctx, room, members); !errors.Is(err, errZadd) {
		t.Fatalf("expected errZadd, got %v", err)
	}
}

func TestValkeyRoomRepository_GetRoom(t *testing.T) {
	ctx := context.Background()

	room := gvg.GvGRoom{
		ID:        "room-get-1",
		Name:      "Get GvG Room",
		Status:    gvg.StatusRecruiting,
		PrizePool: 5,
	}
	members := []gvg.GvGMember{
		{CharacterID: "c1", CharacterName: "Leader1", GuildID: "g1", GuildColor: "#FF3333"},
	}
	validJSON, _ := json.Marshal(gvg.RoomDetail{Room: room, Members: members})

	// 1. Success
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[0] == "GET" && cmd.Commands()[1] == gvg.DefaultRoomKeyPrefix+"room-get-1" {
			return valkeytest.MakeStringResult(string(validJSON))
		}
		return valkeytest.MakeNilResult()
	}))
	repo, _ := gvg.NewValkeyRoomRepository(client)

	detail, err := repo.GetRoom(ctx, "room-get-1")
	if err != nil {
		t.Fatalf("unexpected GetRoom error: %v", err)
	}
	if detail.Room.ID != "room-get-1" || len(detail.Members) != 1 || detail.Room.PrizePool != 5 {
		t.Errorf("unexpected room detail: %+v", detail)
	}

	// 2. Room not found (nil result)
	_, err = repo.GetRoom(ctx, "nonexistent-gvg-room")
	if !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}

	// 3. Network / Valkey error
	errGet := errors.New("valkey read timeout")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errGet)
	}))
	repoErr, _ := gvg.NewValkeyRoomRepository(clientErr)
	if _, err := repoErr.GetRoom(ctx, "room-get-1"); !errors.Is(err, errGet) {
		t.Fatalf("expected errGet, got %v", err)
	}

	// 4. Corrupted JSON payload
	clientCorrupted := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeStringResult("invalid{gvg-json")
	}))
	repoCorrupted, _ := gvg.NewValkeyRoomRepository(clientCorrupted)
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
	repo, _ := gvg.NewValkeyRoomRepository(client)

	if err := repo.DeleteRoom(ctx, "del-gvg-1"); err != nil {
		t.Fatalf("unexpected DeleteRoom error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0][0] != "DEL" || cmds[0][1] != gvg.DefaultRoomKeyPrefix+"del-gvg-1" {
		t.Errorf("unexpected DEL command: %v", cmds[0])
	}
	if cmds[1][0] != "ZREM" || cmds[1][1] != gvg.DefaultRoomsIndexKey || cmds[1][2] != "del-gvg-1" {
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
	repoErr, _ := gvg.NewValkeyRoomRepository(clientErr)
	if err := repoErr.DeleteRoom(ctx, "del-gvg-1"); !errors.Is(err, errZrem) {
		t.Fatalf("expected errZrem, got %v", err)
	}
}

func TestValkeyRoomRepository_ListRooms(t *testing.T) {
	ctx := context.Background()

	r1 := gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:                "gvg-1",
			Name:              "GvG Room 1",
			LeaderCharacterID: "c1",
			LeaderName:        "Leader 1",
			Speed:             3,
			Stage:             1,
			MaxMembers:        4,
			PrizePool:         6,
			TargetWins:        2,
			Status:            gvg.StatusRecruiting,
			Round:             1,
			CreatedAt:         time.Now().UTC(),
		},
		Members: []gvg.GvGMember{{CharacterID: "c1"}},
	}
	r2InProgress := gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:     "gvg-2",
			Status: gvg.StatusInProgress, // should be filtered out
		},
		Members: []gvg.GvGMember{{CharacterID: "c2"}},
	}
	r3 := gvg.RoomDetail{
		Room: gvg.GvGRoom{
			ID:                "gvg-3",
			Name:              "GvG Room 3",
			LeaderCharacterID: "c3",
			LeaderName:        "Leader 3",
			Status:            gvg.StatusRecruiting,
			CreatedAt:         time.Now().UTC(),
		},
		Members: []gvg.GvGMember{{CharacterID: "c3"}},
	}

	r1JSON, _ := json.Marshal(r1)
	r2JSON, _ := json.Marshal(r2InProgress)
	r3JSON, _ := json.Marshal(r3)

	// 1. Success with filtering and eviction of expired rooms
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		cmdParts := cmd.Commands()
		switch cmdParts[0] {
		case "ZREVRANGE":
			return valkeytest.MakeStringSliceResult([]string{"gvg-1", "gvg-2", "gvg-expired", "gvg-3"})
		case "GET":
			switch cmdParts[1] {
			case gvg.DefaultRoomKeyPrefix + "gvg-1":
				return valkeytest.MakeStringResult(string(r1JSON))
			case gvg.DefaultRoomKeyPrefix + "gvg-2":
				return valkeytest.MakeStringResult(string(r2JSON))
			case gvg.DefaultRoomKeyPrefix + "gvg-3":
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
	repo, _ := gvg.NewValkeyRoomRepository(client)

	summaries, err := repo.ListRooms(ctx)
	if err != nil {
		t.Fatalf("unexpected ListRooms error: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 active recruiting rooms, got %d", len(summaries))
	}
	if summaries[0].ID != "gvg-1" || summaries[1].ID != "gvg-3" {
		t.Errorf("unexpected summaries order or IDs: %+v", summaries)
	}

	// Verify that gvg-expired was purged via ZREM
	purged := false
	for _, recorded := range client.RecordedCommandStrings() {
		if recorded[0] == "ZREM" && recorded[1] == gvg.DefaultRoomsIndexKey && recorded[2] == "gvg-expired" {
			purged = true
			break
		}
	}
	if !purged {
		t.Error("expected expired GvG room to be removed from index via ZREM")
	}

	// 2. Error on ZREVRANGE
	errZrev := errors.New("zrevrange error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errZrev)
	}))
	repoErr, _ := gvg.NewValkeyRoomRepository(clientErr)
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
	repo, _ := gvg.NewValkeyRoomRepository(client)

	if err := repo.SetCharacterRoom(ctx, "char-1", "room-gvg-1"); err != nil {
		t.Fatalf("unexpected SetCharacterRoom error: %v", err)
	}
	cmds := client.RecordedCommandStrings()
	if len(cmds) != 1 || cmds[0][0] != "SET" || !strings.Contains(cmds[0][1], "char-1") || cmds[0][2] != "room-gvg-1" {
		t.Errorf("unexpected SetCharacterRoom command: %v", cmds)
	}

	errSet := errors.New("set char error")
	clientSetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errSet)
	}))
	repoSetErr, _ := gvg.NewValkeyRoomRepository(clientSetErr)
	if err := repoSetErr.SetCharacterRoom(ctx, "char-1", "room-gvg-1"); !errors.Is(err, errSet) {
		t.Fatalf("expected errSet, got %v", err)
	}

	// 2. GetCharacterRoom success, not found, error
	clientGet := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		if cmd.Commands()[1] == gvg.DefaultCharacterKeyPrefix+"char-1" {
			return valkeytest.MakeStringResult("room-gvg-1")
		}
		return valkeytest.MakeNilResult()
	}))
	repoGet, _ := gvg.NewValkeyRoomRepository(clientGet)

	roomID, err := repoGet.GetCharacterRoom(ctx, "char-1")
	if err != nil || roomID != "room-gvg-1" {
		t.Fatalf("expected 'room-gvg-1', got %q, err=%v", roomID, err)
	}

	_, err = repoGet.GetCharacterRoom(ctx, "char-missing")
	if !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}

	clientGetErr := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("conn error"))
	}))
	repoGetErr, _ := gvg.NewValkeyRoomRepository(clientGetErr)
	if _, err := repoGetErr.GetCharacterRoom(ctx, "char-1"); err == nil {
		t.Fatal("expected error on get char room failure, got nil")
	}

	// 3. DeleteCharacterRoom success & error
	clientDel := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		return valkeytest.MakeOKResult()
	}))
	repoDel, _ := gvg.NewValkeyRoomRepository(clientDel)

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
	repoDelErr, _ := gvg.NewValkeyRoomRepository(clientDelErr)
	if err := repoDelErr.DeleteCharacterRoom(ctx, "char-1"); !errors.Is(err, errDel) {
		t.Fatalf("expected errDel, got %v", err)
	}
}
