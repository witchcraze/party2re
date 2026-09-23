package battle_test

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestPartyBattle_StatusDofuu(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithStatus(corebattle.StatusDofuu).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(100, 10, 5).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 10},
		RNG:           fixedFloatRNG{val: 0.5},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundDofuuSkip := false
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.ActionName == "行動不能" && l.Message == "しかし、Hero は動くことができない！" {
			foundDofuuSkip = true
			break
		}
	}
	if !foundDofuuSkip {
		t.Fatalf("expected Hero dofuu turn skip log, got logs: %+v", res.Logs)
	}

	// Turn 1 hero skipped, turn 2 hero acts normally
	foundHeroAttackTurn2 := false
	for _, l := range res.Logs {
		if l.Turn == 2 && l.ActorID == "hero" && l.ActionName == "攻撃" {
			foundHeroAttackTurn2 = true
			break
		}
	}
	if !foundHeroAttackTurn2 {
		t.Fatalf("expected Hero to act normally on turn 2 after Dofuu cleared, got logs: %+v", res.Logs)
	}
}

func TestPartyBattle_StatusKinju(t *testing.T) {
	engine := corebattle.Engine{}

	t.Run("Trigger self-damage and skip on roll < 0.25", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 30, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusKinju).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.1}, // < 0.25
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundKinjuDamage := false
		for _, l := range res.Logs {
			if l.ActorID == "hero" && l.ActionName == "禁呪" && l.DamageDealt == 10 {
				foundKinjuDamage = true
				break
			}
		}
		if !foundKinjuDamage {
			t.Fatalf("expected Hero kinju self-damage (10), got logs: %+v", res.Logs)
		}
	})

	t.Run("No trigger on roll >= 0.25 acts normally", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 30, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusKinju).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.5}, // >= 0.25
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundKinjuDamage := false
		foundHeroAttackTurn1 := false
		for _, l := range res.Logs {
			if l.Turn == 1 && l.ActorID == "hero" {
				if l.ActionName == "禁呪" {
					foundKinjuDamage = true
				}
				if l.ActionName == "攻撃" {
					foundHeroAttackTurn1 = true
				}
			}
		}
		if foundKinjuDamage {
			t.Fatalf("expected no kinju damage when roll >= 0.25, got logs: %+v", res.Logs)
		}
		if !foundHeroAttackTurn1 {
			t.Fatalf("expected Hero to act normally on turn 1, got logs: %+v", res.Logs)
		}
	})
}

func TestPartyBattle_StatusSabaku(t *testing.T) {
	engine := corebattle.Engine{}

	t.Run("Trigger skip and cure when sub-roll < 0.50", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 30, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusSabaku).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.1}, // < 0.25 skip, < 0.50 cure
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundSabakuCure := false
		for _, l := range res.Logs {
			if l.Turn == 1 && l.ActorID == "hero" && l.ActionName == "行動不能" {
				if l.Message == "しかし、Hero は動くことができない！ Hero は鎖から解放された！" {
					foundSabakuCure = true
					break
				}
			}
		}
		if !foundSabakuCure {
			t.Fatalf("expected sabaku skip and cure log, got: %+v", res.Logs)
		}

		// Turn 2 should act normally
		foundHeroAttackTurn2 := false
		for _, l := range res.Logs {
			if l.Turn == 2 && l.ActorID == "hero" && l.ActionName == "攻撃" {
				foundHeroAttackTurn2 = true
				break
			}
		}
		if !foundHeroAttackTurn2 {
			t.Fatalf("expected Hero to attack on turn 2 after cure, got: %+v", res.Logs)
		}
	})

	t.Run("Blocks stat buffs", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("caster").
					WithName("Caster").
					WithStats(100, 10, 10).
					WithMP(50, 50).
					WithAgility(150).
					WithSkills(corebattle.ActionSkill{
						ID:          "buff_skill",
						Name:        "スカラ",
						MPCost:      5,
						Power:       20,
						Kind:        "buff",
						TargetScope: "all_allies",
						BuffStat:    "defense",
					}).
					MustBuild(),
				corebattle.NewParticipantBuilder("bound_hero").
					WithName("BoundHero").
					WithStats(100, 20, 10).
					WithAgility(50).
					WithStatus(corebattle.StatusSabaku).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.9}, // no skip
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundBlockedLog := false
		for _, l := range res.Logs {
			if l.TargetID == "bound_hero" && l.Message == "BoundHero は鎖縛により能力があがらない！" {
				foundBlockedLog = true
				break
			}
		}
		if !foundBlockedLog {
			t.Fatalf("expected buff blockage on BoundHero, got logs: %+v", res.Logs)
		}
	})
}

func TestPartyBattle_StatusConfusion(t *testing.T) {
	engine := corebattle.Engine{}

	t.Run("Natural cure on roll < 0.20", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 30, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusConfusion).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.1}, // < 0.20
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundCureLog := false
		for _, l := range res.Logs {
			if l.ActorID == "hero" && l.ActionName == "回復" && l.Message == "Hero の混乱が治った！" {
				foundCureLog = true
				break
			}
		}
		if !foundCureLog {
			t.Fatalf("expected confusion cure log on roll < 0.20, got: %+v", res.Logs)
		}
	})

	t.Run("Redirects targeting across allies and enemies", func(t *testing.T) {
		// Hero is confused and has an ally (Fighter).
		// With Intn returning 0, candidateTargets[0] is Fighter (ally)!
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 40, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusConfusion).
					MustBuild(),
				corebattle.NewParticipantBuilder("fighter").
					WithName("Fighter").
					WithStats(100, 20, 10).
					WithAgility(50).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG: customRNG{
				float64Func: func() float64 { return 0.5 }, // >= 0.20 remains confused
				intnFunc: func(n int) int {
					// 0 selects first candidate (fighter)
					return 0
				},
			},
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		foundConfusedLog := false
		foundFriendlyFire := false
		for _, l := range res.Logs {
			if l.ActorID == "hero" && l.ActionName == "混乱" {
				foundConfusedLog = true
			}
			if l.ActorID == "hero" && l.ActionName == "攻撃" && l.TargetID == "fighter" {
				foundFriendlyFire = true
			}
		}
		if !foundConfusedLog {
			t.Fatalf("expected 'Hero は混乱している！' log, got: %+v", res.Logs)
		}
		if !foundFriendlyFire {
			t.Fatalf("expected Hero to strike ally Fighter while confused, got logs: %+v", res.Logs)
		}
	})
}

func TestPartyBattle_StatusDeadlyPoison(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 30, 10).
				WithAgility(100).
				WithStatus(corebattle.StatusDeadlyPoison).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(100, 10, 5).
				WithAgility(10).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 10},
		RNG:           fixedFloatRNG{val: 0.05}, // even with roll < 0.20, deadly poison cannot cure
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundDeadlyPoisonDamage := false
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.ActionName == "毒ダメージ" && l.DamageDealt == 10 && l.Message == "Hero は猛毒により 10 のダメージをうけた！" {
			foundDeadlyPoisonDamage = true
			break
		}
	}
	if !foundDeadlyPoisonDamage {
		t.Fatalf("expected Hero deadly poison damage log, got: %+v", res.Logs)
	}

	// Verify it did not naturally cure
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.ActionName == "回復" && l.Message == "Hero の毒が治った！" {
			t.Fatalf("deadly poison must not cure naturally, but found cure log: %+v", l)
		}
	}
}

func TestPartyBattle_PoisonPostActionAndCure(t *testing.T) {
	engine := corebattle.Engine{}

	t.Run("Post-action poison and cure roll < 0.20", func(t *testing.T) {
		req := corebattle.PartyBattleRequest{
			Allies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("hero").
					WithName("Hero").
					WithStats(100, 30, 10).
					WithAgility(100).
					WithStatus(corebattle.StatusPoison).
					MustBuild(),
			},
			Enemies: []corebattle.Participant{
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.1}, // < 0.20
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		// Turn 1 order should be: Hero Attack -> Hero Poison Damage -> Hero Poison Cure
		foundAttack := false
		foundPoison := false
		foundCure := false
		for _, l := range res.Logs {
			if l.Turn == 1 && l.ActorID == "hero" {
				if l.ActionName == "攻撃" {
					foundAttack = true
				}
				if l.ActionName == "毒ダメージ" {
					if !foundAttack {
						t.Fatalf("poison damage occurred before attack, expected post-action timing")
					}
					foundPoison = true
				}
				if l.ActionName == "回復" && l.Message == "Hero の毒が治った！" {
					if !foundPoison {
						t.Fatalf("poison cure occurred before damage")
					}
					foundCure = true
				}
			}
		}
		if !foundPoison || !foundCure {
			t.Fatalf("expected post-action poison and cure, got logs: %+v", res.Logs)
		}
	})

	t.Run("Action skipped due to paralysis -> no post-action poison damage", func(t *testing.T) {
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
				corebattle.NewParticipantBuilder("goblin").
					WithName("Goblin").
					WithStats(100, 10, 5).
					WithAgility(10).
					MustBuild(),
			},
			VictoryReward: corebattle.Reward{Experience: 10},
			RNG:           fixedFloatRNG{val: 0.5}, // >= 0.33 paralyze skip
		}

		res, err := engine.ResolvePartyBattle(req)
		if err != nil {
			t.Fatalf("ResolvePartyBattle failed: %v", err)
		}

		for _, l := range res.Logs {
			if l.Turn == 1 && l.ActorID == "hero" && l.ActionName == "毒ダメージ" {
				t.Fatalf("paralyzed skipped hero should not trigger poison damage on turn 1")
			}
		}
	})
}

func TestPartyBattle_DokuroAmuletRevivalAppliesDofuu(t *testing.T) {
	engine := corebattle.Engine{}

	req := corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 10, 0).
				WithAgility(10).
				WithAbilities("dokuro_amulet").
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("boss").
				WithName("Boss").
				WithStats(200, 200, 50). // One-shots hero
				WithAgility(100).
				MustBuild(),
		},
		VictoryReward: corebattle.Reward{Experience: 10},
		RNG:           fixedFloatRNG{val: 0.5},
	}

	res, err := engine.ResolvePartyBattle(req)
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	foundRevive := false
	foundDofuuSkip := false
	for _, l := range res.Logs {
		if l.ActorID == "hero" && l.ActionName == "行動不能" && l.Message == "しかし、Hero は動くことができない！" {
			foundDofuuSkip = true
		}
		if l.Message == "Heroは瀕死でよみがえった！" || (len(l.Message) > 0 && l.RemainingHP["hero"] == 25) {
			foundRevive = true
		}
	}
	if !foundRevive {
		t.Fatalf("expected dokuro_amulet revival log, got: %+v", res.Logs)
	}
	if !foundDofuuSkip {
		t.Fatalf("expected Hero to be immobilized by Dofuu after dokuro_amulet revive, got: %+v", res.Logs)
	}
}
