package scheduling

import (
	"context"
	
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
)

// ActionHandler processes a specific type of scheduled action.
type ActionHandler interface {
	Handle(ctx context.Context, action core_scheduling.ScheduledAction) error
}
