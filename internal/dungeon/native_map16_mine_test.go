package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/dungeon"
)

// placeAtSite stands the party on main-floor site B (6,8) with the site active.
func (f *nativeFixture) placeAtSite(t *testing.T, id string, round int, depleted ...string) {
	t.Helper()
	f.setFloor(t, id, 1, 6, 8)
	f.setMap16State(t, id, map16State{Site: "B", Depleted: depleted}, round)
}

// Mining needs a living member at an active site with no living enemy or
// unopened chest.
func TestNativeMap16_MineEligibility(t *testing.T) {
	ctx := context.Background()
	f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "elig", map16Monday)
	if _, err := f.svc.Mine(ctx, "elig"); !errors.Is(err, dungeon.ErrNoMiningSite) {
		t.Fatalf("no site: %v", err)
	}
	if exp := f.reload(t, "elig"); exp.Controls("elig").CanMine {
		t.Fatal("CanMine without a site")
	}
	f.placeAtSite(t, "elig", 5)
	if exp := f.reload(t, "elig"); !exp.Controls("elig").CanMine {
		t.Fatal("CanMine false at an active site")
	}
	f.rng.floats = []float64{0.3, 0.2, 0, 0, 0, 0, 0, 0, 0, 0.99} // a crystal-rock group that stays
	if res, err := f.svc.Mine(ctx, "elig"); err != nil || res.EventType != dungeon.EventBattle {
		t.Fatalf("mine: %s err=%v", res.EventType, err)
	}
	if _, err := f.svc.Mine(ctx, "elig"); !errors.Is(err, dungeon.ErrCombatActive) {
		t.Fatalf("living enemy: %v", err)
	}
	if f.reload(t, "elig").Controls("elig").CanMine {
		t.Fatal("CanMine with a living enemy")
	}
	f.consume(t, "elig")
	f.rng.floats = []float64{0.3, 0.3, 0.99} // treasure, site kept
	if res, err := f.svc.Mine(ctx, "elig"); err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("treasure: %s err=%v", res.EventType, err)
	}
	if _, err := f.svc.Mine(ctx, "elig"); !errors.Is(err, dungeon.ErrCombatActive) {
		t.Fatalf("unopened chest: %v", err)
	}
	f.consume(t, "elig")
	f.setVitality(t, "elig", "elig", 0, 0)
	if _, err := f.svc.Mine(ctx, "elig"); !errors.Is(err, dungeon.ErrActorFallen) {
		t.Fatalf("fallen actor: %v", err)
	}
	if _, err := f.svc.Escape(ctx, "elig"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Mine(ctx, "elig"); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatalf("no run: %v", err)
	}
	// Another map has no mining at all.
	g := newMap17Fixture(t, dungeon.NewMemoryExpeditionRepository(), "other")
	if _, err := g.svc.Mine(ctx, "other"); !errors.Is(err, dungeon.ErrNoMiningSite) {
		t.Fatalf("map17: %v", err)
	}
}

// Each branch and its depletion boundary; mining spends no round, works past
// the limit and keeps the outcome across reload.
func TestNativeMap16_MineOutcomes(t *testing.T) {
	testMap14Stores(t, mineOutcomes)
}

func mineOutcomes(t *testing.T, store dungeon.ActiveExpeditionStore, id string) {
	ctx := context.Background()
	group := []float64{0, 0, 0, 0, 0, 0, 0} // one member: size, then six rolls
	withGroup := func(head []float64, deplete float64) []float64 {
		return append(append(slices.Clone(head), group...), deplete)
	}
	for _, tc := range []struct {
		name     string
		floats   []float64
		event    dungeon.TileEventType
		depleted bool
	}{
		{"nothing depletes", []float64{0.1666, 0.2499}, dungeon.EventMineEmpty, true},
		{"nothing stays", []float64{0.1666, 0.25}, dungeon.EventMineEmpty, false},
		{"monster depletes", withGroup([]float64{0.1667, 0.2499}, 0.3333), dungeon.EventBattle, true},
		{"monster stays", withGroup([]float64{0.1667, 0.2499}, 0.3334), dungeon.EventBattle, false},
		{"treasure depletes", []float64{0.1667, 0.25, 0.4999}, dungeon.EventTreasure, true},
		{"treasure stays", []float64{0.1667, 0.25, 0.5}, dungeon.EventTreasure, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMap16Fixture(t, store, id, map16Monday)
			defer func() { _, _ = f.svc.Escape(ctx, id) }()
			f.placeAtSite(t, id, 150) // at the limit, where moves are rejected
			before := f.reload(t, id)
			f.rng.floats = tc.floats
			res, err := f.svc.Mine(ctx, id)
			if err != nil {
				t.Fatal(err)
			}
			after := f.reload(t, id)
			st := map16StateOf(t, after)
			if res.EventType != tc.event || st.Site != "B" || slices.Contains(st.Depleted, "B") != tc.depleted ||
				after.Round != 150 || after.PosX != 6 || after.PosY != 8 || after.Revision != before.Revision+1 {
				t.Fatalf("event %s state %s round %d at (%d,%d) rev %d→%d", res.EventType, after.MapState, after.Round, after.PosX, after.PosY, before.Revision, after.Revision)
			}
			switch tc.event {
			case dungeon.EventBattle:
				if after.Encounter == nil || monsterIDs(after.Encounter.Monsters)[0] != "nm16-crystal-rock" {
					t.Fatalf("monster: %+v", after.Encounter)
				}
			case dungeon.EventTreasure:
				if after.Encounter == nil || after.Encounter.EventType != dungeon.EventTreasure || len(after.Encounter.Chests) != 1 {
					t.Fatalf("treasure: %+v", after.Encounter)
				}
			default:
				if after.Encounter != nil {
					t.Fatalf("nothing created %+v", after.Encounter)
				}
			}
			f.rejectStaleMap14Step(t, id, before, after)
		})
	}
}

// A depleted site accepts mining with no draw and no change.
func TestNativeMap16_MineDepletedSite(t *testing.T) {
	ctx := context.Background()
	f := newMap16Fixture(t, dungeon.NewMemoryExpeditionRepository(), "dry", map16Monday)
	f.placeAtSite(t, "dry", 3, "B")
	before := f.reload(t, "dry")
	res, err := f.svc.Mine(ctx, "dry")
	after := f.reload(t, "dry")
	if err != nil || res.EventType != dungeon.EventMineDepleted || f.rng.calls() != 0 || after.Revision != before.Revision || string(after.MapState) != string(before.MapState) {
		t.Fatalf("depleted: %s draws %d rev %d→%d err=%v", res.EventType, f.rng.calls(), before.Revision, after.Revision, err)
	}
	if !after.Controls("dry").CanMine {
		t.Fatal("availability disclosed the depletion")
	}
}

// Opened chests from an earlier treasure do not block mining, and mining
// skips the pre-move reset (item 188's revival).
func TestNativeMap16_MineAfterOpenedChestsWithoutReset(t *testing.T) {
	ctx := context.Background()
	catalog := append(dungeon.DefaultDungeonCatalog(), dungeon.NativeMap16())
	f := newNativeFixtureOn(t, dungeon.NewMemoryExpeditionRepository(), map16Hero("lead"), dungeon.WithCustomDungeons(catalog))
	f.chars.chars["ally"] = map16Hero("ally")
	f.startNativeParty(t, dungeon.NativeMap16ID, 1, 6, 8)
	f.setMap16State(t, "lead", map16State{Site: "B"}, 4)
	exp := f.reload(t, "lead")
	exp.Encounter = &dungeon.ActiveEncounter{EventType: dungeon.EventTreasure, X: 6, Y: 8, Chests: []dungeon.TreasureChest{{Opened: true}}}
	for i := range exp.Members {
		if exp.Members[i].CharacterID == "lead" {
			exp.Members[i].ItemIDs = []string{"item-188"}
		} else {
			exp.Members[i].CurrentHP = 0
		}
	}
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.floats, f.rng.intCalls, f.rng.floatCalls = []float64{0.1666, 0.25}, 0, 0
	res, err := f.svc.Mine(ctx, "lead")
	if err != nil || res.EventType != dungeon.EventMineEmpty || f.rng.calls() != 2 {
		t.Fatalf("mine: %s draws %d err=%v", res.EventType, f.rng.calls(), err)
	}
	after := f.reload(t, "lead")
	if after.Encounter != nil || memberOf(t, after, "ally").CurrentHP != 0 {
		t.Fatalf("encounter %+v, ally HP %d", after.Encounter, memberOf(t, after, "ally").CurrentHP)
	}
	// The same roster revives on a move, so the check above is not vacuous.
	f.rng.floats = []float64{0, 0.99}
	if moved := f.move16(t, "lead", dungeon.DirectionNorth); memberOf(t, moved, "ally").CurrentHP == 0 {
		t.Fatal("item 188 did not revive on a move either")
	}
}
