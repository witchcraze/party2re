package battle_test

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestPartyBattle_DefendMitigation(t *testing.T) {
	engine := corebattle.Engine{}

	// Enemy with defense stance takes 50% damage
	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 50, 10).
				WithAgility(100).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("shield_guard").
				WithName("ShieldGuard").
				WithStats(200, 10, 10).
				WithAgility(10).
				WithDefending(true).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Normal damage: 50 - 10 = 40. With defending: 40 / 2 = 20.
	if len(res.Logs) > 0 {
		firstAtk := res.Logs[0]
		if firstAtk.DamageDealt != 20 {
			t.Errorf("expected 20 damage due to 50%% defense mitigation, got %d", firstAtk.DamageDealt)
		}
	}
}

func TestPartyBattle_JobSkill_AllAlliesHeal(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("priest").
				WithName("Priest").
				WithStats(100, 10, 10).
				WithMP(20, 20).
				WithAgility(100).
				WithSkills(corebattle.ActionSkill{
					ID:          "behomara",
					Name:        "ベホマラー",
					MPCost:      10,
					Power:       50,
					Kind:        corebattle.ActionKindHeal,
					TargetScope: corebattle.TargetScopeAllAllies,
				}).
				MustBuild(),
			corebattle.NewParticipantBuilder("warrior").
				WithName("Warrior").
				WithStats(100, 50, 10).
				WithCurrentHP(10).
				WithAgility(10).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			{ID: "dummy", Name: "Dummy", HP: 10, Attack: 5, Defense: 5, Agility: 5},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Warrior's HP should be restored from 10 by 50
	if hp := res.RemainingHP["warrior"]; hp < 60 {
		t.Errorf("expected warrior to have at least 60 HP after party heal, got %d", hp)
	}
}

func TestPartyBattle_JobSkill_BuffStat(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("bard").
				WithName("Bard").
				WithStats(100, 10, 10).
				WithMP(10, 10).
				WithAgility(100).
				WithSkills(corebattle.ActionSkill{
					ID:          "battle_song",
					Name:        "たたかいのうた",
					MPCost:      5,
					Power:       30,
					Kind:        corebattle.ActionKindBuff,
					BuffStat:    "attack",
					TargetScope: corebattle.TargetScopeAllAllies,
				}).
				MustBuild(),
			corebattle.NewParticipantBuilder("attacker").
				WithName("Attacker").
				WithStats(100, 40, 10).
				WithAgility(50).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			{ID: "dummy", Name: "Dummy", HP: 200, Attack: 10, Defense: 10, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Attacker normal damage would be 40 - 10 = 30.
	// With +30 attack buff from Bard: (40 + 30) - 10 = 60!
	foundBuffedAttack := false
	for _, l := range res.Logs {
		if l.ActorID == "attacker" && l.DamageDealt == 60 {
			foundBuffedAttack = true
			break
		}
	}
	if !foundBuffedAttack {
		t.Errorf("expected attacker to deal 60 damage with buff, logs: %+v", res.Logs)
	}
}

func TestPartyBattle_StatusAilment_ParalyzeAndPoison(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithStatus(corebattle.StatusParalyze).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("poisoned_goblin").
				WithName("Goblin").
				WithStats(50, 10, 5).
				WithAgility(10).
				WithStatus(corebattle.StatusPoison).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundPoisonLog := false
	for _, l := range res.Logs {
		if l.ActionName == "毒ダメージ" {
			foundPoisonLog = true
			break
		}
	}
	if !foundPoisonLog {
		t.Errorf("expected poison DOT damage log, got logs: %+v", res.Logs)
	}
}

func TestPartyBattle_MultiGemCustomSkill(t *testing.T) {
	engine := corebattle.Engine{}

	// Custom skill with 3 blended gems: Attack, Heal, and Fire Field!
	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("sage").
				WithName("Sage").
				WithStats(100, 30, 10).
				WithCMP(15, 15).
				WithAgility(100).
				WithCustomSkills(corebattle.ActionCustomSkill{
					ID:          "trinity_burst",
					Name:        "トリニティバースト",
					Incantation: "万物を照らせ！",
					CMPCost:     10,
					Gems: []corebattle.GemEffect{
						{
							Kind:        corebattle.ActionKindAttack,
							Power:       20,
							Element:     corebattle.ElementFire,
							TargetScope: corebattle.TargetScopeAllEnemies,
						},
						{
							Kind:        corebattle.ActionKindHeal,
							Power:       30,
							TargetScope: corebattle.TargetScopeAllAllies,
						},
						{
							Kind:     "field",
							Element:  corebattle.ElementFire,
							Duration: 3,
						},
					},
				}).
				MustBuild(),
			corebattle.NewParticipantBuilder("injured_ally").
				WithName("InjuredAlly").
				WithStats(100, 10, 10).
				WithCurrentHP(20).
				WithAgility(10).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			{ID: "orc", Name: "Orc", HP: 30, Attack: 10, Defense: 10, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// 1. Check quote in logs
	foundIncantation := false
	for _, l := range res.Logs {
		if l.ActorID == "sage" && len(l.Message) > 0 && (l.Message[len("Sage 「"):] != "") {
			foundIncantation = true
			break
		}
	}
	if !foundIncantation {
		t.Errorf("expected incantation quote in log, logs: %+v", res.Logs)
	}

	// 2. Injured ally was healed
	if res.RemainingHP["injured_ally"] < 50 {
		t.Errorf("expected injured ally to be healed above 50, got %d", res.RemainingHP["injured_ally"])
	}

	// 3. Field was created
	if res.FinalField == nil || res.FinalField.Element != corebattle.ElementFire {
		t.Errorf("expected final field to be fire, got %+v", res.FinalField)
	}
}

func TestCheckRevivalCursedItemAddsStatBuffs(t *testing.T) {
	mp := 10
	result := corebattle.CheckRevival(&corebattle.Participant{
		ID: "cursed", HP: 0, MaxHP: 100,
		ItemDefinitionIDs: []string{"item-260"},
	}, &mp)
	if !result.Revived || !result.Cursed || result.HP != 30 {
		t.Fatalf("unexpected cursed revival: %+v", result)
	}
	if result.AttackBuff != 300 || result.DefenseBuff != 300 || result.AgilityBuff != 300 {
		t.Fatalf("unexpected cursed buffs: %+v", result)
	}
}

func TestPartyBattleMazinSetAbsorbsFallenAllyAttack(t *testing.T) {
	res, err := (corebattle.Engine{}).ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{ID: "fallen", HP: 1, MaxHP: 1, Attack: 80, Defense: 1, Agility: 1},
			{ID: "mazin", HP: 100, MaxHP: 100, Attack: 10, Defense: 1, Agility: 50, ItemDefinitionIDs: []string{"item-037", "item-038"}},
		},
		Enemies:       []corebattle.Participant{{ID: "enemy", HP: 100, MaxHP: 100, Attack: 100, Defense: 1, Agility: 100}},
		VictoryReward: corebattle.Reward{Experience: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, log := range res.Logs {
		if log.ActorID == "mazin" && log.DamageDealt == 49 {
			return
		}
	}
	t.Fatalf("expected Mazin attack 10 + 40 against defense 1, logs: %+v", res.Logs)
}

type fixedFloatRNG struct {
	val float64
}

func (f fixedFloatRNG) Intn(n int) int   { return 0 }
func (f fixedFloatRNG) IntN(n int) int   { return 0 }
func (f fixedFloatRNG) Float64() float64 { return f.val }

func TestPartyBattle_DeterministicStatusRecovery(t *testing.T) {
	engine := corebattle.Engine{}

	// 1. Recovered scenario: RNG returns 0.1 (< 0.33) -> status clears to "" and character acts
	reqRecovered := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithStatus(corebattle.StatusParalyze).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(10, 10, 5).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 10},
		RNG:           fixedFloatRNG{val: 0.1},
	}

	resRecovered, err := engine.ResolvePartyBattle(reqRecovered)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundRecovered := false
	for _, l := range resRecovered.Logs {
		if l.ActionName == "回復" {
			foundRecovered = true
			break
		}
	}
	if !foundRecovered {
		t.Errorf("expected paralyze recovery log with RNG < 0.33, got: %+v", resRecovered.Logs)
	}

	// 2. Paralyzed skip scenario: RNG returns 0.5 (>= 0.33) -> remains paralyzed and cannot act
	reqBlocked := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithStatus(corebattle.StatusParalyze).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(10, 10, 5).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 10},
		RNG:           fixedFloatRNG{val: 0.5},
	}

	resBlocked, err := engine.ResolvePartyBattle(reqBlocked)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundBlocked := false
	for _, l := range resBlocked.Logs {
		if l.ActionName == "行動不能" {
			foundBlocked = true
			break
		}
	}
	if !foundBlocked {
		t.Errorf("expected paralyze skip log with RNG >= 0.33, got: %+v", resBlocked.Logs)
	}
}
