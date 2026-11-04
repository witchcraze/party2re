package adventure

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// PrepareJobEncounters rebuilds operation ports from a locked run. The consumer
// registers the returned context, admits the whole action before cost/draws,
// then commits its changed enemy order and the transition with the normal tail.
// Transaction: enclosing Command; no independent commit or settlement.
func (s *NormalRunService) PrepareJobEncounters(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext) (*corebattle.JobEncounterContext, *CombatTravelTransition, error) {
	if r == nil || r.actor(actor) == nil || !slices.Contains(r.Roster, actor) {
		return nil, nil, ErrNotParticipant
	}
	frame := corebattle.TargetContext{ActorID: actor, EnemyIDs: slices.Clone(r.Enemies)}
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.actor(actor).TeamID {
			frame.PartyIDs = append(frame.PartyIDs, a.ID)
		} else if !slices.Contains(frame.EnemyIDs, a.ID) {
			frame.MemberOnlyIDs = append(frame.MemberOnlyIDs, a.ID)
		}
	}
	if reset != nil {
		frame = reset.TargetContext
	}
	c := &corebattle.JobEncounterContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame}}}
	if reset != nil {
		c.Cursor = reset.Cursor
	}
	transition := &CombatTravelTransition{ResponseSuppressed: r.Floor == TreasureRoomFloor}
	_, modeErr := ValidateCrawlMode(r.Mode)
	room := &corebattle.JobRoomContext{ActorID: actor, NormalStage: modeErr == nil, Floor: r.Floor}
	room.Cursor = c.Cursor
	for _, box := range r.Boxes {
		if box.OpenedBy == "" {
			room.TreasureIDs = append(room.TreasureIDs, box.ID)
		}
	}
	room.Validate = func(id string) error {
		if id == "job-78-entry-2" {
			return s.ValidateCombatTravel(r, actor, "item-150", CombatTravelInput{Floor: r.Floor + 1}, reset)
		}
		if r.Stage.Ordinary == nil || r.Floor < 0 || r.Floor > TreasureRoomFloor {
			return corebattle.ErrUnsupportedEffect
		}
		if r.Floor < TreasureRoomFloor {
			return s.validateTravelEncounter(r.Stage)
		}
		return nil
	}
	room.Apply = func(id string) (corebattle.TargetContext, error) {
		if ctx.Value(normalMutationContext{}) != r.ID {
			return frame, ErrRunForbidden
		}
		if err := room.Validate(id); err != nil {
			return frame, err
		}
		if err := ctx.Err(); err != nil {
			return frame, err
		}
		blessing := ""
		if r.Floor == TreasureRoomFloor && s.cfg.Blessings != nil {
			var err error
			blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actor)
			if err != nil {
				return frame, err
			}
		}
		floor := r.Floor
		if id == "job-78-entry-2" {
			if err := battle.ResetAll(&r.Battle, reset, s.cfg.RNG); err != nil {
				return frame, err
			}
			floor++
		}
		t, err := s.replaceCombatEncounter(ctx, r, actor, floor, blessing)
		if err != nil {
			return frame, err
		}
		t.UnsealDemonKing = r.Stage.ID == "stage-19" && floor == TreasureRoomFloor
		*transition = t
		room.Floor = r.Floor
		frame.EnemyIDs, frame.MemberOnlyIDs = slices.Clone(r.Enemies), nil
		if reset != nil {
			room.Cursor = reset.Cursor
		}
		return frame, nil
	}
	c.Room = room
	return c, transition, nil
}
