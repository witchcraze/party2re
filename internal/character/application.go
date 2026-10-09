package character

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
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

// WithJobDefinitionProvider sets the job definition provider.
func WithJobDefinitionProvider(provider corejob.DefinitionProvider) Option {
	return func(s *Service) {
		s.jobProvider = provider
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
	jobProvider  corejob.DefinitionProvider
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
	if s.jobProvider == nil {
		if cat, err := corejob.InitialCatalog(); err == nil {
			s.jobProvider = cat
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
	return s.CreateWithOptions(ctx, playerID, name, CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
}

func (s *Service) CreateWithOptions(ctx context.Context, playerID, name string, options CreationOptions) (corecharacter.Character, error) {
	playerID = strings.TrimSpace(playerID)
	if playerID == "" {
		return corecharacter.Character{}, ErrInvalidPlayer
	}
	if err := ValidateName(name); err != nil {
		return corecharacter.Character{}, err
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
		if err := char.DeductMoney(NameChangeCost); err != nil {
			return ErrInsufficientGold
		}
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

		if s.jobProvider != nil && char.JobID != "" {
			jobDef, err := s.jobProvider.FindByID(char.JobID)
			if err != nil && !errors.Is(err, corejob.ErrDefinitionNotFound) {
				return err
			}
			if err == nil && jobDef.RequiredGender != "" && jobDef.RequiredGender != validatedGender {
				return ErrGenderIncompatibleWithJob
			}
		}

		if err := char.DeductMoney(GenderChangeCost); err != nil {
			return ErrInsufficientGold
		}
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
