package adventure

import (
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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
			removeHeldItem(p, e)
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

// removeHeldItem drops the consumed instance(s) from the owner's carried
// combat state. The Dokuro amulet is held as a passive revival ability rather
// than an ActionItem, so its ability and definition are released with it.
func removeHeldItem(p *corebattle.Participant, e corebattle.ConsumedItem) {
	for n := 0; n < e.Quantity; n++ {
		idx := slices.IndexFunc(p.ActionItems, func(it corebattle.ActionItem) bool {
			return it.ID == e.ID && it.InstanceID == e.InstanceID
		})
		if idx < 0 {
			break
		}
		p.ActionItems = slices.Delete(p.ActionItems, idx, idx+1)
	}
	if e.ID == battle.ItemDokuroAmulet {
		p.Abilities = slices.DeleteFunc(p.Abilities, func(a string) bool { return a == "dokuro_amulet" })
	}
	for n := 0; n < e.Quantity; n++ {
		if idx := slices.Index(p.ItemDefinitionIDs, e.ID); idx >= 0 {
			p.ItemDefinitionIDs = slices.Delete(p.ItemDefinitionIDs, idx, idx+1)
		}
	}
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
