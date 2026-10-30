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
)

type mockAdmissionTimer struct {
	timer.Service
	tryLockErr  error
	isLockedErr error
}

func (m *mockAdmissionTimer) TryLock(ctx context.Context, category, targetID string, duration time.Duration) (bool, error) {
	if m.tryLockErr != nil {
		return false, m.tryLockErr
	}
	return m.Service.TryLock(ctx, category, targetID, duration)
}

func (m *mockAdmissionTimer) IsLocked(ctx context.Context, category, targetID string) (bool, error) {
	if m.isLockedErr != nil {
		return false, m.isLockedErr
	}
	return m.Service.IsLocked(ctx, category, targetID)
}

func createReadyPartyWithStage(
	ctx context.Context,
	t *testing.T,
	svc *Service,
	partyRepo *mockPartyRepository,
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
		Name:       "TestParty",
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

func TestPartyAdventure_DailyOnceLock_Lifecycle(t *testing.T) {
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

	// 1. 初回成功: Leader creates and starts party adventure for stage-26
	partyID := createReadyPartyWithStage(ctx, t, svc, partyRepo, charRepo, "stage-26", leader, member)

	res, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if err != nil {
		t.Fatalf("first StartPartyAdventure on stage-26 failed: %v", err)
	}
	if res.Outcome != "win" {
		t.Fatalf("expected outcome win, got %s", res.Outcome)
	}

	// Leader must have CategoryDungeonOnce locked
	leaderLocked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, leader.ID)
	if err != nil || !leaderLocked {
		t.Fatalf("expected leader to be locked after crawl, locked=%v, err=%v", leaderLocked, err)
	}

	// 2. 参加者のみの場合: Member must NOT have CategoryDungeonOnce locked
	memberLocked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, member.ID)
	if err != nil || memberLocked {
		t.Fatalf("expected member NOT to be locked, locked=%v, err=%v", memberLocked, err)
	}

	// 3. 再作成拒否: Leader attempts to create another party for stage-26 -> rejected with ErrDailyOnceAlreadyUsed
	_ = svc.DisbandParty(ctx, partyID, leader.ID)
	_, err = svc.CreateParty(ctx, leader.ID, CreatePartyRequest{
		Name:    "RetryParty",
		StageID: "stage-26",
	})
	if !errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
		t.Fatalf("expected ErrDailyOnceAlreadyUsed on re-create by leader, got: %v", err)
	}

	// Member CAN create a party for stage-26
	memberPartyID := createReadyPartyWithStage(ctx, t, svc, partyRepo, charRepo, "stage-26", member)
	if memberPartyID == "" {
		t.Fatal("expected member party creation on stage-26 to succeed")
	}

	// 4. 起床後許可: Releasing leader lock (simulating wake reset at Home)
	if err := timerSvc.ReleaseLock(ctx, timer.CategoryDungeonOnce, leader.ID); err != nil {
		t.Fatal(err)
	}

	// Leader can now create party for stage-26 again
	_ = svc.DisbandParty(ctx, memberPartyID, member.ID)
	newPartyID := createReadyPartyWithStage(ctx, t, svc, partyRepo, charRepo, "stage-26", leader)
	if newPartyID == "" {
		t.Fatal("expected leader party creation to succeed after lock release")
	}
}

func TestPartyAdventure_DailyOnceLock_StorageFailurePropagates(t *testing.T) {
	ctx := context.Background()
	injectedErr := errors.New("valkey admission storage unavailable")
	mockTimer := &mockAdmissionTimer{
		Service:    timer.NewService(nil),
		tryLockErr: injectedErr,
	}

	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(mockTimer))

	leader, err := corecharacter.New("FailLead")
	if err != nil {
		t.Fatal(err)
	}
	leader.Level = 99
	leader.Stats.HP = 500
	leader.Stats.MaxHP = 500

	partyID := createReadyPartyWithStage(ctx, t, svc, partyRepo, charRepo, "stage-26", leader)

	_, err = svc.StartPartyAdventure(ctx, partyID, leader.ID)
	if !errors.Is(err, injectedErr) {
		t.Fatalf("expected injected error %v, got: %v", injectedErr, err)
	}

	// Party status must remain recruiting (compensatory rollback)
	p, err := partyRepo.GetParty(ctx, partyID)
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != StatusRecruiting {
		t.Fatalf("expected status recruiting after storage failure, got: %s", p.Status)
	}
}

func TestPartyAdventure_DailyOnceLock_ConcurrentSingleConsumption(t *testing.T) {
	ctx := context.Background()
	timerSvc := timer.NewService(nil)
	svc, partyRepo, charRepo, _, _ := setupPartyAdventureTestService(t, nil, WithTimerService(timerSvc))

	leader, err := corecharacter.New("RaceLead")
	if err != nil {
		t.Fatal(err)
	}
	leader.Level = 99
	leader.Stats.HP = 500
	leader.Stats.MaxHP = 500

	partyID := createReadyPartyWithStage(ctx, t, svc, partyRepo, charRepo, "stage-26", leader)

	const concurrency = 10
	var successCount atomic.Int32
	var rejectedCount atomic.Int32
	var otherCount atomic.Int32
	var wg sync.WaitGroup
	startBarrier := make(chan struct{})

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-startBarrier
			_, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err == nil {
				successCount.Add(1)
			} else if errors.Is(err, adventure.ErrDailyOnceAlreadyUsed) {
				rejectedCount.Add(1)
			} else {
				otherCount.Add(1)
			}
		}()
	}

	close(startBarrier)
	wg.Wait()

	if successCount.Load() != 1 {
		t.Fatalf("expected exactly 1 successful start, got %d (other=%d, rejected=%d)", successCount.Load(), otherCount.Load(), rejectedCount.Load())
	}

	// Leader must be locked exactly once
	locked, err := timerSvc.IsLocked(ctx, timer.CategoryDungeonOnce, leader.ID)
	if err != nil || !locked {
		t.Fatalf("expected leader to be locked once, got %v", locked)
	}
}
