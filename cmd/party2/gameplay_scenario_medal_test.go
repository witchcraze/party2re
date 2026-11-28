package main

import (
	"context"
	"encoding/json"
	"net/http"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
)

func TestGameplayScenario_MedalPrizeExchange(t *testing.T) {
	a := newScenarioApp(t)
	ctx := context.Background()
	token, hero := a.register()

	// Medal drops come from unconnected treasure paths; seed three Small Medal
	// items. Using one at home banks one medal (_data.cgi:759-763).
	const medalItem, prize, cost = "item-023", "armor-32", 3
	medals, err := item.NewInstance(medalItem, cost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, a.db, hero, 0, []item.Instance{medals}); err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}
	assertAssets := func(step string, banked int, want map[string]int) {
		t.Helper()
		var stored int
		if err := a.db.QueryRowContext(ctx, `SELECT small_medals FROM characters WHERE id = ?`, hero).Scan(&stored); err != nil {
			t.Fatal(err)
		}
		dep, err := depots.FindByCharacterID(ctx, hero)
		if err != nil {
			t.Fatal(err)
		}
		got := map[string]int{}
		for _, it := range dep.Items {
			got[it.DefinitionID] += it.Quantity
		}
		if stored != banked || !reflect.DeepEqual(got, want) {
			t.Fatalf("%s: stored medals=%d Depot=%v; want %d and %v", step, stored, got, banked, want)
		}
	}

	a.enter(token, hero, a.context(token, hero), "home")
	for used := 1; used <= cost; used++ {
		a.elapseActionWait(hero)
		home := a.context(token, hero)
		var target map[string]any
		for _, it := range home.Scene.Data.View.Private.Items {
			if it.DefinitionID == medalItem {
				target = map[string]any{"instance_id": it.InstanceID, "source": it.Source}
			}
		}
		if target == nil {
			t.Fatalf("home does not offer the medal item: %+v", home.Scene.Data.View.Private)
		}
		a.offered(token, hero, home, "home_item_use", target, http.StatusOK)
		want := map[string]int{}
		if used < cost {
			want[medalItem] = cost - used
		}
		assertAssets("medal used", used, want)
	}

	a.elapseActionWait(hero)
	town := a.command(token, hero, "scene_back", map[string]any{}, http.StatusOK).Context
	castle := a.enter(token, hero, town, "medal")
	// medal.cgi:20-21 lists 3 medals for armor No. 32 first.
	if castle.Scene.Data.SmallMedals != cost || len(castle.Scene.Data.Rewards) == 0 ||
		castle.Scene.Data.Rewards[0].Cost != cost || castle.Scene.Data.Rewards[0].ItemID != prize {
		t.Fatalf("castle: %+v", castle.Scene.Data)
	}
	params := map[string]any{"item_id": castle.Scene.Data.Rewards[0].ItemID}
	claimed := a.offered(token, hero, castle, "medal_claim", params, http.StatusOK)
	var result struct {
		Cost        int    `json:"cost"`
		ItemID      string `json:"item_id"`
		SmallMedals int    `json:"small_medals"`
	}
	if err := json.Unmarshal(claimed.Result, &result); err != nil || result.Cost != cost || result.ItemID != prize || result.SmallMedals != 0 {
		t.Fatalf("claim: %s %v", claimed.Result, err)
	}
	// medal.cgi:80-101 deducts the tier and sends one prize to the Depot.
	exchanged := map[string]int{prize: 1}
	assertAssets("claimed", 0, exchanged)

	// The common wait rejects a replay of the observed claim before execution.
	rejected := a.command(token, hero, "medal_claim", params, http.StatusConflict)
	if rejected.Error.Code != "ACTION_WAIT" || rejected.Context.Scene.Data.SmallMedals != 0 {
		t.Fatalf("claim during action wait: %+v", rejected)
	}
	assertAssets("rejected replay", 0, exchanged)
	a.elapseActionWait(hero)
	a.restart()
	for range 2 {
		if c := a.context(token, hero); c.Scene.LocationID != "medal" || c.Scene.Data.SmallMedals != 0 {
			t.Fatalf("observed castle after restart: %+v", c.Scene)
		}
		assertAssets("observed after restart", 0, exchanged)
	}

	town = a.offered(token, hero, a.context(token, hero), "scene_back", nil, http.StatusOK).Context
	depot := a.enter(token, hero, town, "depot")
	if len(depot.Scene.Data.Items) != 1 || depot.Scene.Data.Items[0].DefinitionID != prize || depot.Scene.Data.Items[0].Quantity != 1 {
		t.Fatalf("observed Depot: %+v", depot.Scene.Data.Items)
	}
	// medal.cgi passes no sender to send_item (system.cgi:818-835): no notice.
	a.elapseActionWait(hero)
	town = a.command(token, hero, "scene_back", map[string]any{}, http.StatusOK).Context
	if notices := a.homeNotices(token, hero, town, "depot"); len(notices) != 0 {
		t.Fatalf("medal exchange left delivery notices: %+v", notices)
	}
	assertAssets("final", 0, exchanged)
}
