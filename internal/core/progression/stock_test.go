package progression

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
)

type stockGrowthRNG struct {
	random.Generator
	value float64
}

func (r stockGrowthRNG) Float64() float64 { return r.value }

func TestStockLevelsGrowthUsesRequestStartSPAndHeldOrb(t *testing.T) {
	for _, tc := range []struct {
		name, job, held string
		sp, wantSP      int
		draw            float64
		gain            character.Stats
	}{
		{"job 49 fractional bound", "job-49", "", 25, 27, .8, character.Stats{MaxHP: 2, MaxMP: 1, Attack: 1, Defense: 1, Agility: 1}},
		{"job 49 keeps its load-time bound when SP crosses 50", "job-49", "", 49, 51, .502, character.Stats{MaxHP: 1}},
		{"matching active stat orb", "job-01", ItemDefenseStatOrb, 20, 22, .99, character.Stats{MaxHP: 3, MaxMP: 2, Attack: 2, Defense: 3, Agility: 2}},
		{"skill orb bonus applies per level", "job-01", "item-157", 20, 24, 0, character.Stats{MaxHP: 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := character.Character{Level: 1, Experience: 40, SP: tc.sp}
			def := job.Definition{ID: tc.job, HPGrowth: 2, MPGrowth: 2, AttackGrowth: 2, DefenseGrowth: 2, AgilityGrowth: 2}
			gains, err := ApplyStockLevels(&c, def, stockGrowthRNG{value: tc.draw}, tc.held, false)
			if err != nil || len(gains) != 2 || gains[0] != tc.gain || gains[1] != tc.gain || c.Level != 3 || c.SP != tc.wantSP || c.Experience != 40 {
				t.Fatalf("growth = %+v, level/SP/EXP = %d/%d/%d, err = %v", gains, c.Level, c.SP, c.Experience, err)
			}
		})
	}
}
