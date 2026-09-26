// Package helperquest implements the helper quest service and repository interfaces.
package helperquest

import (
	"context"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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

type DepotRepository interface {
	FindByCharacterIDForUpdate(ctx context.Context, characterID string) (depot.Depot, error)
	Save(ctx context.Context, d depot.Depot) error
}

type FarmMonster struct {
	ID        string `json:"id"`
	MonsterID string `json:"monster_id"`
}

type FarmRepository interface {
	ListRanchMonsters(ctx context.Context, characterID string) ([]FarmMonster, error)
	Delete(ctx context.Context, id string) error
}

type GuildRepository interface {
	FindGuildIDByCharacterID(ctx context.Context, characterID string) (string, error)
	AddGuildPoints(ctx context.Context, guildID string, points int) error
}

type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

type Option func(*Service)

// WithDepotRepository sets the DepotRepository for storage sourcing and reward delivery.
func WithDepotRepository(repo DepotRepository) Option {
	return func(s *Service) {
		s.depotRepo = repo
	}
}

// WithFarmRepository sets the FarmRepository for ranch companion monster turn-ins.
func WithFarmRepository(repo FarmRepository) Option {
	return func(s *Service) {
		s.farmRepo = repo
	}
}

type CompletionResult struct {
	Character      corecharacter.Character `json:"character"`
	Depot          depot.Depot             `json:"depot"`
	CompletedQuest Quest                   `json:"completed_quest"`
	NewQuest       *Quest                  `json:"new_quest,omitempty"`
}

type Service struct {
	quests       QuestRepository
	characters   CharacterRepository
	depotRepo    DepotRepository
	farmRepo     FarmRepository
	guilds       GuildRepository
	txProvider   TransactionProvider
	randomSource RandomSource
}

func NewService(
	quests QuestRepository,
	characters CharacterRepository,
	guilds GuildRepository,
	txProvider TransactionProvider,
	opts ...Option,
) *Service {
	s := &Service{
		quests:       quests,
		characters:   characters,
		guilds:       guilds,
		txProvider:   txProvider,
		randomSource: DefaultRandomSource(),
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
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

		if s.depotRepo == nil {
			return errors.New("depot repository is required")
		}
		dep, err := depot.FindOrCreate(txCtx, s.depotRepo, char)
		if err != nil {
			return err
		}

		if q.Kind == KindMonster {
			if s.farmRepo == nil {
				return errors.New("farm repository is required for monster quest")
			}
			monsters, err := s.farmRepo.ListRanchMonsters(txCtx, characterID)
			if err != nil {
				return err
			}

			var matching []FarmMonster
			for _, m := range monsters {
				if m.MonsterID == q.TargetID {
					matching = append(matching, m)
				}
			}

			if len(matching) < q.RequiredCount {
				return ErrInsufficientItems
			}

			for i := 0; i < q.RequiredCount; i++ {
				if err := s.farmRepo.Delete(txCtx, matching[i].ID); err != nil {
					return err
				}
			}
		} else {
			if dep.Quantity(q.TargetID) < q.RequiredCount {
				return ErrInsufficientItems
			}

			type consumeTarget struct {
				id  string
				qty int
			}
			needed := q.RequiredCount
			var targets []consumeTarget
			for _, it := range dep.Items {
				if it.DefinitionID == q.TargetID && needed > 0 {
					qty := it.Quantity
					if qty > needed {
						qty = needed
					}
					targets = append(targets, consumeTarget{id: it.ID, qty: qty})
					needed -= qty
				}
			}

			for _, t := range targets {
				if _, err := dep.Consume(t.id, t.qty); err != nil {
					return err
				}
			}
		}

		// Deliver reward directly to Depot
		if q.RewardItemID != "" {
			rewardInst, err := item.NewInstance(q.RewardItemID, 1)
			if err != nil {
				return err
			}
			if err := dep.AddItem(rewardInst); err != nil {
				return err
			}
		}

		if err := s.depotRepo.Save(txCtx, dep); err != nil {
			return err
		}

		// Update character
		char.HelpCount++

		// Award Guild Points if Guild Quest
		if q.IsGuild && guildID != "" && s.guilds != nil {
			if err := s.guilds.AddGuildPoints(txCtx, guildID, 100); err != nil {
				return err
			}
		}

		// Update completed quest
		q.CompletedAt = &now
		q.CompletedBy = characterID

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}
		if err := s.quests.Save(txCtx, q); err != nil {
			return err
		}

		// Generate replacement quest
		newQ, err := GenerateQuest(s.randomSource, now)
		if err != nil {
			return err
		}
		if err := s.quests.Save(txCtx, newQ); err != nil {
			return err
		}
		newQPtr := &newQ

		res = CompletionResult{
			Character:      char,
			Depot:          dep,
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
