package main

import (
	"context"
	"time"

	"github.com/witchcraze/party2re/internal/activity"
	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/ranking"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func registerWorkers(soc *socServices, cmbt *cmbtServices, misc *miscServices) error {
	if err := registerRankingWorker(soc); err != nil {
		return err
	}
	if cmbt.pvp != nil {
		soc.worker.RegisterReconciler("pvp_inactivity_sweep", cmbt.pvp.SweepInactiveRooms)
	}
	if cmbt.normal != nil {
		soc.worker.RegisterReconciler("normal_run_idle_sweep", cmbt.normal.SweepIdle)
	}
	if cmbt.boss != nil {
		soc.worker.RegisterReconciler("boss_run_idle_sweep", cmbt.boss.SweepIdle)
	}

	soc.registerWorkerHandlers(misc.activity, misc.chapel, misc.lottery, misc.contest)

	if soc.sched != nil && misc.lottery != nil {
		if err := wireTakarakujiDrawing(soc.sched, misc.lottery); err != nil {
			return err
		}
	}
	if soc.sched != nil && soc.guild != nil {
		wireGuildInactivityCheck(soc.sched)
		wireGuildPointDecay(soc.sched)
	}
	if soc.sched != nil && misc.contest != nil {
		if err := wireContestSettlement(soc.sched, misc.contest); err != nil {
			return err
		}
	}
	return nil
}

// wireContestSettlement enqueues the contest settlement action if not already scheduled.
func wireContestSettlement(sched *scheduling.Service, contestSvc *contest.Service) error {
	if contestSvc == nil {
		return nil
	}
	if sched != nil {
		contestSvc.SetScheduler(sched)
	}
	return contestSvc.ReconcileSettlementSchedule(context.Background())
}

// wireGuildInactivityCheck enqueues the daily JST midnight inactivity check if not already scheduled.
func wireGuildInactivityCheck(sched *scheduling.Service) {
	ctx := context.Background()
	next := guild.NextMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, guild.DailyInactivityCheckActionID(next), guild.ActionTypeGuildInactivityCheck, "system", nil, next)
}

// wireGuildPointDecay enqueues the daily JST midnight guild point decay if not already scheduled.
func wireGuildPointDecay(sched *scheduling.Service) {
	ctx := context.Background()
	next := guild.NextMidnightJST(time.Now())
	_ = sched.ScheduleWithID(ctx, guild.DailyPointDecayActionID(next), guild.ActionTypeGuildPointDecay, "system", nil, next)
}

// wireTakarakujiDrawing enqueues the recurring Takarakuji drawing action if not already scheduled.
func wireTakarakujiDrawing(sched *scheduling.Service, lotterySvc *lottery.Service) error {
	return lottery.NewDrawHandler(lotterySvc, lottery.WithScheduler(sched)).Reconcile(context.Background())
}

func registerRankingWorker(soc *socServices) error {
	weekly := ranking.NewRotateWeeklyHandler(soc.ranking, soc.sched)
	if err := weekly.Reconcile(context.Background()); err != nil {
		return err
	}
	soc.worker.RegisterHandler(ranking.RankingActionTypeRotateWeekly, weekly)
	soc.worker.RegisterReconciler(ranking.RankingActionTypeRotateWeekly, weekly.Reconcile)
	daily := ranking.NewRefreshHandler(soc.ranking, ranking.WithRefreshScheduler(soc.sched))
	if err := daily.Reconcile(context.Background()); err != nil {
		return err
	}
	soc.worker.RegisterHandler(ranking.RankingActionTypeRefresh, daily)
	soc.worker.RegisterReconciler(ranking.RankingActionTypeRefresh, daily.Reconcile)
	return nil
}

func (s *socServices) registerWorkerHandlers(activityService *activity.Service, chapelService *chapel.Service, lotteryService *lottery.Service, contestService *contest.Service) {
	if s.worker != nil {
		if activityService != nil {
			s.worker.RegisterHandler(activity.ActivityActionTypeTrainingComplete, activity.NewTrainingHandler(activityService))
		}
		if chapelService != nil {
			s.worker.RegisterHandler(chapel.ActionTypeChapelReset, chapel.NewResetHandler(chapelService, chapel.WithScheduler(s.sched)))
		}
		if lotteryService != nil {
			draw := lottery.NewDrawHandler(lotteryService, lottery.WithScheduler(s.sched))
			s.worker.RegisterHandler(lottery.ActionTypeTakarakujiDraw, draw)
			s.worker.RegisterReconciler(lottery.ActionTypeTakarakujiDraw, draw.Reconcile)
		}
		if s.guild != nil {
			s.worker.RegisterHandler(guild.ActionTypeGuildInactivityCheck, guild.NewInactivityCheckHandler(s.guild, guild.WithScheduler(s.sched)))
			s.worker.RegisterHandler(guild.ActionTypeGuildPointDecay, guild.NewPointDecayHandler(s.guild, guild.WithPointDecayScheduler(s.sched)))
		}
		if contestService != nil {
			handler := contest.NewContestSettlementHandler(contestService, contest.WithScheduler(s.sched))
			s.worker.RegisterHandler(contest.ActionTypeContestSettlement, handler)
			s.worker.RegisterReconciler(contest.ActionTypeContestSettlement, handler.Reconcile)
		}
	}
}
