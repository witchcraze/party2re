package database_test

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestReplayRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	replayID := fmt.Sprintf("rep_%016x", now.UnixNano())
	r := replay.BattleReplay{
		ID:            replayID,
		CombatType:    replay.CombatTypePvP,
		InitiatorID:   "charA",
		InitiatorName: "Hero A",
		OpponentID:    "charB",
		OpponentName:  "Hero B",
		Outcome:       corebattle.OutcomeWin,
		WinnerID:      "charA",
		LoserID:       "charB",
		TotalTurns:    3,
		InitialParticipants: []replay.ParticipantSnapshot{
			{ID: "charA", Name: "Hero A", MaxHP: 100, Attack: 30, Defense: 10, Level: 10},
			{ID: "charB", Name: "Hero B", MaxHP: 80, Attack: 20, Defense: 8, Level: 9},
		},
		TurnLogs: []corebattle.TurnLog{
			{
				Turn:        1,
				ActorID:     "charA",
				ActionName:  "こうげき",
				TargetID:    "charB",
				DamageDealt: 22,
				Message:     "Hero A の攻撃！ 22ダメージ！",
				RemainingHP: map[string]int{"charA": 100, "charB": 58},
			},
		},
		CreatedAt: now,
	}

	// 1. Save
	if err := repo.Save(ctx, r); err != nil {
		t.Fatalf("Save replay failed: %v", err)
	}

	// 2. FindByID
	fetched, err := repo.FindByID(ctx, replayID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if fetched.ID != replayID || fetched.CombatType != replay.CombatTypePvP || len(fetched.TurnLogs) != 1 {
		t.Errorf("unexpected fetched replay: %#v", fetched)
	}

	// 3. FindByCharacter
	headers, err := repo.FindByCharacter(ctx, "charA", "", 10)
	if err != nil || len(headers) == 0 {
		t.Fatalf("FindByCharacter failed: %v, len=%d", err, len(headers))
	}
	if headers[0].ID != replayID || headers[0].InitiatorName != "Hero A" {
		t.Errorf("unexpected header: %#v", headers[0])
	}

	// 4. FindRecent
	recent, err := repo.FindRecent(ctx, replay.CombatTypePvP, 10)
	if err != nil || len(recent) == 0 {
		t.Fatalf("FindRecent failed: %v, len=%d", err, len(recent))
	}

	// 5. DeleteOlderThan
	deleted, err := repo.DeleteOlderThan(ctx, now.Add(time.Minute))
	if err != nil || deleted == 0 {
		t.Errorf("expected at least 1 deleted replay, got %d (err: %v)", deleted, err)
	}
}
