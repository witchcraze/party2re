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

func TestSettlement_ActiveUnderQuorumWithPreparingQuorum_AdvancesWithPrizesAndPromotesPreparing(t *testing.T) {
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

	// Round 1 is active with 3 entries (< 5 entries, under quorum)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})
	for i := 1; i <= 3; i++ {
		charID := fmt.Sprintf("char-active-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("ActivePlayer%d", i), Money: 1000})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-active-%d", i),
			Round:       1,
			CharacterID: charID,
			Title:       fmt.Sprintf("Active Title %d", i),
			Votes:       10 - i, // 1st place has 9 votes, 2nd has 8, 3rd has 7
		})
	}

	// Round 2 is preparing with 5 entries (>= 5 entries, reaches quorum)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-prep-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("PrepPlayer%d", i)})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-prep-%d", i),
			Round:       2,
			CharacterID: charID,
			Title:       fmt.Sprintf("Prep Title %d", i),
		})
	}

	// SettleContest: because preparing has 5 entries, Round 1 concludes with prizes
	// and Round 2 advances to active, preventing infinite postponement.
	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}

	if res.Postponed {
		t.Errorf("expected contest not to be postponed when preparing has quorum, got %+v", res)
	}
	if !res.PrizesDistributed {
		t.Error("expected prizes to be distributed to Round 1 entrants")
	}
	if res.NextRound != 2 {
		t.Errorf("expected NextRound 2, got %d", res.NextRound)
	}
	// Hall of Fame / Legend is omitted because active round had < 5 entries (matching legacy contest.cgi:186)
	if res.WinnerLegend != nil {
		t.Errorf("expected WinnerLegend to be nil when entries < 5, got %+v", res.WinnerLegend)
	}

	// Round 1 must be settled
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 settled, got %s", r1.Status)
	}

	// Entrants in Round 1 must have received their prizes
	winnerChar, _ := charRepo.FindByID(ctx, "char-active-1")
	if winnerChar.Money != 1000+15000 {
		t.Errorf("expected 1st place winner to receive 15000 gold, got %d", winnerChar.Money)
	}

	// Active round entries must be preserved (3 entries)
	r1Entries, _ := contestRepo.ListEntriesByRound(ctx, 1)
	if len(r1Entries) != 3 {
		t.Errorf("expected 3 active entries preserved, got %d", len(r1Entries))
	}

	// Round 2 must be promoted to active
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 active, got %s", r2.Status)
	}

	// Round 3 must be created as preparing
	r3, _ := contestRepo.GetRoundByNumber(ctx, 3)
	if r3.Status != contest.StatusPreparing {
		t.Errorf("expected round 3 preparing, got %s", r3.Status)
	}

	// Preparing round entries must be preserved in Round 2 (5 entries)
	r2Entries, _ := contestRepo.ListEntriesByRound(ctx, 2)
	if len(r2Entries) != 5 {
		t.Errorf("expected 5 preparing entries preserved in round 2, got %d", len(r2Entries))
	}
}

func TestSettlement_ActiveUnderQuorumWithPreparingUnderQuorum_PostponesAndRetainsEntries(t *testing.T) {
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

	// Round 1 is active with 3 entries (< 5 entries, under quorum)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})
	for i := 1; i <= 3; i++ {
		charID := fmt.Sprintf("char-active-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("ActivePlayer%d", i), Money: 1000})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-active-%d", i),
			Round:       1,
			CharacterID: charID,
			Title:       fmt.Sprintf("Active Title %d", i),
		})
	}

	// Round 2 is preparing with 4 entries (< 5 entries, under quorum)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 4; i++ {
		charID := fmt.Sprintf("char-prep-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("PrepPlayer%d", i)})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-prep-%d", i),
			Round:       2,
			CharacterID: charID,
			Title:       fmt.Sprintf("Prep Title %d", i),
		})
	}

	// SettleContest: both rounds under quorum -> postpone and retain submissions in both rounds
	res, err := svc.SettleContest(ctx, true)
	if err != nil {
		t.Fatalf("SettleContest failed: %v", err)
	}

	if !res.Postponed {
		t.Errorf("expected contest to be postponed when both active and preparing are under quorum, got %+v", res)
	}
	if res.PrizesDistributed {
		t.Error("expected prizes NOT to be distributed on postponement")
	}

	// Round 1 must remain active and extended
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to remain active, got %s", r1.Status)
	}
	if !r1.EndTime.After(now) {
		t.Errorf("expected round 1 end time extended, got %v", r1.EndTime)
	}

	// Round 1 entries preserved
	r1Entries, _ := contestRepo.ListEntriesByRound(ctx, 1)
	if len(r1Entries) != 3 {
		t.Errorf("expected 3 entries preserved in round 1, got %d", len(r1Entries))
	}

	// Round 2 must remain preparing and extended
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusPreparing {
		t.Errorf("expected round 2 to remain preparing, got %s", r2.Status)
	}
	if !r2.EndTime.Equal(r1.EndTime) {
		t.Errorf("expected round 2 end time synced to round 1 end time, got %v, want %v", r2.EndTime, r1.EndTime)
	}

	// Round 2 entries preserved
	r2Entries, _ := contestRepo.ListEntriesByRound(ctx, 2)
	if len(r2Entries) != 4 {
		t.Errorf("expected 4 entries preserved in round 2, got %d", len(r2Entries))
	}
}

func TestSettlement_GetPreparingRoundDBError_AbortsWithoutMutationsOrPrizes(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()
	txProvider := &trackingTxProvider{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Round 2 is preparing
	contestRepo.SaveRound(context.Background(), contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})

	initialRound1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	winnerCharInitial, _ := charRepo.FindByID(context.Background(), "char-1")

	// Inject DB error on GetPreparingRoundForUpdate
	injectedErr := errors.New("database connection failed during preparing round query")
	contestRepo.getPreparingRoundForUpdateErr = injectedErr

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
		t.Fatal("expected error when preparing round query fails, got nil")
	}
	if !errors.Is(err, injectedErr) && !strings.Contains(err.Error(), injectedErr.Error()) {
		t.Fatalf("expected injected error, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}

	// Active round must remain active with unchanged EndTime
	r1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	if r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to remain active, got %s", r1.Status)
	}
	if !r1.EndTime.Equal(initialRound1.EndTime) {
		t.Errorf("expected round 1 end time unchanged, got %v", r1.EndTime)
	}

	// Preparing round must remain preparing
	r2, _ := contestRepo.GetRoundByNumber(context.Background(), 2)
	if r2.Status != contest.StatusPreparing {
		t.Errorf("expected round 2 to remain preparing, got %s", r2.Status)
	}

	// No prizes awarded to winners
	winnerCharAfter, _ := charRepo.FindByID(context.Background(), "char-1")
	if winnerCharAfter.Money != winnerCharInitial.Money || winnerCharAfter.SmallMedals != winnerCharInitial.SmallMedals {
		t.Errorf("expected winner character not to receive prizes on failure, initial money: %d, after: %d", winnerCharInitial.Money, winnerCharAfter.Money)
	}
}

func TestSettlement_ListPreparingEntriesDBError_AbortsWithoutMutationsOrPrizes(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()
	txProvider := &trackingTxProvider{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Round 2 is preparing
	contestRepo.SaveRound(context.Background(), contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})

	initialRound1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	winnerCharInitial, _ := charRepo.FindByID(context.Background(), "char-1")

	// Inject DB error on ListEntriesByRound for round 2
	injectedErr := errors.New("disk I/O error listing preparing round entries")
	contestRepo.listEntriesByRoundErrByRound[2] = injectedErr

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
		t.Fatal("expected error when preparing entries query fails, got nil")
	}
	if !errors.Is(err, injectedErr) && !strings.Contains(err.Error(), injectedErr.Error()) {
		t.Fatalf("expected injected error, got: %v", err)
	}

	if !txProvider.rollbackCalled {
		t.Error("expected transaction rollback to be triggered")
	}

	// Active round must remain active with unchanged EndTime
	r1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	if r1.Status != contest.StatusActive {
		t.Errorf("expected round 1 to remain active, got %s", r1.Status)
	}
	if !r1.EndTime.Equal(initialRound1.EndTime) {
		t.Errorf("expected round 1 end time unchanged, got %v", r1.EndTime)
	}

	// No prizes awarded to winners
	winnerCharAfter, _ := charRepo.FindByID(context.Background(), "char-1")
	if winnerCharAfter.Money != winnerCharInitial.Money || winnerCharAfter.SmallMedals != winnerCharInitial.SmallMedals {
		t.Errorf("expected winner character not to receive prizes on failure, initial money: %d, after: %d", winnerCharInitial.Money, winnerCharAfter.Money)
	}
}

func TestSettlement_EmptyActiveRound_DBErrorAbortsWithoutMutations(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()
	txProvider := &trackingTxProvider{}

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)

	// Round 1 active with 0 entries
	contestRepo.SaveRound(context.Background(), contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now.Add(-11 * 24 * time.Hour),
		EndTime:   now.Add(-1 * time.Hour),
	})
	initialRound1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)

	injectedErr := errors.New("network timeout querying preparing round")
	contestRepo.getPreparingRoundForUpdateErr = injectedErr

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	_, err = svc.SettleContest(context.Background(), true)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), injectedErr.Error()) {
		t.Fatalf("expected injected error, got: %v", err)
	}
	if !txProvider.rollbackCalled {
		t.Error("expected rollback called")
	}

	r1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	if r1.Status != contest.StatusActive {
		t.Errorf("expected active round status unchanged, got %s", r1.Status)
	}
	if !r1.EndTime.Equal(initialRound1.EndTime) {
		t.Errorf("expected active round end time unchanged, got %v", r1.EndTime)
	}
}

func TestSettlement_HappyPathAndDuplicateExecution(t *testing.T) {
	charRepo := newInjectableCharRepo()
	contestRepo := newMockContestRepo()

	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	setupContestForSettlement(t, charRepo, contestRepo, now)

	// Round 2 is preparing with 5 entries
	contestRepo.SaveRound(context.Background(), contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: now.Add(-1 * time.Hour),
		EndTime:   now.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-p2-%d", i)
		charRepo.Update(context.Background(), corecharacter.Character{ID: charID, Name: fmt.Sprintf("P2-%d", i)})
		contestRepo.SaveEntry(context.Background(), contest.ContestEntry{
			ID:          fmt.Sprintf("entry-p2-%d", i),
			Round:       2,
			CharacterID: charID,
			Title:       fmt.Sprintf("Photo P2 %d", i),
		})
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	// 1. Happy path settlement
	res, err := svc.SettleContest(context.Background(), false)
	if err != nil {
		t.Fatalf("expected happy path settlement to succeed, got %v", err)
	}
	if !res.PrizesDistributed {
		t.Error("expected prizes to be distributed")
	}
	if res.NextRound != 2 {
		t.Errorf("expected next round 2, got %d", res.NextRound)
	}

	r1, _ := contestRepo.GetRoundByNumber(context.Background(), 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 settled, got %s", r1.Status)
	}

	r2, _ := contestRepo.GetRoundByNumber(context.Background(), 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 active, got %s", r2.Status)
	}

	r3, _ := contestRepo.GetRoundByNumber(context.Background(), 3)
	if r3.Status != contest.StatusPreparing {
		t.Errorf("expected round 3 preparing, got %s", r3.Status)
	}

	// 2. Immediate duplicate execution (force = false)
	// Round 2 end time is now in the future (now + 10 days)
	_, err = svc.SettleContest(context.Background(), false)
	if !errors.Is(err, contest.ErrContestNotReadyToSettle) {
		t.Errorf("expected ErrContestNotReadyToSettle on duplicate run, got: %v", err)
	}
}

func TestSettlement_MultipleConsecutiveCyclesAdvanceWithoutStallingOrDuplicateRewards(t *testing.T) {
	charRepo := newMockCharRepo()
	contestRepo := newMockContestRepo()

	currentTime := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithNowFunc(func() time.Time { return currentTime }),
	)
	if err != nil {
		t.Fatalf("failed to create contest service: %v", err)
	}

	ctx := context.Background()

	// Initial State:
	// Round 1 active with 3 entries (under quorum)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: currentTime.Add(-11 * 24 * time.Hour),
		EndTime:   currentTime.Add(-1 * time.Hour),
	})
	for i := 1; i <= 3; i++ {
		charID := fmt.Sprintf("char-r1-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("PlayerR1_%d", i), Money: 1000})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-r1-%d", i),
			Round:       1,
			CharacterID: charID,
			Title:       fmt.Sprintf("Title R1 %d", i),
			Votes:       10 - i,
		})
	}

	// Round 2 preparing with 5 entries (quorum met)
	contestRepo.SaveRound(ctx, contest.ContestRound{
		Round:     2,
		Status:    contest.StatusPreparing,
		StartTime: currentTime.Add(-1 * time.Hour),
		EndTime:   currentTime.Add(9 * 24 * time.Hour),
	})
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-r2-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("PlayerR2_%d", i), Money: 2000})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-r2-%d", i),
			Round:       2,
			CharacterID: charID,
			Title:       fmt.Sprintf("Title R2 %d", i),
			Votes:       15 - i,
		})
	}

	// --- CYCLE 1 SETTLEMENT ---
	res1, err := svc.SettleContest(ctx, false)
	if err != nil {
		t.Fatalf("Cycle 1 settlement failed: %v", err)
	}
	if res1.Postponed {
		t.Fatal("expected cycle 1 not to be postponed")
	}
	if !res1.PrizesDistributed {
		t.Fatal("expected cycle 1 prizes to be distributed")
	}
	if res1.Round != 1 || res1.NextRound != 2 {
		t.Fatalf("expected cycle 1 settled round 1, next round 2; got round %d, next %d", res1.Round, res1.NextRound)
	}

	// Verify Round 1 winner received prize
	w1, _ := charRepo.FindByID(ctx, "char-r1-1")
	if w1.Money != 1000+15000 {
		t.Errorf("expected R1 winner to have 16000 money, got %d", w1.Money)
	}

	// Verify Round 1 is settled, Round 2 is active
	r1, _ := contestRepo.GetRoundByNumber(ctx, 1)
	if r1.Status != contest.StatusSettled {
		t.Errorf("expected round 1 settled, got %s", r1.Status)
	}
	r2, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2.Status != contest.StatusActive {
		t.Errorf("expected round 2 active, got %s", r2.Status)
	}
	r3, _ := contestRepo.GetRoundByNumber(ctx, 3)
	if r3.Status != contest.StatusPreparing {
		t.Errorf("expected round 3 preparing, got %s", r3.Status)
	}

	// --- INTERMEDIATE DUPLICATE CHECK ---
	// Calling SettleContest before Round 2 end time must return ErrContestNotReadyToSettle
	_, err = svc.SettleContest(ctx, false)
	if !errors.Is(err, contest.ErrContestNotReadyToSettle) {
		t.Fatalf("expected ErrContestNotReadyToSettle on immediate duplicate call, got %v", err)
	}

	// Add 5 entries to preparing Round 3 during the active voting of Round 2
	for i := 1; i <= 5; i++ {
		charID := fmt.Sprintf("char-r3-%d", i)
		charRepo.Update(ctx, corecharacter.Character{ID: charID, Name: fmt.Sprintf("PlayerR3_%d", i), Money: 3000})
		contestRepo.SaveEntry(ctx, contest.ContestEntry{
			ID:          fmt.Sprintf("entry-r3-%d", i),
			Round:       3,
			CharacterID: charID,
			Title:       fmt.Sprintf("Title R3 %d", i),
			Votes:       20 - i,
		})
	}

	// --- CYCLE 2 SETTLEMENT (Advance time by 10 days to Round 2 deadline) ---
	currentTime = r2.EndTime.Add(1 * time.Minute)
	res2, err := svc.SettleContest(ctx, false)
	if err != nil {
		t.Fatalf("Cycle 2 settlement failed: %v", err)
	}
	if res2.Postponed {
		t.Fatal("expected cycle 2 not to be postponed")
	}
	if !res2.PrizesDistributed {
		t.Fatal("expected cycle 2 prizes distributed")
	}
	if res2.Round != 2 || res2.NextRound != 3 {
		t.Fatalf("expected cycle 2 settled round 2, next round 3; got round %d, next %d", res2.Round, res2.NextRound)
	}

	// Verify Round 2 winner received prize once (2000 + 15000 = 17000)
	w2, _ := charRepo.FindByID(ctx, "char-r2-1")
	if w2.Money != 2000+15000 {
		t.Errorf("expected R2 winner to have 17000 money, got %d", w2.Money)
	}

	// Verify Round 1 winner did NOT receive duplicate prizes (still 16000)
	w1AfterCycle2, _ := charRepo.FindByID(ctx, "char-r1-1")
	if w1AfterCycle2.Money != 1000+15000 {
		t.Errorf("expected R1 winner money unchanged after cycle 2, got %d", w1AfterCycle2.Money)
	}

	// Verify Round 2 winner was archived to Legend because Round 2 had 5 entries
	if res2.WinnerLegend == nil {
		t.Error("expected Round 2 winner to be archived to WinnerLegend")
	}

	// Verify Round 2 is settled, Round 3 is active, Round 4 is preparing
	r2Updated, _ := contestRepo.GetRoundByNumber(ctx, 2)
	if r2Updated.Status != contest.StatusSettled {
		t.Errorf("expected round 2 settled, got %s", r2Updated.Status)
	}
	r3Updated, _ := contestRepo.GetRoundByNumber(ctx, 3)
	if r3Updated.Status != contest.StatusActive {
		t.Errorf("expected round 3 active, got %s", r3Updated.Status)
	}
	r4, _ := contestRepo.GetRoundByNumber(ctx, 4)
	if r4.Status != contest.StatusPreparing {
		t.Errorf("expected round 4 preparing, got %s", r4.Status)
	}
}
