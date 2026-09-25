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
	svcCharErr, _ := NewService(repo, WithCharacterRepository(charRepoErr))
	_, _, err = svcCharErr.ChangeJob(ctx, "char-1", "job-02")
	if err == nil || err.Error() != "db error" {
		t.Fatalf("expected db error, got %v", err)
	}

	// Target definition not found
	char := corecharacter.Character{ID: "char-1", Name: "Hero", JobID: "job-01", Level: 50, Gender: "male", OverLevel: true}
	charRepo := &charRepoStub{char: char}
	svc, _ := NewService(repo, WithCharacterRepository(charRepo))
	_, _, err = svc.ChangeJob(ctx, "char-1", "non-existent-job")
	if err == nil {
		t.Fatal("expected error for non-existent target job")
	}

	// Character has JobMemory (cannot change job while in temporary memory)
	charWithMemory := char
	charWithMemory.JobMemory = &corecharacter.JobMemory{JobID: "job-03", SP: 50}
	charRepoMem := &charRepoStub{char: charWithMemory}
	svcMem, _ := NewService(repo, WithCharacterRepository(charRepoMem))
	_, _, err = svcMem.ChangeJob(ctx, "char-1", "job-02")
	if !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("expected ErrJobUnavailable when JobMemory is present, got %v", err)
	}

	// Level requirement not met (job-02 requires level 20, char is level 5)
	charLowLevel := char
	charLowLevel.Level = 5
	charRepoLow := &charRepoStub{char: charLowLevel}
	svcLow, _ := NewService(repo, WithCharacterRepository(charRepoLow))
	_, _, err = svcLow.ChangeJob(ctx, "char-1", "job-02")
	if err == nil {
		t.Fatal("expected level requirement error")
	}

	// Gender requirement not met (job-81 踊り子 requires female)
	charMale := char
	charMale.Level = 50
	charMale.Gender = "male"
	charRepoMale := &charRepoStub{char: charMale}
	svcMale, _ := NewService(repo, WithCharacterRepository(charRepoMale))
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
			opts = append(opts, WithInventoryRepository(invRepo))
		}
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

	// 3. Target job is already mastered -> no item needed
	char.JobID = "job-01"
	char.OldJobID = "job-02"
	state, _ = corejob.NewCharacterJob(char.ID, char.JobID)
	state.RecordMastery("job-34", 180, 180) // job-34 mastered
	svc, _, _, _ = setupSvc(char, state, nil)
	updatedChar, _, err = svc.ChangeJob(ctx, char.ID, "job-34")
	if err != nil {
		t.Fatalf("expected no item needed when target is mastered: %v", err)
	}
	if updatedChar.JobID != "job-34" || updatedChar.SP != 180 {
		t.Fatalf("expected job-34 with mastered SP 180, got job=%s SP=%d", updatedChar.JobID, updatedChar.SP)
	}

	// 4. job-33 (賢者) access when old job is job-08 (遊び人) -> no item needed
	// Legacy: &_is_need_job(8, 33) means current or old job must be job-08 or job-33.
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
	svcCharErr, _ := NewService(&repositoryStub{value: state}, WithCharacterRepository(charRepoErr))
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
	svc, _ := NewService(repo, WithCharacterRepository(charRepo))
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
		{"target not mastered", "job-05", "job-04"},
		{"targetOld not mastered", "job-03", "job-05"},
	}
	charRepo = &charRepoStub{char: char}
	svc, _ = NewService(repo, WithCharacterRepository(charRepo))
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := svc.ExchangeJob(ctx, char.ID, tc.target, tc.targetOld)
			if !errors.Is(err, corejob.ErrJobUnavailable) {
				t.Fatalf("expected ErrJobUnavailable, got %v", err)
			}
		})
	}

	// 5. Non-economy mode: inventory is nil -> ErrRequiredItem
	_, _, err = svc.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when inventory is nil, got %v", err)
	}

	// 6. Non-economy mode: item-168 missing -> ErrRequiredItem
	inv, _ := coreinventory.New(char.ID)
	invRepo := &inventoryRepoStub{inventory: inv}
	svcInv, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))
	_, _, err = svcInv.ExchangeJob(ctx, char.ID, "job-03", "job-04")
	if !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("expected ErrRequiredItem when item-168 is missing, got %v", err)
	}

	// 7. Non-economy mode: item-168 present -> success, memory saved, item consumed
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

func TestService_MasterAndCheckMasteryBranches(t *testing.T) {
	ctx := context.Background()
	state, _ := corejob.NewCharacterJob("char-1", "job-01")
	repo := &repositoryStub{value: state}

	// Master with repository error
	errRepo := &errRepoStub{findErr: errors.New("repo find error")}
	svcErrRepo, _ := NewService(errRepo)
	_, err := svcErrRepo.Master(ctx, "char-1", "job-01")
	if err == nil || err.Error() != "repo find error" {
		t.Fatalf("expected repo find error, got %v", err)
	}

	// Master with character repo lookup error
	charRepoErr := &errCharRepoStub{findErr: errors.New("char find error")}
	svcCharErr, _ := NewService(repo, WithCharacterRepository(charRepoErr))
	mastered, err := svcCharErr.Master(ctx, "char-1", "job-02")
	if err != nil || !mastered.IsMastered("job-02") {
		t.Fatalf("expected job-02 mastered despite char lookup error: err=%v", err)
	}

	// Master with unknown definition falls back to state.Master(jobID)
	mastered, err = svcCharErr.Master(ctx, "char-1", "unknown-job-id")
	if err != nil || !mastered.IsMastered("unknown-job-id") {
		t.Fatalf("expected unknown-job-id mastered: err=%v", err)
	}

	// CheckAndApplyMastery: repo find error
	svcRepoErr2, _ := NewService(errRepo)
	_, err = svcRepoErr2.CheckAndApplyMastery(ctx, "char-1", 50)
	if err == nil || err.Error() != "repo find error" {
		t.Fatalf("expected repo find error, got %v", err)
	}

	// CheckAndApplyMastery: char repo error
	_, err = svcCharErr.CheckAndApplyMastery(ctx, "char-1", 50)
	if err == nil || err.Error() != "char find error" {
		t.Fatalf("expected char find error, got %v", err)
	}

	// CheckAndApplyMastery: unknown current job -> required <= 0 -> returns false, nil
	unknownState, _ := corejob.NewCharacterJob("char-1", "unknown-job")
	repoUnknown := &repositoryStub{value: unknownState}
	svcUnknown, _ := NewService(repoUnknown)
	ok, err := svcUnknown.CheckAndApplyMastery(ctx, "char-1", 50)
	if err != nil || ok {
		t.Fatalf("expected false, nil for unknown job: ok=%v, err=%v", ok, err)
	}
}

func TestService_ChangeMethod(t *testing.T) {
	ctx := context.Background()

	// 1. When FindByCharacterID returns error, Change falls back to NewCharacterJob and succeeds
	errRepo := &errRepoStub{findErr: errors.New("not found")}
	svc, _ := NewService(errRepo)
	// Use a real job (job-01) since ChangeTo now rejects "starter" per legacy spec.
	target, _ := corejob.NewDefinition("job-01", "Warrior", 1, 1, 1, 1, 1, 1, "")
	state, err := svc.Change(ctx, "char-new", target, 20, "unspecified")
	if err != nil {
		t.Fatalf("expected Change to initialize new state on find error: %v", err)
	}
	if state.CharacterID != "char-new" || state.CurrentJobID != "job-01" {
		t.Fatalf("unexpected state: %#v", state)
	}

	// 2. When ChangeTo fails (e.g. level below minimum), returns error
	_, err = svc.Change(ctx, "char-new", target, 19, "unspecified")
	if err == nil {
		t.Fatal("expected level requirement error in Change")
	}

	// 3. When Save fails, returns error
	initialState, _ := corejob.NewCharacterJob("char-new", "starter")
	saveErrRepo := &errRepoStub{value: initialState, saveErr: errors.New("save error")}
	svcSaveErr, _ := NewService(saveErrRepo)
	_, err = svcSaveErr.Change(ctx, "char-new", target, 20, "unspecified")
	if err == nil || err.Error() != "save error" {
		t.Fatalf("expected save error in Change, got %v", err)
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
