package adventure

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// recordConsumedItems carries one floor's combat consumption into the crawl
// run: each owner's held state is reduced so later floors cannot reuse the
// instance, and the consumption accumulates for the single final settlement.
func (s *CrawlSession) recordConsumedItems(consumed map[string][]corebattle.ConsumedItem) {
	for i := range s.Participants {
		p := &s.Participants[i]
		entries := consumed[p.ID]
		if len(entries) == 0 {
			continue
		}
		// Detach from slices shared with the caller-supplied participant.
		p.ActionItems = slices.Clone(p.ActionItems)
		p.Abilities = slices.Clone(p.Abilities)
		p.ItemDefinitionIDs = slices.Clone(p.ItemDefinitionIDs)
		for _, e := range entries {
			s.addConsumedItem(p.ID, e)
			runhost.RemoveHeldItem(p, e)
		}
	}
}

func (s *CrawlSession) addConsumedItem(ownerID string, e corebattle.ConsumedItem) {
	if s.ConsumedItems == nil {
		s.ConsumedItems = make(map[string][]corebattle.ConsumedItem)
	}
	for i := range s.ConsumedItems[ownerID] {
		have := &s.ConsumedItems[ownerID][i]
		if have.ID == e.ID && have.InstanceID == e.InstanceID {
			have.Quantity += e.Quantity
			return
		}
	}
	s.ConsumedItems[ownerID] = append(s.ConsumedItems[ownerID], e)
}

func cloneConsumedItems(m map[string][]corebattle.ConsumedItem) map[string][]corebattle.ConsumedItem {
	if len(m) == 0 {
		return nil
	}
	out := make(map[string][]corebattle.ConsumedItem, len(m))
	for k, v := range m {
		out[k] = slices.Clone(v)
	}
	return out
}
