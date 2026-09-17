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
