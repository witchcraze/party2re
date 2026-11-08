package exile

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/validation"
)

// Repository persists the single shared board.
type Repository interface {
	// LockBoard serializes board mutations and returns the last board write time.
	LockBoard(ctx context.Context) (time.Time, error)
	TouchBoard(ctx context.Context, at time.Time) error
	// ListPetitions returns petitions in submission order with votes newest first.
	ListPetitions(ctx context.Context) ([]Petition, error)
	CreatePetition(ctx context.Context, p Petition) error
	AddVote(ctx context.Context, petitionID string, v Vote) error
	DeletePetition(ctx context.Context, petitionID string) error
	IsInvolved(ctx context.Context, characterID string) (bool, error)
	SentenceStore
}

// SentenceStore keeps committed sleep deadlines until the timer confirms them.
type SentenceStore interface {
	SaveSentence(ctx context.Context, s Sentence) error
	ListSentences(ctx context.Context) ([]Sentence, error)
	DeleteSentence(ctx context.Context, s Sentence) error
}

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	FindByName(ctx context.Context, name string) (corecharacter.Character, error)
}

type SleepTimer interface {
	SetLockUntil(ctx context.Context, category, targetID string, until time.Time) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

const newsAuthor = "@追放騎士"

type Service struct {
	repo       Repository
	characters CharacterReader
	timer      SleepTimer
	tx         TransactionProvider
	news       NewsPublisher
}

func NewService(repo Repository, characters CharacterReader, sleep SleepTimer, tx TransactionProvider, news NewsPublisher) *Service {
	return &Service{repo: repo, characters: characters, timer: sleep, tx: tx, news: news}
}

// IsInvolved reports whether the character applied, is targeted or voted on an open petition.
func (s *Service) IsInvolved(ctx context.Context, characterID string) (bool, error) {
	return s.repo.IsInvolved(ctx, characterID)
}

// GetBoard shows open petitions only to eligible actors, as the legacy list required voting rights.
func (s *Service) GetBoard(ctx context.Context, actorID string) (Board, error) {
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return Board{}, err
	}
	if actor.JobLevel < MinJobLevel {
		return Board{Petitions: []Petition{}}, nil
	}
	petitions, err := s.repo.ListPetitions(ctx)
	if err != nil {
		return Board{}, err
	}
	return Board{Eligible: true, Petitions: petitions}, nil
}

// Petition files a petition and records the applicant's approval.
func (s *Service) Petition(ctx context.Context, actorID, targetName, reason string, now time.Time) (Petition, error) {
	if targetName == "" || strings.TrimSpace(reason) == "" {
		return Petition{}, ErrInvalidTarget
	}
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return Petition{}, err
	}
	if actor.JobLevel < MinJobLevel {
		return Petition{}, ErrJobLevelTooLow
	}
	if targetName == actor.Name {
		return Petition{}, ErrSelfPetition
	}
	reason, err = validation.ValidateLegacyComment(reason, MaxCommandLength-petitionSyntaxLen-utf8.RuneCountInString(targetName))
	if err != nil {
		return Petition{}, fmt.Errorf("%w: %w", ErrInvalidReason, err)
	}
	target, err := s.characters.FindByName(ctx, targetName)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return Petition{}, ErrTargetNotFound
		}
		return Petition{}, err
	}
	var p Petition
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.LockBoard(ctx); err != nil {
			return err
		}
		locked, err := s.lockCharacters(ctx, actor.ID, target.ID)
		if err != nil {
			return err
		}
		applicant, target := locked[actor.ID], locked[target.ID]
		if target.Name != targetName {
			return ErrTargetNotFound // renamed after the lookup
		}
		p = Petition{
			ID: id.New(), ApplicantID: applicant.ID, ApplicantName: applicant.Name, TargetID: target.ID, TargetName: target.Name,
			Reason: reason, CreatedAt: now, Votes: []Vote{{VoterID: applicant.ID, VoterName: applicant.Name, Approve: true, VotedAt: now}},
		}
		open, err := s.repo.ListPetitions(ctx)
		if err != nil {
			return err
		}
		// Legacy scans rows in order: a duplicate target or any applicant's
		// counted row passing the limit refuses the new petition.
		counts := map[string]int{}
		for _, o := range open {
			if o.TargetID == target.ID {
				return ErrAlreadyPetitioned
			}
			counts[o.ApplicantID]++
			if counts[o.ApplicantID] > MaxOpenPetitions {
				return ErrPetitionLimit
			}
		}
		if err := s.repo.CreatePetition(ctx, p); err != nil {
			return err
		}
		if err := s.repo.TouchBoard(ctx, now); err != nil {
			return err
		}
		return s.publish(ctx, fmt.Sprintf("%sが%sを追放申請しました", applicant.Name, target.Name), "理由: "+reason, now)
	})
	if err != nil {
		return Petition{}, err
	}
	s.projectSentences(ctx, now)
	return p, nil
}

// Vote applies an approval or opposition to the petition naming targetName.
// An applicant's opposition first withdraws all of that applicant's petitions,
// refused while the board was written within CancelLockWindow.
func (s *Service) Vote(ctx context.Context, actorID, targetName string, approve bool, now time.Time) (VoteResult, error) {
	if targetName == "" {
		return VoteResult{}, ErrInvalidTarget
	}
	actor, err := s.actor(ctx, actorID)
	if err != nil {
		return VoteResult{}, err
	}
	if actor.JobLevel < MinJobLevel {
		return VoteResult{}, ErrJobLevelTooLow
	}
	var result VoteResult
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		result = VoteResult{Withdrawn: []Petition{}}
		updatedAt, err := s.repo.LockBoard(ctx)
		if err != nil {
			return err
		}
		locked, err := s.lockCharacters(ctx, actor.ID)
		if err != nil {
			return err
		}
		actor := locked[actor.ID]
		open, err := s.repo.ListPetitions(ctx)
		if err != nil {
			return err
		}
		for _, p := range open {
			if p.ApplicantID == actor.ID && !approve {
				if updatedAt.Add(CancelLockWindow).After(now) {
					return ErrCancelLocked
				}
				result.Withdrawn = append(result.Withdrawn, p)
				continue
			}
			if p.TargetName != targetName || result.Petition != nil {
				continue
			}
			if p.hasVoter(actor.ID) {
				return ErrAlreadyVoted
			}
			if p.TargetID == actor.ID {
				return ErrTargetCannotVote
			}
			p.Votes = append([]Vote{{VoterID: actor.ID, VoterName: actor.Name, Approve: approve, VotedAt: now}}, p.Votes...)
			result.Petition = &p
			result.Approvals, result.Oppositions = p.Tally()
			switch {
			case approve && result.Approvals >= RequiredVotes:
				result.Verdict = VerdictGuilty
			case !approve && result.Oppositions >= RequiredVotes:
				result.Verdict = VerdictAcquitted
			}
		}
		if len(result.Withdrawn) == 0 && result.Petition == nil {
			return ErrPetitionNotFound
		}
		return s.apply(ctx, actor, result, now)
	})
	if err != nil {
		return VoteResult{}, err
	}
	s.projectSentences(ctx, now)
	return result, nil
}

// projectSentences confirms committed deadlines in the Valkey sleep timer.
// SQL is authoritative: a failed or interrupted projection leaves the row,
// and the next board command projects the same absolute deadline again.
// Holding the board lock orders projections after every committed verdict,
// so an older deadline cannot overwrite a newer one.
func (s *Service) projectSentences(ctx context.Context, now time.Time) {
	//lint:ignore error-swallow unprojected rows stay and are retried by the next board command
	_ = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.LockBoard(ctx); err != nil {
			return err
		}
		sentences, err := s.repo.ListSentences(ctx)
		if err != nil {
			return err
		}
		for _, sentence := range sentences {
			if sentence.SleepUntil.After(now) {
				if err := s.timer.SetLockUntil(ctx, timer.CategorySleep, sentence.CharacterID, sentence.SleepUntil); err != nil {
					continue
				}
			}
			if err := s.repo.DeleteSentence(ctx, sentence); err != nil {
				return err
			}
		}
		return nil
	})
}

// lockCharacters locks rows in ascending ID order after the board, so renames
// (which lock the character, then check involvement) serialize with board writes.
func (s *Service) lockCharacters(ctx context.Context, ids ...string) (map[string]corecharacter.Character, error) {
	slices.Sort(ids)
	locked := make(map[string]corecharacter.Character, len(ids))
	for _, id := range ids {
		c, err := s.characters.FindByIDForUpdate(ctx, id)
		if err != nil {
			return nil, err
		}
		locked[id] = c
	}
	return locked, nil
}

func (s *Service) apply(ctx context.Context, actor corecharacter.Character, r VoteResult, now time.Time) error {
	for _, p := range r.Withdrawn {
		if err := s.repo.DeletePetition(ctx, p.ID); err != nil {
			return err
		}
		if err := s.publish(ctx, fmt.Sprintf("%sが%sの追放申請を取り消しました", actor.Name, p.TargetName), "追放申請は取り消されました。", now); err != nil {
			return err
		}
	}
	if p := r.Petition; p != nil {
		var err error
		if r.Verdict == "" {
			err = s.repo.AddVote(ctx, p.ID, p.Votes[0])
		} else {
			err = s.resolve(ctx, *p, r, now)
		}
		if err != nil {
			return err
		}
	}
	return s.repo.TouchBoard(ctx, now)
}

// resolve removes the petition and records the sleep deadline that replaces
// the sentenced character's remaining sleep (legacy regist_you_data).
func (s *Service) resolve(ctx context.Context, p Petition, r VoteResult, now time.Time) error {
	if err := s.repo.DeletePetition(ctx, p.ID); err != nil {
		return err
	}
	tally := fmt.Sprintf("賛成 %d 票 / 反対 %d 票", r.Approvals, r.Oppositions)
	sleeper, sleep := p.TargetID, GuiltySleep
	title := fmt.Sprintf("【議決】%sは有罪として追放されました", p.TargetName)
	content := tally + "。"
	if r.Verdict == VerdictAcquitted {
		title = fmt.Sprintf("【議決】%sは無罪となりました", p.TargetName)
		sleeper, sleep = p.ApplicantID, AcquittalSleep
		if _, err := s.characters.FindByID(ctx, sleeper); errors.Is(err, corecharacter.ErrNotFound) {
			sleeper = ""
		} else if err != nil {
			return err
		} else {
			content += fmt.Sprintf("申請者の%sは5日間の眠りの刑となりました。", p.ApplicantName)
		}
	}
	if err := s.publish(ctx, title, content, now); err != nil {
		return err
	}
	if sleeper == "" {
		return nil
	}
	return s.repo.SaveSentence(ctx, Sentence{CharacterID: sleeper, SleepUntil: now.Add(sleep)})
}

func (s *Service) publish(ctx context.Context, title, content string, now time.Time) error {
	if s.news == nil {
		return nil
	}
	return s.news.PublishNews(ctx, "exile", title, content, newsAuthor, now)
}

func (s *Service) actor(ctx context.Context, actorID string) (corecharacter.Character, error) {
	if strings.TrimSpace(actorID) == "" {
		return corecharacter.Character{}, ErrInvalidCharacterID
	}
	return s.characters.FindByID(ctx, actorID)
}
