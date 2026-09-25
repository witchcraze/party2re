package job_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/job"
)

func TestValidateRequirements_LevelAndGender(t *testing.T) {
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	warrior, err := catalog.FindByID("job-01")
	if err != nil {
		t.Fatalf("warrior not found: %v", err)
	}

	// Level < 20 rejected
	lowLvlCtx := job.ChangeContext{Level: 19, CurrentJobID: "starter"}
	if err := job.ValidateRequirements(warrior, lowLvlCtx); err != job.ErrJobUnavailable {
		t.Errorf("expected ErrJobUnavailable for level 19, got %v", err)
	}

	// Level 20 accepted
	validLvlCtx := job.ChangeContext{Level: 20, CurrentJobID: "starter"}
	if err := job.ValidateRequirements(warrior, validLvlCtx); err != nil {
		t.Errorf("expected valid for level 20 warrior, got %v", err)
	}

	// Male-only job (吟遊詩人 job-13)
	bard, err := catalog.FindByID("job-13")
	if err != nil {
		t.Fatalf("bard not found: %v", err)
	}
	if err := job.ValidateRequirements(bard, job.ChangeContext{Level: 20, Gender: "f", CurrentJobID: "job-01"}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for female bard, got %v", err)
	}
	if err := job.ValidateRequirements(bard, job.ChangeContext{Level: 20, Gender: "m", CurrentJobID: "job-01"}); err != nil {
		t.Errorf("expected success for male bard, got %v", err)
	}

	// Female-only job (踊り子 job-14)
	dancer, err := catalog.FindByID("job-14")
	if err != nil {
		t.Fatalf("dancer not found: %v", err)
	}
	if err := job.ValidateRequirements(dancer, job.ChangeContext{Level: 20, Gender: "m", CurrentJobID: "job-01"}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for male dancer, got %v", err)
	}
	if err := job.ValidateRequirements(dancer, job.ChangeContext{Level: 20, Gender: "f", CurrentJobID: "job-01"}); err != nil {
		t.Errorf("expected success for female dancer, got %v", err)
	}
}

func TestValidateRequirements_PrerequisiteTree(t *testing.T) {
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	// 竜騎士 (job-23): required_job_ids = [9 (盗賊), 11 (弓使い), 23, 26 (忍者)]
	dragonKnight, err := catalog.FindByID("job-23")
	if err != nil {
		t.Fatalf("dragon knight not found: %v", err)
	}

	// From warrior (job-01) with old job starter -> rejected
	if err := job.ValidateRequirements(dragonKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-01", OldJobID: "starter"}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection from warrior, got %v", err)
	}

	// From thief (job-09) -> accepted
	if err := job.ValidateRequirements(dragonKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-09", OldJobID: "job-01"}); err != nil {
		t.Errorf("expected success from thief, got %v", err)
	}

	// From archer (job-11) as old job -> accepted
	if err := job.ValidateRequirements(dragonKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-01", OldJobID: "job-11"}); err != nil {
		t.Errorf("expected success with archer old job, got %v", err)
	}

	// Re-changing to dragon knight (job-23) -> accepted
	if err := job.ValidateRequirements(dragonKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-23"}); err != nil {
		t.Errorf("expected success re-changing to dragon knight, got %v", err)
	}
}

func TestValidateRequirements_Milestones(t *testing.T) {
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	// バーサーカー (job-21): kill_m > 200 && required [1, 4, 12, 21]
	berserker, _ := catalog.FindByID("job-21")
	// Meets job (job-01) but kill_m <= 200 -> rejected
	if err := job.ValidateRequirements(berserker, job.ChangeContext{Level: 20, CurrentJobID: "job-01", MonsterKills: 200}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for kill_m=200, got %v", err)
	}
	// Meets job and kill_m=201 -> accepted
	if err := job.ValidateRequirements(berserker, job.ChangeContext{Level: 20, CurrentJobID: "job-01", MonsterKills: 201}); err != nil {
		t.Errorf("expected success for kill_m=201, got %v", err)
	}

	// 暗黒騎士 (job-22): kill_p > 50 && required [2, 3, 17, 20, 22, 52]
	darkKnight, _ := catalog.FindByID("job-22")
	if err := job.ValidateRequirements(darkKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-02", PvPWins: 50}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for pvp_wins=50, got %v", err)
	}
	if err := job.ValidateRequirements(darkKnight, job.ChangeContext{Level: 20, CurrentJobID: "job-02", PvPWins: 51}); err != nil {
		t.Errorf("expected success for pvp_wins=51, got %v", err)
	}

	// 勇者 (job-34): hero_c >= 5 + item-028 (or already job-34)
	hero, _ := catalog.FindByID("job-34")
	if err := job.ValidateRequirements(hero, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, HeroCount: 4}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for hero_c=4, got %v", err)
	}
	if err := job.ValidateRequirements(hero, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: false, HeroCount: 5}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection without item, got %v", err)
	}
	if err := job.ValidateRequirements(hero, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, HeroCount: 5}); err != nil {
		t.Errorf("expected success for hero_c=5 with item, got %v", err)
	}
	if err := job.ValidateRequirements(hero, job.ChangeContext{Level: 20, CurrentJobID: "job-34", HasRequiredItem: false, HeroCount: 0}); err != nil {
		t.Errorf("expected success re-changing to hero, got %v", err)
	}

	// 魔王 (job-35): mao_c >= 1 + item-029 (or already job-35)
	demonLord, _ := catalog.FindByID("job-35")
	if err := job.ValidateRequirements(demonLord, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, MaoCount: 0}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for mao_c=0, got %v", err)
	}
	if err := job.ValidateRequirements(demonLord, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, MaoCount: 1}); err != nil {
		t.Errorf("expected success for mao_c=1 with item, got %v", err)
	}

	// ギャンブラー (job-46): cas_c >= 10 + item-039 (or already job-46)
	gambler, _ := catalog.FindByID("job-46")
	if err := job.ValidateRequirements(gambler, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, CasinoWins: 9}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for cas_c=9, got %v", err)
	}
	if err := job.ValidateRequirements(gambler, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true, CasinoWins: 10}); err != nil {
		t.Errorf("expected success for cas_c=10 with item, got %v", err)
	}

	// 魔人 (job-52): kill_m > 1000 && required [1, 21, 25, 52]
	majin, _ := catalog.FindByID("job-52")
	if err := job.ValidateRequirements(majin, job.ChangeContext{Level: 20, CurrentJobID: "job-01", MonsterKills: 1000}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for kill_m=1000, got %v", err)
	}
	if err := job.ValidateRequirements(majin, job.ChangeContext{Level: 20, CurrentJobID: "job-01", MonsterKills: 1001}); err != nil {
		t.Errorf("expected success for kill_m=1001, got %v", err)
	}

	// 剣闘士 (job-74): kill_p > 30 && required [2, 3, 4, 8] (or already job-74)
	gladiator, _ := catalog.FindByID("job-74")
	if err := job.ValidateRequirements(gladiator, job.ChangeContext{Level: 20, CurrentJobID: "job-02", PvPWins: 30}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for kill_p=30, got %v", err)
	}
	if err := job.ValidateRequirements(gladiator, job.ChangeContext{Level: 20, CurrentJobID: "job-02", PvPWins: 31}); err != nil {
		t.Errorf("expected success for kill_p=31, got %v", err)
	}

	// 双剣士 (job-77): kill_m > 1500 && required [2, 24, 28] (or already job-77)
	dualBlader, _ := catalog.FindByID("job-77")
	if err := job.ValidateRequirements(dualBlader, job.ChangeContext{Level: 20, CurrentJobID: "job-02", MonsterKills: 1500}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for kill_m=1500, got %v", err)
	}
	if err := job.ValidateRequirements(dualBlader, job.ChangeContext{Level: 20, CurrentJobID: "job-02", MonsterKills: 1501}); err != nil {
		t.Errorf("expected success for kill_m=1501, got %v", err)
	}

	// トレジャーハンター (job-78): job_lv >= 50 && required [7, 8, 9, 26, 50] (or already job-78)
	treasureHunter, _ := catalog.FindByID("job-78")
	if err := job.ValidateRequirements(treasureHunter, job.ChangeContext{Level: 20, CurrentJobID: "job-07", JobLevel: 49}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for job_lv=49, got %v", err)
	}
	if err := job.ValidateRequirements(treasureHunter, job.ChangeContext{Level: 20, CurrentJobID: "job-07", JobLevel: 50}); err != nil {
		t.Errorf("expected success for job_lv=50, got %v", err)
	}

	// たまねぎ剣士 (job-49): sp >= 300 (or already job-49)
	onion, _ := catalog.FindByID("job-49")
	if err := job.ValidateRequirements(onion, job.ChangeContext{Level: 20, CurrentJobID: "job-01", SP: 299}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for sp=299, got %v", err)
	}
	if err := job.ValidateRequirements(onion, job.ChangeContext{Level: 20, CurrentJobID: "job-01", SP: 300}); err != nil {
		t.Errorf("expected success for sp=300, got %v", err)
	}

	// すっぴん (job-73): all_jobs_mastered == true
	suppin, _ := catalog.FindByID("job-73")
	if err := job.ValidateRequirements(suppin, job.ChangeContext{Level: 20, CurrentJobID: "job-01", AllJobsMastered: false}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection for incomplete mastery, got %v", err)
	}
	if err := job.ValidateRequirements(suppin, job.ChangeContext{Level: 20, CurrentJobID: "job-01", AllJobsMastered: true}); err != nil {
		t.Errorf("expected success for all jobs mastered, got %v", err)
	}
}

func TestValidateRequirements_SpecialEquipmentAndItem(t *testing.T) {
	catalog, err := job.InitialCatalog()
	if err != nil {
		t.Fatalf("failed to load catalog: %v", err)
	}

	// 炎闘士 (job-84): requires armor-29 equipped && prerequisite [1, 4, 25, 30] (or already job-84)
	fireFighter, _ := catalog.FindByID("job-84")
	// From job-01 without armor-29 -> rejected
	if err := job.ValidateRequirements(fireFighter, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasEquippedArmor: false}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection without armor, got %v", err)
	}
	// From job-01 with armor-29 -> accepted
	if err := job.ValidateRequirements(fireFighter, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasEquippedArmor: true}); err != nil {
		t.Errorf("expected success with armor-29, got %v", err)
	}
	// From non-prerequisite job (job-02) even with armor-29 -> rejected
	if err := job.ValidateRequirements(fireFighter, job.ChangeContext{Level: 20, CurrentJobID: "job-02", HasEquippedArmor: true}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection from non-prerequisite job, got %v", err)
	}
	// Already job-84 -> accepted without armor
	if err := job.ValidateRequirements(fireFighter, job.ChangeContext{Level: 20, CurrentJobID: "job-84", HasEquippedArmor: false}); err != nil {
		t.Errorf("expected success re-changing to job-84, got %v", err)
	}

	// 賢者 (job-33): item-027 OR current/old job in [8, 33]
	sage, _ := catalog.FindByID("job-33")
	// From job-01 without item -> rejected
	if err := job.ValidateRequirements(sage, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: false}); err != job.ErrJobUnavailable {
		t.Errorf("expected rejection without item, got %v", err)
	}
	// From job-01 with item -> accepted
	if err := job.ValidateRequirements(sage, job.ChangeContext{Level: 20, CurrentJobID: "job-01", HasRequiredItem: true}); err != nil {
		t.Errorf("expected success with item, got %v", err)
	}
	// From 遊び人 (job-08) without item -> accepted
	if err := job.ValidateRequirements(sage, job.ChangeContext{Level: 20, CurrentJobID: "job-08", HasRequiredItem: false}); err != nil {
		t.Errorf("expected success from asobinin without item, got %v", err)
	}
}

func TestIsItemAndArmorConsumed(t *testing.T) {
	// Standard item job: consumed on first change
	if !job.IsItemConsumed("job-34", "job-01", "starter") {
		t.Errorf("expected item to be consumed for hero from warrior")
	}
	// Not consumed if already hero
	if job.IsItemConsumed("job-34", "job-34", "starter") {
		t.Errorf("expected item NOT to be consumed if already hero")
	}
	if job.IsItemConsumed("job-34", "job-01", "job-34") {
		t.Errorf("expected item NOT to be consumed if old job is hero")
	}

	// Sage and Gambler from Asobinin: NOT consumed
	if job.IsItemConsumed("job-33", "job-08", "starter") {
		t.Errorf("expected sage item NOT consumed from asobinin")
	}
	if job.IsItemConsumed("job-46", "job-08", "starter") {
		t.Errorf("expected gambler item NOT consumed from asobinin")
	}
	if job.IsItemConsumed("job-33", "job-01", "job-08") {
		t.Errorf("expected sage item NOT consumed with asobinin old job")
	}

	// Armor consumption (job-84)
	if !job.IsArmorConsumed("job-84", "job-01", "starter") {
		t.Errorf("expected armor consumed for job-84 from job-01")
	}
	if job.IsArmorConsumed("job-84", "job-84", "starter") {
		t.Errorf("expected armor NOT consumed if already job-84")
	}
	if job.IsArmorConsumed("job-84", "job-01", "job-84") {
		t.Errorf("expected armor NOT consumed if old job was job-84")
	}
	if job.IsArmorConsumed("job-01", "job-02", "starter") {
		t.Errorf("expected non-armor job to not consume armor")
	}
}

func TestRequiresItemPossession(t *testing.T) {
	// Standard item job: Hero (job-34) requires item-028
	hero := job.Definition{ID: "job-34"}
	if !job.RequiresItemPossession(hero, "job-01", "starter") {
		t.Errorf("expected hero to require item possession from warrior")
	}
	if job.RequiresItemPossession(hero, "job-34", "starter") {
		t.Errorf("expected hero NOT to require item if current job is hero")
	}
	if job.RequiresItemPossession(hero, "job-01", "job-34") {
		t.Errorf("expected hero NOT to require item if old job is hero")
	}

	// Sage (job-33): exempt if coming from Asobinin (job-08)
	sage := job.Definition{ID: "job-33"}
	if job.RequiresItemPossession(sage, "job-08", "starter") {
		t.Errorf("expected sage NOT to require item from asobinin")
	}
	if job.RequiresItemPossession(sage, "job-01", "job-08") {
		t.Errorf("expected sage NOT to require item if old job was asobinin")
	}
	if !job.RequiresItemPossession(sage, "job-01", "starter") {
		t.Errorf("expected sage to require item from warrior")
	}

	// Gambler (job-46): STILL requires item-039 even if coming from Asobinin (job-08)
	gambler := job.Definition{ID: "job-46"}
	if !job.RequiresItemPossession(gambler, "job-08", "starter") {
		t.Errorf("expected gambler to require item possession even from asobinin")
	}
	if !job.RequiresItemPossession(gambler, "job-01", "job-08") {
		t.Errorf("expected gambler to require item possession even with asobinin old job")
	}
	if !job.RequiresItemPossession(gambler, "job-01", "starter") {
		t.Errorf("expected gambler to require item possession from warrior")
	}
	if job.RequiresItemPossession(gambler, "job-46", "starter") {
		t.Errorf("expected gambler NOT to require item if already gambler")
	}

	// FireFighter (job-84): uses armor, not standard item
	fireFighter := job.Definition{ID: "job-84"}
	if job.RequiresItemPossession(fireFighter, "job-01", "starter") {
		t.Errorf("expected firefighter NOT to require standard item")
	}

	// Non-item job (job-01)
	warrior := job.Definition{ID: "job-01"}
	if job.RequiresItemPossession(warrior, "job-02", "starter") {
		t.Errorf("expected warrior NOT to require item")
	}
}
