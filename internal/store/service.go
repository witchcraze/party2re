package store

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/town"
)

type Service struct {
	repo        StoreRepository
	charRepo    CharacterRepository
	depotRepo   DepotRepository
	itemCatalog ItemCatalog
	guildPoints GuildPointsRegistrar
	timer       TimerService
	txProvider  TxProvider
	idGen       func() string
	nowFunc     func() time.Time
}

type StoreCheckResult struct {
	CharacterID string    `json:"character_id"`
	OwnerName   string    `json:"owner_name"`
	TownID      string    `json:"town_id"`
	TownName    string    `json:"town_name"`
	StoreName   string    `json:"store_name"`
	HouseStyle  string    `json:"house_style"`
	Wallpaper   string    `json:"wallpaper"`
	ExpiresAt   time.Time `json:"expires_at"`
	Message     string    `json:"message"`
}

type StoreDetails struct {
	Store     Store      `json:"store"`
	OwnerName string     `json:"owner_name"`
	Sales     []Sale     `json:"sales"`
	Interiors []Interior `json:"interiors"`
	MaxSales  int        `json:"max_sales"`
}

type Option func(*Service)

func WithGuildPoints(gp GuildPointsRegistrar) Option {
	return func(s *Service) {
		s.guildPoints = gp
	}
}

func WithTimer(t TimerService) Option {
	return func(s *Service) {
		s.timer = t
	}
}

func WithIDGen(fn func() string) Option {
	return func(s *Service) {
		s.idGen = fn
	}
}

func WithNowFunc(fn func() time.Time) Option {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func NewService(
	repo StoreRepository,
	charRepo CharacterRepository,
	depotRepo DepotRepository,
	itemCatalog ItemCatalog,
	txProvider TxProvider,
	opts ...Option,
) *Service {
	s := &Service{
		repo:        repo,
		charRepo:    charRepo,
		depotRepo:   depotRepo,
		itemCatalog: itemCatalog,
		txProvider:  txProvider,
		idGen:       func() string { return fmt.Sprintf("store_%d", time.Now().UnixNano()) },
		nowFunc:     time.Now,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// BuildStore builds a new store in the specified town for 50,000 G and 90 days.
func (s *Service) BuildStore(ctx context.Context, characterID, townID, houseStyle, storeName string) (*StoreCheckResult, error) {
	t, ok := town.GetTown(townID)
	if !ok {
		return nil, ErrInvalidTownID
	}
	if !town.IsValidHouseStyle(townID, houseStyle) {
		return nil, ErrInvalidHouseStyle
	}

	cleanStoreName := strings.TrimSpace(storeName)
	if cleanStoreName != "" {
		if err := ValidateStoreName(cleanStoreName); err != nil {
			return nil, err
		}
	}

	now := s.nowFunc().UTC()
	var res *StoreCheckResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		if char.Money < StorePrice {
			return ErrInsufficientFunds
		}

		// 1-store globally check
		existing, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err == nil && existing.IsActive(now) {
			return ErrAlreadyOwnsStore
		}

		// Max 10 stores per town check
		count, err := s.repo.CountActiveTownStores(txCtx, townID, now)
		if err != nil {
			return err
		}
		if count >= MaxTownStores {
			return ErrTownMaxStoresReached
		}

		resolvedName := cleanStoreName
		if resolvedName == "" {
			resolvedName = char.Name + "の店"
		}

		// Check name collision with another active store
		namedStore, err := s.repo.GetStoreByName(txCtx, resolvedName)
		if err == nil && namedStore.CharacterID != characterID && namedStore.IsActive(now) {
			return ErrStoreNameTaken
		}

		if err := char.DeductMoney(StorePrice); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Save(txCtx, char); err != nil {
			return err
		}

		// Award guild points if applicable: 90 days * 10 = 900 GP
		if s.guildPoints != nil {
			_ = s.guildPoints.AddGuildPoints(txCtx, characterID, StoreCycleDays*10)
		}

		expiresAt := now.Add(time.Duration(StoreCycleDays) * 24 * time.Hour)
		storeID := existing.ID
		if storeID == "" {
			storeID = s.idGen()
		}

		storeObj := Store{
			ID:          storeID,
			CharacterID: characterID,
			TownID:      townID,
			StoreName:   resolvedName,
			HouseStyle:  houseStyle,
			Wallpaper:   "none",
			ExpiresAt:   expiresAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.SaveStore(txCtx, storeObj); err != nil {
			return err
		}

		jstExpires := expiresAt.In(timer.JST)
		msg := fmt.Sprintf("<b>%s の店</b>を建てました！店の所有期間は %d月%d日%d時 までです",
			char.Name, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour())

		res = &StoreCheckResult{
			CharacterID: characterID,
			OwnerName:   char.Name,
			TownID:      townID,
			TownName:    t.Name,
			StoreName:   resolvedName,
			HouseStyle:  houseStyle,
			Wallpaper:   storeObj.Wallpaper,
			ExpiresAt:   expiresAt,
			Message:     msg,
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	if s.timer != nil {
		_ = s.timer.SetLock(ctx, timer.CategoryStore, characterID, time.Duration(StoreCycleDays)*24*time.Hour)
	}

	return res, nil
}

// GetStore retrieves full details of a store including its listings and interiors.
func (s *Service) GetStore(ctx context.Context, storeID string) (*StoreDetails, error) {
	now := s.nowFunc().UTC()
	st, err := s.repo.GetStoreByID(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if !st.IsActive(now) {
		return nil, ErrStoreExpired
	}

	char, err := s.charRepo.FindByID(ctx, st.CharacterID)
	ownerName := "Unknown"
	maxSales := BaseMaxListings
	if err == nil {
		ownerName = char.Name
		maxSales = MaxListings(char.OverStore)
	}

	sales, err := s.repo.GetSalesByStoreID(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if sales == nil {
		sales = []Sale{}
	}

	interiors, err := s.repo.GetInteriorsByStoreID(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if interiors == nil {
		interiors = []Interior{}
	}

	return &StoreDetails{
		Store:     st,
		OwnerName: ownerName,
		Sales:     sales,
		Interiors: interiors,
		MaxSales:  maxSales,
	}, nil
}

// GetTownStores lists all active stores in the specified town.
func (s *Service) GetTownStores(ctx context.Context, townID string) ([]Store, error) {
	now := s.nowFunc().UTC()
	return s.repo.ListActiveStoresInTown(ctx, townID, now)
}

// CheckStore retrieves the active store status and expiration for a character.
func (s *Service) CheckStore(ctx context.Context, characterID string) (*StoreCheckResult, error) {
	now := s.nowFunc().UTC()
	st, err := s.repo.GetStoreByCharacterID(ctx, characterID)
	if err != nil {
		return nil, ErrStoreNotFound
	}
	if !st.IsActive(now) {
		return nil, ErrStoreExpired
	}

	char, err := s.charRepo.FindByID(ctx, characterID)
	ownerName := characterID
	if err == nil {
		ownerName = char.Name
	}

	t, _ := town.GetTown(st.TownID)
	jstExpires := st.ExpiresAt.In(timer.JST)
	msg := fmt.Sprintf("<b>%s</b>の所有期間は %d月%d日%d時 までです",
		st.StoreName, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour())

	return &StoreCheckResult{
		CharacterID: characterID,
		OwnerName:   ownerName,
		TownID:      st.TownID,
		TownName:    t.Name,
		StoreName:   st.StoreName,
		HouseStyle:  st.HouseStyle,
		Wallpaper:   st.Wallpaper,
		ExpiresAt:   st.ExpiresAt,
		Message:     msg,
	}, nil
}
