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

// CustomLoadout freezes a player's custom command at battle entry. Cost is the
// persisted requirement, already floored at zero unlike an NPC's aggregate.
type CustomLoadout struct {
	Name string    `json:"name"`
	Gems [3]string `json:"gems"`
	Cost int       `json:"cost"`
}

func (load *NPCLoadout) validate() error {
	if load == nil {
		return nil
	}
	if load.CustomCost < 0 {
		return ErrInvalidParticipant
	}
	for _, cost := range load.GemCosts {
		if cost < 0 {
			return ErrInvalidParticipant
		}
	}
	return nil
}

// ValidateOrdinaryNPC excludes shared hooks with no ordinary producer proof.
func (a LifetimeActor) ValidateOrdinaryNPC() error {
	if !a.NPC {
		return ErrInvalidParticipant
	}
	if hasAbility(a.Abilities, "specter") {
		return fmt.Errorf("%w: npc specter producer", ErrUnsupportedEffect)
	}
	return a.NPCLoadout.validate()
}

// CustomGemCost counts occupied slots, including repeated identities.
func CustomGemCost(gems [3]string, costs [3]int) int {
	count, sum := 0, 0
	for i, id := range gems {
		if id != "" {
			count++
			sum += max(0, costs[i])
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
