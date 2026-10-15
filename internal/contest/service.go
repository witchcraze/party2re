package contest

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
)

type Service struct {
	characters        CharacterRepository
	contests          ContestRepository
	guilds            GuildService
	news              NewsPublisher
	scheduler         Scheduler
	txProvider        TransactionProvider
	nowFunc           func() time.Time
	settlementActorID string
	reconcileMu       sync.Mutex
}

type Option func(*Service)

func WithTransactionProvider(tp TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tp
	}
}

func WithServiceScheduler(scheduler Scheduler) Option {
	return func(s *Service) {
		s.scheduler = scheduler
	}
}

func WithSettlementActorID(actorID string) Option {
	return func(s *Service) {
		s.settlementActorID = actorID
	}
}

// SettlementActorID returns the actor ID used for contest settlement reservations.
func (s *Service) SettlementActorID() string {
	if s.settlementActorID != "" {
		return s.settlementActorID
	}
	return ContestSettlementActorID
}

// SetScheduler sets the scheduler used by the service if not already configured.
func (s *Service) SetScheduler(scheduler Scheduler) {
	s.scheduler = scheduler
}

func WithGuildService(gs GuildService) Option {
	return func(s *Service) {
		s.guilds = gs
	}
}

func WithNewsPublisher(np NewsPublisher) Option {
	return func(s *Service) {
		s.news = np
	}
}

func WithNowFunc(fn func() time.Time) Option {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func NewService(
	characters CharacterRepository,
	contests ContestRepository,
	opts ...Option,
) (*Service, error) {
	if characters == nil {
		return nil, errors.New("characters repository is nil")
	}
	if contests == nil {
		return nil, errors.New("contests repository is nil")
	}

	s := &Service{
		characters: characters,
		contests:   contests,
		nowFunc:    func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(txCtx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

// GetOverview returns a summary of current contest rounds and active submissions.
// Opportunistically reconciles the contest settlement schedule to self-heal missing reservations during runtime.
func (s *Service) GetOverview(ctx context.Context) (ContestOverview, error) {
	if err := s.ReconcileSettlementSchedule(ctx); err != nil {
		return ContestOverview{}, err
	}

	overview := ContestOverview{
		MinEntries: MinEntriesForContest,
	}

	activeRound, err := s.contests.GetActiveRound(ctx)
	if err == nil {
		overview.ActiveRound = &activeRound
		entries, err := s.contests.ListEntriesByRound(ctx, activeRound.Round)
		if err == nil {
			overview.ActiveEntries = entries
			overview.EntryCount = len(entries)
			overview.IsPostponed = len(entries) < MinEntriesForContest
		}
	}

	prepRound, err := s.contests.GetPreparingRound(ctx)
	if err == nil {
		overview.PreparingRound = &prepRound
	}

	return overview, nil
}

// EnterContest submits a photo entry for the upcoming/preparing contest round.
func (s *Service) EnterContest(
	ctx context.Context,
	characterID string,
	photoID string,
	title string,
) (ContestEntry, error) {
	characterID = strings.TrimSpace(characterID)
	photoID = strings.TrimSpace(photoID)
	title = strings.TrimSpace(title)

	if characterID == "" {
		return ContestEntry{}, ErrCharacterNotFound
	}
	if photoID == "" {
		return ContestEntry{}, ErrPhotoNotFound
	}
	if err := ValidateTitle(title); err != nil {
		return ContestEntry{}, err
	}

	var entry ContestEntry
	var prepRoundEndTime time.Time
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByID(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		photo, err := s.contests.FindPhotoByID(txCtx, photoID)
		if err != nil {
			return err
		}
		if photo.CharacterID != characterID {
			return ErrForbidden
		}

		// Consecutive entry check: if character is in active contest, cannot enter preparing contest
		activeRound, err := s.contests.GetActiveRound(txCtx)
		if err == nil && activeRound.Status == StatusActive {
			_, err := s.contests.FindEntryByRoundAndCharacter(txCtx, activeRound.Round, characterID)
			if err == nil {
				return ErrConsecutiveEntryDisallowed
			}
		}

		// Ensure preparing round exists
		prepRound, err := s.ensurePreparingRoundInTx(txCtx)
		if err != nil {
			return err
		}
		prepRoundEndTime = prepRound.EndTime

		// Check if character already entered this preparing round
		if _, err := s.contests.FindEntryByRoundAndCharacter(txCtx, prepRound.Round, characterID); err == nil {
			return ErrAlreadyEntered
		}

		// Check if title already taken in this preparing round
		if _, err := s.contests.FindEntryByRoundAndTitle(txCtx, prepRound.Round, title); err == nil {
			return ErrDuplicateTitle
		}

		var guildRef GuildRef
		if s.guilds != nil {
			ref, ok, err := s.guilds.GuildOf(txCtx, characterID)
			if err != nil {
				return err
			}
			if ok {
				guildRef = ref
			}
		}

		now := s.nowFunc()
		entry = ContestEntry{
			ID:            id.New(),
			Round:         prepRound.Round,
			CharacterID:   char.ID,
			CharacterName: char.Name,
			GuildID:       guildRef.ID,
			GuildName:     guildRef.Name,
			Title:         title,
			PhotoID:       photo.ID,
			ImageURL:      photo.ImageURL,
			Caption:       photo.Caption,
			Votes:         0,
			Ranking:       0,
			CreatedAt:     now,
			UpdatedAt:     now,
		}

		return s.contests.SaveEntry(txCtx, entry)
	})
	if err != nil {
		return ContestEntry{}, err
	}

	if s.scheduler != nil && !prepRoundEndTime.IsZero() {
		if err := s.ReconcileSettlementSchedule(ctx); err != nil {
			return entry, fmt.Errorf("%w: %v", ErrScheduleFailed, err)
		}
	}

	return entry, nil

}

// Vote casts a vote for an entry in the currently active contest round.
func (s *Service) Vote(
	ctx context.Context,
	voterCharacterID string,
	entryID string,
	comment string,
) (ContestVote, error) {
	voterCharacterID = strings.TrimSpace(voterCharacterID)
	entryID = strings.TrimSpace(entryID)
	comment = strings.TrimSpace(comment)

	if voterCharacterID == "" {
		return ContestVote{}, ErrCharacterNotFound
	}
	if entryID == "" {
		return ContestVote{}, ErrEntryNotFound
	}
	if err := ValidateComment(comment); err != nil {
		return ContestVote{}, err
	}

	var vote ContestVote
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		voter, err := s.characters.FindByID(txCtx, voterCharacterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		activeRound, err := s.contests.GetActiveRound(txCtx)
		if err != nil {
			if errors.Is(err, ErrContestNotFound) {
				return ErrContestNotActive
			}
			return err
		}
		if activeRound.Status != StatusActive {
			return ErrContestNotActive
		}

		hasVoted, err := s.contests.HasVotedInRound(txCtx, activeRound.Round, voterCharacterID)
		if err != nil {
			return err
		}
		if hasVoted {
			return ErrAlreadyVoted
		}

		entry, err := s.contests.FindEntryByIDForUpdate(txCtx, entryID)
		if err != nil {
			return err
		}
		if entry.Round != activeRound.Round {
			return ErrEntryNotFound
		}

		now := s.nowFunc()
		vote = ContestVote{
			ID:                 id.New(),
			Round:              activeRound.Round,
			EntryID:            entry.ID,
			VoterCharacterID:   voter.ID,
			VoterCharacterName: voter.Name,
			Comment:            comment,
			CreatedAt:          now,
		}

		if err := s.contests.SaveVote(txCtx, vote); err != nil {
			return err
		}

		return s.contests.IncrementEntryVotes(txCtx, entry.ID)
	})
	if err != nil {
		return ContestVote{}, err
	}

	return vote, nil
}

// GetCurrentEntries returns all entries in the active contest round.
func (s *Service) GetCurrentEntries(ctx context.Context) ([]ContestEntry, error) {
	activeRound, err := s.contests.GetActiveRound(ctx)
	if err != nil {
		if errors.Is(err, ErrContestNotFound) {
			return []ContestEntry{}, nil
		}
		return nil, err
	}
	return s.contests.ListEntriesByRound(ctx, activeRound.Round)
}

// GetPastResults retrieves rankings and feedback from the latest settled contest round.
func (s *Service) GetPastResults(ctx context.Context) (*ContestRound, []ContestEntry, error) {
	round, err := s.contests.GetLatestSettledRound(ctx)
	if err != nil {
		if errors.Is(err, ErrContestNotFound) {
			return nil, []ContestEntry{}, nil
		}
		return nil, nil, err
	}

	entries, err := s.contests.ListEntriesByRound(ctx, round.Round)
	if err != nil {
		return nil, nil, err
	}

	// Attach comments to entries
	for i := range entries {
		votes, err := s.contests.ListVotesByEntryID(ctx, entries[i].ID)
		if err == nil {
			var comments []string
			for _, v := range votes {
				if v.Comment != "" {
					comments = append(comments, fmt.Sprintf("%s: %s", v.VoterCharacterName, v.Comment))
				}
			}
			entries[i].Comments = comments
		}
	}

	return &round, entries, nil
}

// GetLegends retrieves the Hall of Fame archive of past 1st-place winners.
func (s *Service) GetLegends(ctx context.Context, limit, offset int) (pagination.Page[ContestLegend], error) {
	limit, offset = pagination.Normalize(limit, offset)
	items, total, err := s.contests.ListLegends(ctx, limit, offset)
	if err != nil {
		return pagination.Page[ContestLegend]{}, err
	}
	return pagination.NewPage(items, total, limit, offset), nil
}

// GetActiveRound returns the current active contest round.
func (s *Service) GetActiveRound(ctx context.Context) (ContestRound, error) {
	return s.contests.GetActiveRound(ctx)
}

// GetRoundByNumber retrieves a contest round by its round number.
func (s *Service) GetRoundByNumber(ctx context.Context, round int) (ContestRound, error) {
	return s.contests.GetRoundByNumber(ctx, round)
}

// ReconcileSettlementSchedule ensures the settlement scheduled action exists for the current contest round.
// It inspects current SQL round state (active round, or preparing round if no active round exists),
// discovers active reservations via FindPendingByActorID, avoids duplicating Pending or overwriting
// Processing actions, and registers the deterministic settlement reservation with a safe nonce suffix.
func (s *Service) ReconcileSettlementSchedule(ctx context.Context) error {
	if s.scheduler == nil {
		return nil
	}

	s.reconcileMu.Lock()
	defer s.reconcileMu.Unlock()

	var targetRound int
	var targetEndTime time.Time

	activeRound, err := s.contests.GetActiveRound(ctx)
	if err == nil {
		targetRound = activeRound.Round
		targetEndTime = activeRound.EndTime
	} else if errors.Is(err, ErrContestNotFound) {
		prepRound, prepErr := s.EnsurePreparingRound(ctx)
		if prepErr != nil {
			return fmt.Errorf("ensure preparing contest round: %w", prepErr)
		}
		targetRound = prepRound.Round
		targetEndTime = prepRound.EndTime
	} else {
		return fmt.Errorf("read active contest round: %w", err)
	}

	if targetEndTime.IsZero() {
		return nil
	}

	actorID := s.SettlementActorID()
	activeActions, err := s.scheduler.FindPendingByActorID(ctx, actorID)
	if err != nil {
		return fmt.Errorf("discover contest settlement reservation: %w", err)
	}
	// Check legacy actor ID "system" for backward compatibility with pre-upgrade actions
	if actorID != "system" {
		legacyActions, err := s.scheduler.FindPendingByActorID(ctx, "system")
		if err != nil {
			return fmt.Errorf("discover legacy contest settlement reservation: %w", err)
		}
		activeActions = append(activeActions, legacyActions...)
	}

	for _, action := range activeActions {
		if action.ActionType != ActionTypeContestSettlement {
			continue
		}
		round, targetTime, ok := ParseSettlementActionID(action.ID)
		if !ok {
			continue
		}
		if round == targetRound && action.ExecuteAt.Equal(targetEndTime) && targetTime.Equal(targetEndTime) {
			if action.State == core_scheduling.StatePending || action.State == core_scheduling.StateProcessing {
				return nil
			}
		}
	}

	actionID := fmt.Sprintf("%s:%s", SettlementActionID(targetRound, targetEndTime), id.New())
	if err := s.scheduler.ScheduleWithID(ctx, actionID, ActionTypeContestSettlement, actorID, nil, targetEndTime); err != nil {
		return fmt.Errorf("%w: %v", ErrScheduleFailed, err)
	}

	return nil
}
