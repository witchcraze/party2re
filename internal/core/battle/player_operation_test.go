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

func TestPlayerOperationSecondValidationAvoidedPaths(t *testing.T) {
	for _, tc := range []struct {
		name         string
		setup        func(*LifetimeState, *PlayerOperation, *int)
		wantAccepted bool
		wantMP       int
		wantCalls    int
		wantExtTurns int
	}{
		{
			name: "strict_mp_boundary",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 9, ExtendedAdditional, 2
				op.First.Cost, op.First.Whitelisted = 3, true
				op.Second.Cost, op.Second.Whitelisted = 6, true
			},
			wantAccepted: true, wantMP: 6, wantCalls: 1, wantExtTurns: 1,
		},
		{
			name: "missing_extended_additional",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP = 10
				op.First.Cost, op.First.Whitelisted = 3, true
				op.Second.Cost, op.Second.Whitelisted = 6, true
			},
			wantAccepted: true, wantMP: 7, wantCalls: 1, wantExtTurns: 0,
		},
		{
			name: "unlisted_second_entry",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
				op.First.Cost, op.First.Whitelisted = 3, true
				op.Second.Cost, op.Second.Whitelisted = 6, false
			},
			wantAccepted: true, wantMP: 7, wantCalls: 1, wantExtTurns: 2,
		},
		{
			name: "unlisted_first_with_counter_only_second",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
				op.First.Cost, op.First.Whitelisted = 3, false
				op.Second = &PlayerAction{Whitelisted: true}
			},
			wantAccepted: true, wantMP: 7, wantCalls: 1, wantExtTurns: 1,
		},
		{
			name: "skipped_normal_action_paralyze",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP, s.Actors[0].Status, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, StatusParalyze, ExtendedAdditional, 2
				op.First.Cost, op.First.Whitelisted = 3, true
				op.Second.Cost, op.Second.Whitelisted = 6, true
			},
			wantAccepted: true, wantMP: 10, wantCalls: 0, wantExtTurns: 1,
		},
		{
			name: "inert_first_message",
			setup: func(s *LifetimeState, op *PlayerOperation, _ *int) {
				s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
				op.First.Cost, op.First.Whitelisted = 3, true
				op.First.Execute = func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
					s.actor("e").HP = 0
					return PlayerActionFacts{Inert: true}, nil
				}
				op.Second.Cost, op.Second.Whitelisted = 6, true
			},
			wantAccepted: false, wantMP: 10, wantCalls: 0, wantExtTurns: 2,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, op, calls := playerOperationFixture()
			secondValCalls := 0
			second := op.First
			second.Validate = func(*LifetimeState) error {
				secondValCalls++
				return ErrUnsupportedEffect
			}
			op.Second = &second
			tc.setup(&s, op, &secondValCalls)
			result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9}})
			if err != nil || result.Accepted != tc.wantAccepted || secondValCalls != 0 ||
				s.Actors[0].MP != tc.wantMP || *calls != tc.wantCalls || s.Actors[0].ExtendedTurns != tc.wantExtTurns {
				t.Fatalf("result=%+v err=%v secondValCalls=%d mp=%d calls=%d extTurns=%d",
					result, err, secondValCalls, s.Actors[0].MP, *calls, s.Actors[0].ExtendedTurns)
			}
		})
	}
}

func TestPlayerOperationSecondValidationPostFirstState(t *testing.T) {
	t.Run("newly_supported", func(t *testing.T) {
		for _, tc := range []struct {
			mp, wantMP, wantCalls, wantSecondVal int
		}{{9, 6, 1, 0}, {10, 1, 2, 1}} {
			s, op, calls := playerOperationFixture()
			s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = tc.mp, ExtendedAdditional, 2
			op.First.Cost, op.First.Whitelisted = 3, true
			secondValCalls := 0
			second := op.First
			second.Cost = 6
			second.Validate = func(s *LifetimeState) error {
				secondValCalls++
				if s.actor("e").HP != 90 || s.actor("a").MP != 7 {
					return ErrUnsupportedEffect
				}
				return nil
			}
			op.Second = &second
			result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
			if err != nil || !result.Accepted || s.Actors[0].MP != tc.wantMP || *calls != tc.wantCalls ||
				secondValCalls != tc.wantSecondVal || s.Actors[0].ExtendedTurns != 1 {
				t.Fatal(tc, result, err, s.Actors[0], *calls, secondValCalls)
			}
		}
	})

	t.Run("newly_unsupported_restores_first", func(t *testing.T) {
		s, op, calls := playerOperationFixture()
		s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
		op.First.Cost, op.First.Whitelisted = 3, true
		second := op.First
		second.Cost = 6
		second.Validate = func(s *LifetimeState) error {
			if s.actor("e").HP != 100 {
				return ErrUnsupportedEffect
			}
			return nil
		}
		op.Second = &second
		before := reloadLifetime(t, s)
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
		if !errors.Is(err, ErrUnsupportedEffect) || result.Accepted || *calls != 1 || !reflect.DeepEqual(before, s) {
			t.Fatal(result, err, *calls, s)
		}

		// At the strict MP boundary, second validation is avoided and first completes.
		s.Actors[0].MP = 9
		*calls = 0
		result, err = s.ApplyPlayerOperation(op, &playerOperationRNG{})
		if err != nil || !result.Accepted || *calls != 1 || s.Actors[0].MP != 6 || s.actor("e").HP != 90 || s.Actors[0].ExtendedTurns != 1 {
			t.Fatal(result, err, *calls, s)
		}
	})
}

func TestPlayerOperationCertifiesReachableSecondAndRestoresExecutionRejection(t *testing.T) {
	newCatalog := func(includeChild bool) CallbackCatalog {
		d := CallbackCatalog{Entries: map[string]CallbackDefinition{
			"wrapper": {
				Children: []CallbackInvocation{{Kind: "entry", EntryID: "child"}},
				Validate: func(*LifetimeState, *CallbackContext) error { return nil },
				Execute: func(_ *LifetimeState, c *CallbackContext, _ random.Generator) error {
					return c.Invoke(CallbackInvocation{Kind: "entry", EntryID: "child"})
				},
			},
		}}
		if includeChild {
			d.Entries["child"] = CallbackDefinition{
				Validate: func(s *LifetimeState, _ *CallbackContext) error {
					if s.actor("e").HP != 100 {
						return ErrUnsupportedEffect
					}
					return nil
				},
				Execute: func(s *LifetimeState, _ *CallbackContext, _ random.Generator) error {
					s.actor("e").HP -= 20
					return nil
				},
			}
		}
		return d
	}

	// Unregistered reachable descendant rejects at admission even at MP=9 or paralysis.
	for _, tc := range []struct {
		name   string
		mp     int
		status string
	}{{"strict_mp_boundary", 9, ""}, {"paralyzed", 10, StatusParalyze}} {
		t.Run(tc.name, func(t *testing.T) {
			s, op, calls := playerOperationFixture()
			s.Actors[0].MP, s.Actors[0].Status, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = tc.mp, tc.status, ExtendedAdditional, 2
			op.First.Cost, op.First.Whitelisted = 3, true
			d := newCatalog(false)
			f := CallbackContext{TargetContext: op.Tail.TargetContext}
			inv := []CallbackInvocation{{Kind: "entry", EntryID: "wrapper"}}
			op.Second = &PlayerAction{
				EntryID: "wrapper", Cost: 6, Whitelisted: true,
				Certify:  func(s *LifetimeState) error { return s.CertifyInvocations(&f, d, inv, &playerOperationRNG{}) },
				Validate: func(s *LifetimeState) error { return s.ValidateInvocations(&f, d, inv, &playerOperationRNG{}) },
				Execute: func(s *LifetimeState, rng random.Generator) (PlayerActionFacts, error) {
					return PlayerActionFacts{}, s.ApplyInvocations(&f, d, inv, rng)
				},
			}
			before := reloadLifetime(t, s)
			rng := &playerOperationRNG{values: []float64{.9}}
			if _, err := s.ApplyPlayerOperation(op, rng); !errors.Is(err, ErrUnsupportedEffect) || *calls != 0 || rng.draws != 0 || !reflect.DeepEqual(before, s) {
				t.Fatalf("calls=%d draws=%d err=%v state=%+v", *calls, rng.draws, err, s)
			}
		})
	}

	t.Run("certified_second_skips_at_mp9_and_restores_on_post_first_reject", func(t *testing.T) {
		d := newCatalog(true)
		inv := []CallbackInvocation{{Kind: "entry", EntryID: "wrapper"}}
		bindSecond := func(op *PlayerOperation) {
			f := CallbackContext{TargetContext: op.Tail.TargetContext}
			op.Second = &PlayerAction{
				EntryID: "wrapper", Cost: 6, Whitelisted: true,
				Certify:  func(s *LifetimeState) error { return s.CertifyInvocations(&f, d, inv, &playerOperationRNG{}) },
				Validate: func(s *LifetimeState) error { return s.ValidateInvocations(&f, d, inv, &playerOperationRNG{}) },
				Execute: func(s *LifetimeState, rng random.Generator) (PlayerActionFacts, error) {
					return PlayerActionFacts{}, s.ApplyInvocations(&f, d, inv, rng)
				},
			}
		}

		// MP 9: certification passes without entry state validation; first runs and second is skipped.
		s, op, calls := playerOperationFixture()
		s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 9, ExtendedAdditional, 2
		op.First.Cost, op.First.Whitelisted = 3, true
		bindSecond(op)
		if res, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); err != nil || !res.Accepted || *calls != 1 || s.Actors[0].MP != 6 || s.actor("e").HP != 90 || s.Actors[0].ExtendedTurns != 1 {
			t.Fatal(res, err, *calls, s)
		}

		// MP 10: certification passes, first reduces HP to 90, second rejects post-first state and restores all effects.
		s, op, calls = playerOperationFixture()
		s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
		op.First.Cost, op.First.Whitelisted = 3, true
		bindSecond(op)
		before := reloadLifetime(t, s)
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); !errors.Is(err, ErrUnsupportedEffect) || *calls != 1 || !reflect.DeepEqual(before, s) {
			t.Fatal(err, *calls, s)
		}
	})

	t.Run("second_execute_error_restores_first", func(t *testing.T) {
		s, op, calls := playerOperationFixture()
		s.Actors[0].MP, s.Actors[0].Extended, s.Actors[0].ExtendedTurns = 10, ExtendedAdditional, 2
		op.First.Cost, op.First.Whitelisted = 3, true
		op.Second = &PlayerAction{
			EntryID: "second", Cost: 6, Whitelisted: true,
			Validate: func(*LifetimeState) error { return nil },
			Execute: func(s *LifetimeState, _ random.Generator) (PlayerActionFacts, error) {
				s.actor("e").HP = 1
				return PlayerActionFacts{}, ErrUnsupportedEffect
			},
		}
		before := reloadLifetime(t, s)
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{}); !errors.Is(err, ErrUnsupportedEffect) || *calls != 1 || !reflect.DeepEqual(before, s) {
			t.Fatal(err, *calls, s)
		}
	})

	t.Run("first_post_cure_validation_restores_state", func(t *testing.T) {
		s, op, calls := playerOperationFixture()
		s.Actors[0].Status = StatusConfusion
		op.First.Validate = func(s *LifetimeState) error {
			if s.actor("a").Status == "" {
				return ErrUnsupportedEffect
			}
			return nil
		}
		before := reloadLifetime(t, s)
		// Draw 0.1 cures confusion in CheckIncapacitation; post-cure validation rejects and restores state.
		if _, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.1}}); !errors.Is(err, ErrUnsupportedEffect) || *calls != 0 || !reflect.DeepEqual(before, s) {
			t.Fatal(err, *calls, s)
		}
	})
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
	for _, reason := range []string{"suppressed", "levelup", "comment", "no_enemy", "last_enemy_fell", "response_disabled"} {
		s, op, _ := playerOperationFixture()
		tail := reason == "last_enemy_fell" || reason == "response_disabled"
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
		case "response_disabled":
			op.ResponseEnabled = false
		}
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
		// The tail runs without the response flag; only the NPC response needs it.
		if err != nil || !result.Accepted || result.ResponseReached != (tail && op.ResponseEnabled) || result.Suppressed != (reason == "suppressed") {
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

func TestPlayerOperationCarriesGuaranteedHitPerOperation(t *testing.T) {
	s, op, _ := playerOperationFixture()
	hit := true
	op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
		return PlayerActionFacts{GuaranteedHit: hit}, nil
	}
	res, err := s.ApplyPlayerOperation(op, &playerOperationRNG{})
	if err != nil || !res.Accepted || !res.GuaranteedHit || op.Tail.GuaranteedHit {
		t.Fatalf("first result=%+v op.Tail.GuaranteedHit=%v err=%v", res, op.Tail.GuaranteedHit, err)
	}
	hit = false
	res, err = s.ApplyPlayerOperation(op, &playerOperationRNG{})
	if err != nil || !res.Accepted || res.GuaranteedHit {
		t.Fatalf("second result=%+v err=%v", res, err)
	}
}

// _battle.cgi:1039–1069: CMP recharge or charge increment applies before
// frenzy, crown and incense healing, once per eligible tail.
func TestPlayerOperationAppliesCMPRechargeBeforeFrenzy(t *testing.T) {
	s, op, _ := playerOperationFixture()
	s.Actors[0].CMP, s.Actors[0].CMPCharge, s.Actors[0].CMPNeedCharge = 1, 3, 3
	s.Actors[0].BaseStats.Attack, s.Actors[0].Attack = 100, 100
	s.Actors[0].Extended, s.Actors[0].ExtendedTurns = ExtendedFrenzy, 2
	s.Actors[2].HP = 1
	op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
		return PlayerActionFacts{}, nil
	}
	var seen []struct {
		target            string
		cmp, charge, need int
	}
	op.Tail.DefeatContext.Rewards.Apply = func(s *LifetimeState, _ TargetContext, id string) error {
		a := s.actor("a")
		seen = append(seen, struct {
			target            string
			cmp, charge, need int
		}{id, a.CMP, a.CMPCharge, a.CMPNeedCharge})
		return nil
	}
	rng := &playerOperationRNG{values: []float64{.9, 0, 0, .9, 0, 0, .9, .9}}
	result, err := s.ApplyPlayerOperation(op, rng)
	if err != nil || !result.Accepted || !result.CMPRecharged || !result.ResponseReached {
		t.Fatal(result, err)
	}
	if len(seen) != 1 || seen[0].target != "e" || seen[0].cmp != 2 || seen[0].charge != 0 || seen[0].need != 3 {
		t.Fatal(seen)
	}
	a := s.Actors[0]
	if a.CMP != 2 || a.CMPCharge != 0 || a.CMPNeedCharge != 3 || a.ExtendedTurns != 1 || s.Actors[2].HP != 0 {
		t.Fatal(a, s.Actors[2])
	}
}

func TestPlayerOperationCMPChargeSingleIncrementClampAndRestore(t *testing.T) {
	t.Run("no_recharge_before_frenzy", func(t *testing.T) {
		s, op, _ := playerOperationFixture()
		s.Actors[0].CMP, s.Actors[0].CMPCharge, s.Actors[0].CMPNeedCharge = 1, 2, 3
		s.Actors[0].BaseStats.Attack, s.Actors[0].Attack = 100, 100
		s.Actors[0].Extended, s.Actors[0].ExtendedTurns = ExtendedFrenzy, 2
		s.Actors[2].HP = 1
		op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
			return PlayerActionFacts{}, nil
		}
		var seenCharge, seenCMP int
		op.Tail.DefeatContext.Rewards.Apply = func(s *LifetimeState, _ TargetContext, _ string) error {
			a := s.actor("a")
			seenCharge, seenCMP = a.CMPCharge, a.CMP
			return nil
		}
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9, .9, 0, 0, .9, .9}})
		if err != nil || !result.Accepted || result.CMPRecharged || !result.ResponseReached {
			t.Fatal(result, err)
		}
		if seenCMP != 1 || seenCharge != 3 || s.Actors[0].CMP != 1 || s.Actors[0].CMPCharge != 3 || s.Actors[0].CMPNeedCharge != 3 {
			t.Fatal(seenCMP, seenCharge, s.Actors[0])
		}
	})

	t.Run("clamp", func(t *testing.T) {
		s, op, _ := playerOperationFixture()
		s.Actors[0].CMP, s.Actors[0].MaxCMP, s.Actors[0].CMPCharge, s.Actors[0].CMPNeedCharge = 9, 10, 3, 3
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9, .9, .9}})
		if err != nil || !result.Accepted || !result.CMPRecharged || !result.ResponseReached {
			t.Fatal(result, err)
		}
		if s.Actors[0].CMP != 10 || s.Actors[0].CMPCharge != 0 || s.Actors[0].CMPNeedCharge != 9 {
			t.Fatal(s.Actors[0])
		}
	})

	t.Run("ineligible_tail", func(t *testing.T) {
		s, op, _ := playerOperationFixture()
		s.Actors[0].CMP, s.Actors[0].CMPCharge, s.Actors[0].CMPNeedCharge = 1, 3, 3
		op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
			return PlayerActionFacts{Suppressed: true}, nil
		}
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9}})
		if err != nil || !result.Accepted || !result.Suppressed || result.CMPRecharged || result.ResponseReached {
			t.Fatal(result, err)
		}
		if s.Actors[0].CMP != 1 || s.Actors[0].CMPCharge != 3 || s.Actors[0].CMPNeedCharge != 3 {
			t.Fatal(s.Actors[0])
		}
	})

	t.Run("failed_frenzy_restores_cmp", func(t *testing.T) {
		s, op, _ := playerOperationFixture()
		s.Actors[0].CMP, s.Actors[0].CMPCharge, s.Actors[0].CMPNeedCharge = 1, 3, 3
		s.Actors[0].BaseStats.Attack, s.Actors[0].Attack = 100, 100
		s.Actors[0].Extended, s.Actors[0].ExtendedTurns = ExtendedFrenzy, 2
		s.Actors[2].HP = 1
		op.First.Execute = func(*LifetimeState, random.Generator) (PlayerActionFacts, error) {
			return PlayerActionFacts{}, nil
		}
		op.Tail.DefeatContext.Rewards.Apply = func(*LifetimeState, TargetContext, string) error {
			return ErrUnsupportedEffect
		}
		before := reloadLifetime(t, s)
		result, err := s.ApplyPlayerOperation(op, &playerOperationRNG{values: []float64{.9, 0, 0, .9, 0, 0, .9, .9}})
		if !errors.Is(err, ErrUnsupportedEffect) || result.Accepted || result.CMPRecharged || result.ResponseReached || !reflect.DeepEqual(before, s) {
			t.Fatal(result, err, s)
		}
	})
}
