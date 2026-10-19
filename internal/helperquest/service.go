// Package helperquest implements the helper quest service and repository interfaces.
package helperquest

import (
	"context"
	"errors"
	"slices"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

type QuestRepository interface {
	Save(ctx context.Context, q Quest) error
	FindByIDForUpdate(ctx context.Context, id string) (Quest, error)
	ListActive(ctx context.Context, now time.Time) ([]Quest, error)
	ListUncompleted(ctx context.Context) ([]Quest, error)
	// ListExpired reads uncompleted requests whose deadline is before now, without locking.
	ListExpired(ctx context.Context, now time.Time) ([]Quest, error)
	// Delete removes an uncompleted request that is being exchanged for a replacement.
	Delete(ctx context.Context, id string) error
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
	Character      *corecharacter.Character `json:"character,omitempty"`
	Depot          *depot.Depot             `json:"depot,omitempty"`
	CompletedQuest *Quest                   `json:"completed_quest,omitempty"`
	NewQuest       *Quest                   `json:"new_quest,omitempty"`
	RenewedCount   int                      `json:"renewed_count"`
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

// ListQuests reads the board, including expired requests that callers can still complete.
func (s *Service) ListQuests(ctx context.Context, _ time.Time) ([]Quest, error) {
	return s.quests.ListUncompleted(ctx)
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

// CompleteQuest completes the target regardless of its deadline and exchanges expired
// non-target requests atomically. An empty questID only exchanges expired requests.
// Failed turn-ins leave both the board and character assets unchanged.
// Transaction: RunInTx
// Lock Order: helper_quests(0, ascending IDs) -> characters(2) -> depot(5) -> guilds(7, guild quests)
func (s *Service) CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (CompletionResult, error) {
	if s.txProvider == nil {
		return CompletionResult{}, errors.New("transaction provider is missing")
	}
	var res CompletionResult
	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		candidates, err := s.quests.ListExpired(txCtx, now)
		if err != nil {
			return err
		}
		var ids []string
		for _, candidate := range candidates {
			ids = append(ids, candidate.ID)
		}
		if questID != "" {
			ids = append(ids, questID)
		}
		slices.Sort(ids)
		var target Quest
		var expired []Quest
		// Acquire every shared row before character assets; recheck stale candidates under lock.
		for _, id := range slices.Compact(ids) {
			q, err := s.quests.FindByIDForUpdate(txCtx, id)
			if id == questID {
				if err != nil {
					return err
				}
				target = q
				continue
			}
			if errors.Is(err, ErrQuestNotFound) {
				continue // exchanged concurrently
			}
			if err != nil {
				return err
			}
			if q.CompletedAt == nil && now.After(q.ExpiresAt) {
				expired = append(expired, q)
			}
		}
		if questID != "" {
			res, err = s.completeQuest(txCtx, characterID, target, now)
			if err != nil {
				return err
			}
		}
		for _, old := range expired {
			replacement, err := GenerateQuest(s.randomSource, now)
			if err != nil {
				return err
			}
			if err := s.quests.Delete(txCtx, old.ID); err != nil {
				return err
			}
			if err := s.quests.Save(txCtx, replacement); err != nil {
				return err
			}
			res.RenewedCount++
		}
		return nil
	})
	if err != nil {
		return CompletionResult{}, err
	}
	return res, nil
}

// completeQuest uses the target row and transaction already locked by CompleteQuest.
func (s *Service) completeQuest(ctx context.Context, characterID string, q Quest, now time.Time) (CompletionResult, error) {
	if q.CompletedAt != nil {
		return CompletionResult{}, ErrQuestAlreadyDone
	}

	char, err := s.characters.FindByIDForUpdate(ctx, characterID)
	if err != nil {
		return CompletionResult{}, err
	}

	var guildID string
	if q.IsGuild {
		if s.guilds == nil {
			return CompletionResult{}, ErrGuildRequired
		}
		gID, err := s.guilds.FindGuildIDByCharacterID(ctx, characterID)
		if err != nil {
			return CompletionResult{}, err
		}
		if gID == "" {
			return CompletionResult{}, ErrGuildRequired
		}
		guildID = gID
	}

	if s.depotRepo == nil {
		return CompletionResult{}, errors.New("depot repository is required")
	}
	dep, err := depot.FindOrCreate(ctx, s.depotRepo, char)
	if err != nil {
		return CompletionResult{}, err
	}

	if q.Kind == KindMonster {
		if s.farmRepo == nil {
			return CompletionResult{}, errors.New("farm repository is required for monster quest")
		}
		monsters, err := s.farmRepo.ListRanchMonsters(ctx, characterID)
		if err != nil {
			return CompletionResult{}, err
		}

		var matching []FarmMonster
		for _, m := range monsters {
			if m.MonsterID == q.TargetID {
				matching = append(matching, m)
			}
		}

		if len(matching) < q.RequiredCount {
			return CompletionResult{}, ErrInsufficientItems
		}

		for i := 0; i < q.RequiredCount; i++ {
			if err := s.farmRepo.Delete(ctx, matching[i].ID); err != nil {
				return CompletionResult{}, err
			}
		}
	} else {
		if dep.Quantity(q.TargetID) < q.RequiredCount {
			return CompletionResult{}, ErrInsufficientItems
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
				return CompletionResult{}, err
			}
		}
	}

	// Deliver reward directly to Depot
	if q.RewardItemID != "" {
		rewardInst, err := item.NewInstance(q.RewardItemID, 1)
		if err != nil {
			return CompletionResult{}, err
		}
		if err := dep.AddItem(rewardInst); err != nil {
			return CompletionResult{}, err
		}
	}

	if err := s.depotRepo.Save(ctx, dep); err != nil {
		return CompletionResult{}, err
	}

	// Update character
	char.HelpCount++

	// Award Guild Points if Guild Quest
	if q.IsGuild && guildID != "" && s.guilds != nil {
		if err := s.guilds.AddGuildPoints(ctx, guildID, 100); err != nil {
			return CompletionResult{}, err
		}
	}

	// Update completed quest
	q.CompletedAt = &now
	q.CompletedBy = characterID

	if err := s.characters.Update(ctx, char); err != nil {
		return CompletionResult{}, err
	}
	if err := s.quests.Save(ctx, q); err != nil {
		return CompletionResult{}, err
	}

	// Generate replacement quest
	newQ, err := GenerateQuest(s.randomSource, now)
	if err != nil {
		return CompletionResult{}, err
	}
	if err := s.quests.Save(ctx, newQ); err != nil {
		return CompletionResult{}, err
	}
	return CompletionResult{
		Character:      &char,
		Depot:          &dep,
		CompletedQuest: &q,
		NewQuest:       &newQ,
	}, nil
}
