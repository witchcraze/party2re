package battle

import (
	"errors"
	"reflect"
	"testing"
)

func transformationFixture() (LifetimeState, TransformationContext) {
	s, effect := recoveryFixture()
	for i := range s.Actors {
		s.Actors[i].MaxHP = 999
	}
	a := s.actor("self")
	a.Jobs = BattleJobs{JobID: "job-82", SP: 111, OldJobID: "job-36", OldSP: 100}
	a.OriginalJobs = a.Jobs
	a.Icon = "original.svg"
	a.JobHistory = map[string]*int{"job-01": new(int), "job-02": nil}
	*a.JobHistory["job-01"] = 17
	target := s.actor("ally")
	target.Jobs = BattleJobs{JobID: "job-79", SP: 77, OldJobID: "job-03", OldSP: 8}
	target.MP, target.MaxMP, target.Icon = 3, 20, "ally.svg"
	return s, TransformationContext{EffectContext: effect, RequesterIcon: a.Icon, RequesterGender: "m"}
}

func TestTransformationCopyBoundaries(t *testing.T) {
	for _, id := range []string{"job-36-copy", "book-211-copy", "tool-214-copy"} {
		for _, tc := range []struct {
			hp, maxHP, baseDF int
			applied           bool
		}{
			{1, 999, 999, true}, {0, 999, 999, false}, {-1, 999, 999, false},
			{1, 1000, 999, false}, {1, 999, 1000, false},
		} {
			s, c := transformationFixture()
			a, target := s.actor("self"), s.actor("ally")
			target.HP, target.MaxHP, target.BaseStats.Defense = tc.hp, tc.maxHP, tc.baseDF
			target.Defense = 2000 // Eligibility uses base DF, never current DF.
			before := reloadLifetime(t, s)
			rng := &targetRNG{t: t}
			if id == "tool-214-copy" {
				rng.draws = []float64{.25}
			}
			r, err := s.ApplyTransformation(&c, id, "ally", rng)
			if err != nil || r.Applied != tc.applied {
				t.Fatal(id, tc, r, err)
			}
			if tc.applied {
				wantMP := 50
				if id == "tool-214-copy" {
					wantMP = 3
				}
				if a.Jobs != target.Jobs || a.HP != target.HP || a.MaxHP != 999 || a.MP != wantMP || a.MaxMP != 20 || a.Defense != 2000 || a.BaseStats != target.BaseStats || a.Icon != target.Icon {
					t.Fatal(id, a, target)
				}
				if (a.Status == StatusDofuu) != (id == "book-211-copy") || (s.SkillBans["copy"] == 999) != (id == "book-211-copy") {
					t.Fatal(id, s)
				}
				if a.OriginalJobs != before.Actors[0].OriginalJobs || !reflect.DeepEqual(a.JobHistory, before.Actors[0].JobHistory) || a.CMP != before.Actors[0].CMP || a.Tension != before.Actors[0].Tension {
					t.Fatal("copied identity/resources outside payload", s)
				}
			} else if !reflect.DeepEqual(before, s) {
				t.Fatal("failed copy changed state", s)
			}
			rng.done()
			s = reloadLifetime(t, s)
			if id == "book-211-copy" && tc.applied {
				s.TickSkillBans()
				if s.SkillBans["copy"] != 999 {
					t.Fatal("copy ban expired", s)
				}
			}
		}
	}
}

func TestTransformationCopyFallbackAndBookGuards(t *testing.T) {
	for _, id := range []string{"job-36-copy", "book-211-copy"} {
		s, c := transformationFixture()
		rng := &targetRNG{t: t, draws: []float64{0}}
		r, err := s.ApplyTransformation(&c, id, "missing", rng)
		if err != nil || !r.Applied || r.TargetID != "self" || s.actor("self").MP < 50 {
			t.Fatal(id, r, err)
		}
		rng.done()
		// A present fallen target never falls back to another member.
		s, c = transformationFixture()
		r, err = s.ApplyTransformation(&c, id, "fallen", &targetRNG{t: t})
		if err != nil || r.Applied {
			t.Fatal(id, r, err)
		}
	}
	for _, previous := range []bool{false, true} {
		s, c := transformationFixture()
		a := s.actor("self")
		a.Status = StatusMagicSeal
		if previous {
			a.Jobs.OldJobID = "job-300"
		} else {
			a.Jobs.JobID = "job-300"
		}
		before := reloadLifetime(t, s)
		r, err := s.ApplyTransformation(&c, "book-211-copy", "missing", &targetRNG{t: t})
		if err != nil || r.Applied || !reflect.DeepEqual(before, s) {
			t.Fatal(r, err, s)
		}
	}
	s, c := transformationFixture()
	s.actor("self").Status = StatusMagicSeal
	r, err := s.ApplyTransformation(&c, "job-36-copy", "ally", &targetRNG{t: t, draws: []float64{.25}})
	if err != nil || r.Applied {
		t.Fatal(r, err)
	}
	s, c = transformationFixture()
	s.Actors = s.Actors[:1]
	c.PartyIDs, c.EnemyIDs = []string{"self"}, nil
	r, err = s.ApplyTransformation(&c, "tool-214-copy", "ally", &targetRNG{t: t})
	if err != nil || r.Applied {
		t.Fatal(r, err)
	}
}

func TestFormSwitchPriorityCostsAndNPC(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, npc := range []bool{false, true} {
			s, c := transformationFixture()
			a := s.actor("self")
			a.NPC = npc
			if previous {
				a.Jobs = BattleJobs{JobID: "job-01", SP: 7, OldJobID: "job-82", OldSP: 111}
			} else {
				a.Jobs.OldJobID = "job-82"
			}
			entry, ok := a.BetweenEncounterForm(1, 0)
			if !ok || entry.MPCost != 10 {
				t.Fatal(entry, ok)
			}
			mp, jobs := a.MP, a.Jobs
			r, err := s.ApplyTransformation(&c, entry.ID, "", &targetRNG{t: t})
			if err != nil || !r.Applied || (a.Icon == "original.svg") != npc {
				t.Fatal(r, err, a)
			}
			if previous && (a.Jobs.OldJobID != "job-300" || a.Jobs.JobID != "job-01") || !previous && (a.Jobs.JobID != "job-300" || a.Jobs.OldJobID != "job-82") {
				t.Fatal(a.Jobs)
			}
			if a.Jobs.SP != jobs.SP || a.Jobs.OldSP != jobs.OldSP || a.MP != mp {
				t.Fatal("callback charged or changed SP", a)
			}
			icon := a.Icon
			entry, ok = a.BetweenEncounterForm(1, 0)
			if !ok || entry.MPCost != 0 {
				t.Fatal(entry, ok)
			}
			if _, err := s.ApplyTransformation(&c, entry.ID, "", &targetRNG{t: t}); err != nil || a.Icon != icon || a.Jobs != jobs {
				t.Fatal(err, a)
			}
		}
	}
	s, _ := transformationFixture()
	a := s.actor("self")
	for _, tc := range []struct {
		round, enemies, sp int
		cursed             bool
	}{{0, 0, 111, false}, {1, 1, 111, false}, {1, 0, 110, false}, {1, 0, 111, true}} {
		a.Jobs.SP = tc.sp
		a.Abilities = nil
		if tc.cursed {
			a.Abilities = []string{"cursed"}
		}
		if _, ok := a.BetweenEncounterForm(tc.round, tc.enemies); ok {
			t.Fatal(tc)
		}
	}
}

func TestJobMemoryAndCosmeticReload(t *testing.T) {
	s, c := transformationFixture()
	before := reloadLifetime(t, s)
	for _, id := range []string{"missing", "job-82", "job-36", ""} {
		r, err := s.ApplyTransformation(&c, "tool-168-memory", id, &targetRNG{t: t})
		if err != nil || r.Applied || !reflect.DeepEqual(s, before) {
			t.Fatal(id, r, err)
		}
	}
	if _, err := s.ApplyTransformation(&c, "tool-168-memory", "job-02", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	r, err := s.ApplyTransformation(&c, "tool-168-memory", "job-01", &targetRNG{t: t})
	if err != nil || !r.Applied || s.actor("self").Jobs.OldSP != 17 || s.actor("self").Jobs.OldJobID != "job-01" {
		t.Fatal(r, err, s)
	}
	s = reloadLifetime(t, s)
	if !reflect.DeepEqual(s.actor("self").JobHistory, before.actor("self").JobHistory) {
		t.Fatal("memory changed history")
	}
	before = reloadLifetime(t, s)
	c.MonsterIcons[107] = "native-monsters.svg#108"
	rng := &targetRNG{t: t, draws: []float64{.999}}
	r, err = s.ApplyTransformation(&c, "gem-100-appearance", "", rng)
	if err != nil || !r.Applied || s.actor("self").Icon != c.MonsterIcons[107] {
		t.Fatal(r, err, s)
	}
	before.actor("self").Icon = c.MonsterIcons[107]
	if !reflect.DeepEqual(s, before) {
		t.Fatal("cosmetic changed combat state", s)
	}
	rng.done()
	s = reloadLifetime(t, s)
	r, err = s.ApplyTransformation(&c, "gem-100-appearance", "", &targetRNG{t: t, draws: []float64{0}})
	if err != nil || !r.Applied || s.actor("self").Icon != "assets/images/placeholders/battle/monster-default.svg" {
		t.Fatal(r, err)
	}
}
