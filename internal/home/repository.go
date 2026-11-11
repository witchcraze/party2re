package home

import (
	"context"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type EstateRepository interface {
	GetHome(ctx context.Context, characterID string) (CharacterHome, error)
	SaveHome(ctx context.Context, home CharacterHome) error
	UpdateCompanionName(ctx context.Context, characterID, companionName string, updatedAt time.Time) (CharacterHome, error)
	// LockTownHouseCapacityForUpdate serializes house construction in one town (Rank 0).
	LockTownHouseCapacityForUpdate(ctx context.Context, townID string) error
	CountActiveTownHouses(ctx context.Context, townID string, now time.Time) (int, error)
	FindActiveHomeByCharacterID(ctx context.Context, characterID string, now time.Time) (CharacterHome, error)
	FindActiveHomeByCharacterName(ctx context.Context, characterName string, now time.Time) (CharacterHome, corecharacter.Character, error)
	ListActiveTownHouses(ctx context.Context, townID string, now time.Time) ([]CharacterHome, error)
}

type LetterRepository interface {
	CreateLetter(ctx context.Context, letter Letter, logLimit int) error
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
	// AddCompanionPhrase stores the phrase and atomically keeps only the newest limit entries.
	AddCompanionPhrase(ctx context.Context, phrase CompanionPhrase, limit int) error
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

type DepotManager interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	Consume(ctx context.Context, characterID, itemInstanceID string, quantity int) error
	ConsumeOne(ctx context.Context, characterID, itemInstanceID string) error
	// AppendDeliveries locks the Depot and appends separate instances even above capacity.
	AppendDeliveries(ctx context.Context, characterID string, items []item.Instance) error
}
