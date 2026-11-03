package battle

import "fmt"

// NPCLoadout preserves supplied catalog facts independently of hydrated callbacks.
// Source is provenance only; display names and assets are native.
type NPCLoadout struct {
	Source     string    `json:"source"`
	CustomName string    `json:"custom_name,omitempty"`
	Gems       [3]string `json:"gems,omitempty"`
	GemCosts   [3]int    `json:"gem_costs,omitempty"`
	CustomCost int       `json:"custom_cost,omitempty"`
}

// ValidateOrdinaryNPC excludes shared hooks with no ordinary producer proof.
func (a LifetimeActor) ValidateOrdinaryNPC() error {
	if !a.NPC {
		return ErrInvalidParticipant
	}
	if hasAbility(a.Abilities, "specter") {
		return fmt.Errorf("%w: npc specter producer", ErrUnsupportedEffect)
	}
	return nil
}

// CustomGemCost counts occupied slots, including repeated identities.
func CustomGemCost(gems [3]string, costs [3]int) int {
	count, sum := 0, 0
	for i, id := range gems {
		if id != "" {
			count++
			sum += costs[i]
		}
	}
	rate := 1.0
	if count == 2 {
		rate = 1.1
	}
	if count == 3 {
		rate = 1.2
	}
	return int(float64(sum) * rate)
}
