package rescue

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corescheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
)

type RescueRepository interface {
	Save(ctx context.Context, record RescueRecord) error
	FindLatestByCharacterID(ctx context.Context, characterID string) (RescueRecord, error)
	MarkApplied(ctx context.Context, id string, at time.Time) error
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
}

type ActionCleaner interface {
	CancelByActorID(ctx context.Context, characterID string) (int, error)
	FindPendingByActorID(ctx context.Context, actorID string) ([]corescheduling.ScheduledAction, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// ActivityReader lists feature activities still owned by the character.
type ActivityReader interface {
	ActivityKinds(ctx context.Context, ownerID, characterID string) ([]string, error)
}

type TimerService interface {
	ExtendLockUntil(ctx context.Context, category, targetID string, until time.Time) error
	GetRemainingLock(ctx context.Context, category, targetID string) (time.Duration, error)
}

type Service struct {
	rescues    RescueRepository
	characters CharacterRepository
	cleaner    ActionCleaner
	timer      TimerService
	tx         TransactionProvider
	activities ActivityReader
}

// SetActivityReader wires the feature activity observation after composition.
func (s *Service) SetActivityReader(r ActivityReader) {
	s.activities = r
}

// ensureNoActivity rejects rescue while feature-owned state the cleaner cannot end remains.
func (s *Service) ensureNoActivity(ctx context.Context, char corecharacter.Character) error {
	if s.activities == nil {
		return nil
	}
	kinds, err := s.activities.ActivityKinds(ctx, char.PlayerID, char.ID)
	if err != nil {
		return err
	}
	if len(kinds) > 0 {
		return fmt.Errorf("%w: %s", ErrActivityRemaining, strings.Join(kinds, ","))
	}
	return nil
}

func NewService(
	rescues RescueRepository,
	characters CharacterRepository,
	cleaner ActionCleaner,
	timer TimerService,
	tx TransactionProvider,
) *Service {
	return &Service{
		rescues:    rescues,
		characters: characters,
		cleaner:    cleaner,
		timer:      timer,
		tx:         tx,
	}
}

// Unstuck resets the character's active and pending scheduled actions without applying
// a rescue penalty cooldown (admin refresh per legacy party2/admin.cgi:121-131).
// It returns ErrActivityRemaining without changes while feature activity remains.
func (s *Service) Unstuck(ctx context.Context, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrInvalidCharacterID
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return err
	}
	if err := s.ensureNoActivity(ctx, char); err != nil {
		return err
	}

	if s.cleaner != nil {
		if _, err := s.cleaner.CancelByActorID(ctx, characterID); err != nil {
			return err
		}
	}

	return nil
}

// EmergencyRescue clears scheduled work and adds the penalty to any remaining sleep.
// With no pending work it returns a 0-second result without changes (legacy rescue.cgi:56-65).
// Feature activity the scheduler cannot end returns ErrActivityRemaining without changes,
// except that an unconfirmed penalty whose work is already cleared still receives its sleep.
//
// The owed deadline is committed to rescue_records before Valkey is touched; clearing and
// the sleep timer follow and the record is confirmed last. Any failure leaves the record
// unconfirmed and the next rescue resumes it with the same deadline (#1100).
// Transaction: separate RunInTx boundaries for the durable receipt and its application.
// Lock Order: characters(2), held through application confirmation in the second boundary.
func (s *Service) EmergencyRescue(ctx context.Context, characterID, reason string, now time.Time) (RescueRecord, error) {
	if strings.TrimSpace(characterID) == "" {
		return RescueRecord{}, ErrInvalidCharacterID
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return RescueRecord{}, ErrInvalidReason
	}

	rec, err := s.recordPenalty(ctx, characterID, reason, now)
	if err != nil || rec.ID == "" {
		return rec, err
	}
	if err := s.applyPenalty(ctx, rec); err != nil {
		return RescueRecord{}, err
	}
	return rec, nil
}

// recordPenalty returns the unconfirmed penalty to resume, a new committed one, or an
// idle result without ID. The character row lock makes concurrent rescues share one record.
// Remaining activity only permits resuming a penalty whose work is already gone:
// a player who started an activity after a failed sleep write still owes that sleep.
func (s *Service) recordPenalty(ctx context.Context, characterID, reason string, now time.Time) (RescueRecord, error) {
	rec := RescueRecord{CharacterID: characterID, Reason: reason, CreatedAt: now}
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(ctx, characterID)
		if err != nil {
			return err
		}
		activityErr := s.ensureNoActivity(ctx, char)
		if activityErr != nil && !errors.Is(activityErr, ErrActivityRemaining) {
			return activityErr
		}
		latest, err := s.rescues.FindLatestByCharacterID(ctx, characterID)
		resume := err == nil && latest.AppliedAt.IsZero()
		if err != nil && !errors.Is(err, ErrNoRescueRecord) {
			return err
		}
		if resume && activityErr == nil {
			rec = latest
			return nil
		}
		if !resume && activityErr != nil {
			return activityErr
		}
		pending, err := s.cleaner.FindPendingByActorID(ctx, characterID)
		if err != nil {
			return err
		}
		if resume {
			if len(pending) > 0 {
				return activityErr
			}
			rec = latest
			return nil
		}
		if len(pending) == 0 {
			return nil
		}
		remaining, err := s.timer.GetRemainingLock(ctx, timer.CategorySleep, characterID)
		if err != nil {
			return err
		}
		rec.ID = id.New()
		rec.PenaltySeconds = DefaultPenaltySeconds
		rec.SleepUntil = now.Add(remaining + DefaultPenaltySeconds*time.Second)
		return s.rescues.Save(ctx, rec)
	})
	if err != nil {
		return RescueRecord{}, err
	}
	return rec, nil
}

// applyPenalty revalidates the committed receipt under the character lock and holds
// it through confirmation. A delayed duplicate must not cancel successor work.
func (s *Service) applyPenalty(ctx context.Context, rec RescueRecord) error {
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(ctx, rec.CharacterID)
		if err != nil {
			return err
		}
		latest, err := s.rescues.FindLatestByCharacterID(ctx, rec.CharacterID)
		if err != nil {
			return err
		}
		if latest.ID != rec.ID || !latest.AppliedAt.IsZero() {
			return nil
		}
		activityErr := s.ensureNoActivity(ctx, char)
		if activityErr != nil {
			if !errors.Is(activityErr, ErrActivityRemaining) {
				return activityErr
			}
			pending, err := s.cleaner.FindPendingByActorID(ctx, rec.CharacterID)
			if err != nil {
				return err
			}
			if len(pending) > 0 {
				return activityErr
			}
		} else if _, err := s.cleaner.CancelByActorID(ctx, rec.CharacterID); err != nil {
			return err
		}
		if err := s.timer.ExtendLockUntil(ctx, timer.CategorySleep, rec.CharacterID, latest.SleepUntil); err != nil {
			return err
		}
		return s.rescues.MarkApplied(ctx, rec.ID, time.Now().UTC())
	})
}
