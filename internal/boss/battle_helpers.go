package boss

import (
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *Service) buildBossParticipants(stage BossStage, allies []corecharacter.Character) []corebattle.Participant {
	if stage.ID == "king99" {
		participants := make([]corebattle.Participant, 0, len(allies))
		for i, a := range allies {
			hp := a.Stats.MaxHP * 50
			if hp <= 0 {
				hp = 5000
			}
			mp := a.Stats.MaxMP * 50
			if mp <= 0 {
				mp = 500
			}

			skills := []corebattle.ActionSkill{
				{
					ID:     "dejon",
					Name:   "デジョン",
					Kind:   corebattle.ActionKindDejon,
					MPCost: 0,
				},
			}
			jobIDNum := parseJobID(a.JobID)
			oldJobIDNum := parseJobID(a.OldJobID)
			skills = append(skills, bossSkillsForJob(jobIDNum, a.SP)...)
			skills = append(skills, bossSkillsForJob(oldJobIDNum, a.OldSP)...)

			p := corebattle.Participant{
				ID:        fmt.Sprintf("king99-clone-%d", i),
				Name:      "@" + a.Name,
				HP:        hp,
				MaxHP:     hp,
				MP:        mp,
				MaxMP:     mp,
				Attack:    a.Stats.Attack * 2,
				Defense:   a.Stats.Defense * 2,
				Agility:   a.Stats.Agility * 2,
				Abilities: []string{"大防御"},
				Skills:    skills,
			}
			participants = append(participants, p)
		}
		return participants
	}

	participants := make([]corebattle.Participant, 0, len(stage.Bosses))
	for i, b := range stage.Bosses {
		var abilities []string
		if b.TMP != "" {
			abilities = []string{b.TMP}
		}

		sp := b.SP
		if sp <= 0 && b.Job > 0 {
			sp = 999
		}
		oldSP := b.OldSP
		if oldSP <= 0 && b.OldJob > 0 {
			oldSP = 999
		}

		skills := []corebattle.ActionSkill{
			{
				ID:     "dejon",
				Name:   "デジョン",
				Kind:   corebattle.ActionKindDejon,
				MPCost: 0,
			},
		}
		skills = append(skills, bossSkillsForJob(b.Job, sp)...)
		skills = append(skills, bossSkillsForJob(b.OldJob, oldSP)...)

		p := corebattle.Participant{
			ID:        fmt.Sprintf("%s-boss-%d", stage.ID, i),
			Name:      b.Name,
			HP:        b.HP,
			MaxHP:     b.MaxHP,
			MP:        b.MP,
			MaxMP:     b.MaxMP,
			Attack:    b.Attack,
			Defense:   b.Defense,
			Agility:   b.Agility,
			Abilities: abilities,
			Skills:    skills,
		}
		participants = append(participants, p)
	}
	return participants
}

func (s *Service) pickTreasure(treasures []string) string {
	if len(treasures) == 0 {
		return ""
	}
	idx := 0
	if s.rng != nil {
		if n, err := s.rng.Intn(len(treasures)); err == nil {
			idx = n
		}
	} else {
		idx = random.Intn(len(treasures))
	}
	return treasures[idx]
}

func (s *Service) calculateTotalExp(stage BossStage, allies []corecharacter.Character) int {
	if stage.ID == "king99" {
		total := 0
		for _, a := range allies {
			total += (a.Level + a.JobLevel) * 30
		}
		return total
	}
	total := 0
	for _, b := range stage.Bosses {
		total += b.GetExp
	}
	return total
}

func (s *Service) calculateTotalGold(stage BossStage, allies []corecharacter.Character) int {
	if stage.ID == "king99" {
		total := 0
		for _, a := range allies {
			total += int(float64(a.Level)*0.5) * 30
		}
		return total
	}
	total := 0
	for _, b := range stage.Bosses {
		total += b.GetMoney
	}
	return total
}

func calculateCrystalMultiplier(elapsed time.Duration) float64 {
	secs := elapsed.Seconds()
	if secs <= 600 {
		return 3.0
	}
	if secs <= 1800 {
		return 2.0
	}
	if secs <= 3600 {
		return 1.5
	}
	return 1.0
}

func calculateTotalCrystals(stage BossStage, allies []corecharacter.Character, elapsed time.Duration) int {
	base := 0
	if stage.ID == "king99" {
		base = len(allies) * 20
	} else {
		for _, b := range stage.Bosses {
			base += b.GetCrystal
		}
	}
	mult := calculateCrystalMultiplier(elapsed)
	return int(float64(base) * mult)
}

func getDayOfWeekOrb(t time.Time, rng corecharacter.RandomSource) string {
	wday := t.Weekday() // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
	if wday == time.Sunday {
		offset := 0
		if rng != nil {
			if n, err := rng.Intn(6); err == nil {
				offset = n
			}
		} else {
			offset = random.Intn(6)
		}
		return fmt.Sprintf("item-%03d", 60+offset)
	}
	return fmt.Sprintf("item-%03d", 60+int(wday)-1)
}

func stageTierFromID(id string) int {
	switch id {
	case "king1":
		return 1
	case "king2":
		return 2
	case "king3":
		return 3
	case "king4":
		return 4
	case "king5":
		return 5
	case "king6":
		return 6
	case "king7":
		return 7
	case "king8":
		return 8
	case "king9":
		return 9
	case "king10":
		return 10
	case "king99":
		return 99
	default:
		return 1
	}
}

func buildFallbackParticipant(char corecharacter.Character) corebattle.Participant {
	hp := char.Stats.HP
	if hp <= 0 && char.Stats.MaxHP > 0 {
		hp = char.Stats.MaxHP
	}
	p := corebattle.MustNewParticipant(char.ID, hp, char.Stats.Attack, char.Stats.Defense)
	p.Name = char.Name
	p.MaxHP = char.Stats.MaxHP
	p.MP = char.Stats.MP
	p.MaxMP = char.Stats.MaxMP
	p.Agility = char.Stats.Agility
	return p
}
