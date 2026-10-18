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

// ListQuests reads the board, including expired requests that callers can submit for exchange.
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

// CompleteQuest atomically completes a shared request and creates its replacement.
// Like legacy `kaiketsu` (lib/helper.cgi:92-145), every call also exchanges each expired
// uncompleted request other than the target one-for-one, even when this completion fails.
// An expired target is rejected with ErrQuestExpired and is then exchanged as well.
// Transaction: RunInTx (renewal), then RunInTx (completion)
// Lock Order: helper_quests(0) -> characters(2) -> depot(5) -> guilds(7, guild quests)
func (s *Service) CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (CompletionResult, error) {
	if s.txProvider == nil {
		return CompletionResult{}, errors.New("transaction provider is missing")
	}
	if err := s.renewExpired(ctx, now, questID); err != nil {
		return CompletionResult{}, err
	}
	res, err := s.completeQuest(ctx, characterID, questID, now)
	if errors.Is(err, ErrQuestExpired) {
		if renewErr := s.renewExpired(ctx, now, ""); renewErr != nil {
			return CompletionResult{}, errors.Join(err, renewErr)
		}
	}
	return res, err
}

// renewExpired replaces each expired uncompleted request except keepID with one new request.
// Each candidate is re-read FOR UPDATE by id and re-checked, so concurrent callers exchange each
// row once, and any failure rolls back the deletions together with the replacements.
// Transaction: RunInTx
// Lock Order: helper_quests(0)
func (s *Service) renewExpired(ctx context.Context, now time.Time, keepID string) error {
	return s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		candidates, err := s.quests.ListExpired(txCtx, now)
		if err != nil {
			return err
		}
		for _, candidate := range candidates {
			if candidate.ID == keepID {
				continue
			}
			old, err := s.quests.FindByIDForUpdate(txCtx, candidate.ID)
			if errors.Is(err, ErrQuestNotFound) {
				continue // exchanged concurrently
			}
			if err != nil {
				return err
			}
			if old.CompletedAt != nil || !now.After(old.ExpiresAt) {
				continue
			}
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
		}
		return nil
	})
}

func (s *Service) completeQuest(ctx context.Context, characterID, questID string, now time.Time) (CompletionResult, error) {
	var res CompletionResult

	err := s.txProvider.RunInTx(ctx, func(txCtx context.Context) error {
		q, err := s.quests.FindByIDForUpdate(txCtx, questID)
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
			if err != nil {
				return err
			}
			if gID == "" {
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
