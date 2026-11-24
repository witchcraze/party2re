package challenge

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/runhost"
)

// stageRoom is the Challenge room library (vs_challenge.cgi) the run host
// consults inside its command transaction.
type stageRoom struct{ s *RunService }

// Prepare fixes the stage; Challenge has no season or mode selection.
func (room stageRoom) Prepare(a *runhost.Admission) error {
	a.StageID, a.Season, a.Mode = strings.TrimSpace(a.StageID), "", ""
	tier, ok := room.s.cfg.Tiers[a.StageID]
	if !ok {
		return ErrTierNotFound
	}
	if len(a.CharacterIDs) > tier.MaxParticipants {
		return ErrTooManyPartyMembers
	}
	return nil
}

// AdmitMember applies the creator's stage gates (quest.cgi:539–540,712)
// and every member's join condition and stocked level-up (quest.cgi:928–972).
func (room stageRoom) AdmitMember(ctx context.Context, r *Run, id string) error {
	c, err := room.s.cfg.Characters.FindByID(ctx, id)
	if err != nil {
		return err
	}
	tier := room.s.cfg.Tiers[r.Admission.StageID]
	if id == r.Admission.ActorID {
		stage, err := strconv.Atoi(tier.ID)
		if err != nil {
			return ErrTierNotFound
		}
		if stage*2 > c.JobLevel+1 {
			return ErrJobLevelTooLow
		}
		if tier.NeedOverLv && c.Level <= progression.MaxLevel {
			return ErrNeedOverLvNotMet
		}
		if !tier.NeedOverLv && c.Level > progression.MaxLevel {
			return ErrLevelTooHigh
		}
	}
	if next, err := progression.ExperienceForNextLevel(c.Level); err == nil && c.Experience >= next {
		return ErrPendingLevelUp
	}
	if err := corecharacter.ValidateNeedJoin(tier.NeedJoin, c); err != nil {
		return fmt.Errorf("%w: %v", ErrNeedJoinNotMet, err)
	}
	return nil
}

// Open reads the world record the room must beat (get_max_round, quest.cgi:732).
func (room stageRoom) Open(ctx context.Context, r *Run) (func(context.Context) error, error) {
	hof, err := room.s.cfg.HallOfFame.GetHallOfFame(ctx, r.Admission.StageID)
	if err != nil {
		return nil, err
	}
	if hof != nil {
		r.Record = hof.HighestRound
	}
	return nil, nil
}

func (stageRoom) Actions() []string { return []string{"advance"} }

func (room stageRoom) Act(ctx context.Context, r *Run, cmd runhost.Command) (runhost.Effects, error) {
	return room.s.advance(ctx, r, cmd.ActorID)
}

// Exit is nigeru's Challenge part; Challenge has no calm mode.
func (room stageRoom) Exit(r *Run, actor string) (runhost.Earnings, int, error) {
	return r.DefaultExit(actor, room.s.cfg.RNG)
}

// Record is a no-op: nigeru only prints the result screen for Challenge.
func (stageRoom) Record(context.Context, *Run, string, string) error { return nil }

func (stageRoom) Conditional(*Run) bool { return false }

// Choices are add_battle_action's しらべる while a box remains and すすむ once
// no enemy or box remains (vs_challenge.cgi:21–30).
func (stageRoom) Choices(r *Run, _ string) []runhost.Choice {
	var choices []runhost.Choice
	if hasBoxes(r) {
		choices = append(choices, runhost.Choice{Action: "chest", Source: "しらべる"})
	}
	if !r.LivingEnemies() {
		choices = append(choices, runhost.Choice{Action: "advance", Source: "すすむ"})
	}
	return choices
}

// Responds is false while a box leads the enemies (vs_challenge.cgi:22–23).
func (stageRoom) Responds(r *Run) bool { return !hasBoxes(r) }

func hasBoxes(r *Run) bool { return slices.ContainsFunc(r.Boxes, runhost.TreasureBox.IsUnopened) }

// Combat keeps the host's non-vs_monster branches; $win is the stage record.
func (stageRoom) Combat(_ context.Context, r *Run, _ string, _ *corebattle.EquipmentResetContext, _ bool) (runhost.RoomCombat, error) {
	return runhost.RoomCombat{Win: r.Record > 0}, nil
}
