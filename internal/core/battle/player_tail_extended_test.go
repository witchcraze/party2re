package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestPlayerTailResetDifferenceAndCapturedRestoration(t *testing.T) {
	for _, attack := range []int{0, 100, 200} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.ItemDefinitionIDs, a.Gear.AccessoryID = []string{"item-194"}, "item-194"
		a.Attack, a.Tension, a.Status, a.Temporary, a.Hit = attack, 3, StatusSleep, TemporaryDouble, 40
		a.Extended, a.ExtendedTurns = ExtendedCrown, 3
		draws := []float64{}
		if attack < 100 {
			draws = []float64{0, 0}
		} else if attack > 100 {
			draws = []float64{0}
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyPlayerTail(&c, "item-194-combat-tail", rng); err != nil {
			t.Fatal(err)
		}
		if a.Attack != 100 || a.Defense != 100 || a.Agility != 100 || a.Tension != 3 || a.Status != StatusSleep || a.Temporary != TemporaryDouble || a.Hit != 40 || a.Extended != "" || a.ExtendedTurns != 0 {
			t.Fatal(a)
		}
		wantHP, wantEnemy := 100, 100
		if attack > 100 {
			wantHP = 145
		} else if attack < 100 {
			wantEnemy = 55
		}
		if a.HP != wantHP || s.actor("enemy").HP != wantEnemy || (c.HealCalls == 1) != (attack > 100) || (c.DamageCalls == 1) != (attack < 100) {
			t.Fatal(s, c)
		}
		rng.done()
	}
}

func TestPlayerTailResetDescendantAdmissionAndNoEffect(t *testing.T) {
	for _, blocked := range []bool{false, true} {
		s, c := playerTailFixture()
		s.Actors = []LifetimeActor{*s.actor("self"), *s.actor("enemy")}
		c.PartyIDs, c.EnemyIDs = []string{"self"}, []string{"enemy"}
		a, enemy := s.actor("self"), s.actor("enemy")
		a.ItemDefinitionIDs, a.Attack, a.Gear.AccessoryID = []string{"item-194"}, 0, "item-194"
		enemy.HP, enemy.Gear.AccessoryID = 1, "item-110"
		if blocked {
			enemy.Temporary = TemporaryOneHitDefense
		}
		before, cursor := s.callbackCopy(), c.DefeatContext.Cursor
		rng := &targetRNG{t: t}
		err := s.ValidatePlayerTail(&c, "item-194-combat-tail", rng)
		if (err == nil) != blocked || !reflect.DeepEqual(before, s) || c.DefeatContext.Cursor != cursor {
			t.Fatal(err, s)
		}
		if !blocked && !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		rng.done()
		if blocked {
			rng = &targetRNG{t: t, draws: []float64{0}}
			if err := s.ApplyPlayerTail(&c, "item-194-combat-tail", rng); err != nil || enemy.HP != 1 || enemy.Temporary != "" || c.DamageCalls != 1 {
				t.Fatal(err, s, c)
			}
			rng.done()
		}
	}
}

func TestPlayerTailDamageDefeatRetainsNewResetCursor(t *testing.T) {
	for _, id := range []string{"item-194-combat-tail", "frenzy-tail"} {
		s, c := playerTailFixture()
		a, enemy := s.actor("self"), s.actor("enemy")
		a.ItemDefinitionIDs, a.Attack, a.Extended = []string{"item-194"}, 0, ExtendedFrenzy
		a.Gear.AccessoryID = "item-194"
		enemy.HP, enemy.Gear.AccessoryID, enemy.ItemDefinitionIDs = 1, "item-159", []string{"item-159"}
		c.Cursor = TargetCursor{Source: "item150", ActorID: "ally"}
		selection := 0.0
		if id == "frenzy-tail" {
			selection = .6
			a.Attack = 100
		}
		rng := &targetRNG{t: t, draws: []float64{selection, 0, 0, .9, .9}}
		if err := s.ApplyPlayerTail(&c, id, rng); err != nil || enemy.HP != 0 || c.Cursor != (TargetCursor{Source: "item159", ActorID: "enemy"}) || c.Cursor != c.DefeatContext.Cursor {
			t.Fatal(id, err, s, c)
		}
		rng.done()
	}
}

func TestPlayerTailFrenzyBodyFallenExpiryAndAllMemberPool(t *testing.T) {
	for _, target := range []struct {
		draw float64
		id   string
	}{{0, "self"}, {.2, "fallen"}, {.4, "ally"}, {.6, "enemy"}, {.8, "dead_enemy"}} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.Extended, a.ExtendedTurns = ExtendedFrenzy, 2
		rng := &targetRNG{t: t, draws: []float64{target.draw, 0}}
		if err := s.ApplyPlayerTail(&c, "frenzy-tail", rng); err != nil || a.Attack != 160 || s.actor(target.id).HP != 55 || c.DamageCalls != 0 {
			t.Fatal(err, s, c)
		}
		if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil || a.ExtendedTurns != 1 {
			t.Fatal(err, a)
		}
		rng.done()
	}
	for _, living := range []bool{false, true} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.Extended, a.ExtendedTurns, a.Attack, a.BaseStats.Attack = ExtendedFrenzy, 1, 4900, 1000
		for i := range s.Actors {
			s.Actors[i].HP, s.Actors[i].MaxHP = 10000, 10000
		}
		draws := []float64{.6, 0}
		if !living {
			a.HP, draws = 0, nil
		}
		rng := &targetRNG{t: t, draws: draws}
		if err := s.ApplyPlayerTail(&c, "frenzy-tail", rng); err != nil {
			t.Fatal(err)
		}
		if living && a.Attack != 5000 {
			t.Fatal(a)
		}
		if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil || a.Extended != "" || a.Attack != 999 {
			t.Fatal(err, a)
		}
		rng.done()
	}
}

func TestPlayerTailFrenzyLethalSelfRevivalAndLateUnsupported(t *testing.T) {
	for _, revival := range []bool{false, true} {
		s, c := playerTailFixture()
		for i := range s.Actors {
			s.Actors[i].HP, s.Actors[i].MaxHP = 1000, 1000
		}
		a := s.actor("self")
		a.HP, a.Extended, a.ExtendedTurns = 1, ExtendedFrenzy, 2
		if revival {
			a.ItemDefinitionIDs, a.Gear.AccessoryID = []string{"item-193"}, "item-193"
		}
		rng := &targetRNG{t: t, draws: []float64{0, 0}}
		if err := s.ApplyPlayerTail(&c, "frenzy-tail", rng); err != nil {
			t.Fatal(err)
		}
		want := 0
		if revival {
			want = 500
		}
		if a.HP != want || a.Extended != "" || a.Attack != 100 {
			t.Fatal(a)
		}
		rng.done()
	}
	s, c := playerTailFixture()
	a := s.actor("self")
	a.Extended = ExtendedFrenzy
	// A later member is unsupported even if the actual first draw would be self.
	s.actor("dead_enemy").HP, s.actor("dead_enemy").Gear.AccessoryID = 0, "item-110"
	before := s.callbackCopy()
	if err := s.ApplyPlayerTail(&c, "frenzy-tail", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
}

func TestPlayerTailCrownCountProbabilityAndFallenDrawOrder(t *testing.T) {
	for _, tc := range []struct {
		count, hp, want int
		draws           []float64
	}{
		{0, 100, 100, nil}, {1, 100, 172, []float64{0, 0, 0}},
		{1, 100, 100, []float64{.2}}, {4, 100, 255, []float64{.49, .999, .999}},
		{1, 0, 0, []float64{0, 0}},
	} {
		s, c := playerTailFixture()
		a := s.actor("self")
		a.HP, a.Extended, a.ExtendedTurns, c.DamageCalls = tc.hp, ExtendedCrown, 1, tc.count
		rng := &targetRNG{t: t, draws: tc.draws}
		if err := s.ApplyPlayerTail(&c, "crown-tail", rng); err != nil || a.HP != tc.want {
			t.Fatal(err, a)
		}
		c.Completion.Tail.DamageCalls = c.DamageCalls
		if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil || (a.Extended == "") != (tc.count > 0) {
			t.Fatal(err, a)
		}
		rng.done()
	}
}

func TestPlayerTailReloadFrozenInputAndReceiptRetryContract(t *testing.T) {
	s, c := playerTailFixture()
	a := s.actor("self")
	a.ItemDefinitionIDs, a.Extended, a.ExtendedTurns = []string{"item-210", "weapon-30", "item-238"}, ExtendedCrown, 2
	a.CMPCharge = a.CMPNeedCharge
	s.actor("ally").HP = 0
	var err error
	c.Item210Fallen, err = s.CaptureItem210(c.TargetContext)
	if err != nil {
		t.Fatal(err)
	}
	s.actor("ally").HP = 1
	c.DamageCalls = 1
	// Freeze only input/facts. Rebuild contexts/ports, never serialize closures.
	frozen, err := json.Marshal(struct {
		Fallen     []string
		Completion LifetimeCompletion
		Level      int
	}{c.Item210Fallen, c.Completion, *c.RequesterLevel})
	if err != nil {
		t.Fatal(err)
	}
	loaded := reloadLifetime(t, s)
	_, rebuilt := playerTailFixture()
	var input struct {
		Fallen     []string
		Completion LifetimeCompletion
		Level      int
	}
	if err := json.Unmarshal(frozen, &input); err != nil {
		t.Fatal(err)
	}
	rebuilt.Item210Fallen, rebuilt.Completion, rebuilt.RequesterLevel, rebuilt.DamageCalls = input.Fallen, input.Completion, &input.Level, 1
	for _, run := range []struct {
		state   *LifetimeState
		context *PlayerTailContext
	}{{&s, &c}, {&loaded, &rebuilt}} {
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0, .9, 0, 0, 0, 0}}
		for _, id := range []string{"item-210-revival-tail", "weapon-30-tail", "cmp-recharge-tail", "crown-tail"} {
			if err := run.state.ApplyPlayerTail(run.context, id, rng); err != nil {
				t.Fatal(err)
			}
		}
		run.context.Completion.Tail.DamageCalls, run.context.Completion.End = run.context.DamageCalls, run.context.End
		if _, err := run.state.CompleteOperation(run.context.Completion, run.context.PartyIDs); err != nil {
			t.Fatal(err)
		}
		rng.done()
	}
	if !reflect.DeepEqual(s, loaded) || s.actor("ally").HP != 271 || s.actor("self").HP != 190 || s.actor("self").CMP != 21 || s.actor("self").CMPNeedCharge != 3 || s.actor("self").ExtendedTurns != 1 {
		t.Fatal(s, loaded)
	}
	// A committed receipt returns the saved result; this unit deliberately has
	// no retry executor/storage. Even accepted=false is inert when rechecked.
	receipt := reloadLifetime(t, s)
	c.Completion.Accepted = false
	for _, id := range PlayerTailEntries() {
		if err := s.ApplyPlayerTail(&c, id, &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil || !reflect.DeepEqual(receipt, s) {
		t.Fatal(err, s)
	}
}
