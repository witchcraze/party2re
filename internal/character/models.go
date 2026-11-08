package character

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/validation"
)

const (
	NameChangeCost    = 500000 // 500,000 Gold
	GenderChangeCost  = 10000  // 10,000 Gold
	MaxCommentLength  = 160    // 160 runes
	MaxBioFieldLength = 160    // 160 runes
	MaxBioKeyLength   = 32     // 32 runes
	// PersonalityBioKey is the legacy profile key for 性格・特徴 (lib/profile.cgi:18).
	PersonalityBioKey = "character"
)

var (
	ErrInvalidName               = corecharacter.ErrInvalidName
	ErrInsufficientGold          = errors.New("insufficient gold")
	ErrNameAlreadyTaken          = errors.New("character name is already taken")
	ErrInGuildDisallowed         = errors.New("cannot change name while belonging to a guild")
	ErrActiveMarketDisallowed    = errors.New("cannot change name with active flea market listings")
	ErrExileInvolvedDisallowed   = errors.New("cannot change name while involved in an exile vote")
	ErrSameName                  = errors.New("new name is identical to current name")
	ErrSameGender                = errors.New("new gender is identical to current gender")
	ErrInvalidGender             = errors.New("invalid gender: must be m or f")
	ErrGenderIncompatibleWithJob = errors.New("gender is incompatible with current job")
	ErrCommentTooLong            = errors.New("profile comment exceeds maximum length of 160 characters")
	ErrBioKeyTooLong             = errors.New("bio field key exceeds maximum length of 32 characters")
	ErrBioValueTooLong           = errors.New("bio field value exceeds maximum length of 160 characters")
	ErrInvalidAvatarURL          = errors.New("invalid avatar URL: must be https://i.imgur.com/<5-10 alphanumerics>.gif")
	ErrForbidden                 = errors.New("forbidden: character does not belong to authenticated player")
	ErrInvalidAuraEffect         = errors.New("invalid aura effect: must be between 0 and 8")
)

// Profile represents player customization bio and avatar state.
type Profile struct {
	CharacterID string            `json:"character_id"`
	Comment     string            `json:"comment"`
	AvatarURL   string            `json:"avatar_url"`
	AuraEffect  int               `json:"aura_effect"`
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
	Comment    *string           `json:"comment,omitempty"`
	AvatarURL  *string           `json:"avatar_url,omitempty"`
	AuraEffect *int              `json:"aura_effect,omitempty"`
	BioData    map[string]string `json:"bio_data,omitempty"`
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
	if _, err := validation.ValidateCharacterName(name); err != nil {
		return ErrInvalidName
	}
	return nil
}

// ValidateAuraEffect verifies that aura effect integer is between 0 and 8.
func ValidateAuraEffect(effect int) error {
	if effect < 0 || effect > 8 {
		return ErrInvalidAuraEffect
	}
	return nil
}

// ValidateGender checks that gender is among allowed values ('m' or 'f').
func ValidateGender(gender string) (string, error) {
	val, err := validation.ValidateGender(gender)
	if err != nil {
		return "", ErrInvalidGender
	}
	return val, nil
}

// ValidateComment checks comment length and contents.
func ValidateComment(comment string) error {
	trimmed := strings.TrimSpace(comment)
	if trimmed == "" {
		return nil
	}
	if _, err := validation.ValidateMultiLine(trimmed, MaxCommentLength); err != nil {
		if errors.Is(err, validation.ErrTooLong) {
			return ErrCommentTooLong
		}
		return err
	}
	return nil
}

// avatarURLPattern is the only non-empty avatar: legacy stores an imgur image ID
// (5–10 alphanumerics) as this URL (custom_image.cgi:65–70).
var avatarURLPattern = regexp.MustCompile(`^https://i\.imgur\.com/[A-Za-z0-9]{5,10}\.gif$`)

// ValidateAvatarURL accepts "" (no avatar) or the canonical imgur avatar URL, exactly.
func ValidateAvatarURL(avatarURL string) error {
	if avatarURL == "" || avatarURLPattern.MatchString(avatarURL) {
		return nil
	}
	return ErrInvalidAvatarURL
}

// ValidateBioData validates bio keys and values.
func ValidateBioData(bio map[string]string) error {
	if bio == nil {
		return nil
	}
	for k, v := range bio {
		if utf8.RuneCountInString(k) > MaxBioKeyLength {
			return ErrBioKeyTooLong
		}
		kTrimmed := strings.TrimSpace(k)
		if _, err := validation.ValidateSingleLine(kTrimmed, MaxBioKeyLength); err != nil {
			if errors.Is(err, validation.ErrTooLong) || errors.Is(err, validation.ErrEmpty) {
				return ErrBioKeyTooLong
			}
			return err
		}
		if utf8.RuneCountInString(v) > MaxBioFieldLength {
			return ErrBioValueTooLong
		}
		vTrimmed := strings.TrimSpace(v)
		if vTrimmed != "" {
			if _, err := validation.ValidateMultiLine(vTrimmed, MaxBioFieldLength); err != nil {
				if errors.Is(err, validation.ErrTooLong) {
					return ErrBioValueTooLong
				}
				return err
			}
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

// ExileChecker reports participation in an open exile petition.
type ExileChecker interface {
	IsInvolved(ctx context.Context, characterID string) (bool, error)
}

// NewsPublisher defines announcement publication contract.
// Creation news must join the supplied transaction without external side effects
// or independent commits, so publication errors roll back creation as a whole.
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
