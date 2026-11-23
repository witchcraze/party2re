package runhost

import (
	"context"
	"maps"
	"reflect"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

// ItemTreasurePocket opens one more box than the per-member allowance (_data.cgi:1913).
const ItemTreasurePocket = "item-215"

type TreasureBox struct {
	ID          string                    `json:"id"`
	Name        string                    `json:"name"`
	OpenedBy    string                    `json:"opened_by,omitempty"`
	Defeated    bool                      `json:"defeated,omitempty"`
	ItemID      string                    `json:"item_id,omitempty"`
	ItemName    string                    `json:"item_name,omitempty"`
	DeliveredTo string                    `json:"delivered_to,omitempty"` // "inventory" or "depot"
	Fixed       bool                      `json:"fixed,omitempty"`        // EX fixed-item chest (over_lv); never uses the ordinary allowance
	Combat      *corebattle.LifetimeActor `json:"combat,omitempty"`
}

func (b TreasureBox) IsUnopened() bool {
	return b.OpenedBy == "" && !b.Defeated
}

func (b TreasureBox) defaultCombatActor() corebattle.LifetimeActor {
	hp, df := 1000, 1000
	if b.Fixed {
		hp, df = 2000, 2000
	}
	jobs := corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID:        b.ID,
			Name:      b.Name,
			TeamID:    "enemies",
			HP:        hp,
			MaxHP:     hp,
			Defense:   df,
			BaseStats: corebattle.BattleBaseStats{Defense: df},
			Gear:      &corebattle.BattleGearStats{},
			OverLevel: b.Fixed,
		},
		Jobs:         jobs,
		OriginalJobs: jobs,
		Tension:      1,
		Temporary:    corebattle.TemporaryNullMagic,
		NPC:          true,
	}
}

func (b TreasureBox) combatActor() corebattle.LifetimeActor {
	if b.Combat != nil {
		return b.Combat.Clone()
	}
	return b.defaultCombatActor()
}

func (r *State) HasBox(id string) bool {
	return slices.ContainsFunc(r.Boxes, func(b TreasureBox) bool { return b.ID == id })
}

// ClaimChest is shiraberu (_npc_action.cgi:6): one box per member, more only
// by consuming a Treasure Pocket from inventories; a fixed EX chest ignores
// the allowance.
func (r *State) ClaimChest(ctx context.Context, inventories Inventories, actorID, boxID string) (*TreasureBox, map[string][]corebattle.ConsumedItem, error) {
	if len(r.Boxes) == 0 {
		return nil, nil, ErrInvalidAction
	}
	a := r.Actor(actorID)
	if a == nil || a.HP <= 0 {
		return nil, nil, ErrActorUnconscious
	}
	i := slices.IndexFunc(r.Boxes, func(b TreasureBox) bool { return b.ID == boxID })
	if i < 0 || !r.Boxes[i].IsUnopened() {
		return nil, nil, ErrNoUnopenedTreasure
	}
	consumed := map[string][]corebattle.ConsumedItem{}
	if r.Boxes[i].Fixed {
		r.Boxes[i].OpenedBy = actorID
		r.Boxes[i].Combat = nil
		return &r.Boxes[i], consumed, nil
	}
	p := r.Recipients[actorID]
	if p.Inspected > 0 {
		if inventories == nil {
			return nil, nil, ErrUnavailable
		}
		inv, err := inventories.FindByCharacterID(ctx, actorID)
		if err != nil {
			return nil, nil, err
		}
		for _, item := range inv.Items {
			if item.DefinitionID == ItemTreasurePocket && item.Quantity > 0 {
				entry := corebattle.ConsumedItem{ID: item.DefinitionID, InstanceID: item.ID, Quantity: 1}
				consumed[actorID] = []corebattle.ConsumedItem{entry}
				RemoveHeldItem(&a.Participant, entry)
				break
			}
		}
		if len(consumed[actorID]) == 0 {
			return nil, nil, ErrTreasureLimitReached
		}
	}
	p.Inspected++
	r.Recipients[actorID] = p
	r.Boxes[i].OpenedBy = actorID
	r.Boxes[i].Combat = nil
	return &r.Boxes[i], consumed, nil
}

// RemoveHeldItem drops the consumed instance(s) from the owner's carried
// combat state. The Dokuro amulet is held as a passive revival ability rather
// than an ActionItem, so its ability and definition are released with it.
func RemoveHeldItem(p *corebattle.Participant, e corebattle.ConsumedItem) {
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

// UpdateTreasureDeliveries records where the settler delivered each opened box.
func UpdateTreasureDeliveries(boxes []TreasureBox, resp battle.ApplyPostBattleResponse) {
	remaining := func(drops map[string][]item.Instance) map[string][]string {
		out := make(map[string][]string)
		for cID, items := range drops {
			for _, inst := range items {
				out[cID] = append(out[cID], inst.DefinitionID)
			}
		}
		return out
	}
	remainingInv, remainingDepot, remainingLost := remaining(resp.InventoryDrops), remaining(resp.DepotDeliveries), remaining(resp.LostDrops)
	for i := range boxes {
		box := &boxes[i]
		if box.OpenedBy == "" || box.ItemID == "" {
			continue
		}
		cID := box.OpenedBy
		box.DeliveredTo = "inventory"
		for _, dest := range []struct {
			name  string
			items map[string][]string
		}{{"inventory", remainingInv}, {"depot", remainingDepot}, {"lost", remainingLost}} {
			if idx := slices.Index(dest.items[cID], box.ItemID); idx != -1 {
				box.DeliveredTo = dest.name
				dest.items[cID] = slices.Delete(dest.items[cID], idx, idx+1)
				break
			}
		}
	}
}

// ProjectCombat projects durable battle state and unopened TreasureBoxes into
// an operation-local LifetimeState, TargetContext, and inspection metadata map
// without duplicating boxes into r.Battle.Actors.
func (r *State) ProjectCombat(actorID string) (corebattle.LifetimeState, corebattle.TargetContext, map[string]corebattle.InspectionContent, error) {
	if r == nil || r.Actor(actorID) == nil {
		return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrNotParticipant
	}
	state := CloneBattle(r.Battle)
	frame := corebattle.TargetContext{ActorID: actorID}
	for _, id := range r.Enemies {
		if enemy := FindActor(state.Actors, id); enemy != nil && enemy.HP > 0 {
			frame.EnemyIDs = append(frame.EnemyIDs, id)
		}
	}
	var catalog *item.Catalog
	for _, box := range r.Boxes {
		if !box.IsUnopened() {
			continue
		}
		if box.ID == "" || FindActor(state.Actors, box.ID) != nil {
			return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrConflict
		}
		state.Actors = append(state.Actors, box.combatActor())
		frame.EnemyIDs = append(frame.EnemyIDs, box.ID)
		if box.ItemID != "" && catalog == nil {
			var err error
			catalog, err = item.DefaultCatalog()
			if err != nil {
				return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, err
			}
		}
	}
	team := r.Actor(actorID).TeamID
	for _, member := range state.Actors {
		switch {
		case member.TeamID == team:
			frame.PartyIDs = append(frame.PartyIDs, member.ID)
		case !slices.Contains(frame.EnemyIDs, member.ID):
			frame.MemberOnlyIDs = append(frame.MemberOnlyIDs, member.ID)
		}
	}
	inspection := make(map[string]corebattle.InspectionContent, len(state.Actors))
	for _, member := range state.Actors {
		inspection[member.ID] = corebattle.InspectionContent{Empty: true}
	}
	for _, box := range r.Boxes {
		if !box.IsUnopened() {
			continue
		}
		if box.ItemID == "" {
			inspection[box.ID] = corebattle.InspectionContent{Treasure: true, Empty: true}
			continue
		}
		def, err := catalog.FindByID(box.ItemID)
		if err != nil {
			return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, err
		}
		inspection[box.ID] = corebattle.InspectionContent{
			Treasure:     true,
			RewardKind:   def.Kind(),
			DefinitionID: def.ID,
			Price:        def.Price,
		}
	}
	return state, frame, inspection, nil
}

// ReconcileCombat writes non-box combat state back to r.Battle and reconciles
// projected box combat state, defeat, and encounter order back onto r.Boxes.
func (r *State) ReconcileCombat(state corebattle.LifetimeState, frame corebattle.TargetContext) error {
	if r == nil {
		return ErrConflict
	}
	boxIndex := make(map[string]int, len(r.Boxes))
	for i, box := range r.Boxes {
		boxIndex[box.ID] = i
	}
	next := CloneBattle(state)
	actors := make([]corebattle.LifetimeActor, 0, len(next.Actors))
	for _, a := range next.Actors {
		if _, isBox := boxIndex[a.ID]; isBox {
			continue
		}
		actors = append(actors, a)
	}
	next.Actors = actors
	r.Battle = next

	enemies := make([]string, 0, len(frame.EnemyIDs))
	for _, id := range frame.EnemyIDs {
		if _, isBox := boxIndex[id]; isBox {
			continue
		}
		if a := r.Actor(id); a != nil && a.HP > 0 {
			enemies = append(enemies, id)
		}
	}
	r.Enemies = enemies

	for i := range r.Boxes {
		a := FindActor(state.Actors, r.Boxes[i].ID)
		if a == nil {
			continue
		}
		if a.HP <= 0 {
			r.Boxes[i].Defeated = true
			r.Boxes[i].Combat = nil
			continue
		}
		r.Boxes[i].Defeated = false
		if !reflect.DeepEqual(*a, r.Boxes[i].defaultCombatActor()) {
			cp := a.Clone()
			r.Boxes[i].Combat = &cp
		} else {
			r.Boxes[i].Combat = nil
		}
	}

	reordered := make([]TreasureBox, 0, len(r.Boxes))
	seen := make(map[string]bool, len(r.Boxes))
	for _, id := range slices.Concat(frame.EnemyIDs, frame.MemberOnlyIDs) {
		idx, isBox := boxIndex[id]
		if !isBox || seen[id] || !r.Boxes[idx].IsUnopened() {
			continue
		}
		seen[id] = true
		reordered = append(reordered, r.Boxes[idx])
	}
	for _, box := range r.Boxes {
		if box.IsUnopened() && !seen[box.ID] {
			reordered = append(reordered, box)
		}
	}
	for i := range r.Boxes {
		if r.Boxes[i].IsUnopened() {
			r.Boxes[i], reordered = reordered[0], reordered[1:]
		}
	}
	return nil
}

// CloneBattle deep-copies the actors and history maps a battle step mutates.
func CloneBattle(s corebattle.LifetimeState) corebattle.LifetimeState {
	s.Actors = slices.Clone(s.Actors)
	for i := range s.Actors {
		s.Actors[i] = s.Actors[i].Clone()
	}
	s.SkillBans = maps.Clone(s.SkillBans)
	s.SkillHistory = maps.Clone(s.SkillHistory)
	for id, history := range s.SkillHistory {
		s.SkillHistory[id] = maps.Clone(history)
	}
	s.StoredToolHistory = maps.Clone(s.StoredToolHistory)
	for color, history := range s.StoredToolHistory {
		s.StoredToolHistory[color] = maps.Clone(history)
	}
	return s
}
