package challenge_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func openTestValkey(t *testing.T) valkeygo.Client {
	t.Helper()
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClient()
	if err != nil {
		t.Fatalf("open valkey client: %v", err)
	}
	return client
}

func cleanupChallengeValkey(t *testing.T, client valkeygo.Client, charIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for _, cid := range charIDs {
		sKey := fmt.Sprintf("party2:challenge:{char:%s}:session", cid)
		rKey := fmt.Sprintf("party2:challenge:{char:%s}:rewards", cid)
		client.Do(ctx, client.B().Del().Key(sKey).Build())
		client.Do(ctx, client.B().Del().Key(rKey).Build())
	}
}

func sampleChallengeSession(id, charID, tierID string) challenge.ChallengeSession {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return challenge.ChallengeSession{
		ID:                 id,
		CharacterID:        charID,
		TierID:             tierID,
		CurrentRound:       1,
		CharacterCurrentHP: 200,
		AccumulatedExp:     0,
		AccumulatedGold:    0,
		AccumulatedItems:   []string{},
		Status:             challenge.StatusActive,
		CreatedAt:          now,
		UpdatedAt:          now,
	}
}

func TestMemorySessionRepository_LifecycleAndErrors(t *testing.T) {
	ctx := context.Background()
	repo := challenge.NewMemorySessionRepository(challenge.WithSessionTTL(2 * time.Hour))

	charID := "mem-chal-char-01"
	sessID := "mem-chal-sess-01"
	sess := sampleChallengeSession(sessID, charID, "novice")

	// 1. Initially nil
	got, err := repo.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil active session, got %#v", got)
	}

	// 2. Save
	if err := repo.SaveActiveSession(ctx, sess); err != nil {
		t.Fatalf("SaveActiveSession failed: %v", err)
	}

	// 3. Get
	got, err = repo.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if got == nil || got.ID != sessID || got.CurrentRound != 1 {
		t.Fatalf("unexpected session: %#v", got)
	}

	// 4. AdvanceRound
	advanceParams := challenge.AdvanceRoundParams{
		ExpectedSessionID: sessID,
		SurvivingHP:       180,
		ExpDelta:          50,
		GoldDelta:         100,
		RewardItemID:      "herb",
		Now:               time.Now().UTC(),
	}
	outcome, err := repo.AdvanceRound(ctx, charID, advanceParams)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if outcome.Status != challenge.StatusActive {
		t.Errorf("expected StatusActive, got %v", outcome.Status)
	}
	if outcome.Session.CurrentRound != 2 {
		t.Errorf("expected round 2, got %d", outcome.Session.CurrentRound)
	}
	if outcome.Session.CharacterCurrentHP != 180 {
		t.Errorf("expected HP 180, got %d", outcome.Session.CharacterCurrentHP)
	}
	if outcome.Session.AccumulatedExp != 50 || outcome.Session.AccumulatedGold != 100 {
		t.Errorf("unexpected rewards: exp=%d, gold=%d", outcome.Session.AccumulatedExp, outcome.Session.AccumulatedGold)
	}
	if len(outcome.Session.AccumulatedItems) != 1 || outcome.Session.AccumulatedItems[0] != "herb" {
		t.Errorf("unexpected items: %v", outcome.Session.AccumulatedItems)
	}

	// 5. Error: Mismatch Session ID
	badParams := advanceParams
	badParams.ExpectedSessionID = "wrong-session-id"
	_, err = repo.AdvanceRound(ctx, charID, badParams)
	if !errors.Is(err, challenge.ErrSessionIDMismatch) {
		t.Errorf("expected ErrSessionIDMismatch, got %v", err)
	}

	// 6. Delete
	if err := repo.DeleteActiveSession(ctx, charID); err != nil {
		t.Fatalf("DeleteActiveSession failed: %v", err)
	}
	got, _ = repo.GetActiveSession(ctx, charID)
	if got != nil {
		t.Errorf("expected nil after delete, got %#v", got)
	}

	// 7. Error: Not Found
	_, err = repo.AdvanceRound(ctx, "nonexistent-char", advanceParams)
	if !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestMemorySessionRepository_TTL(t *testing.T) {
	ctx := context.Background()
	shortTTL := 30 * time.Millisecond
	repo := challenge.NewMemorySessionRepository(challenge.WithSessionTTL(shortTTL))

	charID := "mem-ttl-chal-char"
	sess := sampleChallengeSession("mem-ttl-chal-sess", charID, "novice")

	if err := repo.SaveActiveSession(ctx, sess); err != nil {
		t.Fatal(err)
	}

	// Immediate get works
	got, err := repo.GetActiveSession(ctx, charID)
	if err != nil || got == nil {
		t.Fatalf("expected active session, got %v, err: %v", got, err)
	}

	// Sleep past TTL
	time.Sleep(50 * time.Millisecond)

	got, err = repo.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected expired session to be nil, got %#v", got)
	}

	_, err = repo.AdvanceRound(ctx, charID, challenge.AdvanceRoundParams{ExpectedSessionID: "mem-ttl-chal-sess"})
	if !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound after expiry, got %v", err)
	}
}

func TestValkeySessionRepository_LifecycleAndErrors(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	charID := "vk-chal-char-01"
	sessID := "vk-chal-sess-01"
	cleanupChallengeValkey(t, client, charID)
	defer cleanupChallengeValkey(t, client, charID)

	repo, err := challenge.NewValkeySessionRepository(client, challenge.WithSessionTTL(7200*time.Second))
	if err != nil {
		t.Fatalf("NewValkeySessionRepository failed: %v", err)
	}

	ctx := context.Background()

	// 1. Verify initially nil
	got, err := repo.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil session, got %#v", got)
	}

	// 2. SaveActiveSession
	sess := sampleChallengeSession(sessID, charID, "novice")
	if err := repo.SaveActiveSession(ctx, sess); err != nil {
		t.Fatalf("SaveActiveSession failed: %v", err)
	}

	// 3. Verify Keys and Hash Tags in Valkey
	sessionKey := fmt.Sprintf("party2:challenge:{char:%s}:session", charID)
	rewardsKey := fmt.Sprintf("party2:challenge:{char:%s}:rewards", charID)

	ttlCmd := client.Do(ctx, client.B().Ttl().Key(sessionKey).Build())
	stateTTL, err := ttlCmd.AsInt64()
	if err != nil || stateTTL <= 0 {
		t.Errorf("expected positive TTL for sessionKey, got %d, err: %v", stateTTL, err)
	}
	if stateTTL > 7200 {
		t.Errorf("expected TTL <= 7200, got %d", stateTTL)
	}

	// 4. GetActiveSession
	got, err = repo.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil session")
	}
	if got.ID != sessID || got.CharacterID != charID || got.CurrentRound != 1 || got.CharacterCurrentHP != 200 {
		t.Errorf("unexpected session values: %#v", got)
	}

	// 5. Atomic AdvanceRound with Lua Script
	advanceParams := challenge.AdvanceRoundParams{
		ExpectedSessionID: sessID,
		SurvivingHP:       175,
		ExpDelta:          75,
		GoldDelta:         150,
		RewardItemID:      "iron_sword",
		Now:               time.Now().UTC(),
	}
	outcome, err := repo.AdvanceRound(ctx, charID, advanceParams)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if outcome.Status != challenge.StatusActive {
		t.Errorf("expected StatusActive, got %v", outcome.Status)
	}
	if outcome.Session.CurrentRound != 2 {
		t.Errorf("expected round 2, got %d", outcome.Session.CurrentRound)
	}
	if outcome.Session.CharacterCurrentHP != 175 {
		t.Errorf("expected HP 175, got %d", outcome.Session.CharacterCurrentHP)
	}
	if outcome.Session.AccumulatedExp != 75 || outcome.Session.AccumulatedGold != 150 {
		t.Errorf("unexpected accumulated rewards: %#v", outcome.Session)
	}
	if len(outcome.Session.AccumulatedItems) != 1 || outcome.Session.AccumulatedItems[0] != "iron_sword" {
		t.Errorf("unexpected accumulated items: %v", outcome.Session.AccumulatedItems)
	}

	// 6. Advance another round accumulating another item
	advanceParams2 := challenge.AdvanceRoundParams{
		ExpectedSessionID: sessID,
		SurvivingHP:       190,
		ExpDelta:          80,
		GoldDelta:         160,
		RewardItemID:      "magic_potion",
		Now:               time.Now().UTC(),
	}
	outcome2, err := repo.AdvanceRound(ctx, charID, advanceParams2)
	if err != nil {
		t.Fatalf("second AdvanceRound failed: %v", err)
	}
	if outcome2.Session.CurrentRound != 3 {
		t.Errorf("expected round 3, got %d", outcome2.Session.CurrentRound)
	}
	if outcome2.Session.AccumulatedExp != 155 || outcome2.Session.AccumulatedGold != 310 {
		t.Errorf("unexpected cumulative rewards: exp=%d, gold=%d", outcome2.Session.AccumulatedExp, outcome2.Session.AccumulatedGold)
	}
	if len(outcome2.Session.AccumulatedItems) != 2 || outcome2.Session.AccumulatedItems[1] != "magic_potion" {
		t.Errorf("unexpected items after second advance: %v", outcome2.Session.AccumulatedItems)
	}

	// 7. Test Mismatch Session ID Error
	badMismatch := advanceParams
	badMismatch.ExpectedSessionID = "other-session"
	_, err = repo.AdvanceRound(ctx, charID, badMismatch)
	if !errors.Is(err, challenge.ErrSessionIDMismatch) {
		t.Errorf("expected ErrSessionIDMismatch, got %v", err)
	}

	// 8. Test DeleteActiveSession purges both session and rewards keys
	if err := repo.DeleteActiveSession(ctx, charID); err != nil {
		t.Fatalf("DeleteActiveSession failed: %v", err)
	}

	existsCmd := client.Do(ctx, client.B().Exists().Key(sessionKey, rewardsKey).Build())
	existCount, _ := existsCmd.AsInt64()
	if existCount != 0 {
		t.Errorf("expected 0 keys existing after delete, got %d", existCount)
	}

	// 9. Test AdvanceRound on nonexistent returns ErrSessionNotFound
	_, err = repo.AdvanceRound(ctx, "nonexistent-char-id", advanceParams)
	if !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

func TestValkeySessionRepository_ConcurrentRounds(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	repo, err := challenge.NewValkeySessionRepository(client)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	concurrency := 10
	charIDs := make([]string, concurrency)
	for i := 0; i < concurrency; i++ {
		charIDs[i] = fmt.Sprintf("conc-chal-%02d", i)
	}

	cleanupChallengeValkey(t, client, charIDs...)
	defer cleanupChallengeValkey(t, client, charIDs...)

	// Initialize sessions for all characters
	for i, cid := range charIDs {
		sess := sampleChallengeSession(fmt.Sprintf("conc-sess-%02d", i), cid, "novice")
		if err := repo.SaveActiveSession(ctx, sess); err != nil {
			t.Fatalf("SaveActiveSession for %s failed: %v", cid, err)
		}
	}

	// Advance rounds concurrently across characters
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency*5)

	roundsPerChar := 5
	for _, cid := range charIDs {
		cid := cid
		wg.Add(1)
		go func() {
			defer wg.Done()
			for r := 1; r <= roundsPerChar; r++ {
				_, stepErr := repo.AdvanceRound(ctx, cid, challenge.AdvanceRoundParams{
					ExpectedSessionID: fmt.Sprintf("conc-sess-%s", cid[len(cid)-2:]),
					SurvivingHP:       150,
					ExpDelta:          20,
					GoldDelta:         40,
					RewardItemID:      "",
					Now:               time.Now().UTC(),
				})
				if stepErr != nil {
					errCh <- fmt.Errorf("char %s round %d failed: %w", cid, r, stepErr)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent round error: %v", err)
	}

	// Verify all characters reached round 6 (1 + 5) and accumulated exact rewards
	for _, cid := range charIDs {
		sess, err := repo.GetActiveSession(ctx, cid)
		if err != nil {
			t.Fatalf("GetActiveSession failed: %v", err)
		}
		if sess == nil {
			t.Fatalf("expected session for %s", cid)
		}
		expectedRound := 1 + roundsPerChar
		expectedExp := roundsPerChar * 20
		expectedGold := roundsPerChar * 40

		if sess.CurrentRound != expectedRound {
			t.Errorf("char %s round mismatch: got %d (want %d)", cid, sess.CurrentRound, expectedRound)
		}
		if sess.AccumulatedExp != expectedExp || sess.AccumulatedGold != expectedGold {
			t.Errorf("char %s rewards mismatch: exp=%d (want %d), gold=%d (want %d)",
				cid, sess.AccumulatedExp, expectedExp, sess.AccumulatedGold, expectedGold)
		}
	}
}

type failFinalizeMockChallengeRepo struct {
	*mockChallengeRepo
	failFinalize bool
}

func (m *failFinalizeMockChallengeRepo) FinalizeSession(
	ctx context.Context,
	session challenge.ChallengeSession,
	expReward int,
	goldReward int,
	items []string,
	newStreak int,
) error {
	if m.failFinalize {
		return errors.New("simulated MariaDB commit failure")
	}
	return m.mockChallengeRepo.FinalizeSession(ctx, session, expReward, goldReward, items, newStreak)
}

func TestTwoPhaseSettlement_CommitFailureRetainsValkey(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	charID := "two-phase-chal-char"
	cleanupChallengeValkey(t, client, charID)
	defer cleanupChallengeValkey(t, client, charID)

	valkeyStore, err := challenge.NewValkeySessionRepository(client)
	if err != nil {
		t.Fatal(err)
	}

	baseRepo := newMockChallengeRepo()
	failingRepo := &failFinalizeMockChallengeRepo{
		mockChallengeRepo: baseRepo,
		failFinalize:      true, // Simulate MariaDB failure
	}

	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			charID: {
				ID:         charID,
				Level:      20,
				Experience: 5000,
				Stats:      corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 100, Defense: 50},
			},
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := challenge.NewService(
		failingRepo,
		charRepo,
		battleEngine,
		challenge.WithActiveSessionStore(valkeyStore),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Start session
	sess, err := service.StartSession(ctx, charID, "novice")
	if err != nil {
		t.Fatalf("StartSession failed: %v", err)
	}

	// Advance 1 round to win
	_, updatedSess, err := service.AdvanceRound(ctx, charID, sess.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if updatedSess.CurrentRound != 2 {
		t.Fatalf("expected round 2, got %d", updatedSess.CurrentRound)
	}

	// Verify it's in Valkey
	valkeySess, err := valkeyStore.GetActiveSession(ctx, charID)
	if err != nil || valkeySess == nil {
		t.Fatalf("expected session in Valkey, got %v, err: %v", valkeySess, err)
	}

	// 2. Attempt Cashout while MariaDB fails
	_, err = service.Cashout(ctx, sess.ID)
	if err == nil {
		t.Fatalf("expected Cashout to fail due to MariaDB commit failure")
	}

	// Two-Phase Settlement Contract: Valkey buffer MUST NOT be deleted on MariaDB failure!
	valkeySessAfterFail, err := valkeyStore.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if valkeySessAfterFail == nil {
		t.Fatalf("Valkey buffer was wiped prematurely on MariaDB failure! Buffer must be preserved for retry.")
	}

	// 3. Resolve MariaDB failure and retry Cashout
	failingRepo.failFinalize = false

	cashoutRes, err := service.Cashout(ctx, sess.ID)
	if err != nil {
		t.Fatalf("retry Cashout failed: %v", err)
	}
	if cashoutRes.RoundsCleared != 1 {
		t.Errorf("expected 1 round cleared, got %d", cashoutRes.RoundsCleared)
	}

	// Upon successful MariaDB commit, Valkey buffer MUST be purged
	valkeySessAfterSuccess, err := valkeyStore.GetActiveSession(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveSession failed: %v", err)
	}
	if valkeySessAfterSuccess != nil {
		t.Fatalf("Valkey buffer was NOT purged after successful commit: %#v", valkeySessAfterSuccess)
	}
}
