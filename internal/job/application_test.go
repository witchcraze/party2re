package job

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/skill"
)

type mockNewsPublisher struct {
	published []string
}

func (m *mockNewsPublisher) PublishNews(_ context.Context, category, title, content, author string, _ time.Time) error {
	m.published = append(m.published, fmt.Sprintf("%s|%s|%s|%s", category, title, content, author))
	return nil
}

type futureMemoryRepoStub struct {
	memories []corecharacter.FutureMemory
}

func (f *futureMemoryRepoStub) Save(_ context.Context, memory corecharacter.FutureMemory) error {
	f.memories = append(f.memories, memory)
	return nil
}

func (f *futureMemoryRepoStub) FindByCharacterID(_ context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
	var results []corecharacter.FutureMemory
	for _, m := range f.memories {
		if m.CharacterID == characterID {
			results = append(results, m)
		}
	}
	return results, nil
}

func (f *futureMemoryRepoStub) Delete(_ context.Context, characterID, memoryID string) error {
	filtered := make([]corecharacter.FutureMemory, 0, len(f.memories))
	for _, m := range f.memories {
		if m.CharacterID == characterID && m.ID == memoryID {
			continue
		}
		filtered = append(filtered, m)
	}
	f.memories = filtered
	return nil
}

type repositoryStub struct{ value corejob.CharacterJob }

func (r *repositoryStub) Save(_ context.Context, value corejob.CharacterJob) error {
	r.value = value
	return nil
}
func (r *repositoryStub) FindByCharacterID(_ context.Context, _ string) (corejob.CharacterJob, error) {
	return r.value, nil
}

func TestServiceChangePersistsJobHistory(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	target, _ := corejob.NewDefinition("vanguard", "Vanguard", 6, 1, 3, 5, 2, 1, "")
	repository := &repositoryStub{value: state}
	service, _ := NewService(repository)
	got, err := service.Change(context.Background(), "character-1", target, 20, "unspecified")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentJobID != "vanguard" || len(repository.value.History) != 1 {
		t.Fatalf("got = %#v, saved = %#v", got, repository.value)
	}
}

func TestServiceMaster(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	repository := &repositoryStub{value: state}
	service, _ := NewService(repository)

	got, err := service.Master(context.Background(), "character-1", "warrior")
	if err != nil {
		t.Fatal(err)
	}
	if !got.IsMastered("warrior") || !repository.value.IsMastered("warrior") {
		t.Errorf("expected warrior to be mastered: %#v", got)
	}
}

func TestServiceCheckAndApplyMastery(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "mage")
	repository := &repositoryStub{value: state}
	catalog, _ := corejob.NewCatalog([]corejob.Definition{{ID: "mage", Name: "Mage", MinLevel: 1, MasterySP: 5}})
	service, _ := NewService(repository, WithCatalog(catalog))

	// Level 50 does not trigger mastery
	applied, err := service.CheckAndApplyMastery(context.Background(), "character-1", 4)
	if err != nil || applied {
		t.Errorf("level 50 should not apply mastery: applied=%v, err=%v", applied, err)
	}

	// Level 99 triggers mastery
	applied, err = service.CheckAndApplyMastery(context.Background(), "character-1", 5)
	if err != nil || !applied {
		t.Errorf("level 99 should apply mastery: applied=%v, err=%v", applied, err)
	}
	if !repository.value.IsMastered("mage") {
		t.Errorf("expected mage to be mastered in repository")
	}

	// Re-checking does not re-apply
	applied, err = service.CheckAndApplyMastery(context.Background(), "character-1", 99)
	if err != nil || applied {
		t.Errorf("already mastered should not re-apply: applied=%v", applied)
	}
}

type charRepoStub struct {
	char corecharacter.Character
}

func (c *charRepoStub) FindByID(_ context.Context, _ string) (corecharacter.Character, error) {
	return c.char, nil
}

func (c *charRepoStub) Update(_ context.Context, char corecharacter.Character) error {
	c.char = char
	return nil
}

type inventoryRepoStub struct {
	inventory coreinventory.Inventory
}

type skillProviderStub map[string][]skill.Definition

func (s skillProviderStub) SkillsForJob(jobID string) []skill.Definition {
	return s[jobID]
}

func (r *inventoryRepoStub) FindByCharacterID(_ context.Context, _ string) (coreinventory.Inventory, error) {
	return r.inventory, nil
}

func (r *inventoryRepoStub) Save(_ context.Context, value coreinventory.Inventory) error {
	r.inventory = value
	return nil
}

func TestServiceListAndChangeJob(t *testing.T) {
	state, _ := corejob.NewCharacterJob("character-1", "starter")
	repo := &repositoryStub{value: state}
	char := corecharacter.Character{
		ID:        "character-1",
		JobID:     "starter",
		Level:     20,
		Gender:    "male",
		OverLevel: true,
	}
	charRepo := &charRepoStub{char: char}
	svc, err := NewService(repo, WithCharacterRepository(charRepo))
	if err != nil {
		t.Fatal(err)
	}

	defs := svc.ListDefinitions()
	if len(defs) == 0 {
		t.Fatal("expected non-empty job definitions list")
	}

	updatedChar, updatedJob, err := svc.ChangeJob(context.Background(), "character-1", "job-01")
	if err != nil {
		t.Fatal(err)
	}
	if updatedChar.JobID != "job-01" || updatedJob.CurrentJobID != "job-01" {
		t.Fatalf("expected job job-01, got char=%s, job=%s", updatedChar.JobID, updatedJob.CurrentJobID)
	}
	if updatedChar.OverLevel {
		t.Fatalf("expected OverLevel to be reset to false on job change, got %v", updatedChar.OverLevel)
	}
}

func TestServiceChangeJobUsesSPMasteryAndConsumesRequiredItem(t *testing.T) {
	char := corecharacter.Character{
		ID: "character-1", JobID: "job-old", Gender: "unspecified", Level: 20, SP: 10,
		Stats: corecharacter.Stats{MaxHP: 40, MaxMP: 20, HP: 10, MP: 5, Attack: 20, Defense: 20, Agility: 20},
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	inventory, _ := coreinventory.New(char.ID)
	itemInstance, _ := item.NewInstance("job-token", 1)
	_ = inventory.Add(itemInstance)
	invRepo := &inventoryRepoStub{inventory: inventory}
	catalog, _ := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-old", Name: "Old", MinLevel: 1, MasterySP: 10},
		{ID: "job-new", Name: "New", MinLevel: 1, RequiredItemID: "job-token"},
	})
	svc, err := NewService(repo, WithCatalog(catalog), WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))
	if err != nil {
		t.Fatal(err)
	}

	updated, updatedState, err := svc.ChangeJob(context.Background(), char.ID, "job-new")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Level != 1 || updated.Experience != 0 || updated.JobLevel != 1 ||
		updated.Stats.MaxHP != 20 || updated.Stats.HP != 20 || updated.Stats.MP != 10 ||
		updated.SP != 0 || updated.OldJobID != "job-old" || updated.OldSP != 10 {
		t.Fatalf("updated character = %#v", updated)
	}
	if !updatedState.IsMastered("job-old") {
		t.Fatalf("old job was not mastered: %#v", updatedState)
	}
	if invRepo.inventory.Quantity("job-token") != 0 {
		t.Fatal("required item was not consumed")
	}
}

func TestServiceChangeJobRejectsLowLevelAndMissingItem(t *testing.T) {
	char := corecharacter.Character{ID: "character-1", JobID: "job-old", Level: 19, Gender: "unspecified"}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	catalog, _ := corejob.NewCatalog([]corejob.Definition{
		{ID: "job-old", Name: "Old", MinLevel: 1},
		{ID: "job-new", Name: "New", MinLevel: 1, RequiredItemID: "token"},
	})
	svc, _ := NewService(repo, WithCatalog(catalog), WithCharacterRepository(charRepo))
	if _, _, err := svc.ChangeJob(context.Background(), char.ID, "job-new"); !errors.Is(err, corejob.ErrJobUnavailable) {
		t.Fatalf("low-level error = %v", err)
	}

	char.Level = 20
	charRepo.char = char
	if _, _, err := svc.ChangeJob(context.Background(), char.ID, "job-new"); !errors.Is(err, ErrRequiredItem) {
		t.Fatalf("missing item error = %v", err)
	}
}

func TestServiceExchangeJobRestoresRememberedPair(t *testing.T) {
	char := corecharacter.Character{
		ID: "character-1", JobID: "job-current", SP: 2, OldJobID: "job-old", OldSP: 1,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	state.Master("job-a")
	state.Master("job-b")
	state.MasteredJobSP["job-a"] = 20
	state.MasteredJobSP["job-b"] = 30
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	inventory, _ := coreinventory.New(char.ID)
	token, _ := item.NewInstance("item-168", 1)
	_ = inventory.Add(token)
	invRepo := &inventoryRepoStub{inventory: inventory}
	svc, _ := NewService(repo, WithCharacterRepository(charRepo), WithInventoryRepository(invRepo))

	updated, _, err := svc.ExchangeJob(context.Background(), char.ID, "job-a", "job-b")
	if err != nil {
		t.Fatal(err)
	}
	if updated.JobID != "job-a" || updated.SP != 20 || updated.OldJobID != "job-b" ||
		updated.OldSP != 30 || updated.JobMemory == nil {
		t.Fatalf("exchanged character = %#v", updated)
	}
	restored, _, err := svc.ExchangeJob(context.Background(), char.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if restored.JobID != "job-current" || restored.SP != 2 || restored.OldJobID != "job-old" ||
		restored.OldSP != 1 || restored.JobMemory != nil {
		t.Fatalf("restored character = %#v", restored)
	}
}

func TestServiceMasteryUsesFinalSkillSPInsteadOfLevel(t *testing.T) {
	char := corecharacter.Character{ID: "character-1", JobID: "mage", SP: 7}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	catalog, _ := corejob.NewCatalog([]corejob.Definition{{ID: "mage", Name: "Mage", MinLevel: 1}})
	skills := skillProviderStub{"mage": {
		{ID: "first", RequiredSP: 2, Effect: corebattle.Effect{Kind: "damage"}},
		{ID: "final", RequiredSP: 7, Effect: corebattle.Effect{Kind: "damage"}},
	}}
	svc, _ := NewService(repo, WithCatalog(catalog), WithCharacterRepository(charRepo), WithSkillProvider(skills))

	applied, err := svc.CheckAndApplyMastery(context.Background(), char.ID, 1)
	if err != nil || !applied || !repo.value.IsMastered("mage") {
		t.Fatalf("mastery = %v, %v, state = %#v", applied, err, repo.value)
	}
}

func TestServiceAllJobsCompletionNotification(t *testing.T) {
	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "Hero",
		JobID: "job-01",
		Level: 25,
		SP:    50,
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	// Master 71 completion jobs
	for i := 1; i <= 71; i++ {
		state.Master(fmt.Sprintf("job-%02d", i))
	}
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	newsPub := &mockNewsPublisher{}

	svc, err := NewService(repo, WithCharacterRepository(charRepo), WithNewsPublisher(newsPub))
	if err != nil {
		t.Fatal(err)
	}

	// Master 72nd job
	_, err = svc.Master(context.Background(), char.ID, "job-72")
	if err != nil {
		t.Fatal(err)
	}

	if !repo.value.AllJobsMastered {
		t.Fatal("expected AllJobsMastered to be true after 72nd job mastery")
	}
	if len(newsPub.published) != 1 {
		t.Fatalf("expected 1 news article, got %d", len(newsPub.published))
	}
	expectedContent := "<span class=\"comp\">Heroが全ての職業をマスターしました！</span>"
	if newsPub.published[0] != fmt.Sprintf("job|全ジョブコンプリート|%s|@システム", expectedContent) {
		t.Fatalf("unexpected news article: %s", newsPub.published[0])
	}

	// Another mastery does not re-publish
	_, err = svc.Master(context.Background(), char.ID, "job-73")
	if err != nil {
		t.Fatal(err)
	}
	if len(newsPub.published) != 1 {
		t.Fatalf("expected news article count to remain 1, got %d", len(newsPub.published))
	}
}

func TestServiceFutureMemorySaveAndRecall(t *testing.T) {
	char := corecharacter.Character{
		ID:         "char-1",
		Name:       "Hero",
		JobID:      "job-05",
		OldJobID:   "job-01",
		Level:      50,
		Experience: 12000,
		SP:         80,
		OldSP:      40,
		Gender:     "female",
		OverLevel:  true,
		OverFuture: 0, // max 1 slot
		Stats: corecharacter.Stats{
			MaxHP:   600,
			MaxMP:   300,
			HP:      200,
			MP:      100,
			Attack:  150,
			Defense: 110,
			Agility: 90,
		},
	}
	state, _ := corejob.NewCharacterJob(char.ID, char.JobID)
	repo := &repositoryStub{value: state}
	charRepo := &charRepoStub{char: char}
	futureRepo := &futureMemoryRepoStub{}

	inventory, _ := coreinventory.New(char.ID)
	frag, _ := item.NewInstance("item-207", 2)
	_ = inventory.Add(frag)
	invRepo := &inventoryRepoStub{inventory: inventory}

	svc, err := NewService(
		repo,
		WithCharacterRepository(charRepo),
		WithInventoryRepository(invRepo),
		WithFutureMemoryRepository(futureRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Save future memory
	snapshot, err := svc.SaveFutureMemory(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("SaveFutureMemory failed: %v", err)
	}
	if snapshot.JobID != "job-05" || snapshot.Level != 50 || snapshot.MaxHP != 600 {
		t.Fatalf("unexpected saved snapshot: %#v", snapshot)
	}
	if invRepo.inventory.Quantity("item-207") != 1 {
		t.Fatalf("expected 1 fragment remaining, got %d", invRepo.inventory.Quantity("item-207"))
	}
	// SP saved to state
	if repo.value.MasteredJobSP["job-05"] != 80 || repo.value.MasteredJobSP["job-01"] != 40 {
		t.Fatalf("expected SP recorded in state: %#v", repo.value.MasteredJobSP)
	}

	// 2. Capacity limit check: OverFuture=0 allows only 1 slot (0 <= 0 is ok, next count 1 <= 0 is false)
	_, err = svc.SaveFutureMemory(context.Background(), char.ID)
	if err == nil || err.Error() != "future memory slot limit reached" {
		t.Fatalf("expected slot limit reached error, got %v", err)
	}

	// 3. Mutate character state
	charRepo.char.JobID = "job-02"
	charRepo.char.OldJobID = "job-05"
	charRepo.char.Level = 1
	charRepo.char.Experience = 0
	charRepo.char.Stats = corecharacter.Stats{MaxHP: 30, MaxMP: 10, HP: 30, MP: 10, Attack: 10, Defense: 10, Agility: 10}
	charRepo.char.SP = 0
	charRepo.char.OldSP = 80
	charRepo.char.OverLevel = false

	// 4. Recall future memory
	restoredChar, restoredState, err := svc.RecallFutureMemory(context.Background(), char.ID, snapshot.ID)
	if err != nil {
		t.Fatalf("RecallFutureMemory failed: %v", err)
	}

	if restoredChar.JobID != "job-05" || restoredChar.OldJobID != "job-01" || restoredChar.Level != 50 ||
		restoredChar.Experience != 12000 || !restoredChar.OverLevel {
		t.Fatalf("unexpected restored char: %#v", restoredChar)
	}
	if restoredChar.Stats.MaxHP != 600 || restoredChar.Stats.HP != 600 || restoredChar.Stats.MaxMP != 300 || restoredChar.Stats.MP != 300 {
		t.Fatalf("unexpected restored stats: %#v", restoredChar.Stats)
	}
	if restoredChar.SP != 80 || restoredChar.OldSP != 40 {
		t.Fatalf("unexpected restored SP: SP=%d, OldSP=%d", restoredChar.SP, restoredChar.OldSP)
	}
	if restoredState.CurrentJobID != "job-05" {
		t.Fatalf("expected CurrentJobID to be restored to job-05, got %s", restoredState.CurrentJobID)
	}

	// Memory slot should be consumed / removed
	memories, err := svc.ListFutureMemories(context.Background(), char.ID)
	if err != nil || len(memories) != 0 {
		t.Fatalf("expected memories to be empty, got %v (err: %v)", memories, err)
	}
}
