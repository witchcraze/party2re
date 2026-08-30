package delivery

import (
	"context"
	"crypto/rand"
	"errors"
	"math/big"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
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
	GetIncomingParcels(ctx context.Context, recipientCharacterID string) ([]Parcel, error)
	GetSentParcels(ctx context.Context, senderCharacterID string) ([]Parcel, error)
	UpdateParcel(ctx context.Context, p *Parcel) error
}

type ItemDefinitionProvider interface {
	FindByID(id string) (coreitem.Definition, error)
}

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

type QuestTemplate struct {
	ClientName    string
	ClientMessage string
	TargetItemID  string
	TargetName    string
	Quantity      int
	RecipientName string
	Destination   string
	RewardGold    int
	RewardExp     int
	RewardItemID  string
}

var SeedQuestTemplates = []QuestTemplate{
	{
		ClientName:    "薬草師のミレイユ",
		ClientMessage: "調合用の薬草が切れて困っています。至急届けてください！",
		TargetItemID:  "item-001",
		TargetName:    "薬草",
		Quantity:      3,
		RecipientName: "見習い調合師",
		Destination:   "薬草研究所",
		RewardGold:    180,
		RewardExp:     90,
		RewardItemID:  "item-007", // 毒消し草
	},
	{
		ClientName:    "鍛冶屋のトバル",
		ClientMessage: "関所の見張り兵から頼まれていた武器だ。届けてやってくれ。",
		TargetItemID:  "weapon-01",
		TargetName:    "ヒノキの棒",
		Quantity:      2,
		RecipientName: "見張りの衛兵",
		Destination:   "西の関所",
		RewardGold:    250,
		RewardExp:     120,
		RewardItemID:  "",
	},
	{
		ClientName:    "教会のシスター・アンナ",
		ClientMessage: "巡回神父様へ聖水をお届けいただけますでしょうか。",
		TargetItemID:  "item-011",
		TargetName:    "聖水",
		Quantity:      2,
		RecipientName: "巡回神父",
		Destination:   "北の礼拝堂",
		RewardGold:    320,
		RewardExp:     160,
		RewardItemID:  "item-008", // 満月草
	},
	{
		ClientName:    "酒場の看板娘エレナ",
		ClientMessage: "砦の守備隊長さんへ特製弁当の差し入れをお願いね！",
		TargetItemID:  "item-002",
		TargetName:    "上薬草",
		Quantity:      2,
		RecipientName: "守備隊長ロベルト",
		Destination:   "北の砦",
		RewardGold:    350,
		RewardExp:     200,
		RewardItemID:  "item-012", // キメラの翼
	},
	{
		ClientName:    "魔法学校の教授バルツ",
		ClientMessage: "魔術実験のための素材が必要です。至急手配をお願いしたい。",
		TargetItemID:  "item-009",
		TargetName:    "目覚まし草",
		Quantity:      2,
		RecipientName: "研究室の助手",
		Destination:   "魔術図書館",
		RewardGold:    400,
		RewardExp:     220,
		RewardItemID:  "item-010", // 天使のすず
	},
	{
		ClientName:    "城下町の豪商ポルテ",
		ClientMessage: "東の港に停泊している船長へ、装備品の納品を頼む。",
		TargetItemID:  "armor-01",
		TargetName:    "布の服",
		Quantity:      2,
		RecipientName: "貿易船の船長",
		Destination:   "東の港町",
		RewardGold:    450,
		RewardExp:     250,
		RewardItemID:  "",
	},
	{
		ClientName:    "森の狩人ガレック",
		ClientMessage: "山小屋の隠者へ毒消し草を届けてやってくれ。急ぎだ。",
		TargetItemID:  "item-007",
		TargetName:    "毒消し草",
		Quantity:      3,
		RecipientName: "山小屋の隠者",
		Destination:   "迷いの森深部",
		RewardGold:    380,
		RewardExp:     190,
		RewardItemID:  "item-002", // 上薬草
	},
	{
		ClientName:    "防具職人グレゴリー",
		ClientMessage: "新米衛兵用の防具一式を詰所まで運んでほしい。",
		TargetItemID:  "armor-03",
		TargetName:    "皮の鎧",
		Quantity:      1,
		RecipientName: "新人衛兵",
		Destination:   "衛兵詰所",
		RewardGold:    500,
		RewardExp:     280,
		RewardItemID:  "",
	},
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

// GenerateQuests creates a list of fresh delivery quests.
func (s *Service) GenerateQuests(count int, now time.Time) []Quest {
	if count <= 0 {
		count = 5
	}
	var quests []Quest
	for i := 0; i < count; i++ {
		idx := i % len(SeedQuestTemplates)
		if s.randomSource != nil {
			rIdx, err := s.randomSource.Intn(len(SeedQuestTemplates))
			if err == nil {
				idx = rIdx
			}
		}
		tmpl := SeedQuestTemplates[idx]
		quests = append(quests, Quest{
			ID:               id.New(),
			ClientName:       tmpl.ClientName,
			ClientMessage:    tmpl.ClientMessage,
			TargetItemID:     tmpl.TargetItemID,
			TargetItemName:   tmpl.TargetName,
			RequiredQuantity: tmpl.Quantity,
			RecipientName:    tmpl.RecipientName,
			Destination:      tmpl.Destination,
			RewardGold:       tmpl.RewardGold,
			RewardExp:        tmpl.RewardExp,
			RewardItemID:     tmpl.RewardItemID,
			ExpiresAt:        now.Add(DefaultQuestTTL),
			CreatedAt:        now,
		})
	}
	return quests
}

// GetAvailableQuests returns all active non-expired quests, auto-generating if fewer than 5.
func (s *Service) GetAvailableQuests(ctx context.Context, now time.Time) ([]Quest, error) {
	quests, err := s.repo.GetAvailableQuests(ctx, now)
	if err != nil {
		return nil, err
	}

	if len(quests) < 5 {
		needed := 5 - len(quests)
		generated := s.GenerateQuests(needed, now)
		if err := s.repo.SaveQuests(ctx, generated); err != nil {
			return nil, err
		}
		quests = append(quests, generated...)
	}

	return quests, nil
}

// GetCharacterDeliveries returns all delivery quests accepted by the character.
func (s *Service) GetCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetCharacterDeliveries(ctx, characterID)
}

// GetActiveCharacterDeliveries returns only in-progress delivery quests for the character.
func (s *Service) GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]CharacterDelivery, error) {
	if strings.TrimSpace(characterID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetActiveCharacterDeliveries(ctx, characterID)
}

// AcceptQuest accepts a delivery quest for a character.
func (s *Service) AcceptQuest(ctx context.Context, characterID string, questID string, now time.Time) (*CharacterDelivery, error) {
	if strings.TrimSpace(characterID) == "" || strings.TrimSpace(questID) == "" {
		return nil, ErrInvalidInput
	}

	var delivery *CharacterDelivery

	action := func(txCtx context.Context) error {
		// 1. Verify character exists
		if _, err := s.charRepo.FindByID(txCtx, characterID); err != nil {
			return ErrCharacterNotFound
		}

		// 2. Fetch quest and check validity
		quest, err := s.repo.GetQuestByID(txCtx, questID)
		if err != nil {
			return ErrQuestNotFound
		}
		if !quest.ExpiresAt.After(now) {
			return ErrQuestExpired
		}

		// 3. Check active deliveries limit
		activeDeliveries, err := s.repo.GetActiveCharacterDeliveries(txCtx, characterID)
		if err != nil {
			return err
		}
		if len(activeDeliveries) >= MaxActiveDeliveries {
			return ErrMaxActiveDeliveries
		}

		// 4. Check if quest is already active
		for _, d := range activeDeliveries {
			if d.QuestID == questID {
				return ErrAlreadyAccepted
			}
		}

		// 5. Create and save character delivery
		d := &CharacterDelivery{
			ID:          id.New(),
			CharacterID: characterID,
			QuestID:     questID,
			Status:      StatusInProgress,
			AcceptedAt:  now,
			Quest:       quest,
		}

		if err := s.repo.SaveCharacterDelivery(txCtx, d); err != nil {
			return err
		}

		delivery = d
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return delivery, nil
}

// CompleteDelivery validates required items, consumes them, grants rewards, and marks delivery complete.
func (s *Service) CompleteDelivery(
	ctx context.Context,
	characterID string,
	deliveryID string,
	now time.Time,
) (*DeliveryCompletionResult, error) {
	if strings.TrimSpace(characterID) == "" || strings.TrimSpace(deliveryID) == "" {
		return nil, ErrInvalidInput
	}

	var result *DeliveryCompletionResult

	action := func(txCtx context.Context) error {
		// 1. Fetch character with lock
		char, err := s.charRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Fetch delivery and verify status and ownership
		delivery, err := s.repo.GetCharacterDeliveryByID(txCtx, deliveryID)
		if err != nil {
			return ErrDeliveryNotFound
		}
		if delivery.CharacterID != characterID {
			return ErrForbidden
		}
		if delivery.Status != StatusInProgress {
			return ErrDeliveryNotActive
		}

		// 3. Fetch quest details
		quest, err := s.repo.GetQuestByID(txCtx, delivery.QuestID)
		if err != nil {
			return ErrQuestNotFound
		}
		if !quest.ExpiresAt.After(now) {
			return ErrQuestExpired
		}

		// 4. Fetch inventory with lock
		inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Check if character has enough of the requested target item
		totalOwned := inv.Quantity(quest.TargetItemID)
		if totalOwned < quest.RequiredQuantity {
			return ErrInsufficientItems
		}

		// Consume required quantity from inventory instances
		remainingToConsume := quest.RequiredQuantity
		for _, inst := range inv.Items {
			if inst.DefinitionID == quest.TargetItemID {
				consumeQty := inst.Quantity
				if consumeQty > remainingToConsume {
					consumeQty = remainingToConsume
				}
				if err := inv.Consume(inst.ID, consumeQty); err != nil {
					return err
				}
				remainingToConsume -= consumeQty
				if remainingToConsume <= 0 {
					break
				}
			}
		}

		// 5. Grant Gold & EXP
		if char.Money > 2000000000-quest.RewardGold {
			char.Money = 2000000000
		} else {
			char.Money += quest.RewardGold
		}

		if char.Experience > 2000000000-quest.RewardExp {
			char.Experience = 2000000000
		} else {
			char.Experience += quest.RewardExp
		}

		// 6. Optional Reward Item
		if quest.RewardItemID != "" {
			rewardInst, err := coreitem.NewInstance(quest.RewardItemID, 1)
			if err != nil {
				return err
			}
			if err := inv.Add(rewardInst); err != nil {
				return err
			}
		}

		// 7. Update character delivery status
		delivery.Status = StatusCompleted
		delivery.CompletedAt = &now

		// 8. Persist changes
		if err := s.invRepo.Save(txCtx, inv); err != nil {
			return err
		}
		if err := s.charRepo.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.repo.UpdateCharacterDelivery(txCtx, delivery); err != nil {
			return err
		}

		result = &DeliveryCompletionResult{
			DeliveryID:     delivery.ID,
			QuestID:        quest.ID,
			RewardedGold:   quest.RewardGold,
			RewardedExp:    quest.RewardExp,
			RewardedItemID: quest.RewardItemID,
			CurrentGold:    char.Money,
			CurrentExp:     char.Experience,
		}

		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// CancelDelivery cancels an in-progress character delivery quest.
func (s *Service) CancelDelivery(ctx context.Context, characterID string, deliveryID string) error {
	if strings.TrimSpace(characterID) == "" || strings.TrimSpace(deliveryID) == "" {
		return ErrInvalidInput
	}

	action := func(txCtx context.Context) error {
		delivery, err := s.repo.GetCharacterDeliveryByID(txCtx, deliveryID)
		if err != nil {
			return ErrDeliveryNotFound
		}
		if delivery.CharacterID != characterID {
			return ErrForbidden
		}
		if delivery.Status != StatusInProgress {
			return ErrDeliveryNotActive
		}

		delivery.Status = StatusCancelled
		return s.repo.UpdateCharacterDelivery(txCtx, delivery)
	}

	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, action)
	}
	return action(ctx)
}

// SendParcel sends an item and/or gold package to another character via town courier.
func (s *Service) SendParcel(
	ctx context.Context,
	senderID string,
	req SendParcelRequest,
	now time.Time,
) (*Parcel, error) {
	if strings.TrimSpace(senderID) == "" || strings.TrimSpace(req.RecipientCharacterID) == "" {
		return nil, ErrInvalidInput
	}
	if senderID == req.RecipientCharacterID {
		return nil, ErrSelfParcelNotAllowed
	}
	if req.GoldAmount < 0 {
		return nil, ErrInvalidInput
	}
	if req.GoldAmount == 0 && req.ItemInstanceID == "" {
		return nil, ErrInvalidParcelPayload
	}

	var parcel *Parcel

	action := func(txCtx context.Context) error {
		// 1. Fetch sender with lock
		sender, err := s.charRepo.FindByIDForUpdate(txCtx, senderID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Fetch recipient
		recipient, err := s.charRepo.FindByID(txCtx, req.RecipientCharacterID)
		if err != nil {
			return ErrRecipientNotFound
		}

		// 3. Verify sender funds
		totalGoldNeeded := req.GoldAmount + DefaultCourierFee
		if sender.Money < totalGoldNeeded {
			return ErrInsufficientFunds
		}

		var itemID, itemName string
		var itemQty int

		// 4. If sending an item, verify and consume from sender inventory
		if req.ItemInstanceID != "" {
			if req.ItemQuantity <= 0 {
				req.ItemQuantity = 1
			}
			inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, senderID)
			if err != nil {
				return err
			}

			inst, found := inv.Find(req.ItemInstanceID)
			if !found || inst.Quantity < req.ItemQuantity {
				return ErrInsufficientItems
			}

			itemID = inst.DefinitionID
			itemName = inst.DefinitionID
			if s.itemDefs != nil {
				if def, defErr := s.itemDefs.FindByID(inst.DefinitionID); defErr == nil {
					itemName = def.Name
				}
			}
			itemQty = req.ItemQuantity

			if err := inv.Consume(inst.ID, req.ItemQuantity); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		// 5. Deduct funds from sender
		sender.Money -= totalGoldNeeded
		if err := s.charRepo.Update(txCtx, sender); err != nil {
			return err
		}

		// 6. Create and save Parcel
		p := &Parcel{
			ID:                   id.New(),
			SenderCharacterID:    senderID,
			SenderCharacterName:  sender.Name,
			RecipientCharacterID: recipient.ID,
			ItemID:               itemID,
			ItemName:             itemName,
			ItemQuantity:         itemQty,
			GoldAmount:           req.GoldAmount,
			Message:              strings.TrimSpace(req.Message),
			CourierFee:           DefaultCourierFee,
			Status:               ParcelStatusPending,
			CreatedAt:            now,
		}

		if err := s.repo.SaveParcel(txCtx, p); err != nil {
			return err
		}

		parcel = p
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return parcel, nil
}

// GetIncomingParcels returns pending courier parcels for recipient.
func (s *Service) GetIncomingParcels(ctx context.Context, recipientID string) ([]Parcel, error) {
	if strings.TrimSpace(recipientID) == "" {
		return nil, ErrInvalidInput
	}
	return s.repo.GetIncomingParcels(ctx, recipientID)
}

// ClaimParcel deposits the parcel's gold and items into the recipient character's possession.
func (s *Service) ClaimParcel(
	ctx context.Context,
	recipientID string,
	parcelID string,
	now time.Time,
) (*ParcelClaimResult, error) {
	if strings.TrimSpace(recipientID) == "" || strings.TrimSpace(parcelID) == "" {
		return nil, ErrInvalidInput
	}

	var result *ParcelClaimResult

	action := func(txCtx context.Context) error {
		// 1. Fetch recipient with lock
		recipient, err := s.charRepo.FindByIDForUpdate(txCtx, recipientID)
		if err != nil {
			return ErrCharacterNotFound
		}

		// 2. Fetch parcel and check status & ownership
		parcel, err := s.repo.GetParcelByID(txCtx, parcelID)
		if err != nil {
			return ErrParcelNotFound
		}
		if parcel.RecipientCharacterID != recipientID {
			return ErrForbidden
		}
		if parcel.Status != ParcelStatusPending {
			return ErrParcelAlreadyClaimed
		}

		// 3. Credit Gold
		if parcel.GoldAmount > 0 {
			if recipient.Money > 2000000000-parcel.GoldAmount {
				recipient.Money = 2000000000
			} else {
				recipient.Money += parcel.GoldAmount
			}
			if err := s.charRepo.Update(txCtx, recipient); err != nil {
				return err
			}
		}

		// 4. Add item if present
		if parcel.ItemID != "" && parcel.ItemQuantity > 0 {
			inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, recipientID)
			if err != nil {
				return err
			}
			inst, err := coreitem.NewInstance(parcel.ItemID, parcel.ItemQuantity)
			if err != nil {
				return err
			}
			if err := inv.Add(inst); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		// 5. Update parcel status
		parcel.Status = ParcelStatusClaimed
		parcel.ClaimedAt = &now
		if err := s.repo.UpdateParcel(txCtx, parcel); err != nil {
			return err
		}

		result = &ParcelClaimResult{
			ParcelID:     parcel.ID,
			SenderName:   parcel.SenderCharacterName,
			ItemID:       parcel.ItemID,
			ItemName:     parcel.ItemName,
			ItemQuantity: parcel.ItemQuantity,
			GoldAmount:   parcel.GoldAmount,
			CurrentGold:  recipient.Money,
		}

		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, action); err != nil {
			return nil, err
		}
	} else {
		if err := action(ctx); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// CancelParcel cancels a pending parcel and returns items/gold to sender.
func (s *Service) CancelParcel(ctx context.Context, senderID string, parcelID string) error {
	if strings.TrimSpace(senderID) == "" || strings.TrimSpace(parcelID) == "" {
		return ErrInvalidInput
	}

	action := func(txCtx context.Context) error {
		// 1. Fetch parcel and check status & sender
		parcel, err := s.repo.GetParcelByID(txCtx, parcelID)
		if err != nil {
			return ErrParcelNotFound
		}
		if parcel.SenderCharacterID != senderID {
			return ErrForbidden
		}
		if parcel.Status != ParcelStatusPending {
			return ErrParcelAlreadyClaimed
		}

		// 2. Return gold payload (fee is non-refundable)
		if parcel.GoldAmount > 0 {
			sender, err := s.charRepo.FindByIDForUpdate(txCtx, senderID)
			if err != nil {
				return ErrCharacterNotFound
			}
			sender.Money += parcel.GoldAmount
			if err := s.charRepo.Update(txCtx, sender); err != nil {
				return err
			}
		}

		// 3. Return item if present
		if parcel.ItemID != "" && parcel.ItemQuantity > 0 {
			inv, err := s.invRepo.FindByCharacterIDForUpdate(txCtx, senderID)
			if err != nil {
				return err
			}
			inst, err := coreitem.NewInstance(parcel.ItemID, parcel.ItemQuantity)
			if err != nil {
				return err
			}
			if err := inv.Add(inst); err != nil {
				return err
			}
			if err := s.invRepo.Save(txCtx, inv); err != nil {
				return err
			}
		}

		parcel.Status = ParcelStatusCancelled
		return s.repo.UpdateParcel(txCtx, parcel)
	}

	if s.txProvider != nil {
		return s.txProvider.RunInTx(ctx, action)
	}
	return action(ctx)
}
