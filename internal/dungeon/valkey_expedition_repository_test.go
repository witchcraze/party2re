package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
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

func cleanupDungeonValkey(t *testing.T, client valkeygo.Client, charIDs ...string) {
	t.Helper()
	ctx := context.Background()
	for _, cid := range charIDs {
		sKey := fmt.Sprintf("party2:dungeon:{char:%s}:state", cid)
		rKey := fmt.Sprintf("party2:dungeon:{char:%s}:rewards", cid)
		client.Do(ctx, client.B().Del().Key(sKey).Build())
		client.Do(ctx, client.B().Del().Key(rKey).Build())
	}
}

func sampleExpedition(id, charID, dungeonID string) dungeon.ActiveExpedition {
	now := time.Now().UTC().Truncate(time.Millisecond)
	return dungeon.ActiveExpedition{
		ID:                id,
		CharacterID:       charID,
		DungeonID:         dungeonID,
		CurrentFloor:      1,
		PosX:              0,
		PosY:              0,
		CurrentHP:         100,
		TurnsRemaining:    50,
		AccumulatedExp:    0,
		AccumulatedGold:   0,
		AccumulatedMedals: 0,
		AccumulatedItems:  []string{},
		Status:            dungeon.StatusExploring,
		StartedAt:         now,
		UpdatedAt:         now,
	}
}

func TestMemoryExpeditionRepository_LifecycleAndErrors(t *testing.T) {
	ctx := context.Background()
	repo := dungeon.NewMemoryExpeditionRepository(dungeon.WithExpeditionTTL(2 * time.Hour))

	charID := "mem-char-01"
	expID := "mem-exp-01"
	exp := sampleExpedition(expID, charID, "dungeon-01")

	// 1. Initially nil
	got, err := repo.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil active expedition, got %#v", got)
	}

	// 2. Save
	if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	// 3. Get
	got, err = repo.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if got == nil || got.ID != expID || got.CurrentFloor != 1 {
		t.Fatalf("unexpected expedition: %#v", got)
	}

	// 4. Step: normal progression
	stepParams := dungeon.StepParams{
		ExpectedExpeditionID: expID,
		NewFloor:             1,
		NewX:                 1,
		NewY:                 0,
		HPDelta:              -10,
		TurnsDelta:           -1,
		ExpDelta:             25,
		GoldDelta:            50,
		MedalsDelta:          1,
		RewardItemID:         "herb",
		Now:                  time.Now().UTC(),
	}
	outcome, err := repo.Step(ctx, charID, stepParams)
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if outcome.Status != dungeon.StatusExploring {
		t.Errorf("expected StatusExploring, got %v", outcome.Status)
	}
	if outcome.Expedition.CurrentHP != 90 {
		t.Errorf("expected HP 90, got %d", outcome.Expedition.CurrentHP)
	}
	if outcome.Expedition.TurnsRemaining != 49 {
		t.Errorf("expected turns 49, got %d", outcome.Expedition.TurnsRemaining)
	}
	if outcome.Expedition.AccumulatedExp != 25 || outcome.Expedition.AccumulatedGold != 50 {
		t.Errorf("unexpected rewards: exp=%d, gold=%d", outcome.Expedition.AccumulatedExp, outcome.Expedition.AccumulatedGold)
	}
	if len(outcome.Expedition.AccumulatedItems) != 1 || outcome.Expedition.AccumulatedItems[0] != "herb" {
		t.Errorf("unexpected items: %v", outcome.Expedition.AccumulatedItems)
	}

	// 5. Error: Mismatch Expedition ID
	badParams := stepParams
	badParams.ExpectedExpeditionID = "wrong-exp-id"
	_, err = repo.Step(ctx, charID, badParams)
	if !errors.Is(err, dungeon.ErrExpeditionIDMismatch) {
		t.Errorf("expected ErrExpeditionIDMismatch, got %v", err)
	}

	// 6. Step to Wipeout (HP drops to 0)
	fatalParams := stepParams
	fatalParams.HPDelta = -200
	outcome, err = repo.Step(ctx, charID, fatalParams)
	if err != nil {
		t.Fatalf("Step to fatal failed: %v", err)
	}
	if outcome.Status != dungeon.StatusWipedOut {
		t.Errorf("expected StatusWipedOut, got %v", outcome.Status)
	}
	if outcome.Expedition.CurrentHP != 0 {
		t.Errorf("expected HP 0, got %d", outcome.Expedition.CurrentHP)
	}

	// 7. Error: Expedition Not Active (already wiped out)
	_, err = repo.Step(ctx, charID, stepParams)
	if !errors.Is(err, dungeon.ErrExpeditionNotActive) {
		t.Errorf("expected ErrExpeditionNotActive, got %v", err)
	}

	// 8. Delete
	if err := repo.DeleteActiveExpedition(ctx, charID); err != nil {
		t.Fatalf("DeleteActiveExpedition failed: %v", err)
	}
	got, _ = repo.GetActiveExpedition(ctx, charID)
	if got != nil {
		t.Errorf("expected nil after delete, got %#v", got)
	}

	// 9. Error: Not Found
	_, err = repo.Step(ctx, "nonexistent-char", stepParams)
	if !errors.Is(err, dungeon.ErrExpeditionNotFound) {
		t.Errorf("expected ErrExpeditionNotFound, got %v", err)
	}
}

func TestMemoryExpeditionRepository_TTL(t *testing.T) {
	ctx := context.Background()
	shortTTL := 30 * time.Millisecond
	repo := dungeon.NewMemoryExpeditionRepository(dungeon.WithExpeditionTTL(shortTTL))

	charID := "mem-ttl-char"
	exp := sampleExpedition("mem-ttl-exp", charID, "dungeon-01")

	if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}

	// Immediate get works
	got, err := repo.GetActiveExpedition(ctx, charID)
	if err != nil || got == nil {
		t.Fatalf("expected active expedition, got %v, err: %v", got, err)
	}

	// Sleep past TTL
	time.Sleep(50 * time.Millisecond)

	got, err = repo.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected expired expedition to be nil, got %#v", got)
	}

	_, err = repo.Step(ctx, charID, dungeon.StepParams{ExpectedExpeditionID: "mem-ttl-exp"})
	if !errors.Is(err, dungeon.ErrExpeditionNotFound) {
		t.Errorf("expected ErrExpeditionNotFound after expiry, got %v", err)
	}
}

func TestValkeyExpeditionRepository_LifecycleAndErrors(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	charID := "vk-char-01"
	expID := "vk-exp-01"
	cleanupDungeonValkey(t, client, charID)
	defer cleanupDungeonValkey(t, client, charID)

	repo, err := dungeon.NewValkeyExpeditionRepository(client, dungeon.WithExpeditionTTL(7200*time.Second))
	if err != nil {
		t.Fatalf("NewValkeyExpeditionRepository failed: %v", err)
	}

	ctx := context.Background()

	// 1. Verify initially nil
	got, err := repo.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil expedition, got %#v", got)
	}

	// 2. SaveActiveExpedition
	exp := sampleExpedition(expID, charID, "dungeon-01")
	if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("SaveActiveExpedition failed: %v", err)
	}

	// 3. Verify Keys and Hash Tags in Valkey
	stateKey := fmt.Sprintf("party2:dungeon:{char:%s}:state", charID)
	rewardsKey := fmt.Sprintf("party2:dungeon:{char:%s}:rewards", charID)

	ttlCmd := client.Do(ctx, client.B().Ttl().Key(stateKey).Build())
	stateTTL, err := ttlCmd.AsInt64()
	if err != nil || stateTTL <= 0 {
		t.Errorf("expected positive TTL for stateKey, got %d, err: %v", stateTTL, err)
	}
	if stateTTL > 7200 {
		t.Errorf("expected TTL <= 7200, got %d", stateTTL)
	}

	// 4. GetActiveExpedition
	got, err = repo.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if got == nil {
		t.Fatalf("expected non-nil expedition")
	}
	if got.ID != expID || got.CharacterID != charID || got.CurrentFloor != 1 || got.CurrentHP != 100 {
		t.Errorf("unexpected expedition values: %#v", got)
	}

	// 5. Atomic Step with Lua Script
	stepParams := dungeon.StepParams{
		ExpectedExpeditionID: expID,
		NewFloor:             1,
		NewX:                 2,
		NewY:                 0,
		HPDelta:              -15,
		TurnsDelta:           -1,
		ExpDelta:             40,
		GoldDelta:            80,
		MedalsDelta:          2,
		RewardItemID:         "potion",
		Now:                  time.Now().UTC(),
	}
	outcome, err := repo.Step(ctx, charID, stepParams)
	if err != nil {
		t.Fatalf("Step failed: %v", err)
	}
	if outcome.Status != dungeon.StatusExploring {
		t.Errorf("expected StatusExploring, got %v", outcome.Status)
	}
	if outcome.Expedition.CurrentHP != 85 {
		t.Errorf("expected HP 85, got %d", outcome.Expedition.CurrentHP)
	}
	if outcome.Expedition.TurnsRemaining != 49 {
		t.Errorf("expected turns 49, got %d", outcome.Expedition.TurnsRemaining)
	}
	if outcome.Expedition.AccumulatedExp != 40 || outcome.Expedition.AccumulatedGold != 80 || outcome.Expedition.AccumulatedMedals != 2 {
		t.Errorf("unexpected accumulated rewards: %#v", outcome.Expedition)
	}
	if len(outcome.Expedition.AccumulatedItems) != 1 || outcome.Expedition.AccumulatedItems[0] != "potion" {
		t.Errorf("unexpected accumulated items: %v", outcome.Expedition.AccumulatedItems)
	}

	// 6. Step appending another item
	stepParams2 := dungeon.StepParams{
		ExpectedExpeditionID: expID,
		NewFloor:             1,
		NewX:                 2,
		NewY:                 1,
		HPDelta:              0,
		TurnsDelta:           -1,
		ExpDelta:             10,
		GoldDelta:            20,
		MedalsDelta:          0,
		RewardItemID:         "magic_key",
		Now:                  time.Now().UTC(),
	}
	outcome2, err := repo.Step(ctx, charID, stepParams2)
	if err != nil {
		t.Fatalf("second Step failed: %v", err)
	}
	if outcome2.Expedition.AccumulatedExp != 50 || outcome2.Expedition.AccumulatedGold != 100 {
		t.Errorf("unexpected cumulative rewards: exp=%d, gold=%d", outcome2.Expedition.AccumulatedExp, outcome2.Expedition.AccumulatedGold)
	}
	if len(outcome2.Expedition.AccumulatedItems) != 2 || outcome2.Expedition.AccumulatedItems[1] != "magic_key" {
		t.Errorf("unexpected items after second step: %v", outcome2.Expedition.AccumulatedItems)
	}

	// 7. Test Mismatch Expedition ID Error
	badMismatch := stepParams
	badMismatch.ExpectedExpeditionID = "other-expedition"
	_, err = repo.Step(ctx, charID, badMismatch)
	if !errors.Is(err, dungeon.ErrExpeditionIDMismatch) {
		t.Errorf("expected ErrExpeditionIDMismatch, got %v", err)
	}

	// 8. Test Step leading to wipeout (Turns exhausted)
	turnsExhaustParams := dungeon.StepParams{
		ExpectedExpeditionID: expID,
		NewFloor:             1,
		NewX:                 2,
		NewY:                 2,
		HPDelta:              0,
		TurnsDelta:           -100, // exceeds remaining turns
		Now:                  time.Now().UTC(),
	}
	wipeOutcome, err := repo.Step(ctx, charID, turnsExhaustParams)
	if err != nil {
		t.Fatalf("wipeout step failed: %v", err)
	}
	if wipeOutcome.Status != dungeon.StatusWipedOut {
		t.Errorf("expected StatusWipedOut, got %v", wipeOutcome.Status)
	}
	if wipeOutcome.Expedition.TurnsRemaining != 0 {
		t.Errorf("expected 0 turns remaining, got %d", wipeOutcome.Expedition.TurnsRemaining)
	}

	// 9. Test Step on inactive expedition returns ErrExpeditionNotActive
	_, err = repo.Step(ctx, charID, stepParams)
	if !errors.Is(err, dungeon.ErrExpeditionNotActive) {
		t.Errorf("expected ErrExpeditionNotActive, got %v", err)
	}

	// 10. Test DeleteActiveExpedition purges both state and rewards keys
	if err := repo.DeleteActiveExpedition(ctx, charID); err != nil {
		t.Fatalf("DeleteActiveExpedition failed: %v", err)
	}

	existsCmd := client.Do(ctx, client.B().Exists().Key(stateKey, rewardsKey).Build())
	existCount, _ := existsCmd.AsInt64()
	if existCount != 0 {
		t.Errorf("expected 0 keys existing after delete, got %d", existCount)
	}

	// 11. Test Step on nonexistent returns ErrExpeditionNotFound
	_, err = repo.Step(ctx, "nonexistent-char-id", stepParams)
	if !errors.Is(err, dungeon.ErrExpeditionNotFound) {
		t.Errorf("expected ErrExpeditionNotFound, got %v", err)
	}
}

func TestValkeyExpeditionRepository_ConcurrentSteps(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	repo, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()
	concurrency := 10
	charIDs := make([]string, concurrency)
	for i := 0; i < concurrency; i++ {
		charIDs[i] = fmt.Sprintf("conc-char-%02d", i)
	}

	cleanupDungeonValkey(t, client, charIDs...)
	defer cleanupDungeonValkey(t, client, charIDs...)

	// Initialize expeditions for all characters
	for i, cid := range charIDs {
		exp := sampleExpedition(fmt.Sprintf("conc-exp-%02d", i), cid, "dungeon-01")
		if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
			t.Fatalf("SaveActiveExpedition for %s failed: %v", cid, err)
		}
	}

	// Step concurrently across characters
	var wg sync.WaitGroup
	errCh := make(chan error, concurrency*5)

	stepsPerChar := 5
	for _, cid := range charIDs {
		cid := cid
		wg.Add(1)
		go func() {
			defer wg.Done()
			for s := 1; s <= stepsPerChar; s++ {
				_, stepErr := repo.Step(ctx, cid, dungeon.StepParams{
					ExpectedExpeditionID: fmt.Sprintf("conc-exp-%s", cid[len(cid)-2:]),
					NewFloor:             1,
					NewX:                 s,
					NewY:                 0,
					HPDelta:              -2,
					TurnsDelta:           -1,
					ExpDelta:             10,
					GoldDelta:            20,
					MedalsDelta:          1,
					Now:                  time.Now().UTC(),
				})
				if stepErr != nil {
					errCh <- fmt.Errorf("char %s step %d failed: %w", cid, s, stepErr)
					return
				}
			}
		}()
	}

	wg.Wait()
	close(errCh)

	for err := range errCh {
		t.Errorf("concurrent step error: %v", err)
	}

	// Verify all characters accumulated exactly stepsPerChar rewards
	for _, cid := range charIDs {
		exp, err := repo.GetActiveExpedition(ctx, cid)
		if err != nil {
			t.Fatalf("GetActiveExpedition failed: %v", err)
		}
		if exp == nil {
			t.Fatalf("expected expedition for %s", cid)
		}
		expectedExp := stepsPerChar * 10
		expectedGold := stepsPerChar * 20
		expectedMedals := stepsPerChar * 1
		expectedTurns := 50 - stepsPerChar
		expectedHP := 100 - (stepsPerChar * 2)

		if exp.AccumulatedExp != expectedExp || exp.AccumulatedGold != expectedGold || exp.AccumulatedMedals != expectedMedals {
			t.Errorf("char %s rewards mismatch: exp=%d (want %d), gold=%d (want %d), medals=%d (want %d)",
				cid, exp.AccumulatedExp, expectedExp, exp.AccumulatedGold, expectedGold, exp.AccumulatedMedals, expectedMedals)
		}
		if exp.TurnsRemaining != expectedTurns || exp.CurrentHP != expectedHP {
			t.Errorf("char %s stats mismatch: turns=%d (want %d), hp=%d (want %d)",
				cid, exp.TurnsRemaining, expectedTurns, exp.CurrentHP, expectedHP)
		}
	}
}

type failFinalizeMockDungeonRepo struct {
	*mockDungeonRepo
	failFinalize bool
}

func (m *failFinalizeMockDungeonRepo) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	record dungeon.CharacterDungeonRecord,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
) error {
	if m.failFinalize {
		return errors.New("simulated MariaDB commit failure")
	}
	return m.mockDungeonRepo.FinalizeExpedition(ctx, history, record, character, rewardItems)
}

func TestTwoPhaseSettlement_CommitFailureRetainsValkey(t *testing.T) {
	client := openTestValkey(t)
	defer client.Close()

	charID := "two-phase-char"
	cleanupDungeonValkey(t, client, charID)
	defer cleanupDungeonValkey(t, client, charID)

	valkeyStore, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}

	baseRepo := newMockDungeonRepo()
	failingRepo := &failFinalizeMockDungeonRepo{
		mockDungeonRepo: baseRepo,
		failFinalize:    true, // Simulate MariaDB failure
	}

	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			charID: createTestChar(charID, 10, 200, 50, 40),
		},
	}
	battleEngine := corebattle.Engine{}

	service, err := dungeon.NewService(
		failingRepo,
		charRepo,
		battleEngine,
		dungeon.WithActiveExpeditionStore(valkeyStore),
	)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Start expedition
	_, err = service.StartExpedition(ctx, charID, "dungeon-01")
	if err != nil {
		t.Fatalf("StartExpedition failed: %v", err)
	}

	// Verify it's in Valkey
	valkeyExp, err := valkeyStore.GetActiveExpedition(ctx, charID)
	if err != nil || valkeyExp == nil {
		t.Fatalf("expected expedition in Valkey, got %v, err: %v", valkeyExp, err)
	}

	// 2. Attempt Escape while MariaDB fails
	_, err = service.Escape(ctx, charID)
	if err == nil {
		t.Fatalf("expected Escape to fail due to MariaDB commit failure")
	}

	// Two-Phase Settlement Contract: Valkey buffer MUST NOT be deleted on MariaDB failure!
	valkeyExpAfterFail, err := valkeyStore.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if valkeyExpAfterFail == nil {
		t.Fatalf("Valkey buffer was wiped prematurely on MariaDB failure! Buffer must be preserved for retry.")
	}

	// 3. Resolve MariaDB failure and retry Escape
	failingRepo.failFinalize = false

	escapeRes, err := service.Escape(ctx, charID)
	if err != nil {
		t.Fatalf("retry Escape failed: %v", err)
	}
	if !escapeRes.IsFinished || escapeRes.Expedition.Status != dungeon.StatusEscaped {
		t.Fatalf("expected successful escape, got %#v", escapeRes)
	}

	// Upon successful MariaDB commit, Valkey buffer MUST be purged
	valkeyExpAfterSuccess, err := valkeyStore.GetActiveExpedition(ctx, charID)
	if err != nil {
		t.Fatalf("GetActiveExpedition failed: %v", err)
	}
	if valkeyExpAfterSuccess != nil {
		t.Fatalf("Valkey buffer was NOT purged after successful commit: %#v", valkeyExpAfterSuccess)
	}
}
