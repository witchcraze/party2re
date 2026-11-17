package adventure

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/core/character"
)

var ErrRecordForbidden = errors.New("adventure record character is not owned")

// RecordUnlocks are durable permissions, never derived during observation.
type RecordUnlocks struct {
	Try    bool `json:"try"`
	Calm   bool `json:"calm"`
	Hard   bool `json:"hard"`
	Image  bool `json:"image"`
	Avatar bool `json:"avatar"`
}

func (u *RecordUnlocks) unlock(total int) []string {
	newly := []string{}
	// Keep the original notification order, including image after the modes.
	for _, rule := range []struct {
		id        string
		threshold int
		flag      *bool
	}{
		{"try", 50, &u.Try}, {"calm", 150, &u.Calm}, {"hard", 300, &u.Hard},
		{"image", 100, &u.Image}, {"avatar", 500, &u.Avatar},
	} {
		if total >= rule.threshold && !*rule.flag {
			*rule.flag = true
			newly = append(newly, rule.id)
		}
	}
	return newly
}

type RecordEntryResult struct {
	TotalClears   int           `json:"total_clears"`
	Unlocks       RecordUnlocks `json:"unlocks"`
	NewlyUnlocked []string      `json:"newly_unlocked"`
}

type RecordRepository interface {
	GetAggregatedStats(context.Context, string) (AggregatedStats, error)
	GetRecordUnlocks(context.Context, string) (RecordUnlocks, error)
	SaveRecordUnlocks(context.Context, string, RecordUnlocks) error
}

type RecordCharacterRepository interface {
	FindByIDForUpdate(context.Context, string) (character.Character, error)
}

type RecordService struct {
	records    RecordRepository
	characters RecordCharacterRepository
	tx         TransactionProvider
}

func NewRecordService(records RecordRepository, characters RecordCharacterRepository, tx TransactionProvider) (*RecordService, error) {
	if records == nil || characters == nil || tx == nil {
		return nil, errors.New("adventure record dependencies are nil")
	}
	return &RecordService{records: records, characters: characters, tx: tx}, nil
}

// OpenOwnRecord is an entry command; visitors and GETs must not invoke it.
func (s *RecordService) OpenOwnRecord(ctx context.Context, playerID, actor string) (RecordEntryResult, error) {
	var result RecordEntryResult
	err := s.tx.RunInTx(ctx, func(ctx context.Context) error {
		c, err := s.characters.FindByIDForUpdate(ctx, actor)
		if err != nil {
			return err
		}
		if playerID == "" || c.PlayerID != playerID {
			return ErrRecordForbidden
		}
		unlocks, err := s.records.GetRecordUnlocks(ctx, actor)
		if err != nil {
			return err
		}
		stats, err := s.records.GetAggregatedStats(ctx, actor)
		if err != nil {
			return err
		}
		for _, stage := range stats.StageStats {
			result.TotalClears += stage.ClearCount
		}
		result.NewlyUnlocked = unlocks.unlock(result.TotalClears)
		result.Unlocks = unlocks
		if len(result.NewlyUnlocked) == 0 {
			return nil
		}
		return s.records.SaveRecordUnlocks(ctx, actor, unlocks)
	})
	if err != nil {
		return RecordEntryResult{}, err
	}
	return result, nil
}
