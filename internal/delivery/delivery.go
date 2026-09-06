package delivery

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

const (
	MaxActiveDeliveries = 3
	DefaultCourierFee   = 50
	DefaultQuestTTL     = 24 * time.Hour
)

type DeliveryStatus string

const (
	StatusInProgress DeliveryStatus = "in_progress"
	StatusCompleted  DeliveryStatus = "completed"
	StatusCancelled  DeliveryStatus = "cancelled"
)

type ParcelStatus string

const (
	ParcelStatusPending   ParcelStatus = "pending"
	ParcelStatusClaimed   ParcelStatus = "claimed"
	ParcelStatusCancelled ParcelStatus = "cancelled"
)

var (
	ErrNilDependency        = errors.New("delivery dependency is nil")
	ErrCharacterNotFound    = errors.New("character not found")
	ErrRecipientNotFound    = errors.New("recipient character not found")
	ErrQuestNotFound        = errors.New("delivery quest not found")
	ErrQuestExpired         = errors.New("delivery quest has expired")
	ErrMaxActiveDeliveries  = errors.New("maximum active deliveries limit reached")
	ErrAlreadyAccepted      = errors.New("delivery quest is already accepted and in progress")
	ErrDeliveryNotFound     = errors.New("character delivery not found")
	ErrDeliveryNotActive    = errors.New("character delivery is not in progress")
	ErrInsufficientItems    = errors.New("insufficient required items in inventory to complete delivery")
	ErrParcelNotFound       = errors.New("delivery parcel not found")
	ErrParcelAlreadyClaimed = errors.New("delivery parcel has already been claimed or cancelled")
	ErrSelfParcelNotAllowed = errors.New("cannot send a parcel to yourself")
	ErrInsufficientFunds    = errors.New("insufficient gold to send parcel including courier fee")
	ErrInvalidParcelPayload = errors.New("parcel must contain at least an item or a positive gold amount")
	ErrForbidden            = errors.New("access forbidden: character does not own this delivery or parcel")
	ErrInvalidInput         = errors.New("invalid delivery input parameters")
)

// Quest defines an item transport / courier task requested by a town resident.
type Quest struct {
	ID               string    `json:"id"`
	ClientName       string    `json:"client_name"`
	ClientMessage    string    `json:"client_message"`
	TargetItemID     string    `json:"target_item_id"`
	TargetItemName   string    `json:"target_item_name"`
	RequiredQuantity int       `json:"required_quantity"`
	RecipientName    string    `json:"recipient_name"`
	Destination      string    `json:"destination"`
	RewardGold       int       `json:"reward_gold"`
	RewardExp        int       `json:"reward_exp"`
	RewardItemID     string    `json:"reward_item_id,omitempty"`
	ExpiresAt        time.Time `json:"expires_at"`
	CreatedAt        time.Time `json:"created_at"`
}

// CharacterDelivery represents a character's accepted quest instance.
type CharacterDelivery struct {
	ID          string         `json:"id"`
	CharacterID string         `json:"character_id"`
	QuestID     string         `json:"quest_id"`
	Status      DeliveryStatus `json:"status"`
	AcceptedAt  time.Time      `json:"accepted_at"`
	CompletedAt *time.Time     `json:"completed_at,omitempty"`
	Quest       *Quest         `json:"quest,omitempty"`
}

// Parcel represents a player-to-player delivery package.
type Parcel struct {
	ID                   string       `json:"id"`
	SenderCharacterID    string       `json:"sender_character_id"`
	SenderCharacterName  string       `json:"sender_character_name"`
	RecipientCharacterID string       `json:"recipient_character_id"`
	ItemID               string       `json:"item_id,omitempty"`
	ItemName             string       `json:"item_name,omitempty"`
	ItemQuantity         int          `json:"item_quantity,omitempty"`
	GoldAmount           int          `json:"gold_amount,omitempty"`
	Message              string       `json:"message,omitempty"`
	CourierFee           int          `json:"courier_fee"`
	Status               ParcelStatus `json:"status"`
	CreatedAt            time.Time    `json:"created_at"`
	ClaimedAt            *time.Time   `json:"claimed_at,omitempty"`
}

// DeliveryCompletionResult represents the reward payload upon completing a delivery.
type DeliveryCompletionResult struct {
	DeliveryID     string `json:"delivery_id"`
	QuestID        string `json:"quest_id"`
	RewardedGold   int    `json:"rewarded_gold"`
	RewardedExp    int    `json:"rewarded_exp"`
	RewardedItemID string `json:"rewarded_item_id,omitempty"`
	CurrentGold    int    `json:"current_gold"`
	CurrentExp     int    `json:"current_exp"`
}

// SendParcelRequest specifies parcel contents and recipient.
type SendParcelRequest struct {
	RecipientCharacterID string `json:"recipient_character_id"`
	ItemInstanceID       string `json:"item_instance_id,omitempty"`
	ItemQuantity         int    `json:"item_quantity,omitempty"`
	GoldAmount           int    `json:"gold_amount,omitempty"`
	Message              string `json:"message,omitempty"`
}

// ParcelClaimResult contains the payload received from a parcel.
type ParcelClaimResult struct {
	ParcelID     string `json:"parcel_id"`
	SenderName   string `json:"sender_name"`
	ItemID       string `json:"item_id,omitempty"`
	ItemName     string `json:"item_name,omitempty"`
	ItemQuantity int    `json:"item_quantity,omitempty"`
	GoldAmount   int    `json:"gold_amount,omitempty"`
	CurrentGold  int    `json:"current_gold"`
}

// Repository Interfaces

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

type DeliveryRepository interface {
	GetAvailableQuests(ctx context.Context, now time.Time) ([]Quest, error)
	GetQuestByID(ctx context.Context, id string) (*Quest, error)
	SaveQuest(ctx context.Context, q *Quest) error
	SaveQuests(ctx context.Context, quests []Quest) error
	GetCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error)
	GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error)
	GetCharacterDeliveryByID(ctx context.Context, id string) (*CharacterDelivery, error)
	SaveCharacterDelivery(ctx context.Context, d *CharacterDelivery) error
	UpdateCharacterDelivery(ctx context.Context, d *CharacterDelivery) error
	SaveParcel(ctx context.Context, p *Parcel) error
	GetParcelByID(ctx context.Context, id string) (*Parcel, error)
	GetParcelByIDForUpdate(ctx context.Context, id string) (*Parcel, error)
	GetIncomingParcels(ctx context.Context, recipientCharacterID string) ([]Parcel, error)
	GetIncomingParcelsByCursor(ctx context.Context, recipientCharacterID string, limit int, beforeTime time.Time, beforeID string) ([]Parcel, error)
	GetSentParcels(ctx context.Context, senderCharacterID string) ([]Parcel, error)
	GetSentParcelsByCursor(ctx context.Context, senderCharacterID string, limit int, beforeTime time.Time, beforeID string) ([]Parcel, error)
	UpdateParcel(ctx context.Context, p *Parcel) error
}

type ItemDefinitionProvider = coreitem.DefinitionProvider

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type RandomSource interface {
	Intn(max int) (int, error)
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Intn(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	val, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(val.Int64()), nil
}

// Service represents the delivery quests and courier service.
type Service struct {
	repo         DeliveryRepository
	charRepo     CharacterRepository
	invRepo      InventoryRepository
	itemDefs     ItemDefinitionProvider
	txProvider   TransactionProvider
	randomSource RandomSource
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

func WithRandomSource(rnd RandomSource) Option {
	return func(s *Service) {
		s.randomSource = rnd
	}
}

func NewService(
	repo DeliveryRepository,
	charRepo CharacterRepository,
	invRepo InventoryRepository,
	opts ...Option,
) (*Service, error) {
	if repo == nil || charRepo == nil || invRepo == nil {
		return nil, ErrNilDependency
	}

	svc := &Service{
		repo:         repo,
		charRepo:     charRepo,
		invRepo:      invRepo,
		randomSource: cryptoRandomSource{},
	}

	for _, opt := range opts {
		opt(svc)
	}

	return svc, nil
}
