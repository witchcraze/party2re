package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
)

func TestGameplayScenario_HelperQuestCompletion(t *testing.T) {
	a := newIsolatedScenarioApp(t)
	ctx := context.Background()
	token, hero := a.register()

	// No command supplies the initial board (#1226); seed one ordinary request
	// and its materials. helper.cgi:22-31,181-211 permits 3 herbs for item 128.
	now := time.Now().UTC().Truncate(time.Second)
	quest := helperquest.Quest{
		ID: id.New(), Title: "非常用にその1", Kind: helperquest.KindItem,
		TargetID: "item-001", TargetName: "薬草", RequiredCount: 3, RewardItemID: "item-128",
		CreatedAt: now, ExpiresAt: now.Add(6 * 24 * time.Hour),
	}
	quests, err := database.NewHelperRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := quests.Save(ctx, quest); err != nil {
		t.Fatal(err)
	}
	material, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, a.db, hero, 0, []item.Instance{material}); err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}

	assertAssets := func(materials, rewards, helpCount int) {
		t.Helper()
		dep, err := depots.FindByCharacterID(ctx, hero)
		if err != nil {
			t.Fatal(err)
		}
		if dep.Quantity("item-001") != materials || dep.Quantity("item-128") != rewards || len(dep.Items) != 1+rewards {
			t.Fatalf("stored Depot: %+v; want %d herbs and %d rewards", dep, materials, rewards)
		}
		var gold, deposit, storedHelpCount int
		if err := a.db.QueryRowContext(ctx, `SELECT money, deposit, help_count FROM characters WHERE id = ?`, hero).
			Scan(&gold, &deposit, &storedHelpCount); err != nil {
			t.Fatal(err)
		}
		if gold != 200 || deposit != 0 || storedHelpCount != helpCount {
			t.Fatalf("stored character: gold=%d deposit=%d help_count=%d; want 200/0/%d", gold, deposit, storedHelpCount, helpCount)
		}
		stored, err := quests.FindByID(ctx, quest.ID)
		if err != nil {
			t.Fatal(err)
		}
		if (stored.CompletedAt != nil) != (helpCount == 1) || (helpCount == 1 && stored.CompletedBy != hero) {
			t.Fatalf("stored completion: %+v", stored)
		}
	}
	assertBoard := func(observed scenarioContext, want []helperquest.Quest) {
		t.Helper()
		stored, err := quests.ListUncompleted(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if observed.Scene.LocationID != "helper" || !reflect.DeepEqual(observed.Scene.Data.Quests, want) || !reflect.DeepEqual(stored, want) {
			t.Fatalf("board: observed=%+v stored=%+v; want %+v", observed.Scene, stored, want)
		}
	}

	board := a.enter(token, hero, a.context(token, hero), "helper")
	assertBoard(board, []helperquest.Quest{quest})
	assertAssets(4, 0, 0)
	params := map[string]any{"quest_id": board.Scene.Data.Quests[0].ID}
	completed := a.offered(token, hero, board, "helper_complete", params, http.StatusOK)
	var result struct {
		CompletedQuest *helperquest.Quest  `json:"completed_quest"`
		Renewed        []helperquest.Quest `json:"renewed"`
	}
	if err := json.Unmarshal(completed.Result, &result); err != nil {
		t.Fatal(err)
	}
	if result.CompletedQuest == nil || result.CompletedQuest.ID != quest.ID || result.CompletedQuest.CompletedBy != hero ||
		result.CompletedQuest.CompletedAt == nil || len(result.Renewed) != 0 {
		t.Fatalf("completion result: %s", completed.Result)
	}
	// helper.cgi:111-120 consumes three, sends one reward and replaces the request.
	assertAssets(1, 1, 1)
	replacement := completed.Context.Scene.Data.Quests
	if len(replacement) != 1 || replacement[0].ID == "" || replacement[0].ID == quest.ID || replacement[0].CompletedAt != nil {
		t.Fatalf("request was not replaced: %+v", replacement)
	}
	assertBoard(completed.Context, replacement)

	// Explicit negative replay uses the previously observed ID. The common wait
	// must reject before feature execution and leave the board and assets intact.
	rejected := a.command(token, hero, "helper_complete", params, http.StatusConflict)
	if rejected.Error.Code != "ACTION_WAIT" {
		t.Fatalf("completion during action wait: %+v", rejected)
	}
	assertBoard(rejected.Context, replacement)
	assertAssets(1, 1, 1)
	a.elapseActionWait(hero)
	a.restart()
	for range 2 {
		assertBoard(a.context(token, hero), replacement)
		assertAssets(1, 1, 1)
	}

	town := a.offered(token, hero, a.context(token, hero), "scene_back", nil, http.StatusOK).Context
	depot := a.enter(token, hero, town, "depot")
	quantities := map[string]int{}
	for _, it := range depot.Scene.Data.Items {
		quantities[it.DefinitionID] += it.Quantity
	}
	if !reflect.DeepEqual(quantities, map[string]int{"item-001": 1, "item-128": 1}) {
		t.Fatalf("observed Depot: %+v", depot.Scene.Data.Items)
	}
	assertAssets(1, 1, 1)
}
