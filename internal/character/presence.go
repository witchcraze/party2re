package character

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/witchcraze/party2re/internal/validation"
)

const (
	// MaxLoginMessageLength is the login message limit in Unicode code points.
	MaxLoginMessageLength = 60
	// PresenceWindowSeconds is the sampled presence window (legacy 15-minute reference default).
	PresenceWindowSeconds = 900
)

var (
	ErrInvalidLoginMessage = errors.New("invalid login message")
	// ErrUnsafePresenceData reports stored public presence text that fails its output checks.
	ErrUnsafePresenceData = errors.New("stored presence data failed safety checks")
)

// PresenceKind distinguishes an explicit game-entry snapshot from a sampled-save snapshot.
type PresenceKind string

const (
	PresenceEntry  PresenceKind = "entry"
	PresenceSample PresenceKind = "sample"
)

// PresenceSnapshot is an immutable public copy taken when a presence record is published.
type PresenceSnapshot struct {
	CharacterID  string `json:"id"`
	Name         string `json:"name"`
	Color        string `json:"color"`
	GuildID      string `json:"guild_id"`
	GuildName    string `json:"guild_name"`
	IconURL      string `json:"icon_url"`
	AuraEffect   int    `json:"aura_effect"`
	LoginMessage string `json:"login_message"`
}

// PresenceObservation is one list/count observation at a single instant.
type PresenceObservation struct {
	ObservedAt  time.Time          `json:"observed_at"`
	OnlineCount int                `json:"online_count"`
	Characters  []PresenceSnapshot `json:"characters"`
}

// PresenceState is the character's current saved message and sampled-save clock,
// read independently of list snapshots. SampledAt is zero when no save was sampled.
type PresenceState struct {
	LoginMessage string
	SampledAt    time.Time
}

// PresenceRepository persists login messages, the sampled-save clock and snapshots.
// Write methods join the ambient transaction carried by ctx.
type PresenceRepository interface {
	// LockPresenceOwner locks the character row and returns its owning player ID.
	LockPresenceOwner(ctx context.Context, characterID string) (string, error)
	SetLoginMessage(ctx context.Context, characterID, message string) error
	// AdvanceSampleClock sets the clock to now only when unset or now > clock+window,
	// reporting whether it advanced.
	AdvanceSampleClock(ctx context.Context, characterID string, now, window int64) (bool, error)
	// PublishPresence replaces the snapshot of this kind unless the stored one is newer.
	PublishPresence(ctx context.Context, characterID string, kind PresenceKind, at int64, iconURL string) error
	PresenceIconSource(ctx context.Context, characterID string) (string, error)
	// ObservePresence prunes expired, banned and entry-hidden records visible at now
	// and returns the remaining public rows in display order, in one transaction.
	ObservePresence(ctx context.Context, now, window int64) ([]PresenceSnapshot, error)
	GetPresenceState(ctx context.Context, characterID string) (PresenceState, error)
}

// PresenceService records character presence and login messages.
type PresenceService struct {
	repo PresenceRepository
	tx   TransactionProvider
	now  func() time.Time
}

// NewPresenceService builds the service; now defaults to time.Now.
func NewPresenceService(repo PresenceRepository, tx TransactionProvider, now func() time.Time) (*PresenceService, error) {
	if repo == nil || tx == nil {
		return nil, errors.New("presence repository and transaction provider are required")
	}
	if now == nil {
		now = time.Now
	}
	return &PresenceService{repo: repo, tx: tx, now: now}, nil
}

// ValidateLoginMessage applies the verbatim 0–60 code point message contract.
func ValidateLoginMessage(message string) error {
	if err := validation.ValidateVerbatimLine(message, MaxLoginMessageLength); err != nil {
		return errors.Join(ErrInvalidLoginMessage, err)
	}
	return nil
}

// Enter records explicit game entry for the acting player's character. A nil
// message preserves the saved one; "" clears it. Entry joins a due sampled save
// without resetting the sample clock, then publishes its own snapshot.
// Transaction: RunInTx (joins an ambient transaction)
// Lock Order: characters(2) -> character_presence_snapshots
func (s *PresenceService) Enter(ctx context.Context, playerID, characterID string, message *string) error {
	if message != nil {
		if err := ValidateLoginMessage(*message); err != nil {
			return err
		}
	}
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.lockOwned(ctx, playerID, characterID); err != nil {
			return err
		}
		// Read the clock only after the lock so lock order and recorded time agree.
		now := s.now().Unix()
		if message != nil {
			if err := s.repo.SetLoginMessage(ctx, characterID, *message); err != nil {
				return err
			}
		}
		if err := s.sample(ctx, characterID, now); err != nil {
			return err
		}
		return s.publish(ctx, characterID, PresenceEntry, now)
	})
}

// RecordSave participates in a gameplay save: it joins the caller's transaction
// and publishes a sample only when the sampled-save clock is due.
// Transaction: RunInTx (joins an ambient transaction)
// Lock Order: characters(2) -> character_presence_snapshots
func (s *PresenceService) RecordSave(ctx context.Context, playerID, characterID string) error {
	return s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if err := s.lockOwned(ctx, playerID, characterID); err != nil {
			return err
		}
		return s.sample(ctx, characterID, s.now().Unix())
	})
}

// Observe returns the public list and count from one observation instant.
// Unsafe stored data fails the whole observation instead of returning partial rows.
func (s *PresenceService) Observe(ctx context.Context) (PresenceObservation, error) {
	now := s.now().UTC().Truncate(time.Second)
	var rows []PresenceSnapshot
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		var err error
		rows, err = s.repo.ObservePresence(ctx, now.Unix(), PresenceWindowSeconds)
		return err
	})
	if err != nil {
		return PresenceObservation{}, err
	}
	for _, row := range rows {
		if err := checkPublicSnapshot(row); err != nil {
			return PresenceObservation{}, err
		}
	}
	if rows == nil {
		rows = []PresenceSnapshot{}
	}
	return PresenceObservation{ObservedAt: now, OnlineCount: len(rows), Characters: rows}, nil
}

// State returns the character's current saved message and sampled-save clock.
func (s *PresenceService) State(ctx context.Context, characterID string) (PresenceState, error) {
	state, err := s.repo.GetPresenceState(ctx, characterID)
	if err != nil {
		return PresenceState{}, err
	}
	if err := ValidateLoginMessage(state.LoginMessage); err != nil {
		return PresenceState{}, errors.Join(ErrUnsafePresenceData, err)
	}
	return state, nil
}

func (s *PresenceService) lockOwned(ctx context.Context, playerID, characterID string) error {
	if playerID == "" {
		return ErrInvalidPlayer
	}
	owner, err := s.repo.LockPresenceOwner(ctx, characterID)
	if err != nil {
		return err
	}
	if owner != playerID {
		return ErrForbidden
	}
	return nil
}

func (s *PresenceService) sample(ctx context.Context, characterID string, now int64) error {
	due, err := s.repo.AdvanceSampleClock(ctx, characterID, now, PresenceWindowSeconds)
	if err != nil || !due {
		return err
	}
	return s.publish(ctx, characterID, PresenceSample, now)
}

// publish copies the current display fields; an icon outside the avatar contract
// is published empty rather than blocking presence.
func (s *PresenceService) publish(ctx context.Context, characterID string, kind PresenceKind, now int64) error {
	icon, err := s.repo.PresenceIconSource(ctx, characterID)
	if err != nil {
		return err
	}
	if !IsSafePresenceIcon(icon) {
		icon = ""
	}
	return s.repo.PublishPresence(ctx, characterID, kind, now, icon)
}

var (
	presenceColorPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}$`)
)

// IsSafePresenceIcon accepts only the stored avatar contract (empty or canonical imgur URL).
func IsSafePresenceIcon(icon string) bool {
	return ValidateAvatarURL(icon) == nil
}

func checkPublicSnapshot(row PresenceSnapshot) error {
	if row.Name == "" ||
		validation.ValidateVerbatimLine(row.Name, 32) != nil ||
		validation.ValidateVerbatimLine(row.GuildName, 64) != nil ||
		ValidateLoginMessage(row.LoginMessage) != nil ||
		!presenceColorPattern.MatchString(row.Color) ||
		ValidateAuraEffect(row.AuraEffect) != nil ||
		!IsSafePresenceIcon(row.IconURL) {
		return ErrUnsafePresenceData
	}
	return nil
}
