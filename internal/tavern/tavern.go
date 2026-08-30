package tavern

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
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

const (
	NPCName      = "@エレナ"
	LocationName = "冒険者の酒場"
)

// Random dialogue list for the tavern barkeep NPC.
var barkeepDialogues = []string{
	"いらっしゃい！冒険者の酒場へようこそ。美味しいご飯と飲み物を用意してるわよ♪",
	"食材にはMPを回復させる魔法の聖水や、HPを回復させる新鮮な薬草が含まれているのよ。",
	"HPを回復させたいならボリューム満点のご飯やデザートを食べていくといいわ。",
	"MPを回復させたいなら香り高いコーヒーやハーブティーを飲んでいくといいわね。",
	"お腹がいっぱいのときは無理に食べちゃダメよ？冒険で身体を動かしてからまた来てね！",
	"冒険終わりに温かい食事を食べたいなら、事前に「でりばりー」を予約しておくと便利よ♪",
	"お酒は大人になってからね！未成年にはもぎたて果実ジュースがおすすめよ。",
}

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

// DeliveryReservation represents a reserved meal to be eaten upon adventure completion.
type DeliveryReservation struct {
	CharacterID string    `json:"character_id"`
	ItemID      string    `json:"item_id"`
	ItemName    string    `json:"item_name"`
	Price       int       `json:"price"`
	HPHeal      int       `json:"hp_heal"`
	MPHeal      int       `json:"mp_heal"`
	Tickets     int       `json:"tickets"`
	CreatedAt   time.Time `json:"created_at"`
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

// TalkResult represents a conversation response with the tavern barkeep.
type TalkResult struct {
	CharacterID  string `json:"character_id"`
	LocationName string `json:"location_name"`
	NPCName      string `json:"npc_name"`
	Message      string `json:"message"`
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

// OrderMeal executes an immediate meal purchase and consumption.
func (s *Service) OrderMeal(ctx context.Context, characterID string, itemID string) (OrderResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return OrderResult{}, ErrInvalidCharacterID
	}

	item, ok := s.catalog.GetItem(itemID)
	if !ok {
		return OrderResult{}, ErrMenuItemNotFound
	}

	var result OrderResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		char, err := s.charRepo.FindByIDForUpdate(txCtx, charID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		status, err := s.repo.GetCharacterStatus(txCtx, charID)
		if err != nil {
			status = TavernCharacterStatus{
				CharacterID: charID,
				IsFull:      false,
			}
		}

		if status.IsFull {
			return ErrAlreadyFull
		}

		if char.Money < item.Price {
			return ErrInsufficientFunds
		}

		// Calculate HP/MP healing
		hpHealed := 0
		if item.HPHeal > 0 {
			missingHP := char.Stats.MaxHP - char.Stats.HP
			if missingHP > 0 {
				if item.HPHeal > missingHP {
					hpHealed = missingHP
				} else {
					hpHealed = item.HPHeal
				}
				char.Stats.HP += hpHealed
			}
		}

		mpHealed := 0
		if item.MPHeal > 0 {
			missingMP := char.Stats.MaxMP - char.Stats.MP
			if missingMP > 0 {
				if item.MPHeal > missingMP {
					mpHealed = missingMP
				} else {
					mpHealed = item.MPHeal
				}
				char.Stats.MP += mpHealed
			}
		}

		char.Money -= item.Price

		if err := s.charRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character after meal: %w", err)
		}

		// Award lottery tickets
		totalTickets := 0
		if s.lotteryRepo != nil && item.Tickets > 0 {
			newTickets, err := s.lotteryRepo.AddRaffleTickets(txCtx, charID, item.Tickets)
			if err == nil {
				totalTickets = newTickets
			}
		}

		// Update tavern status
		now := time.Now().UTC()
		status.IsFull = true
		status.LastEatenAt = &now
		status.TotalMealsEaten++
		status.TotalGoldSpent += int64(item.Price)
		status.UpdatedAt = now

		if err := s.repo.UpsertCharacterStatus(txCtx, status); err != nil {
			return fmt.Errorf("failed to update tavern character status: %w", err)
		}

		msg := fmt.Sprintf("おまたせ、%sよ♪ 美味しく召し上がれ！", item.Name)
		if hpHealed > 0 && mpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d、MPが%d回復した！", hpHealed, mpHealed)
		} else if hpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d回復した！", hpHealed)
		} else if mpHealed > 0 {
			msg += fmt.Sprintf(" MPが%d回復した！", mpHealed)
		}
		if item.Tickets > 0 {
			msg += fmt.Sprintf(" 福引券を%d枚もらった！", item.Tickets)
		}

		result = OrderResult{
			CharacterID:    char.ID,
			Item:           item,
			HPHealed:       hpHealed,
			MPHealed:       mpHealed,
			CurrentHP:      char.Stats.HP,
			CurrentMP:      char.Stats.MP,
			RemainingGold:  char.Money,
			TicketsAwarded: item.Tickets,
			TotalTickets:   totalTickets,
			Message:        msg,
		}
		return nil
	})

	if err != nil {
		return OrderResult{}, err
	}
	return result, nil
}

// ReserveDelivery sets a pending delivery order.
func (s *Service) ReserveDelivery(ctx context.Context, characterID string, itemID string) (DeliveryReservation, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return DeliveryReservation{}, ErrInvalidCharacterID
	}

	item, ok := s.catalog.GetItem(itemID)
	if !ok {
		return DeliveryReservation{}, ErrMenuItemNotFound
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return DeliveryReservation{}, ErrCharacterNotFound
		}
		return DeliveryReservation{}, err
	}

	if char.Money < item.Price {
		return DeliveryReservation{}, ErrInsufficientFunds
	}

	delivery := DeliveryReservation{
		CharacterID: charID,
		ItemID:      item.ID,
		ItemName:    item.Name,
		Price:       item.Price,
		HPHeal:      item.HPHeal,
		MPHeal:      item.MPHeal,
		Tickets:     item.Tickets,
		CreatedAt:   time.Now().UTC(),
	}

	if err := s.repo.SaveDelivery(ctx, delivery); err != nil {
		return DeliveryReservation{}, err
	}
	return delivery, nil
}

// GetDelivery returns the current delivery reservation for a character.
func (s *Service) GetDelivery(ctx context.Context, characterID string) (DeliveryReservation, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return DeliveryReservation{}, ErrInvalidCharacterID
	}

	delivery, err := s.repo.GetDelivery(ctx, charID)
	if err != nil {
		return DeliveryReservation{}, ErrNoActiveDelivery
	}
	return delivery, nil
}

// CancelDelivery cancels the active delivery reservation.
func (s *Service) CancelDelivery(ctx context.Context, characterID string) error {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return ErrInvalidCharacterID
	}

	_, err := s.repo.GetDelivery(ctx, charID)
	if err != nil {
		return ErrNoActiveDelivery
	}

	return s.repo.DeleteDelivery(ctx, charID)
}

// ClaimDelivery consumes the active delivery meal (e.g. after adventure).
func (s *Service) ClaimDelivery(ctx context.Context, characterID string) (OrderResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return OrderResult{}, ErrInvalidCharacterID
	}

	var result OrderResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		delivery, err := s.repo.GetDelivery(txCtx, charID)
		if err != nil || delivery.ItemID == "" {
			return ErrNoActiveDelivery
		}

		item, ok := s.catalog.GetItem(delivery.ItemID)
		if !ok {
			item = MenuItem{
				ID:       delivery.ItemID,
				Name:     delivery.ItemName,
				Price:    delivery.Price,
				HPHeal:   delivery.HPHeal,
				MPHeal:   delivery.MPHeal,
				Tickets:  delivery.Tickets,
				Category: "Delivery",
			}
		}

		char, err := s.charRepo.FindByIDForUpdate(txCtx, charID)
		if err != nil {
			if errors.Is(err, corecharacter.ErrNotFound) {
				return ErrCharacterNotFound
			}
			return err
		}

		if char.Money < delivery.Price {
			return ErrInsufficientFunds
		}

		// Calculate HP/MP healing
		hpHealed := 0
		if delivery.HPHeal > 0 {
			missingHP := char.Stats.MaxHP - char.Stats.HP
			if missingHP > 0 {
				if delivery.HPHeal > missingHP {
					hpHealed = missingHP
				} else {
					hpHealed = delivery.HPHeal
				}
				char.Stats.HP += hpHealed
			}
		}

		mpHealed := 0
		if delivery.MPHeal > 0 {
			missingMP := char.Stats.MaxMP - char.Stats.MP
			if missingMP > 0 {
				if delivery.MPHeal > missingMP {
					mpHealed = missingMP
				} else {
					mpHealed = delivery.MPHeal
				}
				char.Stats.MP += mpHealed
			}
		}

		char.Money -= delivery.Price

		if err := s.charRepo.Update(txCtx, char); err != nil {
			return fmt.Errorf("failed to update character after delivery meal: %w", err)
		}

		// Award lottery tickets
		totalTickets := 0
		if s.lotteryRepo != nil && delivery.Tickets > 0 {
			newTickets, err := s.lotteryRepo.AddRaffleTickets(txCtx, charID, delivery.Tickets)
			if err == nil {
				totalTickets = newTickets
			}
		}

		// Update tavern status
		status, err := s.repo.GetCharacterStatus(txCtx, charID)
		if err != nil {
			status = TavernCharacterStatus{CharacterID: charID}
		}
		now := time.Now().UTC()
		status.IsFull = true
		status.LastEatenAt = &now
		status.TotalMealsEaten++
		status.TotalGoldSpent += int64(delivery.Price)
		status.UpdatedAt = now

		if err := s.repo.UpsertCharacterStatus(txCtx, status); err != nil {
			return fmt.Errorf("failed to update tavern character status: %w", err)
		}

		if err := s.repo.DeleteDelivery(txCtx, charID); err != nil {
			return fmt.Errorf("failed to delete claimed delivery: %w", err)
		}

		msg := fmt.Sprintf("配達完了！冒険帰りの体に染み渡る%sが届いたわ♪", delivery.ItemName)
		if hpHealed > 0 && mpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d、MPが%d回復した！", hpHealed, mpHealed)
		} else if hpHealed > 0 {
			msg += fmt.Sprintf(" HPが%d回復した！", hpHealed)
		} else if mpHealed > 0 {
			msg += fmt.Sprintf(" MPが%d回復した！", mpHealed)
		}
		if delivery.Tickets > 0 {
			msg += fmt.Sprintf(" 福引券を%d枚もらった！", delivery.Tickets)
		}

		result = OrderResult{
			CharacterID:    char.ID,
			Item:           item,
			HPHealed:       hpHealed,
			MPHealed:       mpHealed,
			CurrentHP:      char.Stats.HP,
			CurrentMP:      char.Stats.MP,
			RemainingGold:  char.Money,
			TicketsAwarded: delivery.Tickets,
			TotalTickets:   totalTickets,
			Message:        msg,
		}
		return nil
	})

	if err != nil {
		return OrderResult{}, err
	}
	return result, nil
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

// Talk returns a randomized friendly greeting and advice from the tavern barkeep.
func (s *Service) Talk(ctx context.Context, characterID string) (TalkResult, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return TalkResult{}, ErrInvalidCharacterID
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return TalkResult{}, ErrCharacterNotFound
		}
		return TalkResult{}, err
	}

	n, err := rand.Int(rand.Reader, big.NewInt(int64(len(barkeepDialogues))))
	var msg string
	if err != nil {
		msg = barkeepDialogues[0]
	} else {
		msg = barkeepDialogues[n.Int64()]
	}

	// Personalize if character name exists
	if char.Name != "" {
		msg = strings.ReplaceAll(msg, "いらっしゃい！", fmt.Sprintf("いらっしゃい、%sさん！", char.Name))
	}

	return TalkResult{
		CharacterID:  char.ID,
		LocationName: LocationName,
		NPCName:      NPCName,
		Message:      msg,
	}, nil
}
