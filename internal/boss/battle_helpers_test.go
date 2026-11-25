package boss

import (
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type fixedRNG struct {
	val int
}

func (f fixedRNG) Intn(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	return f.val % max, nil
}

func TestBossSkillsForJob(t *testing.T) {
	// Job 97: 破壊神 should have 5 skills
	skills97 := bossSkillsForJob(97, 999)
	if len(skills97) != 5 {
		t.Fatalf("expected 5 skills for job 97, got %d", len(skills97))
	}
	expectedIDs := []string{"midareuchi", "bakuretsuken", "ankokuken", "shikkokunohonoo", "jigospark"}
	for i, id := range expectedIDs {
		if skills97[i].ID != id {
			t.Errorf("job 97 skill[%d]: expected %s, got %s", i, id, skills97[i].ID)
		}
	}

	// Job 26: 忍者 with SP 10 should only get kaennonoiki (req: 5)
	skills26Low := bossSkillsForJob(26, 10)
	if len(skills26Low) != 1 || skills26Low[0].ID != "kaennonoiki" {
		t.Errorf("expected only kaennonoiki for job 26 at SP 10, got %v", skills26Low)
	}

	// Job 26: with SP 50 should get kaennonoiki, yaketsukuiki, moudokunokiri
	skills26High := bossSkillsForJob(26, 50)
	if len(skills26High) != 3 {
		t.Fatalf("expected 3 skills for job 26 at SP 50, got %d", len(skills26High))
	}
	if skills26High[1].Status != corebattle.StatusParalyze {
		t.Errorf("expected yaketsukuiki to have paralyze status, got %s", skills26High[1].Status)
	}
	if skills26High[2].Status != corebattle.StatusPoison {
		t.Errorf("expected moudokunokiri to have poison status, got %s", skills26High[2].Status)
	}

	// Invalid or 0 job ID returns nil
	if skills := bossSkillsForJob(0, 999); skills != nil {
		t.Errorf("expected nil for job 0, got %v", skills)
	}
}

func stageByID(id string) BossStage {
	for _, s := range DefaultBossCatalog() {
		if s.ID == id {
			return s
		}
	}
	return BossStage{}
}

func TestStageRoster_CatalogBosses(t *testing.T) {
	roster := stageRoster(stageByID("king1"), nil) // 破壊神 + 6 stones
	if len(roster) != 7 {
		t.Fatalf("expected 7 enemies, got %d", len(roster))
	}
	var hakai, redStone corebattle.LifetimeActor
	for _, e := range roster {
		if e.Name == "破壊神" {
			hakai = e
		}
		if e.Name == "レッドストーン" {
			redStone = e
		}
	}

	// 破壊神 has TMP "攻軽減" and Job 97 skills
	if len(hakai.Abilities) != 1 || hakai.Abilities[0] != "攻軽減" {
		t.Errorf("expected 破壊神 ability '攻軽減', got %v", hakai.Abilities)
	}
	if len(hakai.Skills) != 6 { // dejon + 5 job 97 skills
		t.Errorf("expected 6 skills (dejon + 5 job skills), got %d", len(hakai.Skills))
	}
	if hakai.Skills[0].ID != "dejon" {
		t.Errorf("expected first skill to be dejon, got %s", hakai.Skills[0].ID)
	}
	if hakai.CrystalReward != 40 || !hakai.NPC {
		t.Errorf("expected an NPC carrying 40 crystals, got %+v", hakai)
	}

	// レッドストーン has TMP "魔無効" and Job 26 + OldJob 6 skills
	if len(redStone.Abilities) != 1 || redStone.Abilities[0] != "魔無効" {
		t.Errorf("expected レッドストーン ability '魔無効', got %v", redStone.Abilities)
	}
	if len(redStone.Skills) != 9 { // dejon + 3 ninja skills + 5 mage skills
		t.Errorf("expected 9 skills for レッドストーン, got %d", len(redStone.Skills))
	}
	if redStone.ExperienceReward != 1000 || redStone.GoldReward != 2000 || redStone.Icon != "mon/190.gif" {
		t.Errorf("unexpected catalog rewards: %+v", redStone.Participant)
	}
}

func TestStageRoster_King99ClonesUnsealers(t *testing.T) {
	unsealer := corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID: "ally-1", Name: "HeroA", HP: 1, MaxHP: 200, MaxMP: 50, Attack: 100, Defense: 80, Agility: 60,
			Icon: "hero.png", ExperienceReward: 60, GoldReward: 25,
		},
		Jobs: corebattle.BattleJobs{JobID: "job-26", SP: 100, OldJobID: "job-6", OldSP: 100}, // 忍者 / 魔法使い
	}
	roster := stageRoster(stageByID("king99"), []corebattle.LifetimeActor{unsealer})
	if len(roster) != 1 {
		t.Fatalf("expected 1 clone, got %d", len(roster))
	}
	clone := roster[0]
	if clone.Name != "@HeroA" || clone.Icon != "hero.png" {
		t.Errorf("expected clone '@HeroA' with the unsealer's icon, got %s %s", clone.Name, clone.Icon)
	}
	if len(clone.Abilities) != 1 || clone.Abilities[0] != "大防御" {
		t.Errorf("expected clone ability '大防御', got %v", clone.Abilities)
	}
	// Max HP/MP, not the current HP, scale by 50 (stage/king99.cgi).
	if clone.HP != 200*50 || clone.MaxHP != 200*50 || clone.MP != 50*50 {
		t.Errorf("expected clone HP 10000 and MP 2500, got %d/%d", clone.HP, clone.MP)
	}
	if clone.Attack != 200 || clone.Defense != 160 || clone.Agility != 120 {
		t.Errorf("expected 2x stats, got Atk=%d Def=%d Agi=%d", clone.Attack, clone.Defense, clone.Agility)
	}
	if clone.Skills[0].ID != "dejon" {
		t.Errorf("expected first skill to be dejon, got %s", clone.Skills[0].ID)
	}
	// Clone should have inherited Ninja skills (3) + Mage skills (5) + dejon = 9 skills
	if len(clone.Skills) != 9 {
		t.Errorf("expected 9 skills for clone, got %d", len(clone.Skills))
	}
	if clone.ExperienceReward != 60*30 || clone.GoldReward != 25*30 || clone.CrystalReward != 20 {
		t.Errorf("expected rewards 1800/750/20, got %d/%d/%d", clone.ExperienceReward, clone.GoldReward, clone.CrystalReward)
	}
}

func TestUnsealerFromCharacterUsesBattleRowRewards(t *testing.T) {
	unsealer := unsealerFromCharacter(corecharacter.Character{ID: "c1", Name: "Hero", Level: 50, JobLevel: 10, Stats: corecharacter.Stats{MaxHP: 600, MaxMP: 100, Attack: 300}})
	if unsealer.ExperienceReward != 60 || unsealer.GoldReward != 25 || unsealer.MaxHP != 600 || unsealer.Attack != 300 {
		t.Errorf("unexpected battle row: %+v", unsealer.Participant)
	}
}

func TestRosterRewards_King99(t *testing.T) {
	roster := stageRoster(stageByID("king99"), []corebattle.LifetimeActor{
		unsealerFromCharacter(corecharacter.Character{Level: 50, JobLevel: 30}),
		unsealerFromCharacter(corecharacter.Character{Level: 70, JobLevel: 50}),
	})
	// EXP: (50 + 30)*30 + (70 + 50)*30 = 6000; Gold: int(50*0.5)*30 + int(70*0.5)*30 = 1800
	if exp, gold := rosterRewards(roster); exp != 6000 || gold != 1800 {
		t.Errorf("expected 6000 EXP and 1800 Gold, got %d and %d", exp, gold)
	}
}

func TestClearTimeCrystalMultipliers(t *testing.T) {
	tests := []struct {
		elapsed  time.Duration
		expected float64
	}{
		{5 * time.Minute, 3.0},
		{10 * time.Minute, 3.0},
		{10*time.Minute + time.Second, 2.0},
		{25 * time.Minute, 2.0},
		{30 * time.Minute, 2.0},
		{30*time.Minute + time.Second, 1.5},
		{45 * time.Minute, 1.5},
		{60 * time.Minute, 1.5},
		{60*time.Minute + time.Second, 1.0},
		{120 * time.Minute, 1.0},
	}

	for _, tt := range tests {
		mult := calculateCrystalMultiplier(tt.elapsed)
		if mult != tt.expected {
			t.Errorf("for elapsed %v: expected mult %.1f, got %.1f", tt.elapsed, tt.expected, mult)
		}
	}
}

func TestRosterCrystals(t *testing.T) {
	now := time.Date(2026, 11, 15, 12, 0, 0, 0, time.UTC)
	atElapsed := func(d time.Duration) *time.Time {
		ts := now.Add(-d)
		return &ts
	}

	// Normal stage: king1 has 6 stones (GetCrystal=5 each) + 1 boss (GetCrystal=40), sum=70.
	// Note the 1.5x bracket truncates per enemy: 6*int(5*1.5) + int(40*1.5) = 6*7 + 60 = 102 (NOT int(70*1.5)=105).
	roster := stageRoster(stageByID("king1"), nil)

	tests := []struct {
		name       string
		unsealedAt *time.Time
		expected   int
	}{
		{"not yet unsealed", nil, 70},
		{"600s boundary (3x)", atElapsed(600 * time.Second), 210},
		{"601s boundary (2x)", atElapsed(601 * time.Second), 140},
		{"1800s boundary (2x)", atElapsed(1800 * time.Second), 140},
		{"1801s boundary (1.5x per enemy)", atElapsed(1801 * time.Second), 102},
		{"3600s boundary (1.5x per enemy)", atElapsed(3600 * time.Second), 102},
		{"3601s boundary (1x)", atElapsed(3601 * time.Second), 70},
	}
	for _, tt := range tests {
		t.Run("king1/"+tt.name, func(t *testing.T) {
			if got := rosterCrystals(roster, tt.unsealedAt, now); got != tt.expected {
				t.Errorf("got %d crystals, want %d", got, tt.expected)
			}
		})
	}

	// King 99: 20 crystals per defeated clone, multiplier applied per clone before summing
	roster99 := stageRoster(stageByID("king99"), make([]corebattle.LifetimeActor, 3))
	king99Tests := []struct {
		name       string
		unsealedAt *time.Time
		expected   int
	}{
		{"not yet unsealed", nil, 60},
		{"600s (3x)", atElapsed(600 * time.Second), 180},
		{"1801s (1.5x)", atElapsed(1801 * time.Second), 90},
		{"3601s (1x)", atElapsed(3601 * time.Second), 60},
	}
	for _, tt := range king99Tests {
		t.Run("king99/"+tt.name, func(t *testing.T) {
			if got := rosterCrystals(roster99, tt.unsealedAt, now); got != tt.expected {
				t.Errorf("got %d crystals, want %d", got, tt.expected)
			}
		})
	}
}

func TestGetDayOfWeekOrb(t *testing.T) {
	// Monday (wday = 1) -> item-060
	mon := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC) // 2026-09-21 is Monday
	orbMon := getDayOfWeekOrb(mon, nil)
	if orbMon != "item-060" {
		t.Errorf("expected item-060 for Monday, got %s", orbMon)
	}

	// Saturday (wday = 6) -> item-065
	sat := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC) // 2026-09-26 is Saturday
	orbSat := getDayOfWeekOrb(sat, nil)
	if orbSat != "item-065" {
		t.Errorf("expected item-065 for Saturday, got %s", orbSat)
	}

	// Sunday (wday = 0) -> randomized with RNG
	sun := time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) // 2026-09-27 is Sunday
	orbSun := getDayOfWeekOrb(sun, fixedRNG{val: 3})
	if orbSun != "item-063" {
		t.Errorf("expected item-063 for Sunday with rng=3, got %s", orbSun)
	}
}

func TestGetDayOfWeekOrb_JSTBoundary(t *testing.T) {
	// 2026-09-27 23:00:00 UTC is 2026-09-28 08:00:00 JST (Monday).
	// UTC evaluates to Sunday, but in JST it is Monday -> item-060.
	utcSunJstMon := time.Date(2026, 9, 27, 23, 0, 0, 0, time.UTC)
	orb := getDayOfWeekOrb(utcSunJstMon, nil)
	if orb != "item-060" {
		t.Errorf("expected item-060 for JST Monday morning (UTC Sunday 23:00), got %s", orb)
	}

	// 2026-09-27 14:59:59 UTC is 2026-09-27 23:59:59 JST (Sunday).
	// Sunday -> random orb among item-060..item-065 (with fixedRNG val: 2 -> item-062).
	utcSunJstSun := time.Date(2026, 9, 27, 14, 59, 59, 0, time.UTC)
	orbSun := getDayOfWeekOrb(utcSunJstSun, fixedRNG{val: 2})
	if orbSun != "item-062" {
		t.Errorf("expected item-062 for JST Sunday night, got %s", orbSun)
	}

	// 2026-09-27 15:00:00 UTC is 2026-09-28 00:00:00 JST (Monday 00:00 JST).
	// Boundary check at exactly midnight JST -> item-060.
	utcSunJstMonMidnight := time.Date(2026, 9, 27, 15, 0, 0, 0, time.UTC)
	orbMonMidnight := getDayOfWeekOrb(utcSunJstMonMidnight, nil)
	if orbMonMidnight != "item-060" {
		t.Errorf("expected item-060 for JST Monday 00:00 (UTC Sunday 15:00), got %s", orbMonMidnight)
	}
}
