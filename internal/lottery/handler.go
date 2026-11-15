package lottery

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
)

const (
	// ActionTypeTakarakujiDraw is the scheduled action type for executing recurring takarakuji drawings.
	ActionTypeTakarakujiDraw = "takarakuji_draw"
)

// Scheduler defines the scheduling capability needed for recurring actions.
type Scheduler interface {
	ScheduleWithID(ctx context.Context, id, actionType, actorID string, params map[string]string, executeAt time.Time) error
	FindPendingByActorID(ctx context.Context, actorID string) ([]core_scheduling.ScheduledAction, error)
}

// DrawActionID returns the date prefix shared by reservations for a JST draw time.
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
	reconcileMu sync.Mutex
	service     *Service
	scheduler   Scheduler
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
	now := h.service.now()
	if action.ExecuteAt.After(now) {
		now = action.ExecuteAt
	}
	result, err := h.service.DrawTakarakuji(ctx, now)
	if errors.Is(err, ErrNotReadyToDraw) || errors.Is(err, ErrAlreadyDrawn) {
		return h.Reconcile(ctx)
	}
	if err != nil {
		return err
	}

	h.reconcileMu.Lock()
	defer h.reconcileMu.Unlock()
	return h.scheduleRound(ctx, result.NextRound)
}

// Reconcile restores the current SQL round's reservation without executing a draw.
func (h *DrawHandler) Reconcile(ctx context.Context) error {
	if h.scheduler == nil {
		return nil
	}
	h.reconcileMu.Lock()
	defer h.reconcileMu.Unlock()
	round, err := h.service.getOrCreateActiveRound(ctx, h.service.now())
	if err != nil {
		return err
	}
	return h.scheduleRound(ctx, round)
}

func (h *DrawHandler) scheduleRound(ctx context.Context, round TakarakujiRound) error {
	if h.scheduler == nil {
		return nil
	}
	actions, err := h.scheduler.FindPendingByActorID(ctx, "system")
	if err != nil {
		return err
	}
	for _, action := range actions {
		if action.ActionType == ActionTypeTakarakujiDraw && action.ExecuteAt.Equal(round.DrawDate) &&
			(action.State == core_scheduling.StatePending || action.State == core_scheduling.StateProcessing) {
			return nil
		}
	}
	// Discovery and registration are not atomic; never overwrite a concurrent worker's payload.
	actionID := fmt.Sprintf("%s:round-%d:%s", DrawActionID(round.DrawDate), round.RoundID, id.New())
	return h.scheduler.ScheduleWithID(ctx, actionID, ActionTypeTakarakujiDraw, "system", nil, round.DrawDate)
}
