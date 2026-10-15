package battle

import (
	"errors"
	"reflect"
	"testing"
)

func TestArithmeticMagicStateAndFailedCast(t *testing.T) {
	for _, id := range []string{"job-31-gravity", "job-31-death", "job-31-reverse", "job-72-divisor-3", "job-72-divisor-4", "job-72-divisor-5", "job-72-damage", "job-72-reflect", "job-72-heal", "job-72-revive", "job-72-death"} {
		for _, cure := range []float64{.2499, .25} {
			s, c := arithmeticFixture()
			for i := range s.Actors {
				s.Actors[i].MP = 0
			}
			c.SubmittedText = "＠倍数指定＠×反射 "
			s.actor("self").Status = StatusMagicSeal
			before := reloadLifetime(t, s)
			rng := &targetRNG{t: t, draws: []float64{cure}}
			if _, err := s.ApplyArithmetic(&c, id, "enemy", rng); err != nil {
				t.Fatal(id, err)
			}
			rng.done()
			if cure == .25 && !reflect.DeepEqual(s, before) || (s.actor("self").Status == "") != (cure < .25) {
				t.Fatal(id, cure, s)
			}
		}
	}
	for _, text := range []string{"", "＠ＭＰ４ ＠×反射", "＠ＭＰ４ ＠×未発見 ", "＠ＭＰ４ ×反射 ", "＠ＭＰ４ ＠×反射\n"} {
		s, c := arithmeticFixture()
		c.SubmittedText = text
		before := reloadLifetime(t, s)
		r, err := s.ApplyArithmetic(&c, "job-72-divisor-4", "", &targetRNG{t: t})
		if err != nil || !r.CastFailed || !reflect.DeepEqual(s, before) {
			t.Fatal(text, r, err)
		}
	}
	// A direct random child without a divisor still attempts the magic cure first.
	s, c := arithmeticFixture()
	c.Divisor, s.Actors[0].Status = 0, StatusMagicSeal
	rng := &targetRNG{t: t, draws: []float64{0}}
	r, err := s.ApplyArithmetic(&c, "job-72-damage", "", rng)
	if err != nil || !r.CastFailed || s.Actors[0].Status != "" {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestArithmeticDirectCallbackRetainsCapturedIdentity(t *testing.T) {
	s, c := arithmeticFixture()
	c.Discovered = []ArithmeticEntry{{ID: "job-72-reflect", Name: "×重複"}, {ID: "job-72-heal", Name: "×重複"}}
	c.SubmittedText = "＠倍数指定＠×重複 "
	// A first-match delegated or positional random executor passes its captured
	// ID; the later same-name action must not replace that callback.
	if _, err := s.ApplyArithmetic(&c, c.Discovered[0].ID, "", &targetRNG{t: t}); err != nil || s.actor("enemy").Temporary != TemporaryReflectMagic || s.actor("enemy").HP != 100 {
		t.Fatal(s, err)
	}
	if _, err := s.ApplyArithmetic(&c, "job-72-divisor-4", "", &targetRNG{t: t, draws: []float64{0, 0, 0}}); err != nil || s.actor("enemy").HP != 550 {
		t.Fatal(s, err)
	}
	for _, id := range []string{"", "unsupported"} {
		c.Discovered = append(c.Discovered, ArithmeticEntry{ID: id, Name: "×重複"})
		before := reloadLifetime(t, s)
		if _, err := s.ApplyArithmetic(&c, "job-72-divisor-4", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(s, before) {
			t.Fatal(id, err, s)
		}
	}
}

func TestArithmeticDefeatErrorAndNoEffectAdmission(t *testing.T) {
	s, c := arithmeticFixture()
	failure := errors.New("defeat failed")
	c.Defeat = func(string) error { return failure }
	if _, err := s.ApplyArithmetic(&c, "job-31-death", "", &targetRNG{t: t}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		id          string
		hp, defense int
	}{
		{"job-31-death", 1000, 0}, {"job-72-death", 1000, 0},
		{"job-72-damage", 312, 999}, {"job-72-damage", 3, 1000},
	} {
		s, c := arithmeticFixture()
		for i := range s.Actors {
			s.Actors[i].HP, s.Actors[i].BaseStats.Defense = tc.hp, tc.defense
		}
		if err := s.ValidateArithmetic(&c, tc.id, "", &targetRNG{t: t}); err != nil {
			t.Fatal(tc, err)
		}
	}
	// The one/four-or-more-character cases do not read a decimal sign as a digit.
	s, c = arithmeticFixture()
	s.actor("enemy").HP = -100
	if _, err := s.ApplyArithmetic(&c, "job-31-reverse", "enemy", &targetRNG{t: t}); err != nil || s.actor("enemy").HP != -100 {
		t.Fatal(s, err)
	}
	// Healing's direct cap write never invokes defeat, even for a zero maximum.
	s, c = arithmeticFixture()
	for i := range s.Actors {
		s.Actors[i].MP = 0
	}
	s.actor("enemy").MP, s.actor("enemy").MaxHP = 4, 0
	if _, err := s.ApplyArithmetic(&c, "job-72-heal", "", &targetRNG{t: t, draws: []float64{0}}); err != nil || s.actor("enemy").HP != 0 {
		t.Fatal(s, err)
	}
}

func TestArithmeticOrderedContinuationAndEndProcessing(t *testing.T) {
	var final LifetimeState
	var finalResults []ArithmeticResult
	for _, reload := range []bool{false, true} {
		s, c := arithmeticFixture()
		// The first defeat's injected owner changes later eligibility. This is
		// a boundary fixture, not certification of the absent equipment catalog.
		s.actor("fallen").HP = -1
		s.actor("ally").HP = 1000
		c.Defeat = func(id string) error {
			a := s.actor(id)
			if id == "self" {
				a.HP = 100 // supplied revival owner
				s.actor("fallen").HP = 100
				s.actor("ally").HP = 999
			} else {
				if !a.CrystalDropped {
					a.CrystalDropped = true
					s.DroppedCrystals++
				}
			}
			return nil
		}
		if reload {
			s = reloadLifetime(t, s)
		}
		r, err := s.ApplyArithmetic(&c, "job-31-death", "", &targetRNG{t: t})
		if err != nil || s.actor("fallen").HP != 0 || s.actor("ally").HP != 0 || s.DroppedCrystals != 3 {
			t.Fatal(r, s, err)
		}
		results := []ArithmeticResult{r}
		end := EndProcessing{}
		end.RecordDarkness(40)
		// Arithmetic writes do not record _damage/_risk or retroactively change
		// its queued darkness debit. Flush uses the surviving revived actor.
		if _, err := s.CompleteOperation(LifetimeCompletion{Accepted: true, ActorID: "self", EnemiesRemain: true, CMPAmount: 1, NextCMPThreshold: 3, End: end}, c.PartyIDs); err != nil || s.actor("self").HP != 80 {
			t.Fatal(s, err)
		}
		if reload {
			s = reloadLifetime(t, s)
		}
		r, err = s.ApplyArithmetic(&c, "job-72-revive", "", &targetRNG{t: t})
		if err != nil || s.actor("fallen").HP != 1000 || s.actor("dead_enemy").HP != 1000 || s.DroppedCrystals != 3 {
			t.Fatal(r, s, err)
		}
		results = append(results, r)
		if reload {
			s = reloadLifetime(t, s)
		}
		r, err = s.ApplyArithmetic(&c, "job-31-reverse", "self", &targetRNG{t: t})
		if err != nil || s.actor("self").HP != 8 {
			t.Fatal(r, err)
		}
		results = append(results, r)
		if reload {
			if !reflect.DeepEqual(s, final) || !reflect.DeepEqual(results, finalResults) {
				t.Fatal("reload changed continuation", s, final)
			}
		} else {
			final, finalResults = s, results
		}
	}
}

func TestArithmeticRandomContinuationReload(t *testing.T) {
	var final LifetimeState
	var facts []ArithmeticResult
	for _, reload := range []bool{false, true} {
		s, c := arithmeticFixture()
		for i := range s.Actors {
			if s.Actors[i].HP > 0 {
				s.Actors[i].HP = 1000
			}
		}
		s.actor("enemy").BaseStats.Defense = 1000
		s.actor("fallen").Status, s.actor("fallen").Temporary = StatusPoison, TemporaryRevival
		rng := &targetRNG{t: t, draws: []float64{0, .5, .99, 0, .2, .6, .9}}
		var results []ArithmeticResult
		for _, id := range []string{"job-72-damage", "job-72-heal", "job-72-revive", "job-72-reflect"} {
			if reload {
				s = reloadLifetime(t, s)
			}
			r, err := s.ApplyArithmetic(&c, id, "", rng)
			if err != nil {
				t.Fatal(id, err)
			}
			results = append(results, r)
			if id == "job-72-revive" && (s.actor("fallen").Status != StatusPoison || s.actor("fallen").Temporary != TemporaryRevival) {
				t.Fatal("direct full revival reset effects", s)
			}
			if reload {
				s = reloadLifetime(t, s)
			}
		}
		rng.done()
		if reload {
			if !reflect.DeepEqual(s, final) || !reflect.DeepEqual(results, facts) {
				t.Fatal("reload changed draws or effects", s, results)
			}
		} else {
			final, facts = s, results
		}
	}
}
