package battle

import (
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

type playerOperationRNG struct {
	values []float64
	draws  int
}

// Unlisted draws return .99: no cure, no random temporary expiry, no item hit.
func (r *playerOperationRNG) Float64() float64 {
	r.draws++
	if r.draws <= len(r.values) {
		return r.values[r.draws-1]
	}
	return .99
}
func (r *playerOperationRNG) Intn(n int) int { return int(r.Float64() * float64(n)) }
func (r *playerOperationRNG) IntN(n int) int { return r.Intn(n) }

func playerOperationFixture() (LifetimeState, *PlayerOperation, *int) {
	s := LifetimeState{
		Actors: []LifetimeActor{
			{Participant: Participant{ID: "a", TeamID: "p", HP: 100, MaxHP: 100, MP: 10, MaxMP: 10, CMP: 1, MaxCMP: 10}, CMPNeedCharge: 3, Tension: 1, Hit: 95},
			{Participant: Participant{ID: "b", TeamID: "p", HP: 100, MaxHP: 100}, Tension: 1, Hit: 95},
			{Participant: Participant{ID: "e", TeamID: "n", HP: 100, MaxHP: 100}, NPC: true, Tension: 1, Hit: 95},
		},
		Fields:    LifetimeFields{Temporary: "darkness", Turns: 3},
		SkillBans: map[string]int{"other": 2},
	}
	for i := range s.Actors {
		s.Actors[i].Gear = &BattleGearStats{}
	}
	level, calls := 50, 0
	c := TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}, EnemyIDs: []string{"e"}}
	op := &PlayerOperation{
		Tail:            PlayerTailContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: c, RequesterLevel: &level}}, DefeatContext: nativeDefeatContext(c)},
		ResponseEnabled: true,
		First: PlayerAction{EntryID: "attack", Validate: func(*LifetimeState) error { return nil },
			Execute: func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
				calls++
				s.actor("e").HP -= 10
				return PlayerActionFacts{DamageCalls: 1}, nil
			}},
	}
	return s, op, &calls
}

// Inert submissions consume nothing: fallen actor, banned entry, callback `$mes`.
func TestPlayerOperationUnacceptedIsInert(t *testing.T) {
	for _, reason := range []string{"fallen", "banned", "message"} {
		s, op, _ := playerOperationFixture()
		s.Actors[0].Temporary = TemporaryDefense
		switch reason {
		case "fallen":
			s.Actors[0].HP = 0
		case "banned":
			s.SkillBans["attack"] = 1
		case "message":
			op.First.Execute = func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
				s.actor("e").HP = 0
				return PlayerActionFacts{Inert: true}, nil
			}
		}
		before := reloadLifetime(t, s)
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
		if err != nil || result.Accepted || !reflect.DeepEqual(before, s) {
			t.Fatal(reason, result, err, s)
		}
	}
}

func TestPlayerOperationExecutionFailureRestoresState(t *testing.T) {
	s, op, _ := playerOperationFixture()
	op.First.Execute = func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
		s.actor("e").HP = 0
		return PlayerActionFacts{}, ErrUnsupportedEffect
	}
	before := reloadLifetime(t, s)
	if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(err, s)
	}
	s, op, calls := playerOperationFixture()
	op.First.Validate = func(*LifetimeState) error { return ErrUnsupportedEffect }
	if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); !errors.Is(err, ErrUnsupportedEffect) || *calls != 0 {
		t.Fatal(err, *calls)
	}
}

// _battle.cgi:686–707,913,1039–1053: a held paralysis consumes the operation,
// skips the callback and still reaches field ticks and the eligible tail.
func TestPlayerOperationIncapacitationReachesTail(t *testing.T) {
	s, op, calls := playerOperationFixture()
	s.Actors[0].Status = StatusParalyze
	result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9}})
	if err != nil || !result.Accepted || result.NormalAction || !result.ResponseReached || *calls != 0 {
		t.Fatal(result, err, *calls)
	}
	if s.Fields.Turns != 2 || s.Actors[0].CMPCharge != 1 || s.SkillBans["other"] != 1 || s.Actors[0].Status != StatusParalyze {
		t.Fatal(s)
	}
}

// _battle.cgi:761–785,1084–1088: late first cost, strict MP for the raw second
// cost; the counter follows the whitelisted submission, not its execution.
func TestPlayerOperationSecondSubmission(t *testing.T) {
	for _, tc := range []struct {
		mp, wantMP, wantCalls int
	}{{10, 1, 2}, {9, 6, 1}} {
		s, op, calls := playerOperationFixture()
		s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = tc.mp, ExtendedAdditional, 2
		op.First.Cost, op.First.Whitelisted = 3, true
		second := op.First
		second.Cost = 6
		op.Second = &second
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
		if err != nil || !result.Accepted || s.Actors[0].MP != tc.wantMP || *calls != tc.wantCalls || s.Actors[0].ExtendedTurns != 1 {
			t.Fatal(tc, result, err, s.Actors[0], *calls)
		}
	}
}

// _battle.cgi:792–857: other allies' virulent poison precedes the actor's own.
func TestPlayerOperationPartyPoisonPrecedesActorPoison(t *testing.T) {
	s, op, _ := playerOperationFixture()
	s.Actors[0].Status, s.Actors[1].Status = StatusPoison, StatusVirulentPoison
	s.Actors[1].HP = 5
	var order []string
	op.Tail.DefeatContext.Rewards.Apply = func(s *LifetimeState, _ TargetContext, id string) error {
		order = append(order, id)
		return nil
	}
	if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(order, []string{"b"}) || s.Actors[1].HP != 0 || s.Actors[0].HP != 90 || s.Actors[0].Status != StatusPoison {
		t.Fatal(order, s.Actors[:2])
	}
}

func TestPlayerOperationTailBarriers(t *testing.T) {
	for _, reason := range []string{"suppressed", "levelup", "comment", "no_enemy", "last_enemy_fell"} {
		s, op, _ := playerOperationFixture()
		tail := reason == "last_enemy_fell"
		switch reason {
		case "suppressed":
			op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
				return PlayerActionFacts{Suppressed: true}, nil
			}
		case "levelup":
			op.LevelUp = func(*LifetimeState, random.Generator) (bool, error) { return true, nil }
		case "comment":
			op.NPCComment = func() bool { return true }
		case "no_enemy":
			s.Actors[2].HP = 0
			op.Tail.EnemyIDs, op.Tail.MemberOnlyIDs = nil, []string{"e"}
			s.Fields.AntiTurns = 2
		case "last_enemy_fell":
			s.Actors[2].HP = 10
		}
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
		if err != nil || !result.Accepted || result.ResponseReached != tail || result.Suppressed != (reason == "suppressed") {
			t.Fatal(reason, result, err)
		}
		if (s.Actors[0].CMPCharge == 1) != tail || (s.SkillBans["other"] == 1) != tail {
			t.Fatal(reason, s)
		}
		// Field ticks precede the tail; suppression skips them, anti ticks alone.
		wantTurns, wantAnti := 2, 0
		switch reason {
		case "suppressed":
			wantTurns = 3
		case "no_enemy":
			wantTurns, wantAnti = 3, 1
		}
		if s.Fields.Turns != wantTurns || s.Fields.AntiTurns != wantAnti {
			t.Fatal(reason, s.Fields)
		}
	}
}

// _battle.cgi:578–583,1155–1161: an ally fallen with a death mark at entry and
// revived this operation is defeated again at the end of the eligible tail.
func TestPlayerOperationDeathMark(t *testing.T) {
	s, op, _ := playerOperationFixture()
	s.Actors[1].HP, s.Actors[1].Temporary = 0, TemporaryEffect("death_mark")
	op.First.Execute = func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
		s.actor("b").HP = 30
		return PlayerActionFacts{}, nil
	}
	if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil || s.Actors[1].HP != 0 || s.Actors[1].Temporary != "" {
		t.Fatal(err, s.Actors[1])
	}
}

func TestPlayerOperationReloadMatchesUninterrupted(t *testing.T) {
	var finals []LifetimeState
	for _, reload := range []bool{false, true} {
		s, op, _ := playerOperationFixture()
		s.Actors[0].Extended, s.Actors[0].ExtendedTurns = ExtendedMagicSword, 3
		s.Actors[0].Temporary = TemporaryRegeneration
		rng := &playerOperationRNG{values: []float64{.1, .5}}
		for i := 0; i < 3; i++ {
			if reload {
				s = reloadLifetime(t, s)
			}
			if _, err := s.ApplyPlayerOperation(op, rng); err != nil {
				t.Fatal(err)
			}
		}
		finals = append(finals, s)
	}
	if !reflect.DeepEqual(finals[0], finals[1]) || finals[0].Actors[0].ExtendedTurns != 0 || finals[0].Actors[2].HP != 70 {
		t.Fatal(finals)
	}
}

// _battle.cgi:708–720: the curse branch can defeat the actor before the
// callback; the accepted operation still reaches the tail with no self reward.
func TestPlayerOperationCurseDefeatSkipsCallback(t *testing.T) {
	s, op, calls := playerOperationFixture()
	s.Actors[0].Status, s.Actors[0].HP = StatusKinju, 5
	rewarded := false
	op.Tail.DefeatContext.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { rewarded = true; return nil }
	result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.1}})
	if err != nil || !result.Accepted || result.NormalAction || *calls != 0 || rewarded || s.Actors[0].HP != 0 || s.Actors[0].CMPCharge != 1 {
		t.Fatal(result, err, *calls, rewarded, s.Actors[0])
	}
}

func TestPlayerOperationMirrorSharePreservesReflection(t *testing.T) {
	for _, share := range []bool{false, true} {
		s, op, _ := playerOperationFixture()
		s.Actors[0].Temporary = TemporaryReflectMagic
		op.First.MirrorShare = share
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil || (s.Actors[0].Temporary == TemporaryReflectMagic) != share {
			t.Fatal(share, err, s.Actors[0].Temporary)
		}
	}
}

// _battle.cgi:1164: the queued darkness debit flushes only in the eligible tail.
func TestPlayerOperationFlushesEndProcessingInTail(t *testing.T) {
	for _, suppressed := range []bool{false, true} {
		s, op, _ := playerOperationFixture()
		op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
			debit := 40
			return PlayerActionFacts{Suppressed: suppressed, End: EndProcessing{DarknessDebit: &debit}}, nil
		}
		want := 60
		if suppressed {
			want = 100
		}
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil || s.Actors[0].HP != want {
			t.Fatal(suppressed, err, s.Actors[0].HP)
		}
	}
}

// The step works on a copy: a reused op never inherits earlier facts or the
// defeat cursor, and a missing defeat owner rejects before any mutation.
func TestPlayerOperationLeavesCallerContextUnchanged(t *testing.T) {
	s, op, _ := playerOperationFixture()
	first := true
	op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
		facts := PlayerActionFacts{DamageCalls: 1}
		if first {
			debit := 40
			facts.End.DarknessDebit = &debit
		}
		first = false
		return facts, nil
	}
	tail, defeat := op.Tail, *op.Tail.DefeatContext
	for i := 0; i < 2; i++ {
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil {
			t.Fatal(err)
		}
	}
	if s.Actors[0].HP != 60 || op.Tail.DamageCalls != 0 || op.Tail.End.DarknessDebit != nil || !reflect.DeepEqual(op.Tail.Completion, tail.Completion) || !reflect.DeepEqual(*op.Tail.DefeatContext, defeat) {
		t.Fatal(s.Actors[0].HP, op.Tail)
	}
	s, op, calls := playerOperationFixture()
	op.Tail.DefeatContext = nil
	before := reloadLifetime(t, s)
	if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); !errors.Is(err, ErrInvalidRequest) || *calls != 0 || !reflect.DeepEqual(before, s) {
		t.Fatal(err, *calls)
	}
}
