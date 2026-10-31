package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func TestEquipmentTailRoleAndEnclosingPredicates(t *testing.T) {
	for _, npc := range []bool{false, true} {
		for _, reason := range []string{"eligible", "rejected", "suppressed", "no enemies", "comment", "level up", "fallen"} {
			for _, id := range []string{"item-164-operation-tail", "item-206-operation-tail", "item-208-combat-tail", "item-246-combat-tail"} {
				s, c := equipmentTailFixture(npc, 3)
				a := s.actor("self")
				a.ItemDefinitionIDs, a.Temporary = []string{"item-164", "item-208", "item-246"}, TemporaryDefense
				c.Snapshot.Item206 = true
				switch reason {
				case "rejected":
					c.Completion.Accepted = false
				case "suppressed":
					c.Completion.Suppressed = true
				case "no enemies":
					c.Completion.EnemiesRemain = false
				case "comment":
					c.Completion.NPCComment = true
				case "level up":
					c.Completion.LevelUp = true
				case "fallen":
					a.HP = 0
				}
				eligible := reason != "rejected" && (npc || reason != "suppressed")
				if id == "item-208-combat-tail" || id == "item-246-combat-tail" {
					eligible = eligible && (npc || reason != "no enemies" && reason != "comment" && reason != "level up")
				}
				if id == "item-246-combat-tail" {
					eligible = eligible && reason != "fallen"
				}
				var draws []float64
				if eligible {
					switch id {
					case "item-164-operation-tail":
						draws = []float64{.25}
					case "item-206-operation-tail":
						if reason == "fallen" {
							draws = []float64{.5}
						} else {
							draws = []float64{0, .5}
						}
					case "item-246-combat-tail":
						draws = []float64{0}
					}
				}
				rng := &targetRNG{t: t, draws: draws}
				before := s.callbackCopy()
				if err := s.ApplyEquipmentTail(&c, id, rng); err != nil {
					t.Fatal(npc, reason, id, err)
				}
				if !eligible && !reflect.DeepEqual(before, s) {
					t.Fatal("ineligible mutated", npc, reason, id)
				}
				if eligible && id == "item-208-combat-tail" && a.Hit != 95 {
					t.Fatal(npc, reason, a)
				}
				rng.done()
			}
		}
	}
}

func TestEquipmentTailNPCIncapacitationSkipsNormalEffectsOnly(t *testing.T) {
	for _, cured := range []bool{false, true} {
		s, c := equipmentTailFixture(true, 1)
		a := s.actor("self")
		a.Status, a.Temporary, a.ItemDefinitionIDs = StatusSleep, TemporaryRegeneration, []string{"item-164"}
		draw := .9
		if cured {
			draw = 0
		}
		rng := &targetRNG{t: t, draws: []float64{draw, .25}}
		decision, err := a.CheckIncapacitation(IncapacitationContext{NPC: true}, rng)
		if err != nil || decision.NormalAction || decision.NeedsDefeat {
			t.Fatal(decision, err)
		}
		if err := s.ApplyEquipmentTail(&c, "item-164-operation-tail", rng); err != nil || a.HP != 100 || a.Temporary != "" || (a.Status == "") != cured {
			t.Fatal(a, err)
		}
		rng.done()
	}
}

func TestEquipmentTailBurnClearsStatusBefore206(t *testing.T) {
	for _, npc := range []bool{false, true} {
		s, c := equipmentTailFixture(npc, 1)
		s.Fields = LifetimeFields{Default: "burn"}
		a := s.actor("self")
		a.HP, a.Status, a.ItemDefinitionIDs = 50, StatusSleep, []string{"item-206"}
		a.Extended, a.ExtendedTurns = ExtendedAwakening, 2
		var err error
		c.Snapshot, err = s.CaptureEquipmentTail(c.TargetContext)
		if err != nil {
			t.Fatal(err)
		}
		field := NamedFieldContext{DamageContext: c.DamageContext}
		if err := s.ApplyNamedFieldTail(&field, NamedFieldTail{Before: "burn", NPC: npc, ResponseEnabled: true}, &targetRNG{t: t}); err != nil {
			t.Fatal(err)
		}
		rng := &targetRNG{t: t, draws: []float64{0, .5}}
		if err := s.ApplyEquipmentTail(&c, "item-206-operation-tail", rng); err != nil || a.HP != 271 || a.Status != "" || a.Extended != "" || a.ExtendedTurns != 0 || a.Tension != 3 {
			t.Fatal(a, err)
		}
		rng.done()
	}
}

func TestEquipmentTail208CureUsesBoundTargetAndCategory(t *testing.T) {
	s, c := equipmentTailFixture(false, 3)
	s.actor("self").ItemDefinitionIDs, s.actor("self").Status = []string{"item-208"}, StatusConfusion
	s.actor("ally-1").Status = StatusConfusion
	// The holder's exact-status cure can select a nonholder through confusion.
	rng := &targetRNG{t: t, draws: []float64{.3}}
	if err := s.ApplyEquipmentTail(&c, "item-208-combat-tail", rng); err != nil || s.actor("self").Status != StatusConfusion || s.actor("ally-1").Status != "" || c.CureCalls != 1 {
		t.Fatal(s, c, err)
	}
	rng.done()
}

func TestEquipmentTail206DeathReflectionAndExternalFailureRestoreTension(t *testing.T) {
	for _, fail := range []bool{false, true} {
		s, c := equipmentTailFixture(false, 1)
		c.Snapshot.Item206 = true
		c.Defeat = nativeDefeatContext(c.TargetContext)
		calls := 0
		c.Defeat.Rewards.Apply = func(*LifetimeState, TargetContext, string) error {
			calls++
			if fail {
				return errors.New("reward unavailable")
			}
			return nil
		}
		s.actor("enemy").HP = 50
		// Untyped direct damage ignores magic reflection and physical accuracy.
		s.actor("enemy").Temporary = TemporaryReflectMagic
		s.actor("self").Hit = 0
		rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0}}
		err := s.ApplyEquipmentTail(&c, "item-206-operation-tail", rng)
		if (err != nil) != fail || calls != 1 || s.actor("enemy").HP != 0 || s.actor("self").HP != 370 || s.actor("self").Tension != 3 {
			t.Fatal(s, calls, err)
		}
		rng.done()
	}
}

func TestEquipmentTailAdmissionPreservesNestedReferencesAndRequester(t *testing.T) {
	for _, id := range []string{"item-164-operation-tail", "item-206-operation-tail", "item-208-combat-tail"} {
		s, c := equipmentTailFixture(false, 4)
		a := s.actor("self")
		a.ItemDefinitionIDs, a.Temporary, a.Tension = []string{"item-164", "item-208"}, TemporaryDefense, 5
		c.Snapshot.Item206, c.Defeat = true, nativeDefeatContext(c.TargetContext)
		raw := 10
		a.Gear.WeaponAttack = &raw
		c.Defeat.Seals = &WeaponSealTable{DragonAttack: 30}
		c.Cursor = TargetCursor{Source: "item150", ActorID: "ally-1"}
		before, context := s.callbackCopy(), c
		level, seals, cursor := *c.RequesterLevel, *c.Defeat.Seals, c.Defeat.Cursor
		calls := 0
		c.Defeat.Rewards.Apply = func(*LifetimeState, TargetContext, string) error { calls++; return nil }
		if err := s.ValidateEquipmentTail(&c, id, &targetRNG{t: t}); err != nil || !reflect.DeepEqual(before, s) || !reflect.DeepEqual(context, c) || raw != 10 || *c.RequesterLevel != level || *c.Defeat.Seals != seals || c.Defeat.Cursor != cursor || calls != 0 {
			t.Fatal(id, s, c, err)
		}
		// Only the requesting player's original level admits high tension.
		if id == "item-164-operation-tail" {
			c.RequesterLevel = nil
			if err := s.ApplyEquipmentTail(&c, id, &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
				t.Fatal(err, s)
			}
		}
	}
}

func TestEquipmentTailFrozenReloadContinuesWithoutReplayAndExpiry(t *testing.T) {
	var reference LifetimeState
	for _, reload := range []bool{false, true} {
		s, c := equipmentTailFixture(false, 3)
		a := s.actor("self")
		a.ItemDefinitionIDs = []string{"item-246"}
		a.Extended, a.ExtendedTurns, a.CMPNeedCharge = ExtendedAwakening, 1, 5
		s.actor("ally-1").RosterIdentity = "guardian"
		c.Command, c.Custom = "frozen-equipment-command", true
		var err error
		c.Snapshot, err = s.CaptureEquipmentTail(c.TargetContext)
		if err != nil {
			t.Fatal(err)
		}
		rng := &targetRNG{t: t, draws: []float64{0, 0, .75}}
		if err := s.ApplyEquipmentTail(&c, "guardian-mp-tail", rng); err != nil {
			t.Fatal(err)
		}
		frozen := struct {
			State      LifetimeState
			Target     TargetContext
			Snapshot   EquipmentTailSnapshot
			Completion LifetimeCompletion
			Next       string
		}{s, c.TargetContext, c.Snapshot, c.Completion, "item-246-combat-tail"}
		if reload {
			data, err := json.Marshal(frozen)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(data, &frozen); err != nil {
				t.Fatal(err)
			}
			s = frozen.State
			c = EquipmentTailContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: frozen.Target}}, Snapshot: frozen.Snapshot, Completion: frozen.Completion}
		}
		if err := s.ApplyEquipmentTail(&c, frozen.Next, rng); err != nil {
			t.Fatal(err)
		}
		if _, err := s.CompleteOperation(c.Completion, c.PartyIDs); err != nil {
			t.Fatal(err)
		}
		rng.done()
		if s.actor("self").MP != 104 || s.actor("self").Temporary != TemporaryReflectBreath || s.actor("self").Extended != "" || c.Command != "frozen-equipment-command" || !c.Custom {
			t.Fatal(s, c)
		}
		if reload && !reflect.DeepEqual(reference, s) {
			t.Fatal(reference, s)
		}
		reference = s
	}
}

func TestEquipmentTail208SurvivalBoundAndStatefulFallback(t *testing.T) {
	for _, row := range []struct {
		hp                             int
		interception, activation, want bool
	}{
		{49, false, false, true}, {48, false, false, false},
		{1000, true, false, false}, {1000, false, true, false},
	} {
		s, c := equipmentTailFixture(false, 4)
		s.actor("self").Tension = 1
		s.actor("enemy").HP = row.hp
		if row.interception {
			s.actor("enemy").Extended = ExtendedInterception
		}
		c.Item195Activated = row.activation
		before := s.callbackCopy()
		safe, err := s.equipment208Nonlethal(&c, 4, &targetRNG{t: t})
		if err != nil || safe != row.want || !reflect.DeepEqual(before, s) {
			t.Fatal(row, safe, err, s)
		}
	}
}

func TestEquipmentTail208ContinuesAfterFinalStrikeResetsActorAttack(t *testing.T) {
	s, c := equipmentTailFixture(false, 4)
	s.actor("self").HP, s.actor("self").Attack = 1, 200
	s.actor("self").ItemDefinitionIDs = []string{"item-208"}
	s.actor("self").Gear.AccessoryID = "item-208"
	enemy := *s.actor("enemy")
	enemy.ID = "enemy-2"
	s.Actors = append(s.Actors, enemy)
	c.EnemyIDs = append(c.EnemyIDs, enemy.ID)
	s.actor("enemy").HP = 5
	s.actor("enemy").Gear = &BattleGearStats{AccessoryID: "item-212"}
	s.actor("enemy").ItemDefinitionIDs = []string{"item-212"}
	c.Defeat = nativeDefeatContext(c.TargetContext)
	rng := &targetRNG{t: t, draws: []float64{0, 0, 0, 0, .75, 0, .75, 0, .75, 0}}
	if err := s.ApplyEquipmentTail(&c, "item-208-combat-tail", rng); err != nil || s.actor("self").HP != 0 || s.actor("self").Attack != 100 || s.actor("self").Tension != 3 || s.actor("enemy").HP != 0 || s.actor("enemy-2").HP != 973 || c.DamageCalls != 4 {
		t.Fatal(s, c, err)
	}
	rng.done()
}

func TestEquipmentTail208LaterItem110DefeatRejectsBeforeCure(t *testing.T) {
	s, c := equipmentTailFixture(false, 4)
	s.actor("self").ItemDefinitionIDs, s.actor("self").Status = []string{"item-208"}, StatusSleep
	s.actor("enemy").HP = 30
	s.actor("enemy").Gear.AccessoryID, s.actor("enemy").ItemDefinitionIDs = "item-110", []string{"item-110"}
	c.Defeat = nativeDefeatContext(c.TargetContext)
	before := s.callbackCopy()
	if err := s.ApplyEquipmentTail(&c, "item-208-combat-tail", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) || c.CureCalls != 0 || c.DamageCalls != 0 {
		t.Fatal(err, s, c)
	}
}
