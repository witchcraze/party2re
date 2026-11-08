package exile

import (
	"context"
	"errors"
	"fmt"
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
}

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByName(ctx context.Context, name string) (corecharacter.Character, error)
}

type SleepTimer interface {
	SetLock(ctx context.Context, category, targetID string, duration time.Duration) error
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
	p := Petition{
		ID: id.New(), ApplicantID: actor.ID, ApplicantName: actor.Name, TargetID: target.ID, TargetName: target.Name,
		Reason: reason, CreatedAt: now, Votes: []Vote{{VoterID: actor.ID, VoterName: actor.Name, Approve: true, VotedAt: now}},
	}
	err = s.tx.RunInTx(ctx, func(ctx context.Context) error {
		if _, err := s.repo.LockBoard(ctx); err != nil {
			return err
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
		return s.publish(ctx, fmt.Sprintf("%sが%sを追放申請しました", actor.Name, target.Name), "理由: "+reason, now)
	})
	if err != nil {
		return Petition{}, err
	}
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
	return result, nil
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
		if r.Verdict == "" {
			if err := s.repo.AddVote(ctx, p.ID, p.Votes[0]); err != nil {
				return err
			}
		} else if err := s.resolve(ctx, *p, r, now); err != nil {
			return err
		}
	}
	return s.repo.TouchBoard(ctx, now)
}

// resolve removes the petition and overwrites the sentenced sleep (legacy
// regist_you_data). The timer is written last so its failure rolls back the vote.
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
	return s.timer.SetLock(ctx, timer.CategorySleep, sleeper, sleep)
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
