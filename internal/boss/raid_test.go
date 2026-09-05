package boss_test

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/valkey"
)

// stubSettlementRepo provides an in-memory transactional settlement repository.
type stubSettlementRepo struct {
	mu          sync.Mutex
	settledRuns map[string]boss.RaidSettlement
	settleCalls int
}

func newStubSettlementRepo() *stubSettlementRepo {
	return &stubSettlementRepo{
		settledRuns: make(map[string]boss.RaidSettlement),
	}
}

func (s *stubSettlementRepo) IsRunSettled(ctx context.Context, runID string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, exists := s.settledRuns[runID]
	return exists, nil
}

func (s *stubSettlementRepo) RecordRaidSettlement(ctx context.Context, settlement boss.RaidSettlement) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.settleCalls++
	s.settledRuns[settlement.RunID] = settlement
	return nil
}

func TestRaidCoordinator_Memory_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo := boss.NewValkeyRaidRepository(nil) // in-memory fallback
	settleRepo := newStubSettlementRepo()

	coordinator := boss.NewRaidCoordinator(repo, settleRepo)

	bossID := "world-boss-01"
	runID := "run-uuid-1"
	initialHP := 1000

	if err := coordinator.StartRaid(ctx, bossID, runID, initialHP); err != nil {
		t.Fatalf("failed to start raid: %v", err)
	}

	state, err := coordinator.GetRaidState(ctx, bossID)
	if err != nil {
		t.Fatalf("failed to get raid state: %v", err)
	}
	if state.CurrentHP != 1000 || state.Status != boss.RaidStatusActive {
		t.Fatalf("unexpected state: %+v", state)
	}

	// 1. Normal hit
	res, settlement, err := coordinator.AttackBoss(ctx, bossID, "char-1", 200)
	if err != nil {
		t.Fatalf("attack failed: %v", err)
	}
	if res.Status != boss.RaidDamageHit || res.ActualDamage != 200 || res.RemainingHP != 800 {
		t.Errorf("unexpected damage result: %+v", res)
	}
	if settlement != nil {
		t.Errorf("expected no settlement on normal hit")
	}

	// 2. Another attacker hits
	res, _, err = coordinator.AttackBoss(ctx, bossID, "char-2", 300)
	if err != nil {
		t.Fatalf("attack failed: %v", err)
	}
	if res.Status != boss.RaidDamageHit || res.ActualDamage != 300 || res.RemainingHP != 500 {
		t.Errorf("unexpected damage result: %+v", res)
	}

	// 3. Overkill hit: attack for 9999 when only 500 HP remains
	res, settlement, err = coordinator.AttackBoss(ctx, bossID, "char-3", 9999)
	if err != nil {
		t.Fatalf("killing attack failed: %v", err)
	}
	if res.Status != boss.RaidDamageKilled {
		t.Fatalf("expected killed status, got: %+v", res)
	}
	if res.ActualDamage != 500 {
		t.Errorf("expected actual damage to be capped at 500 (overkill prevention), got %d", res.ActualDamage)
	}
	if res.RemainingHP != 0 {
		t.Errorf("expected remaining hp 0, got %d", res.RemainingHP)
	}
	if res.KillerID != "char-3" {
		t.Errorf("expected killer char-3, got %q", res.KillerID)
	}

	// Verify settlement was triggered automatically
	if settlement == nil {
		t.Fatalf("expected non-nil settlement on kill")
	}
	if settlement.KillerID != "char-3" {
		t.Errorf("expected settlement killer char-3, got %q", settlement.KillerID)
	}
	if settlement.MVPID != "char-3" { // char-3 dealt 500, char-2 dealt 300, char-1 dealt 200
		t.Errorf("expected MVP char-3, got %q", settlement.MVPID)
	}
	if settlement.TotalDamage != 1000 {
		t.Errorf("expected total damage 1000, got %d", settlement.TotalDamage)
	}

	// Verify rewards structure
	reward3 := settlement.Rewards["char-3"]
	if !reward3.IsLastHit || !reward3.IsMVP {
		t.Errorf("expected char-3 to have LastHit and MVP flags: %+v", reward3)
	}

	// 4. Subsequent attack after defeat
	resAfter, _, err := coordinator.AttackBoss(ctx, bossID, "char-4", 100)
	if err != nil {
		t.Fatalf("attack after defeat failed: %v", err)
	}
	if resAfter.Status != boss.RaidDamageAlreadyDead {
		t.Errorf("expected already_dead, got %+v", resAfter)
	}
	if resAfter.ActualDamage != 0 {
		t.Errorf("expected 0 actual damage after defeat, got %d", resAfter.ActualDamage)
	}

	// Verify durable repo was settled exactly once
	if settleRepo.settleCalls != 1 {
		t.Errorf("expected exactly 1 settlement call, got %d", settleRepo.settleCalls)
	}
}

func TestRaidCoordinator_CrashRecovery_Reconciliation(t *testing.T) {
	ctx := context.Background()
	repo := boss.NewValkeyRaidRepository(nil)
	settleRepo := newStubSettlementRepo()

	coordinator := boss.NewRaidCoordinator(repo, settleRepo)

	bossID := "boss-crash-test"
	runID := "run-crash-1"

	_ = coordinator.StartRaid(ctx, bossID, runID, 500)
	_, _, _ = coordinator.AttackBoss(ctx, bossID, "hero-1", 500) // Kills boss

	// Check settled
	isSettled, _ := settleRepo.IsRunSettled(ctx, runID)
	if !isSettled {
		t.Fatalf("expected settled")
	}

	// Reconciliation should be idempotent and return nil without duplicate settlement calls
	beforeCalls := settleRepo.settleCalls
	reconciled, err := coordinator.ReconcileUnsettledBoss(ctx, bossID)
	if err != nil {
		t.Fatalf("reconciliation failed: %v", err)
	}
	if reconciled != nil {
		t.Errorf("expected nil settlement for already-settled boss")
	}
	if settleRepo.settleCalls != beforeCalls {
		t.Errorf("reconciliation duplicated settlement calls: %d vs %d", settleRepo.settleCalls, beforeCalls)
	}
}

func TestRaid_LiveValkey_Lua_Concurrency(t *testing.T) {
	client, err := valkey.NewClient()
	if err != nil {
		t.Skip("Valkey is not reachable on localhost:6379, skipping live Valkey test")
	}
	defer client.Close()

	ctx := context.Background()
	if err := client.Do(ctx, client.B().Ping().Build()).Error(); err != nil {
		t.Skipf("Valkey ping failed: %v", err)
	}

	testPrefix := fmt.Sprintf("party2:test:boss:%d:", time.Now().UnixNano())
	repo := boss.NewValkeyRaidRepository(client, boss.WithRaidKeyPrefix(testPrefix))
	settleRepo := newStubSettlementRepo()
	coordinator := boss.NewRaidCoordinator(repo, settleRepo)

	bossID := "concurrent-boss"
	runID := fmt.Sprintf("run-%d", time.Now().UnixNano())
	initialHP := 50000

	if err := coordinator.StartRaid(ctx, bossID, runID, initialHP); err != nil {
		t.Fatalf("failed to start live raid: %v", err)
	}

	// 120 concurrent attackers continuously attacking
	numRaiders := 120
	var wg sync.WaitGroup
	var killerCount int32
	var totalDamageDealt int64

	startSignal := make(chan struct{})

	for i := 0; i < numRaiders; i++ {
		wg.Add(1)
		charID := fmt.Sprintf("raider-%03d", i)
		go func(attacker string) {
			defer wg.Done()
			<-startSignal

			rng := rand.New(rand.NewSource(time.Now().UnixNano() + int64(len(attacker))))
			for {
				dmg := rng.Intn(100) + 10 // 10 to 109 damage
				res, _, err := coordinator.AttackBoss(ctx, bossID, attacker, dmg)
				if err != nil {
					return
				}

				atomic.AddInt64(&totalDamageDealt, int64(res.ActualDamage))

				if res.Status == boss.RaidDamageKilled {
					atomic.AddInt32(&killerCount, 1)
					break
				}
				if res.Status == boss.RaidDamageAlreadyDead {
					break
				}
			}
		}(charID)
	}

	// Fire all raiders simultaneously
	close(startSignal)
	wg.Wait()

	// 1. Verify exactly one killer elected
	if killerCount != 1 {
		t.Fatalf("expected exactly 1 killer elected, got %d", killerCount)
	}

	// 2. Verify total actual damage dealt equals initial HP (zero overkill, zero underkill)
	if int(totalDamageDealt) != initialHP {
		t.Fatalf("expected total damage %d, got %d", initialHP, totalDamageDealt)
	}

	// 3. Verify contributors tally equals initial HP
	contributors, err := repo.GetContributors(ctx, bossID)
	if err != nil {
		t.Fatalf("failed to get contributors: %v", err)
	}
	contribTotal := 0
	for _, dmg := range contributors {
		contribTotal += dmg
	}
	if contribTotal != initialHP {
		t.Fatalf("expected contributor sum %d, got %d", initialHP, contribTotal)
	}

	// 4. Verify exactly one settlement call occurred
	if settleRepo.settleCalls != 1 {
		t.Fatalf("expected exactly 1 settlement call, got %d", settleRepo.settleCalls)
	}

	// 5. Verify final status is settled
	state, err := coordinator.GetRaidState(ctx, bossID)
	if err != nil {
		t.Fatalf("failed to get final state: %v", err)
	}
	if state.CurrentHP != 0 || (state.Status != boss.RaidStatusDefeated && state.Status != boss.RaidStatusSettled) {
		t.Fatalf("unexpected final state: %+v", state)
	}

	// Cleanup test keys
	keys := []string{
		testPrefix + "{boss:" + bossID + "}:hp",
		testPrefix + "{boss:" + bossID + "}:status",
		testPrefix + "{boss:" + bossID + "}:contributors",
		testPrefix + "{boss:" + bossID + "}:killer",
		testPrefix + "{boss:" + bossID + "}:run_id",
	}
	for _, k := range keys {
		_ = client.Do(ctx, client.B().Del().Key(k).Build()).Error()
	}
}

func BenchmarkApplyDamage_Memory(b *testing.B) {
	ctx := context.Background()
	repo := boss.NewValkeyRaidRepository(nil)
	_ = repo.InitializeRaid(ctx, "bench-boss", "run-bench", 1000000000, 2*time.Hour)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := "bench-raider"
		for pb.Next() {
			_, _ = repo.ApplyDamage(ctx, "bench-boss", id, 10, 2*time.Hour)
		}
	})
}

func BenchmarkApplyDamage_Valkey(b *testing.B) {
	client, err := valkey.NewClient()
	if err != nil {
		b.Skip("Valkey not available")
	}
	defer client.Close()

	ctx := context.Background()
	testPrefix := fmt.Sprintf("party2:test:boss:bench:%d:", time.Now().UnixNano())
	repo := boss.NewValkeyRaidRepository(client, boss.WithRaidKeyPrefix(testPrefix))
	_ = repo.InitializeRaid(ctx, "bench-boss", "run-bench", 1000000000, 2*time.Hour)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		id := "bench-raider"
		for pb.Next() {
			_, _ = repo.ApplyDamage(ctx, "bench-boss", id, 10, 2*time.Hour)
		}
	})

	b.StopTimer()
	keys := []string{
		testPrefix + "{boss:bench-boss}:hp",
		testPrefix + "{boss:bench-boss}:status",
		testPrefix + "{boss:bench-boss}:contributors",
		testPrefix + "{boss:bench-boss}:killer",
		testPrefix + "{boss:bench-boss}:run_id",
	}
	for _, k := range keys {
		_ = client.Do(ctx, client.B().Del().Key(k).Build()).Error()
	}
}
