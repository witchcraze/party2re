package job

import (
	"context"
	"errors"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/economy"
)

type errRepoStub struct {
	findErr error
	saveErr error
	value   corejob.CharacterJob
}

func (r *errRepoStub) Save(_ context.Context, value corejob.CharacterJob) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.value = value
	return nil
}

func (r *errRepoStub) FindByCharacterID(_ context.Context, _ string) (corejob.CharacterJob, error) {
	if r.findErr != nil {
		return corejob.CharacterJob{}, r.findErr
	}
	return r.value, nil
}

type errCharRepoStub struct {
	findErr   error
	updateErr error
	char      corecharacter.Character
}

func (c *errCharRepoStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	if c.findErr != nil {
		return corecharacter.Character{}, c.findErr
	}
	return c.char, nil
}

func (c *errCharRepoStub) FindByIDForUpdate(_ context.Context, _ string) (corecharacter.Character, error) {
	if c.findErr != nil {
		return corecharacter.Character{}, c.findErr
	}
	return c.char, nil
}

func (c *errCharRepoStub) Update(_ context.Context, char corecharacter.Character) error {
	if c.updateErr != nil {
		return c.updateErr
	}
	c.char = char
	return nil
}

type errInventoryRepoStub struct {
	findErr error
	saveErr error
	inv     coreinventory.Inventory
}

func (r *errInventoryRepoStub) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	if r.findErr != nil {
		return coreinventory.Inventory{}, r.findErr
	}
	return r.inv, nil
}

func (r *errInventoryRepoStub) FindByCharacterIDForUpdate(_ context.Context, _ string) (coreinventory.Inventory, error) {
	if r.findErr != nil {
		return coreinventory.Inventory{}, r.findErr
	}
	return r.inv, nil
}

func (r *errInventoryRepoStub) Save(_ context.Context, value coreinventory.Inventory) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.inv = value
	return nil
}

func TestService_ConfigAndCatalog(t *testing.T) {
	// Nil repository returns error
	_, err := NewService(nil)
	if err == nil {
		t.Fatal("expected error when repository is nil")
	}

	state, _ := corejob.NewCharacterJob("char-1", "job-01")
	repo := &repositoryStub{value: state}

	// Service without WithCatalog initializes default catalog
	svc, err := NewService(repo)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if svc.catalog == nil {
		t.Fatal("expected catalog to be initialized by default")
	}
	defs := svc.ListDefinitions()
	if len(defs) == 0 {
		t.Fatal("expected non-empty definitions")
	}

	// Get existing definition
	def, err := svc.GetDefinition("job-01")
	if err != nil || def.ID != "job-01" {
		t.Fatalf("unexpected get definition: def=%v, err=%v", def, err)
	}

	// Get non-existent definition
	_, err = svc.GetDefinition("non-existent-job")
	if err == nil {
		t.Fatal("expected error for non-existent job")
	}

	// Nil catalog edge cases
	nilCatSvc := &Service{repository: repo}
	if nilCatSvc.ListDefinitions() != nil {
		t.Fatal("expected nil definitions when catalog is nil")
	}
	if _, err := nilCatSvc.GetDefinition("job-01"); !errors.Is(err, corejob.ErrDefinitionNotFound) {
		t.Fatalf("expected ErrDefinitionNotFound, got %v", err)
	}

	// SetCostumeResetter hook
	costumeResetter := &mockJobCostumeResetter{}
	svc.SetCostumeResetter(costumeResetter)
	if svc.costume != costumeResetter {
		t.Fatal("expected costume resetter to be set")
	}

	// NewsPublisherFunc
	var newsCalled bool
	var publisher NewsPublisher = NewsPublisherFunc(func(_ context.Context, _, _, _, _ string, _ time.Time) error {
		newsCalled = true
		return nil
	})
	if err := publisher.PublishNews(context.Background(), "cat", "title", "content", "author", time.Now()); err != nil || !newsCalled {
		t.Fatalf("expected NewsPublisherFunc to succeed: %v", err)
	}
}

func TestChangeJob_ValidationAndPrerequisites(t *testing.T) {
	ctx := context.Background()
	state, _ := corejob.NewCharacterJob("char-1", "job-01")
	repo := &repositoryStub{value: state}

	// Characters repo not configured
	svcNoChar, _ := NewService(repo)
	_, _, err := svcNoChar.ChangeJob(ctx, "char-1", "job-02")
	if err == nil || err.Error() != "character repository not configured" {
		t.Fatalf("expected character repository not configured error, got %v", err)
	}

	// Character lookup fails
	charRepoErr := &errCharRepoStub{findErr: errors.New("db error")}
	svcCharErr, _ := NewService(repo, WithCharacterRepository(charRepoErr), WithInventoryRepository(&errInventoryRepoStub{}))
	_, _, err = svcCharErr.ChangeJob(ctx, "char-1", "job-02")
	if err == nil || err.Error() != "db error" {
		t.Fatalf("expected db error, got %v", err)
	}

	// Target definition not found
	char := corecharacter.Character{ID: "char-1", Name: "Hero", JobID: "job-01", Level: 50, Gender: "male", OverLevel: true}
	charRepo := &charRepoStub{char: char}
	svc, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(&inventoryRepoStub{}))
	_, _, err = svc.ChangeJob(ctx, "char-1", "non-existent-job")
	if err == nil {
		t.Fatal("expected error for non-existent target job")
	}

	// Character has JobMemory (cannot change job while in temporary memory)
	charWithMemory := char
	charWithMemory.JobMemory = &corecharacter.JobMemory{JobID: "job-03", SP: 50}
	charRepoMem := &charRepoStub{char: charWithMemory}
	svcMem, _ := NewService(repo, WithCharacterRepository(charRepoMem), WithInventoryRepository(&inventoryRepoStub{}))
	_, _, err = svcMem.ChangeJob(ctx, "char-1", "job-02")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when JobMemory is present, got %v", err)
	}

	// Level requirement not met (job-02 requires level 20, char is level 5)
	charLowLevel := char
	charLowLevel.Level = 5
	charRepoLow := &charRepoStub{char: charLowLevel}
	svcLow, _ := NewService(repo, WithCharacterRepository(charRepoLow), WithInventoryRepository(&inventoryRepoStub{}))
	_, _, err = svcLow.ChangeJob(ctx, "char-1", "job-02")
	if err == nil {
		t.Fatal("expected level requirement error")
	}

	// Gender requirement not met (job-81 踊り子 requires female)
	charMale := char
	charMale.Level = 50
	charMale.Gender = "male"
	charRepoMale := &charRepoStub{char: charMale}
	svcMale, _ := NewService(repo, WithCharacterRepository(charRepoMale), WithInventoryRepository(&inventoryRepoStub{}))
	_, _, err = svcMale.ChangeJob(ctx, "char-1", "job-81")
	if err == nil {
		t.Fatal("expected gender requirement error for job-81")
	}
}

func TestChangeJob_ItemRequirementsAndExemptions(t *testing.T) {
	ctx := context.Background()

	// job-34 (勇者) requires item-028 (勇者の証)
	setupSvc := func(char corecharacter.Character, state corejob.CharacterJob, inv *coreinventory.Inventory) (*Service, *repositoryStub, *charRepoStub, *inventoryRepoStub) {
		repo := &repositoryStub{value: state}
		charRepo := &charRepoStub{char: char}
		var invRepo *inventoryRepoStub
		var opts []Option
		opts = append(opts, WithCharacterRepository(charRepo))
		if inv != nil {
			invRepo = &inventoryRepoStub{inventory: *inv}
		} else {
			emptyInv, _ := coreinventory.New(char.ID)
			invRepo = &inventoryRepoStub{inventory: emptyInv}
		}
		opts = append(opts, WithInventoryRepository(invRepo))
		svc, _ := NewService(repo, opts...)
		return svc, repo, charRepo, invRepo
	}

	// 1. Target job == char.JobID -> no item needed
	char := corecharacter.Character{ID: "char-1", Name: "Hero", JobID: "job-34", Level: 50, Gender: "male", OverLevel: true}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, _ := setupSvc(char, state, nil)
	updatedChar, _, err := svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("expected no item needed when target is current job: %v", err)
	}
	if updatedChar.JobID != "job-34" {
		t.Fatalf("expected job-34, got %s", updatedChar.JobID)
	}

	// 2. Target job == char.OldJobID -> no item needed
	char.JobID = "job-01"
	char.OldJobID = "job-34"
	char.OldSP = 50
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, _ = setupSvc(char, state, nil)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("expected no item needed when target is old job: %v", err)
	}
	if updatedChar.JobID != "job-34" || updatedChar.SP != 50 {
		t.Fatalf("expected job-34 with restored SP 50, got job=%s SP=%d", updatedChar.JobID, updatedChar.SP)
	}

	// 3. Target job is already mastered (outside current/old) -> still requires prerequisites and item
	// Regression check for #1158: past mastery restores SP on success, never bypasses requirements.
	char = corecharacter.Character{
		ID: "char-1", Name: "Hero", JobID: "job-01", OldJobID: "job-02",
		Level: 50, Gender: "male", SP: 45, OldSP: 25, OverLevel: true,
	}
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	state.RecordMastery("job-34", 180, 180) // job-34 mastered
	svc, repo, charRepo, _ := setupSvc(char, state, nil)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when mastered target job-34 lacks HeroCount >= 5, got %v", err)
	}
	if charRepo.char.JobID != "job-01" || charRepo.char.SP != 45 || repo.value.CurrentJobID != "job-01" {
		t.Fatalf("state mutated on rejection: char=%+v state=%+v", charRepo.char, repo.value)
	}

	char.HeroCount = 5
	svc, repo, charRepo, _ = setupSvc(char, state, nil)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when mastered target job-34 lacks item-028, got %v", err)
	}
	if charRepo.char.JobID != "job-01" || charRepo.char.SP != 45 || repo.value.CurrentJobID != "job-01" {
		t.Fatalf("state mutated on ErrRequiredItem: char=%+v state=%+v", charRepo.char, repo.value)
	}

	invMastered, _ := coreinventory.New(char.ID)
	heroProof, _ := item.NewInstance("item-028", 1)
	_ = invMastered.Add(heroProof)
	svc, _, _, invMasteredRepo := setupSvc(char, state, &invMastered)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("expected mastered job-34 with HeroCount>=5 and item-028 to succeed: %v", err)
	}
	if updatedChar.JobID != "job-34" || updatedChar.SP != 180 || invMasteredRepo.inventory.Quantity("item-028") != 0 {
		t.Fatalf("expected job-34 with mastered SP 180 and item-028 consumed, got job=%s SP=%d qty=%d",
			updatedChar.JobID, updatedChar.SP, invMasteredRepo.inventory.Quantity("item-028"))
	}
	char.HeroCount = 0

	// 4. job-33 (賢者) access when old/current job is job-08 (遊び人) -> no item needed and item preserved
	// Legacy: &_is_need_job(8, 33) means current or old job must be job-08 or job-33.
	char.JobID = "job-01"
	char.OldJobID = "job-08"
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, _ = setupSvc(char, state, nil)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-33")
	if err != nil {
		t.Fatalf("expected job-33 access when old job is job-08: %v", err)
	}
	if updatedChar.JobID != "job-33" {
		t.Fatalf("expected job-33, got %s", updatedChar.JobID)
	}

	char.JobID = "job-08"
	char.OldJobID = "job-02"
	invSageBook, _ := coreinventory.New(char.ID)
	sageBook, _ := item.NewInstance("item-027", 1)
	_ = invSageBook.Add(sageBook)
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, invSageRepo := setupSvc(char, state, &invSageBook)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-33")
	if err != nil {
		t.Fatalf("expected job-33 access when current job is job-08: %v", err)
	}
	if updatedChar.JobID != "job-33" || invSageRepo.inventory.Quantity("item-027") != 1 {
		t.Fatalf("expected job-33 without consuming item-027, got job=%s qty=%d", updatedChar.JobID, invSageRepo.inventory.Quantity("item-027"))
	}
	char.JobID = "job-01"
	char.OldJobID = "job-02" // restore

	// 5. job-46 (ギャンブラー) requires item-039 AND CasinoWins >= 10
	// 5a. CasinoWins < 10 -> ErrJobUnavailable even if holding item-039
	char.JobID = "job-01"
	char.OldJobID = ""
	char.CasinoWins = 9
	invWithCard, _ := coreinventory.New(char.ID)
	cardToken, _ := item.NewInstance("item-039", 1)
	_ = invWithCard.Add(cardToken)
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, _ = setupSvc(char, state, &invWithCard)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when CasinoWins < 10, got %v", err)
	}

	// 5b. CasinoWins >= 10 but missing item-039 -> ErrRequiredItem
	char.CasinoWins = 10
	invEmpty, _ := coreinventory.New(char.ID)
	svc, _, _, _ = setupSvc(char, state, &invEmpty)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when item-039 missing, got %v", err)
	}

	// 5c. CasinoWins >= 10 and holding item-039 -> succeeds and consumes item-039
	svc, _, _, invRepoStub := setupSvc(char, state, &invWithCard)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
	if err != nil {
		t.Fatalf("expected job-46 change to succeed with CasinoWins >= 10 and item-039: %v", err)
	}
	if updatedChar.JobID != "job-46" {
		t.Fatalf("expected job-46, got %s", updatedChar.JobID)
	}
	if invRepoStub.inventory.Quantity("item-039") != 0 {
		t.Fatalf("expected item-039 to be consumed, remaining %d", invRepoStub.inventory.Quantity("item-039"))
	}

	// 6. When item IS needed, but inventories is nil -> ErrRequiredItem
	// job-34 (勇者) also requires HeroCount >= 5.
	char.HeroCount = 5
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	svc, _, _, _ = setupSvc(char, state, nil)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when inventory is nil, got %v", err)
	}

	// 7. When item IS needed, but inventory has 0 copies -> ErrRequiredItem
	inv, _ := coreinventory.New(char.ID)
	svc, _, _, _ = setupSvc(char, state, &inv)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when item count is 0, got %v", err)
	}

	// 8. When item IS needed and inventory has it -> item consumed and success
	token, _ := item.NewInstance("item-028", 2)
	_ = inv.Add(token)
	svc, _, _, invRepo := setupSvc(char, state, &inv)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("unexpected error when item is present: %v", err)
	}
	if updatedChar.JobID != "job-34" {
		t.Fatalf("expected job-34, got %s", updatedChar.JobID)
	}
	if invRepo.inventory.Quantity("item-028") != 1 {
		t.Fatalf("expected 1 item-028 remaining, got %d", invRepo.inventory.Quantity("item-028"))
	}
}

func TestChangeJob_EconomyBranches(t *testing.T) {
	ctx := context.Background()
	// job-34 (Hero) requires HeroCount >= 5 and item-028 when changing fresh.
	char := corecharacter.Character{ID: "char-eco", Name: "EcoHero", JobID: "job-01", Level: 50, Gender: "male", OverLevel: true, HeroCount: 5}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}

	inv, _ := coreinventory.New(char.ID)
	invRepo := &errInventoryRepoStub{inv: inv}
	ecoCharRepo := &errCharRepoStub{char: char}
	ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := NewService(
		repo,
		WithCharacterRepository(charRepo),
		WithInventoryRepository(invRepo),
		WithEconomy(ecoSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Missing item in economy mode -> ErrRequiredItem
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem in economy mode, got %v", err)
	}

	// 2. Having item in economy mode -> success and item consumed
	token, _ := item.NewInstance("item-028", 1)
	_ = invRepo.inv.Add(token)
	updatedChar, _, err := svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("ChangeJob failed in economy mode: %v", err)
	}
	if updatedChar.JobID != "job-34" {
		t.Fatalf("expected job-34, got %s", updatedChar.JobID)
	}
	if invRepo.inv.Quantity("item-028") != 0 {
		t.Fatalf("expected item-028 consumed, got %d", invRepo.inv.Quantity("item-028"))
	}

	// 5. job-46 in economy mode: CasinoWins < 10 returns ErrJobUnavailable
	charRepo.char.JobID = "job-01"
	charRepo.char.Level = 50
	charRepo.char.CasinoWins = 5
	ecoCharRepo.char.JobID = "job-01"
	ecoCharRepo.char.Level = 50
	ecoCharRepo.char.CasinoWins = 5
	card, _ := item.NewInstance("item-039", 1)
	_ = invRepo.inv.Add(card)
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-46")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable in economy mode for job-46 when CasinoWins < 10, got %v", err)
	}

	// 6. job-46 in economy mode: CasinoWins >= 10 succeeds and consumes item-039
	charRepo.char.CasinoWins = 10
	ecoCharRepo.char.CasinoWins = 10
	updatedGambler, _, err := svc.ChangeJob(ctx, char.ID, "job-46")
	if err != nil {
		t.Fatalf("expected job-46 change to succeed in economy mode: %v", err)
	}
	if updatedGambler.JobID != "job-46" {
		t.Fatalf("expected job-46, got %s", updatedGambler.JobID)
	}
	if invRepo.inv.Quantity("item-039") != 0 {
		t.Fatalf("expected item-039 consumed in economy mode, got %d", invRepo.inv.Quantity("item-039"))
	}
}

func TestChangeJob_GuildPointsAndNews(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{ID: "char-1", Name: "MasterHero", JobID: "job-01", Level: 50, Gender: "male", OverLevel: true}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	awarder := &guildAwarderStub{points: make(map[string]int)}
	news := &mockNewsPublisher{}

	svc, err := NewService(
		repo,
		WithCharacterRepository(charRepo),
		WithInventoryRepository(&inventoryRepoStub{}),
		WithGuildPointAwarder(awarder),
		WithNewsPublisher(news),
	)
	if err != nil {
		t.Fatal(err)
	}

	// Award guild points on job change
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-02")
	if err != nil {
		t.Fatalf("ChangeJob failed: %v", err)
	}
	if awarder.points[char.ID] != 50 {
		t.Fatalf("expected 50 guild points, got %d", awarder.points[char.ID])
	}

	// All jobs mastered announcement
	catalog := svc.catalog
	for _, def := range catalog.Definitions() {
		state.RecordMastery(def.ID, def.MasteryThreshold(), def.MasteryThreshold())
	}
	repo.value = state
	charRepo.char.Level = 50
	_, _, err = svc.ChangeJob(ctx, char.ID, "job-03")
	if err != nil {
		t.Fatalf("ChangeJob failed: %v", err)
	}
	if len(news.published) == 0 {
		t.Fatal("expected completion news to be published")
	}
}

func TestExchangeJob_Comprehensive(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{
		ID:       "char-ex",
		Name:     "ExHero",
		JobID:    "job-01",
		OldJobID: "job-02",
		SP:       50,
		OldSP:    40,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	state.RecordMastery("job-03", 80, 80)
	state.RecordMastery("job-04", 100, 100)

	// 1. Characters repository not configured
	svcNoChar, _ := NewService(&repositoryStub{value: state})
	_, _, err := svcNoChar.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if err == nil || err.Error() != "character repository not configured" {
		t.Fatalf("expected character repository not configured, got %v", err)
	}

	// 2. Character lookup error
	charRepoErr := &errCharRepoStub{findErr: errors.New("char find err")}
	svcCharErr, _ := NewService(&repositoryStub{value: state}, WithCharacterRepository(charRepoErr), WithInventoryRepository(&errInventoryRepoStub{}))
	_, _, err = svcCharErr.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if err == nil || err.Error() != "char find err" {
		t.Fatalf("expected char find err, got %v", err)
	}

	// 3. Restore from JobMemory when JobMemory != nil
	charWithMemory := char
	charWithMemory.JobMemory = &corecharacter.JobMemory{
		JobID:    "job-10",
		SP:       70,
		OldJobID: "job-11",
		OldSP:    60,
	}
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: charWithMemory}
	svc, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(&inventoryRepoStub{}))
	restoredChar, _, err := svc.ExchangeJob(ctx, char.ID, "ignored", "ignored")
	if err != nil {
		t.Fatalf("ExchangeJob restore failed: %v", err)
	}
	if restoredChar.JobMemory != nil {
		t.Fatal("expected JobMemory to be cleared after restoration")
	}
	if restoredChar.JobID != "job-10" || restoredChar.SP != 70 || restoredChar.OldJobID != "job-11" || restoredChar.OldSP != 60 {
		t.Fatalf("unexpected restored jobs: %#v", restoredChar)
	}

	// 4. Invalid target job IDs
	testCases := []struct {
		name      string
		target    string
		targetOld string
	}{
		{"empty target", "", "job-04"},
		{"empty targetOld", "job-03", ""},
		{"same targets", "job-03", "job-03"},
		{"target not recorded", "job-05", "job-04"},
		{"targetOld not recorded", "job-03", "job-05"},
	}
	charRepo = &charRepoStub{char: char}
	svc, _ = NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(&inventoryRepoStub{}))
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.ExchangeJob(ctx, char.ID, tc.target, tc.targetOld)
			if !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ErrJobUnavailable, got %v", err)
			}
		})
	}

	// 5. Calling ExchangeJob when s.economy == nil returns "economy service not configured"
	svcNoEco, _ := NewService(repo, WithCharacterRepository(charRepo))
	_, _, err = svcNoEco.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if err == nil || err.Error() != "economy service not configured" {
		t.Fatalf("expected economy service not configured error, got %v", err)
	}

	// 6. Missing item-168 -> ErrRequiredItem
	inv, _ := coreinventory.New(char.ID)
	invRepo := &inventoryRepoStub{inventory: inv}
	svcInv, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))
	_, _, err = svcInv.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when item-168 is missing, got %v", err)
	}

	// 7. Item-168 present -> success, memory saved, item consumed
	book, _ := item.NewInstance("item-168", 1)
	_ = invRepo.inventory.Add(book)
	exchangedChar, _, err := svcInv.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if err != nil {
		t.Fatalf("ExchangeJob failed: %v", err)
	}
	if exchangedChar.JobID != "job-03" || exchangedChar.SP != 80 || exchangedChar.OldJobID != "job-04" || exchangedChar.OldSP != 100 {
		t.Fatalf("unexpected exchanged jobs: %#v", exchangedChar)
	}
	if exchangedChar.JobMemory == nil || exchangedChar.JobMemory.JobID != "job-01" || exchangedChar.JobMemory.OldJobID != "job-02" {
		t.Fatalf("expected previous jobs stored in JobMemory: %#v", exchangedChar.JobMemory)
	}
	if invRepo.inventory.Quantity("item-168") != 0 {
		t.Fatalf("expected item-168 consumed, got %d", invRepo.inventory.Quantity("item-168"))
	}

	// 8. Economy mode: missing item-168 -> ErrRequiredItem; having item-168 -> success
	ecoInv, _ := coreinventory.New(char.ID)
	ecoInvRepo := &errInventoryRepoStub{inv: ecoInv}
	ecoCharRepo := &errCharRepoStub{char: char}
	ecoSvc, _ := economy.NewService(ecoCharRepo, ecoInvRepo)
	svcEco, _ := NewService(repo, WithCharacterRepository(ecoCharRepo), WithInventoryRepository(ecoInvRepo), WithEconomy(ecoSvc))

	_, _, err = svcEco.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem in economy mode, got %v", err)
	}

	_ = ecoInvRepo.inv.Add(book)
	ecoExchanged, _, err := svcEco.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if err != nil {
		t.Fatalf("ExchangeJob failed in economy mode: %v", err)
	}
	if ecoExchanged.JobID != "job-03" || ecoExchanged.JobMemory == nil {
		t.Fatalf("unexpected economy exchanged character: %#v", ecoExchanged)
	}
}

func TestChangeJob_RepositoryErrorPropagation(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{ID: "char-1", JobID: "job-01", Level: 20, Gender: "unspecified"}

	// 1. Unexpected repository FindByCharacterID error is propagated (not treated as not-found).
	findErr := errors.New("repo find error")
	svcFindErr, _ := NewService(
		&errRepoStub{findErr: findErr},
		WithCharacterRepository(&charRepoStub{char: char}),
		WithInventoryRepository(&inventoryRepoStub{}),
	)
	if _, _, err := svcFindErr.ChangeJob(ctx, char.ID, "job-02"); !errors.Is(err, findErr) {
		t.Fatalf("expected repo find error, got %v", err)
	}

	// 2. Repository Save error is propagated.
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	saveErr := errors.New("save error")
	svcSaveErr, _ := NewService(
		&errRepoStub{value: state, saveErr: saveErr},
		WithCharacterRepository(&charRepoStub{char: char}),
		WithInventoryRepository(&inventoryRepoStub{}),
	)
	if _, _, err := svcSaveErr.ChangeJob(ctx, char.ID, "job-02"); !errors.Is(err, saveErr) {
		t.Fatalf("expected save error, got %v", err)
	}

	// 3. Unknown departing CurrentJobID does not fail or master.
	unknownChar := corecharacter.Character{ID: "char-unknown", JobID: "unknown-job", Level: 20, SP: 50, Gender: "unspecified"}
	unknownState, _ := corejob.NewCharacterJob(unknownChar.ID, unknownChar.JobID)
	repoUnknown := &repositoryStub{value: unknownState}
	svcUnknown, _ := NewService(
		repoUnknown,
		WithCharacterRepository(&charRepoStub{char: unknownChar}),
		WithInventoryRepository(&inventoryRepoStub{}),
	)
	if _, got, err := svcUnknown.ChangeJob(ctx, unknownChar.ID, "job-01"); err != nil || got.IsMastered("unknown-job") {
		t.Fatalf("unexpected result for unknown departing job: err=%v, state=%#v", err, got)
	}
}

func TestExchangeJob_OverLevel_Rejected(t *testing.T) {
	ctx := context.Background()
	char := corecharacter.Character{
		ID:        "char-overlevel",
		Name:      "ReincarnatedHero",
		JobID:     "job-01",
		OldJobID:  "job-02",
		SP:        50,
		OldSP:     40,
		OverLevel: true,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	state.RecordMastery("job-03", 80, 80)
	state.RecordMastery("job-04", 100, 100)

	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	inv, _ := coreinventory.New(char.ID)
	crystal, _ := item.NewInstance("item-168", 1)
	_ = inv.Add(crystal)
	invRepo := &inventoryRepoStub{inventory: inv}

	svc, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))

	// 1. OverLevel character cannot exchange jobs (item-168 present, mastered jobs)
	_, _, err := svc.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable for OverLevel character, got %v", err)
	}

	// 2. OverLevel character cannot exchange jobs in economy mode
	ecoCharRepo := &errCharRepoStub{char: char}
	ecoInvRepo := &errInventoryRepoStub{inv: inv}
	ecoSvc, _ := economy.NewService(ecoCharRepo, ecoInvRepo)
	svcEco, _ := NewService(repo, WithCharacterRepository(ecoCharRepo), WithInventoryRepository(ecoInvRepo), WithEconomy(ecoSvc))
	_, _, err = svcEco.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable for OverLevel character in economy mode, got %v", err)
	}

	// 3. OverLevel character cannot restore previous job memory
	charWithMemory := char
	charWithMemory.JobMemory = &corecharacter.JobMemory{
		JobID:    "job-01",
		SP:       50,
		OldJobID: "job-02",
		OldSP:    40,
	}
	charRepoMemory := &charRepoStub{char: charWithMemory}
	svcMemory, _ := NewService(repo, WithCharacterRepository(charRepoMemory), WithInventoryRepository(invRepo))
	_, _, err = svcMemory.ExchangeJob(ctx, char.ID, "ignored", "ignored")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when restoring job memory for OverLevel character, got %v", err)
	}
}

func TestExchangeJob_GenderMismatch_Rejected(t *testing.T) {
	ctx := context.Background()

	// Male character
	maleChar := corecharacter.Character{
		ID:        "char-male",
		Name:      "MaleHero",
		Gender:    "m",
		JobID:     "job-01",
		OldJobID:  "job-02",
		SP:        50,
		OldSP:     40,
		OverLevel: false,
	}
	maleState, _ := corejob.NewCharacterJob(maleChar.ID, maleChar.JobID)
	maleState.RecordMastery("job-03", 80, 80)
	maleState.RecordMastery("job-13", 90, 90)   // 吟遊詩人: required_gender = "m"
	maleState.RecordMastery("job-14", 100, 100) // 踊り子: required_gender = "f"
	maleState.RecordMastery("job-16", 140, 140) // 白魔術師: required_gender = "f"

	invMale, _ := coreinventory.New(maleChar.ID)
	crystal, _ := item.NewInstance("item-168", 10)
	_ = invMale.Add(crystal)

	maleRepo := &repositoryStub{value: maleState}
	maleCharRepo := &charRepoStub{char: maleChar}
	maleInvRepo := &inventoryRepoStub{inventory: invMale}
	svcMale, _ := NewService(maleRepo, WithCharacterRepository(maleCharRepo), WithInventoryRepository(maleInvRepo))

	// Male attempting to recall female-exclusive job as targetJobID
	_, _, err := svcMale.ExchangeJob(ctx, maleChar.ID, "job-14", "job-03")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when male recalls female-exclusive job-14 as targetJobID, got %v", err)
	}

	// Male attempting to recall female-exclusive job as targetOldJobID
	_, _, err = svcMale.ExchangeJob(ctx, maleChar.ID, "job-03", "job-14")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when male recalls female-exclusive job-14 as targetOldJobID, got %v", err)
	}

	// Male attempting to recall female-exclusive job-16
	_, _, err = svcMale.ExchangeJob(ctx, maleChar.ID, "job-16", "job-03")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when male recalls female-exclusive job-16, got %v", err)
	}

	// Male recalling male-exclusive job-13 and neutral job-03 succeeds
	maleUpdated, _, err := svcMale.ExchangeJob(ctx, maleChar.ID, "job-13", "job-03")
	if err != nil {
		t.Fatalf("expected male recalling job-13 to succeed, got %v", err)
	}
	if maleUpdated.JobID != "job-13" || maleUpdated.OldJobID != "job-03" {
		t.Fatalf("unexpected jobs after exchange: JobID=%s, OldJobID=%s", maleUpdated.JobID, maleUpdated.OldJobID)
	}

	// Female character
	femaleChar := corecharacter.Character{
		ID:        "char-female",
		Name:      "FemaleHero",
		Gender:    "f",
		JobID:     "job-01",
		OldJobID:  "job-02",
		SP:        50,
		OldSP:     40,
		OverLevel: false,
	}
	femaleState, _ := corejob.NewCharacterJob(femaleChar.ID, femaleChar.JobID)
	femaleState.RecordMastery("job-03", 80, 80)
	femaleState.RecordMastery("job-13", 90, 90)   // 吟遊詩人: required_gender = "m"
	femaleState.RecordMastery("job-14", 100, 100) // 踊り子: required_gender = "f"
	femaleState.RecordMastery("job-47", 160, 160) // ソルジャー: required_gender = "m"

	invFemale, _ := coreinventory.New(femaleChar.ID)
	crystalFemale, _ := item.NewInstance("item-168", 10)
	_ = invFemale.Add(crystalFemale)

	femaleRepo := &repositoryStub{value: femaleState}
	femaleCharRepo := &charRepoStub{char: femaleChar}
	femaleInvRepo := &inventoryRepoStub{inventory: invFemale}
	svcFemale, _ := NewService(femaleRepo, WithCharacterRepository(femaleCharRepo), WithInventoryRepository(femaleInvRepo))

	// Female attempting to recall male-exclusive job-13 as targetJobID
	_, _, err = svcFemale.ExchangeJob(ctx, femaleChar.ID, "job-13", "job-03")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when female recalls male-exclusive job-13 as targetJobID, got %v", err)
	}

	// Female attempting to recall male-exclusive job-13 as targetOldJobID
	_, _, err = svcFemale.ExchangeJob(ctx, femaleChar.ID, "job-03", "job-13")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when female recalls male-exclusive job-13 as targetOldJobID, got %v", err)
	}

	// Female attempting to recall male-exclusive job-47
	_, _, err = svcFemale.ExchangeJob(ctx, femaleChar.ID, "job-47", "job-03")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when female recalls male-exclusive job-47, got %v", err)
	}

	// Female recalling female-exclusive job-14 and neutral job-03 succeeds
	femaleUpdated, _, err := svcFemale.ExchangeJob(ctx, femaleChar.ID, "job-14", "job-03")
	if err != nil {
		t.Fatalf("expected female recalling job-14 to succeed, got %v", err)
	}
	if femaleUpdated.JobID != "job-14" || femaleUpdated.OldJobID != "job-03" {
		t.Fatalf("unexpected jobs after exchange: JobID=%s, OldJobID=%s", femaleUpdated.JobID, femaleUpdated.OldJobID)
	}
}

// TestChangeJob_MasteredTargetStillRequiresPrerequisitesAndLeavesStateUnchangedOnReject verifies
// #1158: mastering a target job in the past restores its retained SP upon a valid change,
// but never exempts job-tree, milestone counter, level, or gender checks when the target is
// neither the character's current nor previous job.
func TestChangeJob_MasteredTargetStillRequiresPrerequisitesAndLeavesStateUnchangedOnReject(t *testing.T) {
	ctx := context.Background()

	type testCase struct {
		name         string
		char         corecharacter.Character
		targetJobID  string
		masteredSP   int
		masteryLimit int
		wantErr      error
		wantSP       int
	}

	cases := []testCase{
		{
			name: "mastered_prerequisite_tree_rejected",
			char: corecharacter.Character{
				ID: "char-reg-1", Name: "Tester", JobID: "job-01", OldJobID: "job-02",
				Level: 50, Gender: "m", SP: 77, OldSP: 33, OverLevel: true,
			},
			targetJobID:  "job-23", // 竜騎士 requires current/old in [job-09, job-11, job-23, job-26]
			masteredSP:   130,
			masteryLimit: 130,
			wantErr:      corejob.ErrJobUnavailable,
		},
		{
			name: "mastered_monster_kills_milestone_rejected",
			char: corecharacter.Character{
				ID: "char-reg-2", Name: "Tester", JobID: "job-01", OldJobID: "job-04",
				Level: 50, Gender: "m", SP: 60, OldSP: 20, MonsterKills: 200, OverLevel: true,
			},
			targetJobID:  "job-21", // バーサーカー requires MonsterKills > 200 unless current/old is job-21
			masteredSP:   110,
			masteryLimit: 110,
			wantErr:      corejob.ErrJobUnavailable,
		},
		{
			name: "mastered_sp_milestone_rejected",
			char: corecharacter.Character{
				ID: "char-reg-3", Name: "Tester", JobID: "job-01", OldJobID: "job-02",
				Level: 50, Gender: "m", SP: 100, OldSP: 40, OverLevel: true,
			},
			targetJobID:  "job-49", // 若芽剣士 requires SP >= 300 unless current/old is job-49
			masteredSP:   300,
			masteryLimit: 300,
			wantErr:      corejob.ErrJobUnavailable,
		},
		{
			name: "mastered_gender_mismatch_rejected",
			char: corecharacter.Character{
				ID: "char-reg-4", Name: "Tester", JobID: "job-01", OldJobID: "job-02",
				Level: 50, Gender: "f", SP: 55, OldSP: 15, OverLevel: true,
			},
			targetJobID:  "job-13", // requires gender "m"
			masteredSP:   90,
			masteryLimit: 90,
			wantErr:      corejob.ErrJobUnavailable,
		},
		{
			name: "mastered_prerequisite_tree_satisfied_restores_sp",
			char: corecharacter.Character{
				ID: "char-reg-5", Name: "Tester", JobID: "job-09", OldJobID: "job-02",
				Level: 50, Gender: "m", SP: 77, OldSP: 33, OverLevel: true,
			},
			targetJobID:  "job-23",
			masteredSP:   135,
			masteryLimit: 130,
			wantSP:       135,
		},
		{
			name: "mastered_old_job_exempts_milestone_counter",
			char: corecharacter.Character{
				ID: "char-reg-6", Name: "Tester", JobID: "job-01", OldJobID: "job-74",
				Level: 50, Gender: "m", SP: 60, OldSP: 155, PvPWins: 0, OverLevel: true,
			},
			targetJobID:  "job-74", // 剣闘士: _is_need_job(74) || (PvPWins > 30 && _is_need_job(2,3,4,8))
			masteredSP:   150,
			masteryLimit: 150,
			wantSP:       155,
		},
		{
			name: "mastered_gladiator_outside_current_old_still_requires_pvp_wins",
			char: corecharacter.Character{
				ID: "char-reg-7", Name: "Tester", JobID: "job-02", OldJobID: "job-03",
				Level: 50, Gender: "m", SP: 60, OldSP: 20, PvPWins: 0, OverLevel: true,
			},
			targetJobID:  "job-74",
			masteredSP:   150,
			masteryLimit: 150,
			wantErr:      corejob.ErrJobUnavailable,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			state, _ := corejob.NewCharacterJob(tc.char.ID, tc.char.JobID)
			state.RecordMastery(tc.targetJobID, tc.masteredSP, tc.masteryLimit)
			repo := &repositoryStub{value: state}
			ecoCharRepo := &errCharRepoStub{char: tc.char}

			inv, _ := coreinventory.New(tc.char.ID)
			sampleItem, _ := item.NewInstance("item-001", 2)
			_ = inv.Add(sampleItem)
			invRepo := &errInventoryRepoStub{inv: inv}

			ecoSvc, err := economy.NewService(ecoCharRepo, invRepo)
			if err != nil {
				t.Fatal(err)
			}
			svc, err := NewService(
				repo,
				WithCharacterRepository(ecoCharRepo),
				WithInventoryRepository(invRepo),
				WithEconomy(ecoSvc),
			)
			if err != nil {
				t.Fatal(err)
			}

			updatedChar, _, err := svc.ChangeJob(ctx, tc.char.ID, tc.targetJobID)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				savedChar, findErr := ecoCharRepo.FindByID(ctx, tc.char.ID)
				if findErr != nil {
					t.Fatal(findErr)
				}
				if savedChar.JobID != tc.char.JobID || savedChar.OldJobID != tc.char.OldJobID ||
					savedChar.SP != tc.char.SP || savedChar.OldSP != tc.char.OldSP {
					t.Fatalf("character state mutated on rejection: got=%+v want=%+v", savedChar, tc.char)
				}
				if repo.value.CurrentJobID != tc.char.JobID {
					t.Fatalf("job state CurrentJobID mutated to %s, expected %s", repo.value.CurrentJobID, tc.char.JobID)
				}
				if retained, ok := repo.value.SavedSP(tc.targetJobID); !ok || retained != tc.masteredSP {
					t.Fatalf("mastered SP mutated on rejection: retained=%d ok=%v", retained, ok)
				}
				if invRepo.inv.Quantity("item-001") != 2 {
					t.Fatalf("inventory mutated on rejection: qty=%d", invRepo.inv.Quantity("item-001"))
				}
				return
			}

			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
			if updatedChar.JobID != tc.targetJobID || updatedChar.SP != tc.wantSP {
				t.Fatalf("unexpected updated character: JobID=%s SP=%d (want job=%s SP=%d)",
					updatedChar.JobID, updatedChar.SP, tc.targetJobID, tc.wantSP)
			}
		})
	}
}
