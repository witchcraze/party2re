package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	valkeygo "github.com/valkey-io/valkey-go"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// Gameplay scenarios drive the production composition (wireApp) through its
// router with real MariaDB, Valkey and authentication. Nothing is a double, so
// a scenario acts only on what registration, context and outcomes return.
type scenarioApp struct {
	t      *testing.T
	db     *sql.DB
	valkey valkeygo.Client
	router http.Handler
}

func newScenarioApp(t *testing.T) *scenarioApp {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	a := &scenarioApp{t: t, db: db, valkey: client}
	a.restart()
	return a
}

// restart rebuilds every service over the same stores, as a new process would.
func (a *scenarioApp) restart() {
	a.t.Helper()
	cfg, err := ConfigFromEnv()
	if err != nil {
		a.t.Fatal(err)
	}
	// The shared test database holds other tests' players; the population cap is not under test.
	cfg.Player.MaxPlayers = 1 << 30
	app, err := wireApp(a.db, a.valkey, cfg, logging.Nop())
	if err != nil {
		a.t.Fatal(err)
	}
	a.router = app.handler.Router()
}

func (a *scenarioApp) call(token, method, path, body string, wantStatus int, out any) {
	a.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	a.router.ServeHTTP(w, r)
	if w.Code != wantStatus {
		a.t.Fatalf("%s %s %s: want status %d, got %d: %s", method, path, body, wantStatus, w.Code, w.Body.String())
	}
	if out != nil {
		if err := json.Unmarshal(w.Body.Bytes(), out); err != nil {
			a.t.Fatalf("%s %s: %v: %s", method, path, err, w.Body.String())
		}
	}
}

// register creates an account and character through the public routes and
// deletes the account, with everything it owns, when the test ends.
func (a *scenarioApp) register() (token, characterID string) {
	a.t.Helper()
	name := "s" + id.New()[:7]
	credentials := `{"username":"` + name + `","password":"scenario-pass-1"}`
	a.call("", http.MethodPost, "/players", credentials, http.StatusCreated, nil)
	var session, character struct{ ID string }
	a.call("", http.MethodPost, "/sessions", credentials, http.StatusCreated, &session)
	a.t.Cleanup(func() {
		a.call(session.ID, http.MethodDelete, "/players/me", `{"password":"scenario-pass-1"}`, http.StatusOK, nil)
	})
	a.call(session.ID, http.MethodPost, "/characters", `{"name":"`+name+`","job_id":"job-01","gender":"m"}`, http.StatusCreated, &character)
	return session.ID, character.ID
}

type scenarioContext struct {
	Character struct {
		Gold int `json:"gold"`
	} `json:"character"`
	Scene struct {
		LocationID string `json:"location_id"`
		Data       struct {
			Destinations []struct {
				ID          string          `json:"id"`
				EnterParams json.RawMessage `json:"enter_params"`
			} `json:"destinations"`
			Deposit   int64 `json:"deposit"`
			SaleOffer *struct {
				ItemDefinitionID string `json:"item_definition_id"`
			} `json:"sale_offer"`
			// Shop wares and Depot contents share the key.
			Items []struct {
				ID             string         `json:"id"`
				RetailPrice    int            `json:"retail_price"`
				PurchaseParams map[string]any `json:"purchase_params"`
				DefinitionID   string         `json:"definition_id"`
				Quantity       int            `json:"quantity"`
			} `json:"items"`
			ItemCount int `json:"item_count"`
		} `json:"data"`
	} `json:"scene"`
	AvailableActions []struct {
		Action         string         `json:"action"`
		ParamsTemplate map[string]any `json:"params_template"`
	} `json:"available_actions"`
}

func (c scenarioContext) offers(action string) bool {
	for _, a := range c.AvailableActions {
		if a.Action == action {
			return true
		}
	}
	return false
}

type scenarioOutcome struct {
	Result json.RawMessage `json:"result"`
	Error  struct {
		Code string `json:"code"`
	} `json:"error"`
	Context scenarioContext `json:"context"`
}

func (a *scenarioApp) context(token, characterID string) scenarioContext {
	a.t.Helper()
	var c scenarioContext
	a.call(token, http.MethodGet, "/api/v1/characters/"+characterID+"/context", "", http.StatusOK, &c)
	return c
}

func (a *scenarioApp) command(token, characterID, action string, params any, wantStatus int) scenarioOutcome {
	a.t.Helper()
	body, err := json.Marshal(map[string]any{"action": action, "params": params})
	if err != nil {
		a.t.Fatal(err)
	}
	var out scenarioOutcome
	a.call(token, http.MethodPost, "/api/v1/characters/"+characterID+"/actions", string(body), wantStatus, &out)
	return out
}

// elapseActionWait expires the legacy action wait without a wall-clock sleep.
func (a *scenarioApp) elapseActionWait(characterID string) {
	a.t.Helper()
	if err := timer.NewService(a.valkey).ReleaseLock(context.Background(), timer.CategoryActionWait, characterID); err != nil {
		a.t.Fatal(err)
	}
}

// enter follows a destination the current town observation offers.
func (a *scenarioApp) enter(token, characterID string, observed scenarioContext, destination string) scenarioContext {
	a.t.Helper()
	if !observed.offers("scene_enter") {
		a.t.Fatalf("scene_enter is not offered: %+v", observed.AvailableActions)
	}
	for _, d := range observed.Scene.Data.Destinations {
		if d.ID == destination {
			return a.command(token, characterID, "scene_enter", d.EnterParams, http.StatusOK).Context
		}
	}
	a.t.Fatalf("destination %s is not offered: %+v", destination, observed.Scene.Data.Destinations)
	return scenarioContext{}
}

func TestGameplayScenario_NoviceDepositPurchaseEquip(t *testing.T) {
	a := newScenarioApp(t)
	token, hero := a.register()

	town := a.context(token, hero)
	const startingGold = 200 // legacy new-character money
	if town.Character.Gold != startingGold || town.Scene.LocationID != "town" || town.offers("bank_deposit") || town.offers("shop_purchase") {
		t.Fatalf("unexpected first observation: %+v", town)
	}

	bank := a.enter(token, hero, town, "bank")
	if !bank.offers("bank_deposit") {
		t.Fatalf("bank does not offer deposit: %+v", bank.AvailableActions)
	}
	deposited := a.command(token, hero, "bank_deposit", map[string]any{"amount": 10}, http.StatusOK).Context
	if deposited.Character.Gold != startingGold-10 || deposited.Scene.Data.Deposit != 10 {
		t.Fatalf("deposit did not conserve assets: %+v", deposited)
	}

	// The production action wait rejects the next command without changing assets.
	waiting := a.command(token, hero, "bank_deposit", map[string]any{"amount": 10}, http.StatusConflict)
	if waiting.Error.Code != "ACTION_WAIT" || waiting.Context.Character.Gold != startingGold-10 || waiting.Context.Scene.Data.Deposit != 10 {
		t.Fatalf("action wait did not reject cleanly: %+v", waiting)
	}

	// Another account cannot observe or command this character.
	stranger, _ := a.register()
	a.call(stranger, http.MethodGet, "/api/v1/characters/"+hero+"/context", "", http.StatusForbidden, nil)
	a.call(stranger, http.MethodPost, "/api/v1/characters/"+hero+"/actions", `{"action":"bank_withdraw","params":{"amount":10}}`, http.StatusForbidden, nil)

	a.elapseActionWait(hero)
	// Facilities sit under town, so the town observation still lists them after a restart.
	a.restart()
	if recovered := a.context(token, hero); recovered.Scene.LocationID != "bank" || recovered.Character.Gold != startingGold-10 || recovered.Scene.Data.Deposit != 10 {
		t.Fatalf("restart lost session, scene or assets: %+v", recovered)
	}
	shop := a.enter(token, hero, town, "shop_weapon")
	if !shop.offers("shop_purchase") || len(shop.Scene.Data.Items) == 0 || shop.Scene.Data.SaleOffer != nil {
		t.Fatalf("unexpected weapon shop: %+v", shop)
	}
	item := shop.Scene.Data.Items[0]
	params := map[string]any{"quantity": 1}
	for _, action := range shop.AvailableActions {
		if action.Action == "shop_purchase" {
			for k, v := range action.ParamsTemplate {
				params[k] = v
			}
		}
	}
	for k, v := range item.PurchaseParams {
		params[k] = v
	}

	gold := startingGold - 10
	// Legacy purchase: the first item goes to the empty hand, later ones to the Depot.
	for _, wantDepot := range []bool{false, true} {
		a.elapseActionWait(hero)
		bought := a.command(token, hero, "shop_purchase", params, http.StatusOK)
		var result struct {
			TotalCost          int  `json:"total_cost"`
			TransferredToDepot bool `json:"transferred_to_depot"`
		}
		if err := json.Unmarshal(bought.Result, &result); err != nil {
			t.Fatal(err)
		}
		gold -= item.RetailPrice
		offer := bought.Context.Scene.Data.SaleOffer
		if result.TotalCost != item.RetailPrice || result.TransferredToDepot != wantDepot || bought.Context.Character.Gold != gold ||
			offer == nil || offer.ItemDefinitionID != item.ID {
			t.Fatalf("purchase (depot=%t) lost price, destination or equipped item: %s %+v", wantDepot, bought.Result, bought.Context)
		}
	}

	// The Depot itself holds exactly the second purchase; the first stayed in the hand.
	a.elapseActionWait(hero)
	stored := func(c scenarioContext) bool {
		d := c.Scene.Data
		return c.Character.Gold == gold && d.ItemCount == 1 && len(d.Items) == 1 && d.Items[0].DefinitionID == item.ID && d.Items[0].Quantity == 1
	}
	if depot := a.enter(token, hero, town, "depot"); !stored(depot) {
		t.Fatalf("depot does not hold the purchase: %+v", depot)
	}
	// GET observes without mutating: repeated reads agree with the last outcome.
	for range 2 {
		if final := a.context(token, hero); !stored(final) {
			t.Fatalf("observation drifted: %+v", final)
		}
	}
}
