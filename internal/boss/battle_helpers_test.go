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

func TestBuildBossParticipants_NormalBoss(t *testing.T) {
	svc := &Service{}
	stage := stageByID("king1") // 破壊神 + 6 stones
	participants := svc.buildBossParticipants(stage, nil)
	if len(participants) != 7 {
		t.Fatalf("expected 7 participants, got %d", len(participants))
	}
	var hakai, redStone corebattle.Participant
	for _, p := range participants {
		if p.Name == "破壊神" {
			hakai = p
		}
		if p.Name == "レッドストーン" {
			redStone = p
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

	// レッドストーン has TMP "魔無効" and Job 26 + OldJob 6 skills
	if len(redStone.Abilities) != 1 || redStone.Abilities[0] != "魔無効" {
		t.Errorf("expected レッドストーン ability '魔無効', got %v", redStone.Abilities)
	}
	if len(redStone.Skills) != 9 { // dejon + 3 ninja skills + 5 mage skills
		t.Errorf("expected 9 skills for レッドストーン, got %d", len(redStone.Skills))
	}
}

func TestBuildBossParticipants_King99Clones(t *testing.T) {
	svc := &Service{}
	stage := stageByID("king99")
	allies := []corecharacter.Character{
		{
			ID:       "ally-1",
			Name:     "HeroA",
			JobID:    "job-26", // 忍者
			OldJobID: "job-6",  // 魔法使い
			SP:       100,
			OldSP:    100,
			Stats: corecharacter.Stats{
				HP:      200,
				MaxHP:   200,
				MP:      50,
				MaxMP:   50,
				Attack:  100,
				Defense: 80,
				Agility: 60,
			},
		},
	}
	participants := svc.buildBossParticipants(stage, allies)
	if len(participants) != 1 {
		t.Fatalf("expected 1 clone participant, got %d", len(participants))
	}
	clone := participants[0]
	if clone.Name != "@HeroA" {
		t.Errorf("expected clone name '@HeroA', got %s", clone.Name)
	}
	if len(clone.Abilities) != 1 || clone.Abilities[0] != "大防御" {
		t.Errorf("expected clone ability '大防御', got %v", clone.Abilities)
	}
	if clone.HP != 200*50 {
		t.Errorf("expected clone HP 10000, got %d", clone.HP)
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
}

func TestCalculateTotalExpAndGold_King99(t *testing.T) {
	svc := &Service{}
	stage := stageByID("king99")
	allies := []corecharacter.Character{
		{
			Level:    50,
			JobLevel: 30,
		},
		{
			Level:    70,
			JobLevel: 50,
		},
	}

	// EXP: (50 + 30)*30 + (70 + 50)*30 = 2400 + 3600 = 6000
	totalExp := svc.calculateTotalExp(stage, allies)
	if totalExp != 6000 {
		t.Errorf("expected 6000 EXP, got %d", totalExp)
	}

	// Gold: int(50*0.5)*30 + int(70*0.5)*30 = 25*30 + 35*30 = 750 + 1050 = 1800
	totalGold := svc.calculateTotalGold(stage, allies)
	if totalGold != 1800 {
		t.Errorf("expected 1800 Gold, got %d", totalGold)
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

func TestCalculateTotalCrystals(t *testing.T) {
	// Normal stage: king1 has 7 bosses with total GetCrystal = 40 + 5*6 = 70
	stage := stageByID("king1")
	allies := []corecharacter.Character{{ID: "a"}}

	cFast := calculateTotalCrystals(stage, allies, 5*time.Minute) // 70 * 3.0 = 210
	if cFast != 210 {
		t.Errorf("expected 210 crystals for <=10m clear, got %d", cFast)
	}

	cSlow := calculateTotalCrystals(stage, allies, 70*time.Minute) // 70 * 1.0 = 70
	if cSlow != 70 {
		t.Errorf("expected 70 crystals for >60m clear, got %d", cSlow)
	}

	// King 99: base is len(allies) * 20
	stage99 := stageByID("king99")
	allies3 := []corecharacter.Character{{ID: "a"}, {ID: "b"}, {ID: "c"}}
	c99Fast := calculateTotalCrystals(stage99, allies3, 5*time.Minute) // 3 * 20 * 3.0 = 180
	if c99Fast != 180 {
		t.Errorf("expected 180 crystals for king99 3 allies <=10m clear, got %d", c99Fast)
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
