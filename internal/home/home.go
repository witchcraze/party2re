package home

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidSender     = errors.New("sender character ID cannot be empty")
	ErrInvalidRecipient  = errors.New("recipient character ID cannot be empty")
	ErrCannotSendToSelf  = errors.New("cannot send a letter to yourself")
	ErrEmptyContent      = errors.New("letter content cannot be empty")
	ErrContentTooLong    = errors.New("letter content exceeds maximum length of 1000 characters")
	ErrEmptyPhrase       = errors.New("phrase cannot be empty")
	ErrPhraseTooLong     = errors.New("phrase exceeds maximum length of 200 characters")
	ErrMaxPhrasesReached = errors.New("companion has reached the maximum phrase capacity of 10")
	ErrLetterNotFound    = errors.New("letter not found")
	ErrPhraseNotFound    = errors.New("companion phrase not found")
	ErrForbidden         = errors.New("forbidden: letter or home belongs to another character")
	ErrCharacterNotFound = errors.New("character not found")
)

const (
	DefaultTheme           = "#ffffff"
	DefaultCompanionName   = "ペット"
	MaxLetterContentLength = 1000
	MaxPhraseLength        = 200
	MaxPhrasesPerCompanion = 10
	MaxColorLength         = 16
	MaxCompanionNameLength = 64
	MaxMottoLength         = 255
)

// CharacterHome represents private home estate settings for a character.
type CharacterHome struct {
	CharacterID   string     `json:"character_id"`
	Theme         string     `json:"theme"`
	Motto         string     `json:"motto"`
	CompanionName string     `json:"companion_name"`
	VisitorCount  int        `json:"visitor_count"`
	LastVisitedAt *time.Time `json:"last_visited_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
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
