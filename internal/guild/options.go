package guild

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type LetterSender interface {
	SendLetter(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error
}

type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// TransactionProvider executes functions within atomic database transactions.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type ServiceOption func(*Service)

func WithCharacterReader(cr CharacterReader) ServiceOption {
	return func(s *Service) {
		s.charReader = cr
	}
}

func WithLetterSender(ls LetterSender) ServiceOption {
	return func(s *Service) {
		s.letterSender = ls
	}
}

func WithNewsPublisher(np NewsPublisher) ServiceOption {
	return func(s *Service) {
		s.newsPub = np
	}
}

func WithTransactionProvider(tx TransactionProvider) ServiceOption {
	return func(s *Service) {
		s.txProvider = tx
	}
}

func WithClock(clock func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = clock
	}
}

func authorizeCurrentLeader(g Guild, members []Member, charID string) error {
	if g.LeaderCharacterID != charID {
		return ErrUnauthorized
	}
	for i := range members {
		if members[i].CharacterID == charID {
			if members[i].Role != RoleLeader || members[i].IsPending {
				return ErrUnauthorized
			}
			return nil
		}
	}
	return ErrUnauthorized
}
