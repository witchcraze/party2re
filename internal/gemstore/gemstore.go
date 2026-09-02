package gemstore

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
)

var (
	ErrNilDependency         = errors.New("gemstore dependency is nil")
	ErrGemNotFound           = errors.New("gem not found in catalog")
	ErrRecipeNotFound        = errors.New("recipe not found in catalog")
	ErrLevelTooLow           = errors.New("character level too low to purchase gem")
	ErrInsufficientFunds     = errors.New("insufficient funds to purchase gem")
	ErrItemNotOwned          = errors.New("item not found in inventory")
	ErrCannotSendToSelf      = errors.New("cannot send gem to self")
	ErrInsufficientMaterials = errors.New("insufficient materials for gem synthesis")
	ErrInvalidCharacterID    = errors.New("invalid character ID")
	ErrInvalidGemID          = errors.New("invalid gem ID")
	ErrInvalidRecipeID       = errors.New("invalid recipe ID")
)

// ShopPriceMultiplier is the shop price multiplier applied to base gem price (legacy standard: 5x).
const ShopPriceMultiplier = 5

// RandomSource provides random integer generation for orb appraisals and RNG mechanics.
type RandomSource interface {
	Intn(n int) (int, error)
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Intn(n int) (int, error) {
	if n <= 0 {
		return 0, nil
	}
	val, err := rand.Int(rand.Reader, big.NewInt(int64(n)))
	if err != nil {
		return 0, err
	}
	return int(val.Int64()), nil
}

// DefaultRandomSource returns the default crypto-secure random source.
func DefaultRandomSource() RandomSource {
	return cryptoRandomSource{}
}

// CharacterRepository defines character persistence methods required by gemstore.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// InventoryRepository defines inventory persistence methods required by gemstore.
type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inventory coreinventory.Inventory) error
}

// ItemDefinitionProvider resolves item definitions by ID.
type ItemDefinitionProvider = coreitem.DefinitionProvider

// TransactionProvider executes work inside a transaction boundary.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

// WithItemDefinitionProvider configures the item definition catalog.
func WithItemDefinitionProvider(provider ItemDefinitionProvider) Option {
	return func(s *Service) {
		s.items = provider
	}
}

// WithTransactionProvider configures the transaction provider.
func WithTransactionProvider(provider TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = provider
	}
}

// WithRandomSource configures an explicit random source (useful for deterministic tests).
func WithRandomSource(r RandomSource) Option {
	return func(s *Service) {
		s.randomSource = r
	}
}

// Service implements all gem store operations and domain invariants.
type Service struct {
	catalog      *Catalog
	characters   CharacterRepository
	inventories  InventoryRepository
	items        ItemDefinitionProvider
	txProvider   TransactionProvider
	randomSource RandomSource
}

// NewService creates a new GemStore service instance.
func NewService(
	catalog *Catalog,
	characters CharacterRepository,
	inventories InventoryRepository,
	opts ...Option,
) (*Service, error) {
	if catalog == nil || characters == nil || inventories == nil {
		return nil, ErrNilDependency
	}

	s := &Service{
		catalog:      catalog,
		characters:   characters,
		inventories:  inventories,
		randomSource: DefaultRandomSource(),
	}

	for _, opt := range opts {
		opt(s)
	}

	return s, nil
}

// BuyResult represents the outcome of purchasing a gem.
type BuyResult struct {
	Character    corecharacter.Character `json:"character"`
	Inventory    coreinventory.Inventory `json:"inventory"`
	Gem          Gem                     `json:"gem"`
	Cost         int                     `json:"cost"`
	ItemInstance coreitem.Instance       `json:"item_instance"`
}

// SellResult represents the outcome of selling a gem.
type SellResult struct {
	Character corecharacter.Character `json:"character"`
	Inventory coreinventory.Inventory `json:"inventory"`
	Gem       Gem                     `json:"gem"`
	Payout    int                     `json:"payout"`
}

// SendResult represents the outcome of transferring a gem to another player character.
type SendResult struct {
	SenderCharacter    corecharacter.Character `json:"sender_character"`
	RecipientCharacter corecharacter.Character `json:"recipient_character"`
	Gem                Gem                     `json:"gem"`
}

// SynthesizeResult represents the outcome of synthesizing advanced gems from materials.
type SynthesizeResult struct {
	Character    corecharacter.Character `json:"character"`
	Inventory    coreinventory.Inventory `json:"inventory"`
	CreatedGem   Gem                     `json:"created_gem"`
	Recipe       Recipe                  `json:"recipe"`
	ItemInstance coreitem.Instance       `json:"item_instance"`
}

// AppraiseResult represents the outcome of appraising an item or unidentified orb.
type AppraiseResult struct {
	Character      corecharacter.Character `json:"character"`
	Inventory      coreinventory.Inventory `json:"inventory"`
	IsGem          bool                    `json:"is_gem"`
	IdentifiedGem  *Gem                    `json:"identified_gem,omitempty"`
	IdentifiedName string                  `json:"identified_name"`
	Message        string                  `json:"message"`
}

// BuyGem purchases a gem from the gem store if character has sufficient level and gold.
func (s *Service) BuyGem(ctx context.Context, characterID, gemID string) (BuyResult, error) {
	characterID = strings.TrimSpace(characterID)
	gemID = strings.TrimSpace(gemID)
	if characterID == "" {
		return BuyResult{}, ErrInvalidCharacterID
	}
	if gemID == "" {
		return BuyResult{}, ErrInvalidGemID
	}

	gem, ok := s.catalog.FindGemByID(gemID)
	if !ok {
		// Fallback lookup by name
		gem, ok = s.catalog.FindGemByName(gemID)
		if !ok {
			return BuyResult{}, ErrGemNotFound
		}
	}

	price := gem.Price * ShopPriceMultiplier

	var res BuyResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		if char.Level < gem.RequiredLevel && char.RebirthCount == 0 {
			return ErrLevelTooLow
		}

		if char.Money < price {
			return ErrInsufficientFunds
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		instance, err := coreitem.NewInstance(gem.ID, 1)
		if err != nil {
			return err
		}

		if err := inv.Add(instance); err != nil {
			return err
		}

		if err := char.DeductMoney(price); err != nil {
			return ErrInsufficientFunds
		}

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		res = BuyResult{
			Character:    char,
			Inventory:    inv,
			Gem:          gem,
			Cost:         price,
			ItemInstance: instance,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return BuyResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return BuyResult{}, err
		}
	}

	return res, nil
}

// SellGem sells a gem from the character's inventory for 50% of its base price.
func (s *Service) SellGem(ctx context.Context, characterID, itemInstanceOrDefID string) (SellResult, error) {
	characterID = strings.TrimSpace(characterID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)
	if characterID == "" {
		return SellResult{}, ErrInvalidCharacterID
	}
	if itemInstanceOrDefID == "" {
		return SellResult{}, ErrInvalidGemID
	}

	var res SellResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		targetItem, ok := findItemInInventory(inv, itemInstanceOrDefID, s.catalog, s.items)
		if !ok {
			return ErrItemNotOwned
		}

		gem, ok := s.catalog.FindGemByID(targetItem.DefinitionID)
		if !ok {
			// Lookup by name
			itemName := resolveItemName(targetItem.DefinitionID, s.catalog, s.items)
			gem, ok = s.catalog.FindGemByName(itemName)
			if !ok {
				return ErrGemNotFound
			}
		}

		sellPrice := int(float64(gem.Price) * 0.5)
		if sellPrice < 1 {
			sellPrice = 1
		}

		if err := inv.Consume(targetItem.ID, 1); err != nil {
			return err
		}

		_ = char.AddMoney(sellPrice)

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		res = SellResult{
			Character: char,
			Inventory: inv,
			Gem:       gem,
			Payout:    sellPrice,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SellResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SellResult{}, err
		}
	}

	return res, nil
}

// SendGem transfers a gem from sender inventory to recipient inventory with deterministic deadlock-free locking.
func (s *Service) SendGem(ctx context.Context, senderID, recipientID, itemInstanceOrDefID string) (SendResult, error) {
	senderID = strings.TrimSpace(senderID)
	recipientID = strings.TrimSpace(recipientID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)

	if senderID == "" || recipientID == "" {
		return SendResult{}, ErrInvalidCharacterID
	}
	if senderID == recipientID {
		return SendResult{}, ErrCannotSendToSelf
	}
	if itemInstanceOrDefID == "" {
		return SendResult{}, ErrInvalidGemID
	}

	firstID, secondID := senderID, recipientID
	if firstID > secondID {
		firstID, secondID = recipientID, senderID
	}

	var res SendResult
	run := func(txCtx context.Context) error {
		char1, err := s.characters.FindByIDForUpdate(txCtx, firstID)
		if err != nil {
			return err
		}
		char2, err := s.characters.FindByIDForUpdate(txCtx, secondID)
		if err != nil {
			return err
		}

		senderChar := char1
		recipientChar := char2
		if senderID == secondID {
			senderChar = char2
			recipientChar = char1
		}

		senderInv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, senderID)
		if err != nil {
			return err
		}
		recipientInv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, recipientID)
		if err != nil {
			return err
		}

		targetItem, ok := findItemInInventory(senderInv, itemInstanceOrDefID, s.catalog, s.items)
		if !ok {
			return ErrItemNotOwned
		}

		gem, ok := s.catalog.FindGemByID(targetItem.DefinitionID)
		if !ok {
			itemName := resolveItemName(targetItem.DefinitionID, s.catalog, s.items)
			gem, ok = s.catalog.FindGemByName(itemName)
			if !ok {
				return ErrGemNotFound
			}
		}

		if err := senderInv.Consume(targetItem.ID, 1); err != nil {
			return err
		}

		transferInstance, err := coreitem.NewInstance(gem.ID, 1)
		if err != nil {
			return err
		}

		if err := recipientInv.Add(transferInstance); err != nil {
			return err
		}

		if err := s.inventories.Save(txCtx, senderInv); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, recipientInv); err != nil {
			return err
		}

		res = SendResult{
			SenderCharacter:    senderChar,
			RecipientCharacter: recipientChar,
			Gem:                gem,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SendResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SendResult{}, err
		}
	}

	return res, nil
}

// SynthesizeGem synthesizes two ingredient items/gems into an advanced gem.
func (s *Service) SynthesizeGem(ctx context.Context, characterID, recipeID string) (SynthesizeResult, error) {
	characterID = strings.TrimSpace(characterID)
	recipeID = strings.TrimSpace(recipeID)
	if characterID == "" {
		return SynthesizeResult{}, ErrInvalidCharacterID
	}
	if recipeID == "" {
		return SynthesizeResult{}, ErrInvalidRecipeID
	}

	recipe, ok := s.catalog.FindRecipeByID(recipeID)
	if !ok {
		return SynthesizeResult{}, ErrRecipeNotFound
	}

	resultGem, ok := s.catalog.FindGemByName(recipe.ResultName)
	if !ok {
		resultGem, ok = s.catalog.FindGemByID(recipe.ResultName)
		if !ok {
			return SynthesizeResult{}, ErrGemNotFound
		}
	}

	var res SynthesizeResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		mat1Item, ok1 := findMaterialInInventory(inv, recipe.Material1, s.catalog, s.items)
		if !ok1 {
			return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material1)
		}

		if err := inv.Consume(mat1Item.ID, 1); err != nil {
			return err
		}

		mat2Item, ok2 := findMaterialInInventory(inv, recipe.Material2, s.catalog, s.items)
		if !ok2 {
			return fmt.Errorf("%w: missing %s", ErrInsufficientMaterials, recipe.Material2)
		}

		if err := inv.Consume(mat2Item.ID, 1); err != nil {
			return err
		}

		newInstance, err := coreitem.NewInstance(resultGem.ID, 1)
		if err != nil {
			return err
		}

		if err := inv.Add(newInstance); err != nil {
			return err
		}

		if err := s.inventories.Save(txCtx, inv); err != nil {
			return err
		}

		res = SynthesizeResult{
			Character:    char,
			Inventory:    inv,
			CreatedGem:   resultGem,
			Recipe:       recipe,
			ItemInstance: newInstance,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return SynthesizeResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return SynthesizeResult{}, err
		}
	}

	return res, nil
}

// AppraiseItem appraises an unidentified orb or equipment in character inventory.
func (s *Service) AppraiseItem(ctx context.Context, characterID, itemInstanceOrDefID string) (AppraiseResult, error) {
	characterID = strings.TrimSpace(characterID)
	itemInstanceOrDefID = strings.TrimSpace(itemInstanceOrDefID)
	if characterID == "" {
		return AppraiseResult{}, ErrInvalidCharacterID
	}
	if itemInstanceOrDefID == "" {
		return AppraiseResult{}, ErrItemNotOwned
	}

	var res AppraiseResult
	run := func(txCtx context.Context) error {
		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		targetItem, ok := findItemInInventory(inv, itemInstanceOrDefID, s.catalog, s.items)
		if !ok {
			return ErrItemNotOwned
		}

		itemName := resolveItemName(targetItem.DefinitionID, s.catalog, s.items)

		// Check if it's an unidentified orb that can be appraised into a gem
		if gem, isUnidentified, err := s.catalog.AppraiseUnidentifiedItem(itemName, s.randomSource); err != nil {
			return err
		} else if isUnidentified {
			if err := inv.Consume(targetItem.ID, 1); err != nil {
				return err
			}

			gemInstance, err := coreitem.NewInstance(gem.ID, 1)
			if err != nil {
				return err
			}

			if err := inv.Add(gemInstance); err != nil {
				return err
			}

			if err := s.inventories.Save(txCtx, inv); err != nil {
				return err
			}

			res = AppraiseResult{
				Character:      char,
				Inventory:      inv,
				IsGem:          true,
				IdentifiedGem:  &gem,
				IdentifiedName: gem.Name,
				Message:        fmt.Sprintf("これは… %sですね。宝石箱（インベントリ）に入れておきました", gem.Name),
			}
			return nil
		}

		// Otherwise, it's standard identified equipment/item
		res = AppraiseResult{
			Character:      char,
			Inventory:      inv,
			IsGem:          false,
			IdentifiedName: itemName,
			Message:        fmt.Sprintf("これは… %sですね", itemName),
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return AppraiseResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return AppraiseResult{}, err
		}
	}

	return res, nil
}

// GetCatalog returns the gem catalog available for purchase at the specified character level.
func (s *Service) GetCatalog(level int) []Gem {
	return s.catalog.GetGemsForLevel(level)
}

// GetRecipes returns all known gem synthesis recipes.
func (s *Service) GetRecipes() []Recipe {
	return s.catalog.AllRecipes()
}

// GetDialogue returns shopkeeper @ジェマ dialogue lines.
func (s *Service) GetDialogue() []string {
	return []string{
		"ここは宝石店です。色んな宝石を売っていますよ",
		"どんな宝石がお好きですか？",
		"アイテムや宝石を使って、新しい宝石に加工することもできますよ",
		"宝石ごとに不思議な力があるんです",
		"あなたにはキラリと光る素敵な宝石がお似合いですね",
	}
}

// -------------------------------------------------------------------
// Helper functions
// -------------------------------------------------------------------

func findItemInInventory(
	inv coreinventory.Inventory,
	target string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, bool) {
	for _, inst := range inv.Items {
		if inst.ID == target || inst.DefinitionID == target {
			return inst, true
		}
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if name == target {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

func findMaterialInInventory(
	inv coreinventory.Inventory,
	matName string,
	catalog *Catalog,
	items ItemDefinitionProvider,
) (coreitem.Instance, bool) {
	for _, inst := range inv.Items {
		name := resolveItemName(inst.DefinitionID, catalog, items)
		if name == matName || inst.DefinitionID == matName {
			return inst, true
		}
	}
	return coreitem.Instance{}, false
}

func resolveItemName(defID string, catalog *Catalog, items ItemDefinitionProvider) string {
	if g, ok := catalog.FindGemByID(defID); ok {
		return g.Name
	}
	if items != nil {
		if d, err := items.FindByID(defID); err == nil {
			return d.Name
		}
	}
	return defID
}
