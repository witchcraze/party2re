package job

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

type legendCallEntry struct {
	Category    string
	CharacterID string
}

func setupNearCompleteState(svc *Service, charID string, currentJob string) corejob.CharacterJob {
	state, _ := corejob.NewCharacterJob(charID, currentJob)
	// Master all completion jobs except currentJob.
	// When changing from currentJob, currentJob will be mastered, completing all jobs.
	for _, def := range svc.catalog.Definitions() {
		if def.ID != currentJob && corejob.IsCompletionJob(def.ID) {
			state.RecordMastery(def.ID, def.MasteryThreshold(), def.MasteryThreshold())
		}
	}
	return state
}

func TestChangeJob_TransactionFailure_DoesNotTriggerSideEffects(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-side-effect-fail",
		Name:      "FlameHero",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "male",
		SP:        1000, // plenty of SP to master job-01
		OverLevel: true,
	}

	ecoCharRepo := &errCharRepoStub{char: char}
	inv, _ := coreinventory.New(char.ID)
	fireArmor, _ := item.NewInstance("armor-29", 1)
	_ = inv.Add(fireArmor)
	invRepo := &errInventoryRepoStub{inv: inv}

	equip, _ := coreequipment.New(char.ID)
	equip.Slots[item.SlotBody] = fireArmor.ID
	equipRepo := &equipmentRepoStub{
		equipment: equip,
		saveErr:   errors.New("simulated equipment save error"),
	}

	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	var legendCalls []legendCallEntry
	legend := LegendInductorFunc(func(_ context.Context, category, characterID string) error {
		legendCalls = append(legendCalls, legendCallEntry{Category: category, CharacterID: characterID})
		return nil
	})
	news := &mockNewsPublisher{}

	dummyRepo := &repositoryStub{}
	svc, err := NewService(
		dummyRepo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
		WithEconomy(ecoSvc),
		WithLegendInductor(legend),
		WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatal(err)
	}

	state := setupNearCompleteState(svc, char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	svc.repository = repo

	_, _, err = svc.ChangeJob(ctx, char.ID, "job-84")
	if err == nil {
		t.Fatal("expected ChangeJob to fail due to equipment save failure")
	}

	if len(news.published) != 0 {
		t.Fatalf("expected 0 news articles on transaction failure, got %d: %v", len(news.published), news.published)
	}
	if len(legendCalls) != 0 {
		t.Fatalf("expected 0 legend calls on transaction failure, got %d: %v", len(legendCalls), legendCalls)
	}
}

func TestChangeJob_TransactionSuccess_TriggersSideEffectsExactlyOnce(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-side-effect-succ",
		Name:      "MasterHero",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "male",
		SP:        1000,
		OverLevel: true,
	}

	ecoCharRepo := &errCharRepoStub{char: char}
	inv, _ := coreinventory.New(char.ID)
	invRepo := &errInventoryRepoStub{inv: inv}

	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	var legendCalls []legendCallEntry
	legend := LegendInductorFunc(func(_ context.Context, category, characterID string) error {
		legendCalls = append(legendCalls, legendCallEntry{Category: category, CharacterID: characterID})
		return nil
	})
	news := &mockNewsPublisher{}

	dummyRepo := &repositoryStub{}
	svc, err := NewService(
		dummyRepo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEconomy(ecoSvc),
		WithLegendInductor(legend),
		WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatal(err)
	}

	state := setupNearCompleteState(svc, char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	svc.repository = repo

	_, _, err = svc.ChangeJob(ctx, char.ID, "job-02")
	if err != nil {
		t.Fatalf("expected ChangeJob to succeed, got %v", err)
	}

	if len(news.published) != 1 {
		t.Fatalf("expected exactly 1 news article on transaction success, got %d: %v", len(news.published), news.published)
	}
	if len(legendCalls) != 1 {
		t.Fatalf("expected exactly 1 legend call on transaction success, got %d: %v", len(legendCalls), legendCalls)
	}
}

func TestChangeJob_NoEconomy_TriggersSideEffectsExactlyOnce(t *testing.T) {
	ctx := context.Background()

	char := corecharacter.Character{
		ID:        "char-side-effect-noeco",
		Name:      "FallbackHero",
		JobID:     "job-01",
		OldJobID:  "job-04",
		Level:     50,
		Gender:    "male",
		SP:        1000,
		OverLevel: true,
	}

	charRepo := &charRepoStub{char: char}
	var legendCalls []legendCallEntry
	legend := LegendInductorFunc(func(_ context.Context, category, characterID string) error {
		legendCalls = append(legendCalls, legendCallEntry{Category: category, CharacterID: characterID})
		return nil
	})
	news := &mockNewsPublisher{}

	dummyRepo := &repositoryStub{}
	svc, err := NewService(
		dummyRepo,
		WithCharacterRepository(charRepo),
		WithLegendInductor(legend),
		WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatal(err)
	}

	state := setupNearCompleteState(svc, char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	svc.repository = repo

	updatedChar, updatedState, err := svc.ChangeJob(ctx, char.ID, "job-02")
	if err != nil {
		t.Fatalf("expected ChangeJob to succeed in fallback mode, got %v", err)
	}

	if updatedChar.JobID != "job-02" {
		t.Fatalf("expected job-02, got %s", updatedChar.JobID)
	}
	if updatedState.CurrentJobID != "job-02" {
		t.Fatalf("expected current job state job-02, got %s", updatedState.CurrentJobID)
	}
	if len(news.published) != 1 {
		t.Fatalf("expected exactly 1 news article in fallback mode, got %d: %v", len(news.published), news.published)
	}
	if len(legendCalls) != 1 {
		t.Fatalf("expected exactly 1 legend call in fallback mode, got %d: %v", len(legendCalls), legendCalls)
	}
}
