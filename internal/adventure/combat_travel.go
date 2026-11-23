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
	"github.com/witchcraze/party2re/internal/runhost"
)

// combatTravelInput is a destination, never a participant name or target ID.
type combatTravelInput struct {
	Floor              int    `json:"floor,omitempty"`
	DestinationStageID string `json:"destination_stage_id,omitempty"`
}

func parseCombatTravelInput(input string) (combatTravelInput, error) {
	var value combatTravelInput
	if !strings.HasPrefix(strings.TrimSpace(input), "{") {
		return value, runhost.ErrConflict
	}
	d := json.NewDecoder(strings.NewReader(input))
	d.DisallowUnknownFields()
	if err := d.Decode(&value); err != nil {
		return value, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return combatTravelInput{}, runhost.ErrConflict
	}
	return value, nil
}

// combatTravelTransition is operation-local. The command consumer binds and
// consumes its owned instance only when Consume is true, then runs the ordinary
// player tail and beginNPCResponse. Persist only the final run, never this port.
type combatTravelTransition struct {
	Consume            bool
	ResponseSuppressed bool
	Reload             bool
	// A genuine job-driven stage19 treasure creation, consumed once by Command.
	UnsealDemonKing bool
	// ResponseEnemies retains the old enemy side for the enclosing player tail;
	// generated members live in r.Battle. Collisions use the current run actor.
	ResponseEnemies []corebattle.LifetimeActor
	oldTimes        map[string]time.Time
	oldFloor        int
	oldStage        string
	replaced        bool
}

// beginNPCResponse restores the old enemy list only when an old living member
// can respond. The consumer executes that one NPC and calls RecordNPCResponse.
// Generated payloads win identity collisions, including their fresh timestamps.
func (t combatTravelTransition) beginNPCResponse(r *NormalRun, now time.Time) string {
	if t.ResponseSuppressed {
		return ""
	}
	if !t.replaced {
		if isTreasureFloor(r.Round) {
			return ""
		}
		return r.NextNPC(now)
	}
	actors := make([]corebattle.LifetimeActor, len(t.ResponseEnemies))
	times := make(map[string]time.Time, len(actors))
	npc := ""
	for i, a := range t.ResponseEnemies {
		times[a.ID] = t.oldTimes[a.ID]
		if current := r.Actor(a.ID); current != nil {
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
	r.DiscardEncounter()
	for _, a := range actors {
		r.Battle.Actors = append(r.Battle.Actors, a)
		r.Enemies = append(r.Enemies, a.ID)
		r.LastActions[a.ID] = times[a.ID]
	}
	r.EncounterFloor, r.EncounterStageID = t.oldFloor, t.oldStage
	return npc
}

// validateCombatTravel is pure admission for these finite tool callbacks.
// Incapacitation, whole-action tails and instance ownership stay with the host executor.
func (s *NormalRunService) validateCombatTravel(r *NormalRun, actor, tool string, input combatTravelInput, reset *corebattle.EquipmentResetContext) error {
	if r == nil || !slices.Contains(r.Roster, actor) || r.Actor(actor) == nil {
		return ErrNotParticipant
	}
	if r.Actor(actor).HP <= 0 {
		return ErrUnconsciousCharacter
	}
	if input.Floor != 0 && tool != "item-150" || input.DestinationStageID != "" && tool != "item-184" {
		return runhost.ErrConflict
	}
	if _, err := ValidateCrawlMode(r.Mode); err != nil && tool != "item-083" {
		return err
	}
	stage := travelStage(tool, input)
	switch tool {
	case "item-071":
		gear := r.Actor(actor).Gear
		if gear == nil || gear.WeaponID != "weapon-35" || gear.ArmorID != "armor-37" {
			return runhost.ErrConflict
		}
	case "item-184":
		if stage != "stage-23" && stage != "stage-25" {
			return runhost.ErrConflict
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
		for _, a := range r.Battle.Actors {
			// shortcut: a member dismissed earlier in the operation stays a gate; upgrade with an original-side projection.
			if a.RosterDetached {
				return corebattle.ErrUnsupportedEffect
			}
		}
		for _, id := range r.Enemies {
			if a := r.Actor(id); a == nil || !a.NPC || a.EncounterIdentity == "" {
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

func travelStage(tool string, input combatTravelInput) string {
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

// applyCombatTravel mutates the already locked run inside Command. It neither
// settles assets nor runs action tails; external failure rolls back that SQL
// transaction. Selection/support errors are rejected before mutation or draws.
func (s *NormalRunService) applyCombatTravel(ctx context.Context, r *NormalRun, actor, tool string, input combatTravelInput, reset *corebattle.EquipmentResetContext) (combatTravelTransition, error) {
	if err := s.validateCombatTravel(r, actor, tool, input, reset); err != nil {
		return combatTravelTransition{}, err
	}
	if err := ctx.Err(); err != nil {
		return combatTravelTransition{}, err
	}
	t := combatTravelTransition{Consume: true, ResponseSuppressed: isTreasureFloor(r.Round)}
	if stage := travelStage(tool, input); stage != "" {
		destination, err := s.cfg.Stages.FindByID(stage)
		if err != nil {
			return combatTravelTransition{}, err
		}
		r.Stage = destination
		r.Round = 1
		return t, nil
	}
	switch tool {
	case "item-083":
		if _, err := ValidateCrawlMode(r.Mode); err == nil && r.Round < 9 {
			r.Round++
		}
	case "item-107":
		r.Round = BossFloor
	case "item-150":
		frame := *reset
		if err := battle.ResetAll(&r.Battle, &frame, s.cfg.RNG); err != nil {
			return combatTravelTransition{}, err
		}
		reset.Cursor = frame.Cursor
		replacement, err := s.replaceCombatEncounter(ctx, r, actor, input.Floor, "")
		if err != nil {
			return combatTravelTransition{}, err
		}
		replacement.Consume = true
		return replacement, nil
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

func (s *NormalRunService) replaceCombatEncounter(ctx context.Context, r *NormalRun, actor string, floor int, blessing string) (combatTravelTransition, error) {
	t := combatTravelTransition{ResponseSuppressed: isTreasureFloor(r.Round), Reload: true}
	t.replaced, t.oldFloor, t.oldStage = true, r.EncounterFloor, r.EncounterStageID
	t.oldTimes = make(map[string]time.Time, len(r.Enemies))
	for _, id := range r.Enemies {
		t.ResponseEnemies = append(t.ResponseEnemies, r.Actor(id).Clone())
		t.oldTimes[id] = r.LastActions[id]
	}
	r.DiscardEncounter()
	r.Round = floor
	var err error
	switch floor {
	case TreasureRoomFloor:
		err = s.createTreasureEncounter(r, actor, blessing)
	case EXTreasureFloor:
		err = s.createEXTreasure(r, actor, blessing)
	default:
		err = s.createEncounter(ctx, r, actor)
	}
	if err != nil {
		return combatTravelTransition{}, err
	}
	for i, id := range r.Enemies {
		a := r.Actor(id)
		for _, old := range t.ResponseEnemies {
			if a.EncounterIdentity != "" && a.EncounterIdentity == old.EncounterIdentity {
				a.ID, r.Enemies[i] = old.ID, old.ID
				break
			}
		}
	}
	return t, nil
}

// prepareStoredToolTravel adapts these tools for job50's stored invocation. The
// consumer registers the port on its JobAssetContext and finishes the returned
// transition's tail; the stored instance is consumed only through
// the host's asset-session outcome, so the transition never consumes a held tool.
// Transaction: enclosing Command; no independent commit.
func (s *NormalRunService) prepareStoredToolTravel(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext) (*corebattle.StoredToolTravel, *combatTravelTransition) {
	transition := &combatTravelTransition{}
	input := travelInput
	return &corebattle.StoredToolTravel{
		Validate: func(tool, target string) error {
			value, err := input(tool, target)
			if err != nil {
				return err
			}
			return s.validateCombatTravel(r, actor, tool, value, reset)
		},
		Apply: func(tool, target string, frame corebattle.TargetContext) (corebattle.TargetContext, error) {
			value, err := input(tool, target)
			if err != nil {
				return frame, err
			}
			t, err := s.applyCombatTravel(ctx, r, actor, tool, value, reset)
			if err != nil {
				return frame, err
			}
			t.Consume = false
			*transition = t
			frame.EnemyIDs, frame.MemberOnlyIDs = slices.Clone(r.Enemies), nil
			return frame, nil
		},
	}, transition
}

// travelTools are the finite travel callbacks, held or stored.
var travelTools = []string{"item-066", "item-067", "item-068", "item-069", "item-070", "item-071", "item-083", "item-104", "item-107", "item-150", "item-184"}

func travelInput(tool, target string) (combatTravelInput, error) {
	if !slices.Contains(travelTools, tool) {
		return combatTravelInput{}, corebattle.ErrUnsupportedEffect
	}
	if !strings.HasPrefix(target, "{") {
		return combatTravelInput{}, nil // Coordinate tools ignore the remaining text.
	}
	return parseCombatTravelInput(target)
}
