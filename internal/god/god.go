package god

import (
	"context"
	"fmt"
	"math/rand"
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
	return rand.Float64()
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
		hasMaid := false
		if s.homeMembers != nil {
			members, err := s.homeMembers.FindByCharacterID(ctx, characterID)
			if err == nil {
				for _, m := range members {
					if m.Name == "メイド" {
						hasMaid = true
						break
					}
				}
			}
		}

		inGuild := false
		if s.guilds != nil {
			_, _, err := s.guilds.GetGuildByCharacter(ctx, characterID)
			inGuild = (err == nil)
		}

		return buildHeavenWishes(char, inGuild, hasMaid), nil

	case RealmUnderworld:
		return buildUnderworldWishes(char), nil

	default:
		return nil, ErrInvalidRealm
	}
}

func (s *Service) GrantWish(
	ctx context.Context,
	characterID string,
	wishID string,
	realm Realm,
) (WishResult, error) {
	if characterID == "" {
		return WishResult{}, ErrInvalidCharacterID
	}
	if wishID == "" {
		return WishResult{}, ErrInvalidWishID
	}
	if realm != RealmHeaven && realm != RealmUnderworld {
		return WishResult{}, ErrInvalidRealm
	}

	var res WishResult

	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrCharacterNotFound, err)
		}

		switch realm {
		case RealmHeaven:
			return s.executeHeavenWish(txCtx, &char, wishID, &res)
		case RealmUnderworld:
			return s.executeUnderworldWish(txCtx, &char, wishID, &res)
		default:
			return ErrInvalidRealm
		}
	})

	if err != nil {
		return WishResult{}, err
	}

	return res, nil
}

func (s *Service) GetDialogue(realm Realm) []string {
	switch realm {
	case RealmHeaven:
		return []string{
			"よくぞここまでたどり着いた旅人よ…。",
			"私がお前の願いをひとつだけ叶えてやろう…。",
		}
	case RealmUnderworld:
		return []string{
			"深淵の闇を越え、よくぞ参った…。",
			"貴様の限界を超越する力を授けよう…。",
		}
	default:
		return nil
	}
}
