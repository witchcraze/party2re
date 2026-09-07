package battle_test

import (
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestPartyBattle_AgilityTurnOrder(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{ID: "slow_hero", Name: "SlowHero", HP: 100, Attack: 20, Defense: 10, Agility: 10},
			{ID: "fast_hero", Name: "FastHero", HP: 100, Attack: 20, Defense: 10, Agility: 90},
		},
		Enemies: []corebattle.Participant{
			{ID: "mid_goblin", Name: "MidGoblin", HP: 200, Attack: 15, Defense: 5, Agility: 50},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	if len(res.Logs) < 3 {
		t.Fatalf("expected at least 3 logs, got %d", len(res.Logs))
	}

	// Turn 1 actions order should be: fast_hero (90) -> mid_goblin (50) -> slow_hero (10)
	if res.Logs[0].ActorID != "fast_hero" {
		t.Errorf("expected first actor fast_hero, got %s", res.Logs[0].ActorID)
	}
	if res.Logs[1].ActorID != "mid_goblin" {
		t.Errorf("expected second actor mid_goblin, got %s", res.Logs[1].ActorID)
	}
	if res.Logs[2].ActorID != "slow_hero" {
		t.Errorf("expected third actor slow_hero, got %s", res.Logs[2].ActorID)
	}
}

func TestPartyBattle_JobSkillAndMP(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{
				ID:      "mage",
				Name:    "Mage",
				HP:      80,
				MaxHP:   80,
				MP:      30,
				MaxMP:   30,
				Attack:  10,
				Defense: 5,
				Agility: 50,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "fireball_all",
						Name:        "イオナズン",
						MPCost:      15,
						Power:       40,
						Kind:        "attack",
						TargetScope: "all_enemies",
						Element:     "fire",
					},
				},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "e1", Name: "Enemy1", HP: 40, Attack: 5, Defense: 2, Agility: 10},
			{ID: "e2", Name: "Enemy2", HP: 40, Attack: 5, Defense: 2, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected win, got %s", res.Outcome)
	}

	// Verify skill usage in logs
	foundSkill := false
	for _, l := range res.Logs {
		if l.ActionName == "イオナズン" {
			foundSkill = true
			if l.ActorID != "mage" {
				t.Errorf("expected actor mage, got %s", l.ActorID)
			}
		}
	}
	if !foundSkill {
		t.Error("expected skill イオナズン to be used in battle")
	}
}

func TestPartyBattle_CustomSkillAndCMP(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{
				ID:      "crafter",
				Name:    "Crafter",
				HP:      100,
				Attack:  20,
				Defense: 10,
				Agility: 50,
				CMP:     10,
				MaxCMP:  10,
				CustomSkills: []corebattle.ActionCustomSkill{
					{
						ID:          "my_super_spell",
						Name:        "奥義・蒼炎波",
						Incantation: "すべてを灰燼に帰せ！",
						CMPCost:     5,
						Gems: []corebattle.GemEffect{
							{Kind: "attack", Power: 80, Element: "fire", TargetScope: "single_enemy"},
						},
					},
				},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "boss", Name: "Boss", HP: 50, Attack: 10, Defense: 5, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 200},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	if res.Outcome != corebattle.OutcomeWin {
		t.Fatalf("expected win, got %s", res.Outcome)
	}

	foundIncantation := false
	for _, l := range res.Logs {
		if strings.Contains(l.Message, "すべてを灰燼に帰せ！") {
			foundIncantation = true
		}
	}
	if !foundIncantation {
		t.Error("expected incantation quote in turn log")
	}
}

func TestPartyBattle_FieldEffectsAndAntiField(t *testing.T) {
	engine := corebattle.Engine{}

	// Initial field: Fire. Fire skills get 1.3x boost.
	reqWithField := corebattle.PartyBattleRequest{
		InitialField: &corebattle.FieldState{
			Element:       "fire",
			RemainingTurn: 3,
		},
		Allies: []corebattle.Participant{
			{
				ID:      "fire_mage",
				Name:    "FireMage",
				HP:      100,
				Attack:  50,
				Defense: 10,
				Agility: 60,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "fire_strike",
						Name:        "火炎斬り",
						MPCost:      5,
						Power:       20,
						Kind:        "attack",
						TargetScope: "single_enemy",
						Element:     "fire",
					},
				},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "dummy", Name: "Dummy", HP: 500, Attack: 1, Defense: 10, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	resField, err := engine.ResolvePartyBattle(reqWithField)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Normal damage without field: (50 + 20) - 10 = 60.
	// With 1.3x fire field multiplier: int(60 * 1.3) = 78.
	if len(resField.Logs) > 0 && resField.Logs[0].ActionName == "火炎斬り" {
		if resField.Logs[0].DamageDealt < 70 {
			t.Errorf("expected boosted fire field damage (~78), got %d", resField.Logs[0].DamageDealt)
		}
	}
}

func TestPartyBattle_DefeatAndRevival(t *testing.T) {
	engine := corebattle.Engine{}

	// Pharaoh ability participant
	reqPharaoh := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{
				ID:        "pharaoh_hero",
				Name:      "PharaohHero",
				HP:        20,
				MaxHP:     100,
				MP:        50,
				MaxMP:     100,
				Attack:    50,
				Defense:   5,
				Agility:   10,
				Abilities: []string{"pharaoh"},
			},
		},
		Enemies: []corebattle.Participant{
			// Enemy acts first and deals lethal damage
			{ID: "one_shotter", Name: "OneShotter", HP: 80, Attack: 60, Defense: 5, Agility: 90},
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(reqPharaoh)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundRevive := false
	for _, l := range res.Logs {
		if strings.Contains(l.Message, "不死の呪いでよみがえった") {
			foundRevive = true
		}
	}
	if !foundRevive {
		t.Error("expected pharaoh revival message in logs")
	}
	if res.Outcome != corebattle.OutcomeWin {
		t.Errorf("expected win after revival, got %s", res.Outcome)
	}
}

func TestPartyBattle_AntiFieldSuppression(t *testing.T) {
	engine := corebattle.Engine{}

	reqAnti := corebattle.PartyBattleRequest{
		InitialField: &corebattle.FieldState{
			Element:       "fire",
			RemainingTurn: 5,
			AntiFieldTurn: 2, // Active anti-field suppresses fire field
		},
		Allies: []corebattle.Participant{
			{
				ID:      "fire_mage",
				Name:    "FireMage",
				HP:      100,
				Attack:  50,
				Defense: 10,
				Agility: 50,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "fire_strike",
						Name:        "火炎斬り",
						MPCost:      5,
						Power:       20,
						Kind:        "attack",
						TargetScope: "single_enemy",
						Element:     "fire",
					},
				},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "target", Name: "Target", HP: 500, Attack: 5, Defense: 10, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(reqAnti)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Normal damage: (50 + 20) - 10 = 60.
	// Because anti-field is active, multiplier is 1.0 (NOT 1.3).
	if len(res.Logs) > 0 && res.Logs[0].ActionName == "火炎斬り" {
		if res.Logs[0].DamageDealt != 60 {
			t.Errorf("expected standard unboosted damage (60) under anti-field, got %d", res.Logs[0].DamageDealt)
		}
	}
}

func TestPartyBattle_UndyingRevival(t *testing.T) {
	engine := corebattle.Engine{}

	reqUndying := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{
				ID:        "undying_knight",
				Name:      "Knight",
				HP:        10,
				MaxHP:     100,
				Attack:    50,
				Defense:   5,
				Agility:   10,
				Abilities: []string{"undying"},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "reaper", Name: "Reaper", HP: 40, Attack: 50, Defense: 5, Agility: 90},
		},
		VictoryReward: corebattle.Reward{Experience: 100},
	}

	res, err := engine.ResolvePartyBattle(reqUndying)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundRevive := false
	for _, l := range res.Logs {
		if strings.Contains(l.Message, "よみがえった") {
			foundRevive = true
		}
	}
	if !foundRevive {
		t.Error("expected undying revival in logs")
	}
	if res.Outcome != corebattle.OutcomeWin {
		t.Errorf("expected win after undying revival, got %s", res.Outcome)
	}
}

func TestPartyBattle_HealingSkill(t *testing.T) {
	engine := corebattle.Engine{}

	reqHeal := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			{
				ID:      "cleric",
				Name:    "Cleric",
				HP:      100,
				MaxHP:   100,
				MP:      20,
				MaxMP:   20,
				Attack:  10,
				Defense: 10,
				Agility: 50,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "heal_spell",
						Name:        "ホイミ",
						MPCost:      5,
						Power:       40,
						Kind:        "heal",
						TargetScope: "single_ally",
					},
				},
			},
			{
				ID:      "wounded_warrior",
				Name:    "Warrior",
				HP:      20,
				MaxHP:   100,
				Attack:  30,
				Defense: 10,
				Agility: 40,
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "slime", Name: "Slime", HP: 10, Attack: 5, Defense: 2, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(reqHeal)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundHeal := false
	for _, l := range res.Logs {
		if l.ActionName == "ホイミ" {
			foundHeal = true
			if l.HealingDone != 40 {
				t.Errorf("expected 40 healing done, got %d", l.HealingDone)
			}
			if l.TargetID != "wounded_warrior" {
				t.Errorf("expected target wounded_warrior, got %s", l.TargetID)
			}
		}
	}
	if !foundHeal {
		t.Error("expected healing skill ホイミ to be executed")
	}
}

func TestPartyBattle_OpposingElementFieldPenalty(t *testing.T) {
	engine := corebattle.Engine{}

	reqWaterField := corebattle.PartyBattleRequest{
		InitialField: &corebattle.FieldState{
			Element:       "water",
			RemainingTurn: 3,
		},
		Allies: []corebattle.Participant{
			{
				ID:      "fire_knight",
				Name:    "FireKnight",
				HP:      100,
				Attack:  50,
				Defense: 10,
				Agility: 50,
				Skills: []corebattle.ActionSkill{
					{
						ID:          "fire_slash",
						Name:        "火炎斬り",
						MPCost:      5,
						Power:       20,
						Kind:        "attack",
						TargetScope: "single_enemy",
						Element:     "fire",
					},
				},
			},
		},
		Enemies: []corebattle.Participant{
			{ID: "dummy", Name: "Dummy", HP: 500, Attack: 5, Defense: 10, Agility: 10},
		},
		VictoryReward: corebattle.Reward{Experience: 50},
	}

	res, err := engine.ResolvePartyBattle(reqWaterField)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// Normal damage: (50 + 20) - 10 = 60.
	// Opposing water field penalty: 60 * 0.8 = 48.
	if len(res.Logs) > 0 && res.Logs[0].ActionName == "火炎斬り" {
		if res.Logs[0].DamageDealt != 48 {
			t.Errorf("expected 48 damage under opposing water field, got %d", res.Logs[0].DamageDealt)
		}
	}
}
