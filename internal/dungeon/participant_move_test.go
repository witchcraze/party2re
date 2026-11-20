package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// startMap1Party starts lead+ally on map1 variant 1 beside its locked door (4,0).
func startMap1Party(t *testing.T, allyJob string) *nativeFixture {
	t.Helper()
	f := newNativeFixture(t, map1Hero("lead", "job-01"))
	f.chars.chars["ally"] = map1Hero("ally", allyJob)
	f.startNativeParty(t, dungeon.NativeMap1ID, 1, 4, 0)
	return f
}

// Legacy registers the directions for every member (vs_dungeon.cgi:37–44) and
// the map event acts through the mover: the ally's job 9 opens the door.
func TestMove_LivingMemberMovesAsTheActor(t *testing.T) {
	f := startMap1Party(t, "job-09")
	if c := f.reload(t, "ally").Controls("ally"); !c.CanMove || c.CanEscape || c.CanFight {
		t.Fatalf("member controls %+v", c)
	}
	if _, err := f.svc.Move(context.Background(), "ally", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "lead"); exp.PosX != 3 || exp.Round != 2 || !slices.Equal(exp.ConsumedEvents, []string{"D"}) {
		t.Fatalf("member move %+v", exp)
	}
}

// A fallen character can only Escape (_battle.cgi:654–663); living members continue.
func TestMove_FallenActorIsRejected(t *testing.T) {
	ctx := context.Background()
	f := startMap1Party(t, "job-01")
	exp := f.reload(t, "lead")
	exp.Members[0].CurrentHP, exp.CurrentHP = 0, 0
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "lead")
	if c := before.Controls("lead"); c.CanMove || !c.CanEscape {
		t.Fatalf("fallen leader controls %+v", c)
	}
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrActorFallen) {
		t.Fatalf("fallen leader err=%v", err)
	}
	if after := f.reload(t, "lead"); after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatalf("rejected move changed %+v rng=%d", after, f.rng.calls())
	}
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionWest); err != nil {
		t.Fatalf("living member err=%v", err)
	}
	if _, err := f.svc.Move(ctx, "stranger", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatalf("non-member err=%v", err)
	}
}

// Concurrent moves by two members never apply one revision twice.
func TestMove_ConcurrentMembersApplyOneStepPerRevision(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		f := startMap1Party(t, "job-01")
		raceDoorMoves(t, f.svc, f.store, "lead", "ally")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		suffix := time.Now().Format("150405.000000")
		lead, ally := "pm-lead-"+suffix, "pm-ally-"+suffix
		t.Cleanup(func() {
			cleanupDungeonValkey(t, client, lead)
			client.Do(context.Background(), client.B().Del().Key(fmt.Sprintf("party2:dungeon:{char:%s}:member", ally)).Build())
		})
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		chars := &mockCharRepo{chars: map[string]corecharacter.Character{lead: map1Hero(lead, "job-01"), ally: map1Hero(ally, "job-01")}}
		svc, err := dungeon.NewService(newMockDungeonRepo(), chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store),
			dungeon.WithRNG(&scriptedRNG{ints: []int{0}}), dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := svc.StartPartyExpedition(ctx, lead, []string{lead, ally}, dungeon.NativeMap1ID, "p-"+suffix); err != nil {
			t.Fatal(err)
		}
		exp, err := store.GetActiveExpedition(ctx, lead)
		if err != nil || exp == nil {
			t.Fatalf("reload %v %v", exp, err)
		}
		exp.PosX, exp.PosY = 4, 0
		if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatal(err)
		}
		raceDoorMoves(t, svc, store, lead, ally)
	})
}

// raceDoorMoves races both members into variant 1's locked door, which spends a
// round without drawing RNG; the round must equal the accepted moves.
func raceDoorMoves(t *testing.T, svc *dungeon.Service, store dungeon.ActiveExpeditionStore, lead, ally string) {
	t.Helper()
	ctx := context.Background()
	before, err := store.GetActiveExpedition(ctx, lead)
	if err != nil || before == nil {
		t.Fatalf("before %v %v", before, err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for range 20 {
		for _, id := range []string{lead, ally} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := svc.Move(ctx, id, dungeon.DirectionWest)
				if err != nil && !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
					t.Errorf("%s: %v", id, err)
					return
				}
				if err == nil {
					mu.Lock()
					applied++
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	after, err := store.GetActiveExpedition(ctx, lead)
	if err != nil || after == nil || applied == 0 || after.Round != before.Round+applied || after.Revision != before.Revision+applied {
		t.Fatalf("after %+v err=%v, want %d accepted moves from round %d", after, err, applied, before.Round)
	}
}

func newItem188PartyFixture(
	t *testing.T,
	store dungeon.ActiveExpeditionStore,
	chars map[string]corecharacter.Character,
	holders []string,
) *nativeFixture {
	t.Helper()
	invRepo := &mockInventoryRepo{invs: make(map[string]coreinventory.Inventory, len(holders))}
	for _, id := range holders {
		inv, err := coreinventory.New(id)
		if err != nil {
			t.Fatal(err)
		}
		if err := inv.Add(coreitem.Instance{ID: "acc-188-" + id, DefinitionID: "item-188", Quantity: 1}); err != nil {
			t.Fatal(err)
		}
		invRepo.invs[id] = inv
	}
	rng := &scriptedRNG{}
	svc, err := dungeon.NewService(newMockDungeonRepo(), &mockCharRepo{chars: chars}, corebattle.Engine{},
		dungeon.WithActiveExpeditionStore(store), dungeon.WithRNG(rng),
		dungeon.WithInventoryProvider(invRepo), dungeon.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatal(err)
	}
	return &nativeFixture{
		svc:   svc,
		store: store,
		chars: &mockCharRepo{chars: chars},
		rng:   rng,
	}
}

func startItem188PartyAt(
	t *testing.T,
	f *nativeFixture,
	mapID string,
	variant, x, y, round int,
	memberIDs []string,
	fallenIDs []string,
) dungeon.ActiveExpedition {
	t.Helper()
	ctx := context.Background()
	f.rng.ints = []int{variant - 1}
	if _, err := f.svc.StartPartyExpedition(ctx, memberIDs[0], memberIDs, mapID, "p-188"); err != nil {
		t.Fatal(err)
	}
	exp := f.reload(t, memberIDs[0])
	exp.PosX, exp.PosY, exp.Round = x, y, round
	for i := range exp.Members {
		if slices.Contains(fallenIDs, exp.Members[i].CharacterID) {
			exp.Members[i].CurrentHP = 0
			if exp.Members[i].CharacterID == exp.CharacterID {
				exp.CurrentHP = 0
			}
		}
	}
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	f.rng.ints, f.rng.floats, f.rng.intCalls, f.rng.floatCalls = nil, nil, 0, 0
	return f.reload(t, memberIDs[0])
}

func TestMove_Item188RevivesFallenMembersOnMove(t *testing.T) {
	run := func(t *testing.T, store dungeon.ActiveExpeditionStore, suffix string) {
		t.Helper()
		ctx := context.Background()
		lead := "rev-lead-" + suffix
		mover := "rev-mover-" + suffix
		holder2 := "rev-h2-" + suffix
		stayed := "rev-stayed-" + suffix
		chars := map[string]corecharacter.Character{
			lead:    map4Hero(lead, 101),
			mover:   map4Hero(mover, 500),
			holder2: map4Hero(holder2, 100),
			stayed:  map4Hero(stayed, 100),
		}
		f := newItem188PartyFixture(t, store, chars, []string{mover, holder2})
		before := startItem188PartyAt(t, f, dungeon.NativeMap4ID, 1, 1, 0, 2, []string{lead, mover, holder2, stayed}, []string{lead, stayed})

		// First living holder triggers one draw per fallen member; second holder does not repeat.
		f.rng.floats = []float64{0.49, 0.5}
		res, err := f.svc.Move(ctx, mover, dungeon.DirectionEast)
		if err != nil {
			t.Fatal(err)
		}
		if f.rng.calls() != 2 {
			t.Fatalf("rng calls=%d, want 2 (single holder pass)", f.rng.calls())
		}
		// map4 passage attrition charges only the mover: int(500 * 0.02 + 0.5) = 10.
		if len(res.MemberDamages) != 1 || res.MemberDamages[mover] != 10 {
			t.Fatalf("damages=%v, want only %s:10", res.MemberDamages, mover)
		}

		exp := f.reload(t, lead)
		if exp.Revision != before.Revision+1 || exp.Round != before.Round+1 || exp.PosX != 2 || exp.PosY != 0 {
			t.Fatalf("step %+v from %+v", exp, before)
		}
		if got := memberOf(t, exp, lead).CurrentHP; got != 50 || exp.CurrentHP != 50 {
			t.Fatalf("revived leader HP=%d run=%d, want 50", got, exp.CurrentHP)
		}
		if got := memberOf(t, exp, stayed).CurrentHP; got != 0 {
			t.Fatalf("unrevived member HP=%d, want 0", got)
		}
		if got := memberOf(t, exp, mover).CurrentHP; got != 490 {
			t.Fatalf("mover HP=%d, want 490", got)
		}
	}

	t.Run("memory", func(t *testing.T) {
		run(t, dungeon.NewMemoryExpeditionRepository(), "mem")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		suffix := time.Now().Format("150405.000000")
		ids := []string{"rev-lead-" + suffix, "rev-mover-" + suffix, "rev-h2-" + suffix, "rev-stayed-" + suffix}
		t.Cleanup(func() {
			cleanupDungeonValkey(t, client, ids[0])
			for _, id := range ids[1:] {
				client.Do(context.Background(), client.B().Del().Key(fmt.Sprintf("party2:dungeon:{char:%s}:member", id)).Build())
			}
		})
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		run(t, store, suffix)
	})
}

func TestMove_Item188SkipsWhenNoHolderOrHolderFallen(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		holders []string
	}{
		{name: "no holder", holders: nil},
		{name: "holder fallen", holders: []string{"lead"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			chars := map[string]corecharacter.Character{
				"lead": map4Hero("lead", 100),
				"ally": map4Hero("ally", 100),
			}
			f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), chars, tc.holders)
			startItem188PartyAt(t, f, dungeon.NativeMap4ID, 1, 0, 0, 2, []string{"lead", "ally"}, []string{"lead"})

			f.rng.floats = []float64{0}
			if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionEast); err != nil {
				t.Fatal(err)
			}
			exp := f.reload(t, "lead")
			if memberOf(t, exp, "lead").CurrentHP != 0 || exp.CurrentHP != 0 || f.rng.calls() != 0 {
				t.Fatalf("unexpected revival or RNG draw: members=%+v rng=%d", exp.Members, f.rng.calls())
			}
		})
	}
}

func TestMove_WallHitsAndLimitRejectionDoNotRevive(t *testing.T) {
	ctx := context.Background()
	t.Run("impassable wall", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"lead":   strongHero("lead"),
			"holder": strongHero("holder"),
		}
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), chars, []string{"holder"})
		before := startItem188PartyAt(t, f, dungeon.NativeMap0ID, 1, 3, 0, 2, []string{"lead", "holder"}, []string{"lead"})
		f.rng.floats = []float64{0}
		if _, err := f.svc.Move(ctx, "holder", dungeon.DirectionSouth); !errors.Is(err, dungeon.ErrImpassableWall) {
			t.Fatalf("wall err=%v", err)
		}
		after := f.reload(t, "lead")
		if after.Revision != before.Revision || memberOf(t, after, "lead").CurrentHP != 0 || after.CurrentHP != 0 || f.rng.calls() != 0 {
			t.Fatalf("wall changed state: %+v rng=%d", after, f.rng.calls())
		}
	})

	t.Run("map8 scatter wall", func(t *testing.T) {
		cLead := createTestChar("lead", 30, 100, 900, 900)
		cLead.JobLevel = 17
		cHolder := createTestChar("holder", 30, 100, 900, 900)
		cHolder.JobLevel = 17
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), map[string]corecharacter.Character{"lead": cLead, "holder": cHolder}, []string{"holder"})
		startItem188PartyAt(t, f, dungeon.NativeMap8ID, 1, 2, 0, 2, []string{"lead", "holder"}, []string{"lead"})
		f.rng.ints, f.rng.floats = []int{1, 2}, []float64{0}
		if _, err := f.svc.Move(ctx, "holder", dungeon.DirectionNorth); err != nil {
			t.Fatal(err)
		}
		after := f.reload(t, "lead")
		if after.PosX != 5 || after.PosY != 4 || memberOf(t, after, "lead").CurrentHP != 0 || after.CurrentHP != 0 || f.rng.floatCalls != 0 || f.rng.intCalls != 2 {
			t.Fatalf("scatter wall revived or drew float RNG: %+v int=%d float=%d", after, f.rng.intCalls, f.rng.floatCalls)
		}
	})

	t.Run("round limit", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"lead":   strongHero("lead"),
			"holder": strongHero("holder"),
		}
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), chars, []string{"holder"})
		before := startItem188PartyAt(t, f, dungeon.NativeMap0ID, 1, 2, 4, 20, []string{"lead", "holder"}, []string{"lead"})
		f.rng.floats = []float64{0}
		if _, err := f.svc.Move(ctx, "holder", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrMovementExhausted) {
			t.Fatalf("limit err=%v", err)
		}
		after := f.reload(t, "lead")
		if after.Revision != before.Revision || memberOf(t, after, "lead").CurrentHP != 0 || after.CurrentHP != 0 || f.rng.calls() != 0 {
			t.Fatalf("limit changed state: %+v rng=%d", after, f.rng.calls())
		}
	})
}

func TestMove_Item188RevivalPrecedesDestinationEvent(t *testing.T) {
	ctx := context.Background()
	t.Run("treasure counts revived member", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"lead":   createTestChar("lead", 30, 100, 900, 900),
			"holder": createTestChar("holder", 30, 100, 900, 900),
		}
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), chars, []string{"holder"})
		before := startItem188PartyAt(t, f, dungeon.NativeMap0ID, 1, 3, 0, 2, []string{"lead", "holder"}, []string{"lead"})

		f.rng.floats = []float64{0}
		res, err := f.svc.Move(ctx, "holder", dungeon.DirectionEast)
		if err != nil {
			t.Fatal(err)
		}
		if res.EventType != dungeon.EventTreasure || res.Expedition.Encounter == nil || len(res.Expedition.Encounter.Chests) != 2 {
			t.Fatalf("treasure %+v, want 2 chests counting revived member", res)
		}
		exp := f.reload(t, "lead")
		if exp.Revision != before.Revision+1 || memberOf(t, exp, "lead").CurrentHP != 50 || exp.CurrentHP != 50 || len(exp.Encounter.Chests) != 2 {
			t.Fatalf("reloaded treasure %+v", exp)
		}
	})

	t.Run("combat includes revived member taking zero damage in same step", func(t *testing.T) {
		chars := map[string]corecharacter.Character{
			"lead":   createTestChar("lead", 30, 101, 900, 900),
			"holder": createTestChar("holder", 30, 500, 900, 900),
		}
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), chars, []string{"holder"})
		before := startItem188PartyAt(t, f, dungeon.NativeMap0ID, 1, 2, 2, 2, []string{"lead", "holder"}, []string{"lead"})

		f.rng.floats = []float64{0}
		res, err := f.svc.Move(ctx, "holder", dungeon.DirectionNorth)
		if err != nil {
			t.Fatal(err)
		}
		if res.EventType != dungeon.EventBoss || res.CombatOutcome != corebattle.OutcomeWin || res.BattleResult == nil {
			t.Fatalf("boss %+v", res)
		}
		boss := mapDef(t, dungeon.NativeMap0ID).Variants[0].Boss
		wantLeadHP := res.BattleResult.RemainingHP["lead"]
		if wantLeadHP != 50 {
			t.Fatalf("revived leader combat HP=%d, want 50", wantLeadHP)
		}
		exp := f.reload(t, "lead")
		leadMem := memberOf(t, exp, "lead")
		if exp.Revision != before.Revision+1 || leadMem.CurrentHP != wantLeadHP || exp.CurrentHP != wantLeadHP ||
			leadMem.AccumulatedExp != boss.ExpReward || leadMem.AccumulatedGold != boss.GoldReward {
			t.Fatalf("reloaded boss win %+v (revision before=%d)", exp, before.Revision)
		}
	})

	t.Run("combat persists damage to revived member in same step", func(t *testing.T) {
		lead := createTestChar("lead", 30, 100, 1, 1)
		lead.Stats.Agility = 1
		holder := createTestChar("holder", 30, 500, 150, 900)
		holder.Stats.Agility = 1
		f := newItem188PartyFixture(t, dungeon.NewMemoryExpeditionRepository(), map[string]corecharacter.Character{"lead": lead, "holder": holder}, []string{"holder"})
		before := startItem188PartyAt(t, f, dungeon.NativeMap0ID, 1, 2, 2, 2, []string{"lead", "holder"}, []string{"lead"})

		f.rng.floats = []float64{0}
		res, err := f.svc.Move(ctx, "holder", dungeon.DirectionNorth)
		if err != nil {
			t.Fatal(err)
		}
		if res.EventType != dungeon.EventBoss || res.BattleResult == nil {
			t.Fatalf("boss %+v", res)
		}
		wantLeadHP := res.BattleResult.RemainingHP["lead"]
		if wantLeadHP >= 50 {
			t.Fatalf("expected revived leader to take boss damage from 50 HP, got %d", wantLeadHP)
		}
		exp := f.reload(t, "lead")
		if exp.Revision != before.Revision+1 || memberOf(t, exp, "lead").CurrentHP != wantLeadHP || exp.CurrentHP != wantLeadHP {
			t.Fatalf("reloaded damaged combat %+v, want lead HP %d", exp, wantLeadHP)
		}
	})
}
