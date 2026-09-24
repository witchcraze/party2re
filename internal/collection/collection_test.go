package collection_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCollectionRepo struct {
	monsters    map[string]collection.MonsterBookEntry
	items       map[string]collection.ItemCollectionEntry
	completions map[string]bool // key: charID + ":" + kind
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

func (m *mockCollectionRepo) MarkCompleted(_ context.Context, charID, kind string) (bool, error) {
	key := charID + ":" + kind
	if m.completions[key] {
		return false, nil
	}
	m.completions[key] = true
	return true, nil
}

func (m *mockCollectionRepo) IsCompleted(_ context.Context, charID, kind string) (bool, error) {
	return m.completions[charID+":"+kind], nil
}

type mockNewsPublisher struct {
	articles []newsArticle
}

type newsArticle struct {
	Category    string
	Title       string
	Content     string
	Author      string
	PublishedAt time.Time
}

func (p *mockNewsPublisher) PublishNews(_ context.Context, category, title, content, author string, publishedAt time.Time) error {
	p.articles = append(p.articles, newsArticle{
		Category:    category,
		Title:       title,
		Content:     content,
		Author:      author,
		PublishedAt: publishedAt,
	})
	return nil
}

type mockCharRepo struct {
	characters map[string]corecharacter.Character
}

func (r *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if c, ok := r.characters[id]; ok {
		return c, nil
	}
	return corecharacter.Character{}, fmt.Errorf("character not found: %s", id)
}

type mockLegendInductor struct {
	inductions []legendCall
}

type legendCall struct {
	Category    string
	CharacterID string
}

func (m *mockLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	m.inductions = append(m.inductions, legendCall{Category: category, CharacterID: characterID})
	return nil
}

func TestCollectionService_Defaults(t *testing.T) {
	repo := &mockCollectionRepo{
		monsters:    make(map[string]collection.MonsterBookEntry),
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}

	// 0 or negative thresholds fall back to canonical defaults (180 monsters, 141 items)
	svc, err := collection.NewService(repo, 0, 0)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	_, prog, err := svc.GetMonsterBook(context.Background(), "char1")
	if err != nil {
		t.Fatalf("GetMonsterBook failed: %v", err)
	}
	if prog.TotalCatalogCount != 180 {
		t.Errorf("expected 180 total monsters, got %d", prog.TotalCatalogCount)
	}

	_, itemProg, err := svc.GetItemCollection(context.Background(), "char1", "")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if itemProg.TotalCatalogCount != 141 {
		t.Errorf("expected 141 total items, got %d", itemProg.TotalCatalogCount)
	}
}

func TestCollectionService_MonsterBook(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters:    make(map[string]collection.MonsterBookEntry),
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
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
	if progress.IsCompleted {
		t.Errorf("expected IsCompleted=false")
	}

	bookCount, err := repo.GetMonsterBookCount(ctx, "char1")
	if err != nil || bookCount != 1 {
		t.Errorf("expected bookCount 1, got %d (err: %v)", bookCount, err)
	}
}

func TestCollectionService_MonsterBookCompletionNews(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters:    make(map[string]collection.MonsterBookEntry),
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	pub := &mockNewsPublisher{}
	legend := &mockLegendInductor{}
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-hero": {ID: "char-hero", Name: "勇者アベル"},
		},
	}

	// Threshold set to 2 monsters for test
	svc, err := collection.NewService(
		repo,
		2,
		141,
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1st monster
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", "mon-001", "スライム", "平原"); err != nil {
		t.Fatalf("defeat 1 failed: %v", err)
	}
	if len(pub.articles) != 0 {
		t.Fatalf("expected no news yet, got %d", len(pub.articles))
	}
	if len(legend.inductions) != 0 {
		t.Fatalf("expected no legend induction yet, got %d", len(legend.inductions))
	}

	// 2nd monster -> hits 100%!
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", "mon-002", "ドラキー", "洞窟"); err != nil {
		t.Fatalf("defeat 2 failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Fatalf("expected 1 news article, got %d", len(pub.articles))
	}
	expectedMsg := "勇者アベルがモンスターブックをコンプリートしました！"
	if pub.articles[0].Content != expectedMsg {
		t.Errorf("expected content %q, got %q", expectedMsg, pub.articles[0].Content)
	}
	if pub.articles[0].Category != "collection" {
		t.Errorf("expected category collection, got %q", pub.articles[0].Category)
	}
	if len(legend.inductions) != 1 {
		t.Fatalf("expected 1 legend induction, got %d", len(legend.inductions))
	}
	if legend.inductions[0].Category != "comp_mon" || legend.inductions[0].CharacterID != "char-hero" {
		t.Errorf("unexpected legend induction: %+v", legend.inductions[0])
	}

	// 3rd monster -> already completed, idempotent (no duplicate news or induction)
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", "mon-003", "ゴーレム", "砂漠"); err != nil {
		t.Fatalf("defeat 3 failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Errorf("expected still 1 news article, got %d", len(pub.articles))
	}
	if len(legend.inductions) != 1 {
		t.Errorf("expected still 1 legend induction, got %d", len(legend.inductions))
	}

	_, prog, _ := svc.GetMonsterBook(ctx, "char-hero")
	if !prog.IsCompleted {
		t.Errorf("expected IsCompleted=true")
	}
}

func TestCollectionService_ItemCollectionCompletionNews(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters:    make(map[string]collection.MonsterBookEntry),
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	pub := &mockNewsPublisher{}
	legend := &mockLegendInductor{}
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-collector": {ID: "char-collector", Name: "コレクター"},
		},
	}

	// Threshold set to 2 items for test
	svc, err := collection.NewService(
		repo,
		180,
		2,
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1st item
	if err := svc.RecordItemDiscovered(ctx, "char-collector", "item-001", "やくそう", "ITEM"); err != nil {
		t.Fatalf("item 1 failed: %v", err)
	}
	if len(pub.articles) != 0 {
		t.Fatalf("expected no news yet, got %d", len(pub.articles))
	}
	if len(legend.inductions) != 0 {
		t.Fatalf("expected no legend induction yet, got %d", len(legend.inductions))
	}

	// 2nd item -> hits 100%!
	if err := svc.RecordItemDiscovered(ctx, "char-collector", "item-002", "どくけしそう", "ITEM"); err != nil {
		t.Fatalf("item 2 failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Fatalf("expected 1 news article, got %d", len(pub.articles))
	}
	expectedMsg := "コレクターがアイテム図鑑をコンプリートしました！"
	if pub.articles[0].Content != expectedMsg {
		t.Errorf("expected content %q, got %q", expectedMsg, pub.articles[0].Content)
	}
	if len(legend.inductions) != 1 {
		t.Fatalf("expected 1 legend induction, got %d", len(legend.inductions))
	}
	if legend.inductions[0].Category != "comp_ite" || legend.inductions[0].CharacterID != "char-collector" {
		t.Errorf("unexpected legend induction: %+v", legend.inductions[0])
	}

	// Duplicate discovery -> no duplicate news or induction
	if err := svc.RecordItemDiscovered(ctx, "char-collector", "item-001", "やくそう", "ITEM"); err != nil {
		t.Fatalf("item dup failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Errorf("expected still 1 news article, got %d", len(pub.articles))
	}
	if len(legend.inductions) != 1 {
		t.Errorf("expected still 1 legend induction, got %d", len(legend.inductions))
	}
}
