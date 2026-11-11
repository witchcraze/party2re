package gemstore_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// legacyNos expands legacy gem numbers; a two-element array is an inclusive range.
func legacyNos(parts ...any) []int {
	var out []int
	for _, p := range parts {
		switch v := p.(type) {
		case int:
			out = append(out, v)
		case [2]int:
			for n := v[0]; n <= v[1]; n++ {
				out = append(out, n)
			}
		}
	}
	return out
}

// Legacy gem_store.cgi sales branches, written as legacy gem numbers.
var (
	legacySales0   = legacyNos(1, 19, 52)
	legacySales10  = legacyNos(1, 6, 13, 19, 52, 54)
	legacySales30  = legacyNos(1, 6, 10, 13, 16, 19, 28, 30, 32, 34, [2]int{37, 42}, 52, 54, 74, [2]int{87, 97})
	legacySales50  = legacyNos(1, 6, 9, 10, 13, 16, 18, 19, 22, 28, 30, 32, 34, [2]int{37, 44}, 46, 47, 49, 50, 52, [2]int{54, 61}, 68, 70, 74, [2]int{75, 81}, [2]int{87, 97})
	legacySales100 = legacyNos(1, 2, 6, 9, 10, 11, 13, 14, 16, 18, 19, 22, [2]int{28, 35}, [2]int{38, 44}, 46, 47, 49, 50, 52, [2]int{54, 62}, 66, 68, 70, 73, 74, [2]int{75, 81}, [2]int{87, 97})
)

// The catalog lists gems in legacy number order, so legacy number n is AllGems()[n-1].
func gemIDsForLegacyNos(t *testing.T, c *gemstore.Catalog, nos []int) []string {
	t.Helper()
	all := c.AllGems()
	ids := make([]string, 0, len(nos))
	for _, n := range nos {
		ids = append(ids, all[n-1].ID)
	}
	return ids
}

func TestCatalog_GemsFollowLegacyNumberOrder(t *testing.T) {
	c, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	all := c.AllGems()
	if len(all) != 97 {
		t.Fatalf("catalog has %d gems, want legacy 1..97", len(all))
	}
	for no, name := range map[int]string{1: "攻撃の宝珠Ⅰ", 3: "攻撃の天珠Ⅰ", 37: "大防装の天珠", 86: "秘技の天珠", 87: "呼出の宝珠", 92: "特令の天珠", 93: "攻減の宝珠", 97: "魔減の宝珠"} {
		if all[no-1].Name != name {
			t.Errorf("legacy No.%d = %q, want %q", no, all[no-1].Name, name)
		}
	}
}

func TestCatalog_SalesMatchLegacyTiers(t *testing.T) {
	c, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tt := range []struct {
		jobLevel int
		nos      []int
	}{
		{0, legacySales0}, {9, legacySales0},
		{10, legacySales10}, {29, legacySales10},
		{30, legacySales30}, {49, legacySales30},
		{50, legacySales50}, {99, legacySales50},
		{100, legacySales100}, {250, legacySales100},
	} {
		var got []string
		for _, g := range c.GetGemsForJobLevel(tt.jobLevel) {
			got = append(got, g.ID)
		}
		if want := gemIDsForLegacyNos(t, c, tt.nos); !slices.Equal(got, want) {
			t.Errorf("job_lv %d: got %d gems %v, want %d %v", tt.jobLevel, len(got), got, len(want), want)
		}
	}
}

func TestGemStore_BuyGem_EnforcesSalesTier(t *testing.T) {
	c, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	all := c.AllGems()
	for _, tt := range []struct {
		name     string
		jobLevel int
		no       int
		wantErr  error
	}{
		{"tier 0 sells cover", 0, 52, nil},
		{"combo below tier 10", 9, 6, gemstore.ErrGemNotForSale},
		{"combo at tier 10", 10, 6, nil},
		{"call gem below tier 30", 29, 87, gemstore.ErrGemNotForSale},
		{"call gem at tier 30", 30, 87, nil},
		{"big guard at tier 50", 99, 37, nil},
		{"big guard dropped at tier 100", 100, 37, gemstore.ErrGemNotForSale},
		{"synthesis-only sky gem never sold", 100, 3, gemstore.ErrGemNotForSale},
		{"special order at tier 100", 100, 92, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			charRepo := newMockCharacterRepo()
			charRepo.characters["c"] = corecharacter.Character{ID: "c", JobLevel: tt.jobLevel, Money: 100000}
			svc, err := gemstore.NewService(c, charRepo, newMockInventoryRepo())
			if err != nil {
				t.Fatal(err)
			}
			gem := all[tt.no-1]
			res, err := svc.BuyGem(context.Background(), "c", gem.ID)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("BuyGem(%s) at job_lv %d: err = %v, want %v", gem.ID, tt.jobLevel, err, tt.wantErr)
			}
			stored := charRepo.characters["c"]
			if tt.wantErr != nil {
				if stored.Money != 100000 {
					t.Errorf("rejected purchase changed money to %d", stored.Money)
				}
				return
			}
			if res.Cost != gem.Price*5 || stored.Money != 100000-gem.Price*5 || len(res.GemBox.Items) != 1 || res.GemBox.Items[0].DefinitionID != gem.ID {
				t.Errorf("purchase: cost=%d money=%d box=%v", res.Cost, stored.Money, res.GemBox.Items)
			}
		})
	}
}
