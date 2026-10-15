package battle

import (
	"errors"
	"reflect"
	"testing"
)

func arithmeticFixture() (LifetimeState, ArithmeticContext) {
	s, target := targetFixture()
	for i := range s.Actors {
		s.Actors[i].MP, s.Actors[i].MaxHP = 20, 1000
	}
	return s, ArithmeticContext{TargetContext: target, Divisor: 4, Discovered: ArithmeticEntries("job-72")}
}

func TestArithmeticResiduesAndSides(t *testing.T) {
	for _, id := range []string{"job-31-gravity", "job-31-death", "job-72-damage", "job-72-heal", "job-72-reflect", "job-72-revive", "job-72-death"} {
		for _, divisor := range []int{3, 4, 5} {
			if id == "job-31-gravity" && divisor != 4 || id == "job-31-death" && divisor != 5 {
				continue
			}
			for mp := 0; mp <= divisor*2; mp++ {
				s, c := arithmeticFixture()
				c.Divisor = divisor
				for i := range s.Actors {
					s.Actors[i].MP = mp
				}
				var defeated []string
				c.Defeat = func(id string) error { defeated = append(defeated, id); return nil }
				before := reloadLifetime(t, s)
				rng := &targetRNG{t: t}
				selected := mp != 0 && mp%divisor == 0
				if selected && (id == "job-72-damage" || id == "job-72-heal") {
					rng.draws = []float64{0, .5, .99}
				}
				r, err := s.ApplyArithmetic(&c, id, "", rng)
				if err != nil || r.CastFailed {
					t.Fatal(id, mp, r, err)
				}
				rng.done()
				for i, a := range s.Actors {
					want := before.Actors[i].HP
					eligible := selected && (want > 0) == (id != "job-72-revive")
					if eligible {
						switch id {
						case "job-31-gravity":
							want = 25
						case "job-31-death", "job-72-death", "job-72-damage":
							want = 0
						case "job-72-heal":
							want = []int{550, 0, 625, 698, 0}[i]
						case "job-72-revive":
							want = 1000
						}
					}
					if a.HP != want || (a.Temporary == TemporaryReflectMagic) != (eligible && id == "job-72-reflect") {
						t.Fatal(id, divisor, mp, a, want)
					}
				}
				if !selected && !reflect.DeepEqual(s, before) {
					t.Fatal("MP0/residue mutated", s)
				}
				if selected && (id == "job-31-death" || id == "job-72-death" || id == "job-72-damage") && !reflect.DeepEqual(defeated, []string{"self", "ally", "enemy"}) {
					t.Fatal(defeated)
				}
			}
		}
	}
}

func TestArithmeticHPDefenseAndDrawBoundaries(t *testing.T) {
	for _, hp := range []int{1, 10, 999, 1000} {
		for _, defense := range []int{999, 1000} {
			for _, id := range []string{"job-31-gravity", "job-31-death", "job-72-death", "job-72-damage"} {
				s, c := arithmeticFixture()
				for i := range s.Actors {
					s.Actors[i].MP = 0
				}
				a := s.actor("enemy")
				a.MP, a.HP, a.BaseStats.Defense = 20, hp, defense
				c.Defeat = func(string) error { return nil }
				rng := &targetRNG{t: t}
				want := hp
				switch id {
				case "job-31-gravity":
					if hp <= 999 {
						want = max(1, hp/4)
					}
				case "job-31-death", "job-72-death":
					if hp <= 999 {
						want = 0
					}
				case "job-72-damage":
					rng.draws = []float64{.99}
					amount := 311
					if defense > 999 {
						rng.draws = append(rng.draws, .99)
						amount = 2
					}
					want = max(0, hp-amount)
				}
				if _, err := s.ApplyArithmetic(&c, id, "", rng); err != nil || a.HP != want {
					t.Fatal(id, hp, defense, a.HP, want, err)
				}
				rng.done()
			}
		}
	}
	// No ordinary damage/healing modifiers, shields, tension or counters apply.
	s, c := arithmeticFixture()
	for i := range s.Actors {
		s.Actors[i].MP = 0
	}
	a := s.actor("enemy")
	a.MP, a.HP, a.MaxHP, a.Temporary = 4, 900, 1000, TemporaryOneHitDefense
	s.actor("self").Tension = 8
	s.Fields.Create("sanctuary", 7)
	if _, err := s.ApplyArithmetic(&c, "job-72-damage", "", &targetRNG{t: t, draws: []float64{0}}); err != nil || a.HP != 666 || a.Temporary != TemporaryOneHitDefense {
		t.Fatal(a, err)
	}
	if _, err := s.ApplyArithmetic(&c, "job-72-heal", "", &targetRNG{t: t, draws: []float64{.99}}); err != nil || a.HP != 1000 || s.actor("self").Tension != 8 {
		t.Fatal(a, err)
	}
}

func TestArithmeticDigitReversal(t *testing.T) {
	for _, tc := range []struct{ hp, want int }{{1, 1}, {9, 9}, {10, 1}, {12, 21}, {100, 1}, {123, 321}, {999, 999}, {1000, 1000}, {0, 0}} {
		s, c := arithmeticFixture()
		a := s.actor("enemy")
		a.HP, a.MaxHP, a.Temporary = tc.hp, 15, TemporaryReflectMagic
		if _, err := s.ApplyArithmetic(&c, "job-31-reverse", "enemy", &targetRNG{t: t}); err != nil || a.HP != tc.want {
			t.Fatal(tc, a, err)
		}
	}
	s, c := arithmeticFixture()
	s.actor("ally").HP = 120
	rng := &targetRNG{t: t, draws: []float64{.99}}
	r, err := s.ApplyArithmetic(&c, "job-31-reverse", "missing", rng)
	if err != nil || s.actor("ally").HP != 21 || r.Applications[0].TargetID != "ally" {
		t.Fatal(r, err)
	}
	rng.done()
}

func TestArithmeticSubmissionAndAdmission(t *testing.T) {
	for _, separator := range []string{" ", "　"} {
		for _, divisor := range []int{3, 4, 5} {
			s, c := arithmeticFixture()
			c.SubmittedText = "＠倍数指定" + separator + "＠×重複" + separator
			c.Discovered = []ArithmeticEntry{{ID: "unsupported-first", Name: "×重複"}, {ID: "job-72-reflect", Name: "×重複"}}
			id := map[int]string{3: "job-72-divisor-3", 4: "job-72-divisor-4", 5: "job-72-divisor-5"}[divisor]
			if err := s.ValidateArithmetic(&c, id, "", &targetRNG{t: t}); err != nil {
				t.Fatal(err)
			}
			r, err := s.ApplyArithmetic(&c, id, "", &targetRNG{t: t})
			if err != nil || r.CastFailed || c.Divisor != divisor || (s.actor("enemy").Temporary == TemporaryReflectMagic) != (20%divisor == 0) {
				t.Fatal(r, err, c)
			}
		}
	}
	for _, invalid := range []string{"hidden child", "defeat hook", "negative reversal"} {
		s, c := arithmeticFixture()
		id, target := "job-72-divisor-4", ""
		c.SubmittedText = "＠倍数指定＠×反射 "
		if invalid == "hidden child" {
			c.Discovered = append(c.Discovered, ArithmeticEntry{ID: "unsupported-last", Name: "×反射"})
		}
		if invalid == "defeat hook" {
			id = "job-72-damage"
		}
		if invalid == "negative reversal" {
			id, target = "job-31-reverse", "enemy"
			s.actor(target).HP = -1
		}
		s.actor("self").Status = StatusMagicSeal
		before := reloadLifetime(t, s)
		rng := &targetRNG{t: t}
		if _, err := s.ApplyArithmetic(&c, id, target, rng); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
			t.Fatal(invalid, err, s)
		}
		rng.done()
	}
	for _, id := range []string{"job-72-damage", "job-72-divisor-4"} {
		s, c := arithmeticFixture()
		c.Divisor = 0
		before := reloadLifetime(t, s)
		r, err := s.ApplyArithmetic(&c, id, "", &targetRNG{t: t})
		if err != nil || !r.CastFailed || !reflect.DeepEqual(before, s) {
			t.Fatal(r, err)
		}
	}
}

func TestArithmeticReturnedNoChangeFacts(t *testing.T) {
	for _, tc := range []struct {
		id    string
		hp    int
		draws []float64
	}{
		{"job-31-gravity", 1, nil}, {"job-31-reverse", 999, nil},
		{"job-72-heal", 1000, []float64{0}}, {"job-72-reflect", 100, nil},
	} {
		s, c := arithmeticFixture()
		for i := range s.Actors {
			s.Actors[i].MP = 0
		}
		s.actor("enemy").MP, s.actor("enemy").HP = 20, tc.hp
		s.actor("enemy").Temporary = TemporaryReflectMagic
		r, err := s.ApplyArithmetic(&c, tc.id, "enemy", &targetRNG{t: t, draws: tc.draws})
		if err != nil || len(r.Applications) != 1 || !r.Applications[0].Returned || r.Applications[0].Applied || r.Applications[0].Delta != 0 {
			t.Fatal(tc.id, r, err)
		}
	}
}
