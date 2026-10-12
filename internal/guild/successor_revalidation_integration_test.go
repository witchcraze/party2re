package guild_test

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// gateGuildRepository intercepts GetGuildForUpdate to allow deterministic
// execution of interleaved operations between succession evaluation and disbandment.
type gateGuildRepository struct {
	*database.GuildRepository
	getGuildForUpdateHook func(ctx context.Context, guildID string, callCount int)
	beforeDisbandHook     func(ctx context.Context, guildID string)
	callCount             atomic.Int64
}

func (r *gateGuildRepository) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	c := int(r.callCount.Add(1))
	if r.getGuildForUpdateHook != nil {
		r.getGuildForUpdateHook(ctx, guildID, c)
	}
	return r.GuildRepository.GetGuildForUpdate(ctx, guildID)
}

func (r *gateGuildRepository) DisbandGuild(ctx context.Context, guildID string) error {
	if r.beforeDisbandHook != nil {
		r.beforeDisbandHook(ctx, guildID)
	}
	return r.GuildRepository.DisbandGuild(ctx, guildID)
}

func TestGuildSuccession_RevalidateSuccessor_ApprovedDuringDeparture(t *testing.T) {
	// Reproduction & Acceptance Criteria #1:
	// Guild A leader L, Guild B leader LB, character C.
	// C is pending in A and active in B.
	// L calls Leave(A, L). C is initially judged ineligible (active in B).
	// Before Step 3 executes GetGuildForUpdate(A), C leaves B and is approved in A as formal member.
	// Leave resumes: must NOT disband A; must promote C to leader and remove L only.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LRevA", 20000)
	leaderB := f.createCharacter(t, "LRevB", 20000)
	candidate := f.createCharacter(t, "CRev", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "RevGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "RevGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// C applies to A (pending)
	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gA: %v", err)
	}
	// C applies to B and is approved (active in B)
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gB: %v", err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatalf("approve in gB: %v", err)
	}

	if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
		t.Fatalf("candidate active affiliation before test = %d, want 1", count)
	}

	hookExecuted := false
	gateRepo := &gateGuildRepository{GuildRepository: f.repo}
	gateRepo.getGuildForUpdateHook = func(hookCtx context.Context, guildID string, callCount int) {
		if guildID != gA.ID {
			return
		}
		// callCount == 1: Step 2 evaluating candidate (candidate is active in B, rolls back)
		// callCount == 2: Step 3 about to lock guild for disbandment!
		if callCount == 2 && !hookExecuted {
			hookExecuted = true
			// Interleaved operation: C leaves B, L approves C in A as formal member
			if err := f.service.Leave(ctx, gB.ID, candidate.ID); err != nil {
				t.Errorf("interleaved Leave(B, C) failed: %v", err)
			}
			if err := f.service.ApproveApplication(ctx, gA.ID, leaderA.ID, candidate.ID, "member"); err != nil {
				t.Errorf("interleaved ApproveApplication(A, L, C) failed: %v", err)
			}
			if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
				t.Errorf("candidate active affiliation after approval = %d, want 1", count)
			}
		}
	}

	gateService, err := guild.NewService(gateRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	if err := gateService.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	if !hookExecuted {
		t.Fatal("gate hook was not executed")
	}

	// Guild A must survive with candidate as leader
	detailA, err := f.service.Get(ctx, gA.ID)
	if err != nil {
		t.Fatalf("expected Guild A to survive, got error: %v", err)
	}
	if detailA.Guild.LeaderCharacterID != candidate.ID {
		t.Errorf("Guild A leader = %s, want candidate %s", detailA.Guild.LeaderCharacterID, candidate.ID)
	}

	// candidate must have exactly 1 active affiliation (Guild A)
	if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
		t.Errorf("candidate active affiliation count = %d, want 1", count)
	}

	// leaderA must have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}
}

func TestGuildSuccession_RevalidateSuccessor_LeftOtherGuildDuringDeparture(t *testing.T) {
	// Acceptance Criteria #2:
	// C is pending in A, active in B.
	// L calls Leave(A, L). C is initially judged ineligible (active in B).
	// Before Step 3 disbandment, C leaves B only (now pending in A with 0 active guilds).
	// Leave resumes: must NOT disband A; must promote pending C to leader and remove L only.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LLeftA", 20000)
	leaderB := f.createCharacter(t, "LLeftB", 20000)
	candidate := f.createCharacter(t, "CLeft", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "LeftGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "LeftGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// C applies to A (pending)
	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gA: %v", err)
	}
	// C applies to B and is approved (active in B)
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gB: %v", err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatalf("approve in gB: %v", err)
	}

	hookExecuted := false
	gateRepo := &gateGuildRepository{GuildRepository: f.repo}
	gateRepo.getGuildForUpdateHook = func(hookCtx context.Context, guildID string, callCount int) {
		if guildID != gA.ID {
			return
		}
		if callCount == 2 && !hookExecuted {
			hookExecuted = true
			// Interleaved operation: C leaves B only (C is still pending in A, now 0 active affiliations)
			if err := f.service.Leave(ctx, gB.ID, candidate.ID); err != nil {
				t.Errorf("interleaved Leave(B, C) failed: %v", err)
			}
			if count := f.countActiveAffiliations(t, candidate.ID); count != 0 {
				t.Errorf("candidate active affiliation after Leave(B, C) = %d, want 0", count)
			}
		}
	}

	gateService, err := guild.NewService(gateRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	if err := gateService.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	if !hookExecuted {
		t.Fatal("gate hook was not executed")
	}

	// Guild A must survive with candidate as leader
	detailA, err := f.service.Get(ctx, gA.ID)
	if err != nil {
		t.Fatalf("expected Guild A to survive, got error: %v", err)
	}
	if detailA.Guild.LeaderCharacterID != candidate.ID {
		t.Errorf("Guild A leader = %s, want candidate %s", detailA.Guild.LeaderCharacterID, candidate.ID)
	}

	// candidate must have exactly 1 active affiliation (Guild A) and not pending
	if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
		t.Errorf("candidate active affiliation count = %d, want 1", count)
	}

	// leaderA must have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}
}

func TestGuildSuccession_RevalidateSuccessor_StillIneligibleDisbands(t *testing.T) {
	// Acceptance Criteria #3:
	// C remains in Guild B (all remaining members are still ineligible).
	// Guild A must cleanly disband.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LInelA", 20000)
	leaderB := f.createCharacter(t, "LInelB", 20000)
	candidate := f.createCharacter(t, "CInel", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "InelGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "InelGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// C applies to A (pending)
	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gA: %v", err)
	}
	// C applies to B and is approved (active in B)
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gB: %v", err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatalf("approve in gB: %v", err)
	}

	if err := f.service.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	// Guild A must be disbanded
	if _, err := f.service.Get(ctx, gA.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected Guild A to be disbanded, got: %v", err)
	}

	// candidate must still have exactly 1 active affiliation (Guild B)
	if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
		t.Errorf("candidate active affiliation count = %d, want 1", count)
	}

	// leaderA must have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}
}

func TestGuildSuccession_RevalidateSuccessor_SoleLeaderDisbands(t *testing.T) {
	// Acceptance Criteria #3:
	// Sole leader leaves -> cleanly disbands.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leader := f.createCharacter(t, "LSole", 20000)
	g, _, _, err := f.service.Create(ctx, leader.ID, "SoleG_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.Leave(ctx, g.ID, leader.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	if _, err := f.service.Get(ctx, g.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected guild to be disbanded, got: %v", err)
	}

	if count := f.countActiveAffiliations(t, leader.ID); count != 0 {
		t.Errorf("leader active affiliation count = %d, want 0", count)
	}
}

type errorInjectGateRepo struct {
	*gateGuildRepository
	failGetGuildByCharacter atomic.Bool
}

func (r *errorInjectGateRepo) GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
	if r.failGetGuildByCharacter.Load() {
		return guild.Guild{}, guild.Member{}, sql.ErrConnDone
	}
	return r.gateGuildRepository.GetGuildByCharacter(ctx, characterID)
}

func TestGuildSuccession_RevalidateSuccessor_DBErrorPropagation(t *testing.T) {
	// Acceptance Criteria #5:
	// Database error during GetGuildByCharacter in Step 3 must propagate and abort disbandment.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LErrA", 20000)
	leaderB := f.createCharacter(t, "LErrB", 20000)
	candidate := f.createCharacter(t, "CErr", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "ErrGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "ErrGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatal(err)
	}

	gateRepo := &gateGuildRepository{GuildRepository: f.repo}
	errRepo := &errorInjectGateRepo{gateGuildRepository: gateRepo}
	gateRepo.getGuildForUpdateHook = func(hookCtx context.Context, guildID string, callCount int) {
		if guildID == gA.ID && callCount == 2 {
			// Enable DB failure right before Step 3's candidate check
			errRepo.failGetGuildByCharacter.Store(true)
		}
	}

	composedService, err := guild.NewService(errRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	leaveErr := composedService.Leave(ctx, gA.ID, leaderA.ID)
	if !errors.Is(leaveErr, sql.ErrConnDone) {
		t.Fatalf("expected sql.ErrConnDone, got: %v", leaveErr)
	}

	// Guild A must NOT be disbanded
	if _, err := f.service.Get(ctx, gA.ID); err != nil {
		t.Errorf("Guild A should survive after DB error, got: %v", err)
	}
}

func TestGuildSuccession_RevalidateSuccessor_RemoveCharacterFromGuild(t *testing.T) {
	// Acceptance Criteria #6:
	// RemoveCharacterFromGuild shares the revalidation logic.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LRcvA", 20000)
	leaderB := f.createCharacter(t, "LRcvB", 20000)
	candidate := f.createCharacter(t, "CRcv", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "RcvGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "RcvGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatal(err)
	}

	hookExecuted := false
	gateRepo := &gateGuildRepository{GuildRepository: f.repo}
	gateRepo.getGuildForUpdateHook = func(hookCtx context.Context, guildID string, callCount int) {
		if guildID != gA.ID {
			return
		}
		if callCount == 2 && !hookExecuted {
			hookExecuted = true
			if err := f.service.Leave(ctx, gB.ID, candidate.ID); err != nil {
				t.Errorf("interleaved Leave(B, C) failed: %v", err)
			}
			if err := f.service.ApproveApplication(ctx, gA.ID, leaderA.ID, candidate.ID, "member"); err != nil {
				t.Errorf("interleaved ApproveApplication(A, L, C) failed: %v", err)
			}
		}
	}

	gateService, err := guild.NewService(gateRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	if err := gateService.RemoveCharacterFromGuild(ctx, leaderA.ID); err != nil {
		t.Fatalf("RemoveCharacterFromGuild failed: %v", err)
	}

	if !hookExecuted {
		t.Fatal("gate hook was not executed")
	}

	// Guild A survives with candidate as leader
	detailA, err := f.service.Get(ctx, gA.ID)
	if err != nil {
		t.Fatalf("expected Guild A to survive, got error: %v", err)
	}
	if detailA.Guild.LeaderCharacterID != candidate.ID {
		t.Errorf("Guild A leader = %s, want candidate %s", detailA.Guild.LeaderCharacterID, candidate.ID)
	}

	if count := f.countActiveAffiliations(t, candidate.ID); count != 1 {
		t.Errorf("candidate active affiliation count = %d, want 1", count)
	}
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}
}

func TestGuildSuccession_RevalidateSuccessor_ConcurrentApprovalRace(t *testing.T) {
	// Acceptance Criteria #7:
	// Concurrency stress race between Leader leaving Guild A and
	// candidate leaving Guild B & getting approved in Guild A.
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	leaderA := f.createCharacter(t, "LRaceA", 20000)
	leaderB := f.createCharacter(t, "LRaceB", 20000)
	candidate := f.createCharacter(t, "CRace", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "RaceGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "RaceGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatal(err)
	}

	// Concurrent race:
	// Op 1: Leader A leaves Guild A
	// Op 2: Candidate leaves Guild B and Leader A approves Candidate in Guild A
	errLeaveA, errLeaveBApprove := testutil.RunRace2(
		func() error {
			return f.service.Leave(ctx, gA.ID, leaderA.ID)
		},
		func() error {
			if err := f.service.Leave(ctx, gB.ID, candidate.ID); err != nil {
				return err
			}
			err := f.service.ApproveApplication(ctx, gA.ID, leaderA.ID, candidate.ID, "member")
			if err != nil && (errors.Is(err, guild.ErrGuildNotFound) || errors.Is(err, guild.ErrUnauthorized)) {
				// Guild A already disbanded or leaderA already left
				return nil
			}
			return err
		},
	)

	if errLeaveA != nil {
		t.Fatalf("unexpected error from Leave(A): %v", errLeaveA)
	}
	if errLeaveBApprove != nil {
		t.Fatalf("unexpected error from Leave(B)+Approve: %v", errLeaveBApprove)
	}

	// Invariants:
	// leaderA must have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}

	// candidate must have AT MOST 1 active affiliation (either 1 if in A, or 0 if A disbanded)
	candAffiliations := f.countActiveAffiliations(t, candidate.ID)
	if candAffiliations > 1 {
		t.Errorf("candidate active affiliations = %d, want <= 1", candAffiliations)
	}

	// Check Guild A state consistency:
	detailA, err := f.service.Get(ctx, gA.ID)
	if err == nil {
		// If Guild A survived, candidate must be leader of Guild A and have 1 active affiliation!
		if detailA.Guild.LeaderCharacterID != candidate.ID {
			t.Errorf("Guild A leader = %s, want candidate %s", detailA.Guild.LeaderCharacterID, candidate.ID)
		}
		if candAffiliations != 1 {
			t.Errorf("candidate active affiliations = %d, want 1 since Guild A survived", candAffiliations)
		}
	} else if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Fatalf("unexpected error getting Guild A: %v", err)
	}
}

func TestGuildSuccession_RevalidateSuccessor_CandidateLeavesOtherGuildPostRevalidation(t *testing.T) {
	// Linearizability & Invariant Verification:
	// Candidate C is pending in Guild A, active in Guild B.
	// Leader L leaves Guild A.
	// Step 2 evaluates C: C is active in B -> errCandidateIneligible.
	// Step 3 revalidates C under Rank 7 lock: GetGuildByCharacter(C) sees C is still active in B.
	// Post-revalidation & immediately before DisbandGuild(A), C leaves Guild B on an independent transaction.
	// DisbandGuild(A) then proceeds and commits.
	//
	// Verification targets:
	// 1. Transaction linearizability: Equivalent to Disband(A) [when C was in B] -> Leave(B, C).
	// 2. Strict invariant preservation:
	//    - Active affiliation count for all characters: C has 0, L has 0 (conserved <= 1).
	//    - Guild A is disbanded cleanly (no orphaned records or leaderless guild).
	//    - Guild B remains intact with leader LB.
	//    - No deadlocks, lock order inversions, or integrity errors.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LPostA", 20000)
	leaderB := f.createCharacter(t, "LPostB", 20000)
	candidate := f.createCharacter(t, "CPost", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "PostGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "PostGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// C applies to A (pending)
	if err := f.service.ApplyToJoin(ctx, gA.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gA: %v", err)
	}
	// C applies to B and is approved (active in B)
	if err := f.service.ApplyToJoin(ctx, gB.ID, candidate.ID); err != nil {
		t.Fatalf("apply to gB: %v", err)
	}
	if err := f.service.ApproveApplication(ctx, gB.ID, leaderB.ID, candidate.ID, "member"); err != nil {
		t.Fatalf("approve in gB: %v", err)
	}

	disbandHookFired := false
	gateRepo := &gateGuildRepository{GuildRepository: f.repo}
	gateRepo.beforeDisbandHook = func(hookCtx context.Context, guildID string) {
		if guildID == gA.ID {
			disbandHookFired = true
			// Interleaved operation immediately before DisbandGuild execution:
			// C leaves Guild B on an independent transaction.
			if err := f.service.Leave(ctx, gB.ID, candidate.ID); err != nil {
				t.Errorf("interleaved Leave(B, C) failed: %v", err)
			}
			// At this point, C has 0 active affiliations.
			if count := f.countActiveAffiliations(t, candidate.ID); count != 0 {
				t.Errorf("candidate active affiliations after Leave(B, C) = %d, want 0", count)
			}
		}
	}

	gateService, err := guild.NewService(gateRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	if err := gateService.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("Leave(A) failed: %v", err)
	}

	if !disbandHookFired {
		t.Fatal("beforeDisbandHook was not fired")
	}

	// Guild A must be disbanded
	if _, err := f.service.Get(ctx, gA.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected Guild A to be disbanded, got: %v", err)
	}

	// Guild B must survive
	if _, err := f.service.Get(ctx, gB.ID); err != nil {
		t.Errorf("expected Guild B to survive, got: %v", err)
	}

	// Invariants:
	// leaderA has 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}

	// candidate has 0 active affiliations (0 <= 1)
	if count := f.countActiveAffiliations(t, candidate.ID); count != 0 {
		t.Errorf("candidate active affiliation count = %d, want 0", count)
	}
}
