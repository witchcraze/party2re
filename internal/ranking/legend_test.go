package ranking_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/ranking"
)

func TestLegendCategories(t *testing.T) {
	categories := ranking.AllLegendCategories()
	if len(categories) != 6 {
		t.Fatalf("expected 6 legend categories, got %d", len(categories))
	}

	expectedCats := []ranking.LegendCategory{
		ranking.LegendCategoryJobMastery,
		ranking.LegendCategoryMonsterMastery,
		ranking.LegendCategoryWeaponMastery,
		ranking.LegendCategoryArmorMastery,
		ranking.LegendCategoryItemMastery,
		ranking.LegendCategoryAlchemyMastery,
	}

	for _, cat := range expectedCats {
		if !ranking.IsValidLegendCategory(cat) {
			t.Fatalf("expected category %s to be valid", cat)
		}
		info, found := ranking.GetLegendCategoryInfo(cat)
		if !found {
			t.Fatalf("expected to find info for category %s", cat)
		}
		if info.Title == "" {
			t.Fatalf("expected non-empty title for category %s", cat)
		}
	}

	if ranking.IsValidLegendCategory(ranking.LegendCategory("invalid_cat")) {
		t.Fatal("expected invalid category to return false")
	}
}

func TestService_HallOfFame(t *testing.T) {
	repo := newMockRepo()
	fixedTime := time.Date(2026, 9, 23, 10, 0, 0, 0, time.UTC)
	svc, err := ranking.NewService(repo, ranking.WithNowFunc(func() time.Time { return fixedTime }))
	if err != nil {
		t.Fatalf("failed to create ranking service: %v", err)
	}

	ctx := context.Background()

	// 1. Initial categories listing
	cats, err := svc.GetLegends(ctx)
	if err != nil {
		t.Fatalf("GetLegends failed: %v", err)
	}
	if len(cats) != 6 {
		t.Fatalf("expected 6 categories, got %d", len(cats))
	}
	for _, c := range cats {
		if c.TotalCount != 0 {
			t.Fatalf("expected 0 inductees initially for %s, got %d", c.Category, c.TotalCount)
		}
	}

	// 2. Induct hero into comp_job
	entry1 := ranking.LegendEntry{
		Category:      ranking.LegendCategoryJobMastery,
		CharacterID:   "char-1",
		CharacterName: "LegendaryHero",
		GuildName:     "Knights",
		Color:         "#ff0000",
		Icon:          "hero.png",
		Message:       "Mastered all jobs!",
	}
	inducted, err := svc.RecordLegend(ctx, entry1)
	if err != nil {
		t.Fatalf("RecordLegend failed: %v", err)
	}
	if !inducted {
		t.Fatal("expected character to be newly inducted")
	}

	// 3. Duplicate prevention: second induction of same character in same category returns false
	inductedAgain, err := svc.RecordLegend(ctx, entry1)
	if err != nil {
		t.Fatalf("second RecordLegend failed: %v", err)
	}
	if inductedAgain {
		t.Fatal("expected duplicate induction to return false")
	}

	// 4. Induct second character into comp_job
	entry2 := ranking.LegendEntry{
		Category:      ranking.LegendCategoryJobMastery,
		CharacterID:   "char-2",
		CharacterName: "SecondHero",
		GuildName:     "Mages",
		Color:         "#0000ff",
		Icon:          "mage.png",
		Message:       "All jobs finished!",
	}
	inducted2, err := svc.RecordLegend(ctx, entry2)
	if err != nil || !inducted2 {
		t.Fatalf("expected second character induction to succeed: %v", err)
	}

	// 5. Query comp_job category
	page, err := svc.GetLegendCategory(ctx, ranking.LegendCategoryJobMastery)
	if err != nil {
		t.Fatalf("GetLegendCategory failed: %v", err)
	}
	if page.Total != 2 {
		t.Fatalf("expected 2 inductees, got %d", page.Total)
	}
	if len(page.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(page.Entries))
	}
	if page.Entries[0].CharacterID != "char-1" || page.Entries[1].CharacterID != "char-2" {
		t.Fatalf("unexpected inductee order: %+v", page.Entries)
	}

	// 6. Check counts in GetLegends
	catsAfter, err := svc.GetLegends(ctx)
	if err != nil {
		t.Fatalf("GetLegends after induction failed: %v", err)
	}
	for _, c := range catsAfter {
		if c.Category == ranking.LegendCategoryJobMastery {
			if c.TotalCount != 2 {
				t.Fatalf("expected 2 inductees for comp_job, got %d", c.TotalCount)
			}
		} else {
			if c.TotalCount != 0 {
				t.Fatalf("expected 0 inductees for %s, got %d", c.Category, c.TotalCount)
			}
		}
	}

	// 7. Invalid category returns error
	_, err = svc.GetLegendCategory(ctx, ranking.LegendCategory("invalid_category"))
	if err != ranking.ErrInvalidLegendCategory {
		t.Fatalf("expected ErrInvalidLegendCategory, got %v", err)
	}

	_, err = svc.RecordLegend(ctx, ranking.LegendEntry{
		Category:    ranking.LegendCategory("invalid_category"),
		CharacterID: "char-3",
	})
	if err != ranking.ErrInvalidLegendCategory {
		t.Fatalf("expected ErrInvalidLegendCategory on record, got %v", err)
	}
}
