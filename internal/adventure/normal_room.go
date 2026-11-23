package adventure

import (
	"context"
	"slices"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// stageRoom is the Adventure stage room library (vs_monster.cgi) the run host
// consults inside its command transaction.
type stageRoom struct{ s *NormalRunService }

func (stageRoom) Prepare(a *runhost.Admission) error {
	mode, err := ValidateCrawlMode(a.Mode)
	if err != nil {
		return err
	}
	a.Mode, a.StageID = mode, strings.TrimSpace(a.StageID)
	return nil
}

func (room stageRoom) AdmitMember(ctx context.Context, r *NormalRun, id string) error {
	c, err := room.s.cfg.Characters.FindByID(ctx, id)
	if err != nil {
		return err
	}
	return room.s.cfg.Stages.CanAccessStageAt(c, r.Admission.StageID, r.StartedAt)
}

// Open fixes the stage, season and room-creation EX draw. A solo daily-once
// stage claims the entry lock with the run identity as its token.
func (room stageRoom) Open(ctx context.Context, r *NormalRun) (func(context.Context) error, error) {
	s, a := room.s, r.Admission
	stage, err := s.cfg.Stages.FindByID(a.StageID)
	if err != nil {
		return nil, err
	}
	var release func(context.Context) error
	if stage.DailyOnce && a.PartyID == "" {
		if s.cfg.EntryLocks == nil {
			return nil, runhost.ErrUnavailable
		}
		// A failed earlier attempt may own this token but no committed receipt.
		// The admission lock and receipt check make only that token releasable.
		if _, err := s.cfg.EntryLocks.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, a.ActorID, r.ID); err != nil {
			return nil, err
		}
		ok, err := s.cfg.EntryLocks.TryLockWithToken(ctx, timer.CategoryDungeonOnce, a.ActorID, r.ID, 0)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, ErrDailyOnceAlreadyUsed
		}
		release = func(ctx context.Context) error {
			_, err := s.cfg.EntryLocks.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, a.ActorID, r.ID)
			return err
		}
	}
	season := a.Season
	if a.PartyID == "" && season == "" {
		// Solo room creation selects its season here; Party selected it at lobby creation.
		season = stage.SelectSeason(s.cfg.RNG.Intn).Season
	}
	r.Mode = a.Mode
	r.Stage = stage.OrdinarySeason(season)
	r.RecordSeason = r.Stage.Season
	// quest.cgi:596–600: one EX draw after season selection, never repeated by travel.
	r.EXUnlocked = r.Stage.exMarker() == markerRoomCreation && s.cfg.RNG.Intn(3) == 0
	return release, nil
}

func (stageRoom) Actions() []string { return []string{"advance"} }

func (room stageRoom) Act(ctx context.Context, r *NormalRun, cmd runhost.Command) (runhost.Effects, error) {
	if err := room.s.advance(ctx, r, cmd.ActorID); err != nil {
		return runhost.Effects{}, err
	}
	return runhost.Effects{UnsealDemonKing: r.Stage.UnsealsOnTreasure && r.Round == TreasureRoomFloor}, nil
}

func (room stageRoom) Exit(r *NormalRun, actor string) (runhost.Earnings, int, error) {
	bonus := runhost.Earnings{}
	if a := r.Actor(actor); a != nil && a.Gear != nil && slices.Contains(a.ItemDefinitionIDs, a.Gear.AccessoryID) {
		switch a.Gear.AccessoryID {
		case "item-105":
			var err error
			bonus.EXP, err = economy.SafeMultiply(r.Round, 20)
			if err != nil {
				return bonus, 0, err
			}
		case "item-106":
			bonus.Gold = int(float64(r.Round) * room.s.cfg.RNG.Float64() * 77)
		}
	}
	return bonus, CalculateExitFatigue(r.Round, r.Mode), nil
}

func (room stageRoom) Record(ctx context.Context, r *NormalRun, actor, disposition string) error {
	p := r.Recipients[actor]
	outcome := corebattle.OutcomeDraw
	winner := ""
	if disposition == "timeout" {
		outcome = corebattle.Outcome("timeout")
	} else if a := r.Actor(actor); a != nil && a.HP <= 0 {
		outcome = corebattle.OutcomeDefeat
	} else if r.StageCleared {
		outcome, winner = corebattle.OutcomeWin, actor
	}
	return room.s.cfg.History.Save(ctx, Adventure{ID: room.s.ID(r.ID + ":" + actor), CharacterID: actor, Type: r.Admission.StageID, StageID: r.Admission.StageID, Season: r.RecordSeason, SeasonalSubID: SeasonToSubID(r.RecordSeason), StartedAt: r.StartedAt, FloorsCleared: r.FloorsCleared, IsCleared: r.StageCleared, PartySize: len(r.Admission.CharacterIDs), ExperienceReward: p.Applied.EXP, Resolved: true, BattleResult: corebattle.Result{Outcome: outcome, WinnerID: winner, Reward: runhost.Reward(p.Applied)}})
}

func (stageRoom) Conditional(r *NormalRun) bool { return r.Mode == CrawlModeTry }

// Execute runs the stage executor and records the stage facts it reports:
// enemies a callback moved out of the frame, the EX marker and floor clear credit.
func (room stageRoom) Execute(ctx context.Context, r *NormalRun, cmd runhost.Command) (runhost.Effects, error) {
	if room.s.cfg.Combat == nil {
		return runhost.Effects{}, runhost.ErrUnavailable
	}
	fx, err := room.s.cfg.Combat.ExecuteRunAction(ctx, r, cmd)
	if err != nil {
		return runhost.Effects{}, err
	}
	r.EXUnlocked = r.EXUnlocked || fx.EXUnlocked
	for _, id := range fx.MemberOnlyIDs {
		if r.HasBox(id) {
			continue
		}
		a, actor := r.Actor(id), r.Actor(cmd.ActorID)
		if a == nil || actor == nil || a.TeamID == actor.TeamID || slices.Contains(r.Enemies, id) {
			return runhost.Effects{}, runhost.ErrConflict
		}
		if a.HP > 0 {
			r.Enemies = append(r.Enemies, id)
		}
	}
	if fx.EncounterDefeated {
		if r.LivingEnemies() || r.EncounterFloor < 1 || r.EncounterFloor > EXBossFloor || isTreasureFloor(r.EncounterFloor) {
			return runhost.Effects{}, runhost.ErrConflict
		}
		r.FloorsCleared = max(r.FloorsCleared, r.EncounterFloor)
		if r.EncounterFloor == BossFloor {
			r.StageCleared = true
		}
	}
	return runhost.Effects{Earnings: fx.Earnings, Consumed: fx.Consumed, RoomActivity: fx.RoomActivity, ActorActivity: fx.ActorActivity, ReleaseStock: fx.ReleaseStock, UnsealDemonKing: fx.UnsealDemonKing, Defeats: fx.Defeats}, nil
}

func (room stageRoom) StockLevel(ctx context.Context, r *NormalRun, cmd runhost.Command, requester corecharacter.Character, now time.Time) error {
	return room.s.stockLevel(ctx, r, cmd, requester, now)
}
