package database

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type failingRecordRepository struct {
	adventure.RecordRepository
	failRead bool
}

func (r failingRecordRepository) GetAggregatedStats(ctx context.Context, actor string) (adventure.AggregatedStats, error) {
	if r.failRead {
		return adventure.AggregatedStats{}, errors.New("record read failed")
	}
	return r.RecordRepository.GetAggregatedStats(ctx, actor)
}

func (r failingRecordRepository) SaveRecordUnlocks(ctx context.Context, actor string, u adventure.RecordUnlocks) error {
	if err := r.RecordRepository.SaveRecordUnlocks(ctx, actor, u); err != nil {
		return err
	}
	return errors.New("failed after writing unlocks")
}

func TestAdventureRecordPersistenceAndEntry(t *testing.T) {
	db := OpenTestDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := CreateTestCharacter(ctx, db, "Record owner")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, c)
	repo, err := NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := NewTransactionProvider(db)
	service, err := adventure.NewRecordService(repo, chars, tx)
	if err != nil {
		t.Fatal(err)
	}
	// Counts come from clears across stages/seasons, including a cleared run
	// whose actor later fell.
	for i := range 50 {
		season := ""
		subID := 0
		stage := "stage-01"
		if i >= 25 {
			stage = "stage-26"
			subID = 1 + i%4
			season = adventure.SubIDToSeason(subID)
		}
		if err := repo.Save(ctx, adventure.Adventure{ID: id.New(), CharacterID: c.ID, Type: stage, StageID: stage,
			Season: season, SeasonalSubID: subID, StartedAt: time.Now(), IsCleared: true, Resolved: true,
			BattleResult: corebattle.Result{Outcome: corebattle.OutcomeDefeat}}); err != nil {
			t.Fatal(err)
		}
	}
	u, err := repo.GetRecordUnlocks(ctx, c.ID)
	if err != nil || u != (adventure.RecordUnlocks{}) {
		t.Fatalf("clears alone unlocked: %+v %v", u, err)
	}
	for _, player := range []string{"", "another-player"} {
		if _, err := service.OpenOwnRecord(ctx, player, c.ID); !errors.Is(err, adventure.ErrRecordForbidden) {
			t.Fatalf("ownership %q: %v", player, err)
		}
	}
	for _, failRead := range []bool{true, false} {
		failing, err := adventure.NewRecordService(failingRecordRepository{RecordRepository: repo, failRead: failRead}, chars, tx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := failing.OpenOwnRecord(ctx, c.PlayerID, c.ID); err == nil {
			t.Fatal("hidden storage failure")
		}
		u, err := repo.GetRecordUnlocks(ctx, c.ID)
		if err != nil || u != (adventure.RecordUnlocks{}) {
			t.Fatalf("failed entry persisted: %+v %v", u, err)
		}
	}
	var mu sync.Mutex
	var results []adventure.RecordEntryResult
	enter := func() error {
		r, err := service.OpenOwnRecord(ctx, c.PlayerID, c.ID)
		if err != nil {
			return err
		}
		if r.TotalClears != 50 || r.Unlocks != (adventure.RecordUnlocks{Try: true}) {
			return fmt.Errorf("wrong result: %+v", r)
		}
		mu.Lock()
		results = append(results, r)
		mu.Unlock()
		return nil
	}
	for _, err := range testutil.RunRace(enter, enter) {
		if err != nil {
			t.Fatal(err)
		}
	}
	var newly []string
	for _, r := range results {
		newly = append(newly, r.NewlyUnlocked...)
	}
	if !reflect.DeepEqual(newly, []string{"try"}) {
		t.Fatalf("concurrent duplicate delivery: %v", newly)
	}
	// A newly constructed service still observes the saved flags.
	restarted, err := adventure.NewRecordService(repo, chars, tx)
	if err != nil {
		t.Fatal(err)
	}
	r, err := restarted.OpenOwnRecord(ctx, c.PlayerID, c.ID)
	if err != nil || len(r.NewlyUnlocked) != 0 || !r.Unlocks.Try {
		t.Fatalf("repeat after restart: %+v %v", r, err)
	}
	all := adventure.RecordUnlocks{Try: true, Calm: true, Hard: true, Image: true, Avatar: true}
	if err := repo.SaveRecordUnlocks(ctx, c.ID, all); err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetRecordUnlocks(ctx, c.ID); err != nil || got != all {
		t.Fatalf("all flags round trip: %+v %v", got, err)
	}
}
