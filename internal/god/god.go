package god

import (
	"context"
	"errors"
	"fmt"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"

	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/guild"
)

type Option func(*Service)

func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depots = repo
	}
}

func WithInventoryRepository(repo InventoryRepository) Option {
	return func(s *Service) {
		s.inventories = repo
	}
}

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithCasinoRepository(repo CasinoRepository) Option {
	return func(s *Service) {
		s.casino = repo
	}
}

func WithLotteryRepository(repo LotteryRepository) Option {
	return func(s *Service) {
		s.lottery = repo
	}
}

func WithGuildRepository(repo GuildRepository) Option {
	return func(s *Service) {
		s.guilds = repo
	}
}

func WithHomeMemberRepository(repo HomeMemberRepository) Option {
	return func(s *Service) {
		s.homeMembers = repo
	}
}

func WithProfileRepository(repo ProfileRepository) Option {
	return func(s *Service) {
		s.profiles = repo
	}
}

func WithHomeRepository(repo HomeRepository) Option {
	return func(s *Service) {
		s.homes = repo
	}
}

func WithJobChanger(jobs JobChanger) Option {
	return func(s *Service) {
		s.jobs = jobs
	}
}

// WithMemoryRecorder records granted wishes in the character's trajectory.
func WithMemoryRecorder(recorder corecharacter.MemoryRecorder) Option {
	return func(s *Service) {
		s.memories = recorder
	}
}

func WithRandFloat(fn func() float64) Option {
	return func(s *Service) {
		s.randFloatFn = fn
	}
}

type Service struct {
	characters  CharacterRepository
	depots      DepotRepository
	inventories InventoryRepository
	txProvider  TransactionProvider
	casino      CasinoRepository
	lottery     LotteryRepository
	guilds      GuildRepository
	homeMembers HomeMemberRepository
	profiles    ProfileRepository
	homes       HomeRepository
	jobs        JobChanger
	memories    corecharacter.MemoryRecorder
	randFloatFn func() float64
}

func NewService(characters CharacterRepository, opts ...Option) (*Service, error) {
	if characters == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		characters: characters,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

func (s *Service) runInTx(ctx context.Context, fn func(context.Context) error) error {
	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, fn)
	}
	return fn(ctx)
}

func (s *Service) randFloat() float64 {
	if s.randFloatFn != nil {
		return s.randFloatFn()
	}
	return random.Float64()
}

func (s *Service) GetWishes(ctx context.Context, characterID string, realm Realm) ([]Wish, error) {
	if characterID == "" {
		return nil, ErrInvalidCharacterID
	}
	if realm != RealmHeaven && realm != RealmUnderworld {
		return nil, ErrInvalidRealm
	}

	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCharacterNotFound, err)
	}

	switch realm {
	case RealmHeaven:
		inGuild := false
		if s.guilds != nil {
			_, _, err := s.guilds.GetGuildByCharacter(ctx, characterID)
			if err != nil && !errors.Is(err, guild.ErrCharacterNotInGuild) {
				return nil, fmt.Errorf("read guild: %w", err)
			}
			inGuild = err == nil
		}

		return buildHeavenWishes(char, inGuild), nil

	case RealmUnderworld:
		return buildUnderworldWishes(char), nil

	default:
		return nil, ErrInvalidRealm
	}
}

// GrantWish grants wishID in the realm the character's recorded visit names.
func (s *Service) GrantWish(ctx context.Context, characterID, wishID string) (WishResult, error) {
	if characterID == "" {
		return WishResult{}, ErrInvalidCharacterID
	}
	if wishID == "" {
		return WishResult{}, ErrInvalidWishID
	}

	var res WishResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCharacterNotFound, err)
		}
		realm := Realm(char.GodVisit)
		// A granted wish ends the visit; executors persist the character with it.
		// Rejections roll back, and wish_lover persists nothing.
		char.GodVisit = ""

		switch realm {
		case RealmHeaven:
			err = s.executeHeavenWish(txCtx, &char, wishID, &res)
		case RealmUnderworld:
			err = s.executeUnderworldWish(txCtx, &char, wishID, &res)
		default:
			return ErrNotAdmitted
		}
		if err != nil || char.GodVisit != "" {
			return err // rejected, or refused with the visit continuing
		}
		return s.memories.Record(txCtx, characterID, corecharacter.MemoryWish,
			char.Name+"の願い『"+res.Wish.Name+"』を叶えてもらう")
	})

	if err != nil {
		return WishResult{}, err
	}

	return res, nil
}

// ForfeitVisit ends a recorded visit without a wish, as leaving the realm
// does in legacy (＠いどう/＠まち/＠ほーむ replace $m{lib}).
func (s *Service) ForfeitVisit(ctx context.Context, characterID string) error {
	return s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCharacterNotFound, err)
		}
		if char.GodVisit == "" {
			return nil
		}
		char.GodVisit = ""
		return s.characters.Update(txCtx, char)
	})
}
