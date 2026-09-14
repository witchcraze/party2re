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

func TestPartyBattle_Seal10_Shinsoku_DoubleAttack(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithAbilities("seal_shinsoku").
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("boss").
				WithName("Boss").
				WithStats(200, 10, 10).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// In turn 1, hero should have attacked twice
	heroAttacks := 0
	for _, l := range res.Logs {
		if l.Turn == 1 && l.ActorID == "hero" && l.ActionName == "攻撃" {
			heroAttacks++
		}
	}
	if heroAttacks != 2 {
		t.Errorf("expected 2 normal attacks from seal_shinsoku in turn 1, got %d", heroAttacks)
	}
}

func TestPartyBattle_Seal11_Kuu_Dispel(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithAbilities("seal_kuu").
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("boss").
				WithName("Boss").
				WithStats(200, 10, 10).
				WithAgility(10).
				WithDefending(true).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 100},
		RNG:           fixedFloatRNG{val: 0.0}, // Intn returns 0 (< 1)
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Verify log message contains reset indication
	foundDispelMsg := false
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.TargetID == "boss" {
			if l.Message != "" && (containsSubstr(l.Message, "元に戻った") || containsSubstr(l.Message, "かき消された")) {
				foundDispelMsg = true
				break
			}
		}
	}
	if !foundDispelMsg {
		t.Errorf("expected dispel message in battle logs for seal_kuu, logs: %+v", res.Logs)
	}
}

func TestPartyBattle_Seal12_Kotowari_MagicAttack(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 50, 10).
				WithMP(10, 10).
				WithAgility(100).
				WithAbilities("seal_kotowari").
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("boss").
				WithName("Boss").
				WithStats(30, 10, 10).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Hero has 50 Atk, Boss has 10 Def -> normal base damage = 50 - 10 = 40.
	// Seal 12 consumes 3 MP, scales to int(40 * 0.8) = 32 damage, ActionName = "理力攻撃".
	foundKotowari := false
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.ActionName == "理力攻撃" {
			foundKotowari = true
			if l.DamageDealt != 32 {
				t.Errorf("expected 32 damage (40 * 0.8), got %d", l.DamageDealt)
			}
			break
		}
	}
	if !foundKotowari {
		t.Errorf("expected action name '理力攻撃' in logs, got: %+v", res.Logs)
	}
	// Verify MP was reduced by 3
	if res.RemainingMP["hero"] != 7 {
		t.Errorf("expected hero MP 7 (10 - 3), got %d", res.RemainingMP["hero"])
	}
}

func containsSubstr(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

type crystalDropRNG struct {
	intVal int
}

func (c crystalDropRNG) Intn(n int) int   { return c.intVal }
func (c crystalDropRNG) IntN(n int) int   { return c.intVal }
func (c crystalDropRNG) Float64() float64 { return 0.5 }

func TestPartyBattle_MonsterCrystalDrops(t *testing.T) {
	engine := corebattle.Engine{}

	tests := []struct {
		name         string
		rngVal       int
		enemyAtk     int
		enemyDef     int
		enemyHP      int
		wantCrystals int
	}{
		{
			name:         "Normal enemy with roll 0 (< 1) drops crystal",
			rngVal:       0,
			enemyAtk:     10,
			enemyDef:     10,
			enemyHP:      20,
			wantCrystals: 1,
		},
		{
			name:         "Normal enemy with roll 1 (not < 1) drops no crystals",
			rngVal:       1,
			enemyAtk:     10,
			enemyDef:     10,
			enemyHP:      20,
			wantCrystals: 0,
		},
		{
			name:         "Strong enemy with roll 1 (< 2) drops crystal",
			rngVal:       1,
			enemyAtk:     150, // strong enemy: 20 + 150 + 5 + 10 = 185 > 152
			enemyDef:     10,
			enemyHP:      20,
			wantCrystals: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := corebattle.PartyBattleRequest{
				Allies: []corebattle.Participant{
					corebattle.NewParticipantBuilder("hero").
						WithName("Hero").
						WithStats(100, 100, 10). // strong ally = 100 + 0 + 100 + 5 + 100 = 305. 0.5x = 152.
						WithAgility(100).
						MustBuild(),
				},
				Enemies: []corebattle.Participant{
					corebattle.NewParticipantBuilder("enemy").
						WithName("Enemy").
						WithStats(tt.enemyHP, tt.enemyAtk, tt.enemyDef).
						WithAgility(10).
						MustBuild(),
				},
				VictoryReward: corebattle.Reward{Experience: 100},
				RNG:           crystalDropRNG{intVal: tt.rngVal},
			}

			res, err := engine.ResolvePartyBattle(req)
			if err != nil {
				t.Fatalf("ResolvePartyBattle failed: %v", err)
			}

			if res.TotalReward.Crystals != tt.wantCrystals {
				t.Errorf("res.TotalReward.Crystals = %d, want %d", res.TotalReward.Crystals, tt.wantCrystals)
			}

			if tt.wantCrystals > 0 {
				foundLog := false
				for _, l := range res.Logs {
					if l.ActionName == "刻印晶獲得" {
						foundLog = true
						break
					}
				}
				if !foundLog {
					t.Errorf("expected '刻印晶獲得' in battle logs, got: %+v", res.Logs)
				}
			}
		})
	}
}
