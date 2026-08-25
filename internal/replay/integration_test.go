package replay_test

import (
	"context"
	"os"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestReplayIntegrationFlow(t *testing.T) {
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

	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Resolve combat in Core Battle Engine
	battleEngine := corebattle.Engine{}
	battleReq := corebattle.Request{
		Participants: []corebattle.Participant{
			{ID: "warrior-01", HP: 120, Attack: 35, Defense: 15},
			{ID: "dragon-boss", HP: 200, Attack: 30, Defense: 10},
		},
	}
	battleRes, err := battleEngine.Resolve(battleReq)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 2. Record Battle Replay using standardized recording helper
	initSnap := replay.NewParticipantSnapshot("warrior-01", "Legendary Warrior", 120, 35, 15, 12, "warrior", 20)
	oppSnap := replay.NewParticipantSnapshot("dragon-boss", "Red Fire Dragon", 200, 30, 10, 8, "boss", 25)
	replayID, err := service.RecordMatchFromResult(ctx, replay.CombatTypeBoss, initSnap, oppSnap, battleRes)
	if err != nil {
		t.Fatalf("RecordMatchFromResult failed: %v", err)
	}

	// 3. Fetch Full Replay Log for Playback
	replayDoc, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay failed: %v", err)
	}
	if replayDoc.ID != replayID || replayDoc.TotalTurns != battleRes.Turns || len(replayDoc.TurnLogs) == 0 {
		t.Errorf("unexpected replay playback document: %#v", replayDoc)
	}

	// Verify turn log integrity
	lastLog := replayDoc.TurnLogs[len(replayDoc.TurnLogs)-1]
	if lastLog.Turn != battleRes.Turns {
		t.Errorf("expected final turn index %d, got %d", battleRes.Turns, lastLog.Turn)
	}

	// 4. Query Match History
	history, err := service.GetCharacterHistory(ctx, "warrior-01", replay.CombatTypeBoss, 5)
	if err != nil || len(history) == 0 {
		t.Fatalf("GetCharacterHistory failed: %v, len=%d", err, len(history))
	}
	if history[0].ID != replayID || history[0].InitiatorName != "Legendary Warrior" {
		t.Errorf("unexpected history header: %#v", history[0])
	}
}
