package battle

import (
	"reflect"
	"testing"
)

func TestLifetimeOperationAdmissionAndTails(t *testing.T) {
	for _, reason := range []string{"ineligible", "rejected", "get", "replay", "suppressed", "no_enemy", "npc_comment", "levelup", "incapacitated", "npc"} {
		t.Run(reason, func(t *testing.T) {
			s := LifetimeState{Actors: []LifetimeActor{{Participant: Participant{ID: "actor", HP: 100, CMP: 1, MaxCMP: 10}, CMPNeedCharge: 3, Extended: ExtendedAwakening, ExtendedTurns: 2}}, Fields: LifetimeFields{Temporary: "field", Turns: 2}, SkillBans: map[string]int{"skill": 2}}
			s = reloadLifetime(t, s)
			before := reloadLifetime(t, s)
			c := LifetimeCompletion{ActorID: "actor", Accepted: true, EnemiesRemain: true}
			switch reason {
			case "ineligible", "rejected", "get", "replay":
				c.Accepted = false
			case "suppressed":
				c.Suppressed = true
			case "no_enemy":
				c.EnemiesRemain = false
			case "npc_comment":
				c.NPCComment = true
			case "levelup":
				c.LevelUp = true
			case "npc":
				c.NPC = true
			}
			if _, err := s.CompleteOperation(c, nil); err != nil {
				t.Fatal(err)
			}
			s = reloadLifetime(t, s)
			if !c.Accepted {
				if !reflect.DeepEqual(before, s) {
					t.Fatal("unaccepted command mutated state")
				}
				return
			}
			fieldTick := reason != "suppressed" && reason != "no_enemy" && reason != "npc"
			if (s.Fields.Turns == 1) != fieldTick {
				t.Fatalf("field: %+v", s.Fields)
			}
			playerTail := reason == "incapacitated"
			if (s.Actors[0].ExtendedTurns == 1) != playerTail || (s.SkillBans["skill"] == 1) != playerTail || (s.Actors[0].CMPCharge == 1) != playerTail {
				t.Fatal(s)
			}
		})
	}
}

func TestLifetimeStatusPlayerNPC(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, status := range []string{StatusDofuu, StatusParalyze, StatusSleep, StatusSabaku, StatusKinju, StatusConfusion} {
			s := LifetimeState{Actors: []LifetimeActor{{Participant: Participant{ID: "actor", HP: 100, MaxHP: 100, Status: status}, Temporary: TemporaryDefense}}}
			s = reloadLifetime(t, s)
			decision, err := s.Actors[0].CheckIncapacitation(IncapacitationContext{NPC: npc}, fixedFloatRNG{val: 0})
			if err != nil {
				t.Fatal(err)
			}
			s = reloadLifetime(t, s)
			if decision.NormalAction != (status == StatusConfusion) || s.Actors[0].Status != "" || s.Actors[0].Temporary != TemporaryDefense {
				t.Fatalf("%s npc=%v: %+v", status, npc, decision)
			}
		}
		a := LifetimeActor{Participant: Participant{Status: StatusConfusion}}
		if _, err := a.CheckIncapacitation(IncapacitationContext{NPC: npc}, fixedFloatRNG{val: 0.25}); err != nil {
			t.Fatal(err)
		}
		if (a.Status == "") != npc {
			t.Fatal("player/NPC confusion probability")
		}
	}
	a := LifetimeActor{Participant: Participant{Status: StatusSleep}}
	decision, err := a.CheckIncapacitation(IncapacitationContext{Bypass: true}, fixedFloatRNG{val: 0})
	if err != nil || !decision.NormalAction || a.Status != StatusSleep {
		t.Fatal("bypass", decision, err)
	}
}

func TestLifetimeTemporaryResetAndAdvance(t *testing.T) {
	for _, effect := range []TemporaryEffect{TemporaryDefense, TemporaryStrongDefense, TemporaryOneHitDefense, TemporaryReflectMagic, TemporaryCover} {
		for _, npc := range []bool{false, true} {
			s := LifetimeState{Actors: []LifetimeActor{{Temporary: effect}}}
			s = reloadLifetime(t, s)
			if err := s.Actors[0].BeginNormalAction(npc, true, true, fixedFloatRNG{val: 0.9}); err != nil {
				t.Fatal(err)
			}
			s = reloadLifetime(t, s)
			preserved := effect == TemporaryCover || (effect == TemporaryReflectMagic && !npc)
			if (s.Actors[0].Temporary != "") != preserved {
				t.Fatalf("effect=%s npc=%v", effect, npc)
			}
		}
	}
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "a", TeamID: "ally", HP: 20, MaxHP: 100, MP: 3, CMP: 2, ItemDefinitionIDs: []string{"item-188"}, ActionItems: []ActionItem{{ID: "item-001", InstanceID: "instance"}}}, Tension: 8, Hit: 4, Extended: ExtendedFrenzy, ExtendedTurns: 1},
		{Participant: Participant{ID: "b", TeamID: "enemy", HP: 5, MaxHP: 100, MP: 1, Status: StatusVirulentPoison}, Temporary: TemporaryReflectMagic},
		{Participant: Participant{ID: "c", TeamID: "enemy"}, Temporary: TemporaryCovering},
	}, Fields: LifetimeFields{Temporary: "temp", Turns: 3, AntiTurns: 1, Default: "default"}}
	s = reloadLifetime(t, s)
	target, err := s.TemporaryTarget("a", "b", "magic")
	if err != nil || target != "a" || s.Actors[1].Temporary != TemporaryReflectMagic {
		t.Fatal(target, err)
	}
	s = reloadLifetime(t, s)
	s.Actors[1].Temporary = TemporaryCover
	s = reloadLifetime(t, s)
	target, err = s.TemporaryTarget("a", "b", "magic")
	if err != nil || target != "c" {
		t.Fatal(target, err)
	}
	s = reloadLifetime(t, s)
	s.Actors[1].Temporary = TemporaryOneHitDefense
	s = reloadLifetime(t, s)
	if !s.Actors[1].ConsumeOneHitDefense() || s.Actors[1].ConsumeOneHitDefense() {
		t.Fatal("one-hit defense")
	}
	s = reloadLifetime(t, s)
	dead, err := s.Actors[1].ApplyPoison(fixedFloatRNG{val: 0})
	if err != nil || !dead {
		t.Fatal(dead, err)
	}
	s.Actors[1].ResetEffects(LifetimeReset{Attack: 30})
	s = reloadLifetime(t, s)
	if s.Actors[1].HP != 0 || s.Actors[1].MP != 1 || s.Actors[1].Tension != 1 {
		t.Fatal(s)
	}
	// First living holder revives only its side; c stays fallen.
	s.Actors[1].TeamID = "ally"
	s = reloadLifetime(t, s)
	if err := s.ReviveOnAdvance([]string{"a", "b"}, fixedFloatRNG{val: 0}); err != nil {
		t.Fatal(err)
	}
	s = reloadLifetime(t, s)
	if s.Actors[1].HP != 50 || s.Actors[1].MP != 1 || s.Actors[2].HP != 0 {
		t.Fatal(s)
	}
	s.Actors[0].ResetEffects(LifetimeReset{Attack: 120, Defense: 55, Agility: -5, Temporary: TemporaryRevival})
	s.Fields.ResetFloor()
	s = reloadLifetime(t, s)
	a := s.Actors[0]
	if a.Tension != 1 || a.Hit != 95 || a.Extended != "" || a.ExtendedTurns != 0 || a.Attack != 120 || a.Agility != 0 || a.Temporary != TemporaryRevival || s.Fields.Current() != "default" || s.Fields.AntiTurns != 0 {
		t.Fatal(s)
	}
	if a.HP != 20 || a.MP != 3 || a.CMP != 2 || len(a.ActionItems) != 1 || a.ActionItems[0].InstanceID != "instance" {
		t.Fatal("reset restored assets", a)
	}
	a = LifetimeActor{Participant: Participant{HP: 2, MaxHP: 9, Status: StatusPoison}}
	if dead, err := a.ApplyPoison(fixedFloatRNG{val: 0.9}); err != nil || dead || a.HP != 2 {
		t.Fatal(a, dead, err)
	}
}

func TestLifetimeIncenseAndEndQueue(t *testing.T) {
	for _, risk := range []int{0, 1} {
		for _, npc := range []bool{false, true} {
			s := LifetimeState{Actors: []LifetimeActor{
				{Participant: Participant{ID: "a", HP: 100}, CMPNeedCharge: 3, Extended: ExtendedIncense, ExtendedTurns: 1},
				{Participant: Participant{ID: "b", HP: 0}, Extended: ExtendedIncense, ExtendedTurns: 2},
			}}
			end := EndProcessing{ImmediateRiskCalls: risk}
			end.RecordDarkness(30)
			end.RecordDarkness(1000)
			s = reloadLifetime(t, s)
			result, err := s.CompleteOperation(LifetimeCompletion{Accepted: true, ActorID: "a", NPC: npc, EnemiesRemain: true, End: end}, []string{"a", "b"})
			if err != nil {
				t.Fatal(err)
			}
			s = reloadLifetime(t, s)
			wantHP := 100
			if risk == 0 {
				wantHP = 85
			}
			if s.Actors[0].HP != wantHP {
				t.Fatal(s.Actors[0])
			}
			if npc {
				if result.IncenseHolder != "" || s.Actors[0].ExtendedTurns != 1 {
					t.Fatal(result, s)
				}
			} else if result.IncenseHolder != "a" || s.Actors[0].Extended != "" || s.Actors[1].ExtendedTurns != 1 {
				t.Fatal(result, s)
			}
		}
	}
	a := LifetimeActor{Participant: Participant{HP: 5}}
	end := EndProcessing{ImmediateRiskCalls: 1, SkipPrefilter: true}
	end.RecordDarkness(2000)
	end.Flush(&a)
	if a.HP != 1 {
		t.Fatal(a)
	}
	a.HP = 0
	end.Flush(&a)
	if a.HP != 0 {
		t.Fatal("revived fallen actor")
	}
}

func TestLifetimeResumedSequence(t *testing.T) {
	initial := LifetimeState{Actors: []LifetimeActor{{Participant: Participant{ID: "a", HP: 100, MaxHP: 100, CMP: 1, MaxCMP: 10}, CMPNeedCharge: 2, Tension: 1, Hit: 95, Extended: ExtendedMagicSword, ExtendedTurns: 2}}, Fields: LifetimeFields{Temporary: "field", Turns: 3, Default: "default"}, SkillBans: map[string]int{"ban": 2}}
	run := func(reload bool) LifetimeState {
		s := reloadLifetime(t, initial)
		for _, damageCalls := range []int{0, 1, 1, 0, 0, 0, 0} {
			if reload {
				s = reloadLifetime(t, s)
			}
			_, err := s.CompleteOperation(LifetimeCompletion{Accepted: true, ActorID: "a", EnemiesRemain: true, Tail: LifetimeTail{DamageCalls: damageCalls}, CMPAmount: 3, NextCMPThreshold: 3}, nil)
			if err != nil {
				t.Fatal(err)
			}
			if reload {
				s = reloadLifetime(t, s)
			}
		}
		return s
	}
	if uninterrupted, resumed := run(false), run(true); !reflect.DeepEqual(uninterrupted, resumed) {
		t.Fatalf("%+v != %+v", uninterrupted, resumed)
	}
}
