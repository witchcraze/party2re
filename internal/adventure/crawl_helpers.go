package adventure

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

func (s *CrawlSession) rngFloat() float64 {
	if s.RngFloat != nil {
		return s.RngFloat()
	}
	if s.Rng != nil {
		return float64(s.Rng(1000000)) / 1000000.0
	}
	return random.Float64()
}

func (s *CrawlSession) buildMonsterParticipant(
	id string,
	m Monster,
	partyAdjustment float64,
	isBoss bool,
) corebattle.Participant {
	hp := m.HP
	mp := m.MP
	atk := m.Attack
	def := m.Defense
	ag := m.Agility
	cmp := m.CMP
	expReward := m.ExperienceReward
	goldReward := m.GoldReward

	// Hard mode applies 1.5x multiplier to normal monster stats & rewards (lib/_npc_action.cgi:649-653)
	if !isBoss && s.Mode == CrawlModeHard {
		hp = int(float64(hp) * 1.5)
		mp = int(float64(mp) * 1.5)
		atk = int(float64(atk) * 1.5)
		def = int(float64(def) * 1.5)
		ag = int(float64(ag) * 1.5)
		cmp = int(float64(cmp) * 1.5)
		expReward = int(float64(expReward) * 1.5)
		goldReward = int(float64(goldReward) * 1.5)
	}

	statFactor := func() float64 {
		r := s.rngFloat()
		if r < 0 {
			r = 0
		}
		if r >= 1 {
			r = 0.9999999999999999
		}
		return 0.9 + r*0.3 + partyAdjustment
	}

	// Legacy stat scaling order: hp, mp, at, df, ag, c_mp (lib/_npc_action.cgi:612, 657)
	scaledHP := max(1, int(float64(hp)*statFactor()))
	scaledMP := max(0, int(float64(mp)*statFactor()))
	scaledAtk := max(0, int(float64(atk)*statFactor()))
	scaledDef := max(0, int(float64(def)*statFactor()))
	scaledAg := max(0, int(float64(ag)*statFactor()))
	scaledCMP := max(0, int(float64(cmp)*statFactor()))

	p := corebattle.MustNewParticipant(id, scaledHP, scaledAtk, scaledDef)
	p.Name = m.Name
	p.Agility = scaledAg
	p.MaxHP = scaledHP
	p.MP = scaledMP
	p.MaxMP = scaledMP
	p.CMP = scaledCMP
	p.MaxCMP = scaledCMP
	p.ExperienceReward = expReward
	p.GoldReward = goldReward

	return p
}

// Result compiles the final outcome of the dungeon crawl.
func (s *CrawlSession) Result() DungeonCrawlResult {
	participantHPs := make(map[string]int, len(s.Participants))
	participantMPs := make(map[string]int, len(s.Participants))
	for _, p := range s.Participants {
		participantHPs[p.ID] = p.HP
		participantMPs[p.ID] = p.MP
	}

	outcome := s.Outcome
	if outcome == "" {
		if s.StageCleared {
			outcome = corebattle.OutcomeWin
		} else {
			outcome = corebattle.OutcomeDefeat
		}
	}

	mode := s.Mode
	if mode == "" {
		mode = CrawlModeNormal
	}

	return DungeonCrawlResult{
		AdventureIDs:   make(map[string]string),
		StageID:        s.StageID,
		StageName:      s.StageName,
		Season:         s.Stage.Season,
		SeasonalSubID:  SeasonToSubID(s.Stage.Season),
		Mode:           mode,
		Round:          s.Round,
		ExitFatigue:    CalculateExitFatigue(s.Round, mode),
		FloorsCleared:  s.FloorsCleared,
		StageCleared:   s.StageCleared,
		Outcome:        outcome,
		TotalTurns:     s.TotalTurns,
		TotalEXP:       s.TotalEXP,
		TotalGold:      s.TotalGold,
		TotalCrystals:  s.TotalCrystals,
		FloorResults:   s.FloorResults,
		TreasureBoxes:  s.TreasureBoxes,
		InspectedBoxes: cloneInspectedBoxes(s.InspectedBoxes),
		PartySize:      len(s.Characters),
		ParticipantHPs: participantHPs,
		ParticipantMPs: participantMPs,
		ConsumedItems:  cloneConsumedItems(s.ConsumedItems),
	}
}

func buildFallbackParticipant(c corecharacter.Character) corebattle.Participant {
	hp := c.Stats.HP
	if hp <= 0 && c.Stats.MaxHP > 0 {
		hp = c.Stats.MaxHP
	}
	p := corebattle.MustNewParticipant(c.ID, hp, c.Stats.Attack, c.Stats.Defense)
	p.Name = c.Name
	p.MaxHP = c.Stats.MaxHP
	p.MP = c.Stats.MP
	p.MaxMP = c.Stats.MaxMP
	p.Agility = c.Stats.Agility
	return p
}

func cloneInspectedBoxes(m map[string]int) map[string]int {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

func mathClamp(val, min, max int) int {
	if val < min {
		return min
	}
	if val > max {
		return max
	}
	return val
}
