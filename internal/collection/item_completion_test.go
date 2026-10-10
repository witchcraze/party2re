package collection_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/collection"
)

func TestItemCollection_ExcludeItemsAbove141FromCompletionProgress(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{}
	legend := &mockLegendInductor{}
	svc, err := collection.NewService(repo, collection.DefaultTotalMonsters, collection.DefaultTotalItems,
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	charID := "char-item-collector"

	// 1. Discover item-001 through item-140 (140 basic items)
	for i := 1; i <= 140; i++ {
		id := fmt.Sprintf("item-%03d", i)
		if err := svc.RecordItemDiscovered(ctx, charID, id, "BasicItem "+id, "item"); err != nil {
			t.Fatalf("RecordItemDiscovered %s failed: %v", id, err)
		}
	}

	// 2. Discover item-142 (additional item > 141), while item-141 remains unacquired
	if err := svc.RecordItemDiscovered(ctx, charID, "item-142", "ExtraItem item-142", "item"); err != nil {
		t.Fatalf("RecordItemDiscovered item-142 failed: %v", err)
	}

	// Verify GetItemCollection progress
	entries, prog, err := svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}

	// Additional items must NOT be removed from public entries
	if len(entries) != 141 {
		t.Errorf("expected 141 total entries in public list, got %d", len(entries))
	}

	// Completion progress must only count basic items (up to 141)
	if prog.DiscoveredCount != 140 {
		t.Errorf("expected DiscoveredCount == 140, got %d", prog.DiscoveredCount)
	}
	if prog.TotalCatalogCount != 141 {
		t.Errorf("expected TotalCatalogCount == 141, got %d", prog.TotalCatalogCount)
	}
	if prog.IsCompleted {
		t.Errorf("expected IsCompleted == false when basic item-141 is missing, got true")
	}
	if prog.CompletionPercentage >= 100.0 {
		t.Errorf("expected CompletionPercentage < 100.0, got %f", prog.CompletionPercentage)
	}

	// Verify milestone was NOT marked and legend induction was NOT triggered
	isCompleted, err := repo.IsCompleted(ctx, charID, "item")
	if err != nil {
		t.Fatalf("IsCompleted failed: %v", err)
	}
	if isCompleted {
		t.Errorf("milestone should not be completed when basic item-141 is missing")
	}
	if len(legend.inductions) != 0 {
		t.Errorf("legend induction should not trigger when basic item-141 is missing, got %d inductions", len(legend.inductions))
	}

	// 3. Now discover basic item-141
	if err := svc.RecordItemDiscovered(ctx, charID, "item-141", "BasicItem item-141", "item"); err != nil {
		t.Fatalf("RecordItemDiscovered item-141 failed: %v", err)
	}

	entriesAfter, progAfter, err := svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(entriesAfter) != 142 {
		t.Errorf("expected 142 total entries (141 basic + 1 additional), got %d", len(entriesAfter))
	}
	if progAfter.DiscoveredCount != 141 {
		t.Errorf("expected DiscoveredCount == 141, got %d", progAfter.DiscoveredCount)
	}
	if !progAfter.IsCompleted {
		t.Errorf("expected IsCompleted == true once all 141 basic items are collected")
	}
	if progAfter.CompletionPercentage != 100.0 {
		t.Errorf("expected CompletionPercentage == 100.0, got %f", progAfter.CompletionPercentage)
	}

	isCompletedAfter, err := repo.IsCompleted(ctx, charID, "item")
	if err != nil {
		t.Fatalf("IsCompleted failed: %v", err)
	}
	if !isCompletedAfter {
		t.Errorf("milestone should be marked completed once all 141 basic items are collected")
	}
	if len(legend.inductions) != 1 || legend.inductions[0].Category != "comp_ite" {
		t.Errorf("expected exactly 1 comp_ite legend induction, got %+v", legend.inductions)
	}
}

func TestItemCollection_OnlyAdditionalItemsDoNotTriggerCompletion(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{}
	legend := &mockLegendInductor{}
	svc, err := collection.NewService(repo, collection.DefaultTotalMonsters, collection.DefaultTotalItems,
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	charID := "char-extra-only"

	// Discover only additional items (item-142, item-257)
	if err := svc.RecordItemDiscovered(ctx, charID, "item-142", "Extra 142", "item"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if err := svc.RecordItemDiscovered(ctx, charID, "item-257", "Extra 257", "item"); err != nil {
		t.Fatalf("failed: %v", err)
	}

	entries, prog, err := svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("expected 2 entries in list, got %d", len(entries))
	}
	if prog.DiscoveredCount != 0 {
		t.Errorf("expected DiscoveredCount == 0, got %d", prog.DiscoveredCount)
	}
	if prog.IsCompleted {
		t.Errorf("expected IsCompleted == false, got true")
	}
}
