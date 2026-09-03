package blackmarket

import (
	"context"
	"crypto/rand"
	"database/sql"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

const (
	MinAccessLevel      = 10
	MaxPurchaseQuantity = 99
	NPCName             = "@ヤミジ"
	LocationName        = "闇市"
	BaseBuybackRate     = 0.60
)

var (
	ErrNilDependency           = errors.New("black market dependency is nil")
	ErrCharacterNotFound       = errors.New("character not found")
	ErrAccessDenied            = errors.New("black market access denied: character does not meet discovery requirements")
	ErrItemNotFound            = errors.New("item not found in black market catalog")
	ErrInsufficientFunds       = errors.New("insufficient funds to purchase black market item")
	ErrInvalidQuantity         = errors.New("invalid purchase or sale quantity")
	ErrDailyLimitExceeded      = errors.New("daily purchase limit exceeded for this item")
	ErrUnownedItem             = errors.New("item instance is not owned in inventory")
	ErrPriceOverflow           = errors.New("price calculation overflow")
	ErrNotSacrificeEligible    = errors.New("item is not eligible for rare point sacrifice")
	ErrInsufficientRarePoints  = errors.New("insufficient rare points for prize trade")
	ErrInsufficientURarePoints = errors.New("insufficient u-rare points for prize trade")
	ErrPrizeNotFound           = errors.New("prize item not found in trade catalog")
)

type MarketCondition string

const (
	ConditionQuiet     MarketCondition = "Quiet"     // 平穏 (1.0x buy, 1.0x sell, Low risk)
	ConditionHotDemand MarketCondition = "HotDemand" // 特需 (1.3x buy, 1.2x sell, Medium risk)
	ConditionCrackdown MarketCondition = "Crackdown" // 取締警戒 (1.5x buy, 0.9x sell, High risk)
	ConditionBargain   MarketCondition = "Bargain"   // 在庫処分 (0.8x buy, 0.7x sell, Low risk)
)

// MarketState describes current underground market conditions.
type MarketState struct {
	Condition       MarketCondition `json:"condition"`
	PriceMultiplier float64         `json:"price_multiplier"`
	SellMultiplier  float64         `json:"sell_multiplier"`
	RiskLevel       string          `json:"risk_level"`
	Description     string          `json:"description"`
}

var DefaultMarketStates = map[MarketCondition]MarketState{
	ConditionQuiet: {
		Condition:       ConditionQuiet,
		PriceMultiplier: 1.0,
		SellMultiplier:  1.0,
		RiskLevel:       "Low",
		Description:     "市場は平穏で取引は安定しています。",
	},
	ConditionHotDemand: {
		Condition:       ConditionHotDemand,
		PriceMultiplier: 1.3,
		SellMultiplier:  1.2,
		RiskLevel:       "Medium",
		Description:     "裏取引の需要が急増し、商品価格・買取価格ともに上昇しています。",
	},
	ConditionCrackdown: {
		Condition:       ConditionCrackdown,
		PriceMultiplier: 1.5,
		SellMultiplier:  0.9,
		RiskLevel:       "High",
		Description:     "街の衛兵による取締りが強化されており、密輸コストにより品物が高騰しています。",
	},
	ConditionBargain: {
		Condition:       ConditionBargain,
		PriceMultiplier: 0.8,
		SellMultiplier:  0.7,
		RiskLevel:       "Low",
		Description:     "闇商人が過剰在庫を処分するため、割引価格で放出しています。",
	},
}

var DefaultTalkDialogues = []string{
	"……チッ、誰に聞いてここに来た？ まぁいい、金さえ払えば客だ。",
	"表の店じゃ手に入らない上玉が揃ってるぜ。使い方は自己責任だがな。",
	"ヒッヒッヒ……夜の取引は静かに頼むぜ。衛兵に勘づかれたら面倒だからな。",
	"おいおい、ジロジロ見るんじゃねえ。買うか消えるか、どっちかにしな。",
	"品質は保証するが、出所は聞かないのがここの暗黙のルールだぜ。",
}

var ConditionRumors = map[MarketCondition]string{
	ConditionQuiet:     "今は衛兵の目も緩んでて相場は落ち着いてるぜ。じっくり品定めしな。",
	ConditionHotDemand: "最近は冒険者共がこぞってブツを買い漁ってやがる。少し値は張るが、買い取りも色をつけてやるぜ。",
	ConditionCrackdown: "クソッ、衛兵の巡回が厳しくなってやがる。密輸ルートが塞がれて価格が高騰中だ。騒ぐんじゃねえぞ。",
	ConditionBargain:   "ちょっと在庫を抱えすぎちまったんでな、今なら特別に安く譲ってやるぜ。早い者勝ちだ。",
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

type CharacterPoints struct {
	CharacterID string `json:"character_id"`
	RarePoints  int    `json:"rare_points"`
	URarePoints int    `json:"u_rare_points"`
}

type BlackMarketRepository interface {
	GetDailyPurchases(ctx context.Context, characterID string, date time.Time) (map[string]int, error)
	RecordPurchase(ctx context.Context, characterID string, itemID string, date time.Time, quantity int) error
	GetMarketState(ctx context.Context) (MarketState, error)
	SaveMarketState(ctx context.Context, state MarketState) error
	GetCharacterPoints(ctx context.Context, characterID string) (CharacterPoints, error)
	GetCharacterPointsForUpdate(ctx context.Context, characterID string) (CharacterPoints, error)
	SaveCharacterPoints(ctx context.Context, points CharacterPoints) error
}

type ItemDefinitionProvider = coreitem.DefinitionProvider

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// PointsStatus describes accumulated points and available trade prize catalogs.
type PointsStatus struct {
	CharacterID string  `json:"character_id"`
	RarePoints  int     `json:"rare_points"`
	URarePoints int     `json:"u_rare_points"`
	Prizes      []Prize `json:"prizes"`
	UPrizes     []Prize `json:"u_prizes"`
}

// SacrificeResult contains outcome of sacrificing a rare item.
type SacrificeResult struct {
	CharacterID       string `json:"character_id"`
	ItemInstanceID    string `json:"item_instance_id"`
	ItemDefinitionID  string `json:"item_definition_id"`
	ItemName          string `json:"item_name"`
	RarePointsGained  int    `json:"rare_points_gained"`
	URarePointsGained int    `json:"u_rare_points_gained"`
	TotalRarePoints   int    `json:"total_rare_points"`
	TotalURarePoints  int    `json:"total_u_rare_points"`
	Message           string `json:"message"`
}

// TradeResult contains outcome of trading points for a prize item.
type TradeResult struct {
	CharacterID         string `json:"character_id"`
	PrizeID             string `json:"prize_id"`
	ItemDefinitionID    string `json:"item_definition_id"`
	ItemName            string `json:"item_name"`
	InventoryInstanceID string `json:"inventory_instance_id"`
	Cost                int    `json:"cost"`
	IsURare             bool   `json:"is_u_rare"`
	RemainingRare       int    `json:"remaining_rare"`
	RemainingURare      int    `json:"remaining_u_rare"`
	Message             string `json:"message"`
}

// ShopItemView represents an item listing with dynamic market prices and remaining character quotas.
type ShopItemView struct {
	Item           Item `json:"item"`
	EffectivePrice int  `json:"effective_price"`
	PurchasedToday int  `json:"purchased_today"`
	RemainingQuota int  `json:"remaining_quota"`
}

// ShopStatus contains current black market status, market condition, and item catalog.
type ShopStatus struct {
	CharacterID  string         `json:"character_id"`
	LocationName string         `json:"location_name"`
	NPCName      string         `json:"npc_name"`
	IsEligible   bool           `json:"is_eligible"`
	MarketState  MarketState    `json:"market_state"`
	Items        []ShopItemView `json:"items"`
}

// PurchaseResult contains outcome of a black market purchase.
type PurchaseResult struct {
	CharacterID         string `json:"character_id"`
	Item                Item   `json:"item"`
	Quantity            int    `json:"quantity"`
	UnitPrice           int    `json:"unit_price"`
	TotalPrice          int    `json:"total_price"`
	RemainingGold       int    `json:"remaining_gold"`
	InventoryInstanceID string `json:"inventory_instance_id"`
	RemainingQuota      int    `json:"remaining_quota"`
}

// SaleResult contains outcome of selling an item to the black market.
type SaleResult struct {
	CharacterID    string `json:"character_id"`
	ItemInstanceID string `json:"item_instance_id"`
	ItemName       string `json:"item_name"`
	Quantity       int    `json:"quantity"`
	UnitPrice      int    `json:"unit_price"`
	TotalPayout    int    `json:"total_payout"`
	RemainingGold  int    `json:"remaining_gold"`
}

// TalkResult contains outcome of talking to the NPC.
type TalkResult struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Dialogue    string `json:"dialogue"`
}

// RumorsResult contains outcome of inquiring about underground rumors.
type RumorsResult struct {
	CharacterID     string `json:"character_id"`
	NPCName         string `json:"npc_name"`
	MarketCondition string `json:"market_condition"`
	Rumor           string `json:"rumor"`
}

// CheckEligibility returns true if the character meets black market discovery requirements.
func CheckEligibility(c corecharacter.Character) bool {
	return c.Level >= MinAccessLevel || c.RebirthCount > 0
}

type Option func(*Service)

func WithTransactionProvider(txProvider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = txProvider
	}
}

func WithItemDefinitionProvider(itemDefs ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.itemDefs = itemDefs
	}
}

type Service struct {
	characterRepo   CharacterRepository
	inventoryRepo   InventoryRepository
	blackMarketRepo BlackMarketRepository
	catalog         *Catalog
	itemDefs        ItemDefinitionProvider
	txProvider      TransactionProvider
	economy         *economy.Service
}

func NewService(
	characterRepo CharacterRepository,
	inventoryRepo InventoryRepository,
	blackMarketRepo BlackMarketRepository,
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
		characterRepo:   characterRepo,
		inventoryRepo:   inventoryRepo,
		blackMarketRepo: blackMarketRepo,
		catalog:         catalog,
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

// GetMarketState determines current market state either from repository or deterministic default.
func (s *Service) GetMarketState(ctx context.Context, now time.Time) MarketState {
	if s.blackMarketRepo != nil {
		if state, err := s.blackMarketRepo.GetMarketState(ctx); err == nil && state.Condition != "" {
			if def, ok := DefaultMarketStates[state.Condition]; ok {
				if state.PriceMultiplier <= 0 {
					state.PriceMultiplier = def.PriceMultiplier
				}
				if state.SellMultiplier <= 0 {
					state.SellMultiplier = def.SellMultiplier
				}
				if state.RiskLevel == "" {
					state.RiskLevel = def.RiskLevel
				}
				if state.Description == "" {
					state.Description = def.Description
				}
			}
			return state
		}
	}

	// Deterministic market condition rotation based on hour of the day
	hour := now.UTC().Hour()
	switch hour % 4 {
	case 1:
		return DefaultMarketStates[ConditionHotDemand]
	case 2:
		return DefaultMarketStates[ConditionCrackdown]
	case 3:
		return DefaultMarketStates[ConditionBargain]
	default:
		return DefaultMarketStates[ConditionQuiet]
	}
}

// CalculateEffectiveBuyPrice computes the dynamic purchase price for a black market item.
func CalculateEffectiveBuyPrice(basePrice int, multiplier float64) int {
	if basePrice <= 0 {
		return 0
	}
	if multiplier <= 0 {
		multiplier = 1.0
	}
	price := int(math.Ceil(float64(basePrice) * multiplier))
	if price <= 0 {
		return 1
	}
	return price
}

// CalculateEffectiveSellPrice computes the dynamic buyback payout for an item.
func CalculateEffectiveSellPrice(basePrice int, sellMultiplier float64) int {
	if basePrice <= 0 {
		return 0
	}
	if sellMultiplier <= 0 {
		sellMultiplier = 1.0
	}
	price := int(math.Floor(float64(basePrice) * BaseBuybackRate * sellMultiplier))
	if price <= 0 {
		return 1
	}
	return price
}

func safeMultiply(a, b int) (int, error) {
	if a <= 0 || b <= 0 {
		return 0, nil
	}
	if a > math.MaxInt/b {
		return 0, ErrPriceOverflow
	}
	return a * b, nil
}

// GetStatus returns the black market status, market condition, and catalog view for a character.
func (s *Service) GetStatus(ctx context.Context, characterID string, now time.Time) (*ShopStatus, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return nil, ErrCharacterNotFound
	}

	marketState := s.GetMarketState(ctx, now)
	eligible := CheckEligibility(char)

	var dailyPurchases map[string]int
	if s.blackMarketRepo != nil {
		dailyPurchases, _ = s.blackMarketRepo.GetDailyPurchases(ctx, characterID, now)
	}
	if dailyPurchases == nil {
		dailyPurchases = make(map[string]int)
	}

	catalogItems := s.catalog.Items()
	views := make([]ShopItemView, 0, len(catalogItems))
	for _, it := range catalogItems {
		effectivePrice := CalculateEffectiveBuyPrice(it.BasePrice, marketState.PriceMultiplier)
		purchased := dailyPurchases[it.ID]
		remaining := it.DailyLimit - purchased
		if remaining < 0 {
			remaining = 0
		}
		views = append(views, ShopItemView{
			Item:           it,
			EffectivePrice: effectivePrice,
			PurchasedToday: purchased,
			RemainingQuota: remaining,
		})
	}

	return &ShopStatus{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		IsEligible:   eligible,
		MarketState:  marketState,
		Items:        views,
	}, nil
}

// Talk provides randomized dialogue from shady broker NPC @ヤミジ.
func (s *Service) Talk(ctx context.Context, characterID string) (*TalkResult, error) {
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

	nBig, err := rand.Int(rand.Reader, big.NewInt(int64(len(DefaultTalkDialogues))))
	if err != nil {
		return &TalkResult{
			CharacterID: char.ID,
			NPCName:     NPCName,
			Dialogue:    DefaultTalkDialogues[0],
		}, nil
	}

	return &TalkResult{
		CharacterID: char.ID,
		NPCName:     NPCName,
		Dialogue:    DefaultTalkDialogues[nBig.Int64()],
	}, nil
}

// Rumors returns underground rumors matching current market condition.
func (s *Service) Rumors(ctx context.Context, characterID string, now time.Time) (*RumorsResult, error) {
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

	marketState := s.GetMarketState(ctx, now)
	rumor, ok := ConditionRumors[marketState.Condition]
	if !ok {
		rumor = ConditionRumors[ConditionQuiet]
	}

	return &RumorsResult{
		CharacterID:     char.ID,
		NPCName:         NPCName,
		MarketCondition: string(marketState.Condition),
		Rumor:           rumor,
	}, nil
}

// PurchaseItem purchases black market contraband with concurrency-safe transaction isolation.
func (s *Service) PurchaseItem(
	ctx context.Context,
	characterID string,
	itemID string,
	quantity int,
	now time.Time,
) (*PurchaseResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if quantity <= 0 || quantity > MaxPurchaseQuantity {
		return nil, ErrInvalidQuantity
	}

	shopItem, ok := s.catalog.FindByID(itemID)
	if !ok {
		return nil, ErrItemNotFound
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

		marketState := s.GetMarketState(txCtx, now)
		unitPrice := CalculateEffectiveBuyPrice(shopItem.BasePrice, marketState.PriceMultiplier)
		totalPrice, err := safeMultiply(unitPrice, quantity)
		if err != nil {
			return err
		}

		if char.Money < totalPrice {
			return ErrInsufficientFunds
		}

		purchasedToday := 0
		if s.blackMarketRepo != nil {
			dailyPurchases, err := s.blackMarketRepo.GetDailyPurchases(txCtx, characterID, now)
			if err != nil {
				return err
			}
			purchasedToday = dailyPurchases[shopItem.ID]
		}

		if purchasedToday+quantity > shopItem.DailyLimit {
			return ErrDailyLimitExceeded
		}

		ecoRes, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
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

		if s.blackMarketRepo != nil {
			if err := s.blackMarketRepo.RecordPurchase(txCtx, characterID, shopItem.ID, now, quantity); err != nil {
				return err
			}
		}

		newPurchasedToday := purchasedToday + quantity
		remainingQuota := shopItem.DailyLimit - newPurchasedToday
		if remainingQuota < 0 {
			remainingQuota = 0
		}

		result = &PurchaseResult{
			CharacterID:         char.ID,
			Item:                shopItem,
			Quantity:            quantity,
			UnitPrice:           unitPrice,
			TotalPrice:          totalPrice,
			RemainingGold:       ecoRes.Character.Money,
			InventoryInstanceID: ecoRes.GrantedItem.ID,
			RemainingQuota:      remainingQuota,
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

// SellItem sells an owned inventory item to the shady merchant with dynamic buyback rates.
func (s *Service) SellItem(
	ctx context.Context,
	characterID string,
	itemInstanceID string,
	quantity int,
	now time.Time,
) (*SaleResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return nil, ErrUnownedItem
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}

	var result *SaleResult

	operation := func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrUnownedItem
		}

		inst, ok := inv.Find(itemInstanceID)
		if !ok {
			return ErrUnownedItem
		}

		if inst.Quantity < quantity {
			return ErrInvalidQuantity
		}

		// Determine base price of the item
		basePrice := 100
		itemName := inst.DefinitionID
		if s.itemDefs != nil {
			if def, err := s.itemDefs.FindByID(inst.DefinitionID); err == nil {
				basePrice = def.Price
				if def.Name != "" {
					itemName = def.Name
				}
			}
		} else if bmItem, found := s.catalog.FindByDefinitionID(inst.DefinitionID); found {
			basePrice = bmItem.BasePrice
			itemName = bmItem.Name
		}

		marketState := s.GetMarketState(txCtx, now)
		unitPrice := CalculateEffectiveSellPrice(basePrice, marketState.SellMultiplier)
		totalPayout, err := safeMultiply(unitPrice, quantity)
		if err != nil {
			return err
		}

		ecoRes, err := s.economy.Exchange(txCtx, economy.ExchangeRequest{
			CharacterID:        characterID,
			AddGold:            totalPayout,
			ConsumeInstanceID:  itemInstanceID,
			ConsumeInstanceQty: quantity,
		})
		if err != nil {
			if errors.Is(err, economy.ErrCharacterNotFound) {
				return ErrCharacterNotFound
			}
			if errors.Is(err, economy.ErrItemNotFound) {
				return ErrUnownedItem
			}
			if errors.Is(err, economy.ErrInsufficientItemQuantity) {
				return ErrInvalidQuantity
			}
			return err
		}

		result = &SaleResult{
			CharacterID:    char.ID,
			ItemInstanceID: itemInstanceID,
			ItemName:       itemName,
			Quantity:       quantity,
			UnitPrice:      unitPrice,
			TotalPayout:    totalPayout,
			RemainingGold:  ecoRes.Character.Money,
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

// GetPointsStatus returns accumulated Rare Points and U-Rare Points and available prizes.
func (s *Service) GetPointsStatus(ctx context.Context, characterID string) (*PointsStatus, error) {
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

	points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
	if s.blackMarketRepo != nil {
		if pts, err := s.blackMarketRepo.GetCharacterPoints(ctx, characterID); err == nil {
			points = pts
		} else if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
	}

	return &PointsStatus{
		CharacterID: characterID,
		RarePoints:  points.RarePoints,
		URarePoints: points.URarePoints,
		Prizes:      s.catalog.RegularPrizes(),
		UPrizes:     s.catalog.UPrizes(),
	}, nil
}

// SacrificeItem consumes an eligible rare item instance from character inventory and credits points.
func (s *Service) SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*SacrificeResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(itemInstanceID) == "" {
		return nil, ErrUnownedItem
	}

	var result *SacrificeResult

	operation := func(txCtx context.Context) error {
		// 1. Lock character (Tier 2)
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		// 2. Lock inventory (Tier 3)
		inv, err := s.inventoryRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inst, found := inv.Find(itemInstanceID)
		if !found {
			return ErrUnownedItem
		}

		// 3. Check sacrifice eligibility
		yield, eligible := s.catalog.GetSacrificeYield(inst.DefinitionID)
		if !eligible {
			return ErrNotSacrificeEligible
		}

		// 4. Consume 1 unit of the item instance
		if _, err := s.economy.ConsumeItemInstance(txCtx, characterID, itemInstanceID, 1); err != nil {
			return err
		}

		// 5. Lock and update points (Tier 8)
		points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
		if s.blackMarketRepo != nil {
			pts, err := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, characterID)
			if err == nil {
				points = pts
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}

		points.RarePoints += yield.RarePoints
		points.URarePoints += yield.URarePoints

		if s.blackMarketRepo != nil {
			if err := s.blackMarketRepo.SaveCharacterPoints(txCtx, points); err != nil {
				return err
			}
		}

		itemName := inst.DefinitionID
		if s.itemDefs != nil {
			if def, err := s.itemDefs.FindByID(inst.DefinitionID); err == nil && def.Name != "" {
				itemName = def.Name
			}
		}

		msg := "…レアだな…。いいだろう…。お前のレアポイントを加算しておこう…"
		if yield.URarePoints > 0 {
			msg = fmt.Sprintf("これは……! ……いいだろう…。お前の特別なレアポイントを%d加算しておこう…", yield.URarePoints)
		}

		result = &SacrificeResult{
			CharacterID:       char.ID,
			ItemInstanceID:    itemInstanceID,
			ItemDefinitionID:  inst.DefinitionID,
			ItemName:          itemName,
			RarePointsGained:  yield.RarePoints,
			URarePointsGained: yield.URarePoints,
			TotalRarePoints:   points.RarePoints,
			TotalURarePoints:  points.URarePoints,
			Message:           msg,
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

// TradePrize exchanges accumulated Rare Points or U-Rare Points for an exclusive prize item.
func (s *Service) TradePrize(ctx context.Context, characterID string, prizeID string) (*TradeResult, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrCharacterNotFound
	}
	if strings.TrimSpace(prizeID) == "" {
		return nil, ErrPrizeNotFound
	}

	prize, found := s.catalog.FindPrizeByID(prizeID)
	if !found {
		return nil, ErrPrizeNotFound
	}

	var result *TradeResult

	operation := func(txCtx context.Context) error {
		// 1. Lock character (Tier 2)
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		if !CheckEligibility(char) {
			return ErrAccessDenied
		}

		// 2. Lock and check points (Tier 8)
		points := CharacterPoints{CharacterID: characterID, RarePoints: 0, URarePoints: 0}
		if s.blackMarketRepo != nil {
			pts, err := s.blackMarketRepo.GetCharacterPointsForUpdate(txCtx, characterID)
			if err == nil {
				points = pts
			} else if !errors.Is(err, sql.ErrNoRows) {
				return err
			}
		}

		if prize.IsURare {
			if points.URarePoints < prize.Cost {
				return ErrInsufficientURarePoints
			}
			points.URarePoints -= prize.Cost
		} else {
			if points.RarePoints < prize.Cost {
				return ErrInsufficientRarePoints
			}
			points.RarePoints -= prize.Cost
		}

		// 3. Create and add prize item instance to inventory
		_, inst, err := s.economy.GrantItem(txCtx, characterID, prize.ItemDefinitionID, 1)
		if err != nil {
			return err
		}

		if s.blackMarketRepo != nil {
			if err := s.blackMarketRepo.SaveCharacterPoints(txCtx, points); err != nil {
				return err
			}
		}

		result = &TradeResult{
			CharacterID:         char.ID,
			PrizeID:             prize.ID,
			ItemDefinitionID:    prize.ItemDefinitionID,
			ItemName:            prize.Name,
			InventoryInstanceID: inst.ID,
			Cost:                prize.Cost,
			IsURare:             prize.IsURare,
			RemainingRare:       points.RarePoints,
			RemainingURare:      points.URarePoints,
			Message:             "取引成立だ…。" + prize.Name + " を受け取った…",
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
