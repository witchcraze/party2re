package dungeon_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestValkeyExpeditionRepository_NewAndFallback(t *testing.T) {
	ctx := context.Background()

	// When client is nil, it should seamlessly fall back to in-memory store
	repo, err := dungeon.NewValkeyExpeditionRepository(nil, dungeon.WithExpeditionTTL(15*time.Minute))
	if err != nil || repo == nil {
		t.Fatalf("expected non-nil repo, got repo=%v err=%v", repo, err)
	}

	exp := dungeon.ActiveExpedition{
		ID:                "exp-fallback-1",
		CharacterID:       "char-fallback-1",
		PartyID:           "party-fallback-1",
		PartyName:         "Fallback Party",
		DungeonID:         "cave_01",
		CurrentFloor:      1,
		PosX:              0,
		PosY:              0,
		CurrentHP:         100,
		TurnsRemaining:    50,
		AccumulatedExp:    10,
		AccumulatedGold:   20,
		AccumulatedMedals: 1,
		AccumulatedItems:  []string{"item-1"},
		Status:            dungeon.StatusExploring,
		StartedAt:         time.Now().UTC(),
		UpdatedAt:         time.Now().UTC(),
	}

	// 1. SaveActiveExpedition fallback
	if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("fallback SaveActiveExpedition error: %v", err)
	}

	// 2. GetActiveExpedition fallback
	retrieved, err := repo.GetActiveExpedition(ctx, "char-fallback-1")
	if err != nil || retrieved == nil {
		t.Fatalf("expected retrieved expedition, got %v, err=%v", retrieved, err)
	}
	if retrieved.ID != "exp-fallback-1" || retrieved.PartyID != "party-fallback-1" {
		t.Errorf("unexpected retrieved expedition: %+v", retrieved)
	}

	// 3. Step fallback
	stepOutcome, err := repo.Step(ctx, "char-fallback-1", dungeon.StepParams{
		ExpectedExpeditionID: "exp-fallback-1",
		NewFloor:             1,
		NewX:                 1,
		NewY:                 0,
		HPDelta:              -5,
		TurnsDelta:           -1,
		ExpDelta:             20,
		GoldDelta:            50,
		MedalsDelta:          1,
		RewardItemID:         "item-2",
		Now:                  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("fallback Step error: %v", err)
	}
	if stepOutcome.Expedition.PosX != 1 || stepOutcome.Expedition.CurrentHP != 95 {
		t.Errorf("unexpected fallback Step result: %+v", stepOutcome)
	}

	// 4. DeleteActiveExpedition fallback
	if err := repo.DeleteActiveExpedition(ctx, "char-fallback-1"); err != nil {
		t.Fatalf("fallback DeleteActiveExpedition error: %v", err)
	}
	afterDelete, err := repo.GetActiveExpedition(ctx, "char-fallback-1")
	if err != nil || afterDelete != nil {
		t.Fatalf("expected nil after delete, got %v, err=%v", afterDelete, err)
	}
}

func TestValkeyExpeditionRepository_SaveActiveExpedition(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	exp := dungeon.ActiveExpedition{
		ID:          "exp-save-1",
		CharacterID: "char-100",
		PartyID:     "party-100",
		PartyName:   "Alpha Explorers",
		Members: []dungeon.ExpeditionMember{
			{
				CharacterID: "char-100",
				Name:        "LeaderHero",
				JobID:       "warrior",
				Level:       25,
				CurrentHP:   120,
				MaxHP:       120,
			},
			{
				CharacterID: "char-101",
				Name:        "MageAlly",
				JobID:       "wizard",
				Level:       24,
				CurrentHP:   80,
				MaxHP:       80,
			},
		},
		DungeonID:         "dungeon-01",
		CurrentFloor:      2,
		PosX:              3,
		PosY:              4,
		CurrentHP:         110,
		TurnsRemaining:    45,
		AccumulatedExp:    150,
		AccumulatedGold:   300,
		AccumulatedMedals: 2,
		AccumulatedItems:  []string{"item-1", "item-2"},
		Status:            dungeon.StatusExploring,
		StartedAt:         now,
		UpdatedAt:         now,
	}

	// 1. Success case: DoMulti executes HSET (state), HSET (rewards), EXPIRE (state), EXPIRE (rewards)
	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		res := make([]valkeygo.ValkeyResult, len(multi))
		for i := range multi {
			res[i] = valkeytest.MakeOKResult()
		}
		return res
	}))

	repo, _ := dungeon.NewValkeyExpeditionRepository(client, dungeon.WithExpeditionTTL(1*time.Hour))
	if err := repo.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatalf("unexpected SaveActiveExpedition error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 4 {
		t.Fatalf("expected 4 recorded commands, got %d: %v", len(cmds), cmds)
	}

	// First command: HSET stateKey
	if cmds[0][0] != "HSET" || !strings.Contains(cmds[0][1], "char:char-100}:state") {
		t.Errorf("unexpected HSET state command: %v", cmds[0])
	}
	// Verify members are properly encoded in HSET
	hasMembersField := false
	for i := 2; i < len(cmds[0])-1; i += 2 {
		if cmds[0][i] == "members" {
			hasMembersField = true
			decodedMembers := dungeon.DecodeMembers(cmds[0][i+1])
			if len(decodedMembers) != 2 || decodedMembers[0].Name != "LeaderHero" {
				t.Errorf("unexpected encoded members in HSET: %v", cmds[0][i+1])
			}
			break
		}
	}
	if !hasMembersField {
		t.Errorf("missing members field in HSET state command: %v", cmds[0])
	}

	// Second command: HSET rewardsKey
	if cmds[1][0] != "HSET" || !strings.Contains(cmds[1][1], "char:char-100}:rewards") {
		t.Errorf("unexpected HSET rewards command: %v", cmds[1])
	}
	// Third and Fourth commands: EXPIRE
	if cmds[2][0] != "EXPIRE" || cmds[3][0] != "EXPIRE" {
		t.Errorf("expected EXPIRE commands for state and rewards, got %v and %v", cmds[2], cmds[3])
	}

	// 2. Error case in DoMulti
	errMulti := errors.New("valkey multi error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeOKResult(),
			valkeytest.MakeErrorResult(errMulti),
			valkeytest.MakeOKResult(),
			valkeytest.MakeOKResult(),
		}
	}))
	repoErr, _ := dungeon.NewValkeyExpeditionRepository(clientErr)
	if err := repoErr.SaveActiveExpedition(ctx, exp); !errors.Is(err, errMulti) {
		t.Fatalf("expected errMulti, got %v", err)
	}
}

func TestValkeyExpeditionRepository_GetActiveExpedition(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	membersJSON := dungeon.EncodeMembers([]dungeon.ExpeditionMember{
		{
			CharacterID: "char-200",
			Name:        "SoloHero",
			JobID:       "thief",
			Level:       30,
			CurrentHP:   150,
			MaxHP:       150,
		},
	})
	itemsJSON := dungeon.EncodeItems([]string{"item-alpha", "item-beta"})

	stateMap := map[string]string{
		"expedition_id":   "exp-get-1",
		"character_id":    "char-200",
		"party_id":        "party-solo",
		"party_name":      "Solo Run",
		"members":         membersJSON,
		"dungeon_id":      "dungeon-lost-ruins",
		"current_floor":   "3",
		"pos_x":           "5",
		"pos_y":           "7",
		"current_hp":      "140",
		"turns_remaining": "38",
		"status":          string(dungeon.StatusExploring),
		"started_at":      now.Format(time.RFC3339Nano),
		"updated_at":      now.Format(time.RFC3339Nano),
	}
	rewardsMap := map[string]string{
		"exp":    "500",
		"gold":   "1200",
		"medals": "3",
		"items":  itemsJSON,
	}

	// 1. Success case
	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeStringMapResult(stateMap),
			valkeytest.MakeStringMapResult(rewardsMap),
		}
	}))
	repo, _ := dungeon.NewValkeyExpeditionRepository(client)

	active, err := repo.GetActiveExpedition(ctx, "char-200")
	if err != nil {
		t.Fatalf("unexpected GetActiveExpedition error: %v", err)
	}
	if active == nil {
		t.Fatal("expected non-nil active expedition")
	}

	if active.ID != "exp-get-1" || active.PartyID != "party-solo" || active.DungeonID != "dungeon-lost-ruins" {
		t.Errorf("unexpected identity fields: %+v", active)
	}
	if active.CurrentFloor != 3 || active.PosX != 5 || active.PosY != 7 || active.CurrentHP != 140 {
		t.Errorf("unexpected coordinates or stats: %+v", active)
	}
	if active.AccumulatedExp != 500 || active.AccumulatedGold != 1200 || active.AccumulatedMedals != 3 {
		t.Errorf("unexpected rewards: %+v", active)
	}
	if len(active.AccumulatedItems) != 2 || active.AccumulatedItems[0] != "item-alpha" {
		t.Errorf("unexpected items: %+v", active.AccumulatedItems)
	}
	if len(active.Members) != 1 || active.Members[0].Name != "SoloHero" {
		t.Errorf("unexpected members: %+v", active.Members)
	}

	// 2. Not found: empty stateMap
	clientEmpty := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeStringMapResult(map[string]string{}),
			valkeytest.MakeStringMapResult(map[string]string{}),
		}
	}))
	repoEmpty, _ := dungeon.NewValkeyExpeditionRepository(clientEmpty)
	emptyActive, err := repoEmpty.GetActiveExpedition(ctx, "char-200")
	if err != nil || emptyActive != nil {
		t.Fatalf("expected nil active expedition on empty map, got %v, err=%v", emptyActive, err)
	}

	// 3. Not found: nil result from Valkey
	clientNil := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeNilResult(),
			valkeytest.MakeNilResult(),
		}
	}))
	repoNil, _ := dungeon.NewValkeyExpeditionRepository(clientNil)
	nilActive, err := repoNil.GetActiveExpedition(ctx, "char-200")
	if err != nil || nilActive != nil {
		t.Fatalf("expected nil active expedition on nil result, got %v, err=%v", nilActive, err)
	}

	// 4. Error on state fetch
	errGet := errors.New("hgetall state error")
	clientErr := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeErrorResult(errGet),
			valkeytest.MakeNilResult(),
		}
	}))
	repoErr, _ := dungeon.NewValkeyExpeditionRepository(clientErr)
	if _, err := repoErr.GetActiveExpedition(ctx, "char-200"); !errors.Is(err, errGet) {
		t.Fatalf("expected errGet, got %v", err)
	}
}

func TestValkeyExpeditionRepository_DeleteActiveExpedition(t *testing.T) {
	ctx := context.Background()

	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(ctx context.Context, multi ...valkeygo.Completed) []valkeygo.ValkeyResult {
		return []valkeygo.ValkeyResult{
			valkeytest.MakeOKResult(),
			valkeytest.MakeOKResult(),
		}
	}))
	repo, _ := dungeon.NewValkeyExpeditionRepository(client)

	if err := repo.DeleteActiveExpedition(ctx, "char-del-1"); err != nil {
		t.Fatalf("unexpected DeleteActiveExpedition error: %v", err)
	}

	cmds := client.RecordedCommandStrings()
	if len(cmds) != 2 {
		t.Fatalf("expected 2 delete commands, got %d: %v", len(cmds), cmds)
	}
	if cmds[0][0] != "DEL" || !strings.Contains(cmds[0][1], "char:char-del-1}:state") {
		t.Errorf("unexpected DEL state command: %v", cmds[0])
	}
	if cmds[1][0] != "DEL" || !strings.Contains(cmds[1][1], "char:char-del-1}:rewards") {
		t.Errorf("unexpected DEL rewards command: %v", cmds[1])
	}
}

func TestValkeyExpeditionRepository_Step(t *testing.T) {
	ctx := context.Background()

	now := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	itemsJSON := dungeon.EncodeItems([]string{"reward-potion"})

	validStepVals := []string{
		string(dungeon.StatusExploring), // [0] status
		"2",                             // [1] current_floor
		"4",                             // [2] pos_x
		"5",                             // [3] pos_y
		"85",                            // [4] current_hp
		"40",                            // [5] turns_remaining
		"120",                           // [6] exp
		"250",                           // [7] gold
		"2",                             // [8] medals
		itemsJSON,                       // [9] items
		now.Format(time.RFC3339Nano),    // [10] updated_at
		"dungeon-cave",                  // [11] dungeon_id
		now.Format(time.RFC3339Nano),    // [12] started_at
	}

	params := dungeon.StepParams{
		ExpectedExpeditionID: "exp-step-1",
		NewFloor:             2,
		NewX:                 4,
		NewY:                 5,
		HPDelta:              -15,
		TurnsDelta:           -1,
		ExpDelta:             50,
		GoldDelta:            100,
		MedalsDelta:          1,
		RewardItemID:         "reward-potion",
		Now:                  now,
	}

	// 1. Success case via EVALSHA
	client := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeStringSliceResult(validStepVals)
	}))
	repo, _ := dungeon.NewValkeyExpeditionRepository(client)

	outcome, err := repo.Step(ctx, "char-step-1", params)
	if err != nil {
		t.Fatalf("unexpected Step error: %v", err)
	}

	if outcome.Status != dungeon.StatusExploring {
		t.Errorf("expected StatusExploring, got %v", outcome.Status)
	}
	if outcome.Expedition.CurrentFloor != 2 || outcome.Expedition.PosX != 4 || outcome.Expedition.PosY != 5 {
		t.Errorf("unexpected position: %+v", outcome.Expedition)
	}
	if outcome.Expedition.CurrentHP != 85 || outcome.Expedition.TurnsRemaining != 40 {
		t.Errorf("unexpected hp/turns: %+v", outcome.Expedition)
	}
	if outcome.Expedition.AccumulatedExp != 120 || outcome.Expedition.AccumulatedGold != 250 || outcome.Expedition.AccumulatedMedals != 2 {
		t.Errorf("unexpected rewards: %+v", outcome.Expedition)
	}
	if len(outcome.Expedition.AccumulatedItems) != 1 || outcome.Expedition.AccumulatedItems[0] != "reward-potion" {
		t.Errorf("unexpected items: %+v", outcome.Expedition.AccumulatedItems)
	}

	// 2. Error mapping: ERR_EXPEDITION_NOT_FOUND
	clientNotFound := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_EXPEDITION_NOT_FOUND"))
	}))
	repoNotFound, _ := dungeon.NewValkeyExpeditionRepository(clientNotFound)
	if _, err := repoNotFound.Step(ctx, "char-step-1", params); !errors.Is(err, dungeon.ErrExpeditionNotFound) {
		t.Fatalf("expected ErrExpeditionNotFound, got %v", err)
	}

	// 3. Error mapping: ERR_EXPEDITION_NOT_ACTIVE
	clientNotActive := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_EXPEDITION_NOT_ACTIVE"))
	}))
	repoNotActive, _ := dungeon.NewValkeyExpeditionRepository(clientNotActive)
	if _, err := repoNotActive.Step(ctx, "char-step-1", params); !errors.Is(err, dungeon.ErrExpeditionNotActive) {
		t.Fatalf("expected ErrExpeditionNotActive, got %v", err)
	}

	// 4. Error mapping: ERR_EXPEDITION_ID_MISMATCH
	clientMismatch := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(errors.New("ERR_EXPEDITION_ID_MISMATCH"))
	}))
	repoMismatch, _ := dungeon.NewValkeyExpeditionRepository(clientMismatch)
	if _, err := repoMismatch.Step(ctx, "char-step-1", params); !errors.Is(err, dungeon.ErrExpeditionIDMismatch) {
		t.Fatalf("expected ErrExpeditionIDMismatch, got %v", err)
	}

	// 5. Generic error
	genericErr := errors.New("lua execution error")
	clientGeneric := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeErrorResult(genericErr)
	}))
	repoGeneric, _ := dungeon.NewValkeyExpeditionRepository(clientGeneric)
	if _, err := repoGeneric.Step(ctx, "char-step-1", params); !errors.Is(err, genericErr) {
		t.Fatalf("expected genericErr, got %v", err)
	}

	// 6. Malformed response format (< 13 elements)
	clientMalformed := valkeytest.NewMockClient(valkeytest.WithDoHandler(func(ctx context.Context, cmd valkeygo.Completed) valkeygo.ValkeyResult {
		return valkeytest.MakeStringSliceResult([]string{"exploring", "2"}) // only 2 elements
	}))
	repoMalformed, _ := dungeon.NewValkeyExpeditionRepository(clientMalformed)
	if _, err := repoMalformed.Step(ctx, "char-step-1", params); err == nil || !strings.Contains(err.Error(), "unexpected lua step response format") {
		t.Fatalf("expected malformed format error, got %v", err)
	}
}
