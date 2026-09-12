package boss

import (
	"fmt"
	"math/rand"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
			p := corebattle.Participant{
				ID:      fmt.Sprintf("king99-clone-%d", i),
				Name:    "@" + a.Name,
				HP:      hp,
				MaxHP:   hp,
				MP:      mp,
				MaxMP:   mp,
				Attack:  a.Stats.Attack * 2,
				Defense: a.Stats.Defense * 2,
				Agility: a.Stats.Agility * 2,
				Skills: []corebattle.ActionSkill{
					{
						ID:     "dejon",
						Name:   "デジョン",
						Kind:   corebattle.ActionKindDejon,
						MPCost: 0,
					},
				},
			}
			participants = append(participants, p)
		}
		return participants
	}

	participants := make([]corebattle.Participant, 0, len(stage.Bosses))
	for i, b := range stage.Bosses {
		p := corebattle.Participant{
			ID:      fmt.Sprintf("%s-boss-%d", stage.ID, i),
			Name:    b.Name,
			HP:      b.HP,
			MaxHP:   b.MaxHP,
			MP:      b.MP,
			MaxMP:   b.MaxMP,
			Attack:  b.Attack,
			Defense: b.Defense,
			Agility: b.Agility,
			Skills: []corebattle.ActionSkill{
				{
					ID:     "dejon",
					Name:   "デジョン",
					Kind:   corebattle.ActionKindDejon,
					MPCost: 0,
				},
			},
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
		idx = rand.Intn(len(treasures))
	}
	return treasures[idx]
}

func (s *Service) calculateTotalExp(stage BossStage, allies []corecharacter.Character) int {
	if stage.ID == "king99" {
		return len(allies) * 3000
	}
	total := 0
	for _, b := range stage.Bosses {
		total += b.GetExp
	}
	return total
}

func (s *Service) calculateTotalGold(stage BossStage, allies []corecharacter.Character) int {
	if stage.ID == "king99" {
		return len(allies) * 1500
	}
	total := 0
	for _, b := range stage.Bosses {
		total += b.GetMoney
	}
	return total
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
