package contest

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
)

type Service struct {
	characters CharacterRepository
	contests   ContestRepository
	guilds     GuildService
	news       NewsPublisher
	txProvider TransactionProvider
	nowFunc    func() time.Time
}

type Option func(*Service)

func WithTransactionProvider(tp TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tp
	}
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

// GetDialogue returns the NPC @ワコール dialogue in the Photo Contest venue.
func (s *Service) GetDialogue() Dialogue {
	return Dialogue{
		NPCName:  "@ワコール",
		Title:    "フォトコン会場",
		Greeting: "フォトコンテストの会場、略してフォトコン会場へようこそ。私が主催者のワコールざます",
		Phrases: []string{
			"ここでは、あなたが撮ったスクリーンショットを消したり、コンテストに応募したりできるざます",
			"コンテスト上位入賞者には、ゴールドと賞品が授与されるざます",
			"コンテスト１位の作品に投票した参加者にも小さなメダルが配られるざます",
			"フォトコンで重要なのは、何が写っているかはもちろん。タイトルやコメントなども重要なポイントざます",
			"自分で撮ったスクリーンショットを見たり消すことができるざます",
			"ただ撮るだけではなく、コスプレしたり色々と工夫することが大事ざます",
			"スクリーンショットは最大20枚まで所持することができるざます。それ以上は、＠けす必要があるざます",
		},
	}
}

// GetOverview returns a summary of current contest rounds and active submissions.
func (s *Service) GetOverview(ctx context.Context) (ContestOverview, error) {
	overview := ContestOverview{
		MinEntries: MinEntriesForContest,
		Dialogue:   s.GetDialogue(),
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

// SavePhoto stores a new captured photo/screenshot for a character.
func (s *Service) SavePhoto(
	ctx context.Context,
	characterID string,
	title string,
	location string,
	imageURL string,
	caption string,
	metadata string,
) (Photo, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return Photo{}, ErrCharacterNotFound
	}
	if err := ValidateTitle(title); err != nil {
		return Photo{}, err
	}

	var saved Photo
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		_, err := s.characters.FindByID(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		count, err := s.contests.CountPhotosByCharacterID(txCtx, characterID)
		if err != nil {
			return err
		}
		if count >= MaxPhotosPerCharacter {
			return ErrMaxPhotosReached
		}

		now := s.nowFunc()
		saved = Photo{
			ID:          id.New(),
			CharacterID: characterID,
			Title:       strings.TrimSpace(title),
			Location:    strings.TrimSpace(location),
			ImageURL:    strings.TrimSpace(imageURL),
			Caption:     strings.TrimSpace(caption),
			Metadata:    metadata,
			CreatedAt:   now,
			UpdatedAt:   now,
		}

		return s.contests.SavePhoto(txCtx, saved)
	})
	if err != nil {
		return Photo{}, err
	}

	return saved, nil
}

// ListPhotos retrieves all photos owned by a character.
func (s *Service) ListPhotos(ctx context.Context, characterID string) ([]Photo, error) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil, ErrCharacterNotFound
	}
	_, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return nil, ErrCharacterNotFound
		}
		return nil, err
	}
	return s.contests.ListPhotosByCharacterID(ctx, characterID)
}

// DeletePhoto removes a specific photo owned by a character.
func (s *Service) DeletePhoto(ctx context.Context, characterID, photoID string) error {
	characterID = strings.TrimSpace(characterID)
	photoID = strings.TrimSpace(photoID)
	if characterID == "" {
		return ErrCharacterNotFound
	}
	if photoID == "" {
		return ErrPhotoNotFound
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		photo, err := s.contests.FindPhotoByID(txCtx, photoID)
		if err != nil {
			return err
		}
		if photo.CharacterID != characterID {
			return ErrForbidden
		}
		return s.contests.DeletePhoto(txCtx, photoID)
	})
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
		prepRound, err := s.contests.GetPreparingRoundForUpdate(txCtx)
		if err != nil {
			if errors.Is(err, ErrContestNotFound) {
				// Initialize first round if none exists
				now := s.nowFunc()
				prepRound = ContestRound{
					Round:     1,
					Status:    StatusPreparing,
					StartTime: now,
					EndTime:   now.Add(ContestCycleDays * 24 * time.Hour),
					CreatedAt: now,
					UpdatedAt: now,
				}
				if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
					return err
				}
			} else {
				return err
			}
		}

		// Check if character already entered this preparing round
		if _, err := s.contests.FindEntryByRoundAndCharacter(txCtx, prepRound.Round, characterID); err == nil {
			return ErrAlreadyEntered
		}

		// Check if title already taken in this preparing round
		if _, err := s.contests.FindEntryByRoundAndTitle(txCtx, prepRound.Round, title); err == nil {
			return ErrDuplicateTitle
		}

		now := s.nowFunc()
		entry = ContestEntry{
			ID:            id.New(),
			Round:         prepRound.Round,
			CharacterID:   char.ID,
			CharacterName: char.Name,
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
		if entry.CharacterID == voterCharacterID {
			return ErrSelfVoteDisallowed
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
func (s *Service) GetLegends(ctx context.Context, limit, offset int) ([]ContestLegend, error) {
	if limit <= 0 {
		limit = 20
	}
	if offset < 0 {
		offset = 0
	}
	return s.contests.ListLegends(ctx, limit, offset)
}

// SettleContest concludes the active contest round, awards prizes to winners and voters,
// records the 1st-place entry to Hall of Fame, and rolls over to the next contest round.
func (s *Service) SettleContest(ctx context.Context, force bool) (SettlementResult, error) {
	var result SettlementResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		activeRound, err := s.contests.GetActiveRoundForUpdate(txCtx)
		if err != nil {
			return err
		}

		now := s.nowFunc()
		if !force && now.Before(activeRound.EndTime) {
			return ErrContestNotReadyToSettle
		}

		entries, err := s.contests.ListEntriesByRound(txCtx, activeRound.Round)
		if err != nil {
			return err
		}

		result.Round = activeRound.Round

		// If minimum entries not reached, postpone and extend duration
		if len(entries) < MinEntriesForContest {
			activeRound.EndTime = activeRound.EndTime.Add(ContestCycleDays * 24 * time.Hour)
			activeRound.UpdatedAt = now
			if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
				return err
			}

			result.Postponed = true
			result.ExtendedUntil = activeRound.EndTime
			result.Message = "Contest postponed and extended due to insufficient entries"
			return nil
		}

		// Sort entries descending by Votes, then ascending by CreatedAt
		sort.SliceStable(entries, func(i, j int) bool {
			if entries[i].Votes != entries[j].Votes {
				return entries[i].Votes > entries[j].Votes
			}
			return entries[i].CreatedAt.Before(entries[j].CreatedAt)
		})

		// Assign ranks and distribute prizes to top 3
		prizes := []Prize{PrizeFirst, PrizeSecond, PrizeThird}
		for i := range entries {
			rank := i + 1
			entries[i].Ranking = rank
			if err := s.contests.SaveEntry(txCtx, entries[i]); err != nil {
				return err
			}

			if i < len(prizes) {
				prize := prizes[i]
				char, err := s.characters.FindByIDForUpdate(txCtx, entries[i].CharacterID)
				if err == nil {
					char.Money += prize.Gold
					char.SmallMedals += prize.SmallMedals
					_ = s.characters.Update(txCtx, char)
				}

				if s.guilds != nil {
					_ = s.guilds.AddGuildExp(txCtx, entries[i].CharacterID, int64(prize.GuildPoints))
				}

				if s.news != nil {
					newsContent := fmt.Sprintf("★第%d回フォトコンテスト%d位 %s★", activeRound.Round, rank, entries[i].CharacterName)
					_ = s.news.PublishNews(txCtx, "contest", "フォトコンテスト結果発表", newsContent, "ワコール", now)
				}
			}
		}

		// Archive 1st place winner to Hall of Fame / Legend
		if len(entries) > 0 {
			winner := entries[0]
			legend := ContestLegend{
				Round:         activeRound.Round,
				EntryID:       winner.ID,
				Title:         winner.Title,
				CharacterID:   winner.CharacterID,
				CharacterName: winner.CharacterName,
				GuildName:     winner.GuildName,
				Votes:         winner.Votes,
				ImageURL:      winner.ImageURL,
				Caption:       winner.Caption,
				SettledAt:     now,
			}
			if err := s.contests.SaveLegend(txCtx, legend); err != nil {
				return err
			}
			result.WinnerLegend = &legend

			// Distribute voter bonus medals to characters who voted for 1st place
			votes, err := s.contests.ListVotesByEntryID(txCtx, winner.ID)
			if err == nil {
				voterCount := 0
				for _, v := range votes {
					voterChar, err := s.characters.FindByIDForUpdate(txCtx, v.VoterCharacterID)
					if err == nil {
						voterChar.SmallMedals += VoterBonusSmallMedals
						if err := s.characters.Update(txCtx, voterChar); err == nil {
							voterCount++
						}
					}
				}
				result.VotersRewarded = voterCount
			}
		}

		// Mark active round as settled
		activeRound.Status = StatusSettled
		activeRound.UpdatedAt = now
		if err := s.contests.SaveRound(txCtx, activeRound); err != nil {
			return err
		}

		// Promote preparing round to active round and create next preparing round
		prepRound, err := s.contests.GetPreparingRoundForUpdate(txCtx)
		if err == nil {
			prepRound.Status = StatusActive
			prepRound.StartTime = now
			prepRound.EndTime = now.Add(ContestCycleDays * 24 * time.Hour)
			prepRound.UpdatedAt = now
			if err := s.contests.SaveRound(txCtx, prepRound); err != nil {
				return err
			}

			nextPrep := ContestRound{
				Round:     prepRound.Round + 1,
				Status:    StatusPreparing,
				StartTime: prepRound.EndTime,
				EndTime:   prepRound.EndTime.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextPrep); err != nil {
				return err
			}
		} else {
			// If no preparing round existed, create next active and preparing rounds
			nextActive := ContestRound{
				Round:     activeRound.Round + 1,
				Status:    StatusActive,
				StartTime: now,
				EndTime:   now.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextActive); err != nil {
				return err
			}

			nextPrep := ContestRound{
				Round:     activeRound.Round + 2,
				Status:    StatusPreparing,
				StartTime: nextActive.EndTime,
				EndTime:   nextActive.EndTime.Add(ContestCycleDays * 24 * time.Hour),
				CreatedAt: now,
				UpdatedAt: now,
			}
			if err := s.contests.SaveRound(txCtx, nextPrep); err != nil {
				return err
			}
		}

		result.Rankings = entries
		result.PrizesDistributed = true
		result.Message = fmt.Sprintf("Contest round %d successfully settled with %d entries", activeRound.Round, len(entries))
		return nil
	})
	if err != nil {
		return SettlementResult{}, err
	}

	return result, nil
}
