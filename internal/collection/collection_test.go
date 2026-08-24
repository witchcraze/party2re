package collection_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
)

type mockCollectionRepo struct {
	monsters map[string]collection.MonsterBookEntry
	items    map[string]collection.ItemCollectionEntry
}

func (m *mockCollectionRepo) RecordMonsterDefeat(_ context.Context, charID, mID, mName, habitat string) error {
	e, ok := m.monsters[mID]
	if !ok {
		e = collection.MonsterBookEntry{
			CharacterID:     charID,
			MonsterID:       mID,
			MonsterName:     mName,
			Habitat:         habitat,
			DefeatedCount:   1,
			FirstDefeatedAt: time.Now().UTC(),
			LastDefeatedAt:  time.Now().UTC(),
		}
	} else {
		e.DefeatedCount++
		e.LastDefeatedAt = time.Now().UTC()
	}
	m.monsters[mID] = e
	return nil
}

func (m *mockCollectionRepo) GetMonsterBook(_ context.Context, _ string) ([]collection.MonsterBookEntry, error) {
	var list []collection.MonsterBookEntry
	for _, v := range m.monsters {
		list = append(list, v)
	}
	return list, nil
}

func (m *mockCollectionRepo) GetMonsterBookCount(_ context.Context, _ string) (int, error) {
	return len(m.monsters), nil
}

func (m *mockCollectionRepo) RecordItemDiscovered(_ context.Context, charID, itemID, itemName, category string) error {
	if _, ok := m.items[itemID]; !ok {
		m.items[itemID] = collection.ItemCollectionEntry{
			CharacterID:  charID,
			ItemID:       itemID,
			ItemName:     itemName,
			Category:     category,
			DiscoveredAt: time.Now().UTC(),
		}
	}
	return nil
}

func (m *mockCollectionRepo) GetItemCollection(_ context.Context, _, category string) ([]collection.ItemCollectionEntry, error) {
	var list []collection.ItemCollectionEntry
	for _, v := range m.items {
		if category == "" || v.Category == category {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCollectionRepo) GetItemCollectionCount(_ context.Context, _ string) (int, error) {
	return len(m.items), nil
}

func TestCollectionService_MonsterBook(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters: make(map[string]collection.MonsterBookEntry),
		items:    make(map[string]collection.ItemCollectionEntry),
	}

	svc, err := collection.NewService(repo, 10, 10)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Record Slime defeat
	if err := svc.RecordMonsterDefeat(ctx, "char1", "mon_slime", "Slime", "Plain"); err != nil {
		t.Fatalf("RecordMonsterDefeat failed: %v", err)
	}

	// 2. Record second Slime defeat
	if err := svc.RecordMonsterDefeat(ctx, "char1", "mon_slime", "Slime", "Plain"); err != nil {
		t.Fatalf("RecordMonsterDefeat failed: %v", err)
	}

	// 3. Get Monster Book -> 1 discovered out of 10 = 10%
	entries, progress, err := svc.GetMonsterBook(ctx, "char1")
	if err != nil {
		t.Fatalf("GetMonsterBook failed: %v", err)
	}
	if len(entries) != 1 || entries[0].DefeatedCount != 2 {
		t.Errorf("entries: len=%d, defeated=%d", len(entries), entries[0].DefeatedCount)
	}
	if progress.DiscoveredCount != 1 || progress.CompletionPercentage != 10.0 {
		t.Errorf("progress: count=%d, percentage=%f", progress.DiscoveredCount, progress.CompletionPercentage)
	}
}

func TestCollectionService_ItemCollection(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters: make(map[string]collection.MonsterBookEntry),
		items:    make(map[string]collection.ItemCollectionEntry),
	}

	svc, _ := collection.NewService(repo, 10, 5)

	// 1. Discover items
	_ = svc.RecordItemDiscovered(ctx, "char1", "wea_sword", "Iron Sword", "WEAPON")
	_ = svc.RecordItemDiscovered(ctx, "char1", "arm_shield", "Iron Shield", "SHIELD")
	_ = svc.RecordItemDiscovered(ctx, "char1", "wea_sword", "Iron Sword", "WEAPON") // duplicate

	// 2. Query Weapon category
	entries, progress, err := svc.GetItemCollection(ctx, "char1", "WEAPON")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(entries) != 1 || entries[0].ItemID != "wea_sword" {
		t.Errorf("expected 1 weapon entry, got %d", len(entries))
	}
	// Total discovered items is 2 out of 5 = 40%
	if progress.DiscoveredCount != 2 || progress.CompletionPercentage != 40.0 {
		t.Errorf("progress: discovered=%d, percentage=%f", progress.DiscoveredCount, progress.CompletionPercentage)
	}
}
