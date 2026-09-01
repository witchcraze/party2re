package maintenance

import (
	"context"
	"errors"
	"strings"
	"time"
)

var (
	ErrInvalidMessage = errors.New("maintenance message cannot exceed 500 characters")
)

// Status represents the system maintenance state.
type Status struct {
	Enabled          bool       `json:"enabled"`
	Message          string     `json:"message"`
	EstimatedEndTime *time.Time `json:"estimated_end_time,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Repository persists and retrieves system maintenance status.
type Repository interface {
	GetStatus(ctx context.Context) (Status, error)
	SetStatus(ctx context.Context, status Status) error
}

// Service manages the maintenance mode lifecycle.
type Service struct {
	repo Repository
	now  func() time.Time
}

// NewService creates a new maintenance service.
func NewService(repo Repository) (*Service, error) {
	if repo == nil {
		return nil, errors.New("maintenance repository is nil")
	}
	return &Service{
		repo: repo,
		now:  time.Now,
	}, nil
}

// GetStatus returns the current maintenance status.
func (s *Service) GetStatus(ctx context.Context) (Status, error) {
	return s.repo.GetStatus(ctx)
}

// SetMaintenance updates the maintenance mode state.
func (s *Service) SetMaintenance(ctx context.Context, enabled bool, message string, estimatedEndTime *time.Time) (Status, error) {
	message = strings.TrimSpace(message)
	if len(message) > 500 {
		return Status{}, ErrInvalidMessage
	}
	if message == "" {
		if enabled {
			message = "The system is currently undergoing maintenance. Please check back later."
		} else {
			message = "System is operating normally."
		}
	}

	status := Status{
		Enabled:          enabled,
		Message:          message,
		EstimatedEndTime: estimatedEndTime,
		UpdatedAt:        s.now().UTC(),
	}

	if err := s.repo.SetStatus(ctx, status); err != nil {
		return Status{}, err
	}
	return status, nil
}

// IsEnabled returns whether maintenance mode is currently active.
func (s *Service) IsEnabled(ctx context.Context) bool {
	st, err := s.repo.GetStatus(ctx)
	if err != nil {
		return false
	}
	return st.Enabled
}
