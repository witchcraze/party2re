package contest_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type injectableCharRepo struct {
	mu            sync.Mutex
	characters    map[string]corecharacter.Character
	updateFailIDs map[string]bool
	updateErr     error
	findFailIDs   map[string]bool
	findErr       error
}

func newInjectableCharRepo() *injectableCharRepo {
	return &injectableCharRepo{
		characters:    make(map[string]corecharacter.Character),
		updateFailIDs: make(map[string]bool),
		findFailIDs:   make(map[string]bool),
	}
}

func (m *injectableCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.findFailIDs[id] && m.findErr != nil {
		return corecharacter.Character{}, m.findErr
	}
	char, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return char, nil
}

func (m *injectableCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *injectableCharRepo) Update(_ context.Context, char corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.updateFailIDs[char.ID] && m.updateErr != nil {
		return m.updateErr
	}
	m.characters[char.ID] = char
	return nil
}

type trackingTxProvider struct {
	rollbackCalled bool
}

func (p *trackingTxProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	err := fn(ctx)
	if err != nil {
		p.rollbackCalled = true
		return err
	}
	return nil
}

func setupContestForSettlement(
	t *testing.T,
	charRepo *injectableCharRepo,
	contestRepo *mockContestRepo,
	now time.Time,
) {
	t.Helper()
	activeRound := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	}
	_ = contestRepo.SaveRound(context.Background(), activeRound)

	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-%d", i)
		char := corecharacter.Character{
			ID:    charID,
			Name:  fmt.Sprintf("Hero%d", i),
			Money: 1000,
		}
		_ = charRepo.Update(context.Background(), char)

		entry := contest.ContestEntry{
			ID:          fmt.Sprintf("entry-%d", i),
			Round:       1,
			CharacterID: charID,
			Title:       fmt.Sprintf("Photo %d", i),
			Votes:       10 - i, // Winner is entry-1 (9 votes)
			CreatedAt:   now.Add(-5 * 24 * time.Hour),
		}
		_ = contestRepo.SaveEntry(context.Background(), entry)
	}
}

func TestSettleContest_WinnerUpdateErrorPropagates(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()
	txProvider := &trackingTxProvider{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Inject error when saving 1st-place winner ("char-1")
	charRepo.updateFailIDs["char-1"] = true
	charRepo.updateErr = errors.New("database connection reset on winner update")

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.SettleContest(context.Background(), false)
	if err == nil {
		t.Fatal("expected error when winner update fails, got nil")
	}
	if !strings.Contains(err.Error(), "database connection reset on winner update") {
		t.Fatalf("expected injected error, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}

	// Verify active round was not committed as settled
	round, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	if round.Status == contest.StatusSettled {
		t.Errorf("expected round status NOT to be settled on failure, got %v", round.Status)
	}
}

func TestSettleContest_VoterUpdateErrorPropagates(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()
	txProvider := &trackingTxProvider{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Add voter who voted for 1st-place entry ("entry-1")
	voterChar := corecharacter.Character{
		ID:    "voter-1",
		Name:  "Voter One",
		Money: 500,
	}
	_ = charRepo.Update(context.Background(), voterChar)

	vote := contest.ContestVote{
		ID:               "vote-1",
		Round:            1,
		EntryID:          "entry-1",
		VoterCharacterID: "voter-1",
		CreatedAt:        now.Add(-2 * 24 * time.Hour),
	}
	_ = contestRepo.SaveVote(context.Background(), vote)

	// Inject error when saving voter
	charRepo.updateFailIDs["voter-1"] = true
	charRepo.updateErr = errors.New("database deadlock on voter medal award")

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.SettleContest(context.Background(), false)
	if err == nil {
		t.Fatal("expected error when voter update fails, got nil")
	}
	if !strings.Contains(err.Error(), "database deadlock on voter medal award") {
		t.Fatalf("expected injected error, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}
}

func TestSettleContest_DeletedCharacterIsSkippedGracefully(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Simulate deleted 2nd-place contestant (char-2 removed from repo)
	charRepo.mu.Lock()
	delete(charRepo.characters, "char-2")
	charRepo.mu.Unlock()

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	res, err := svc.SettleContest(context.Background(), false)
	if err != nil {
		t.Fatalf("expected settlement to succeed even if a contestant was deleted, got %v", err)
	}

	if !res.PrizesDistributed {
		t.Errorf("expected prizes to be distributed")
	}

	// 1st place winner still received prizes
	winner, _ := charRepo.FindByID(context.Background(), "char-1")
	if winner.Money != 1000+contest.PrizeFirst.Gold {
		t.Errorf("expected winner money %d, got %d", 1000+contest.PrizeFirst.Gold, winner.Money)
	}
}

func TestCL07AuditFreshContestStartsFromPreparingEntries(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	ctx := context.Background()

	// Register 5 different characters with photos and submit to contest
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("Player%d", i)})

		photo, err := svc.SavePhoto(ctx, charID, fmt.Sprintf("MyPhoto%d", i), "town", "http://example.com/p.jpg", "caption", "")
		if err != nil {
			t.Fatalf("SavePhoto failed for %s: %v", charID, err)
		}

		_, err = svc.EnterContest(ctx, charID, photo.ID, fmt.Sprintf("EntryTitle%d", i))
		if err != nil {
			t.Fatalf("EnterContest failed for %s: %v", charID, err)
		}
	}

	// SettleContest(force=true) on fresh contest
	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("expected fresh contest with 5 preparing entries to start, got error: %v", err)
	}

	if res.Postponed {
		t.Errorf("expected contest to NOT be postponed, got %+v", res)
	}
	if res.Round != 1 {
		t.Errorf("expected round 1, got %d", res.Round)
	}
	if res.NextRound != 1 {
		t.Errorf("expected next active round to be 1, got %d", res.NextRound)
	}
	if res.PrizesDistributed {
		t.Error("expected prizes NOT to be distributed upon starting the round")
	}

	// Round 1 is now active, Round 2 is preparing
	r1, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil || r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to be active, got status %q (err: %v)", r1.Status, err)
	}
	r2, err := contestRepo.GetRoundByNumber(ctx, 2)
	if err != nil || r2.Status != contest.StatusPreparing {
		t.Errorf("expected round 2 to be preparing, got status %q (err: %v)", r2.Status, err)
	}

	// Verify entries in active round 1 are available for voting
	entries, err := svc.GetCurrentEntries(ctx)
	if err != nil || len(entries) != 5 {
		t.Errorf("expected 5 entries in active round 1, got %d (err: %v)", len(entries), err)
	}
}

func TestCL07AuditEmptyActiveIgnoresPreparingEntries_Fixed(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	ctx := context.Background()

	// Set up Round 1 as active with 0 entries (empty active round)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})

	// Set up Round 2 as preparing with 5 entries
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})

	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("Player%d", i)})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-r2-%d", i),
			Round:       2,
			CharacterID: charID,
			Title:       fmt.Sprintf("Title %d", i),
		})
	}

	// SettleContest should recover from empty active round and promote Round 2 to active
	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}

	if res.Postponed {
		t.Errorf("expected contest not to be postponed when preparing has 5 entries, got %+v", res)
	}
	if res.NextRound != 2 {
		t.Errorf("expected NextRound to be 2, got %d", res.NextRound)
	}

	// Round 1 should be settled (without prizes)
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 settled, got %s", r1.Status)
	}

	// Round 2 should be active with its 5 entries
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 active, got %s", r2.Status)
	}

	// Round 3 should be preparing
	r3, _ := contestRepo.GetRoundByNumber(ctx, 3)
	if r3.Status != contest.StatusPreparing {
		t.Errorf("expected round 3 preparing, got %s", r3.Status)
	}
}

func TestPreparingEntriesRetainedWhenUnderQuorum(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	ctx := context.Background()

	// 4 characters enter preparing Round 1 (< 5 entries)
	for i := 1; i <= 4; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("Player%d", i)})

		photo, _ := svc.SavePhoto(ctx, charID, fmt.Sprintf("Photo%d", i), "town", "http://example.com/p.jpg", "caption", "")
		_, err := svc.EnterContest(ctx, charID, photo.ID, fmt.Sprintf("Entry%d", i))
		if err != nil {
			t.Fatalf("EnterContest failed: %v", err)
		}
	}

	// SettleContest(force=true) should postpone and retain the 4 entries
	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}
	if !res.Postponed {
		t.Errorf("expected contest to be postponed under quorum (4 entries), got %+v", res)
	}

	// Verify Round 1 is still preparing and deadline is extended
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusPreparing {
		t.Errorf("expected round 1 still preparing, got %s", r1.Status)
	}
	if !r1.EndTime.After(now) {
		t.Errorf("expected round 1 end time extended, got %v", r1.EndTime)
	}

	// Verify 4 entries still retained in Round 1
	entries, _ := contestRepo.ListEntriesByRound(ctx, 1)
	if len(entries) != 4 {
		t.Errorf("expected 4 entries retained, got %d", len(entries))
	}

	// 5th character enters Round 1
	char5ID := "char-5"
	charRepo.Update(ctx, corecharacter.Character{ID: char5ID, Name: "Player5"})
	photo5, _ := svc.SavePhoto(ctx, char5ID, "Photo5", "town", "http://example.com/p.jpg", "caption", "")
	_, err = svc.EnterContest(ctx, char5ID, photo5.ID, "Entry5")
	if err != nil {
		t.Fatalf("EnterContest 5 failed: %v", err)
	}

	// Now SettleContest should promote Round 1 to active
	res2, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest after 5th entry failed: %v", err)
	}
	if res2.Postponed {
		t.Errorf("expected round 1 to start with 5 entries, got postponed: %+v", res2)
	}

	r1Updated, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1Updated.Status != contest.StatusActive {
		t.Errorf("expected round 1 active, got %s", r1Updated.Status)
	}
}

func TestEmptyActiveRound_UnderQuorumPostpones(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)

	ctx := context.Background()

	// Active round with 0 entries
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})

	// Preparing round with only 3 entries
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 3; i++ {
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-%d", i),
			Round:       2,
			CharacterID: fmt.Sprintf("char-%d", i),
			Title:       fmt.Sprintf("Title %d", i),
		})
	}

	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}
	if !res.Postponed {
		t.Errorf("expected postponement, got %+v", res)
	}

	// Submissions in Round 2 should be preserved
	r2Entries, _ := contestRepo.ListEntriesByRound(ctx, 2)
	if len(r2Entries) != 3 {
		t.Errorf("expected 3 entries preserved in round 2, got %d", len(r2Entries))
	}
}

func TestActiveRoundSettlement_RetainsUnderQuorumPreparingEntries(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)

	ctx := context.Background()

	// Round 1 is active with 5 entries
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("P%d", i)})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("e1-%d", i),
			Round:       1,
			CharacterID: charID,
			Title:       fmt.Sprintf("R1 Title %d", i),
			Votes:       10 - i,
		})
	}

	// Round 2 is preparing with 4 entries (< 5)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 4; i++ {
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("e2-%d", i),
			Round:       2,
			CharacterID: fmt.Sprintf("char-next-%d", i),
			Title:       fmt.Sprintf("R2 Title %d", i),
		})
	}

	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}
	if !res.PrizesDistributed {
		t.Error("expected Round 1 prizes to be distributed")
	}

	// Round 1 settled
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 settled, got %s", r1.Status)
	}

	// Round 2 stays preparing because 4 < 5
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusPreparing {
		t.Errorf("expected round 2 to stay preparing (4 entries retained), got %s", r2.Status)
	}
	entriesR2, _ := contestRepo.ListEntriesByRound(ctx, 2)
	if len(entriesR2) != 4 {
		t.Errorf("expected 4 entries retained in round 2, got %d", len(entriesR2))
	}
}

func TestSchedulerRegisteredOnEnterContest(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()
	sched := &mockScheduler{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, _ := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithServiceScheduler(sched),
		contest.WithNowFunc(func() time.Time { return now }),
	)

	ctx := context.Background()

	charRepo.Update(ctx, corecharacter.Character{ID: "char-1", Name: "Player1"})
	photo, _ := svc.SavePhoto(ctx, "char-1", "Photo1", "town", "http://example.com/p.jpg", "caption", "")

	_, err := svc.EnterContest(ctx, "char-1", photo.ID, "Title1")
	if err != nil {
		t.Fatalf("EnterContest failed: %v", err)
	}

	// Verify settlement check was scheduled
	if len(sched.scheduledActions) != 1 {
		t.Fatalf("expected 1 scheduled action after EnterContest, got %d", len(sched.scheduledActions))
	}
	action := sched.scheduledActions[0]
	if action.ActionType != contest.ActionTypeContestSettlement {
		t.Errorf("expected ActionType %s, got %s", contest.ActionTypeContestSettlement, action.ActionType)
	}
}
