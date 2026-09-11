package gemstore

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

var (
	ErrNilDependency         = errors.New("gemstore dependency is nil")
	ErrGemNotFound           = errors.New("gem not found in catalog")
	ErrRecipeNotFound        = errors.New("recipe not found in catalog")
	ErrLevelTooLow           = errors.New("character job level too low to purchase gem")
	ErrInsufficientFunds     = errors.New("insufficient funds to purchase gem")
	ErrItemNotOwned          = errors.New("item not found in gem box")
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

// DepotRepository defines depot persistence methods if available for materials.
type DepotRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (depot.Depot, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, d depot.Depot) error
}

// ItemDefinitionProvider resolves item definitions by ID.
type ItemDefinitionProvider = coreitem.DefinitionProvider

// TransactionProvider executes work inside a transaction boundary.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// BuyResult represents the outcome of purchasing a gem.
type BuyResult struct {
	Character    corecharacter.Character `json:"character"`
	GemBox       GemBox                  `json:"gem_box"`
	Inventory    coreinventory.Inventory `json:"inventory,omitempty"`
	Gem          Gem                     `json:"gem"`
	Cost         int                     `json:"cost"`
	ItemInstance coreitem.Instance       `json:"item_instance"`
}

// SellResult represents the outcome of selling a gem.
type SellResult struct {
	Character corecharacter.Character `json:"character"`
	GemBox    GemBox                  `json:"gem_box"`
	Inventory coreinventory.Inventory `json:"inventory,omitempty"`
	Gem       Gem                     `json:"gem"`
	Payout    int                     `json:"payout"`
}

// SendResult represents the outcome of transferring a gem to another player character.
type SendResult struct {
	SenderCharacter    corecharacter.Character `json:"sender_character"`
	RecipientCharacter corecharacter.Character `json:"recipient_character"`
	SenderGemBox       GemBox                  `json:"sender_gem_box"`
	RecipientGemBox    GemBox                  `json:"recipient_gem_box"`
	Gem                Gem                     `json:"gem"`
}

// SynthesizeResult represents the outcome of synthesizing advanced gems from materials.
type SynthesizeResult struct {
	Character    corecharacter.Character `json:"character"`
	GemBox       GemBox                  `json:"gem_box"`
	Inventory    coreinventory.Inventory `json:"inventory,omitempty"`
	CreatedGem   Gem                     `json:"created_gem"`
	Recipe       Recipe                  `json:"recipe"`
	ItemInstance coreitem.Instance       `json:"item_instance"`
}

// AppraiseResult represents the outcome of appraising an item or unidentified orb.
type AppraiseResult struct {
	Character      corecharacter.Character `json:"character"`
	GemBox         GemBox                  `json:"gem_box"`
	Inventory      coreinventory.Inventory `json:"inventory,omitempty"`
	IsGem          bool                    `json:"is_gem"`
	IdentifiedGem  *Gem                    `json:"identified_gem,omitempty"`
	IdentifiedName string                  `json:"identified_name"`
	Message        string                  `json:"message"`
}
