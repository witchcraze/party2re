package character

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/progression"
)

var (
	ErrNotFound      = corecharacter.ErrNotFound
	ErrInvalidPlayer = errors.New("player ID is required")
)

type Repository interface {
	Save(ctx context.Context, value corecharacter.Character) error
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
	Delete(ctx context.Context, id string) error
}

// CleanupHook defines an interface for cleaning up external domain resources when a character is deleted.
type CleanupHook interface {
	CleanupCharacter(ctx context.Context, characterID string) error
}

type CleanupHookFunc func(ctx context.Context, characterID string) error

func (f CleanupHookFunc) CleanupCharacter(ctx context.Context, characterID string) error {
	return f(ctx, characterID)
}

type Option func(*Service)

// WithTransactionProvider sets the transaction provider.
func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

// WithNewsPublisher sets the news announcement publisher.
func WithNewsPublisher(news NewsPublisher) Option {
	return func(s *Service) {
		s.news = news
	}
}

// WithGuildChecker sets the guild membership checker.
func WithGuildChecker(checker GuildMembershipChecker) Option {
	return func(s *Service) {
		s.guildChecker = checker
	}
}

// WithFleaMarketChecker sets the flea market active listing checker.
func WithFleaMarketChecker(checker FleaMarketChecker) Option {
	return func(s *Service) {
		s.fleaChecker = checker
	}
}

// WithProfileRepository sets the profile repository.
func WithProfileRepository(repo ProfileRepository) Option {
	return func(s *Service) {
		s.profileRepo = repo
	}
}

// WithCleanupHook registers a cleanup hook to run prior to character deletion.
func WithCleanupHook(hook CleanupHook) Option {
	return func(s *Service) {
		if hook != nil {
			s.cleanupHooks = append(s.cleanupHooks, hook)
		}
	}
}

type Service struct {
	repository   Repository
	txProvider   TransactionProvider
	news         NewsPublisher
	guildChecker GuildMembershipChecker
	fleaChecker  FleaMarketChecker
	profileRepo  ProfileRepository
	cleanupHooks []CleanupHook
}

type CreationOptions struct {
	JobID  string
	Gender string
}

func NewService(repository Repository, opts ...Option) (*Service, error) {
	if repository == nil {
		return nil, errors.New("character repository is nil")
	}
	s := &Service{repository: repository}
	// Default profile repo if repository implements ProfileRepository
	if pRepo, ok := repository.(ProfileRepository); ok {
		s.profileRepo = pRepo
	}
	for _, opt := range opts {
		if opt != nil {
			opt(s)
		}
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) Create(ctx context.Context, playerID, name string) (corecharacter.Character, error) {
	return s.CreateWithOptions(ctx, playerID, name, CreationOptions{})
}

func (s *Service) CreateWithOptions(ctx context.Context, playerID, name string, options CreationOptions) (corecharacter.Character, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return corecharacter.Character{}, ErrInvalidPlayer
	}
	if options.JobID == "" {
		options.JobID = corecharacter.DefaultJobID
	}
	if options.Gender == "" {
		options.Gender = corecharacter.DefaultGender
	}
	value, err := corecharacter.NewWithOptions(name, options.JobID, options.Gender, nil)
	if err != nil {
		return corecharacter.Character{}, err
	}
	value.PlayerID = playerID
	if err := s.repository.Save(ctx, value); err != nil {
		return corecharacter.Character{}, err
	}

	return value, nil
}

func (s *Service) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	return s.repository.FindByID(ctx, id)
}

func (s *Service) ListByPlayer(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return nil, ErrInvalidPlayer
	}
	return s.repository.FindByPlayerID(ctx, playerID)
}

func (s *Service) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	return s.ListByPlayer(ctx, playerID)
}

func (s *Service) Rebirth(ctx context.Context, id string) (corecharacter.Character, error) {
	var result corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findForUpdate(txCtx, id)
		if err != nil {
			return err
		}
		if err := progression.Rebirth(&char); err != nil {
			return err
		}
		if err := s.repository.Update(txCtx, char); err != nil {
			return err
		}
		result = char
		return nil
	})
	return result, err
}

func (s *Service) findForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	if ext, ok := s.repository.(ExtendedRepository); ok {
		return ext.FindByIDForUpdate(ctx, id)
	}
	return s.repository.FindByID(ctx, id)
}

// ChangeName renames a character at the Naming Hall for 500,000 G.
func (s *Service) ChangeName(ctx context.Context, characterID, newName string) (corecharacter.Character, error) {
	if err := ValidateName(newName); err != nil {
		return corecharacter.Character{}, err
	}
	newName = strings.TrimSpace(newName)

	var result corecharacter.Character
	var oldName string

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock character
		char, err := s.findForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.Name == newName {
			return ErrSameName
		}

		// 2. Check funds (500,000 G)
		if char.Money < NameChangeCost {
			return ErrInsufficientGold
		}

		// 3. Check guild membership
		if s.guildChecker != nil {
			inGuild, err := s.guildChecker.IsInGuild(txCtx, characterID)
			if err != nil {
				return fmt.Errorf("check guild membership: %w", err)
			}
			if inGuild {
				return ErrInGuildDisallowed
			}
		}

		// 4. Check active flea market listings
		if s.fleaChecker != nil {
			hasListings, err := s.fleaChecker.HasActiveListings(txCtx, characterID)
			if err != nil {
				return fmt.Errorf("check flea market listings: %w", err)
			}
			if hasListings {
				return ErrActiveMarketDisallowed
			}
		}

		// 5. Check if new name is taken
		if ext, ok := s.repository.(ExtendedRepository); ok {
			existing, err := ext.FindByName(txCtx, newName)
			if err == nil && existing.ID != "" && existing.ID != characterID {
				return ErrNameAlreadyTaken
			}
		}

		// 6. Deduct fee and update name
		oldName = char.Name
		char.Money -= NameChangeCost
		char.Name = newName

		if err := s.repository.Update(txCtx, char); err != nil {
			return err
		}

		result = char
		return nil
	})

	if err != nil {
		return corecharacter.Character{}, err
	}

	// 7. Publish news announcement
	if s.news != nil {
		title := fmt.Sprintf("%sが %s と名前を変更", oldName, newName)
		content := fmt.Sprintf("冒険者 %s が命名の館にて新しい名『%s』を授かりました。", oldName, newName)
		_ = s.news.PublishNews(ctx, "character", title, content, "@マリナン", time.Now().UTC())
	}

	return result, nil
}

// ChangeGender changes a character's gender/appearance at the Naming Hall for 10,000 G.
func (s *Service) ChangeGender(ctx context.Context, characterID, newGender string) (corecharacter.Character, error) {
	validatedGender, err := ValidateGender(newGender)
	if err != nil {
		return corecharacter.Character{}, err
	}

	var result corecharacter.Character

	err = s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.findForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.Gender == validatedGender {
			return ErrSameGender
		}

		if char.Money < GenderChangeCost {
			return ErrInsufficientGold
		}

		char.Money -= GenderChangeCost
		char.Gender = validatedGender

		if err := s.repository.Update(txCtx, char); err != nil {
			return err
		}

		result = char
		return nil
	})

	if err != nil {
		return corecharacter.Character{}, err
	}

	return result, nil
}

// GetProfile retrieves a character's stats and public profile.
func (s *Service) GetProfile(ctx context.Context, characterID string) (ProfileView, error) {
	char, err := s.repository.FindByID(ctx, characterID)
	if err != nil {
		return ProfileView{}, err
	}

	profile := Profile{
		CharacterID: characterID,
		BioData:     make(map[string]string),
		UpdatedAt:   time.Now().UTC(),
	}

	if s.profileRepo != nil {
		stored, err := s.profileRepo.GetProfile(ctx, characterID)
		if err == nil && stored.CharacterID != "" {
			profile = stored
		}
	}

	return ProfileView{
		Character: char,
		Profile:   profile,
	}, nil
}

// UpdateProfile modifies a character's custom bio, comment, and avatar URL.
func (s *Service) UpdateProfile(ctx context.Context, characterID string, req UpdateProfileRequest) (Profile, error) {
	// Verify character exists
	if _, err := s.repository.FindByID(ctx, characterID); err != nil {
		return Profile{}, err
	}

	currentProfile := Profile{
		CharacterID: characterID,
		BioData:     make(map[string]string),
	}

	if s.profileRepo != nil {
		stored, err := s.profileRepo.GetProfile(ctx, characterID)
		if err == nil && stored.CharacterID != "" {
			currentProfile = stored
		}
	}

	if req.Comment != nil {
		if err := ValidateComment(*req.Comment); err != nil {
			return Profile{}, err
		}
		currentProfile.Comment = strings.TrimSpace(*req.Comment)
	}

	if req.AvatarURL != nil {
		if err := ValidateAvatarURL(*req.AvatarURL); err != nil {
			return Profile{}, err
		}
		currentProfile.AvatarURL = strings.TrimSpace(*req.AvatarURL)
	}

	if req.BioData != nil {
		if err := ValidateBioData(req.BioData); err != nil {
			return Profile{}, err
		}
		currentProfile.BioData = req.BioData
	}

	currentProfile.UpdatedAt = time.Now().UTC()

	if s.profileRepo != nil {
		if err := s.profileRepo.SaveProfile(ctx, currentProfile); err != nil {
			return Profile{}, err
		}
	}

	return currentProfile, nil
}

// UploadAvatar validates an image and sets the character's avatar URL to a safe data URI.
func (s *Service) UploadAvatar(ctx context.Context, characterID string, filename string, contentType string, data []byte) (string, error) {
	if len(data) == 0 {
		return "", ErrInvalidImageFormat
	}
	if len(data) > MaxAvatarSizeBytes {
		return "", ErrImageTooLarge
	}

	// Validate content type & image header
	normType := strings.ToLower(strings.TrimSpace(contentType))
	var mimeType string
	switch {
	case strings.Contains(normType, "png") || bytes.HasPrefix(data, []byte("\x89PNG\r\n\x1a\n")):
		mimeType = "image/png"
	case strings.Contains(normType, "jpeg") || strings.Contains(normType, "jpg") || (len(data) > 3 && data[0] == 0xFF && data[1] == 0xD8):
		mimeType = "image/jpeg"
	case strings.Contains(normType, "gif") || bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")):
		mimeType = "image/gif"
	case strings.Contains(normType, "webp") || (len(data) > 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"):
		mimeType = "image/webp"
	case strings.Contains(normType, "svg") || bytes.Contains(data, []byte("<svg")):
		mimeType = "image/svg+xml"
	default:
		// Attempt standard decode
		_, _, err := image.Decode(bytes.NewReader(data))
		if err != nil {
			return "", ErrInvalidImageFormat
		}
		mimeType = "image/png"
	}

	dataURI := fmt.Sprintf("data:%s;base64,%s", mimeType, base64.StdEncoding.EncodeToString(data))

	// Update avatar URL in profile
	_, err := s.UpdateProfile(ctx, characterID, UpdateProfileRequest{
		AvatarURL: &dataURI,
	})
	if err != nil {
		return "", err
	}

	return dataURI, nil
}

// GetNamingHallDialogue returns NPC @マリナン dialogue and fee information.
func (s *Service) GetNamingHallDialogue() NamingHallDialogue {
	return NamingHallDialogue{
		NPCName:       "@マリナン",
		LocationTitle: "命名の館",
		Phrases: []string{
			"ここは命名の館じゃ。お主の名前や性別を変えることができるぞ。",
			"名前を変えるということは運命を変えるということじゃ。とても大きなことなのじゃ。",
			"命名神の怒りに触れる名前にすると、存在が消されるらしいから気をつけることじゃ。",
			"ギルドに参加していたりフリーマーケットに出品している間は名前を変更できんぞ。",
		},
		NameChangeCost:   NameChangeCost,
		GenderChangeCost: GenderChangeCost,
	}
}

// Delete validates character ownership (if playerID is provided), executes all registered cleanup hooks,
// and deletes the character and its associated database records.
func (s *Service) Delete(ctx context.Context, playerID, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrNotFound
	}
	playerID = strings.TrimSpace(playerID)

	char, err := s.repository.FindByID(ctx, characterID)
	if err != nil {
		return err
	}

	if playerID != "" && char.PlayerID != playerID {
		return ErrForbidden
	}

	// Run domain cleanup hooks
	for _, hook := range s.cleanupHooks {
		if hook != nil {
			_ = hook.CleanupCharacter(ctx, characterID)
		}
	}

	return s.runInTx(ctx, func(txCtx context.Context) error {
		return s.repository.Delete(txCtx, characterID)
	})
}
