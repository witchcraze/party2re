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
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MinAccessJobLevel   = 7
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
	ErrDepotFull                    = depot.ErrDepotFull
	ErrDepotNotConfigured           = errors.New("depot repository not configured")
)

var DefaultTalkDialogues = []string{
	"バレちゃったメェ〜。他の人には秘密だメェ〜。",
	"値段は高いメェ〜けれど、他では手に入らないレアものだメェ〜。",
	"メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜メェ〜。",
	"ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜ベェ〜。",
	"＠ぱふぱふはサービスだメェ〜。",
}

const (
	InspectDialogue        = "@ヒミツジ「オイラは羊の@ヒミツジだメェ〜。羊の国から来たよ…ゴホッゴホッ…羊の国から来たメェ〜」"
	PuffPuffDialogue       = "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ わしのパフパフは気持ちいいだろう"
	PuffPuffDialogueFormat = "パフパフ♥ パフパフ♥ パフパフ♥ ……… どうだ %s わしのパフパフは気持ちいいだろう"
)

// FormatPuffPuffMessage formats the puff-puff dialogue with the character name, matching legacy CGI (secret.cgi).
func FormatPuffPuffMessage(charName string) string {
	if strings.TrimSpace(charName) == "" {
		return PuffPuffDialogue
	}
	return fmt.Sprintf(PuffPuffDialogueFormat, charName)
}

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

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, value depot.Depot) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
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

func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = repo
	}
}

func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.itemDefProvider = provider
	}
}

type Service struct {
	characterRepo   CharacterRepository
	inventoryRepo   InventoryRepository
	catalog         *Catalog
	helperFilter    HelperQuestFilter
	txProvider      TransactionProvider
	economy         *economy.Service
	depotRepo       DepotRepository
	itemDefProvider ItemDefinitionProvider
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
	TransferredToDepot  bool   `json:"transferred_to_depot"`
	NPCMessage          string `json:"npc_message"`
}

// PuffPuffResult contains the result of the NPC puff-puff interaction.
// Under legacy parity (secret.cgi), puff-puff provides flavor dialogue only with no healing.
type PuffPuffResult struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Message     string `json:"message"`
}

// CheckEligibility returns true if the character meets secret shop discovery qualifications.
// The original CGI (item.cgi:himitsunomise) is accessible only when job_lv >= 7.
func CheckEligibility(c corecharacter.Character) bool {
	return c.JobLevel >= MinAccessJobLevel
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

// PuffPuff provides the playful secret puff-puff service with flavor text only (no stat modifications).
func (s *Service) PuffPuff(ctx context.Context, characterID string) (*PuffPuffResult, error) {
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

	return &PuffPuffResult{
		CharacterID: char.ID,
		NPCName:     NPCName,
		Message:     FormatPuffPuffMessage(char.Name),
	}, nil
}

// PurchaseItem purchases rare items from the secret shop with transactional protection.
// If inventory consumable slot is empty and quantity == 1, it is placed in character inventory.
// If inventory is occupied or quantity > 1, it is automatically transferred to depot.
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

		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		occupied := false
		for _, inst := range inv.Items {
			if s.itemDefProvider != nil {
				def, err := s.itemDefProvider.FindByID(inst.DefinitionID)
				if err == nil && (def.Slot == coreitem.SlotNone || def.Slot == "") {
					occupied = true
					break
				}
			} else {
				if _, ok := s.catalog.FindByDefinitionID(inst.DefinitionID); ok || strings.HasPrefix(inst.DefinitionID, "item-") {
					occupied = true
					break
				}
			}
		}

		if !occupied && quantity == 1 {
			res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
				CharacterID:       characterID,
				DeductGold:        totalPrice,
				GrantDefinitionID: shopItem.ItemDefinitionID,
				GrantQuantity:     1,
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
				Quantity:            1,
				TotalPrice:          totalPrice,
				RemainingGold:       res.Character.Money,
				InventoryInstanceID: res.GrantedItem.ID,
				TransferredToDepot:  false,
				NPCMessage:          fmt.Sprintf("%sメェ〜。持ってけメェ〜", shopItem.Name),
			}
			return nil
		}

		// Consumable slot is occupied or quantity > 1: route to depot
		if s.depotRepo == nil {
			return ErrDepotNotConfigured
		}

		dep, err := s.depotRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			if !errors.Is(err, depot.ErrNotFound) {
				return err
			}
			dep, err = depot.NewDepotWithCapacity(characterID, char.JobLevel, 0, char.OverDepot)
			if err != nil {
				return err
			}
		}
		dep.RefreshCapacity(char.JobLevel, char.OverDepot)

		inst, err := coreitem.NewInstance(shopItem.ItemDefinitionID, quantity)
		if err != nil {
			return err
		}
		if err := dep.AddItem(inst); err != nil {
			if errors.Is(err, depot.ErrDepotFull) {
				return ErrDepotFull
			}
			return err
		}

		res, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID: characterID,
			DeductGold:  totalPrice,
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

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		result = &PurchaseResult{
			CharacterID:         char.ID,
			Item:                shopItem,
			Quantity:            quantity,
			TotalPrice:          totalPrice,
			RemainingGold:       res.Character.Money,
			InventoryInstanceID: inst.ID,
			TransferredToDepot:  true,
			NPCMessage:          fmt.Sprintf("%sは%sメェ〜の預かり所の方に投げましたメェ〜", shopItem.Name, char.Name),
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
