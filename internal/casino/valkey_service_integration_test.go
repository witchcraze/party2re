package casino_test

import (
	"context"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func newInMemoryValkeyClient() valkey.Client {
	var mu sync.Mutex
	strings := make(map[string]string)
	zsets := make(map[string]map[string]float64)

	return valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
		mu.Lock()
		defer mu.Unlock()
		args := cmd.Commands()
		switch args[0] {
		case "SET":
			strings[args[1]] = args[2]
			return valkeytest.MakeOKResult()
		case "GET":
			val, ok := strings[args[1]]
			if !ok {
				return valkeytest.MakeNilResult()
			}
			return valkeytest.MakeStringResult(val)
		case "DEL":
			delete(strings, args[1])
			delete(zsets, args[1])
			return valkeytest.MakeOKResult()
		case "ZADD":
			key := args[1]
			if zsets[key] == nil {
				zsets[key] = make(map[string]float64)
			}
			score, _ := strconv.ParseFloat(args[2], 64)
			member := args[3]
			zsets[key][member] = score
			return valkeytest.MakeOKResult()
		case "ZREM":
			key := args[1]
			member := args[2]
			if zsets[key] != nil {
				delete(zsets[key], member)
			}
			return valkeytest.MakeOKResult()
		case "ZREVRANGE":
			key := args[1]
			type item struct {
				m string
				s float64
			}
			var list []item
			for m, s := range zsets[key] {
				list = append(list, item{m, s})
			}
			sort.Slice(list, func(i, j int) bool { return list[i].s > list[j].s })
			var res []string
			for _, it := range list {
				res = append(res, it.m)
			}
			return valkeytest.MakeStringSliceResult(res)
		case "ZRANGEBYSCORE":
			key := args[1]
			maxScore, _ := strconv.ParseFloat(args[3], 64)
			var res []string
			for m, s := range zsets[key] {
				if s <= maxScore {
					res = append(res, m)
				}
			}
			return valkeytest.MakeStringSliceResult(res)
		case "ZREMRANGEBYSCORE":
			key := args[1]
			maxScore, _ := strconv.ParseFloat(args[3], 64)
			for m, s := range zsets[key] {
				if s <= maxScore {
					delete(zsets[key], m)
				}
			}
			return valkeytest.MakeOKResult()
		default:
			return valkeytest.MakeOKResult()
		}
	}))
}

func TestCasinoService_EndToEndWithValkeyRoomRepository(t *testing.T) {
	ctx := context.Background()

	client := newInMemoryValkeyClient()
	valkeyRepo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("failed to create valkey room repo: %v", err)
	}

	casinoRepo := newMockPrizeCasinoRepo()
	charLeader := "char-lead"
	charGuest := "char-guest"
	casinoRepo.accounts[charLeader] = casino.Account{CharacterID: charLeader, Coins: 1000}
	casinoRepo.accounts[charGuest] = casino.Account{CharacterID: charGuest, Coins: 1000}

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(valkeyRepo))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	// 1. Create room
	created, err := svc.CreateRoom(ctx, charLeader, casino.CreateRoomRequest{
		Name:            "ValkeyIndianPoker",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedNormal,
		MaxPlayers:      4,
		Rate:            10,
		Password:        "",
		AllowSpectators: true,
	})
	if err != nil {
		t.Fatalf("unexpected CreateRoom error: %v", err)
	}
	roomID := created.Room.ID

	// 2. Guest joins room
	joined, err := svc.JoinRoom(ctx, roomID, charGuest, "", 0)
	if err != nil {
		t.Fatalf("unexpected JoinRoom error: %v", err)
	}
	if len(joined.Members) != 2 {
		t.Fatalf("expected 2 members in room, got %d", len(joined.Members))
	}

	// 3. List active rooms
	activeRooms, err := svc.ListRooms(ctx)
	if err != nil || len(activeRooms) != 1 {
		t.Fatalf("expected 1 active room in valkey, got %d (err=%v)", len(activeRooms), err)
	}

	// 4. Start Indian Poker
	started, err := svc.StartIndianPoker(ctx, roomID, charLeader)
	if err != nil {
		t.Fatalf("unexpected StartIndianPoker error: %v", err)
	}
	if started.Room.Round != 1 {
		t.Errorf("expected round 1, got %d", started.Room.Round)
	}

	// 5. Actions: Guest calls, Leader calls
	_, err = svc.PlayIndianPokerAction(ctx, roomID, charGuest, casino.ActionCall)
	if err != nil {
		t.Fatalf("unexpected Guest call error: %v", err)
	}
	showdown, err := svc.PlayIndianPokerAction(ctx, roomID, charLeader, casino.ActionShowdown)
	if err != nil {
		t.Fatalf("unexpected Leader showdown error: %v", err)
	}

	if showdown.Room.WinnerCharacterID == nil {
		t.Errorf("expected showdown winner to be determined")
	}

	// 6. Leave room (both members leave)
	if err := svc.LeaveRoom(ctx, roomID, charGuest); err != nil {
		t.Fatalf("unexpected Guest LeaveRoom error: %v", err)
	}
	if err := svc.LeaveRoom(ctx, roomID, charLeader); err != nil {
		t.Fatalf("unexpected Leader LeaveRoom error: %v", err)
	}

	// 7. Verify room is no longer in active rooms list
	postDisbandRooms, err := svc.ListRooms(ctx)
	if err != nil || len(postDisbandRooms) != 0 {
		t.Errorf("expected 0 active rooms after disband, got %d", len(postDisbandRooms))
	}
}
