package home

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
)

var (
	ErrInvalidSender        = errors.New("sender character ID cannot be empty")
	ErrInvalidRecipient     = errors.New("recipient character ID cannot be empty")
	ErrCannotSendToSelf     = errors.New("cannot send a letter to yourself")
	ErrEmptyContent         = errors.New("letter content cannot be empty")
	ErrContentTooLong       = errors.New("letter content exceeds maximum length of 1000 characters")
	ErrEmptyPhrase          = errors.New("phrase cannot be empty")
	ErrPhraseTooLong        = errors.New("phrase exceeds maximum length of 120 characters")
	ErrMaxPhrasesReached    = errors.New("companion has reached the maximum phrase capacity of 30")
	ErrLetterNotFound       = errors.New("letter not found")
	ErrPhraseNotFound       = errors.New("companion phrase not found")
	ErrForbidden            = errors.New("forbidden: letter or home belongs to another character")
	ErrCharacterNotFound    = errors.New("character not found")
	ErrAlreadyOwnsHouse     = errors.New("character already owns a house")
	ErrTownMaxHousesReached = errors.New("town has reached maximum house capacity")
	ErrInvalidTownID        = errors.New("invalid town ID")
	ErrInvalidHouseStyle    = errors.New("invalid house style for town")
	ErrHouseNotFound        = errors.New("house not found")
	ErrHouseExpired         = errors.New("house has expired")
	ErrInsufficientFunds    = errors.New("insufficient funds to build house")
	ErrItemNotFound         = errors.New("item not found")
	ErrCannotUseHere        = errors.New("cannot use item here")
)

const (
	DefaultCompanionName   = "ペット"
	MaxLetterContentLength = 1000
	MaxPhraseLength        = 120
	MaxPhrasesPerCompanion = 30
	MaxCompanionNameLength = 64
	MaxColorLength         = 7
)

// CharacterHome represents private home estate settings for a character.
type CharacterHome struct {
	CharacterID   string     `json:"character_id"`
	TownID        string     `json:"town_id"`
	HouseStyle    string     `json:"house_style"`
	ExpiresAt     *time.Time `json:"expires_at,omitempty"`
	CompanionName string     `json:"companion_name"`
	BgImg         string     `json:"bgimg,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

// IsActive returns whether the house is currently active and unexpired.
func (h CharacterHome) IsActive(now time.Time) bool {
	return h.TownID != "" && h.ExpiresAt != nil && h.ExpiresAt.After(now)
}

// Letter represents a player-to-player message.
type Letter struct {
	ID                   string     `json:"id"`
	SenderCharacterID    string     `json:"sender_character_id"`
	SenderName           string     `json:"sender_name"`
	RecipientCharacterID string     `json:"recipient_character_id"`
	RecipientName        string     `json:"recipient_name"`
	Content              string     `json:"content"`
	Color                string     `json:"color"`
	IsRead               bool       `json:"is_read"`
	ReadAt               *time.Time `json:"read_at,omitempty"`
	IsDeletedBySender    bool       `json:"is_deleted_by_sender"`
	IsDeletedByRecipient bool       `json:"is_deleted_by_recipient"`
	CreatedAt            time.Time  `json:"created_at"`
}

// CompanionPhrase represents a taught greeting phrase for the home companion.
type CompanionPhrase struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	Phrase      string    `json:"phrase"`
	CreatedAt   time.Time `json:"created_at"`
}

// DeliveryNotice represents an asynchronous item or gold delivery record.
type DeliveryNotice struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	NoticeType  string    `json:"notice_type"`
	Message     string    `json:"message"`
	IsCleared   bool      `json:"is_cleared"`
	CreatedAt   time.Time `json:"created_at"`
}

// HomeView represents the full aggregated presentation view for a character's home.
type HomeView struct {
	Owner                corecharacter.Character `json:"owner"`
	Home                 CharacterHome           `json:"home"`
	UnreadLetterCount    int                     `json:"unread_letter_count"`
	CompanionPhraseCount int                     `json:"companion_phrase_count"`
	RecentDeliveryCount  int                     `json:"recent_delivery_count"`
	IsOwner              bool                    `json:"is_owner"`
}

// HomeCheckResult represents the response for the legacy "chekku" action.
type HomeCheckResult struct {
	CharacterID string    `json:"character_id"`
	OwnerName   string    `json:"owner_name"`
	TownID      string    `json:"town_id"`
	TownName    string    `json:"town_name"`
	HouseStyle  string    `json:"house_style"`
	ExpiresAt   time.Time `json:"expires_at"`
	Message     string    `json:"message"`
}

// HomeUsableItem represents an item inspected or usable from home (equipment or depot).
type HomeUsableItem struct {
	InstanceID   string    `json:"instance_id"`
	DefinitionID string    `json:"definition_id"`
	Name         string    `json:"name"`
	Kind         int       `json:"kind"` // 1: weapon, 2: armor/shield/acc, 3: consumable
	Slot         item.Slot `json:"slot,omitempty"`
	Attack       int       `json:"attack,omitempty"`
	Defense      int       `json:"defense,omitempty"`
	Weight       int       `json:"weight,omitempty"`
	Price        int       `json:"price"`
	Quantity     int       `json:"quantity"`
	Source       string    `json:"source"` // "inventory", "depot"
}

// UseHomeItemResult represents the outcome of using or inspecting an item from home.
type UseHomeItemResult struct {
	Action    string                   `json:"action"` // "inspect" or "consumed"
	Message   string                   `json:"message"`
	ItemName  string                   `json:"item_name"`
	Kind      int                      `json:"kind"`
	Consumed  bool                     `json:"consumed"`
	Character *corecharacter.Character `json:"character,omitempty"`
}

// ValidateLetter validates sender, recipient, and message content for a letter.
func ValidateLetter(senderID, recipientID, content string) error {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return ErrInvalidSender
	}
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return ErrInvalidRecipient
	}
	if senderID == recipientID {
		return ErrCannotSendToSelf
	}

	cleanContent := strings.TrimSpace(content)
	if cleanContent == "" {
		return ErrEmptyContent
	}
	if utf8.RuneCountInString(cleanContent) > MaxLetterContentLength {
		return ErrContentTooLong
	}

	return nil
}

// ValidatePhrase validates companion phrase text.
func ValidatePhrase(phrase string) error {
	clean := strings.TrimSpace(phrase)
	if clean == "" {
		return ErrEmptyPhrase
	}
	if utf8.RuneCountInString(clean) > MaxPhraseLength {
		return ErrPhraseTooLong
	}
	return nil
}
