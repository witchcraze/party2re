package delivery

import (
	"context"
	"strings"
	"time"

	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
)

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
		_ = char.AddMoney(quest.RewardGold)

		if quest.RewardExp > 0 {
			if _, err := progression.ApplyExperience(&char, quest.RewardExp); err != nil {
				return err
			}
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
