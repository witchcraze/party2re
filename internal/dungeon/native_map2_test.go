package dungeon_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

func map2Hero(id string, hp int) corecharacter.Character {
	c := createTestChar(id, 30, hp, 900, 900)
	c.JobLevel = 5
	return c
}

// placeMap2 starts on variant 1, then rewrites floor, position, round and flags.
func (f *nativeFixture) placeMap2(t *testing.T, id string, variant, x, y, round int, consumed ...string) {
	t.Helper()
	f.placeNative(t, dungeon.NativeMap2ID, 30, id, 1, x, y, round)
	exp := f.reload(t, id)
	exp.MapVariant, exp.RoundLimit, exp.ConsumedEvents = variant, []int{30, 30, 30, 40, 40, 40, 40, 50, 50, 50, 50}[variant-1], consumed
	if err := f.store.SaveActiveExpedition(context.Background(), exp); err != nil {
		t.Fatal(err)
	}
}

func TestNativeMap2_CatalogAndInitialFloors(t *testing.T) {
	var d dungeon.Dungeon
	for _, c := range dungeon.DefaultDungeonCatalog() {
		if c.ID == dungeon.NativeMap2ID {
			d = c
		}
	}
	if d.RoundLimit != 30 || d.LeaderJobLevel != 5 || len(d.Variants) != 11 {
		t.Fatalf("limit=%d job=%d variants=%d", d.RoundLimit, d.LeaderJobLevel, len(d.Variants))
	}
	// Entry skips files with "_": only 1.cgi–3.cgi start a run.
	f := newNativeFixture(t, map2Hero("start", 5000))
	f.rng.ints = []int{5} // 5 % 3 = 2
	exp, err := f.svc.StartExpedition(context.Background(), "start", dungeon.NativeMap2ID)
	if err != nil {
		t.Fatal(err)
	}
	if exp.MapVariant != 3 || exp.PosX != 2 || exp.PosY != 4 || exp.Round != 1 || exp.RoundLimit != 30 {
		t.Fatalf("start %+v", exp)
	}
}

// Floor 1 F (event_F: _1–_3, never _4) keeps position and round; floor 2's limit is 40.
func TestNativeMap2_RouteCarriesPositionAndRound(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("route", 5000))
	f.placeMap2(t, "route", 1, 1, 0, 5, "7")
	f.rng.ints = []int{2}
	res, err := f.svc.Move(ctx, "route", dungeon.DirectionWest)
	if err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "route")
	if res.EventType != dungeon.EventMove || exp.MapVariant != 6 || exp.PosX != 0 || exp.PosY != 0 || exp.Round != 6 || exp.RoundLimit != 40 ||
		exp.Encounter != nil || !slices.Equal(exp.ConsumedEvents, []string{"7"}) || f.rng.intCalls != 1 || f.rng.floatCalls != 0 {
		t.Fatalf("route %+v draws=%d/%d", exp, f.rng.intCalls, f.rng.floatCalls)
	}
	view, err := f.svc.ObserveExpedition(ctx, "route")
	if err != nil || view.Floor != 2 || view.Round != 6 || view.RoundLimit != 40 {
		t.Fatalf("view %+v err=%v", view, err)
	}

	// Third floor F leads to the vault (___1–___3); second floor F always to __1.
	for _, tc := range []struct{ from, x, y, draw, want int }{{4, 2, 3, 0, 8}, {8, 1, 1, 1, 10}} {
		g := newNativeFixture(t, map2Hero("up", 5000))
		g.placeMap2(t, "up", tc.from, tc.x, tc.y, 10, "8", "B:1:1")
		g.rng.ints = []int{tc.draw}
		dir := dungeon.DirectionSouth
		if tc.from == 8 {
			dir = dungeon.DirectionNorth
		}
		if _, err := g.svc.Move(ctx, "up", dir); err != nil {
			t.Fatal(err)
		}
		if exp := g.reload(t, "up"); exp.MapVariant != tc.want || exp.RoundLimit != 50 {
			t.Fatalf("from %d: %+v", tc.from, exp)
		}
	}
}

// T draws the destination, then int(base × (0.9 + rand(0.3))) per member.
func TestNativeMap2_PitfallDamageBasesAndDestinations(t *testing.T) {
	for _, tc := range []struct {
		name                string
		variant, x, y, draw int
		dir                 dungeon.Direction
		tx, ty              int
		float               float64
		wantVariant, damage int
	}{
		{"floor2 min", 4, 2, 2, 1, dungeon.DirectionSouth, 2, 3, 0, 2, 27},
		{"floor2 max", 5, 0, 2, 0, dungeon.DirectionSouth, 0, 3, 0.9999, 1, 35},
		{"floor3", 8, 1, 3, 3, dungeon.DirectionNorth, 1, 2, 0.5, 7, 73},
		{"floor3 max", 8, 0, 2, 0, dungeon.DirectionEast, 1, 2, 0.9999, 4, 83},
		{"vault", 10, 1, 3, 0, dungeon.DirectionSouth, 1, 4, 0, 8, 36},
		{"vault max", 9, 0, 2, 0, dungeon.DirectionSouth, 0, 3, 0.9999, 8, 47},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newNativeFixture(t, map2Hero("pit", 5000))
			f.placeMap2(t, "pit", tc.variant, tc.x, tc.y, 10)
			before := f.reload(t, "pit")
			f.rng.ints, f.rng.floats = []int{tc.draw}, []float64{tc.float}
			res, err := f.svc.Move(context.Background(), "pit", tc.dir)
			if err != nil {
				t.Fatal(err)
			}
			exp := f.reload(t, "pit")
			if exp.MapVariant != tc.wantVariant || exp.PosX != tc.tx || exp.PosY != tc.ty ||
				exp.Round != 11 || exp.Members[0].CurrentHP != before.Members[0].CurrentHP-tc.damage || res.MemberDamages["pit"] != tc.damage ||
				exp.Encounter != nil || len(exp.ConsumedEvents) != 0 {
				t.Fatalf("pitfall %+v damages=%v", exp, res.MemberDamages)
			}
		})
	}
}

// _1/_3 T at (2,3) drops into floor 1 variant 3's wall cell; the party walks out.
func TestNativeMap2_PitfallMayLandInAWall(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("wall", 5000))
	f.placeMap2(t, "wall", 4, 2, 2, 10)
	f.rng.ints = []int{2}
	if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "wall"); exp.MapVariant != 3 || exp.PosX != 2 || exp.PosY != 3 {
		t.Fatalf("landing %+v", exp)
	}
	if _, err := f.svc.Move(ctx, "wall", dungeon.DirectionNorth); err != nil {
		t.Fatalf("leaving the wall cell: %v", err)
	}
}

// ___3 T at (1,2) lands on __1's own T, which stays untriggered until entered again.
func TestNativeMap2_PitfallLandingDispatchesNothingAndRepeats(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("again", 5000))
	f.placeMap2(t, "again", 11, 1, 1, 10)
	f.rng.ints, f.rng.floats = []int{0}, []float64{0}
	if _, err := f.svc.Move(ctx, "again", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "again"); exp.MapVariant != 8 || exp.PosX != 1 || exp.PosY != 2 || f.rng.intCalls != 1 || f.rng.floatCalls != 1 {
		t.Fatalf("landing %+v draws=%d/%d", exp, f.rng.intCalls, f.rng.floatCalls)
	}
	// Q is a quiet passage: no spawn draw.
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	if _, err := f.svc.Move(ctx, "again", dungeon.DirectionWest); err != nil || f.rng.calls() != 0 {
		t.Fatalf("Q err=%v draws=%d", err, f.rng.calls())
	}
	f.rng.ints, f.rng.floats = []int{0}, []float64{0}
	if _, err := f.svc.Move(ctx, "again", dungeon.DirectionEast); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "again"); exp.MapVariant != 4 || exp.Round != 13 {
		t.Fatalf("repeat %+v", exp)
	}
}

// A pitfall to a lower-limit floor can leave the round above the limit.
func TestNativeMap2_PitfallAboveTheLowerLimitExhaustsMovement(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("late", 5000))
	f.placeMap2(t, "late", 4, 2, 2, 34)
	f.rng.ints = []int{0}
	if _, err := f.svc.Move(ctx, "late", dungeon.DirectionSouth); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "late")
	if before.Round != 35 || before.RoundLimit != 30 || before.Status != dungeon.StatusExploring {
		t.Fatalf("after pitfall %+v", before)
	}
	if _, err := f.svc.Move(ctx, "late", dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("move err=%v", err)
	}
	view, err := f.svc.ObserveExpedition(ctx, "late")
	if err != nil || view.Round != 35 || view.RoundLimit != 30 || view.Floor != 1 || view.Controls.CanMove || !view.Controls.CanEscape {
		t.Fatalf("view %+v err=%v", view, err)
	}
}

func TestNativeMap2_PitfallHitsEveryMember(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("lead", 5000))
	f.chars.chars["ally"] = map2Hero("ally", 5000)
	f.rng.ints = []int{0}
	if _, err := f.svc.StartPartyExpedition(ctx, "lead", []string{"lead", "ally"}, dungeon.NativeMap2ID, "p"); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, "lead")
	exp.MapVariant, exp.RoundLimit, exp.PosX, exp.PosY = 4, 40, 2, 2
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.floats = []int{0}, []float64{0, 0.9999}
	res, err := f.svc.Move(ctx, "lead", dungeon.DirectionSouth)
	if err != nil {
		t.Fatal(err)
	}
	if res.MemberDamages["lead"] != 27 || res.MemberDamages["ally"] != 35 {
		t.Fatalf("damages %v", res.MemberDamages)
	}
}

// A pitfall that fells the whole roster keeps the run until Escape, which
// still banks the stock (legacy _trap_d never ends the quest).
func TestNativeMap2_PitfallFellsThePartyUntilEscape(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("frail", 20))
	f.placeMap2(t, "frail", 4, 2, 2, 10)
	exp := f.reload(t, "frail")
	exp.AccumulatedExp, exp.AccumulatedGold = 7, 9
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints = []int{0}
	res, err := f.svc.Move(ctx, "frail", dungeon.DirectionSouth)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsFinished || res.Expedition.Status != dungeon.StatusExploring || res.MemberDamages["frail"] == 0 {
		t.Fatalf("pitfall ended the run %+v", res)
	}
	if c := f.reload(t, "frail").Controls("frail"); c.CanMove || c.CanFight || !c.CanEscape {
		t.Fatalf("fallen controls %+v", c)
	}
	esc, err := f.svc.Escape(ctx, "frail")
	h := f.repo.histories["frail"]
	if err != nil || esc.Expedition.Status != dungeon.StatusEscaped || len(h) != 1 || h[0].ExpReward != 7 || h[0].GoldReward != 9 {
		t.Fatalf("escape %+v %v history=%+v", esc, err, h)
	}
	// A retried Escape settles nothing twice.
	if _, err := f.svc.Escape(ctx, "frail"); err != nil && !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatal(err)
	}
	if len(f.repo.histories["frail"]) != 1 {
		t.Fatalf("double settlement %+v", f.repo.histories["frail"])
	}
}

// 7 (two cells, stage 3 boss) and 8 (stage 4 boss) are once per run across floors.
func TestNativeMap2_FloorBossesAreOncePerRun(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("boss", 5000))
	f.placeMap2(t, "boss", 1, 0, 2, 3)
	res, err := f.svc.Move(ctx, "boss", dungeon.DirectionNorth)
	if err != nil {
		t.Fatal(err)
	}
	if res.Monster == nil || res.Monster.Name != "デビルシェル" || res.EventType != dungeon.EventBoss {
		t.Fatalf("7 %+v", res)
	}
	exp := f.reload(t, "boss")
	if !slices.Contains(exp.ConsumedEvents, "7") {
		t.Fatalf("7 not consumed %+v", exp.ConsumedEvents)
	}
	exp.PosX, exp.PosY, exp.Encounter = 2, 0, nil
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.intCalls, f.rng.floatCalls = 0, 0
	if res, err := f.svc.Move(ctx, "boss", dungeon.DirectionWest); err != nil || res.Monster != nil || f.rng.calls() != 0 {
		t.Fatalf("second 7 %+v err=%v", res, err)
	}

	g := newNativeFixture(t, map2Hero("golem", 5000))
	g.placeMap2(t, "golem", 4, 0, 4, 3)
	if res, err := g.svc.Move(ctx, "golem", dungeon.DirectionEast); err != nil || res.Monster == nil || res.Monster.Name != "ゴーレム" {
		t.Fatalf("8 %+v err=%v", res, err)
	}
}

// Floor 1 loads stage 2, 3 or 4 per spawning request (int(rand(3)+2)).
func TestNativeMap2_PassageDrawsTheFloorStage(t *testing.T) {
	f := newNativeFixture(t, map2Hero("spawn", 5000))
	f.placeMap2(t, "spawn", 1, 0, 2, 3)
	f.rng.floats, f.rng.ints = []float64{0.5}, []int{1, 3}
	res, err := f.svc.Move(context.Background(), "spawn", dungeon.DirectionEast)
	if err != nil {
		t.Fatal(err)
	}
	if res.Monster == nil || res.Monster.Name != "メイジバット" {
		t.Fatalf("spawn %+v", res.Monster)
	}
}

func TestNativeMap2_VaultTreasureAndMarks(t *testing.T) {
	ctx := context.Background()
	f := newNativeFixture(t, map2Hero("vault", 5000))
	f.placeMap2(t, "vault", 9, 1, 2, 10)
	tiles := func() []string {
		v, err := f.svc.ObserveExpedition(ctx, "vault")
		if err != nil {
			t.Fatal(err)
		}
		return v.Map.Tiles[2]
	}
	if got := tiles(); !slices.Equal(got, []string{"□", "宝", "□"}) {
		t.Fatalf("row below %v", got)
	}
	res, err := f.svc.Move(ctx, "vault", dungeon.DirectionSouth)
	if err != nil || res.EventType != dungeon.EventTreasure {
		t.Fatalf("2 %+v err=%v", res, err)
	}
	exp := f.reload(t, "vault")
	if !slices.Equal(exp.ConsumedEvents, []string{"2"}) || exp.Encounter == nil {
		t.Fatalf("after 2 %+v", exp)
	}
	exp.Encounter, exp.PosY = nil, 2
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if got := tiles(); !slices.Equal(got, []string{"□", "□", "□"}) {
		t.Fatalf("opened row %v", got)
	}

	// The third floor shows F, an unbeaten B, and blank Q/T.
	g := newNativeFixture(t, map2Hero("third", 5000))
	g.placeMap2(t, "third", 8, 1, 1, 10)
	v, err := g.svc.ObserveExpedition(ctx, "third")
	if err != nil || !slices.Equal(v.Map.Tiles[0], []string{"■", "凸", "■"}) || !slices.Equal(v.Map.Tiles[2], []string{"　", "　", "　"}) {
		t.Fatalf("third floor %v err=%v", v, err)
	}
	g.moveTo(t, "third", 1, 2)
	if v, _ := g.svc.ObserveExpedition(ctx, "third"); v.Map.Tiles[0][1] != "◎" {
		t.Fatalf("boss mark %v", v.Map.Tiles)
	}
	exp = g.reload(t, "third")
	exp.ConsumedEvents = []string{"B:1:1"}
	if err := g.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	if v, _ := g.svc.ObserveExpedition(ctx, "third"); v.Map.Tiles[0][1] != "□" {
		t.Fatalf("beaten boss mark %v", v.Map.Tiles)
	}
}

// The floor change and its limit survive reload; a stale replay cannot apply twice.
func TestNativeMap2_ValkeyFloorChange(t *testing.T) {
	ctx := context.Background()
	client := testutil.OpenValkey(t)
	charID := "map2-vk-" + time.Now().Format("150405.000000")
	defer cleanupDungeonValkey(t, client, charID)
	store, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	rng := &scriptedRNG{ints: []int{0, 1}}
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{charID: map2Hero(charID, 5000)}}
	svc, err := dungeon.NewService(newMockDungeonRepo(), chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store),
		dungeon.WithRNG(rng), dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartExpedition(ctx, charID, dungeon.NativeMap2ID); err != nil {
		t.Fatal(err)
	}
	exp, err := store.GetActiveExpedition(ctx, charID)
	if err != nil || exp == nil {
		t.Fatalf("reload %+v err=%v", exp, err)
	}
	exp.PosX, exp.PosY, exp.ConsumedEvents = 1, 0, []string{"7"}
	if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
		t.Fatal(err)
	}
	stale := exp.Revision
	if _, err := svc.Move(ctx, charID, dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetActiveExpedition(ctx, charID)
	if err != nil || got.MapVariant != 5 || got.RoundLimit != 40 || got.Round != 2 || got.PosX != 0 {
		t.Fatalf("reload %+v err=%v", got, err)
	}
	if _, err := store.Step(ctx, charID, dungeon.StepParams{ExpectedExpeditionID: got.ID, ExpectedRevision: &stale,
		RoundDelta: 1, MapVariant: 6, RoundLimit: 40, Now: time.Now().UTC()}); !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
		t.Fatalf("stale floor change err=%v", err)
	}
}

// A pitfall keeps the run's leader HP equal to the leader's member HP, solo and in a party.
func TestNativeMap2_PitfallKeepsLeaderHPInSync(t *testing.T) {
	run := func(t *testing.T, store dungeon.ActiveExpeditionStore, lead string, roster []string) {
		ctx := context.Background()
		chars := &mockCharRepo{chars: map[string]corecharacter.Character{}}
		for _, id := range roster {
			chars.chars[id] = map2Hero(id, 500)
		}
		rng := &scriptedRNG{ints: []int{0}}
		svc, err := dungeon.NewService(newMockDungeonRepo(), chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store),
			dungeon.WithRNG(rng), dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.StartPartyExpedition(ctx, lead, roster, dungeon.NativeMap2ID, "p-"+lead); err != nil {
			t.Fatal(err)
		}
		exp, err := store.GetActiveExpedition(ctx, lead)
		if err != nil || exp == nil {
			t.Fatalf("reload %+v err=%v", exp, err)
		}
		exp.MapVariant, exp.RoundLimit, exp.PosX, exp.PosY = 4, 40, 2, 2
		if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatal(err)
		}
		rng.ints, rng.floats = []int{0}, []float64{0, 0.9999}
		res, err := svc.Move(ctx, lead, dungeon.DirectionSouth)
		if err != nil {
			t.Fatal(err)
		}
		got, err := store.GetActiveExpedition(ctx, lead)
		if err != nil || got == nil {
			t.Fatalf("reload %+v err=%v", got, err)
		}
		for _, e := range []dungeon.ActiveExpedition{res.Expedition, *got} {
			if e.Members[0].CurrentHP != 473 || e.CurrentHP != 473 {
				t.Fatalf("leader member HP %d, run HP %d, want 473", e.Members[0].CurrentHP, e.CurrentHP)
			}
			if len(roster) > 1 && e.Members[1].CurrentHP != 465 {
				t.Fatalf("ally HP %d, want 465", e.Members[1].CurrentHP)
			}
		}
	}
	for _, roster := range [][]string{{"sync-solo"}, {"sync-lead", "sync-ally"}} {
		t.Run("memory/"+roster[0], func(t *testing.T) {
			run(t, dungeon.NewMemoryExpeditionRepository(), roster[0], roster)
		})
		t.Run("valkey/"+roster[0], func(t *testing.T) {
			client := testutil.OpenValkey(t)
			suffix := time.Now().Format("150405.000000")
			ids := make([]string, len(roster))
			for i, id := range roster {
				ids[i] = id + "-" + suffix
				defer cleanupDungeonValkey(t, client, ids[i])
			}
			store, err := dungeon.NewValkeyExpeditionRepository(client)
			if err != nil {
				t.Fatal(err)
			}
			run(t, store, ids[0], ids)
		})
	}
}
