package tavern

import (
	"context"
	"errors"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// TransactionProvider executes a callback within a database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

var (
	ErrInvalidCharacterID     = errors.New("invalid character ID")
	ErrCharacterNotFound      = errors.New("character not found")
	ErrMenuItemNotFound       = errors.New("menu item not found")
	ErrInsufficientFunds      = errors.New("insufficient gold to purchase tavern item")
	ErrAlreadyFull            = errors.New("character is already full; go on an adventure to digest your meal")
	ErrNoActiveDelivery       = errors.New("no active delivery reservation found")
	ErrDeliveryAlreadyExists  = errors.New("delivery reservation already exists")
	ErrNilRepository          = errors.New("tavern repository is required")
	ErrNilCatalog             = errors.New("tavern catalog is required")
	ErrNilCharacterRepository = errors.New("character repository is required")
	ErrNilTxProvider          = errors.New("transaction provider is required")
)

// CharacterRepository defines required character persistence operations.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, character corecharacter.Character) error
}

// LotteryRepository defines optional lottery ticket award operations.
type LotteryRepository interface {
	AddRaffleTickets(ctx context.Context, characterID string, count int) (int, error)
	GetRaffleTickets(ctx context.Context, characterID string) (int, error)
}

// TavernCharacterStatus represents the character's eating/fullness state in the tavern.
type TavernCharacterStatus struct {
	CharacterID     string     `json:"character_id"`
	IsFull          bool       `json:"is_full"`
	LastEatenAt     *time.Time `json:"last_eaten_at,omitempty"`
	TotalMealsEaten int        `json:"total_meals_eaten"`
	TotalGoldSpent  int64      `json:"total_gold_spent"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// TavernStatus represents the full tavern status presented to the character.
type TavernStatus struct {
	CharacterID         string               `json:"character_id"`
	CharacterName       string               `json:"character_name"`
	LocationName        string               `json:"location_name"`
	NPCName             string               `json:"npc_name"`
	Gold                int                  `json:"gold"`
	HP                  int                  `json:"hp"`
	MaxHP               int                  `json:"max_hp"`
	MP                  int                  `json:"mp"`
	MaxMP               int                  `json:"max_mp"`
	IsFull              bool                 `json:"is_full"`
	DeliveryReservation *DeliveryReservation `json:"delivery_reservation,omitempty"`
}

// OrderResult represents the result of eating a meal or claiming a delivery.
type OrderResult struct {
	CharacterID    string   `json:"character_id"`
	Item           MenuItem `json:"item"`
	HPHealed       int      `json:"hp_healed"`
	MPHealed       int      `json:"mp_healed"`
	CurrentHP      int      `json:"current_hp"`
	CurrentMP      int      `json:"current_mp"`
	RemainingGold  int      `json:"remaining_gold"`
	TicketsAwarded int      `json:"tickets_awarded"`
	TotalTickets   int      `json:"total_tickets"`
	Message        string   `json:"message"`
}

// Repository defines storage operations for tavern deliveries and character statuses.
type Repository interface {
	GetCharacterStatus(ctx context.Context, characterID string) (TavernCharacterStatus, error)
	UpsertCharacterStatus(ctx context.Context, status TavernCharacterStatus) error
	GetDelivery(ctx context.Context, characterID string) (DeliveryReservation, error)
	SaveDelivery(ctx context.Context, delivery DeliveryReservation) error
	DeleteDelivery(ctx context.Context, characterID string) error
}

// Service provides tavern business logic.
type Service struct {
	catalog     *Catalog
	repo        Repository
	charRepo    CharacterRepository
	lotteryRepo LotteryRepository
	txProvider  TransactionProvider
}

// Option configures the tavern Service.
type Option func(*Service)

// WithLotteryRepository sets the optional lottery repository for ticket rewards.
func WithLotteryRepository(lr LotteryRepository) Option {
	return func(s *Service) {
		s.lotteryRepo = lr
	}
}

// NewService creates a new Tavern service.
func NewService(
	catalog *Catalog,
	repo Repository,
	charRepo CharacterRepository,
	txProvider TransactionProvider,
	opts ...Option,
) (*Service, error) {
	if catalog == nil {
		return nil, ErrNilCatalog
	}
	if repo == nil {
		return nil, ErrNilRepository
	}
	if charRepo == nil {
		return nil, ErrNilCharacterRepository
	}
	if txProvider == nil {
		return nil, ErrNilTxProvider
	}

	s := &Service{
		catalog:    catalog,
		repo:       repo,
		charRepo:   charRepo,
		txProvider: txProvider,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// GetMenu returns all available menu items in the catalog.
func (s *Service) GetMenu() []MenuItem {
	return s.catalog.Items()
}

// GetStatus returns the tavern status for a character.
func (s *Service) GetStatus(ctx context.Context, characterID string) (TavernStatus, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return TavernStatus{}, ErrInvalidCharacterID
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return TavernStatus{}, ErrCharacterNotFound
		}
		return TavernStatus{}, err
	}

	status, err := s.repo.GetCharacterStatus(ctx, charID)
	if err != nil {
		// Non-fatal if no status yet: defaults to not full
		status = TavernCharacterStatus{
			CharacterID: charID,
			IsFull:      false,
		}
	}

	var deliveryPtr *DeliveryReservation
	delivery, err := s.repo.GetDelivery(ctx, charID)
	if err == nil && delivery.ItemID != "" {
		deliveryPtr = &delivery
	}

	return TavernStatus{
		CharacterID:         char.ID,
		CharacterName:       char.Name,
		LocationName:        LocationName,
		NPCName:             NPCName,
		Gold:                char.Money,
		HP:                  char.Stats.HP,
		MaxHP:               char.Stats.MaxHP,
		MP:                  char.Stats.MP,
		MaxMP:               char.Stats.MaxMP,
		IsFull:              status.IsFull,
		DeliveryReservation: deliveryPtr,
	}, nil
}

// ResetFullness clears the fullness state for a character (e.g. after adventure or sleep).
func (s *Service) ResetFullness(ctx context.Context, characterID string) error {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return ErrInvalidCharacterID
	}

	status, err := s.repo.GetCharacterStatus(ctx, charID)
	if err != nil {
		status = TavernCharacterStatus{
			CharacterID: charID,
		}
	}
	status.IsFull = false
	status.UpdatedAt = time.Now().UTC()
	return s.repo.UpsertCharacterStatus(ctx, status)
}
