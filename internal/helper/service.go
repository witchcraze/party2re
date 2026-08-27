package helper

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
)

type QuestRepository interface {
	Save(ctx context.Context, q Quest) error
	FindByID(ctx context.Context, id string) (Quest, error)
	ListActive(ctx context.Context, now time.Time) ([]Quest, error)
}

type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, c corecharacter.Character) error
}

type InventoryRepository interface {
	FindByCharacterID(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (coreinventory.Inventory, error)
	Save(ctx context.Context, inv coreinventory.Inventory) error
}

type GuildRepository interface {
	FindGuildIDByCharacterID(ctx context.Context, characterID string) (string, error)
	AddGuildPoints(ctx context.Context, guildID string, points int) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type CompletionResult struct {
	Character      corecharacter.Character `json:"character"`
	Inventory      coreinventory.Inventory `json:"inventory"`
	CompletedQuest Quest                   `json:"completed_quest"`
	NewQuest       *Quest                  `json:"new_quest,omitempty"`
}

type Service struct {
	quests       QuestRepository
	characters   CharacterRepository
	inventories  InventoryRepository
	guilds       GuildRepository
	txProvider   TransactionProvider
	randomSource RandomSource
}

func NewService(
	quests QuestRepository,
	characters CharacterRepository,
	inventories InventoryRepository,
	guilds GuildRepository,
	txProvider TransactionProvider,
) *Service {
	return &Service{
		quests:       quests,
		characters:   characters,
		inventories:  inventories,
		guilds:       guilds,
		txProvider:   txProvider,
		randomSource: DefaultRandomSource(),
	}
}

func (s *Service) SetRandomSource(r RandomSource) {
	s.randomSource = r
}

func (s *Service) ListQuests(ctx context.Context, now time.Time) ([]Quest, error) {
	return s.quests.ListActive(ctx, now)
}

func (s *Service) GetActiveHelperItemIDs(ctx context.Context, now time.Time) ([]string, error) {
	active, err := s.quests.ListActive(ctx, now)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, q := range active {
		ids = append(ids, q.TargetID)
	}
	return ids, nil
}

func (s *Service) CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (CompletionResult, error) {
	var res CompletionResult

	if s.txProvider == nil {
		return res, errors.New("transaction provider is missing")
	}

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		q, err := s.quests.FindByID(txCtx, questID)
		if err != nil {
			return err
		}

		if q.CompletedAt != nil {
			return ErrQuestAlreadyDone
		}

		if now.After(q.ExpiresAt) {
			return ErrQuestExpired
		}

		char, err := s.characters.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		var guildID string
		if q.IsGuild {
			if s.guilds == nil {
				return ErrGuildRequired
			}
			gID, err := s.guilds.FindGuildIDByCharacterID(txCtx, characterID)
			if err != nil || gID == "" {
				return ErrGuildRequired
			}
			guildID = gID
		}

		inv, err := s.inventories.FindByCharacterIDForUpdate(txCtx, characterID)
		if err != nil {
			return err
		}

		// Count matching items
		matchingCount := 0
		for _, it := range inv.Items {
			if it.DefinitionID == q.TargetID {
				matchingCount += it.Quantity
			}
		}

		if matchingCount < q.RequiredCount {
			return ErrInsufficientItems
		}

		// Deduct items
		needed := q.RequiredCount
		var remainingItems []item.Instance
		for _, it := range inv.Items {
			if it.DefinitionID == q.TargetID && needed > 0 {
				if it.Quantity <= needed {
					needed -= it.Quantity
					continue
				}
				it.Quantity -= needed
				needed = 0
				remainingItems = append(remainingItems, it)
				continue
			}
			remainingItems = append(remainingItems, it)
		}

		// Rebuild inventory with remaining items
		newInv, err := coreinventory.New(characterID)
		if err != nil {
			return err
		}
		for _, it := range remainingItems {
			_ = newInv.Add(it)
		}

		// Add reward item
		rewardInst, err := item.NewInstance(q.RewardItemID, 1)
		if err == nil {
			_ = newInv.Add(rewardInst)
		}

		// Update character
		char.HelpCount++

		// Award Guild Points if Guild Quest
		if q.IsGuild && guildID != "" && s.guilds != nil {
			_ = s.guilds.AddGuildPoints(txCtx, guildID, 100)
		}

		// Update completed quest
		q.CompletedAt = &now
		q.CompletedBy = characterID

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.inventories.Save(txCtx, newInv); err != nil {
			return err
		}
		if err := s.quests.Save(txCtx, q); err != nil {
			return err
		}

		// Generate replacement quest
		newQ, err := GenerateQuest(s.randomSource, now)
		var newQPtr *Quest
		if err == nil {
			_ = s.quests.Save(txCtx, newQ)
			newQPtr = &newQ
		}

		res = CompletionResult{
			Character:      char,
			Inventory:      newInv,
			CompletedQuest: q,
			NewQuest:       newQPtr,
		}
		return nil
	})

	if err != nil {
		return CompletionResult{}, err
	}
	return res, nil
}
