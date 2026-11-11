package gemstore_test

import (
	"context"
	"errors"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/gemstore"
)

// Identity and row order from gem_store.cgi:24–82; alternative recipes remain separate rows.
func TestRecipeReferencesMatchLegacyTable(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	want := [][3]string{
		{"gem_atk_2", "gem_atk_1", "gem_atk_1"},
		{"gem_sky_atk_1", "gem_atk_2", "gem_atk_2"},
		{"gem_sky_atk_1", "gem_sky_atk_2", "item-043"},
		{"gem_sky_atk_2", "gem_sky_atk_1", "item-043"},
		{"gem_burst_sky", "gem_sky_atk_2", "item-059"},
		{"gem_combo_sky", "gem_combo_1", "gem_sky_atk_1"},
		{"gem_combo_burst_sky", "gem_combo_sky", "gem_burst_sky"},
		{"gem_dance_combo", "gem_combo_1", "item-008"},
		{"gem_all_atk_2", "gem_all_atk_1", "gem_all_atk_1"},
		{"gem_all_atk_sky", "gem_all_atk_2", "gem_sky_atk_1"},
		{"gem_magic_atk_2", "gem_magic_atk_1", "gem_magic_atk_1"},
		{"gem_all_magic_1", "gem_all_atk_1", "gem_magic_atk_1"},
		{"gem_all_breath_1", "gem_all_atk_1", "item-041"},
		{"gem_magic_atk_sky", "gem_magic_atk_2", "item-203"},
		{"gem_all_magic_sky", "gem_all_magic_1", "gem_magic_atk_sky"},
		{"gem_heal_sky_1", "gem_heal_1", "item-124"},
		{"gem_heal_sky_1", "gem_heal_sky_2", "item-043"},
		{"gem_heal_sky_2", "gem_heal_sky_1", "item-043"},
		{"gem_all_heal_1", "gem_heal_1", "item-133"},
		{"gem_all_heal_sky", "gem_all_heal_1", "gem_heal_sky_2"},
		{"gem_mp_heal_sky_1", "gem_heal_sky_2", "item-203"},
		{"gem_mp_heal_sky_1", "gem_mp_heal_sky_2", "item-043"},
		{"gem_mp_heal_sky_2", "gem_mp_heal_sky_1", "item-043"},
		{"gem_drain_hp_sky", "gem_magic_atk_sky", "gem_heal_sky_2"},
		{"gem_drain_mp_sky", "gem_magic_atk_sky", "gem_mp_heal_sky_2"},
		{"gem_buff_atk_2", "gem_buff_atk_1", "item-018"},
		{"gem_buff_def_2", "gem_buff_def_1", "item-019"},
		{"gem_buff_spd_2", "gem_buff_spd_1", "item-020"},
		{"gem_big_guard_sky", "gem_guard_sky", "item-161"},
		{"gem_guard_sky", "gem_guard_2", "gem_guard_2"},
		{"gem_counter_phys_sky", "gem_guard_2", "gem_counter_phys_1"},
		{"gem_counter_magic_sky", "gem_guard_2", "gem_counter_magic_1"},
		{"gem_auto_revive_sky", "gem_big_guard_sky", "item-103"},
		{"gem_magic_absorb_sky", "gem_big_guard_sky", "gem_mp_heal_sky_2"},
		{"gem_cure_poison", "gem_heal_1", "item-007"},
		{"gem_cure_paralysis", "gem_heal_1", "item-008"},
		{"gem_cure_sleep", "gem_heal_1", "item-009"},
		{"gem_cure_confusion", "gem_heal_1", "item-011"},
		{"gem_cure_paralyze_action", "gem_heal_1", "item-012"},
		{"gem_cure_atk_seal", "gem_heal_1", "item-005"},
		{"gem_cure_magic_seal", "gem_heal_1", "item-004"},
		{"gem_cure_all_abnormal", "gem_all_heal_1", "item-010"},
		{"gem_revive_sky_1", "gem_heal_sky_2", "item-194"},
		{"gem_revive_sky_1", "gem_revive_sky_2", "item-043"},
		{"gem_revive_sky_2", "gem_revive_sky_1", "item-043"},
		{"gem_revive_sky_2", "gem_revive_sky_3", "item-043"},
		{"gem_revive_sky_3", "gem_revive_sky_2", "item-043"},
		{"gem_divine_tension_sky", "gem_calm_sky", "item-206"},
		{"gem_calm_sky", "gem_calm_orb", "item-203"},
		{"gem_instant_death_sky", "gem_magic_atk_2", "item-085"},
		{"gem_kamikaze_sky", "gem_atk_2", "item-042"},
		{"gem_summon_sky", "gem_instant_death_sky", "item-078"},
		{"gem_metronome", "gem_sacrifice_soul", "item-085"},
		{"gem_awakening_1", "gem_cover", "item-158"},
		{"gem_awakening_sky", "gem_awakening_1", "item-081"},
		{"gem_secret_art_sky", "gem_metronome", "item-158"},
	}
	recipes := catalog.AllRecipes()
	if len(recipes) != len(want) {
		t.Fatalf("recipes = %d, want %d", len(recipes), len(want))
	}
	for i, r := range recipes {
		if got := [3]string{r.ResultID, r.Material1, r.Material2}; got != want[i] {
			t.Errorf("row %d (%s): %v, want %v", i, r.ID, got, want[i])
		}
	}
}

func TestSynthesizeGem_ItemMaterials(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	items, err := coreitem.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, recipe, gem, item, want string
	}{
		{"renamed item", "recipe_burst_sky", "gem_sky_atk_2", "item-059", "gem_burst_sky"},
		{"unchanged item", "recipe_dance_combo", "gem_combo_1", "item-008", "gem_dance_combo"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			char := corecharacter.Character{ID: "char_" + tc.recipe, PlayerID: "p1", Name: "Hero", JobLevel: 10}
			charRepo := newMockCharacterRepo()
			charRepo.characters[char.ID] = char
			depotRepo, boxRepo := newMockDepotRepo(), newMockGemBoxRepo()

			gem, _ := coreitem.NewInstance(tc.gem, 1)
			if err := boxRepo.Save(ctx, gemstore.GemBox{CharacterID: char.ID, Capacity: 5, Items: []coreitem.Instance{gem}}); err != nil {
				t.Fatal(err)
			}
			mat, _ := coreitem.NewInstance(tc.item, 1)
			dep, _ := depot.NewDepot(char.ID)
			dep.Items = append(dep.Items, mat)
			// The matching depot gem must survive: the Gem Box has priority.
			spare, _ := coreitem.NewInstance(tc.gem, 1)
			dep.Items = append(dep.Items, spare)
			if err := depotRepo.Save(ctx, dep); err != nil {
				t.Fatal(err)
			}

			svc, err := gemstore.NewService(catalog, charRepo,
				gemstore.WithDepotRepository(depotRepo),
				gemstore.WithGemBoxRepository(boxRepo),
				gemstore.WithItemDefinitionProvider(items),
			)
			if err != nil {
				t.Fatal(err)
			}
			res, err := svc.SynthesizeGem(ctx, char.ID, tc.recipe)
			if err != nil {
				t.Fatalf("SynthesizeGem: %v", err)
			}
			box, _ := boxRepo.FindByCharacterID(ctx, char.ID)
			dep, _ = depotRepo.FindByCharacterID(ctx, char.ID)
			if res.CreatedGem.ID != tc.want || len(box.Items) != 1 || box.Items[0].DefinitionID != tc.want ||
				len(dep.Items) != 1 || dep.Items[0] != spare {
				t.Fatalf("want %s and spare depot gem, got box=%+v depot=%+v", tc.want, box.Items, dep.Items)
			}
		})
	}
}

func TestSynthesizeGem_RejectsNameAndInstanceIDMatches(t *testing.T) {
	catalog, err := gemstore.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	char := corecharacter.Character{ID: "decoy", PlayerID: "p1"}
	chars := newMockCharacterRepo()
	chars.characters[char.ID] = char
	for _, tc := range []struct{ name, boxGem, depotItem string }{
		{"gem instance ID is not a definition ID", "gem_atk_1", "item-008"},
		{"item display name is not a definition ID", "gem_combo_1", "decoy-item"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			boxes := newMockGemBoxRepo()
			if err := boxes.Save(ctx, gemstore.GemBox{CharacterID: char.ID, Capacity: 5, Items: []coreitem.Instance{
				{ID: "gem_combo_1", DefinitionID: tc.boxGem, Quantity: 1},
			}}); err != nil {
				t.Fatal(err)
			}
			depots := depotHolding(char.ID, coreitem.Instance{ID: "item-008", DefinitionID: tc.depotItem, Quantity: 1})
			provider := &mockItemProvider{items: map[string]coreitem.Definition{"decoy-item": {ID: "decoy-item", Name: "満月草"}}}
			svc, err := gemstore.NewService(catalog, chars, gemstore.WithDepotRepository(depots),
				gemstore.WithGemBoxRepository(boxes), gemstore.WithItemDefinitionProvider(provider))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.SynthesizeGem(ctx, char.ID, "recipe_dance_combo"); !errors.Is(err, gemstore.ErrInsufficientMaterials) {
				t.Fatalf("non-definition match accepted: %v", err)
			}
		})
	}
}
