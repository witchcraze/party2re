package database

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/gemstore"
)

type testGemCatalog map[string]customskill.GemDefinition

func (g testGemCatalog) FindGemByID(id string) (customskill.GemDefinition, bool) {
	def, ok := g[id]
	return def, ok
}

func TestCustomSkillRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := NewCustomSkillRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "CustomSkill DB Test")
	if err != nil {
		t.Fatal(err)
	}

	skill := customskill.CustomSkill{
		CharacterID: char.ID,
		Name:        "ドラゴンソウル",
		Comment:     "燃え上がれ",
		CMP:         15,
		Gems:        [3]string{"gem_atk_1", "gem_heal_1", ""},
		UpdatedAt:   time.Now().UTC().Truncate(time.Second),
	}

	if err := repo.SaveCustomSkill(ctx, skill); err != nil {
		t.Fatalf("SaveCustomSkill() error = %v", err)
	}

	found, err := repo.FindCustomSkill(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindCustomSkill() error = %v", err)
	}
	if found == nil {
		t.Fatal("expected custom skill, got nil")
	}
	if found.Name != "ドラゴンソウル" || found.Comment != "燃え上がれ" || found.CMP != 15 || found.Gems != skill.Gems {
		t.Fatalf("custom skill mismatch: got %+v, want %+v", found, skill)
	}
}

func TestCustomSkillServiceGemBoxIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	csRepo, err := NewCustomSkillRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	gemBoxRepo, err := NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "GemBox CS Synthesis")
	if err != nil {
		t.Fatal(err)
	}

	// Put ruby and diamond in gem box
	inst1, _ := item.NewInstance("ruby", 1)
	inst2, _ := item.NewInstance("diamond", 1)
	box := gemstore.GemBox{
		CharacterID: char.ID,
		Capacity:    10,
		Items:       []item.Instance{inst1, inst2},
	}
	if err := gemBoxRepo.Save(ctx, box); err != nil {
		t.Fatalf("gemBoxRepo.Save() error = %v", err)
	}

	service, err := customskill.NewService(csRepo, charRepo)
	if err != nil {
		t.Fatal(err)
	}
	service.ConfigureGemSynthesis(testGemCatalog{
		"ruby":    {ID: "ruby", SlotCost: 1, MPCost: 4},
		"diamond": {ID: "diamond", SlotCost: 2, MPCost: 8},
	}, gemBoxRepo, gemBoxRepo)

	// Step 1: Synthesize with ruby
	res1, err := service.SetCustomSkill(ctx, char.ID, "炎撃", "燃えよ", [3]string{"ruby", "", ""})
	if err != nil {
		t.Fatalf("SetCustomSkill() step 1 error = %v", err)
	}
	if res1.CMP != 4 {
		t.Fatalf("expected CMP 4, got %d", res1.CMP)
	}

	// Verify gem box now only has diamond
	b1, err := gemBoxRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b1.Items) != 1 || b1.Items[0].DefinitionID != "diamond" {
		t.Fatalf("expected only diamond in gem box, got %+v", b1.Items)
	}

	// Step 2: Swap to diamond (ruby should return to gem box, diamond consumed)
	res2, err := service.SetCustomSkill(ctx, char.ID, "金剛撃", "", [3]string{"diamond", "", ""})
	if err != nil {
		t.Fatalf("SetCustomSkill() step 2 error = %v", err)
	}
	if res2.CMP != 8 {
		t.Fatalf("expected CMP 8, got %d", res2.CMP)
	}

	// Verify gem box now only has ruby
	b2, err := gemBoxRepo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(b2.Items) != 1 || b2.Items[0].DefinitionID != "ruby" {
		t.Fatalf("expected only ruby returned to gem box, got %+v", b2.Items)
	}
}
