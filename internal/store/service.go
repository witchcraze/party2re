package store

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/town"
)

type Service struct {
	mutationCheck     func(context.Context, string) error
	repo              StoreRepository
	charRepo          CharacterRepository
	depotRepo         DepotRepository
	itemCatalog       ItemCatalog
	guildPoints       GuildPointsRegistrar
	timer             TimerService
	costumeRepo       costume.Repository
	homeWallpaperRepo HomeWallpaperRepository
	invRepo           InventoryRepository
	collection        CollectionRecorder
	helper            HelperProvider
	txProvider        TxProvider
	idGen             func() string
	nowFunc           func() time.Time
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
		s.wireCostumeClaimChecker()
	}
}

func WithCostumeRepository(cr costume.Repository) Option {
	return func(s *Service) {
		s.costumeRepo = cr
		s.wireCostumeClaimChecker()
	}
}

func WithHomeWallpaperRepository(hr HomeWallpaperRepository) Option {
	return func(s *Service) {
		s.homeWallpaperRepo = hr
	}
}

func WithInventoryRepository(inv InventoryRepository) Option {
	return func(s *Service) {
		s.invRepo = inv
	}
}

func WithCollectionRecorder(cr CollectionRecorder) Option {
	return func(s *Service) {
		s.collection = cr
	}
}

func WithHelperProvider(h HelperProvider) Option {
	return func(s *Service) {
		s.helper = h
	}
}

func (s *Service) SetCollectionRecorder(cr CollectionRecorder) {
	s.collection = cr
}

func (s *Service) SetHelperProvider(h HelperProvider) {
	s.helper = h
}

func WithTxProvider(tx TxProvider) Option {
	return func(s *Service) {
		s.txProvider = tx
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
	s.wireCostumeClaimChecker()
	return s
}

func (s *Service) wireCostumeClaimChecker() {
	if s.costumeRepo == nil || s.timer == nil {
		return
	}
	if mem, ok := s.costumeRepo.(interface {
		SetClaimExecutor(executor costume.ClaimExecutor)
	}); ok {
		mem.SetClaimExecutor(func(ctx context.Context, category, targetID, token string, action func() error) (bool, error) {
			return s.timer.ExecuteWithClaim(ctx, category, targetID, token, action)
		})
	}
}

// BuildStore builds the actor's store in townID (legacy ＠みせ: only a house
// style is chosen). A rebuilt store keeps its registered name.
func (s *Service) BuildStore(ctx context.Context, characterID, townID, houseStyle string) (*StoreCheckResult, error) {
	t, ok := town.GetTown(townID)
	if !ok {
		return nil, ErrInvalidTownID
	}
	if !town.IsValidHouseStyle(townID, houseStyle) {
		return nil, ErrInvalidHouseStyle
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
		if err := s.checkMutation(txCtx, characterID); err != nil {
			return err
		}

		// Legacy rejection order: funds, town capacity, already-owned store.
		if char.Money < StorePrice {
			return ErrInsufficientFunds
		}
		count, err := s.repo.CountActiveTownStores(txCtx, townID, now)
		if err != nil {
			return err
		}
		if count >= MaxTownStores {
			return ErrTownMaxStoresReached
		}
		existing, err := s.repo.GetStoreByCharacterID(txCtx, characterID)
		if err != nil && !errors.Is(err, ErrStoreNotFound) {
			return err
		}
		if err == nil && existing.IsActive(now) {
			return ErrAlreadyOwnsStore
		}

		// Legacy keeps the name, wallpaper, interiors and listings of an expired store.
		storeID, storeName, wallpaper := existing.ID, existing.StoreName, existing.Wallpaper
		if storeID == "" {
			storeID, storeName, wallpaper = s.idGen(), "@"+char.Name+"の店@", "none"
		}

		if err := char.DeductMoney(StorePrice); err != nil {
			return ErrInsufficientFunds
		}
		if err := s.charRepo.Update(txCtx, char); err != nil {
			return err
		}
		if s.guildPoints != nil {
			if err := s.guildPoints.AddGuildPoints(txCtx, characterID, StoreCycleDays*10); err != nil {
				return err
			}
		}

		expiresAt := now.Add(time.Duration(StoreCycleDays) * 24 * time.Hour)
		storeObj := Store{
			ID:          storeID,
			CharacterID: characterID,
			TownID:      townID,
			StoreName:   storeName,
			HouseStyle:  houseStyle,
			Wallpaper:   wallpaper,
			ExpiresAt:   expiresAt,
			CreatedAt:   now,
			UpdatedAt:   now,
		}
		if err := s.repo.SaveStore(txCtx, storeObj); err != nil {
			return err
		}

		jstExpires := expiresAt.In(timer.JST)
		res = &StoreCheckResult{
			CharacterID: characterID,
			OwnerName:   char.Name,
			TownID:      townID,
			TownName:    t.Name,
			StoreName:   storeName,
			HouseStyle:  houseStyle,
			Wallpaper:   storeObj.Wallpaper,
			ExpiresAt:   expiresAt,
			Message: fmt.Sprintf("%s の店を建てました！店の所有期間は %d月%d日%d時 までです",
				char.Name, jstExpires.Month(), jstExpires.Day(), jstExpires.Hour()),
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
	if err != nil {
		return nil, err
	}

	sales, err := s.repo.GetSalesByStoreID(ctx, storeID)
	if err != nil {
		return nil, err
	}
	if err := s.sortSalesLegacy(sales); err != nil {
		return nil, err
	}

	interiors, err := s.repo.GetInteriorsByStoreID(ctx, storeID)
	if err != nil {
		return nil, err
	}

	return &StoreDetails{
		Store:     st,
		OwnerName: char.Name,
		Sales:     append([]Sale{}, sales...),
		Interiors: append([]Interior{}, interiors...),
		MaxSales:  MaxListings(char.OverStore),
	}, nil
}

// GetTownStores lists all active stores in the specified town.
func (s *Service) GetTownStores(ctx context.Context, townID string) ([]Store, error) {
	now := s.nowFunc().UTC()
	return s.repo.ListActiveStoresInTown(ctx, townID, now)
}

// sortSalesLegacy applies legacy list_sort: gold before barter, then stored
// kind and number, then gold price, then listing number.
func (s *Service) sortSalesLegacy(sales []Sale) error {
	type key struct{ kind, no int }
	keys := make(map[string]key, len(sales))
	for _, sale := range sales {
		def, err := s.itemCatalog.FindByID(sale.ItemDefinitionID)
		if err != nil {
			return fmt.Errorf("store sale %s: %w", sale.ID, err)
		}
		kind, no, err := def.LegacyStorageKey()
		if err != nil {
			return fmt.Errorf("store sale %s: %w", sale.ID, err)
		}
		keys[sale.ID] = key{kind, no}
	}
	slices.SortStableFunc(sales, func(a, b Sale) int {
		ka, kb := keys[a.ID], keys[b.ID]
		return cmp.Or(cmp.Compare(saleTypeRank(a.SaleType), saleTypeRank(b.SaleType)), cmp.Compare(ka.kind, kb.kind),
			cmp.Compare(ka.no, kb.no), cmp.Compare(a.Price, b.Price), cmp.Compare(a.SlotNumber, b.SlotNumber))
	})
	return nil
}

func saleTypeRank(t string) int {
	if t == SaleTypeGold {
		return 0
	}
	return 1
}
