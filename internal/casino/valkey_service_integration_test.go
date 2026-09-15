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
			isNX := false
			for _, arg := range args[3:] {
				if arg == "NX" {
					isNX = true
					break
				}
			}
			if isNX {
				if _, exists := strings[args[1]]; exists {
					return valkeytest.MakeNilResult()
				}
			}
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
		case "EVAL", "EVALSHA":
			if len(args) >= 5 {
				key := args[3]
				token := args[4]
				if strings[key] == token {
					delete(strings, key)
					return valkeytest.MakeIntResult(1)
				}
				return valkeytest.MakeIntResult(0)
			}
			return valkeytest.MakeIntResult(1)
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

func TestValkeyService_ConcurrentIndianPokerTurns(t *testing.T) {
	ctx := context.Background()

	client := newInMemoryValkeyClient()
	valkeyRepo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("failed to create valkey room repo: %v", err)
	}

	casinoRepo := newMockPrizeCasinoRepo()
	players := []string{"char-p1", "char-p2", "char-p3", "char-p4"}
	for _, p := range players {
		_, _ = casinoRepo.AdjustCoins(ctx, p, 1000)
	}

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(valkeyRepo))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	// 1. Create 4-player room
	created, err := svc.CreateRoom(ctx, players[0], casino.CreateRoomRequest{
		Name:            "ConcurrentIndianPoker",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedFast,
		MaxPlayers:      4,
		Rate:            10,
		Password:        "",
		AllowSpectators: false,
	})
	if err != nil {
		t.Fatalf("unexpected CreateRoom error: %v", err)
	}
	roomID := created.Room.ID

	// 2. Remaining 3 players join room
	for _, p := range players[1:] {
		_, err := svc.JoinRoom(ctx, roomID, p, "", 0)
		if err != nil {
			t.Fatalf("unexpected JoinRoom error for %s: %v", p, err)
		}
	}

	// 3. Start Indian Poker game (Round 1, initial bet 10)
	started, err := svc.StartIndianPoker(ctx, roomID, players[0])
	if err != nil {
		t.Fatalf("unexpected StartIndianPoker error: %v", err)
	}
	if started.Room.Round != 1 {
		t.Fatalf("expected round 1, got %d", started.Room.Round)
	}

	// 4. Concurrently submit turn actions (all 4 players Call simultaneously)
	var wg sync.WaitGroup
	errCh := make(chan error, len(players))

	for _, p := range players {
		wg.Add(1)
		go func(charID string) {
			defer wg.Done()
			_, actErr := svc.PlayIndianPokerAction(ctx, roomID, charID, casino.ActionCall)
			if actErr != nil {
				errCh <- actErr
			}
		}(p)
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent PlayIndianPokerAction error: %v", err)
	}

	// 5. Inspect resulting room state
	roomDetail, err := svc.GetRoomDetail(ctx, roomID, players[0])
	if err != nil {
		t.Fatalf("failed to get room detail: %v", err)
	}

	// In Indian Poker, when all 4 players call, each contributes Rate (10 coins).
	// Pot MUST be 40 (4 * 10) without any lost updates.
	expectedPot := int64(40)
	if roomDetail.Room.Pot != expectedPot {
		t.Errorf("expected Pot %d, got %d (lost update detected!)", expectedPot, roomDetail.Room.Pot)
	}

	// Verify all 4 players had exactly 10 coins deducted in account repo
	for _, p := range players {
		acc, _ := casinoRepo.GetAccount(ctx, p)
		if acc.Coins != 990 {
			t.Errorf("player %s: expected coins 990, got %d", p, acc.Coins)
		}
	}

	// When all active players call, the round should advance to Round 2
	if roomDetail.Room.Round != 2 {
		t.Errorf("expected Round 2 after all players called, got %d", roomDetail.Room.Round)
	}
}

func TestValkeyService_ConcurrentJoinsRespectMaxPlayers(t *testing.T) {
	ctx := context.Background()

	client := newInMemoryValkeyClient()
	valkeyRepo, err := casino.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("failed to create valkey room repo: %v", err)
	}

	casinoRepo := newMockPrizeCasinoRepo()
	leader := "char-lead-join"
	candidates := []string{"char-j1", "char-j2", "char-j3", "char-j4", "char-j5"}
	_, _ = casinoRepo.AdjustCoins(ctx, leader, 1000)
	for _, c := range candidates {
		_, _ = casinoRepo.AdjustCoins(ctx, c, 1000)
	}

	svc, err := casino.NewService(casinoRepo, casino.WithRoomRepository(valkeyRepo))
	if err != nil {
		t.Fatalf("failed to create casino service: %v", err)
	}

	// Max 3 players (1 leader + 2 joiners)
	created, err := svc.CreateRoom(ctx, leader, casino.CreateRoomRequest{
		Name:            "MaxPlayersRoom",
		GameType:        casino.GameTypeIndian,
		Speed:           casino.SpeedNormal,
		MaxPlayers:      3,
		Rate:            10,
		AllowSpectators: false,
	})
	if err != nil {
		t.Fatalf("unexpected CreateRoom error: %v", err)
	}
	roomID := created.Room.ID

	var wg sync.WaitGroup
	successCh := make(chan string, len(candidates))

	for _, c := range candidates {
		wg.Add(1)
		go func(charID string) {
			defer wg.Done()
			_, joinErr := svc.JoinRoom(ctx, roomID, charID, "", 0)
			if joinErr == nil {
				successCh <- charID
			}
		}(c)
	}
	wg.Wait()
	close(successCh)

	joinedCount := 0
	for range successCh {
		joinedCount++
	}

	// Exactly 2 joiners should succeed (since leader was player 1, max is 3)
	if joinedCount != 2 {
		t.Errorf("expected exactly 2 successful joiners, got %d", joinedCount)
	}

	detail, err := svc.GetRoomDetail(ctx, roomID, leader)
	if err != nil {
		t.Fatalf("failed to get room detail: %v", err)
	}
	if len(detail.Members) != 3 {
		t.Errorf("expected room member count 3, got %d", len(detail.Members))
	}
}
