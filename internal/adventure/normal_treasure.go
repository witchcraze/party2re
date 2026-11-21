package adventure

import (
	"maps"
	"reflect"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

func (b TreasureBox) isUnopened() bool {
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

func (r *NormalRun) hasBox(id string) bool {
	return slices.ContainsFunc(r.Boxes, func(b TreasureBox) bool { return b.ID == id })
}

// ProjectCombat projects durable battle state and unopened TreasureBoxes into
// an operation-local LifetimeState, TargetContext, and inspection metadata map
// without duplicating boxes into r.Battle.Actors.
func (r *NormalRun) ProjectCombat(actorID string) (corebattle.LifetimeState, corebattle.TargetContext, map[string]corebattle.InspectionContent, error) {
	if r == nil || r.actor(actorID) == nil {
		return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrNotParticipant
	}
	state := cloneLifetimeState(r.Battle)
	frame := corebattle.TargetContext{ActorID: actorID}
	for _, id := range r.Enemies {
		if enemy := findActor(state.Actors, id); enemy != nil && enemy.HP > 0 {
			frame.EnemyIDs = append(frame.EnemyIDs, id)
		}
	}
	var catalog *item.Catalog
	for _, box := range r.Boxes {
		if !box.isUnopened() {
			continue
		}
		if box.ID == "" || findActor(state.Actors, box.ID) != nil {
			return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrRunConflict
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
	team := r.actor(actorID).TeamID
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
		if !box.isUnopened() {
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
func (r *NormalRun) ReconcileCombat(state corebattle.LifetimeState, frame corebattle.TargetContext) error {
	if r == nil {
		return ErrRunConflict
	}
	boxIndex := make(map[string]int, len(r.Boxes))
	for i, box := range r.Boxes {
		boxIndex[box.ID] = i
	}
	next := cloneLifetimeState(state)
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
		if a := r.actor(id); a != nil && a.HP > 0 {
			enemies = append(enemies, id)
		}
	}
	r.Enemies = enemies

	for i := range r.Boxes {
		a := findActor(state.Actors, r.Boxes[i].ID)
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
		if !isBox || seen[id] || !r.Boxes[idx].isUnopened() {
			continue
		}
		seen[id] = true
		reordered = append(reordered, r.Boxes[idx])
	}
	for _, box := range r.Boxes {
		if box.isUnopened() && !seen[box.ID] {
			reordered = append(reordered, box)
		}
	}
	for i := range r.Boxes {
		if r.Boxes[i].isUnopened() {
			r.Boxes[i], reordered = reordered[0], reordered[1:]
		}
	}
	return nil
}

func cloneLifetimeState(s corebattle.LifetimeState) corebattle.LifetimeState {
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

func findActor(actors []corebattle.LifetimeActor, id string) *corebattle.LifetimeActor {
	for i := range actors {
		if actors[i].ID == id {
			return &actors[i]
		}
	}
	return nil
}
