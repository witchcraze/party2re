package home

import (
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/validation"
)

var (
	ErrInvalidSender             = errors.New("sender character ID cannot be empty")
	ErrInvalidRecipient          = errors.New("recipient character ID cannot be empty")
	ErrEmptyContent              = errors.New("letter content cannot be empty")
	ErrContentTooLong            = errors.New("letter content exceeds the maximum length")
	ErrLetterThrottled           = errors.New("letters cannot be sent consecutively; please wait and try again")
	ErrEmptyPhrase               = errors.New("phrase cannot be empty")
	ErrPhraseTooLong             = errors.New("phrase exceeds maximum length of 120 characters")
	ErrLetterNotFound            = errors.New("letter not found")
	ErrPhraseNotFound            = errors.New("companion phrase not found")
	ErrForbidden                 = errors.New("forbidden: letter or home belongs to another character")
	ErrCharacterNotFound         = errors.New("character not found")
	ErrAlreadyOwnsHouse          = errors.New("character already owns a house")
	ErrTownMaxHousesReached      = errors.New("town has reached maximum house capacity")
	ErrInvalidTownID             = errors.New("invalid town ID")
	ErrInvalidHouseStyle         = errors.New("invalid house style for town")
	ErrHouseNotFound             = errors.New("house not found")
	ErrHouseExpired              = errors.New("house has expired")
	ErrInsufficientFunds         = errors.New("insufficient funds to build house")
	ErrTransactionRunnerRequired = errors.New("transaction runner is required")
	ErrItemNotFound              = errors.New("item not found")
	ErrCannotUseHere             = errors.New("cannot use item here")
	ErrNoPetsAtHome              = errors.New("no pets at home")
)

// HomePet represents a resident monster pet staying at character home.
type HomePet struct {
	ID         string `json:"id"`
	CustomName string `json:"custom_name"`
	MonsterID  string `json:"monster_id,omitempty"`
}

// DisplayName returns the display name of the resident pet.
func (p HomePet) DisplayName() string {
	if strings.TrimSpace(p.CustomName) != "" {
		return strings.TrimSpace(p.CustomName)
	}
	if strings.TrimSpace(p.MonsterID) != "" {
		return strings.TrimSpace(p.MonsterID)
	}
	return DefaultCompanionName
}

// CompanionTalkResult represents the chosen phrase and speaker pet name.
type CompanionTalkResult struct {
	PetName string `json:"pet_name"`
	Phrase  string `json:"phrase,omitempty"`
}

const (
	DefaultCompanionName   = "ペット"
	MaxPhraseLength        = 120
	MaxCompanionNameLength = 64
	// DefaultCompanionPhraseLimit is the number of phrases kept per character.
	DefaultCompanionPhraseLimit = 30
	// DefaultLetterMaxLength is the maximum letter body length, counted after legacy HTML escaping.
	DefaultLetterMaxLength = 500
	// DefaultLetterLogLimit is the number of letters kept in each inbox and outbox.
	DefaultLetterLogLimit = 30
	// LetterThrottleSeconds is the wait before a sender can follow their own latest inbox letter.
	LetterThrottleSeconds = 30
	MaxColorLength        = 7
)

// IsLetterThrottled reports whether a sender must wait, given the latest letter in the recipient's inbox.
// Times compare in whole Unix seconds, so a letter is allowed exactly LetterThrottleSeconds later.
func IsLetterThrottled(senderID, latestSenderID string, latestAt, now time.Time) bool {
	return latestSenderID == senderID && latestAt.Unix()+LetterThrottleSeconds > now.Unix()
}

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

// HomeOwner is the public identity whitelist; never expose the raw Character.
type HomeOwner struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Color string `json:"color"`
}

type OwnedHomeDetails struct {
	UnreadLetterCount    int `json:"unread_letter_count"`
	CompanionPhraseCount int `json:"companion_phrase_count"`
	RecentDeliveryCount  int `json:"recent_delivery_count"`
}

// HomeView separates public facts from authenticated owner-only enrichment.
type HomeView struct {
	Owner        HomeOwner         `json:"owner"`
	Home         CharacterHome     `json:"home"`
	Private      *OwnedHomeDetails `json:"private,omitempty"`
	IsOwner      bool              `json:"is_owner"`
	ResidentPets []HomePet         `json:"resident_pets,omitempty"`
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
	// AppearanceNotApplied reports a committed use whose appearance could not be stored.
	AppearanceNotApplied bool `json:"appearance_not_applied,omitempty"`
}

// ValidateLetter validates sender, recipient, and message content for a letter, returning the normalized content.
func ValidateLetter(senderID, recipientID, content string, maxLength int) (string, error) {
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return "", ErrInvalidSender
	}
	recipientID = strings.TrimSpace(recipientID)
	if recipientID == "" {
		return "", ErrInvalidRecipient
	}

	normalized, err := validation.ValidateLegacyComment(content, maxLength)
	if err != nil {
		if errors.Is(err, validation.ErrEmpty) {
			return "", ErrEmptyContent
		}
		if errors.Is(err, validation.ErrTooLong) {
			return "", ErrContentTooLong
		}
		return "", err
	}

	return normalized, nil
}

// ValidatePhrase validates companion phrase text.
func ValidatePhrase(phrase string) error {
	if _, err := validation.ValidateSingleLine(phrase, MaxPhraseLength); err != nil {
		if errors.Is(err, validation.ErrEmpty) {
			return ErrEmptyPhrase
		}
		if errors.Is(err, validation.ErrTooLong) {
			return ErrPhraseTooLong
		}
		return err
	}
	return nil
}
