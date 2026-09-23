package battle_test

import (
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type customRNG struct {
	intnFunc    func(n int) int
	float64Func func() float64
}

func (c customRNG) Intn(n int) int {
	if c.intnFunc != nil {
		return c.intnFunc(n)
	}
	return 0
}

func (c customRNG) IntN(n int) int {
	return c.Intn(n)
}

func (c customRNG) Float64() float64 {
	if c.float64Func != nil {
		return c.float64Func()
	}
	return 0.0
}

func TestCalculateDamage_CanonicalFormula(t *testing.T) {
	// Base: Atk=100, Def=50
	// Expected base = 100*0.5 - 50*0.3 = 50 - 15 = 35
	// With float64=0.0 -> variance = 0.9 -> int(35 * 0.9) = 31
	// With float64=1.0 -> variance = 1.2 -> int(35 * 1.2) = 42

	minVarianceRNG := customRNG{
		float64Func: func() float64 { return 0.0 }, // 0.9
		intnFunc:    func(n int) int { return 0 },
	}
	dmgMin := corebattle.CalculateDamage(100, 50, minVarianceRNG, false)
	if dmgMin != 31 {
		t.Errorf("expected 31 for min variance, got %d", dmgMin)
	}

	maxVarianceRNG := customRNG{
		float64Func: func() float64 { return 0.999999 }, // ~1.2
		intnFunc:    func(n int) int { return 0 },
	}
	dmgMax := corebattle.CalculateDamage(100, 50, maxVarianceRNG, false)
	if dmgMax != 41 && dmgMax != 42 {
		t.Errorf("expected 41 or 42 for max variance, got %d", dmgMax)
	}

	// Defense bypass / Critical:
	// Base = Atk * 0.75 = 100 * 0.75 = 75 (defense ignored!)
	// With float64=0.0 -> variance = 0.9 -> int(75 * 0.9) = 67
	dmgCrit := corebattle.CalculateDamage(100, 50, minVarianceRNG, true)
	if dmgCrit != 67 {
		t.Errorf("expected 67 for critical direct damage, got %d", dmgCrit)
	}

	// High defense resulting in <= 0:
	// Atk=10, Def=100 -> 5 - 30 = -25 <= 0
	// Min damage should be 1 or 2:
	rng1 := customRNG{intnFunc: func(n int) int { return 0 }} // rand(2) returns 0 -> min 1
	if d := corebattle.CalculateDamage(10, 100, rng1, false); d != 1 {
		t.Errorf("expected min damage 1, got %d", d)
	}
	rng2 := customRNG{intnFunc: func(n int) int { return 1 }} // rand(2) returns 1 -> min 2
	if d := corebattle.CalculateDamage(10, 100, rng2, false); d != 2 {
		t.Errorf("expected min damage 2, got %d", d)
	}
}

func TestNormalAttack_EvasionAndHitRate(t *testing.T) {
	engine := corebattle.Engine{}

	// Scenario 1: Miss due to hit rate roll (rand(100) >= 95)
	missRNG := customRNG{
		intnFunc: func(n int) int {
			if n == 100 {
				return 96 // >= 95 -> miss!
			}
			return 1 // rand(3) == 1 -> no critical
		},
		float64Func: func() float64 { return 0.5 },
	}

	resMiss, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 50, 10).
				WithAgility(50).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(100, 10, 10).
				WithAgility(10).
				MustBuild(),
		},
		RNG: missRNG,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resMiss.Logs) == 0 {
		t.Fatal("expected logs")
	}
	firstLog := resMiss.Logs[0]
	if firstLog.DamageDealt != 0 || !strings.Contains(firstLog.Message, "かわした") {
		t.Errorf("expected miss log with 0 damage, got %+v", firstLog)
	}

	// Scenario 2: Attacker holds weapon-63 (必中の剣) -> never misses even if roll >= 95
	resWeapon63, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 50, 10).
				WithAgility(50).
				WithItems("weapon-63").
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(100, 10, 10).
				WithAgility(10).
				MustBuild(),
		},
		RNG: missRNG,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(resWeapon63.Logs) == 0 {
		t.Fatal("expected logs")
	}
	firstLog63 := resWeapon63.Logs[0]
	if firstLog63.DamageDealt <= 0 || strings.Contains(firstLog63.Message, "かわした") {
		t.Errorf("expected weapon-63 to never miss, got %+v", firstLog63)
	}

	// Scenario 3: Defender immobilized (StatusParalyze) -> cannot evade
	resImmobilized, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 50, 10).
				WithAgility(10).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("goblin").
				WithName("Goblin").
				WithStats(100, 10, 10).
				WithAgility(1000). // Huge agility
				WithStatus(corebattle.StatusParalyze).
				MustBuild(),
		},
		RNG: customRNG{
			intnFunc: func(n int) int {
				if n == 100 {
					return 50 // hit roll passes
				}
				return 0 // rand(3) == 0 -> would evade if not immobilized
			},
			float64Func: func() float64 { return 0.5 },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var heroAttackLog *corebattle.TurnLog
	for _, l := range resImmobilized.Logs {
		if l.ActorID == "hero" && l.ActionName == "攻撃" {
			heroAttackLog = &l
			break
		}
	}
	if heroAttackLog == nil {
		t.Fatal("expected hero attack log")
	}
	if heroAttackLog.DamageDealt <= 0 || strings.Contains(heroAttackLog.Message, "かわした") {
		t.Errorf("expected paralyzed defender to be unable to evade, got %+v", heroAttackLog)
	}
}

func TestNormalAttack_CriticalStrike(t *testing.T) {
	engine := corebattle.Engine{}

	// Attacker high agility triggers critical strike
	// Critical strike deals 0.75 * Atk direct damage bypassing high defense (Def=1000)
	critRNG := customRNG{
		intnFunc: func(n int) int {
			if n == 3 {
				return 0 // rand(3) == 0 -> critical roll triggered!
			}
			return 0
		},
		float64Func: func() float64 { return 0.5 }, // attacker agility will exceed defender agility * 3
	}

	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("hero").
				WithName("Hero").
				WithStats(100, 100, 10).
				WithAgility(500). // 500 * 0.5 = 250 >= 10 * 3 * 0.5 = 15
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("iron_golem").
				WithName("IronGolem").
				WithStats(200, 10, 1000). // Massive defense
				WithAgility(10).
				MustBuild(),
		},
		RNG: critRNG,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Logs) == 0 {
		t.Fatal("expected logs")
	}
	firstLog := res.Logs[0]
	if !firstLog.IsCritical {
		t.Errorf("expected TurnLog.IsCritical = true, got false")
	}
	if !strings.Contains(firstLog.Message, "会心の一撃") {
		t.Errorf("expected log message to contain 会心の一撃, got %s", firstLog.Message)
	}
	// Atk = 100. Direct damage = 100 * 0.75 = 75. Variance = 0.9 + 0.5*0.3 = 1.05.
	// Damage ~ 78. Normal damage would be 100*0.5 - 1000*0.3 <= 0 -> 1 or 2.
	if firstLog.DamageDealt < 50 {
		t.Errorf("expected direct critical damage > 50 bypassing 1000 defense, got %d", firstLog.DamageDealt)
	}
}

func TestMultiTarget_DamageDecay(t *testing.T) {
	engine := corebattle.Engine{}

	// Hero casts all-enemies skill against 3 identical enemies
	// Target 1: power * 1.0
	// Target 2: power * 0.85
	// Target 3: power * 0.85 * 0.85
	detRNG := random.NewDeterministic(42)

	res, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("mage").
				WithName("Mage").
				WithStats(100, 10, 10).
				WithMP(50, 50).
				WithAgility(100).
				WithSkills(corebattle.ActionSkill{
					ID:          "ionazun",
					Name:        "イオナズン",
					MPCost:      10,
					Power:       100,
					Kind:        corebattle.ActionKindAttack,
					TargetScope: corebattle.TargetScopeAllEnemies,
				}).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			{ID: "mon-1", Name: "Mon1", HP: 200, Attack: 5, Defense: 10, Agility: 5},
			{ID: "mon-2", Name: "Mon2", HP: 200, Attack: 5, Defense: 10, Agility: 5},
			{ID: "mon-3", Name: "Mon3", HP: 200, Attack: 5, Defense: 10, Agility: 5},
		},
		RNG: detRNG,
	})
	if err != nil {
		t.Fatal(err)
	}

	var damages []int
	for _, l := range res.Logs {
		if l.ActorID == "mage" && l.ActionName == "イオナズン" {
			damages = append(damages, l.DamageDealt)
		}
	}
	if len(damages) < 3 {
		t.Fatalf("expected at least 3 damages for 3 enemies, got %d (%v)", len(damages), damages)
	}

	// Verify cumulative decay across the first turn's 3 targets: damages[0] > damages[1] > damages[2]
	turn1Damages := damages[:3]
	if !(turn1Damages[0] > turn1Damages[1] && turn1Damages[1] > turn1Damages[2]) {
		t.Errorf("expected strictly decaying damage across targets, got %v", turn1Damages)
	}

	// Now test Diamond Ring (item-144): negates decay
	resRing, err := engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies: []corebattle.Participant{
			corebattle.NewParticipantBuilder("mage").
				WithName("Mage").
				WithStats(100, 10, 10).
				WithMP(50, 50).
				WithAgility(100).
				WithItems("item-144").
				WithSkills(corebattle.ActionSkill{
					ID:          "ionazun",
					Name:        "イオナズン",
					MPCost:      10,
					Power:       50,
					Kind:        "attack",
					TargetScope: "all_enemies",
				}).
				MustBuild(),
		},
		Enemies: []corebattle.Participant{
			{ID: "mon-1", Name: "Mon1", HP: 200, Attack: 5, Defense: 10, Agility: 5},
			{ID: "mon-2", Name: "Mon2", HP: 200, Attack: 5, Defense: 10, Agility: 5},
			{ID: "mon-3", Name: "Mon3", HP: 200, Attack: 5, Defense: 10, Agility: 5},
		},
		RNG: customRNG{
			float64Func: func() float64 { return 1.0 / 3.0 }, // variance = 0.9 + 0.1 = 1.0
			intnFunc:    func(n int) int { return 0 },
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var ringDamages []int
	for _, l := range resRing.Logs {
		if l.ActorID == "mage" && l.ActionName == "イオナズン" {
			ringDamages = append(ringDamages, l.DamageDealt)
		}
	}
	if len(ringDamages) < 3 {
		t.Fatalf("expected at least 3 damages, got %d", len(ringDamages))
	}
	turn1RingDamages := ringDamages[:3]
	if turn1RingDamages[0] != turn1RingDamages[1] || turn1RingDamages[1] != turn1RingDamages[2] {
		t.Errorf("expected Diamond Ring to negate decay, got %v", turn1RingDamages)
	}
}
