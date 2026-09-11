package database

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
)

func TestGemBoxRepositoryNilDB(t *testing.T) {
	if _, err := NewGemBoxRepository(nil); err == nil {
		t.Fatal("NewGemBoxRepository(nil) expected error, got nil")
	}
}

func TestGemBoxRepositorySaveAndFind(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := NewGemBoxRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	char, err := CreateTestCharacter(ctx, db, "GemBox DB Test")
	if err != nil {
		t.Fatal(err)
	}

	inst1, err := item.NewInstance("gem_atk_1", 1)
	if err != nil {
		t.Fatal(err)
	}
	inst2, err := item.NewInstance("gem_heal_1", 1)
	if err != nil {
		t.Fatal(err)
	}

	box := gemstore.GemBox{
		CharacterID: char.ID,
		Capacity:    15,
		Items:       []item.Instance{inst1, inst2},
	}

	if err := repo.Save(ctx, box); err != nil {
		t.Fatalf("repo.Save() error = %v", err)
	}

	restored, err := repo.FindByCharacterID(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterID() error = %v", err)
	}

	if restored.CharacterID != char.ID || restored.Capacity != 15 || len(restored.Items) != 2 {
		t.Fatalf("restored gem box mismatch: %#v", restored)
	}
	if restored.Items[0].DefinitionID != "gem_atk_1" || restored.Items[1].DefinitionID != "gem_heal_1" {
		t.Fatalf("restored items mismatch: %#v", restored.Items)
	}

	// Update items (remove one, add another)
	inst3, err := item.NewInstance("gem_mind_1", 1)
	if err != nil {
		t.Fatal(err)
	}
	box.Items = []item.Instance{inst2, inst3}
	if err := repo.Save(ctx, box); err != nil {
		t.Fatalf("repo.Save() second error = %v", err)
	}

	restored2, err := repo.FindByCharacterIDForUpdate(ctx, char.ID)
	if err != nil {
		t.Fatalf("FindByCharacterIDForUpdate() error = %v", err)
	}
	if len(restored2.Items) != 2 || restored2.Items[0].DefinitionID != "gem_heal_1" || restored2.Items[1].DefinitionID != "gem_mind_1" {
		t.Fatalf("restored items after update mismatch: %#v", restored2.Items)
	}
}
