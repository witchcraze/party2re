package guild

import (
	"context"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// ActionTypeGuildInactivityCheck is the scheduled action type for purging inactive guilds (join_guild.cgi:check_dead_guild).
	ActionTypeGuildInactivityCheck = "guild_inactivity_check"
)

// Scheduler defines the scheduling capability needed for recurring actions.
type Scheduler interface {
	ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error
}

// NextMidnightJST returns the next 00:00:00 in Japan Standard Time (UTC+9) after t.
func NextMidnightJST(t time.Time) time.Time {
	jst := time.FixedZone("JST", 9*60*60)
	tJST := t.In(jst)
	return time.Date(tJST.Year(), tJST.Month(), tJST.Day()+1, 0, 0, 0, 0, jst)
}

// DailyInactivityCheckActionID returns the deterministic scheduled action ID for a given JST date.
func DailyInactivityCheckActionID(targetTime time.Time) string {
	jst := time.FixedZone("JST", 9*60*60)
	return "guild_inactivity_check:" + targetTime.In(jst).Format("2006-01-02")
}

// InactivityCheckOption configures an InactivityCheckHandler.
type InactivityCheckOption func(*InactivityCheckHandler)

// WithScheduler configures the scheduler used to re-enqueue daily recurring inactivity checks.
func WithScheduler(scheduler Scheduler) InactivityCheckOption {
	return func(h *InactivityCheckHandler) {
		h.scheduler = scheduler
	}
}

// InactivityCheckHandler implements scheduling.ActionHandler to disband inactive guilds.
type InactivityCheckHandler struct {
	service   *Service
	scheduler Scheduler
	nowFunc   func() time.Time
}

// NewInactivityCheckHandler creates a new InactivityCheckHandler.
func NewInactivityCheckHandler(service *Service, opts ...InactivityCheckOption) *InactivityCheckHandler {
	h := &InactivityCheckHandler{
		service: service,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the scheduled inactivity check and re-schedules the next daily check.
func (h *InactivityCheckHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	now := h.nowFunc()
	_, _, err := h.service.DisbandInactiveGuilds(ctx, now, 100)
	if err != nil {
		return err
	}

	if h.scheduler != nil {
		nextExec := NextMidnightJST(now)
		nextID := DailyInactivityCheckActionID(nextExec)
		_ = h.scheduler.ScheduleWithID(ctx, nextID, ActionTypeGuildInactivityCheck, "system", nil, nextExec)
	}

	return nil
}

const (
	// ActionTypeGuildPointDecay is the scheduled action type for daily 20% guild point decay (login.cgi:448).
	ActionTypeGuildPointDecay = "guild_point_decay"
)

// DailyPointDecayActionID returns the deterministic scheduled action ID for a given JST date.
func DailyPointDecayActionID(targetTime time.Time) string {
	jst := time.FixedZone("JST", 9*60*60)
	return "guild_point_decay:" + targetTime.In(jst).Format("2006-01-02")
}

// PointDecayOption configures a PointDecayHandler.
type PointDecayOption func(*PointDecayHandler)

// WithPointDecayScheduler configures the scheduler used to re-enqueue daily recurring point decay.
func WithPointDecayScheduler(scheduler Scheduler) PointDecayOption {
	return func(h *PointDecayHandler) {
		h.scheduler = scheduler
	}
}

// PointDecayHandler implements scheduling.ActionHandler to decay guild points daily.
type PointDecayHandler struct {
	service   *Service
	scheduler Scheduler
	nowFunc   func() time.Time
}

// NewPointDecayHandler creates a new PointDecayHandler.
func NewPointDecayHandler(service *Service, opts ...PointDecayOption) *PointDecayHandler {
	h := &PointDecayHandler{
		service: service,
		nowFunc: func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the daily guild point decay and re-schedules the next daily action.
func (h *PointDecayHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	now := h.nowFunc()
	if err := h.service.DecayGuildPoints(ctx, DefaultPointDecayFactor); err != nil {
		return err
	}

	if h.scheduler != nil {
		nextExec := NextMidnightJST(now)
		nextID := DailyPointDecayActionID(nextExec)
		_ = h.scheduler.ScheduleWithID(ctx, nextID, ActionTypeGuildPointDecay, "system", nil, nextExec)
	}

	return nil
}
