package battle

import (
	"reflect"
	"testing"
)

type lifetimeCountingRNG struct{ floats int }

func (r *lifetimeCountingRNG) Float64() float64 { r.floats++; return 0.9 }
func (r *lifetimeCountingRNG) Intn(n int) int   { return 0 }
func (r *lifetimeCountingRNG) IntN(n int) int   { return r.Intn(n) }

func TestLifetimeEmptyTemporaryConsumesDraw(t *testing.T) {
	rng := &lifetimeCountingRNG{}
	a := LifetimeActor{}
	if err := a.BeginNormalAction(false, false, false, rng); err != nil {
		t.Fatal(err)
	}
	if rng.floats != 1 {
		t.Fatal(rng.floats)
	}
}

type lifetimeSequenceRNG struct {
	values []float64
	draws  int
}

func (r *lifetimeSequenceRNG) Float64() float64 { v := r.values[r.draws]; r.draws++; return v }
func (r *lifetimeSequenceRNG) Intn(n int) int   { return int(r.Float64() * float64(n)) }
func (r *lifetimeSequenceRNG) IntN(n int) int   { return r.Intn(n) }

func TestLifetimeNPCExtraCuresAndBypassDraw(t *testing.T) {
	for _, status := range []string{StatusParalyze, StatusSleep, StatusConfusion} {
		for _, special := range []bool{false, true} {
			rng := &lifetimeSequenceRNG{values: []float64{0.9, 0.9, 0.1}}
			a := LifetimeActor{Participant: Participant{Status: status}}
			decision, err := a.CheckIncapacitation(IncapacitationContext{NPC: true, SpecialBossField: special, PhantomField: true}, rng)
			if err != nil {
				t.Fatal(err)
			}
			if (a.Status == "") != special || rng.draws != 2+btoi(special) || decision.NormalAction != (status == StatusConfusion) {
				t.Fatal(status, special, a, rng.draws, decision)
			}
		}
	}
	rng := &lifetimeSequenceRNG{values: []float64{0.1}}
	a := LifetimeActor{Participant: Participant{Status: StatusKinju, HP: 100, MaxHP: 100}}
	decision, err := a.CheckIncapacitation(IncapacitationContext{Bypass: true}, rng)
	if err != nil || !decision.NormalAction || a.HP != 100 || rng.draws != 1 {
		t.Fatal(a, decision, rng.draws, err)
	}
}

func btoi(v bool) int {
	if v {
		return 1
	}
	return 0
}

func TestLifetimeAdditionalEligibilityPrecedesFirstAction(t *testing.T) {
	s := LifetimeState{Actors: []LifetimeActor{{Extended: ExtendedAdditional, ExtendedTurns: 1}}}
	s = reloadLifetime(t, s)
	activeAtSubmission := s.Actors[0].Extended == ExtendedAdditional
	s.Actors[0].ResetEffects(LifetimeReset{})
	s = reloadLifetime(t, s)
	if !CanExecuteSecond(activeAtSubmission, true, true, 11, 10) {
		t.Fatal("rechecked effect after first action")
	}
}

func TestLifetimeRandomExpiryAndTension(t *testing.T) {
	for _, effect := range []TemporaryEffect{TemporaryReflectMagic, TemporaryCover} {
		for _, draw := range []float64{0, 0.3333333333333333, 0.99} {
			s := LifetimeState{Actors: []LifetimeActor{{Temporary: effect}}}
			s = reloadLifetime(t, s)
			if err := s.Actors[0].BeginNormalAction(false, false, true, fixedFloatRNG{val: draw}); err != nil {
				t.Fatal(err)
			}
			s = reloadLifetime(t, s)
			if (s.Actors[0].Temporary == "") != (draw < 1.0/3) {
				t.Fatal(draw, s)
			}
		}
	}
	s := LifetimeState{Actors: []LifetimeActor{{Tension: 1, Hit: 95}}}
	for _, want := range []float64{1.7, 3, 5, 8, 8} {
		s = reloadLifetime(t, s)
		if err := s.Actors[0].AdvanceTension(99, fixedFloatRNG{val: 0}); err != nil {
			t.Fatal(err)
		}
		s = reloadLifetime(t, s)
		if s.Actors[0].Tension != want {
			t.Fatal(s)
		}
	}
	for _, level := range []int{24, 25, 69} {
		a := LifetimeActor{Tension: 5}
		if err := a.AdvanceTension(level, fixedFloatRNG{val: 0.99}); err != nil {
			t.Fatal(err)
		}
		if (a.Tension == 8) != (level == 69) {
			t.Fatal(level, a)
		}
	}
	a := LifetimeActor{Participant: Participant{Status: StatusSabaku}, Tension: 1}
	if err := a.AdvanceTension(99, nil); err != nil || a.Tension != 1 {
		t.Fatal(a, err)
	}
}

func TestLifetimePartyPoisonAndSuppressedPoison(t *testing.T) {
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "a", HP: 100, MaxHP: 100, Status: StatusPoison}},
		{Participant: Participant{ID: "b", HP: 5, MaxHP: 100, Status: StatusVirulentPoison}},
		{Participant: Participant{ID: "c", HP: 100, MaxHP: 100, Status: StatusPoison}},
	}}
	s = reloadLifetime(t, s)
	deaths, err := s.ApplyPartyPoison("a", []string{"a", "b", "c"}, fixedFloatRNG{val: 0.9})
	if err != nil || !reflect.DeepEqual(deaths, []string{"b"}) || s.Actors[0].HP != 100 || s.Actors[2].HP != 100 {
		t.Fatal(s, deaths, err)
	}
	s.Actors[1].ResetEffects(LifetimeReset{})
	s = reloadLifetime(t, s)
	if _, err := s.Actors[0].ApplyPoison(fixedFloatRNG{val: 0.9}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.CompleteOperation(LifetimeCompletion{Accepted: true, ActorID: "a", Suppressed: true, EnemiesRemain: true}, nil); err != nil {
		t.Fatal(err)
	}
	s = reloadLifetime(t, s)
	if s.Actors[0].HP != 90 {
		t.Fatal("suppression must not suppress normal-action poison", s)
	}
}

func TestLifetimeInvalidCompletionIsAtomic(t *testing.T) {
	for _, c := range []LifetimeCompletion{
		{Accepted: true, ActorID: "foreign", EnemiesRemain: true},
		{Accepted: true, ActorID: "a", EnemiesRemain: true}, // missing recharge outcome
		{Accepted: true, ActorID: "a", EnemiesRemain: true, CMPAmount: 2, NextCMPThreshold: 10},
		{Accepted: true, ActorID: "a", End: EndProcessing{ImmediateRiskCalls: -1}},
	} {
		s := LifetimeState{Actors: []LifetimeActor{{Participant: Participant{ID: "a", HP: 5}, CMPCharge: 3, CMPNeedCharge: 3}}, Fields: LifetimeFields{Temporary: "field", Turns: 1}}
		before := reloadLifetime(t, s)
		if _, err := s.CompleteOperation(c, nil); err == nil {
			t.Fatal("accepted invalid completion")
		}
		if !reflect.DeepEqual(s, before) {
			t.Fatal("partially mutated invalid completion", s)
		}
	}
}

func TestLifetimeZeroCounterExpiryAndSeal(t *testing.T) {
	s := LifetimeState{Actors: []LifetimeActor{{Extended: ExtendedWarGod, ExtendedTurns: 0, CMPCharge: 8, CMPNeedCharge: 3}}}
	s = reloadLifetime(t, s)
	if !s.Actors[0].FinishExtended(LifetimeTail{}) {
		t.Fatal("war-god zero expires without damage")
	}
	s.Actors[0].EnterSealField()
	s = reloadLifetime(t, s)
	if s.Actors[0].CMPCharge != 0 || s.Actors[0].CMPNeedCharge != 10 {
		t.Fatal(s)
	}
	s.Fields = LifetimeFields{Default: "default"}
	if !s.Fields.Create("default", 2) || s.Fields.Create("default", 3) {
		t.Fatal("same default vs temporary")
	}
	s.Fields.CreateAnti(1)
	if s.Fields.Create("new", 3) {
		t.Fatal("anti creation guard")
	}
}
