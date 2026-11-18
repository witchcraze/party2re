package adventure

import (
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

func (b TreasureBox) isUnopened() bool {
	return b.ID != "" && b.OpenedBy == "" && !b.Defeated && (b.Combat == nil || b.Combat.HP > 0)
}

func (b TreasureBox) defaultActor() corebattle.LifetimeActor {
	name := b.Name
	if name == "" {
		name = b.ID
	}
	return corebattle.LifetimeActor{
		Participant: corebattle.Participant{
			ID:        b.ID,
			Name:      name,
			TeamID:    "enemies",
			HP:        1000,
			MaxHP:     1000,
			Defense:   1000,
			BaseStats: corebattle.BattleBaseStats{Defense: 1000},
			Gear:      &corebattle.BattleGearStats{},
		},
		Jobs:         corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"},
		OriginalJobs: corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"},
		NPC:          true,
		Tension:      1,
		Temporary:    corebattle.TemporaryNullMagic,
	}
}

func (b TreasureBox) lifetimeActor() corebattle.LifetimeActor {
	if b.Combat == nil {
		return b.defaultActor()
	}
	cp := b.Combat.Clone()
	name := b.Name
	if name == "" {
		name = b.ID
	}
	cp.ID = b.ID
	cp.Name = name
	cp.TeamID = "enemies"
	cp.NPC = true
	if cp.Gear == nil {
		cp.Gear = &corebattle.BattleGearStats{}
	}
	if cp.Jobs.JobID == "" {
		cp.Jobs.JobID = "starter"
	}
	if cp.Jobs.OldJobID == "" {
		cp.Jobs.OldJobID = "starter"
	}
	return cp
}

func (b TreasureBox) inspectionContent() (corebattle.InspectionContent, error) {
	itemID := strings.TrimSpace(b.ItemID)
	if itemID == "" || itemID == "0" || itemID == "item-000" {
		return corebattle.InspectionContent{Treasure: true, Empty: true}, nil
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return corebattle.InspectionContent{}, err
	}
	def, err := catalog.FindByID(itemID)
	if err != nil {
		return corebattle.InspectionContent{}, err
	}
	return corebattle.InspectionContent{
		Treasure:     true,
		RewardKind:   def.LegacyHandKind(),
		DefinitionID: def.ID,
		Price:        def.Price,
	}, nil
}

func actorInspectionContent(a corebattle.LifetimeActor) (corebattle.InspectionContent, error) {
	if a.GoldReward <= 0 {
		return corebattle.InspectionContent{Empty: true}, nil
	}
	id := fmt.Sprintf("item-%03d", a.GoldReward)
	switch a.ExperienceReward {
	case 1:
		id = fmt.Sprintf("weapon-%02d", a.GoldReward)
	case 2:
		id = fmt.Sprintf("armor-%02d", a.GoldReward)
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return corebattle.InspectionContent{}, err
	}
	def, err := catalog.FindByID(id)
	if errors.Is(err, item.ErrDefinitionNotFound) {
		return corebattle.InspectionContent{Empty: true}, nil
	}
	if err != nil {
		return corebattle.InspectionContent{}, err
	}
	return corebattle.InspectionContent{
		RewardKind:   def.LegacyHandKind(),
		DefinitionID: def.ID,
		Price:        def.Price,
	}, nil
}

func cloneLifetimeState(s corebattle.LifetimeState) corebattle.LifetimeState {
	s.Actors = slices.Clone(s.Actors)
	for i := range s.Actors {
		s.Actors[i] = s.Actors[i].Clone()
	}
	s.SkillBans = maps.Clone(s.SkillBans)
	s.SkillHistory = maps.Clone(s.SkillHistory)
	for k, v := range s.SkillHistory {
		s.SkillHistory[k] = maps.Clone(v)
	}
	s.StoredToolHistory = maps.Clone(s.StoredToolHistory)
	for k, v := range s.StoredToolHistory {
		s.StoredToolHistory[k] = maps.Clone(v)
	}
	return s
}

// ProjectCombat constructs an operation-local battle state, target frame, and
// frozen inspection metadata with unopened boxes projected as opposing members.
func (r *NormalRun) ProjectCombat(actorID string) (corebattle.LifetimeState, corebattle.TargetContext, map[string]corebattle.InspectionContent, error) {
	if r == nil {
		return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrRunConflict
	}
	actor := r.actor(actorID)
	if actor == nil {
		return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, ErrNotParticipant
	}
	state := cloneLifetimeState(r.Battle)
	inspection := make(map[string]corebattle.InspectionContent, len(state.Actors)+len(r.Boxes))
	for _, a := range state.Actors {
		content, err := actorInspectionContent(a)
		if err != nil {
			return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, err
		}
		inspection[a.ID] = content
	}
	frame := corebattle.TargetContext{ActorID: actorID}
	for _, id := range r.Enemies {
		if enemy := r.actor(id); enemy != nil && enemy.HP > 0 {
			frame.EnemyIDs = append(frame.EnemyIDs, id)
		}
	}
	for _, box := range r.Boxes {
		if !box.isUnopened() {
			continue
		}
		if !slices.ContainsFunc(state.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == box.ID }) {
			state.Actors = append(state.Actors, box.lifetimeActor())
		}
		if !slices.Contains(frame.EnemyIDs, box.ID) {
			frame.EnemyIDs = append(frame.EnemyIDs, box.ID)
		}
		content, err := box.inspectionContent()
		if err != nil {
			return corebattle.LifetimeState{}, corebattle.TargetContext{}, nil, err
		}
		inspection[box.ID] = content
	}
	for _, member := range state.Actors {
		switch {
		case member.TeamID == actor.TeamID:
			frame.PartyIDs = append(frame.PartyIDs, member.ID)
		case !slices.Contains(frame.EnemyIDs, member.ID):
			frame.MemberOnlyIDs = append(frame.MemberOnlyIDs, member.ID)
		}
	}
	return state, frame, inspection, nil
}

// ReconcileCombat folds operation-local combat effects on projected boxes back
// into NormalRun.Boxes and removes projected box entries from durable battle actors.
func (r *NormalRun) ReconcileCombat(state corebattle.LifetimeState, frame *corebattle.TargetContext) {
	if r == nil {
		return
	}
	boxIdx := make(map[string]int, len(r.Boxes))
	for i, b := range r.Boxes {
		boxIdx[b.ID] = i
	}
	for i := range r.Boxes {
		if !r.Boxes[i].isUnopened() {
			continue
		}
		idx := slices.IndexFunc(state.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == r.Boxes[i].ID })
		if idx < 0 {
			continue
		}
		a := state.Actors[idx]
		if a.HP <= 0 {
			r.Boxes[i].Defeated = true
			r.Boxes[i].Combat = nil
		} else if reflect.DeepEqual(a, r.Boxes[i].defaultActor()) {
			r.Boxes[i].Combat = nil
		} else {
			cp := a.Clone()
			r.Boxes[i].Combat = &cp
		}
	}
	if frame != nil {
		var ordered []int
		for _, id := range slices.Concat(frame.EnemyIDs, frame.MemberOnlyIDs) {
			if idx, ok := boxIdx[id]; ok && !slices.Contains(ordered, idx) {
				ordered = append(ordered, idx)
			}
		}
		if len(ordered) > 0 {
			reordered := slices.Clone(r.Boxes)
			pos := 0
			for i := range r.Boxes {
				if slices.Contains(ordered, i) {
					reordered[i] = r.Boxes[ordered[pos]]
					pos++
				}
			}
			r.Boxes = reordered
		}
		isBoxID := func(id string) bool {
			_, ok := boxIdx[id]
			return ok && !slices.Contains(r.Enemies, id)
		}
		frame.EnemyIDs = slices.DeleteFunc(frame.EnemyIDs, isBoxID)
		frame.MemberOnlyIDs = slices.DeleteFunc(frame.MemberOnlyIDs, isBoxID)
	}
	state.Actors = slices.DeleteFunc(state.Actors, func(a corebattle.LifetimeActor) bool {
		_, ok := boxIdx[a.ID]
		return ok && !slices.Contains(r.Enemies, a.ID)
	})
	for _, id := range r.Enemies {
		if !slices.ContainsFunc(state.Actors, func(a corebattle.LifetimeActor) bool { return a.ID == id }) {
			if a := r.actor(id); a != nil {
				state.Actors = append(state.Actors, a.Clone())
			}
		}
	}
	r.Battle = state
}
