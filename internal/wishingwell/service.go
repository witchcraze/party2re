package wishingwell

import (
	"context"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Option configures the Service.
type Option func(*Service)

// WithTransactionProvider sets the database transaction provider.
func WithTransactionProvider(tx TransactionProvider) Option {
	return func(s *Service) {
		s.txProvider = tx
	}
}

// Service provides Wishing Well application operations.
type Service struct {
	characters CharacterRepository
	txProvider TransactionProvider
}

// NewService constructs a new wishing well Service.
func NewService(characters CharacterRepository, opts ...Option) (*Service, error) {
	if characters == nil {
		return nil, ErrNilRepository
	}
	svc := &Service{
		characters: characters,
	}
	for _, opt := range opts {
		opt(svc)
	}
	return svc, nil
}

// GetStatus returns the Wishing Well status for the specified character.
func (s *Service) GetStatus(ctx context.Context, characterID string) (WishingWellStatus, error) {
	char, err := s.characters.FindByID(ctx, characterID)
	if err != nil {
		return WishingWellStatus{}, err
	}

	jobMemoryActive := char.JobMemory != nil
	overLevel := char.OverLevel
	canExchange := !jobMemoryActive && !overLevel

	dialogues := make([]string, len(DefaultDialogues))
	for i, d := range DefaultDialogues {
		switch i {
		case 2:
			dialogues[i] = fmt.Sprintf(d, char.Name, char.SP)
		case 5:
			dialogues[i] = fmt.Sprintf(d, char.Name)
		default:
			dialogues[i] = d
		}
	}

	return WishingWellStatus{
		LocationName:    LocationName,
		NPCName:         NPCName,
		BackgroundImage: BackgroundImage,
		CharacterID:     char.ID,
		CharacterName:   char.Name,
		SP:              char.SP,
		JobMemoryActive: jobMemoryActive,
		OverLevel:       overLevel,
		CanExchange:     canExchange,
		MaxHP:           char.Stats.MaxHP,
		MaxMP:           char.Stats.MaxMP,
		Attack:          char.Stats.Attack,
		Defense:         char.Stats.Defense,
		Agility:         char.Stats.Agility,
		Dialogues:       dialogues,
	}, nil
}

// Exchange sacrifices character SP for a permanent stat increase according to legacy rates.
func (s *Service) Exchange(ctx context.Context, req ExchangeRequest) (ExchangeResult, error) {
	stat, err := corecharacter.ParseSPExchangeStat(req.Stat)
	if err != nil {
		return ExchangeResult{}, err
	}

	var result ExchangeResult

	run := func(txCtx context.Context) error {
		// Rank 2: Character lock
		char, err := s.characters.FindByIDForUpdate(txCtx, req.CharacterID)
		if err != nil {
			return err
		}

		increase, err := char.ApplySPExchange(stat, req.SP)
		if err != nil {
			return err
		}

		if err := s.characters.Update(txCtx, char); err != nil {
			return err
		}

		statName := stat.JapaneseName()
		result = ExchangeResult{
			CharacterID:  char.ID,
			Stat:         stat,
			StatName:     statName,
			SPConsumed:   req.SP,
			StatIncrease: increase,
			RemainingSP:  char.SP,
			Message:      FormatExchangeMessage(req.SP, statName, increase),
			Character:    char,
		}
		return nil
	}

	if s.txProvider != nil {
		if err := s.txProvider.RunInTx(ctx, run); err != nil {
			return ExchangeResult{}, err
		}
	} else {
		if err := run(ctx); err != nil {
			return ExchangeResult{}, err
		}
	}

	return result, nil
}
