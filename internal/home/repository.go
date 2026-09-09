package home

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type EstateRepository interface {
	GetHome(ctx context.Context, characterID string) (CharacterHome, error)
	SaveHome(ctx context.Context, home CharacterHome) error
	CountActiveTownHouses(ctx context.Context, townID string, now time.Time) (int, error)
	FindActiveHomeByCharacterID(ctx context.Context, characterID string, now time.Time) (CharacterHome, error)
	FindActiveHomeByCharacterName(ctx context.Context, characterName string, now time.Time) (CharacterHome, corecharacter.Character, error)
	ListActiveTownHouses(ctx context.Context, townID string, now time.Time) ([]CharacterHome, error)
}

type LetterRepository interface {
	CreateLetter(ctx context.Context, letter Letter) error
	GetLetterByID(ctx context.Context, id string) (Letter, error)
	ListInboxLetters(ctx context.Context, recipientID string, limit, offset int) ([]Letter, int, error)
	ListInboxLettersByCursor(ctx context.Context, recipientID string, limit int, beforeTime time.Time, beforeID string) ([]Letter, error)
	ListOutboxLetters(ctx context.Context, senderID string, limit, offset int) ([]Letter, int, error)
	ListOutboxLettersByCursor(ctx context.Context, senderID string, limit int, beforeTime time.Time, beforeID string) ([]Letter, error)
	GetUnreadLetterCount(ctx context.Context, recipientID string) (int, error)
	MarkLetterAsRead(ctx context.Context, id, recipientID string, readAt time.Time) error
	DeleteLetter(ctx context.Context, id, characterID string) error
}

type PhraseRepository interface {
	AddCompanionPhrase(ctx context.Context, phrase CompanionPhrase) error
	DeleteCompanionPhrase(ctx context.Context, id, characterID string) error
	ListCompanionPhrases(ctx context.Context, characterID string) ([]CompanionPhrase, error)
}

type DeliveryNoticeRepository interface {
	AddDeliveryNotice(ctx context.Context, notice DeliveryNotice) error
	ListDeliveryNotices(ctx context.Context, characterID string, unclearedOnly bool) ([]DeliveryNotice, error)
	ClearDeliveryNotices(ctx context.Context, characterID string) error
}

type Repository interface {
	EstateRepository
	LetterRepository
	PhraseRepository
	DeliveryNoticeRepository
}
