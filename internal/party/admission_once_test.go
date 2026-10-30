package party

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

type mockAdmissionTimer struct {
	timer.Service
	tryLockErr error
}

func (m *mockAdmissionTimer) TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error) {
	if m.tryLockErr != nil {
		return false, m.tryLockErr
	}
	return m.Service.TryLock(ctx, category, targetID, duration)
}

func (m *mockAdmissionTimer) TryLockWithToken(ctx context.Context, category, targetID, token string, duration time.Duration) (bool, error) {
	if m.tryLockErr != nil {
		return false, m.tryLockErr
	}
	return m.Service.TryLockWithToken(ctx, category, targetID, token, duration)
}

func createReadyPartyWithStage(
	ctx context.Context,
	t *testing.T,
	svc *Service,
	charRepo *mockCharacterRepository,
	stageID string,
	leader corecharacter.Character,
	members ...corecharacter.Character,
) string {
	t.Helper()

	charRepo.chars[leader.ID] = leader
	for _, m := range members {
		charRepo.chars[m.ID] = m
	}

	pDetail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:       "TestParty_" + id.New()[:8],
		StageID:    stageID,
		MaxMembers: 1 + len(members),
	})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}

	for _, m := range members {
		_, err := svc.JoinParty(ctx, pDetail.Party.ID, m.ID, "")
		if err != nil {
			t.Fatalf("JoinParty failed for %s: %v", m.ID, err)
		}
		_, err = svc.SetReady(ctx, pDetail.Party.ID, m.ID, true)
		if err != nil {
			t.Fatalf("SetReady failed for %s: %v", m.ID, err)
		}
	}

	return pDetail.Party.ID
}

func TestPartyCreation_DailyOnceLock_Lifecycle(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(timerSvc))

	leader, err := corecharacter.New("LeadChar")
	if err != nil {
		t.Fatal(err)
	}
	leader.Level = 99
	leader.Stats.HP = 500
	leader.Stats.MaxHP = 500

	member, err := corecharacter.New("MembChar")
	if err != nil {
		t.Fatal(err)
	}
	member.Level = 99
	member.Stats.HP = 500
	member.Stats.MaxHP = 500

	// 1. 初回作成成功: Leader creates party for stage-26
	// Lock MUST be consumed immediately upon creation (parity with lib/quest.cgi:596-600 $m{once}=1)
	partyID := createReadyPartyWithStage(ctx, t, svc, charRepo, "stage-26", leader, member)

	leaderLocked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, leader.ID)
	if err != nil || !leaderLocked {
		t.Fatalf("expected leader to be locked immediately after CreateParty, locked=%v, err=%v", leaderLocked, err)
	}

	// 2. 参加者のみの場合: Non-leader member must NOT be locked
	memberLocked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, member.ID)
	if err != nil || memberLocked {
		t.Fatalf("expected member NOT to be locked, locked=%v, err=%v", memberLocked, err)
	}

	// 3. 解散後の再作成拒否: Disbanding before starting does NOT reset once quota
	if err := svc.DisbandParty(ctx, partyID, leader.ID); err != nil {
		t.Fatalf("DisbandParty failed: %v", err)
	}

	_, err = svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:    "RetryParty",
		StageID: "stage-26",
	})
	if !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("expected ErrDailyOnceAlreadyUsed on re-create by leader after disband, got: %v", err)
	}

	// Member CAN create a party for stage-26
	memberPartyID := createReadyPartyWithStage(ctx, t, svc, charRepo, "stage-26", member)
	if memberPartyID == "" {
		t.Fatal("expected member party creation on stage-26 to succeed")
	}

	// 4. 起床後許可: Simulating wake reset at Home (ResetDungeonOnceWithClaim)
	if err := timerSvc.ReleaseLock(ctx, timer.CategoryDungeonOnce, leader.ID); err != nil {
		t.Fatal(err)
	}

	// Leader can now create party for stage-26 again
	_ = svc.DisbandParty(ctx, memberPartyID, member.ID)
	newPartyID := createReadyPartyWithStage(ctx, t, svc, charRepo, "stage-26", leader)
	if newPartyID == "" {
		t.Fatal("expected leader party creation to succeed after lock release")
	}

	// Adventure starts and runs to a winning conclusion
	res, err := svc.StartPartyAdventure(ctx, newPartyID, leader.ID)
	if err != nil {
		t.Fatalf("StartPartyAdventure failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected outcome win, got %s", res.Outcome)
	}
	_ = partyRepo
}

func TestPartyCreation_DailyOnceLock_CreationFailureCompensated(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(timerSvc))

	leader, err := corecharacter.New("FailChar")
	if err != nil {
		t.Fatal(err)
	}
	leader.Level = 99
	leader.Stats.HP = 500
	leader.Stats.MaxHP = 500
	charRepo.chars[leader.ID] = leader

	// Inject database failure into SaveParty
	injectedErr := errors.New("db save party connection reset")
	partyRepo.savePartyErr = injectedErr

	_, err = svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:    "FailParty",
		StageID: "stage-26",
	})
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected db error %v, got: %v", injectedErr, err)
	}

	// Lock MUST be compensated (released) on creation failure
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	if locked {
		t.Fatal("expected leader lock to be compensated after DB transaction failure, but remained locked")
	}

	// Now clear the error: leader can successfully create party
	partyRepo.savePartyErr = nil
	pDetail, err := svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:    "RecoverParty",
		StageID: "stage-26",
	})
	if err != nil {
		t.Fatalf("expected party creation to succeed after DB recovered: %v", err)
	}
	_ = svc.DisbandParty(ctx, pDetail.Party.ID, leader.ID)
}

func TestPartyCreation_DailyOnceLock_StorageFailurePropagates(t *testing.T) {
	ctx := context.Background()
	injectedErr := errors.New("valkey admission storage unavailable")
	mockTimer := &mockAdmissionTimer{
		Service:    timer.NewService(nil),
		tryLockErr: injectedErr,
	}

	svc, _, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(mockTimer))

	leader, err := corecharacter.New("StorLead")
	if err != nil {
		t.Fatal(err)
	}
	leader.Level = 99
	leader.Stats.HP = 500
	leader.Stats.MaxHP = 500
	charRepo.chars[leader.ID] = leader

	_, err = svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:    "StorageFail",
		StageID: "stage-26",
	})
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected timer error %v, got: %v", injectedErr, err)
	}
}

func TestPartyAdmission_ConcurrentSoloCrawlAndPartyCreation(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)
	partySvc, _, partyCharRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(timerSvc))

	char, err := corecharacter.New("Racer")
	if err != nil {
		t.Fatal(err)
	}
	char.Level = 99
	char.Stats.HP = 500
	char.Stats.MaxHP = 500
	partyCharRepo.chars[char.ID] = char

	const attempts = 10
	var successCount atomic.Int32
	var rejectedCount atomic.Int32
	var otherCount atomic.Int32
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	// 5 attempts of CreateParty
	for i := 0; i < attempts/2; i++ {
		wg.Add(1)
		partyName := "RaceParty_" + id.New()[:6]
		go func() {
			defer wg.Done()
			<-startBarrier
			_, err := partySvc.CreateParty(ctx, char.ID, CreatePartyRequest{
				Name:    partyName,
				StageID: "stage-26",
			})
			if err == nil {
				successCount.Add(1)
			} else if errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
				rejectedCount.Add(1)
			} else {
				otherCount.Add(1)
			}
		}()
	}

	// 5 attempts of concurrent solo crawl lock acquisition
	for i := 0; i < attempts/2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			token := id.New()
			var lockAcquired bool
			var committed bool
			defer func() {
				if !committed && lockAcquired {
					_, _ = timerSvc.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, char.ID, token)
				}
			}()
			ok, err := timerSvc.TryLockWithToken(ctx, timer.CategoryDungeonOnce, char.ID, token, 0)
			if err == nil && ok {
				lockAcquired = true
				committed = true // simulated successful crawl completion
				successCount.Add(1)
			} else if err == nil && !ok {
				rejectedCount.Add(1)
			} else {
				otherCount.Add(1)
			}
		}()
	}

	close(startBarrier)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 success across solo and party, got %d (rejected=%d, other=%d)",
			successCount.Load(), rejectedCount.Load(), otherCount.Load())
	}
	if rejectedCount.Load() != attempts-1 {
		t.Fatalf("expected %d rejections, got %d", attempts-1, rejectedCount.Load())
	}

	// Character must remain locked
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, char.ID)
	if err != nil || !locked {
		t.Fatalf("expected character to remain locked, got %v", locked)
	}
}

func TestPartyAdmission_TokenMismatchPreventsUnownedLockRelease(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)

	charID := "char-owner-test"
	tokenA := "token-owner-A"
	tokenB := "token-owner-B"

	// Process A acquires lock
	acquired, err := timerSvc.TryLockWithToken(ctx, timer.CategoryDungeonOnce, charID, tokenA, 0)
	if err != nil || !acquired {
		t.Fatalf("failed to acquire lock with tokenA: %v, %v", acquired, err)
	}

	// Process B attempts to compensate / release with tokenB
	released, err := timerSvc.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, charID, tokenB)
	if err != nil {
		t.Fatalf("ReleaseLockWithToken failed: %v", err)
	}
	if released {
		t.Fatal("ReleaseLockWithToken with mismatched tokenB should have returned false")
	}

	// Lock MUST still be held by tokenA
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, charID)
	if err != nil || !locked {
		t.Fatalf("lock was erroneously released by mismatched token, locked=%v", locked)
	}

	// Process A releases with tokenA -> successfully released
	released, err = timerSvc.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, charID, tokenA)
	if err != nil || !released {
		t.Fatalf("ReleaseLockWithToken with matching tokenA should succeed, got %v, %v", released, err)
	}

	locked, err = timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, charID)
	if err != nil || locked {
		t.Fatalf("expected lock to be released after tokenA release, locked=%v", locked)
	}
}

func TestPartyAdmission_WakeRaceTokenProtection(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)

	charID := "char-wake-race"
	token1 := "token-run-1"
	token2 := "token-run-2"

	// 1. Run 1 acquires lock
	acquired, err := timerSvc.TryLockWithToken(ctx, timer.CategoryDungeonOnce, charID, token1, 0)
	if err != nil || !acquired {
		t.Fatalf("failed to acquire lock for run 1: %v", err)
	}

	// 2. Player sleeps and wakes at Home, resetting the lock
	if err := timerSvc.ReleaseLock(ctx, timer.CategoryDungeonOnce, charID); err != nil {
		t.Fatal(err)
	}

	// 3. Run 2 acquires new lock with token2
	acquired, err = timerSvc.TryLockWithToken(ctx, timer.CategoryDungeonOnce, charID, token2, 0)
	if err != nil || !acquired {
		t.Fatalf("failed to acquire lock for run 2: %v", err)
	}

	// 4. Delayed compensation from Run 1 runs with token1
	released, err := timerSvc.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, charID, token1)
	if err != nil {
		t.Fatal(err)
	}
	if released {
		t.Fatal("Run 1 delayed compensation should NOT have released Run 2's lock")
	}

	// 5. Run 2's lock is completely intact
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, charID)
	if err != nil || !locked {
		t.Fatalf("expected Run 2's lock to remain active, locked=%v", locked)
	}
}
