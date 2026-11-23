package adventure

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// PrepareJobEncounters rebuilds operation ports from a locked run. The consumer
// registers the returned context, admits the whole action before cost/draws,
// then commits its changed enemy order and the transition with the normal tail.
// Transaction: enclosing Command; no independent commit or settlement.
func (s *NormalRunService) PrepareJobEncounters(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext) (*corebattle.JobEncounterContext, *CombatTravelTransition, error) {
	if r == nil || r.Actor(actor) == nil || !slices.Contains(r.Roster, actor) {
		return nil, nil, ErrNotParticipant
	}
	frame := corebattle.TargetContext{ActorID: actor, EnemyIDs: slices.Clone(r.Enemies)}
	for _, a := range r.Battle.Actors {
		if a.TeamID == r.Actor(actor).TeamID {
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
	transition := &CombatTravelTransition{ResponseSuppressed: isTreasureFloor(r.Round)}
	_, modeErr := ValidateCrawlMode(r.Mode)
	room := &corebattle.JobRoomContext{ActorID: actor, NormalStage: modeErr == nil, Floor: r.Round}
	room.Cursor = c.Cursor
	for _, box := range r.Boxes {
		if box.IsUnopened() {
			room.TreasureIDs = append(room.TreasureIDs, box.ID)
		}
	}
	room.Validate = func(id string) error {
		if id == "job-78-entry-2" {
			return s.ValidateCombatTravel(r, actor, "item-150", CombatTravelInput{Floor: r.Round + 1}, reset)
		}
		if r.Round == EXTreasureFloor && len(r.Stage.exTreasureItems()) > 0 {
			return nil
		}
		if r.Stage.Ordinary == nil || r.Round < 0 || r.Round > TreasureRoomFloor {
			return corebattle.ErrUnsupportedEffect
		}
		if r.Round < TreasureRoomFloor {
			return s.validateTravelEncounter(r.Stage)
		}
		return nil
	}
	room.Apply = func(id string) (corebattle.TargetContext, error) {
		if !runhost.InCommand(ctx, r.ID) {
			return frame, runhost.ErrForbidden
		}
		if err := room.Validate(id); err != nil {
			return frame, err
		}
		if err := ctx.Err(); err != nil {
			return frame, err
		}
		blessing := ""
		if isTreasureFloor(r.Round) && s.cfg.Blessings != nil {
			var err error
			blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actor)
			if err != nil {
				return frame, err
			}
		}
		floor := r.Round
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
		t.UnsealDemonKing = r.Stage.UnsealsOnTreasure && floor == TreasureRoomFloor
		*transition = t
		room.Floor = r.Round
		frame.EnemyIDs, frame.MemberOnlyIDs = slices.Clone(r.Enemies), nil
		if reset != nil {
			room.Cursor = reset.Cursor
		}
		return frame, nil
	}
	c.Room = room
	return c, transition, nil
}
