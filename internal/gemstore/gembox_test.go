package gemstore_test

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/gemstore"
)

func TestCalculateGemBoxCapacity(t *testing.T) {
	tests := []struct {
		jobLevel int
		expected int
	}{
		{jobLevel: -5, expected: 5},
		{jobLevel: 0, expected: 5},    // 0 transfers -> 5 slots
		{jobLevel: 1, expected: 10},   // 1 transfer -> 10 slots
		{jobLevel: 2, expected: 15},   // 2 transfers -> 15 slots
		{jobLevel: 19, expected: 100}, // 19 transfers -> 100 slots
		{jobLevel: 20, expected: 100}, // >= 20 transfers -> 100 slots (cap)
		{jobLevel: 100, expected: 100},
	}

	for _, tt := range tests {
		got := gemstore.CalculateGemBoxCapacity(tt.jobLevel)
		if got != tt.expected {
			t.Errorf("CalculateGemBoxCapacity(%d) = %d, expected %d", tt.jobLevel, got, tt.expected)
		}
	}
}

func TestGemBox_BasicOperations(t *testing.T) {
	box := gemstore.GemBox{
		CharacterID: "char_test",
		Capacity:    2,
		Items:       []coreitem.Instance{},
	}

	if box.Count() != 0 {
		t.Errorf("expected count 0, got %d", box.Count())
	}
	if box.IsFull() {
		t.Error("expected box not to be full")
	}

	inst1, _ := coreitem.NewInstance("gem_atk_1", 1)
	inst2, _ := coreitem.NewInstance("gem_heal_1", 1)
	inst3, _ := coreitem.NewInstance("gem_mind_1", 1)

	if err := box.AddItem(inst1); err != nil {
		t.Fatalf("unexpected error adding inst1: %v", err)
	}
	if err := box.AddItem(inst2); err != nil {
		t.Fatalf("unexpected error adding inst2: %v", err)
	}

	if !box.IsFull() {
		t.Error("expected box to be full")
	}

	if err := box.AddItem(inst3); err != gemstore.ErrGemBoxFull {
		t.Errorf("expected ErrGemBoxFull, got %v", err)
	}

	found, ok := box.FindItem("gem_atk_1")
	if !ok || found.DefinitionID != "gem_atk_1" {
		t.Errorf("expected to find gem_atk_1, got %+v (ok=%v)", found, ok)
	}

	removed, err := box.RemoveItem("gem_atk_1")
	if err != nil {
		t.Fatalf("unexpected error removing gem_atk_1: %v", err)
	}
	if removed.DefinitionID != "gem_atk_1" {
		t.Errorf("expected removed gem_atk_1, got %s", removed.DefinitionID)
	}
	if box.Count() != 1 {
		t.Errorf("expected count 1, got %d", box.Count())
	}
	if box.IsFull() {
		t.Error("expected box not to be full after removal")
	}
}

func TestGemStore_SortGemBox(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	char := corecharacter.Character{ID: "char_1", JobLevel: 10, Money: 10000}
	charRepo.characters[char.ID] = char

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Buy a few gems in arbitrary order
	// gem_heal_1 (index 18), gem_atk_1 (index 0), gem_combo_1 (index 5)
	_, err = svc.BuyGem(ctx, char.ID, "gem_heal_1")
	if err != nil {
		t.Fatalf("buy gem_heal_1 failed: %v", err)
	}
	_, err = svc.BuyGem(ctx, char.ID, "gem_atk_1")
	if err != nil {
		t.Fatalf("buy gem_atk_1 failed: %v", err)
	}
	_, err = svc.BuyGem(ctx, char.ID, "gem_combo_1")
	if err != nil {
		t.Fatalf("buy gem_combo_1 failed: %v", err)
	}

	sorted, err := svc.SortGemBox(ctx, char.ID)
	if err != nil {
		t.Fatalf("SortGemBox failed: %v", err)
	}

	if len(sorted.Items) != 3 {
		t.Fatalf("expected 3 items, got %d", len(sorted.Items))
	}

	// Verify order is catalog ascending (gem_atk_1, gem_combo_1, gem_heal_1)
	if sorted.Items[0].DefinitionID != "gem_atk_1" {
		t.Errorf("expected first gem to be gem_atk_1, got %s", sorted.Items[0].DefinitionID)
	}
	if sorted.Items[1].DefinitionID != "gem_combo_1" {
		t.Errorf("expected second gem to be gem_combo_1, got %s", sorted.Items[1].DefinitionID)
	}
	if sorted.Items[2].DefinitionID != "gem_heal_1" {
		t.Errorf("expected third gem to be gem_heal_1, got %s", sorted.Items[2].DefinitionID)
	}
}

func TestGemStore_BuyGem_GemBoxFull(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	// JobLevel 0 -> Capacity 5
	char := corecharacter.Character{ID: "char_full", JobLevel: 0, Money: 50000}
	charRepo.characters[char.ID] = char

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Fill all 5 slots
	for i := 0; i < 5; i++ {
		if _, err := svc.BuyGem(ctx, char.ID, "gem_atk_1"); err != nil {
			t.Fatalf("failed to buy gem %d: %v", i, err)
		}
	}

	// 6th purchase must fail with ErrGemBoxFull
	_, err = svc.BuyGem(ctx, char.ID, "gem_atk_1")
	if err != gemstore.ErrGemBoxFull {
		t.Errorf("expected ErrGemBoxFull, got %v", err)
	}
}

func TestGemStore_SendGem_RecipientGemBoxFull(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	charRepo := newMockCharacterRepo()
	invRepo := newMockInventoryRepo()

	sender := corecharacter.Character{ID: "sender", JobLevel: 1, Money: 10000}
	recipient := corecharacter.Character{ID: "recipient", JobLevel: 0, Money: 10000} // Cap 5
	charRepo.characters[sender.ID] = sender
	charRepo.characters[recipient.ID] = recipient

	svc, err := gemstore.NewService(catalog, charRepo, invRepo)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	ctx := context.Background()

	// Fill recipient's box (5 items)
	for i := 0; i < 5; i++ {
		if _, err := svc.BuyGem(ctx, recipient.ID, "gem_atk_1"); err != nil {
			t.Fatalf("failed to fill recipient box %d: %v", i, err)
		}
	}

	// Buy a gem for sender
	buyRes, err := svc.BuyGem(ctx, sender.ID, "gem_atk_1")
	if err != nil {
		t.Fatalf("sender buy failed: %v", err)
	}

	// Attempt to send to full recipient
	_, err = svc.SendGem(ctx, sender.ID, recipient.ID, buyRes.ItemInstance.ID)
	if err != gemstore.ErrRecipientGemBoxFull {
		t.Errorf("expected ErrRecipientGemBoxFull, got %v", err)
	}
}
