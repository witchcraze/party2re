package lottery

import (
	"context"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

const (
	// ActionTypeTakarakujiDraw is the scheduled action type for executing recurring takarakuji drawings.
	ActionTypeTakarakujiDraw = "takarakuji_draw"
)

// Scheduler defines the scheduling capability needed for recurring actions.
type Scheduler interface {
	ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error
}

// DrawActionID returns the deterministic scheduled action ID for a given JST draw time.
func DrawActionID(targetTime time.Time) string {
	jst := time.FixedZone("JST", 9*60*60)
	return "takarakuji_draw:" + targetTime.In(jst).Format("2006-01-02")
}

// DrawOption configures a DrawHandler.
type DrawOption func(*DrawHandler)

// WithScheduler configures the scheduler used to re-enqueue recurring takarakuji drawings.
func WithScheduler(scheduler Scheduler) DrawOption {
	return func(h *DrawHandler) {
		h.scheduler = scheduler
	}
}

// DrawHandler implements scheduling.ActionHandler to execute periodic takarakuji drawings.
type DrawHandler struct {
	service   *Service
	scheduler Scheduler
}

// NewDrawHandler creates a new DrawHandler.
func NewDrawHandler(service *Service, opts ...DrawOption) *DrawHandler {
	h := &DrawHandler{
		service: service,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// Handle executes the takarakuji drawing logic and schedules the next round.
func (h *DrawHandler) Handle(ctx context.Context, action core_scheduling.ScheduledAction) error {
	now := time.Now().UTC()
	result, err := h.service.DrawTakarakuji(ctx, now)
	if err != nil {
		return err
	}

	if h.scheduler != nil {
		nextDrawDate := result.NextRound.DrawDate
		_ = h.scheduler.ScheduleWithID(ctx, DrawActionID(nextDrawDate), ActionTypeTakarakujiDraw, "system", nil, nextDrawDate)
	}

	return nil
}
