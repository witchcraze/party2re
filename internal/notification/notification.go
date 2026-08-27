package notification

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrEmptyTitle           = errors.New("title cannot be empty")
	ErrTitleTooLong         = errors.New("title exceeds maximum length of 200 characters")
	ErrEmptyContent         = errors.New("content cannot be empty")
	ErrContentTooLong       = errors.New("content exceeds maximum length of 10000 characters")
	ErrInvalidPlayerID      = errors.New("player ID cannot be empty")
	ErrEmptyBody            = errors.New("notification body cannot be empty")
	ErrBodyTooLong          = errors.New("notification body exceeds maximum length of 2000 characters")
	ErrNewsNotFound         = errors.New("news article not found")
	ErrNotificationNotFound = errors.New("notification not found")
	ErrForbidden            = errors.New("forbidden: notification belongs to another player")
)

const (
	MaxTitleLength   = 200
	MaxContentLength = 10000
	MaxBodyLength    = 2000
	MaxCategoryLen   = 50
	MaxAuthorLen     = 100
	MaxLinkLen       = 255
)

// Standard news categories
const (
	CategoryAnnouncement = "announcement"
	CategoryUpdate       = "update"
	CategoryMaintenance  = "maintenance"
	CategoryEvent        = "event"
	CategoryMilestone    = "milestone"
)

// Standard notification categories
const (
	NotificationCategorySystem    = "system"
	NotificationCategoryAuction   = "auction"
	NotificationCategoryGuild     = "guild"
	NotificationCategoryAdventure = "adventure"
	NotificationCategoryGift      = "gift"
	NotificationCategoryReward    = "reward"
	NotificationCategoryEvent     = "event"
)

// NewsArticle represents a system-wide news or announcement entry.
type NewsArticle struct {
	ID          string    `json:"id"`
	Category    string    `json:"category"`
	Title       string    `json:"title"`
	Content     string    `json:"content"`
	Author      string    `json:"author"`
	PublishedAt time.Time `json:"published_at"`
	CreatedAt   time.Time `json:"created_at"`
}

// PlayerNotification represents an individual player notification inbox item.
type PlayerNotification struct {
	ID        string     `json:"id"`
	PlayerID  string     `json:"player_id"`
	Category  string     `json:"category"`
	Title     string     `json:"title"`
	Body      string     `json:"body"`
	Link      string     `json:"link,omitempty"`
	IsRead    bool       `json:"is_read"`
	ReadAt    *time.Time `json:"read_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

// ValidateNewsArticle validates input parameters for a news article.
func ValidateNewsArticle(category, title, content, author string) error {
	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return ErrEmptyTitle
	}
	if utf8.RuneCountInString(cleanTitle) > MaxTitleLength {
		return ErrTitleTooLong
	}

	cleanContent := strings.TrimSpace(content)
	if cleanContent == "" {
		return ErrEmptyContent
	}
	if utf8.RuneCountInString(cleanContent) > MaxContentLength {
		return ErrContentTooLong
	}

	return nil
}

// ValidateNotification validates input parameters for a player notification.
func ValidateNotification(playerID, category, title, body string) error {
	if strings.TrimSpace(playerID) == "" {
		return ErrInvalidPlayerID
	}

	cleanTitle := strings.TrimSpace(title)
	if cleanTitle == "" {
		return ErrEmptyTitle
	}
	if utf8.RuneCountInString(cleanTitle) > MaxTitleLength {
		return ErrTitleTooLong
	}

	cleanBody := strings.TrimSpace(body)
	if cleanBody == "" {
		return ErrEmptyBody
	}
	if utf8.RuneCountInString(cleanBody) > MaxBodyLength {
		return ErrBodyTooLong
	}

	return nil
}
