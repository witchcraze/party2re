package job

import (
	"context"
	"errors"
	"slices"
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
	popTracker := &jobPopularityStub{}

	dummyRepo := &repositoryStub{}
	svc, err := NewService(
		dummyRepo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEquipmentRepository(equipRepo),
		WithEconomy(ecoSvc),
		WithLegendInductor(legend),
		WithNewsPublisher(news),
		WithJobPopularityTracker(popTracker),
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
	if len(popTracker.records) != 0 {
		t.Fatalf("expected 0 popularity records on transaction failure, got %d", len(popTracker.records))
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
	popTracker := &jobPopularityStub{}

	dummyRepo := &repositoryStub{}
	svc, err := NewService(
		dummyRepo,
		WithCharacterRepository(ecoCharRepo),
		WithInventoryRepository(invRepo),
		WithEconomy(ecoSvc),
		WithLegendInductor(legend),
		WithNewsPublisher(news),
		WithJobPopularityTracker(popTracker),
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
	if len(popTracker.records) != 1 {
		t.Fatalf("expected exactly 1 popularity record on transaction success, got %d", len(popTracker.records))
	}
	if popTracker.records[0].jobID != "job-01" || popTracker.records[0].points != 25 {
		t.Fatalf("expected job-01 with 25 points, got %v", popTracker.records[0])
	}
}

func TestChangeJob_NearCompleteMasteryNotifiesNewsAndLegend(t *testing.T) {
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

	charRepo := &errCharRepoStub{char: char}
	inv, _ := coreinventory.New(char.ID)
	invRepo := &errInventoryRepoStub{inv: inv}
	ecoSvc, err := economy.NewService(charRepo, invRepo)
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
		WithCharacterRepository(charRepo),
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

	updatedChar, updatedState, err := svc.ChangeJob(ctx, char.ID, "job-02")
	if err != nil {
		t.Fatalf("expected ChangeJob to succeed, got %v", err)
	}

	if updatedChar.JobID != "job-02" {
		t.Fatalf("expected job-02, got %s", updatedChar.JobID)
	}
	if updatedState.CurrentJobID != "job-02" {
		t.Fatalf("expected current job state job-02, got %s", updatedState.CurrentJobID)
	}
	if len(news.published) != 1 {
		t.Fatalf("expected exactly 1 news article, got %d: %v", len(news.published), news.published)
	}
	if len(legendCalls) != 1 {
		t.Fatalf("expected exactly 1 legend call, got %d: %v", len(legendCalls), legendCalls)
	}
}

type stalePreflightCharRepoStub struct {
	preflightChar corecharacter.Character
	lockedChar    corecharacter.Character
	updateCount   int
	updatedChar   corecharacter.Character
}

func (r *stalePreflightCharRepoStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return r.preflightChar, nil
}

func (r *stalePreflightCharRepoStub) FindByIDForUpdate(_ context.Context, _ string) (corecharacter.Character, error) {
	return r.lockedChar, nil
}

func (r *stalePreflightCharRepoStub) Update(_ context.Context, c corecharacter.Character) error {
	r.updateCount++
	r.updatedChar = c
	return nil
}

type trackingJobRepoStub struct {
	state     corejob.CharacterJob
	saveCount int
}

func (r *trackingJobRepoStub) FindByCharacterID(_ context.Context, _ string) (corejob.CharacterJob, error) {
	return r.state, nil
}

func (r *trackingJobRepoStub) Save(_ context.Context, state corejob.CharacterJob) error {
	r.saveCount++
	r.state = state
	return nil
}

type stalePreflightInvRepoStub struct {
	inv coreinventory.Inventory
}

func (r *stalePreflightInvRepoStub) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	cpy := r.inv
	cpy.Items = slices.Clone(r.inv.Items)
	return cpy, nil
}

func (r *stalePreflightInvRepoStub) FindByCharacterIDForUpdate(ctx context.Context, id string) (coreinventory.Inventory, error) {
	return r.FindByCharacterID(ctx, id)
}

func (r *stalePreflightInvRepoStub) Save(_ context.Context, inv coreinventory.Inventory) error {
	r.inv = inv
	r.inv.Items = slices.Clone(inv.Items)
	return nil
}

func TestChangeJob_StalePreflight_ActiveJobMemoryUnderLock(t *testing.T) {
	ctx := context.Background()

	cases := []struct {
		name string
		kind corecharacter.JobMemoryKind
	}{
		{name: "persistent JobMemory (item-168)", kind: corecharacter.JobMemoryKindPersistent},
		{name: "temporary JobMemory (item-243)", kind: corecharacter.JobMemoryKindTemporary},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			charID := "char-stale-preflight-" + string(tc.kind)
			baseChar := corecharacter.Character{
				ID:        charID,
				Name:      "StaleHero",
				JobID:     "job-01",
				OldJobID:  "job-04",
				Level:     50,
				Gender:    "male",
				SP:        1000,
				OverLevel: true,
			}

			lockedChar := baseChar
			lockedChar.JobMemory = &corecharacter.JobMemory{
				Kind:     tc.kind,
				JobID:    "job-03",
				SP:       150,
				OldJobID: "job-04",
				OldSP:    120,
			}

			charRepo := &stalePreflightCharRepoStub{
				preflightChar: baseChar,
				lockedChar:    lockedChar,
			}

			inv, _ := coreinventory.New(charID)
			fireArmor, _ := item.NewInstance("armor-29", 1)
			_ = inv.Add(fireArmor)
			sageItem, _ := item.NewInstance("item-027", 1)
			_ = inv.Add(sageItem)
			miscItem, _ := item.NewInstance("item-12", 1)
			_ = inv.Add(miscItem)
			invRepo := &stalePreflightInvRepoStub{inv: inv}

			equip, _ := coreequipment.New(charID)
			equip.Slots[item.SlotBody] = fireArmor.ID
			equipRepo := &equipmentRepoStub{equipment: equip}

			ecoSvc, err := economy.NewService(charRepo, invRepo)
			if err != nil {
				t.Fatal(err)
			}

			var legendCalls []legendCallEntry
			legend := LegendInductorFunc(func(_ context.Context, category, characterID string) error {
				legendCalls = append(legendCalls, legendCallEntry{Category: category, CharacterID: characterID})
				return nil
			})
			news := &mockNewsPublisher{}
			popTracker := &jobPopularityStub{}
			tracker := &jobTrackerStub{}
			guildAwarder := &guildAwarderStub{points: make(map[string]int)}
			costumeResetter := &mockJobCostumeResetter{}

			jobRepo := &trackingJobRepoStub{}
			svc, err := NewService(
				jobRepo,
				WithCharacterRepository(charRepo),
				WithInventoryRepository(invRepo),
				WithEquipmentRepository(equipRepo),
				WithEconomy(ecoSvc),
				WithLegendInductor(legend),
				WithNewsPublisher(news),
				WithJobPopularityTracker(popTracker),
				WithJobChangeTracker(tracker),
				WithGuildPointAwarder(guildAwarder),
				WithCostumeResetter(costumeResetter),
			)
			if err != nil {
				t.Fatal(err)
			}

			jobState := setupNearCompleteState(svc, charID, baseChar.JobID)
			// Explicitly unmaster job-33 (賢者) and job-08 (遊び人) so that job-33
			// is not exempt and genuinely requires and consumes item-027 (賢者の悟り).
			jobState.MasteredJobs = slices.DeleteFunc(jobState.MasteredJobs, func(id string) bool {
				return id == "job-33" || id == "job-08"
			})
			delete(jobState.MasteredJobSP, "job-33")
			delete(jobState.MasteredJobSP, "job-08")
			jobRepo.state = jobState

			// ChangeJob should evaluate preflight as clean (no JobMemory),
			// but upon acquiring character row lock in economy callback,
			// it must detect active JobMemory and abort with ErrJobUnavailable.
			// Test with job-84 (which requires and consumes equipped armor-29).
			_, _, err = svc.ChangeJob(ctx, charID, "job-84")
			if !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ErrJobUnavailable, got %v", err)
			}

			// Ensure armor-29 was NOT consumed
			if bodyArmor, ok := equip.Equipped(item.SlotBody); !ok || bodyArmor != fireArmor.ID {
				t.Fatalf("expected armor-29 to remain equipped, got %s (ok=%v)", bodyArmor, ok)
			}
			if _, found := invRepo.inv.Find(fireArmor.ID); !found {
				t.Fatal("expected armor-29 instance to remain in inventory")
			}

			// Test with job-33 (Sage / 賢者, unmastered, requires and consumes item-027 賢者の悟り).
			_, _, err = svc.ChangeJob(ctx, charID, "job-33")
			if !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ErrJobUnavailable for job-33, got %v", err)
			}
			if invRepo.inv.Quantity("item-027") != 1 {
				t.Fatalf("expected item-027 quantity 1 preserved in inventory, got %d", invRepo.inv.Quantity("item-027"))
			}
			if invRepo.inv.Quantity("item-12") != 1 {
				t.Fatalf("expected item-12 quantity 1 preserved in inventory, got %d", invRepo.inv.Quantity("item-12"))
			}

			// Ensure zero side-effects persisted or emitted
			if charRepo.updateCount != 0 {
				t.Fatalf("expected 0 character updates, got %d (saved char: %+v)", charRepo.updateCount, charRepo.updatedChar)
			}
			if jobRepo.saveCount != 0 {
				t.Fatalf("expected 0 job state saves, got %d", jobRepo.saveCount)
			}
			if len(popTracker.records) != 0 {
				t.Fatalf("expected 0 popularity records, got %d", len(popTracker.records))
			}
			if tracker.calledFor != "" {
				t.Fatalf("expected 0 tracker calls, got calledFor=%s", tracker.calledFor)
			}
			if len(news.published) != 0 {
				t.Fatalf("expected 0 news articles, got %d", len(news.published))
			}
			if len(legendCalls) != 0 {
				t.Fatalf("expected 0 legend calls, got %d", len(legendCalls))
			}
			if pts := guildAwarder.points[charID]; pts != 0 {
				t.Fatalf("expected 0 guild points awarded, got %d", pts)
			}
			if costumeResetter.calledFor != "" {
				t.Fatalf("expected costume not to be reset, but was reset for %s", costumeResetter.calledFor)
			}
		})
	}
}
