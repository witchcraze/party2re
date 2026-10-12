package battle

import (
	"encoding/json"
	"reflect"
	"testing"
)

type fixedFloatRNG struct{ val float64 }

func (r fixedFloatRNG) Float64() float64 { return r.val }
func (r fixedFloatRNG) Intn(n int) int   { return int(r.val * float64(n)) }
func (r fixedFloatRNG) IntN(n int) int   { return r.Intn(n) }

func reloadLifetime(t *testing.T, s LifetimeState) LifetimeState {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var out LifetimeState
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(s, out) {
		t.Fatalf("round trip: %#v != %#v", s, out)
	}
	return out
}

func TestLifetimeFields(t *testing.T) {
	for _, suppressed := range []bool{false, true} {
		for _, enemies := range []bool{false, true} {
			s := LifetimeState{Fields: LifetimeFields{Default: "default", Temporary: "temporary", Turns: 1}}
			s = reloadLifetime(t, s)
			s.Fields.FinishOperation(enemies, suppressed)
			s = reloadLifetime(t, s)
			want := "temporary"
			if enemies && !suppressed {
				want = "default"
			}
			if s.Fields.Current() != want {
				t.Fatalf("enemies=%v suppressed=%v: %+v", enemies, suppressed, s.Fields)
			}
		}
	}
	// Creation in this operation ticks too; anti expires even without enemies.
	s := LifetimeState{Fields: LifetimeFields{Default: "default"}}
	s.Fields.Create("new", 2)
	s.Fields.FinishOperation(true, false)
	if s.Fields.Turns != 1 {
		t.Fatal(s.Fields)
	}
	s.Fields.CreateAnti(1)
	s = reloadLifetime(t, s)
	if s.Fields.Current() != "default" || s.Fields.Exercises("default", "default", true, false) {
		t.Fatal(s.Fields)
	}
	s.Fields.FinishOperation(false, false)
	s = reloadLifetime(t, s)
	if s.Fields.AntiTurns != 0 || s.Fields.Temporary != "" {
		t.Fatal(s.Fields)
	}
}

func TestLifetimeCounterPredicates(t *testing.T) {
	for _, kind := range []ExtendedEffect{ExtendedFrenzy, ExtendedAwakening, ExtendedMagicSword, ExtendedCrown, ExtendedWarGod, ExtendedFlame, ExtendedBow, ExtendedAdditional, ExtendedPurification} {
		for _, npc := range []bool{false, true} {
			for _, damage := range []int{0, 1} {
				s := LifetimeState{Actors: []LifetimeActor{{Extended: kind, ExtendedTurns: 1}}}
				s = reloadLifetime(t, s)
				facts := LifetimeTail{NPC: npc, DamageCalls: damage, BowAction: true, SecondSubmitted: true}
				s.Actors[0].FinishExtended(facts)
				s = reloadLifetime(t, s)
				wantTick := !npc
				if kind == ExtendedPurification {
					wantTick = npc
				}
				if kind == ExtendedMagicSword || kind == ExtendedCrown || kind == ExtendedWarGod || kind == ExtendedFlame {
					wantTick = !npc && damage > 0
				}
				if (s.Actors[0].Extended == "") != wantTick {
					t.Fatalf("%s npc=%v damage=%d: %+v", kind, npc, damage, s.Actors[0])
				}
			}
		}
	}
}

func TestLifetimeCMPBansAndSuppression(t *testing.T) {
	s := LifetimeState{Actors: []LifetimeActor{{CMPCharge: 2, CMPNeedCharge: 3}}, SkillBans: map[string]int{"finite": 1, "infinite": 999}}
	s = reloadLifetime(t, s)
	if s.Actors[0].AdvanceCMP(4, 5) {
		t.Fatal("recharged before threshold")
	}
	s = reloadLifetime(t, s)
	if !s.Actors[0].AdvanceCMP(4, 5) || s.Actors[0].CMPCharge != 0 || s.Actors[0].CMPNeedCharge != 5 {
		t.Fatal(s.Actors[0])
	}
	s.TickSkillBans()
	s = reloadLifetime(t, s)
	if len(s.SkillBans) != 1 || s.SkillBans["infinite"] != 999 {
		t.Fatal(s.SkillBans)
	}
	if !s.RecordFirstUse("side", "cosmic") {
		t.Fatal("first use")
	}
	s = reloadLifetime(t, s)
	if s.RecordFirstUse("side", "cosmic") || !s.RecordFirstUse("other", "cosmic") {
		t.Fatal(s.SkillHistory)
	}
}

func TestLifetimeSecondSubmission(t *testing.T) {
	for _, mp := range []int{9, 10, 11} {
		a := LifetimeActor{Extended: ExtendedAdditional, ExtendedTurns: 2}
		if CanExecuteSecond(a.Extended == ExtendedAdditional, true, true, mp, 10) != (mp > 10) {
			t.Fatal(mp)
		}
		a.FinishExtended(LifetimeTail{SecondSubmitted: true})
		if a.ExtendedTurns != 1 {
			t.Fatal("submission must tick without execution")
		}
	}
	a := LifetimeActor{Extended: ExtendedAdditional, ExtendedTurns: 1}
	if CanExecuteSecond(a.Extended == ExtendedAdditional, false, true, 11, 10) || CanExecuteSecond(true, true, false, 11, 10) || CanExecuteSecond(false, true, true, 11, 10) {
		t.Fatal("whitelist")
	}
}
