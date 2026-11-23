package adventure

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// prepareJobEncounters binds job78's room bodies (bypass jump, treasure retry)
// to a locked run. The host admits the whole action before cost/draws, then
// commits the changed enemy order and the transition with the normal tail.
// Transaction: enclosing Command; no independent commit or settlement.
func (s *NormalRunService) prepareJobEncounters(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext) (runhost.JobRoom, *combatTravelTransition, error) {
	if r == nil || r.Actor(actor) == nil || !slices.Contains(r.Roster, actor) {
		return runhost.JobRoom{}, nil, ErrNotParticipant
	}
	transition := &combatTravelTransition{ResponseSuppressed: isTreasureFloor(r.Round)}
	room := runhost.JobRoom{}
	room.Validate = func(id string) error {
		if id == "job-78-entry-2" {
			return s.validateCombatTravel(r, actor, "item-150", combatTravelInput{Floor: r.Round + 1}, reset)
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
	room.Apply = func(id string) error {
		if err := room.Validate(id); err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		blessing := ""
		if isTreasureFloor(r.Round) && s.cfg.Blessings != nil {
			var err error
			blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actor)
			if err != nil {
				return err
			}
		}
		floor := r.Round
		if id == "job-78-entry-2" {
			if err := battle.ResetAll(&r.Battle, reset, s.cfg.RNG); err != nil {
				return err
			}
			floor++
		}
		t, err := s.replaceCombatEncounter(ctx, r, actor, floor, blessing)
		if err != nil {
			return err
		}
		t.UnsealDemonKing = r.Stage.UnsealsOnTreasure && floor == TreasureRoomFloor
		*transition = t
		return nil
	}
	return room, transition, nil
}
