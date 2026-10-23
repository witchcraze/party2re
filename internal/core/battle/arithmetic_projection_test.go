package battle

import (
	"reflect"
	"testing"
)

func TestArithmeticUnknownProjectionLeavesInputAndHooksUntouched(t *testing.T) {
	for _, id := range []string{"job-31-death", "job-72-damage", "job-72-heal", "job-72-death", "job-31-reverse", "job-72-divisor-4"} {
		s, c := arithmeticFixture()
		c.Defeat = func(string) error { t.Fatal("projection ran defeat"); return nil }
		c.SubmittedText = "＠倍数指定＠×回復 "
		c.Divisor = 0
		if id == "job-72-damage" || id == "job-72-heal" || id == "job-72-death" {
			c.Divisor = 4
		}
		before := reloadLifetime(t, s)
		divisor := c.Divisor
		known, err := s.ProjectArithmetic(&c, id, "missing")
		if err != nil || known || !reflect.DeepEqual(s, before) || c.Divisor != divisor {
			t.Fatal(id, known, err, s, c)
		}
	}
}

func TestArithmeticExactProjectionWritesIndependentExpectedFacts(t *testing.T) {
	for _, tc := range []struct {
		id   string
		want int
	}{
		{"job-31-gravity", 30}, {"job-31-reverse", 21},
		{"job-72-reflect", 120}, {"job-72-revive", 1000},
		{"job-72-divisor-4", 120},
	} {
		s, c := arithmeticFixture()
		for i := range s.Actors {
			s.Actors[i].MP = 0
		}
		a := s.actor("enemy")
		a.MP, a.HP = 20, 120
		if tc.id == "job-72-revive" {
			a.HP = 0
		}
		c.SubmittedText = "＠倍数指定＠×反射 "
		known, err := s.ProjectArithmetic(&c, tc.id, "enemy")
		if err != nil || !known || a.HP != tc.want || (a.Temporary == TemporaryReflectMagic) != (tc.id == "job-72-reflect" || tc.id == "job-72-divisor-4") || a.MP != 20 {
			t.Fatal(tc, known, err, a)
		}
	}
	s, c := arithmeticFixture()
	s.actor(c.ActorID).Status = StatusMagicSeal
	before := reloadLifetime(t, s)
	if known, err := s.ProjectArithmetic(&c, "job-31-gravity", ""); err != nil || known || !reflect.DeepEqual(s, before) {
		t.Fatal(known, err, s)
	}
}
