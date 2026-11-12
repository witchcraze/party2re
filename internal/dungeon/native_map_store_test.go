package dungeon_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// nativeStoreContract checks one native transition boundary on any adapter.
func nativeStoreContract(t *testing.T, store dungeon.ActiveExpeditionStore, charID string) {
	t.Helper()
	ctx := context.Background()
	exp := sampleExpedition("native-"+charID, charID, dungeon.NativeMap0ID)
	exp.MapVariant, exp.TurnsRemaining, exp.Revision = 2, 1, 7
	if err := store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	got, err := store.GetActiveExpedition(ctx, charID)
	if err != nil || got == nil || got.MapVariant != 2 || got.TurnsRemaining != 1 {
		t.Fatalf("reload %+v err=%v", got, err)
	}
	stale := got.Revision
	step := func(rev int, key string, enc *dungeon.ActiveEncounter, turns int) (dungeon.StepOutcome, error) {
		return store.Step(ctx, charID, dungeon.StepParams{
			ExpectedExpeditionID: exp.ID, ExpectedRevision: &rev, NewFloor: 1, NewX: 1, NewY: 0,
			TurnsDelta: turns, EventKey: key, Encounter: enc, Now: time.Now().UTC(),
		})
	}
	chests := func(n int) *dungeon.ActiveEncounter {
		return &dungeon.ActiveEncounter{EventType: dungeon.EventTreasure, X: 1, Y: 0, Chests: make([]dungeon.TreasureChest, n)}
	}

	// Reaching the limit keeps the native run active; the trigger saves its chests.
	out, err := step(stale, "C:1:0", chests(2), -1)
	if err != nil {
		t.Fatal(err)
	}
	if out.Status != dungeon.StatusExploring || out.Expedition.TurnsRemaining != 0 || out.Expedition.MapVariant != 2 || !out.EventConsumed ||
		out.Expedition.Encounter == nil || len(out.Expedition.Encounter.Chests) != 2 {
		t.Fatalf("limit step %+v", out)
	}
	// A stale competitor cannot replay the trigger.
	if _, err := step(stale, "C:1:0", chests(2), -1); !errors.Is(err, dungeon.ErrExpeditionStateChanged) && !errors.Is(err, dungeon.ErrCombatActive) {
		t.Fatalf("stale step err=%v", err)
	}
	got, err = store.GetActiveExpedition(ctx, charID)
	if err != nil || got.Revision != stale+1 || len(got.ConsumedEvents) != 1 || len(got.Encounter.Chests) != 2 {
		t.Fatalf("after stale %+v err=%v", got, err)
	}

	// A consumed one-time event never reinstalls its encounter.
	got.Encounter = nil
	if err := store.SaveActiveExpedition(ctx, *got); err != nil {
		t.Fatal(err)
	}
	out, err = step(got.Revision, "C:1:0", chests(3), 0)
	if err != nil {
		t.Fatal(err)
	}
	if out.EventConsumed || out.Expedition.Encounter != nil || out.Status != dungeon.StatusExploring {
		t.Fatalf("consumed replay %+v", out)
	}
}

func TestNativeMap0_StoreContract(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		nativeStoreContract(t, dungeon.NewMemoryExpeditionRepository(), "native-mem")
	})
	t.Run("valkey", func(t *testing.T) {
		client := openTestValkey(t)
		defer client.Close()
		charID := "native-vk-" + time.Now().Format("150405.000000")
		defer cleanupDungeonValkey(t, client, charID)
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		nativeStoreContract(t, store, charID)
	})
}

func TestNativeMap0_EscapeAtLimitThroughSQLFinalizer(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	store, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	dungeonRepo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := dungeon.NewService(dungeonRepo, charRepo, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store))
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacterWithFunds(ctx, db, "Native Exit Hero", 1000)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupDungeonValkey(t, client, char.ID)

	exp, err := svc.StartExpedition(ctx, char.ID, dungeon.NativeMap0ID)
	if err != nil {
		t.Fatal(err)
	}
	exp.TurnsRemaining = 0
	exp.AccumulatedGold, exp.AccumulatedExp = 150, 30
	for i := range exp.Members {
		exp.Members[i].AccumulatedGold, exp.Members[i].AccumulatedExp = 150, 30
	}
	if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Move(ctx, char.ID, dungeon.DirectionNorth); !errors.Is(err, dungeon.ErrMovementExhausted) {
		t.Fatalf("move at limit err=%v", err)
	}
	if res, err := svc.Escape(ctx, char.ID); err != nil || res.Expedition.Status != dungeon.StatusEscaped {
		t.Fatalf("escape %+v err=%v", res, err)
	}
	if _, err := svc.Escape(ctx, char.ID); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatalf("repeated escape err=%v", err)
	}
	after, err := charRepo.FindByID(ctx, char.ID)
	if err != nil {
		t.Fatal(err)
	}
	if after.Money != 1150 {
		t.Fatalf("money=%d, want a single 150G grant", after.Money)
	}
	h, err := svc.GetHistory(ctx, char.ID, 10)
	if err != nil || len(h) != 1 || h[0].Outcome != dungeon.StatusEscaped || h[0].GoldReward != 150 {
		t.Fatalf("history %+v err=%v", h, err)
	}
}
