package secretshop

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MinAccessLevel      = 15
	MaxPurchaseQuantity = 99
	NPCName             = "@ヒミツジ"
	LocationName        = "秘密の店"
)

var (
	ErrNilDependency                = errors.New("secret shop dependency is nil")
	ErrCharacterNotFound            = errors.New("character not found")
	ErrAccessDenied                 = errors.New("secret shop access denied: character does not meet discovery requirements")
	ErrItemNotFound                 = errors.New("item not found in secret shop catalog")
	ErrItemUnavailableInHelperQuest = errors.New("item is temporarily unavailable due to active helper request")
	ErrInsufficientFunds            = errors.New("insufficient funds to purchase secret shop item")
	ErrInvalidQuantity              = errors.New("invalid purchase quantity")
	ErrPriceOverflow                = errors.New("price calculation overflow")
)

var DefaultTalkDialogues = []string{
	"バレちゃったメェ〜。他の人には秘密だメェ〜。",
	"値段は高いメェ〜けれど、他では手に入らないレアものだメェ〜。",
	"メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜。",
	"ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜。",
	"＠ぱふぱふはサービスだメェ〜。",
}

const (
	InspectDialogue  = "@ヒミツジ「オイラは羊の@ヒミツジだメェ〜。羊の国から来たよ…ゴホッゴホッ…羊の国から来たメェ〜」"
	PuffPuffDialogue = "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ わしのパフパフは気持ちいいだろう"
)

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, value coreinventory.Inventory) error
}

type HelperQuestFilter interface {
	GetActiveHelperItemIDs(ctx context.Context) ([]string, error)
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithHelperFilter(filter HelperQuestFilter) Option {
	return func(s *Service) {
		s.helperFilter = filter
	}
}

type Service struct {
	characterRepo CharacterRepository
	inventoryRepo InventoryRepository
	catalog       *Catalog
	helperFilter  HelperQuestFilter
	txProvider    TransactionProvider
	economy       *economy.Service
}

func NewService(
	characterRepo CharacterRepository,
	inventoryRepo InventoryRepository,
	catalog *Catalog,
	opts ...Option,
) (*Service, error) {
	if characterRepo == nil {
		return nil, fmt.Errorf("%w: character repository", ErrNilDependency)
	}
	if inventoryRepo == nil {
		return nil, fmt.Errorf("%w: inventory repository", ErrNilDependency)
	}
	if catalog == nil {
		return nil, fmt.Errorf("%w: catalog", ErrNilDependency)
	}

	s := &Service{
		characterRepo: characterRepo,
		inventoryRepo: inventoryRepo,
		catalog:       catalog,
	}
	for _, opt := range opts {
		opt(s)
	}
	var ecoOpts []economy.Option
	if s.txProvider != nil {
		ecoOpts = append(ecoOpts, economy.WithTransactionProvider(s.txProvider))
	}
	eco, err := economy.NewService(characterRepo, inventoryRepo, ecoOpts...)
	if err != nil {
		return nil, err
	}
	s.economy = eco
	return s, nil
}

// ShopStatus contains current secret shop status for a character.
type ShopStatus struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	IsEligible   bool   `json:"is_eligible"`
	Items        []Item `json:"items"`
}

// PurchaseResult contains outcome of a secret shop purchase.
type PurchaseResult struct {
	CharacterID         string `json:"character_id"`
	Item                Item   `json:"item"`
	Quantity            int    `json:"quantity"`
	TotalPrice          int    `json:"total_price"`
	RemainingGold       int    `json:"remaining_gold"`
	InventoryInstanceID string `json:"inventory_instance_id"`
}

// PuffPuffResult contains the result of the NPC puff-puff interaction.
type PuffPuffResult struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Message     string `json:"message"`
	HPHealed    int    `json:"hp_healed"`
	MPHealed    int    `json:"mp_healed"`
	CurrentHP   int    `json:"current_hp"`
	CurrentMP   int    `json:"current_mp"`
}

// CheckEligibility returns true if the character meets secret shop discovery qualifications.
func CheckEligibility(c corecharacter.Character) bool {
	return c.Level >= MinAccessLevel || c.RebirthCount > 0
}

// GetShopStatus checks access and returns available secret shop items.
func (s *Service) GetShopStatus(ctx context.Context, characterID string) (*ShopStatus, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return nil, ErrAccessDenied
	}

	items, err := s.getAvailableItems(ctx)
	if err != nil {
		return nil, err
	}

	return &ShopStatus{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		IsEligible:   true,
		Items:        items,
	}, nil
}

// Talk returns a secretive sheep dialogue from @ヒミツジ.
func (s *Service) Talk(ctx context.Context, characterID string) (string, error) {
	if strings.TrimSpace(characterID) == "" {
		return "", ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return "", ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return "", ErrAccessDenied
	}

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(DefaultTalkDialogues))))
	if err != nil {
		return DefaultTalkDialogues[0], nil
	}
	return DefaultTalkDialogues[nBig.Int64()], nil
}

// Inspect returns NPC background description.
func (s *Service) Inspect(ctx context.Context, characterID string) (string, error) {
	if strings.TrimSpace(characterID) == "" {
		return "", ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return "", ErrCharacterNotFound
	}

	if !CheckEligibility(char) {
		return "", ErrAccessDenied
	}

	return InspectDialogue, nil
}

// PuffPuff provides the playful secret puff-puff service and minor healing.
func (s *Service) PuffPuff(ctx context.Context, characterID string) (*PuffPuffResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	var result *PuffPuffResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		hpHealed := 0
		mpHealed := 0

		if char.Stats.HP < char.Stats.MaxHP {
			hpHealed = 10
			if char.Stats.HP+hpHealed > char.Stats.MaxHP {
				hpHealed = char.Stats.MaxHP - char.Stats.HP
			}
			char.Stats.HP += hpHealed
		}

		if char.Stats.MP < char.Stats.MaxMP {
			mpHealed = 5
			if char.Stats.MP+mpHealed > char.Stats.MaxMP {
				mpHealed = char.Stats.MaxMP - char.Stats.MP
			}
			char.Stats.MP += mpHealed
		}

		if hpHealed > 0 || mpHealed > 0 {
			if err := s.characterRepo.Update(txCtx, char); err != nil {
				return err
			}
		}

		result = &PuffPuffResult{
			CharacterID: char.ID,
			NPCName:     NPCName,
			Message:     PuffPuffDialogue,
			HPHealed:    hpHealed,
			MPHealed:    mpHealed,
			CurrentHP:   char.Stats.HP,
			CurrentMP:   char.Stats.MP,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return nil, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// PurchaseItem purchases rare items from the secret shop with transactional protection.
func (s *Service) PurchaseItem(
	ctx context.Context,
	characterID string,
	itemID string,
	quantity int,
) (*PurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if quantity <= 0 || quantity > MaxPurchaseQuantity {
		return nil, ErrInvalidQuantity
	}

	var result *PurchaseResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		shopItem, ok := s.catalog.FindByID(itemID)
		if !ok {
			return ErrItemNotFound
		}

		// Check helper quest exclusion filter if configured
		if s.helperFilter != nil {
			activeHelperItemIDs, err := s.helperFilter.GetActiveHelperItemIDs(txCtx)
			if err != nil {
				return err
			}
			for _, helperDefID := range activeHelperItemIDs {
				if helperDefID == shopItem.ItemDefinitionID {
					return ErrItemUnavailableInHelperQuest
				}
			}
		}

		totalPrice, err := safeMultiply(shopItem.Price, quantity)
		if err != nil {
			return err
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID:       characterID,
			DeductGold:        totalPrice,
			GrantDefinitionID: shopItem.ItemDefinitionID,
			GrantQuantity:     quantity,
		})
		if err != nil {
			if errors.Is(err, economy.ErrInsufficientGold) {
				return ErrInsufficientFunds
			}
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		result = &PurchaseResult{
			CharacterID:         char.ID,
			Item:                shopItem,
			Quantity:            quantity,
			TotalPrice:          totalPrice,
			RemainingGold:       res.Character.Money,
			InventoryInstanceID: res.GrantedItem.ID,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, operation); err != nil {
			return nil, err
		}
	} else {
		if err := operation(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

func (s *Service) getAvailableItems(ctx context.Context) ([]Item, error) {
	var excludedDefIDs map[string]bool
	if s.helperFilter != nil {
		activeHelperItemIDs, err := s.helperFilter.GetActiveHelperItemIDs(ctx)
		if err != nil {
			return nil, err
		}
		excludedDefIDs = make(map[string]bool, len(activeHelperItemIDs))
		for _, defID := range activeHelperItemIDs {
			excludedDefIDs[defID] = true
		}
	}

	items := s.catalog.Items()
	if len(excludedDefIDs) == 0 {
		return items, nil
	}

	var filtered []Item
	for _, item := range items {
		if !excludedDefIDs[item.ItemDefinitionID] {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func safeMultiply(price, qty int) (int, error) {
	if price < 0 || qty < 0 {
		return 0, ErrInvalidQuantity
	}
	val, err := economy.SafeMultiply(price, qty)
	if err != nil {
		return 0, ErrPriceOverflow
	}
	return val, nil
}
