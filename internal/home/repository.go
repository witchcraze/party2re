package home

import (
	"context"
	"time"
)

type Repository interface {
	GetHome(ctx context.Context, characterID string) (CharacterHome, error)
	SaveHome(ctx context.Context, home CharacterHome) error
	IncrementVisitorCount(ctx context.Context, characterID string, visitedAt time.Time) error

	CreateLetter(ctx context.Context, letter Letter) error
	GetLetterByID(ctx context.Context, id string) (Letter, error)
	ListInboxLetters(ctx context.Context, recipientID string, limit, offset int) ([]Letter, int, error)
	ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]Letter, int, error)
	GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error)
	MarkLetterAsRead(ctx context.Context, id, recipientID string, readAt time.Time) error
	DeleteLetter(ctx context.Context, id, characterID string) error

	AddCompanionPhrase(ctx context.Context, phrase CompanionPhrase) error
	DeleteCompanionPhrase(ctx context.Context, id, characterID string) error
	ListCompanionPhrases(ctx context.Context, characterID string) ([]CompanionPhrase, error)

	AddDeliveryNotice(ctx context.Context, notice DeliveryNotice) error
	ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]DeliveryNotice, error)
	ClearDeliveryNotices(ctx context.Context, characterID string) error
}
