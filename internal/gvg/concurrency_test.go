package gvg_test

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func newTestValkeyClient() valkey.Client {
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
		case "ZREMRANGEBYSCORE":
			return valkeytest.MakeIntResult(0)
		default:
			return valkeytest.MakeOKResult()
		}
	}))
}

func TestGVG_WithRoomLock_ReentrancyAndMutualExclusion_Memory(t *testing.T) {
	ctx := context.Background()
	repo := gvg.NewMemoryRoomRepository()

	// 1. Basic lock and reentrancy
	err := repo.WithRoomLock(ctx, "room-gvg-1", func(ctx1 context.Context) error {
		return repo.WithRoomLock(ctx1, "room-gvg-1", func(ctx2 context.Context) error {
			return nil
		})
	})
	if err != nil {
		t.Fatalf("expected reentrant lock to succeed, got %v", err)
	}

	// 2. Mutual exclusion
	var active int32
	var maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.WithRoomLock(ctx, "room-shared-gvg", func(_ context.Context) error {
				curr := atomic.AddInt32(&active, 1)
				for {
					old := atomic.LoadInt32(&maxActive)
					if curr <= old || atomic.CompareAndSwapInt32(&maxActive, old, curr) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if maxActive > 1 {
		t.Fatalf("expected max active lock holders <= 1, got %d", maxActive)
	}
}

func TestGVG_WithRoomLock_ReentrancyAndMutualExclusion_Valkey(t *testing.T) {
	ctx := context.Background()
	client := newTestValkeyClient()
	repo, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("unexpected error creating valkey room repo: %v", err)
	}

	// 1. Basic lock and reentrancy
	err = repo.WithRoomLock(ctx, "room-vk-gvg-1", func(ctx1 context.Context) error {
		return repo.WithRoomLock(ctx1, "room-vk-gvg-1", func(ctx2 context.Context) error {
			return nil
		})
	})
	if err != nil {
		t.Fatalf("expected reentrant valkey lock to succeed, got %v", err)
	}

	// 2. Empty room ID is a no-op
	err = repo.WithRoomLock(ctx, "", func(_ context.Context) error {
		return nil
	})
	if err != nil {
		t.Fatalf("expected empty roomID lock to succeed, got %v", err)
	}

	// 3. Mutual exclusion under Valkey lock
	var active int32
	var maxActive int32
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = repo.WithRoomLock(ctx, "room-shared-vk-gvg", func(_ context.Context) error {
				curr := atomic.AddInt32(&active, 1)
				for {
					old := atomic.LoadInt32(&maxActive)
					if curr <= old || atomic.CompareAndSwapInt32(&maxActive, old, curr) {
						break
					}
				}
				time.Sleep(5 * time.Millisecond)
				atomic.AddInt32(&active, -1)
				return nil
			})
		}()
	}
	wg.Wait()

	if maxActive > 1 {
		t.Fatalf("expected max active lock holders <= 1, got %d", maxActive)
	}
}

func testConcurrentGvGJoinRoomStress(t *testing.T, roomRepo gvg.RoomRepository) {
	t.Helper()
	ctx := context.Background()

	standingRepo := newMockStandingRepo()
	guildRepo := newMockGuildRepo()
	charRepo := newMockCharRepo()
	battleEngine := &mockBattleEngine{outcome: corebattle.OutcomeWin}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("failed to create gvg service: %v", err)
	}

	// Setup Leader
	leader := createTestCharacter("char-gvg-leader", "Leader", 100, 0)
	charRepo.characters[leader.ID] = leader
	leaderGuild := guild.Guild{ID: "guild-leader", Name: "LeaderGuild", Color: "#FF0000"}
	guildRepo.guilds[leaderGuild.ID] = leaderGuild
	guildRepo.charGuilds[leader.ID] = leaderGuild.ID

	const totalCandidates = 10
	for i := 1; i <= totalCandidates; i++ {
		cID := fmt.Sprintf("char-cand-%d", i)
		charRepo.characters[cID] = createTestCharacter(cID, fmt.Sprintf("Candidate%d", i), 100, 0)

		gID := fmt.Sprintf("guild-%d", i)
		gColor := fmt.Sprintf("#00%02X00", i*20)
		g := guild.Guild{ID: gID, Name: fmt.Sprintf("Guild%d", i), Color: gColor}
		guildRepo.guilds[gID] = g
		guildRepo.charGuilds[cID] = gID
	}

	// Create room with MaxMembers = 4, initial prize = 2 GP
	created, err := svc.CreateRoom(ctx, leader.ID, gvg.CreateRoomRequest{
		Name:       "StressGvG",
		MaxMembers: 4,
	})
	if err != nil {
		t.Fatalf("failed to create gvg room: %v", err)
	}
	roomID := created.Room.ID

	// Concurrently attempt to join with 10 players
	var wg sync.WaitGroup
	var successCount int32
	var fullCount int32
	errCh := make(chan error, totalCandidates)

	for i := 1; i <= totalCandidates; i++ {
		wg.Add(1)
		candidateID := fmt.Sprintf("char-cand-%d", i)
		go func(cID string) {
			defer wg.Done()
			_, joinErr := svc.JoinRoom(ctx, cID, roomID, "")
			if joinErr == nil {
				atomic.AddInt32(&successCount, 1)
			} else if errors.Is(joinErr, gvg.ErrRoomFull) {
				atomic.AddInt32(&fullCount, 1)
			} else {
				errCh <- joinErr
			}
		}(candidateID)
	}

	wg.Wait()
	close(errCh)

	for e := range errCh {
		t.Errorf("unexpected JoinRoom error: %v", e)
	}

	// Exactly 3 joiners should succeed (since leader is 1st member, max is 4)
	if successCount != 3 {
		t.Errorf("expected exactly 3 successful joiners, got %d", successCount)
	}
	// Exactly 7 candidates should be rejected with ErrRoomFull
	if fullCount != 7 {
		t.Errorf("expected exactly 7 ErrRoomFull rejections, got %d", fullCount)
	}

	// Verify room state
	detail, err := svc.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("failed to get room: %v", err)
	}
	if len(detail.Members) != 4 {
		t.Errorf("expected room member count 4, got %d", len(detail.Members))
	}
	// PrizePool should be exactly 5 GP (Initial 2 + 3 joiners * 1)
	if detail.Room.PrizePool != 5 {
		t.Errorf("expected PrizePool 5 GP, got %d (lost updates detected!)", detail.Room.PrizePool)
	}
}

func TestGVG_ConcurrentJoinRoom_Memory(t *testing.T) {
	testConcurrentGvGJoinRoomStress(t, gvg.NewMemoryRoomRepository())
}

func TestGVG_ConcurrentJoinRoom_Valkey(t *testing.T) {
	client := newTestValkeyClient()
	repo, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatalf("failed to create valkey room repo: %v", err)
	}
	testConcurrentGvGJoinRoomStress(t, repo)
}
