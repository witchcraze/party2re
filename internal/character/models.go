package character

import (
	"context"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

const (
	NameChangeCost      = 500000          // 500,000 Gold
	GenderChangeCost    = 10000           // 10,000 Gold
	MaxCommentLength    = 160             // 160 runes
	MaxBioFieldLength   = 160             // 160 runes
	MaxBioKeyLength     = 32              // 32 runes
	MaxAvatarURLLength  = 512             // 512 chars
	MaxAvatarSizeBytes  = 2 * 1024 * 1024 // 2 MB
	MaxCharacterNameLen = 32              // 32 runes
)

var (
	ErrInvalidName            = corecharacter.ErrInvalidName
	ErrInsufficientGold       = errors.New("insufficient gold")
	ErrNameAlreadyTaken       = errors.New("character name is already taken")
	ErrInGuildDisallowed      = errors.New("cannot change name while belonging to a guild")
	ErrActiveMarketDisallowed = errors.New("cannot change name with active flea market listings")
	ErrSameName               = errors.New("new name is identical to current name")
	ErrSameGender             = errors.New("new gender is identical to current gender")
	ErrInvalidGender          = errors.New("invalid gender: must be m, f, other, or unspecified")
	ErrCommentTooLong         = errors.New("profile comment exceeds maximum length of 160 characters")
	ErrBioKeyTooLong          = errors.New("bio field key exceeds maximum length of 32 characters")
	ErrBioValueTooLong        = errors.New("bio field value exceeds maximum length of 160 characters")
	ErrInvalidAvatarURL       = errors.New("invalid avatar URL: must be http or https URL")
	ErrInvalidImageFormat     = errors.New("invalid image format: allowed formats are PNG, JPEG, GIF, WebP, SVG")
	ErrImageTooLarge          = errors.New("image size exceeds maximum allowed limit (2 MB)")
	ErrForbidden              = errors.New("forbidden: character does not belong to authenticated player")
)

// Prohibited character patterns in character names
var (
	prohibitedNameChars = regexp.MustCompile(`[,;\"\'&<>\\\/@＠]`)
)

// Profile represents player customization bio and avatar state.
type Profile struct {
	CharacterID string            `json:"character_id"`
	Comment     string            `json:"comment"`
	AvatarURL   string            `json:"avatar_url"`
	BioData     map[string]string `json:"bio_data,omitempty"`
	UpdatedAt   time.Time         `json:"updated_at"`
}

// ProfileView provides a combined view of character stats and profile data.
type ProfileView struct {
	Character corecharacter.Character `json:"character"`
	Profile   Profile                 `json:"profile"`
}

// UpdateProfileRequest defines parameters for updating a character's profile.
type UpdateProfileRequest struct {
	Comment   *string           `json:"comment,omitempty"`
	AvatarURL *string           `json:"avatar_url,omitempty"`
	BioData   map[string]string `json:"bio_data,omitempty"`
}

// NamingHallDialogue provides NPC dialogue and pricing information.
type NamingHallDialogue struct {
	NPCName          string   `json:"npc_name"`
	LocationTitle    string   `json:"location_title"`
	Phrases          []string `json:"phrases"`
	NameChangeCost   int      `json:"name_change_cost"`
	GenderChangeCost int      `json:"gender_change_cost"`
}

// ValidateName verifies character name constraints.
func ValidateName(name string) error {
	if !utf8.ValidString(name) {
		return ErrInvalidName
	}
	trimmed := strings.TrimSpace(name)
	runeCount := utf8.RuneCountInString(trimmed)
	if runeCount < 1 || runeCount > MaxCharacterNameLen {
		return ErrInvalidName
	}
	// Check prohibited punctuation/symbols
	if prohibitedNameChars.MatchString(trimmed) {
		return ErrInvalidName
	}
	// Check for any internal whitespace or Japanese fullwidth space
	for _, r := range trimmed {
		if unicode.IsSpace(r) || r == '\u3000' || unicode.IsControl(r) {
			return ErrInvalidName
		}
	}
	return nil
}

// ValidateGender checks that gender is among allowed values.
func ValidateGender(gender string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(gender))
	switch normalized {
	case "m", "male", "男":
		return "m", nil
	case "f", "female", "女":
		return "f", nil
	case "other", "unspecified", "その他":
		return "unspecified", nil
	default:
		return "", ErrInvalidGender
	}
}

// ValidateComment checks comment length.
func ValidateComment(comment string) error {
	trimmed := strings.TrimSpace(comment)
	if utf8.RuneCountInString(trimmed) > MaxCommentLength {
		return ErrCommentTooLong
	}
	return nil
}

// ValidateAvatarURL checks avatar URL format.
func ValidateAvatarURL(avatarURL string) error {
	trimmed := strings.TrimSpace(avatarURL)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "data:image/") {
		if len(trimmed) > 4*1024*1024 {
			return ErrImageTooLarge
		}
		if !strings.Contains(trimmed, ";base64,") {
			return ErrInvalidAvatarURL
		}
		return nil
	}
	if len(trimmed) > MaxAvatarURLLength {
		return ErrInvalidAvatarURL
	}
	if strings.HasPrefix(trimmed, "/") {
		return nil
	}
	parsed, err := url.Parse(trimmed)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return ErrInvalidAvatarURL
	}
	return nil
}

// ValidateBioData validates bio keys and values.
func ValidateBioData(bio map[string]string) error {
	if bio == nil {
		return nil
	}
	for k, v := range bio {
		kTrimmed := strings.TrimSpace(k)
		if utf8.RuneCountInString(kTrimmed) > MaxBioKeyLength || kTrimmed == "" {
			return ErrBioKeyTooLong
		}
		vTrimmed := strings.TrimSpace(v)
		if utf8.RuneCountInString(vTrimmed) > MaxBioFieldLength {
			return ErrBioValueTooLong
		}
	}
	return nil
}

// GuildMembershipChecker checks whether a character belongs to a guild.
type GuildMembershipChecker interface {
	IsInGuild(ctx context.Context, characterID string) (bool, error)
}

type GuildMembershipCheckerFunc func(ctx context.Context, characterID string) (bool, error)

func (f GuildMembershipCheckerFunc) IsInGuild(ctx context.Context, characterID string) (bool, error) {
	return f(ctx, characterID)
}

// FleaMarketChecker checks whether a character has active market listings.
type FleaMarketChecker interface {
	HasActiveListings(ctx context.Context, characterID string) (bool, error)
}

type FleaMarketCheckerFunc func(ctx context.Context, characterID string) (bool, error)

func (f FleaMarketCheckerFunc) HasActiveListings(ctx context.Context, characterID string) (bool, error) {
	return f(ctx, characterID)
}

// NewsPublisher defines announcement publication contract.
type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

// NewsPublisherFunc adapts a standalone function to NewsPublisher.
type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// ProfileRepository defines persistence operations for character profiles.
type ProfileRepository interface {
	GetProfile(ctx context.Context, characterID string) (Profile, error)
	SaveProfile(ctx context.Context, profile Profile) error
}

// ExtendedRepository includes name lookups and profile storage.
type ExtendedRepository interface {
	Repository
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	FindByName(ctx context.Context, name string) (corecharacter.Character, error)
	FindByNameForUpdate(ctx context.Context, name string) (corecharacter.Character, error)
	GetProfile(ctx context.Context, characterID string) (Profile, error)
	SaveProfile(ctx context.Context, profile Profile) error
}

// TransactionProvider executes work inside a transactional context.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}
