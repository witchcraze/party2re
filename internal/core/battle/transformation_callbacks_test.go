package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestTransformedTableAndAppearancePredicates(t *testing.T) {
	for _, gender := range []string{"m", "f"} {
		for _, npc := range []bool{false, true} {
			s, c := transformationFixture()
			c.RequesterGender = gender
			a := s.actor("self")
			a.NPC = npc
			for _, matching := range []bool{false, true} {
				a.Icon = "other.svg"
				if matching {
					a.Icon = c.RequesterIcon
				}
				calls := 0
				c.AreaDamage = func(ctx TargetContext, power float64, category CallbackCategory, direct bool, rng random.Generator) error {
					calls++
					if power != 150 || category != CategoryBreath || !direct || ctx.ActorID != "self" {
						t.Fatal(power, category, direct, ctx)
					}
					return nil
				}
				entry := a.TransformationEntries("job-79", gender)[0]
				r, err := s.ApplyTransformation(&c, entry.ID, "", &targetRNG{t: t})
				if err != nil || r.Applied != matching || calls != 1 {
					t.Fatal(r, err, calls)
				}
				if matching {
					if a.TransformationEntries("job-79", gender)[0].ID != "job-79-speed" || a.TransformationEntries("job-79", map[string]string{"m": "f", "f": "m"}[gender])[0].ID != "job-79-breath" {
						t.Fatal(a.Icon)
					}
					s = reloadLifetime(t, s)
					r, err = s.ApplyTransformation(&c, "job-79-speed", "", &targetRNG{t: t})
					a = s.actor("self")
					if err != nil || !r.Applied || a.Agility != 250 || a.Icon != c.RequesterIcon || a.TransformationEntries("job-79", gender)[0].ID != "job-79-breath" {
						t.Fatal(r, err, a)
					}
				}
			}
		}
	}
	s, _ := transformationFixture()
	entries := s.actor("self").TransformationEntries("job-300", "m")
	if len(entries) != 4 {
		t.Fatal(entries)
	}
	for i, cost := range []int{30, 48, 99, 0} {
		if entries[i].MPCost != cost || entries[i].RequiredSP != 0 {
			t.Fatal(entries)
		}
	}
}

func TestFormTuneOrderAndAdmission(t *testing.T) {
	s, c := transformationFixture()
	s.actor("self").Tension = 3
	rng := &targetRNG{t: t, draws: []float64{0, .99}}
	r, err := s.ApplyTransformation(&c, "job-300-tune", "", rng)
	if err != nil || !r.Applied || len(r.Stats) != 2 || s.actor("self").Agility != 250 || s.actor("self").Attack != 200 || c.StatUpCalls != 2 {
		t.Fatal(r, err, s, c)
	}
	rng.done()
	for _, id := range []string{"job-300-tune", "job-79-speed"} {
		s, c = transformationFixture()
		s.actor("self").ItemDefinitionIDs = []string{"item-201"}
		before := reloadLifetime(t, s)
		r, err = s.ApplyTransformation(&c, id, "", &targetRNG{t: t})
		if !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.StatUpCalls != 0 {
			t.Fatal(id, r, err, s)
		}
		// At cap, the missing damage tail is unreachable and needs no executor.
		s.actor("self").Attack, s.actor("self").Defense, s.actor("self").Agility = 250, 250, 250
		rng := &targetRNG{t: t}
		if id == "job-300-tune" {
			rng.draws = []float64{0, 0}
		}
		if _, err := s.ApplyTransformation(&c, id, "", rng); err != nil {
			t.Fatal(id, err)
		}
		rng.done()
	}
	// A first205 buff can remove cover and expose an unsupported second201 tail.
	s, c = transformationFixture()
	s.actor("self").Temporary = TemporaryCover
	s.actor("ally").Temporary = TemporaryCovering
	s.actor("self").ItemDefinitionIDs = []string{"item-201"}
	s.actor("ally").ItemDefinitionIDs = []string{"item-205"}
	level := 99
	c.RequesterLevel = &level
	before := reloadLifetime(t, s)
	if _, err := s.ApplyTransformation(&c, "job-300-tune", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.StatUpCalls != 0 {
		t.Fatal(err, s, c)
	}
}

func TestFormStrikeRestoresTemporaryAfterDamageReset(t *testing.T) {
	s, c := transformationFixture()
	s.actor("enemy").Temporary = TemporaryStrongDefense
	c.Damage = func(ctx TargetContext, e TargetEffect, rng random.Generator) error {
		if e.TargetID != "enemy" || e.Payload.Number != 150 || e.Category != CategoryPhysical || !reflect.DeepEqual(e.Arguments, []string{"1"}) || s.actor("enemy").Temporary != "" {
			t.Fatal(e, s)
		}
		s.actor("enemy").HP = 0
		s.actor("enemy").ResetEffects(LifetimeReset{Temporary: TemporaryDefense})
		s.Actors = append(s.Actors[:len(s.Actors):len(s.Actors)], LifetimeActor{Participant: Participant{ID: "new-helper"}})
		return nil
	}
	r, err := s.ApplyTransformation(&c, "job-300-strike", "enemy", &targetRNG{t: t})
	if err != nil || !r.Applied || s.actor("enemy").Temporary != TemporaryStrongDefense {
		t.Fatal(r, err, s)
	}
}

func TestFormBurstLaterHitDeathStillRevertsPreviousForm(t *testing.T) {
	s, c := transformationFixture()
	a := s.actor("self")
	a.Jobs.JobID, a.Jobs.OldJobID, a.Tension = "job-01", "job-300", 5
	calls := 0
	c.Damage = func(_ TargetContext, e TargetEffect, _ random.Generator) error {
		calls++
		if a.Tension != 5 || e.Payload.Number != 200 {
			t.Fatal(calls, a, e)
		}
		a.Tension = 1
		if calls == 2 {
			a.HP = 0
		}
		return nil
	}
	rng := &targetRNG{t: t, draws: []float64{.999, 0}}
	if _, err := s.ApplyTransformation(&c, "job-300-burst", "enemy", rng); err != nil || calls != 2 || a.Jobs.OldJobID != "job-82" || a.Status != StatusDofuu || a.Tension != 1 {
		t.Fatal(err, calls, a)
	}
	rng.done()
}

func TestTransformationTailAfterRosterGrowth(t *testing.T) {
	for _, id := range []string{"job-79-breath", "job-300-burst"} {
		s, c := transformationFixture()
		s.actor("self").Jobs.JobID = "job-300"
		grow := func() {
			s.Actors = append(s.Actors[:len(s.Actors):len(s.Actors)], LifetimeActor{Participant: Participant{ID: "new-helper"}})
		}
		c.AreaDamage = func(TargetContext, float64, CallbackCategory, bool, random.Generator) error { grow(); return nil }
		c.Damage = func(TargetContext, TargetEffect, random.Generator) error { grow(); s.actor("self").HP = 0; return nil }
		rng := &targetRNG{t: t}
		if id == "job-300-burst" {
			rng.draws = []float64{0}
		}
		if _, err := s.ApplyTransformation(&c, id, "enemy", rng); err != nil {
			t.Fatal(err)
		}
		a := s.actor("self")
		if id == "job-79-breath" && a.Icon != nativeFormIcon("winged", "m") || id == "job-300-burst" && (a.Jobs.JobID != "job-82" || a.Status != StatusDofuu || a.Tension != 1) {
			t.Fatal(id, a)
		}
		rng.done()
	}
}

func TestFormBurstHitCountTensionPowerAndDeathTail(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, death := range []bool{false, true} {
			for _, draw := range []float64{0, .999} {
				s, c := transformationFixture()
				a := s.actor("self")
				a.Jobs.JobID, a.Jobs.OldJobID, a.Tension = "job-300", "job-300", 5
				if previous {
					a.Jobs.JobID = "job-01"
				}
				s.actor("dead_enemy").MaxHP = 1000 // Subsequent draws keep the invocation's roster, even fallen.
				calls := 0
				c.Damage = func(ctx TargetContext, e TargetEffect, rng random.Generator) error {
					calls++
					want := 500.0
					if calls == 1 {
						want = 200
					}
					if a.Tension != 5 || e.Payload.Number != want || e.Category != CategoryBreath || !reflect.DeepEqual(e.Arguments, []string{"1"}) {
						t.Fatal(calls, a, e)
					}
					a.Tension = 1
					if death {
						a.HP = 0
					}
					return nil
				}
				wantHits := 2 + int(draw*2)
				draws := []float64{draw}
				if death {
					wantHits = 1
				} else {
					for range wantHits - 1 {
						draws = append(draws, .999)
					}
				}
				rng := &targetRNG{t: t, draws: draws}
				r, err := s.ApplyTransformation(&c, "job-300-burst", "enemy", rng)
				if err != nil || !r.Applied || calls != wantHits || a.Tension != 1 || a.Status != StatusDofuu || (previous && a.Jobs.OldJobID != "job-82") || (!previous && (a.Jobs.JobID != "job-82" || a.Jobs.OldJobID != "job-300")) {
					t.Fatal(r, err, calls, a)
				}
				rng.done()
			}
		}
	}
}

func TestTransformationUnsupportedAndFailureDoNotAdvance(t *testing.T) {
	for _, id := range []string{"unknown", "job-300-strike", "job-300-burst", "job-79-breath"} {
		s, c := transformationFixture()
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		admission := s.ValidateTransformation(&c, id, "enemy", rng)
		_, err := s.ApplyTransformation(&c, id, "enemy", rng)
		if !errors.Is(admission, ErrUnsupportedEffect) || !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) || c.DamageCalls != 0 {
			t.Fatal(id, admission, err, s)
		}
		rng.done()
	}
	s, c := transformationFixture()
	sentinel := errors.New("certified damage execution failed")
	c.Damage = func(TargetContext, TargetEffect, random.Generator) error { return sentinel }
	_, err := s.ApplyTransformation(&c, "job-300-burst", "enemy", &targetRNG{t: t, draws: []float64{0}})
	if !errors.Is(err, sentinel) || s.actor("self").Jobs.JobID != "job-82" || s.actor("self").Status != "" {
		t.Fatal(err, s)
	}
	// Generic batch DTOs cannot acquire transformation or indirect support.
	if !errors.Is(ValidateSkillAction(ActionSkill{Kind: "transform"}), ErrUnsupportedEffect) || !errors.Is(ValidateCustomSkillAction(ActionCustomSkill{Gems: []GemEffect{{Kind: "transform"}}}), ErrUnsupportedEffect) {
		t.Fatal("batch bypass admitted")
	}
}

func TestTransformationSerializedContinuationAndReset(t *testing.T) {
	var final []LifetimeState
	for _, reload := range []bool{false, true} {
		s, c := transformationFixture()
		original := reloadLifetime(t, s)
		for _, step := range []struct {
			id, target string
			draws      []float64
		}{
			{"job-82-form", "", nil}, {"job-300-tune", "", []float64{0, .99}}, {"job-300-form", "", nil}, {"tool-168-memory", "job-01", nil},
			{"job-36-copy", "ally", nil}, {"tool-214-copy", "ignored", []float64{.25}}, {"gem-100-appearance", "", []float64{.5}},
		} {
			rng := &targetRNG{t: t, draws: step.draws}
			if _, err := s.ApplyTransformation(&c, step.id, step.target, rng); err != nil {
				t.Fatal(err)
			}
			rng.done()
			if reload {
				s = reloadLifetime(t, s)
			}
		}
		a := s.actor("self")
		before := *a
		a.ResetEffects(LifetimeReset{Attack: a.BaseStats.Attack, Defense: a.BaseStats.Defense, Agility: a.BaseStats.Agility})
		if a.Jobs != before.Jobs || a.Icon != before.Icon || a.MaxHP != before.MaxHP || a.BaseStats != before.BaseStats || a.OriginalJobs != original.actor("self").OriginalJobs || !reflect.DeepEqual(a.JobHistory, original.actor("self").JobHistory) {
			t.Fatal("reset discarded temporary identity", a)
		}
		final = append(final, s)
	}
	if !reflect.DeepEqual(final[0], final[1]) {
		t.Fatal(final)
	}
}
