package battle

import (
	"errors"
	"reflect"
	"testing"
)

// Every random operation uses a source-style continuous range, including
// selection from a one-member roster. Unexpected or missing draws fail tests.
type targetRNG struct {
	t     *testing.T
	draws []float64
	used  int
}

func (r *targetRNG) Float64() float64 {
	r.t.Helper()
	if r.used >= len(r.draws) {
		r.t.Fatal("unexpected target RNG draw")
	}
	v := r.draws[r.used]
	r.used++
	return v
}
func (r *targetRNG) Intn(n int) int { r.t.Fatal("target helpers must use continuous draws"); return 0 }
func (r *targetRNG) IntN(n int) int { return r.Intn(n) }
func (r *targetRNG) done() {
	r.t.Helper()
	if r.used != len(r.draws) {
		r.t.Fatalf("draws: used %d, want %d", r.used, len(r.draws))
	}
}

func targetFixture() (LifetimeState, TargetContext) {
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "self", TeamID: "party", HP: 100, MP: 3, MaxMP: 100}, Tension: 1},
		{Participant: Participant{ID: "fallen", TeamID: "party", HP: 0}},
		{Participant: Participant{ID: "ally", TeamID: "party", HP: 100}},
		{Participant: Participant{ID: "enemy", TeamID: "enemy", HP: 100, MP: 90, MaxMP: 100}},
		{Participant: Participant{ID: "dead_enemy", TeamID: "enemy", HP: 0}},
	}}
	c := TargetContext{ActorID: "self", PartyIDs: []string{"self", "fallen", "ally"}, EnemyIDs: []string{"enemy", "dead_enemy"}}
	return s, c
}

func TestEnemyConfusionTwoPassBoundaries(t *testing.T) {
	for _, tc := range []struct {
		name, target, want string
		boss               bool
		draws              []float64
	}{
		{"explicit self", "self", "self", false, nil},
		{"first self", "enemy", "self", false, []float64{0}},
		{"first other second fallen", "enemy", "fallen", false, []float64{0.6, 0.2}},
		{"first fallen second enemy", "enemy", "enemy", false, []float64{0.2, 0.6}},
		{"missing uses both", "missing", "dead_enemy", false, []float64{0.4, 0.8}},
		{"boss self below one", "self", "fallen", true, []float64{0.3, 0.2}},
		{"boss self exactly one", "self", "ally", true, []float64{1.0 / 3, 0.4}},
		{"boss self above one", "self", "self", true, []float64{0.34}},
		{"boss first self then gate", "enemy", "self", true, []float64{0, 0.9}},
		{"boss nonself skips gate", "enemy", "fallen", true, []float64{0.4, 0.2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := targetFixture()
			s.SpecialBossField, s.Actors[0].Status = tc.boss, StatusConfusion
			s = reloadLifetime(t, s)
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: tc.target, Category: CategoryPhysical, Payload: CallbackPayload{Number: 100}}, rng)
			if err != nil || !r.Resolved || r.TargetID != tc.want || r.Payload.Number != 100 {
				t.Fatal(r, err)
			}
			rng.done()
			reloadLifetime(t, s)
		})
	}
}

func TestPartyFallbackAndRevival(t *testing.T) {
	for _, tc := range []struct {
		name, target, symbol, status, want string
		boss                               bool
		draws                              []float64
	}{
		{"existing", "ally", "", "", "ally", false, nil},
		{"missing random fallen redirects self", "missing", "", "", "self", false, []float64{0.4}},
		{"enemy random ally", "enemy", "", "", "ally", false, []float64{0.9}},
		{"fallen", "fallen", "", "", "self", false, nil},
		{"revival", "fallen", "revival", "", "fallen", false, nil},
		{"revival fallback", "enemy", "revival", "", "fallen", false, []float64{0.4}},
		{"confusion includes fallen enemy", "self", "", StatusConfusion, "dead_enemy", false, []float64{0.9}},
		{"boss confusion replaces below one", "self", "", StatusConfusion, "fallen", true, []float64{0.3, 0.2}},
		{"boss confusion retains at one", "enemy", "", StatusConfusion, "enemy", true, []float64{1.0 / 3}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := targetFixture()
			s.SpecialBossField, s.Actors[0].Status = tc.boss, tc.status
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: tc.target, Category: CategoryMagic, Payload: CallbackPayload{Symbol: tc.symbol}}, rng)
			if err != nil || !r.Resolved || r.TargetID != tc.want {
				t.Fatal(r, err)
			}
			rng.done()
		})
	}
}

func TestCategorySealAndActivationOrder(t *testing.T) {
	for _, category := range []CallbackCategory{CategoryPhysical, CategoryMagic, CategoryBreath, CategoryDance, CategorySong, CategoryUntyped} {
		for _, status := range []string{StatusPhysicalSeal, StatusMagicSeal, StatusDanceSeal, "song_seal"} {
			matching := (category == CategoryPhysical && status == StatusPhysicalSeal) || (category == CategoryMagic && status == StatusMagicSeal) || (category == CategoryDance && status == StatusDanceSeal)
			for _, draw := range []float64{0.2499, 0.25} {
				a := LifetimeActor{Participant: Participant{Status: status}}
				rng := &targetRNG{t: t}
				if matching {
					rng.draws = []float64{draw}
				}
				allowed, err := a.CheckCategorySeal(category, rng)
				if err != nil || allowed != (!matching || draw < 0.25) || (a.Status == "") != (matching && draw < 0.25) {
					t.Fatal(category, status, draw, a, allowed, err)
				}
				rng.done()
			}
		}
	}
	// Enemy activation precedes the empty-enemy return; party category rejection
	// precedes activation. A cure allows a fresh activation draw in that helper.
	for _, enemy := range []bool{false, true} {
		s, c := targetFixture()
		c.Item195Activated = true
		s.Actors[0].Status = StatusMagicSeal
		rng := &targetRNG{t: t, draws: []float64{0.25}}
		e := TargetEffect{TargetID: "enemy", Category: CategoryMagic}
		if enemy {
			if r, err := s.ResolveEnemyTarget(c, e, rng); err != nil || r.Resolved {
				t.Fatal(r, err)
			}
		} else if r, err := s.ResolvePartyTarget(c, e, rng); err != nil || r.Resolved {
			t.Fatal(r, err)
		}
		rng.done()
	}
	s, c := targetFixture()
	s.Actors = s.Actors[:3]
	c.EnemyIDs, c.Item195Activated = nil, true
	rng := &targetRNG{t: t, draws: []float64{0.1999}}
	r, err := s.ResolveEnemyTarget(c, TargetEffect{Category: CategoryPhysical}, rng)
	if err != nil || r.Resolved || s.Actors[0].Status != StatusConfusion {
		t.Fatal(r, err, s)
	}
	rng.done()
	s, c = targetFixture()
	s.Actors[0].Status, c.Item195Activated = StatusMagicSeal, true
	rng = &targetRNG{t: t, draws: []float64{0, 0.32, 0.2}}
	r, err = s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryMagic}, rng)
	if err != nil || r.TargetID != "fallen" || s.Actors[0].Status != StatusConfusion {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestEnemyCategoryConversion(t *testing.T) {
	for _, tc := range []struct {
		name     string
		item166  bool
		tension  float64
		category CallbackCategory
		sword    bool
		command  string
		custom   bool
		seal     bool
		want     float64
		wantCat  CallbackCategory
		debit    int
		direct   bool
	}{
		{"low tension", true, 2.99, CategoryPhysical, false, "attack", false, true, 80, CategoryUntyped, 0, false},
		{"exact tension", true, 3, CategoryPhysical, false, "attack", false, true, 101, CategoryUntyped, 0, false},
		{"high tension", true, 3.01, CategoryPhysical, false, "attack", false, true, 121, CategoryUntyped, 0, false},
		{"item blocks magic", true, 1, CategoryMagic, false, "", false, true, 0, CategoryMagic, 0, false},
		{"sword after item", true, 1, CategoryPhysical, true, "attack", false, true, 96, CategoryUntyped, 0, true},
		{"sword before seal", false, 1, CategoryPhysical, true, "attack", false, true, 121, CategoryMagic, 0, true},
		{"sword other category", false, 1, CategoryBreath, true, "attack", false, true, 121, CategoryBreath, 0, true},
		{"sword custom excluded", false, 1, CategoryPhysical, true, "attack", true, true, 80, CategoryMagic, 3, false},
		{"sword wrong command", false, 1, CategoryPhysical, true, "other", false, true, 80, CategoryMagic, 3, false},
		{"seal only", false, 1, CategoryPhysical, false, "", false, true, 80, CategoryMagic, 3, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := targetFixture()
			a := &s.Actors[0]
			a.Tension = tc.tension
			if tc.item166 {
				a.ItemDefinitionIDs = []string{"item-166"}
			}
			if tc.sword {
				a.Extended = ExtendedMagicSword
			}
			if tc.seal {
				a.Abilities = []string{"seal_kotowari"}
			}
			c.Command, c.Custom = tc.command, tc.custom
			e, debit, err := a.ConvertEnemyCategory(c, TargetEffect{Category: tc.category, Payload: CallbackPayload{Number: 101}, Arguments: []string{"0", "tail"}})
			if err != nil || e.Payload.Number != tc.want || e.Category != tc.wantCat || debit != tc.debit {
				t.Fatal(e, debit, err)
			}
			wantArgs := []string{"0", "tail"}
			if tc.direct {
				wantArgs = []string{"1"}
			}
			if !reflect.DeepEqual(e.Arguments, wantArgs) {
				t.Fatal(e.Arguments)
			}
		})
	}
	for _, command := range []string{"attack", "flame_slash", "lightning_slash", "metal_slash", "wide_slash"} {
		if !magicSwordCommand(command) {
			t.Fatal(command)
		}
	}
	for _, mp := range []int{3, 2, 0} {
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.Actors[0].MP, s.Actors[0].Abilities = mp, []string{"seal_kotowari"}
		rng := &targetRNG{t: t}
		r, err := s.ResolveEnemyTarget(c, TargetEffect{Category: CategoryPhysical, Payload: CallbackPayload{Number: 100}}, rng)
		if err != nil || r.Resolved || r.MPDebit != 3 || r.Category != CategoryMagic || s.Actors[0].MP != mp {
			t.Fatal(r, err)
		}
		// Resource owner applies this operation before processing the no-effect
		// continuation; even insufficient MP floors at zero rather than rejecting.
		s.Actors[0].MP = max(0, s.Actors[0].MP-r.MPDebit)
		if s.Actors[0].MP != 0 {
			t.Fatal(s)
		}
		rng.done()
	}
}

func TestTargetValidationIsInert(t *testing.T) {
	for _, invalid := range []string{"actor", "duplicate", "missing roster member", "wrong side", "category", "payload", "rng"} {
		s, c := targetFixture()
		before := reloadLifetime(t, s)
		e := TargetEffect{TargetID: "enemy", Category: CategoryPhysical}
		rng := &targetRNG{t: t}
		switch invalid {
		case "actor":
			c.ActorID = "foreign"
		case "duplicate":
			c.PartyIDs = append(c.PartyIDs, "ally")
		case "missing roster member":
			c.EnemyIDs = c.EnemyIDs[:1]
		case "wrong side":
			c.EnemyIDs[0] = "ally"
		case "category":
			e.Category = "fire"
		case "payload":
			e.Payload = CallbackPayload{Symbol: "revival", Number: 1}
		}
		var err error
		if invalid == "rng" {
			_, err = s.ResolveEnemyTarget(c, e, nil)
		} else {
			_, err = s.ResolveEnemyTarget(c, e, rng)
		}
		if err == nil || !reflect.DeepEqual(before, s) {
			t.Fatal(invalid, err, s)
		}
		rng.done()
	}
	s, c := targetFixture()
	r, err := s.ResolveEnemyTarget(c, TargetEffect{TargetID: "dead_enemy", Category: CategoryPhysical}, &targetRNG{t: t})
	if err != nil || r.Resolved {
		t.Fatal(r, err)
	}
	_, err = s.ResolveTemporary(c, TargetEffect{TargetID: "foreign"}, &targetRNG{t: t})
	if !errors.Is(err, ErrInvalidParticipant) {
		t.Fatal(err)
	}
}

func TestReachableDeflectionEmptySide(t *testing.T) {
	// 1. Direct target with deflect and empty enemy side rejects.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryDeflect
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		_, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "ally", Category: CategoryPhysical}, rng)
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatalf("expected direct ally deflect to reject with ErrUnsupportedEffect, got err=%v", err)
		}
		rng.done()
	}

	// 2. Unrelated deflect on ally when targeting self succeeds.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryDeflect
		rng := &targetRNG{t: t}
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryPhysical}, rng)
		if err != nil || !r.Resolved || r.TargetID != "self" {
			t.Fatalf("expected target self with unrelated ally deflect to succeed, got err=%v, r=%v", err, r)
		}
		rng.done()
	}

	// 3. Fallback: target not in party reaches ally with deflect -> rejects.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryDeflect
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		_, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "missing", Category: CategoryPhysical}, rng)
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatalf("expected fallback reach to deflect to reject with ErrUnsupportedEffect, got err=%v", err)
		}
		rng.done()
	}

	// 4. Fallback: dead target falls back to self.
	// 4a. If self has deflect -> rejects.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("self").Temporary = TemporaryDeflect
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		_, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "fallen", Category: CategoryPhysical}, rng)
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatalf("expected dead fallback to self with deflect to reject, got err=%v", err)
		}
		rng.done()
	}
	// 4b. If dead target has deflect, but self does not -> redirects to self, deflect is NOT evaluated, succeeds.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("fallen").Temporary = TemporaryDeflect
		rng := &targetRNG{t: t}
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "fallen", Category: CategoryPhysical}, rng)
		if err != nil || !r.Resolved || r.TargetID != "self" {
			t.Fatalf("expected dead fallback to self (no deflect) to succeed, got err=%v, r=%v", err, r)
		}
		rng.done()
	}

	// 5. Confusion / activation reach:
	// 5a. Actor is confused, ally has deflect -> confusion can reach ally -> rejects.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("self").Status = StatusConfusion
		s.actor("ally").Temporary = TemporaryDeflect
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		_, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryPhysical}, rng)
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatalf("expected confusion reaching deflect to reject, got err=%v", err)
		}
		rng.done()
	}
	// 5b. Item195Activated is true, ally has deflect -> activation can reach ally -> rejects.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		c.Item195Activated = true
		s.actor("ally").Temporary = TemporaryDeflect
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		_, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "self", Category: CategoryPhysical}, rng)
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatalf("expected item195 activation reaching deflect to reject, got err=%v", err)
		}
		rng.done()
	}

	// 6. Magic category: deflect only triggers on physical, so magic targeting ally with deflect succeeds.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryDeflect
		rng := &targetRNG{t: t}
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "ally", Category: CategoryMagic}, rng)
		if err != nil || !r.Resolved || r.TargetID != "ally" {
			t.Fatalf("expected magic on ally with deflect to succeed, got err=%v, r=%v", err, r)
		}
		rng.done()
	}

	// 7. One-time temporary rule: reflection and cover do NOT re-evaluate deflect.
	// 7a. Reflection: ally has TemporaryReflectAttack, actor self has TemporaryDeflect.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryReflectAttack
		s.actor("self").Temporary = TemporaryDeflect
		rng := &targetRNG{t: t}
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "ally", Category: CategoryPhysical}, rng)
		if err != nil || !r.Resolved || r.TargetID != "self" {
			t.Fatalf("expected reflection to redirect to self without evaluating deflect, got err=%v, r=%v", err, r)
		}
		rng.done()
	}
	// 7b. Cover: ally has TemporaryCover, self has TemporaryCovering, fallen has TemporaryDeflect.
	{
		s, c := targetFixture()
		s.Actors, c.EnemyIDs = s.Actors[:3], nil
		s.actor("ally").Temporary = TemporaryCover
		s.actor("self").Temporary = TemporaryCovering
		s.actor("fallen").Temporary = TemporaryDeflect
		rng := &targetRNG{t: t}
		r, err := s.ResolvePartyTarget(c, TargetEffect{TargetID: "ally", Category: CategoryPhysical}, rng)
		if err != nil || !r.Resolved || r.TargetID != "self" {
			t.Fatalf("expected cover to redirect to covering member without evaluating deflect, got err=%v, r=%v", err, r)
		}
		rng.done()
	}
}
