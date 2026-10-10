package collection_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockCollectionRepo struct {
	mu                   sync.Mutex
	monsters             map[string]collection.MonsterBookEntry
	items                map[string]collection.ItemCollectionEntry
	completions          map[string]bool // key: charID + ":" + kind
	getMonsterCountErr   error
	getItemCountErr      error
	getItemCollectionErr error
	markCompletedErr     error
	isCompletedErr       error
}

func (m *mockCollectionRepo) RecordMonsterDefeat(_ context.Context, charID string, record collection.DefeatedMonsterRecord) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.monsters == nil {
		m.monsters = make(map[string]collection.MonsterBookEntry)
	}
	e, ok := m.monsters[record.MonsterID]
	if !ok {
		e = collection.MonsterBookEntry{
			CharacterID:      charID,
			MonsterID:        record.MonsterID,
			MonsterName:      record.MonsterName,
			Habitat:          record.Habitat,
			DefeatedCount:    1,
			FirstDefeatedAt:  time.Now().UTC(),
			LastDefeatedAt:   time.Now().UTC(),
			Icon:             record.Icon,
			Strong:           record.Strong,
			HP:               record.HP,
			MP:               record.MP,
			Attack:           record.Attack,
			Defense:          record.Defense,
			Agility:          record.Agility,
			ExperienceReward: record.ExperienceReward,
			GoldReward:       record.GoldReward,
		}
	} else {
		e.DefeatedCount++
		e.LastDefeatedAt = time.Now().UTC()
	}
	m.monsters[record.MonsterID] = e
	return nil
}

func (m *mockCollectionRepo) GetMonsterBook(_ context.Context, _ string) ([]collection.MonsterBookEntry, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var list []collection.MonsterBookEntry
	for _, v := range m.monsters {
		list = append(list, v)
	}
	return list, nil
}

func (m *mockCollectionRepo) GetMonsterBookCount(_ context.Context, _ string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getMonsterCountErr != nil {
		return 0, m.getMonsterCountErr
	}
	return len(m.monsters), nil
}

func (m *mockCollectionRepo) RecordItemDiscovered(_ context.Context, charID, itemID, itemName, category string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.items == nil {
		m.items = make(map[string]collection.ItemCollectionEntry)
	}
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
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getItemCollectionErr != nil {
		return nil, m.getItemCollectionErr
	}
	var list []collection.ItemCollectionEntry
	for _, v := range m.items {
		if category == "" || strings.EqualFold(v.Category, category) {
			list = append(list, v)
		}
	}
	return list, nil
}

func (m *mockCollectionRepo) GetItemCollectionCount(_ context.Context, _, category string) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.getItemCountErr != nil {
		return 0, m.getItemCountErr
	}
	count := 0
	for _, v := range m.items {
		if category == "" || strings.EqualFold(v.Category, category) {
			if strings.EqualFold(v.Category, "item") {
				if collection.IsBasicItem(v.ItemID, collection.DefaultTotalItems) {
					count++
				}
			} else {
				count++
			}
		}
	}
	return count, nil
}

func (m *mockCollectionRepo) MarkCompleted(_ context.Context, charID, kind string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.markCompletedErr != nil {
		return false, m.markCompletedErr
	}
	if m.completions == nil {
		m.completions = make(map[string]bool)
	}
	key := charID + ":" + kind
	if m.completions[key] {
		return false, nil
	}
	m.completions[key] = true
	return true, nil
}

func (m *mockCollectionRepo) IsCompleted(_ context.Context, charID, kind string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.isCompletedErr != nil {
		return false, m.isCompletedErr
	}
	if m.completions == nil {
		return false, nil
	}
	return m.completions[charID+":"+kind], nil
}

type mockNewsPublisher struct {
	mu       sync.Mutex
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
	p.mu.Lock()
	defer p.mu.Unlock()
	p.articles = append(p.articles, newsArticle{
		Category:    category,
		Title:       title,
		Content:     content,
		Author:      author,
		PublishedAt: publishedAt,
	})
	return nil
}

func (p *mockNewsPublisher) getArticles() []newsArticle {
	p.mu.Lock()
	defer p.mu.Unlock()
	copied := make([]newsArticle, len(p.articles))
	copy(copied, p.articles)
	return copied
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
	mu            sync.Mutex
	inductions    []legendCall
	recordErr     error
	failCountdown int
	hasCountdown  bool
}

type legendCall struct {
	Category    string
	CharacterID string
}

func (m *mockLegendInductor) RecordLegend(_ context.Context, category, characterID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.hasCountdown {
		if m.failCountdown > 0 {
			m.failCountdown--
			return m.recordErr
		}
	} else if m.recordErr != nil {
		return m.recordErr
	}
	m.inductions = append(m.inductions, legendCall{Category: category, CharacterID: characterID})
	return nil
}

func (m *mockLegendInductor) getInductions() []legendCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	copied := make([]legendCall, len(m.inductions))
	copy(copied, m.inductions)
	return copied
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
	if err := svc.RecordMonsterDefeat(ctx, "char1", collection.DefeatedMonsterRecord{
		MonsterID:   "mon_slime",
		MonsterName: "Slime",
		Habitat:     "Plain",
		HP:          10,
		Attack:      5,
		Defense:     3,
		Agility:     4,
	}); err != nil {
		t.Fatalf("RecordMonsterDefeat failed: %v", err)
	}

	// 2. Record second Slime defeat
	if err := svc.RecordMonsterDefeat(ctx, "char1", collection.DefeatedMonsterRecord{
		MonsterID:   "mon_slime",
		MonsterName: "Slime",
		Habitat:     "Plain",
		HP:          10,
		Attack:      5,
		Defense:     3,
		Agility:     4,
	}); err != nil {
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
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", collection.DefeatedMonsterRecord{
		MonsterID:   "mon-001",
		MonsterName: "スライム",
		Habitat:     "平原",
	}); err != nil {
		t.Fatalf("defeat 1 failed: %v", err)
	}
	if len(pub.articles) != 0 {
		t.Fatalf("expected no news yet, got %d", len(pub.articles))
	}
	if len(legend.inductions) != 0 {
		t.Fatalf("expected no legend induction yet, got %d", len(legend.inductions))
	}

	// 2nd monster -> hits 100%!
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", collection.DefeatedMonsterRecord{
		MonsterID:   "mon-002",
		MonsterName: "ドラキー",
		Habitat:     "洞窟",
	}); err != nil {
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
	if err := svc.RecordMonsterDefeat(ctx, "char-hero", collection.DefeatedMonsterRecord{
		MonsterID:   "mon-003",
		MonsterName: "ゴーレム",
		Habitat:     "砂漠",
	}); err != nil {
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

func TestCollectionService_WeaponAndArmorDefaults(t *testing.T) {
	repo := &mockCollectionRepo{
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	svc, err := collection.NewService(repo, 0, 0)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	_, weaProg, err := svc.GetWeaponCollection(context.Background(), "char1")
	if err != nil {
		t.Fatalf("GetWeaponCollection failed: %v", err)
	}
	if weaProg.TotalCatalogCount != collection.DefaultTotalWeapons {
		t.Errorf("expected %d total weapons, got %d", collection.DefaultTotalWeapons, weaProg.TotalCatalogCount)
	}

	_, armProg, err := svc.GetArmorCollection(context.Background(), "char1")
	if err != nil {
		t.Fatalf("GetArmorCollection failed: %v", err)
	}
	if armProg.TotalCatalogCount != collection.DefaultTotalArmors {
		t.Errorf("expected %d total armors, got %d", collection.DefaultTotalArmors, armProg.TotalCatalogCount)
	}
}

func TestCollectionService_WeaponCollectionCompletionNews(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
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

	svc, err := collection.NewService(
		repo,
		180,
		141,
		collection.WithTotalWeapons(2),
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1st weapon
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "wea-001", "ひのきの棒", "weapon"); err != nil {
		t.Fatalf("weapon 1 failed: %v", err)
	}
	if len(pub.articles) != 0 || len(legend.inductions) != 0 {
		t.Fatalf("expected no news or induction yet")
	}

	// 2nd weapon -> 100%!
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "wea-002", "竹の槍", "WEAPON"); err != nil {
		t.Fatalf("weapon 2 failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Fatalf("expected 1 news article, got %d", len(pub.articles))
	}
	expectedMsg := "勇者アベルが武器図鑑をコンプリートしました！"
	if pub.articles[0].Content != expectedMsg {
		t.Errorf("expected content %q, got %q", expectedMsg, pub.articles[0].Content)
	}
	if len(legend.inductions) != 1 {
		t.Fatalf("expected 1 legend induction, got %d", len(legend.inductions))
	}
	if legend.inductions[0].Category != "comp_wea" || legend.inductions[0].CharacterID != "char-hero" {
		t.Errorf("unexpected legend induction: %+v", legend.inductions[0])
	}

	// Duplicate discovery -> idempotent
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "wea-001", "ひのきの棒", "main-hand"); err != nil {
		t.Fatalf("weapon dup failed: %v", err)
	}
	if len(pub.articles) != 1 || len(legend.inductions) != 1 {
		t.Errorf("expected no duplicate news or induction")
	}

	entries, prog, err := svc.GetWeaponCollection(ctx, "char-hero")
	if err != nil || len(entries) != 2 || !prog.IsCompleted || prog.CompletionPercentage != 100.0 {
		t.Errorf("progress mismatch: %+v, entries: %d", prog, len(entries))
	}
}

func TestCollectionService_ArmorCollectionCompletionNews(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
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

	svc, err := collection.NewService(
		repo,
		180,
		141,
		collection.WithTotalArmors(2),
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1st armor
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "arm-001", "布の服", "armor"); err != nil {
		t.Fatalf("armor 1 failed: %v", err)
	}
	if len(pub.articles) != 0 || len(legend.inductions) != 0 {
		t.Fatalf("expected no news or induction yet")
	}

	// 2nd armor -> 100%!
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "arm-002", "皮の鎧", "ARMOR"); err != nil {
		t.Fatalf("armor 2 failed: %v", err)
	}
	if len(pub.articles) != 1 {
		t.Fatalf("expected 1 news article, got %d", len(pub.articles))
	}
	expectedMsg := "勇者アベルが防具図鑑をコンプリートしました！"
	if pub.articles[0].Content != expectedMsg {
		t.Errorf("expected content %q, got %q", expectedMsg, pub.articles[0].Content)
	}
	if len(legend.inductions) != 1 {
		t.Fatalf("expected 1 legend induction, got %d", len(legend.inductions))
	}
	if legend.inductions[0].Category != "comp_arm" || legend.inductions[0].CharacterID != "char-hero" {
		t.Errorf("unexpected legend induction: %+v", legend.inductions[0])
	}

	// Duplicate discovery -> idempotent
	if err := svc.RecordItemDiscovered(ctx, "char-hero", "arm-001", "布の服", "shield"); err != nil {
		t.Fatalf("armor dup failed: %v", err)
	}
	if len(pub.articles) != 1 || len(legend.inductions) != 1 {
		t.Errorf("expected no duplicate news or induction")
	}

	entries, prog, err := svc.GetArmorCollection(ctx, "char-hero")
	if err != nil || len(entries) != 2 || !prog.IsCompleted || prog.CompletionPercentage != 100.0 {
		t.Errorf("progress mismatch: %+v, entries: %d", prog, len(entries))
	}
}

func TestCollectionService_ValidationAndClamping(t *testing.T) {
	repo := &mockCollectionRepo{
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	svc, err := collection.NewService(repo, 1, 1, collection.WithTotalWeapons(1), collection.WithTotalArmors(1))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	if _, _, err := svc.GetWeaponCollection(context.Background(), ""); err != collection.ErrInvalidCharacterID {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
	if _, _, err := svc.GetArmorCollection(context.Background(), ""); err != collection.ErrInvalidCharacterID {
		t.Errorf("expected ErrInvalidCharacterID, got %v", err)
	}
}

func TestCollectionService_WeaponAndArmorDiscoveriesDoNotTriggerItemCompletion(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	pub := &mockNewsPublisher{}
	legend := &mockLegendInductor{}
	charRepo := &mockCharRepo{
		characters: map[string]corecharacter.Character{
			"char-collector": {ID: "char-collector", Name: "収集王"},
		},
	}

	// Canonical totals: 180 monsters, 141 items, 71 weapons, 55 armors
	svc, err := collection.NewService(
		repo,
		collection.DefaultTotalMonsters,
		collection.DefaultTotalItems,
		collection.WithNewsPublisher(pub),
		collection.WithCharacterRepository(charRepo),
		collection.WithLegendInductor(legend),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// 1. Discover all 71 weapons
	for i := 1; i <= 71; i++ {
		wID := fmt.Sprintf("wea-%03d", i)
		wName := fmt.Sprintf("武器%d", i)
		if err := svc.RecordItemDiscovered(ctx, "char-collector", wID, wName, "weapon"); err != nil {
			t.Fatalf("weapon %d failed: %v", i, err)
		}
	}

	// 2. Discover all 55 armors
	for i := 1; i <= 55; i++ {
		aID := fmt.Sprintf("arm-%03d", i)
		aName := fmt.Sprintf("防具%d", i)
		if err := svc.RecordItemDiscovered(ctx, "char-collector", aID, aName, "armor"); err != nil {
			t.Fatalf("armor %d failed: %v", i, err)
		}
	}

	// 3. Discover 15 items -> total rows in repo is 71 + 55 + 15 = 141 (the item threshold!)
	for i := 1; i <= 15; i++ {
		itID := fmt.Sprintf("ite-%03d", i)
		itName := fmt.Sprintf("アイテム%d", i)
		if err := svc.RecordItemDiscovered(ctx, "char-collector", itID, itName, "item"); err != nil {
			t.Fatalf("item %d failed: %v", i, err)
		}
	}

	// Verify weapon and armor completions were triggered
	if ok, _ := repo.IsCompleted(ctx, "char-collector", "weapon"); !ok {
		t.Errorf("expected weapon collection to be completed")
	}
	if ok, _ := repo.IsCompleted(ctx, "char-collector", "armor"); !ok {
		t.Errorf("expected armor collection to be completed")
	}

	// Verify item collection is NOT completed despite total entries = 141
	if ok, _ := repo.IsCompleted(ctx, "char-collector", "item"); ok {
		t.Fatalf("item collection MUST NOT be marked completed when only 15 items are discovered")
	}

	// Check item collection progress
	entries, prog, err := svc.GetItemCollection(ctx, "char-collector", "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(entries) != 15 {
		t.Errorf("expected 15 item entries, got %d", len(entries))
	}
	if prog.DiscoveredCount != 15 {
		t.Errorf("expected DiscoveredCount 15, got %d", prog.DiscoveredCount)
	}
	if prog.IsCompleted {
		t.Errorf("expected IsCompleted to be false")
	}
	expectedPct := (15.0 / 141.0) * 100.0
	if fmt.Sprintf("%.2f", prog.CompletionPercentage) != fmt.Sprintf("%.2f", expectedPct) {
		t.Errorf("expected completion percentage %.2f, got %.2f", expectedPct, prog.CompletionPercentage)
	}

	// Verify calling GetItemCollection with empty string defaults to "item" category
	// and does NOT count weapons or armors toward completion
	defaultEntries, defaultProg, err := svc.GetItemCollection(ctx, "char-collector", "")
	if err != nil {
		t.Fatalf("GetItemCollection with empty category failed: %v", err)
	}
	if len(defaultEntries) != 15 {
		t.Errorf("expected 15 item entries with empty category, got %d", len(defaultEntries))
	}
	if defaultProg.DiscoveredCount != 15 {
		t.Errorf("expected DiscoveredCount 15 with empty category, got %d", defaultProg.DiscoveredCount)
	}
	if defaultProg.IsCompleted {
		t.Errorf("expected IsCompleted to be false with empty category")
	}

	// Also verify whitespace-only category defaults to "item"
	wsEntries, wsProg, err := svc.GetItemCollection(ctx, "char-collector", "   ")
	if err != nil {
		t.Fatalf("GetItemCollection with whitespace category failed: %v", err)
	}
	if len(wsEntries) != 15 || wsProg.DiscoveredCount != 15 || wsProg.IsCompleted {
		t.Errorf("expected 15 item entries and uncompleted for whitespace category, got len=%d discovered=%d isCompleted=%v",
			len(wsEntries), wsProg.DiscoveredCount, wsProg.IsCompleted)
	}

	// Verify legend inductor has comp_wea and comp_arm, but NOT comp_ite
	for _, ind := range legend.inductions {
		if ind.Category == "comp_ite" {
			t.Fatalf("comp_ite was prematurely inducted into Hall of Fame!")
		}
	}

	// Now discover the remaining 126 items (from 16 to 141)
	for i := 16; i <= 141; i++ {
		itID := fmt.Sprintf("ite-%03d", i)
		itName := fmt.Sprintf("アイテム%d", i)
		if err := svc.RecordItemDiscovered(ctx, "char-collector", itID, itName, "item"); err != nil {
			t.Fatalf("item %d failed: %v", i, err)
		}
	}

	// Now item collection should be 100% complete
	if ok, _ := repo.IsCompleted(ctx, "char-collector", "item"); !ok {
		t.Errorf("expected item collection to be completed after discovering all 141 items")
	}
	_, finalProg, err := svc.GetItemCollection(ctx, "char-collector", "item")
	if err != nil {
		t.Fatalf("final GetItemCollection failed: %v", err)
	}
	if finalProg.DiscoveredCount != 141 || !finalProg.IsCompleted || finalProg.CompletionPercentage != 100.0 {
		t.Errorf("expected 100%% complete item collection, got %+v", finalProg)
	}

	// Verify comp_ite is now inducted
	foundCompIte := false
	for _, ind := range legend.inductions {
		if ind.Category == "comp_ite" && ind.CharacterID == "char-collector" {
			foundCompIte = true
			break
		}
	}
	if !foundCompIte {
		t.Errorf("expected comp_ite to be inducted into Hall of Fame after 141 items")
	}
}

func TestCollectionService_GetItemCollection_DefaultsToItemCategory(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}
	svc, err := collection.NewService(repo, 180, 10)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	// Record 2 weapons, 2 armors, and 3 items
	_ = svc.RecordItemDiscovered(ctx, "char-1", "w1", "Sword", "weapon")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "w2", "Axe", "weapon")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "a1", "Shield", "armor")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "a2", "Helmet", "armor")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "i1", "Potion", "item")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "i2", "Ether", "item")
	_ = svc.RecordItemDiscovered(ctx, "char-1", "i3", "Elixir", "item")

	testCases := []struct {
		name     string
		category string
	}{
		{name: "empty string", category: ""},
		{name: "whitespace only", category: "   \t\n"},
		{name: "explicit item", category: "item"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			entries, progress, err := svc.GetItemCollection(ctx, "char-1", tc.category)
			if err != nil {
				t.Fatalf("GetItemCollection failed: %v", err)
			}
			if len(entries) != 3 {
				t.Errorf("expected 3 item entries, got %d", len(entries))
			}
			for _, e := range entries {
				if e.Category != "item" {
					t.Errorf("expected category 'item', got %q for item %s", e.Category, e.ItemID)
				}
			}
			if progress.DiscoveredCount != 3 {
				t.Errorf("expected DiscoveredCount 3, got %d", progress.DiscoveredCount)
			}
			if progress.TotalCatalogCount != 10 {
				t.Errorf("expected TotalCatalogCount 10, got %d", progress.TotalCatalogCount)
			}
			if progress.CompletionPercentage != 30.0 {
				t.Errorf("expected CompletionPercentage 30.0, got %f", progress.CompletionPercentage)
			}
			if progress.IsCompleted {
				t.Errorf("expected IsCompleted false, got true")
			}
		})
	}
}

func TestCalculateStrong(t *testing.T) {
	// formula: int(hp + mp + at + df*0.5 + ag)
	got := collection.CalculateStrong(100, 50, 30, 25, 40)
	// 100 + 50 + 30 + 12.5 + 40 = 232.5 -> int(232.5) == 232
	want := 232
	if got != want {
		t.Errorf("CalculateStrong(100, 50, 30, 25, 40) = %d, want %d", got, want)
	}
}

func TestCollectionService_RecordMonsterDefeat_CalculatesStrongAndDerivesIcon(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		monsters:    make(map[string]collection.MonsterBookEntry),
		completions: make(map[string]bool),
	}
	svc, err := collection.NewService(repo, 180, 10)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	record := collection.DefeatedMonsterRecord{
		MonsterID:   "monster-042",
		MonsterName: "ゴブリン",
		Habitat:     "草原",
		HP:          50,
		MP:          10,
		Attack:      20,
		Defense:     15,
		Agility:     18,
		// Strong is 0, Icon is empty -> should be auto-derived
	}

	if err := svc.RecordMonsterDefeat(ctx, "char-1", record); err != nil {
		t.Fatalf("RecordMonsterDefeat failed: %v", err)
	}

	entry, ok := repo.monsters["monster-042"]
	if !ok {
		t.Fatalf("entry not found in repo")
	}
	if entry.Icon != "mon/042.gif" {
		t.Errorf("expected Icon mon/042.gif, got %q", entry.Icon)
	}
	expectedStrong := collection.CalculateStrong(50, 10, 20, 15, 18)
	if entry.Strong != expectedStrong {
		t.Errorf("expected Strong %d, got %d", expectedStrong, entry.Strong)
	}
}

func TestCollectionService_CategorySeparation(t *testing.T) {
	ctx := context.Background()
	repo := &mockCollectionRepo{
		items:       make(map[string]collection.ItemCollectionEntry),
		completions: make(map[string]bool),
	}

	svc, err := collection.NewService(
		repo,
		10, // totalMonsters
		10, // totalItems
		collection.WithTotalWeapons(10),
		collection.WithTotalArmors(10),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	charID := "char-test-sep"

	// 1. Record weapon
	if err := svc.RecordItemDiscovered(ctx, charID, "weapon-40", "流銀の剣", "weapon"); err != nil {
		t.Fatalf("recording weapon failed: %v", err)
	}

	weapEntries, weapProg, err := svc.GetWeaponCollection(ctx, charID)
	if err != nil {
		t.Fatalf("GetWeaponCollection failed: %v", err)
	}
	if len(weapEntries) != 1 || weapProg.DiscoveredCount != 1 {
		t.Errorf("weapon collection expected 1 entry, got entries=%d count=%d", len(weapEntries), weapProg.DiscoveredCount)
	}

	armEntries, armProg, err := svc.GetArmorCollection(ctx, charID)
	if err != nil {
		t.Fatalf("GetArmorCollection failed: %v", err)
	}
	if len(armEntries) != 0 || armProg.DiscoveredCount != 0 {
		t.Errorf("armor collection expected 0 entries, got entries=%d count=%d", len(armEntries), armProg.DiscoveredCount)
	}

	itemEntries, itemProg, err := svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(itemEntries) != 0 || itemProg.DiscoveredCount != 0 {
		t.Errorf("item collection expected 0 entries after weapon discovery, got entries=%d count=%d", len(itemEntries), itemProg.DiscoveredCount)
	}

	// 2. Record armor
	if err := svc.RecordItemDiscovered(ctx, charID, "armor-40", "流銀の鎧", "armor"); err != nil {
		t.Fatalf("recording armor failed: %v", err)
	}

	armEntries, armProg, err = svc.GetArmorCollection(ctx, charID)
	if err != nil {
		t.Fatalf("GetArmorCollection failed: %v", err)
	}
	if len(armEntries) != 1 || armProg.DiscoveredCount != 1 {
		t.Errorf("armor collection expected 1 entry, got entries=%d count=%d", len(armEntries), armProg.DiscoveredCount)
	}

	itemEntries, itemProg, err = svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(itemEntries) != 0 || itemProg.DiscoveredCount != 0 {
		t.Errorf("item collection expected 0 entries after armor discovery, got entries=%d count=%d", len(itemEntries), itemProg.DiscoveredCount)
	}

	// 3. Record item
	if err := svc.RecordItemDiscovered(ctx, charID, "item-129", "神の錬金レシピ", "item"); err != nil {
		t.Fatalf("recording item failed: %v", err)
	}

	itemEntries, itemProg, err = svc.GetItemCollection(ctx, charID, "item")
	if err != nil {
		t.Fatalf("GetItemCollection failed: %v", err)
	}
	if len(itemEntries) != 1 || itemProg.DiscoveredCount != 1 {
		t.Errorf("item collection expected 1 entry after item discovery, got entries=%d count=%d", len(itemEntries), itemProg.DiscoveredCount)
	}
}
