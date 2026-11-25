package boss

import (
	"fmt"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

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

// rosterRewards sums the EXP and gold the roster's enemies carry.
func rosterRewards(roster []corebattle.LifetimeActor) (exp, gold int) {
	for _, e := range roster {
		exp += e.ExperienceReward
		gold += e.GoldReward
	}
	return exp, gold
}

func rosterParticipants(roster []corebattle.LifetimeActor) []corebattle.Participant {
	participants := make([]corebattle.Participant, len(roster))
	for i, e := range roster {
		participants[i] = e.Participant
	}
	return participants
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

// rosterCrystals applies the unseal-clock multiplier to each enemy's crystals.
func rosterCrystals(roster []corebattle.LifetimeActor, unsealedAt *time.Time, now time.Time) int {
	mult := 1.0
	if unsealedAt != nil {
		mult = calculateCrystalMultiplier(max(now.Sub(*unsealedAt), 0))
	}
	total := 0
	for _, e := range roster {
		total += int(float64(e.CrystalReward) * mult)
	}
	return total
}

func getDayOfWeekOrb(t time.Time, rng corecharacter.RandomSource) string {
	jst := time.FixedZone("JST", 9*60*60)
	wday := t.In(jst).Weekday() // 0 = Sunday, 1 = Monday, ..., 6 = Saturday
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

func parseBossMonsterID(icon, defaultID string) string {
	if icon != "" {
		trimmed := strings.TrimPrefix(icon, "mon/")
		trimmed = strings.TrimSuffix(trimmed, ".gif")
		if trimmed != "" {
			return "monster-" + trimmed
		}
	}
	return defaultID
}
