package guild_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

func setupIneligibleCandidates(t *testing.T, f *activeAffiliationFixture, ctx context.Context, gAID, gBID, leaderBID string, count int) []string {
	t.Helper()
	ineligibleChars := make([]string, count)
	for i := 0; i < count; i++ {
		char := f.createCharacter(t, fmt.Sprintf("Inel_%d", i), 5000)
		ineligibleChars[i] = char.ID

		if err := f.service.ApplyToJoin(ctx, gAID, char.ID); err != nil {
			t.Fatalf("apply to gA [%d]: %v", i, err)
		}
		if err := f.service.ApplyToJoin(ctx, gBID, char.ID); err != nil {
			t.Fatalf("apply to gB [%d]: %v", i, err)
		}
		if err := f.service.ApproveApplication(ctx, gBID, leaderBID, char.ID, "member"); err != nil {
			t.Fatalf("approve in gB [%d]: %v", i, err)
		}
	}
	return ineligibleChars
}

func TestGuildSuccession_ExceedsLeaveRetryBudget_Disband(t *testing.T) {
	// Verify that scanning beyond 50 ineligible candidates (51 candidates)
	// cleanly evaluates all candidates and disbands the guild without hitting retry errors.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LScanA", 20000)
	leaderB := f.createCharacter(t, "LScanB", 20000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "ScanGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "ScanGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	const ineligibleCount = 51
	ineligibleChars := setupIneligibleCandidates(t, f, ctx, gA.ID, gB.ID, leaderB.ID, ineligibleCount)

	// Attempt leader departure from gA.
	// All 51 candidates are pending in gA but active in gB (ineligible).
	// gA should cleanly disband and leaderA should leave without error.
	if err := f.service.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("expected Leave to succeed and disband gA, got err: %v", err)
	}

	// gA should be disbanded
	if _, err := f.service.Get(ctx, gA.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected gA to be disbanded (ErrGuildNotFound), got: %v", err)
	}

	// leaderA should have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}

	// All ineligible characters must still have exactly 1 active affiliation (gB)
	for i, charID := range ineligibleChars {
		if count := f.countActiveAffiliations(t, charID); count != 1 {
			t.Errorf("char [%d] active affiliation count = %d, want 1", i, count)
		}
	}
}

func TestGuildSuccession_ExceedsLeaveRetryBudget_Succession(t *testing.T) {
	// Verify that when >50 ineligible candidates are followed by an eligible candidate,
	// all ineligible candidates are scanned and leadership successfully transfers to the eligible candidate.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LSuccA", 20000)
	leaderB := f.createCharacter(t, "LSuccB", 20000)
	eligibleCandidate := f.createCharacter(t, "Eligible", 5000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "SuccGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "SuccGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	const ineligibleCount = 50
	ineligibleChars := setupIneligibleCandidates(t, f, ctx, gA.ID, gB.ID, leaderB.ID, ineligibleCount)

	// Eligible candidate applies to gA only (has no other guild affiliation)
	if err := f.service.ApplyToJoin(ctx, gA.ID, eligibleCandidate.ID); err != nil {
		t.Fatalf("apply eligible candidate: %v", err)
	}

	// Leader leaves gA. Should scan through 50 ineligible candidates and promote eligible candidate.
	if err := f.service.Leave(ctx, gA.ID, leaderA.ID); err != nil {
		t.Fatalf("expected Leave to succeed, got: %v", err)
	}

	// gA must survive with eligibleCandidate as the new leader
	detail, err := f.service.Get(ctx, gA.ID)
	if err != nil {
		t.Fatalf("expected gA to survive, got err: %v", err)
	}
	if detail.Guild.LeaderCharacterID != eligibleCandidate.ID {
		t.Errorf("gA leader = %s, want %s", detail.Guild.LeaderCharacterID, eligibleCandidate.ID)
	}

	// eligibleCandidate must have exactly 1 active affiliation (gA)
	if count := f.countActiveAffiliations(t, eligibleCandidate.ID); count != 1 {
		t.Errorf("eligibleCandidate active affiliation count = %d, want 1", count)
	}

	// leaderA must have 0 active affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}

	// All 50 ineligible characters must still have exactly 1 active affiliation (gB)
	for i, charID := range ineligibleChars {
		if count := f.countActiveAffiliations(t, charID); count != 1 {
			t.Errorf("char [%d] active affiliation count = %d, want 1", i, count)
		}
	}
}

func TestGuildSuccession_ExceedsLeaveRetryBudget_RemoveCharacterFromGuild(t *testing.T) {
	// Verify that RemoveCharacterFromGuild shares the same fix and disbands without hitting retry limit.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LRmA", 20000)
	leaderB := f.createCharacter(t, "LRmB", 20000)

	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "RmGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	gB, _, _, err := f.service.Create(ctx, leaderB.ID, "RmGB_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	const ineligibleCount = 50
	ineligibleChars := setupIneligibleCandidates(t, f, ctx, gA.ID, gB.ID, leaderB.ID, ineligibleCount)

	if err := f.service.RemoveCharacterFromGuild(ctx, leaderA.ID); err != nil {
		t.Fatalf("expected RemoveCharacterFromGuild to succeed, got err: %v", err)
	}

	// gA should be disbanded
	if _, err := f.service.Get(ctx, gA.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected gA to be disbanded, got: %v", err)
	}

	// leaderA should have 0 affiliations
	if count := f.countActiveAffiliations(t, leaderA.ID); count != 0 {
		t.Errorf("leaderA active affiliation count = %d, want 0", count)
	}

	// All 50 ineligible characters still have 1 affiliation (gB)
	for i, charID := range ineligibleChars {
		if count := f.countActiveAffiliations(t, charID); count != 1 {
			t.Errorf("char [%d] active affiliation count = %d, want 1", i, count)
		}
	}
}

func TestGuildSuccession_ExceedsLeaveRetryBudget_ContextCancellation(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-canceled

	leaderA := f.createCharacter(t, "LCancelA", 20000)
	gA, _, _, err := f.service.Create(context.Background(), leaderA.ID, "CnGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	err = f.service.Leave(ctx, gA.ID, leaderA.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}

	// Guild must still exist
	if _, err := f.service.Get(context.Background(), gA.ID); err != nil {
		t.Errorf("guild should still exist after canceled Leave: %v", err)
	}
}

type rosterChangeTriggerRepo struct {
	*database.GuildRepository
	phantomCount atomic.Int64
}

func (r *rosterChangeTriggerRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err != nil {
		return g, members, err
	}
	// Inject a phantom member not in ineligible map to trigger errRosterChanged in Step 3
	c := r.phantomCount.Add(1)
	members = append(members, guild.Member{
		GuildID:     guildID,
		CharacterID: fmt.Sprintf("phantom_%d", c),
		Role:        guild.RoleMember,
		IsPending:   false,
	})
	return g, members, nil
}

func TestGuildSuccession_ExceedsLeaveRetryBudget_RetryLimitEnforcedOnRosterChange(t *testing.T) {
	// Verify that concurrency retry budget (maxLeaveAttempts = 50) is still enforced
	// when roster changes actually occur repeatedly.
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leaderA := f.createCharacter(t, "LRetryA", 20000)
	gA, _, _, err := f.service.Create(ctx, leaderA.ID, "RetryGA_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	customRepo := &rosterChangeTriggerRepo{GuildRepository: f.repo}
	customService, err := guild.NewService(customRepo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}

	err = customService.Leave(ctx, gA.ID, leaderA.ID)
	if err == nil || !strings.Contains(err.Error(), "guild leave exceeded maximum retry attempts") {
		t.Fatalf("expected retry budget exhaustion error, got: %v", err)
	}

	if customRepo.phantomCount.Load() < 50 {
		t.Errorf("expected at least 50 roster change injections, got: %d", customRepo.phantomCount.Load())
	}
}
