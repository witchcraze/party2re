package adventure

import (
	"context"
	"encoding/json"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// CombatTravelInput is a destination, never a participant name or target ID.
type CombatTravelInput struct {
	Floor              int    `json:"floor,omitempty"`
	DestinationStageID string `json:"destination_stage_id,omitempty"`
}

func ParseCombatTravelInput(input string) (CombatTravelInput, error) {
	var value CombatTravelInput
	if !strings.HasPrefix(strings.TrimSpace(input), "{") {
		return value, ErrRunConflict
	}
	d := json.NewDecoder(strings.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return value, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return CombatTravelInput{}, ErrRunConflict
	}
	return value, nil
}

// CombatTravelTransition is operation-local. The command consumer binds and
// consumes its owned instance only when Consume is true, then runs the ordinary
// player tail and BeginNPCResponse. Persist only the final run, never this port.
type CombatTravelTransition struct {
	Consume            bool
	ResponseSuppressed bool
	// ResponseEnemies retains the old enemy side for the enclosing player tail;
	// generated members live in r.Battle. Collisions use the current run actor.
	ResponseEnemies []corebattle.LifetimeActor
	oldTimes        map[string]time.Time
	oldFloor        int
	oldStage        string
	replaced        bool
}

// BeginNPCResponse restores the old enemy list only when an old living member
// can respond. The consumer executes that one NPC and calls RecordNPCResponse.
// Generated payloads win identity collisions, including their fresh timestamps.
func (t CombatTravelTransition) BeginNPCResponse(r *NormalRun, now time.Time) string {
	if t.ResponseSuppressed {
		return ""
	}
	if !t.replaced {
		return r.NextNPC(now)
	}
	actors := make([]corebattle.LifetimeActor, len(t.ResponseEnemies))
	times := make(map[string]time.Time, len(actors))
	npc := ""
	for i, a := range t.ResponseEnemies {
		times[a.ID] = t.oldTimes[a.ID]
		if current := r.actor(a.ID); current != nil {
			a = current.Clone()
			times[a.ID] = r.LastActions[a.ID]
		}
		actors[i] = a
		if npc == "" && a.NPC && a.HP > 0 && now.Sub(times[a.ID]) >= time.Duration(r.Admission.Speed)*time.Second {
			npc = a.ID
		}
	}
	if npc == "" {
		return ""
	}
	r.discardEncounter()
	for _, a := range actors {
		r.Battle.Actors = append(r.Battle.Actors, a)
		r.Enemies = append(r.Enemies, a.ID)
		r.LastActions[a.ID] = times[a.ID]
	}
	r.EncounterFloor, r.EncounterStageID = t.oldFloor, t.oldStage
	return npc
}

// ValidateCombatTravel is pure admission for these finite tool callbacks.
// Incapacitation, whole-action tails and instance ownership stay with RunCombat.
func (s *NormalRunService) ValidateCombatTravel(r *NormalRun, actor, tool string, input CombatTravelInput, reset *corebattle.EquipmentResetContext) error {
	if r == nil || !slices.Contains(r.Roster, actor) || r.actor(actor) == nil {
		return ErrNotParticipant
	}
	if r.actor(actor).HP <= 0 {
		return ErrUnconsciousCharacter
	}
	if input.Floor != 0 && tool != "item-150" || input.DestinationStageID != "" && tool != "item-184" {
		return ErrRunConflict
	}
	if _, err := ValidateCrawlMode(r.Mode); err != nil && tool != "item-083" {
		return err
	}
	stage := travelStage(tool, input)
	switch tool {
	case "item-071":
		gear := r.actor(actor).Gear
		if gear == nil || gear.WeaponID != "weapon-35" || gear.ArmorID != "armor-37" {
			return ErrRunConflict
		}
	case "item-184":
		if stage != "stage-23" && stage != "stage-25" {
			return ErrRunConflict
		}
	case "item-150":
		if input.Floor < 1 || input.Floor > BossFloor {
			return ErrInvalidFloorAction
		}
		if reset == nil || reset.ActorID != actor || r.Stage.Ordinary == nil {
			return corebattle.ErrUnsupportedEffect
		}
		if !slices.Equal(reset.EnemyIDs, r.Enemies) {
			return corebattle.ErrUnsupportedEffect
		}
		if r.Stage.ID == "stage-24" && input.Floor == BossFloor && s.cfg.Combat == nil {
			return corebattle.ErrUnsupportedEffect
		}
		for _, a := range r.Battle.Actors {
			// shortcut: detached-member side projection needs #1329's enclosing composition.
			if a.RosterDetached {
				return corebattle.ErrUnsupportedEffect
			}
		}
		for _, id := range r.Enemies {
			if a := r.actor(id); a == nil || !a.NPC || a.EncounterIdentity == "" {
				return corebattle.ErrUnsupportedEffect
			}
		}
		if err := battle.ValidateResetAll(&r.Battle, reset, s.cfg.RNG); err != nil {
			return err
		}
		if err := s.validateTravelEncounter(r.Stage); err != nil {
			return err
		}
	case "item-066", "item-067", "item-068", "item-069", "item-070", "item-104", "item-083", "item-107":
	default:
		return corebattle.ErrUnsupportedEffect
	}
	if stage != "" {
		_, err := s.cfg.Stages.FindByID(stage)
		return err
	}
	return nil
}

func travelStage(tool string, input CombatTravelInput) string {
	switch tool {
	case "item-066":
		return "stage-15"
	case "item-067":
		return "stage-16"
	case "item-068":
		return "stage-17"
	case "item-069":
		return "stage-18"
	case "item-070":
		return "stage-19"
	case "item-071":
		return "stage-20"
	case "item-104":
		return "stage-21"
	case "item-184":
		return input.DestinationStageID
	}
	return ""
}

// ApplyCombatTravel mutates the already locked run inside Command. It neither
// settles assets nor runs action tails; external failure rolls back that SQL
// transaction. Selection/support errors are rejected before mutation or draws.
func (s *NormalRunService) ApplyCombatTravel(ctx context.Context, r *NormalRun, actor, tool string, input CombatTravelInput, reset *corebattle.EquipmentResetContext) (CombatTravelTransition, error) {
	if err := s.ValidateCombatTravel(r, actor, tool, input, reset); err != nil {
		return CombatTravelTransition{}, err
	}
	if err := ctx.Err(); err != nil {
		return CombatTravelTransition{}, err
	}
	t := CombatTravelTransition{Consume: true, ResponseSuppressed: r.Floor == TreasureRoomFloor}
	if stage := travelStage(tool, input); stage != "" {
		destination, err := s.cfg.Stages.FindByID(stage)
		if err != nil {
			return CombatTravelTransition{}, err
		}
		r.Stage = destination
		r.Floor = 1
		return t, nil
	}
	switch tool {
	case "item-083":
		if _, err := ValidateCrawlMode(r.Mode); err == nil && r.Floor < 9 {
			r.Floor++
		}
	case "item-107":
		r.Floor = BossFloor
	case "item-150":
		frame := *reset
		if err := battle.ResetAll(&r.Battle, &frame, s.cfg.RNG); err != nil {
			return CombatTravelTransition{}, err
		}
		reset.Cursor = frame.Cursor
		t.replaced, t.oldFloor, t.oldStage = true, r.EncounterFloor, r.EncounterStageID
		t.oldTimes = make(map[string]time.Time, len(r.Enemies))
		for _, id := range r.Enemies {
			t.ResponseEnemies = append(t.ResponseEnemies, r.actor(id).Clone())
			t.oldTimes[id] = r.LastActions[id]
		}
		r.discardEncounter()
		r.Floor = input.Floor
		if err := s.createEncounter(ctx, r, actor); err != nil {
			return CombatTravelTransition{}, err
		}
		for i, id := range r.Enemies {
			a := r.actor(id)
			for _, old := range t.ResponseEnemies {
				if a.EncounterIdentity != "" && a.EncounterIdentity == old.EncounterIdentity {
					a.ID, r.Enemies[i] = old.ID, old.ID
					break
				}
			}
		}
	}
	return t, nil
}

func (s *NormalRunService) validateTravelEncounter(stage Stage) error {
	table := stage.Ordinary
	if table.PartyCopy {
		return nil
	}
	if len(table.BossRosters) == 0 || len(table.MonsterIDs) == 0 {
		return corebattle.ErrUnsupportedEffect
	}
	for _, branch := range append([][]string{table.MonsterIDs}, table.BossRosters...) {
		if len(branch) == 0 {
			return corebattle.ErrUnsupportedEffect
		}
		for _, id := range branch {
			if _, err := s.cfg.Monsters.FindByID(id); err != nil {
				return err
			}
		}
	}
	for _, position := range table.Appears {
		if position < 0 || position >= len(table.MonsterIDs) {
			return corebattle.ErrUnsupportedEffect
		}
	}
	return nil
}

func (r *NormalRun) discardEncounter() {
	for _, id := range r.Enemies {
		delete(r.LastActions, id)
	}
	r.Battle.Actors = slices.DeleteFunc(r.Battle.Actors, func(a corebattle.LifetimeActor) bool { return slices.Contains(r.Enemies, a.ID) })
	r.Enemies = nil
	r.Boxes = nil
}
