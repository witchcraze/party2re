package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/fleamarket"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/lottery"
	"github.com/witchcraze/party2re/internal/scheduling"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// Gameplay scenarios drive the production composition (wireApp) through its
// router with real MariaDB, Valkey and authentication. Nothing is a double, so
// a scenario acts only on what registration, context and outcomes return.
type scenarioApp struct {
	t          *testing.T
	db         *sql.DB
	valkey     valkeygo.Client
	router     http.Handler
	worker     *scheduling.Worker
	adminKey   string
	remoteAddr string
}

func newScenarioApp(t *testing.T) *scenarioApp {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	db := database.OpenTestDB(t)
	client := testutil.OpenValkey(t)
	return newScenarioAppWithDB(t, db, client)
}

// newIsolatedScenarioApp runs wireApp against dedicated MariaDB and Valkey (DB 10)
// stores so singleton schedules (e.g. takarakuji_rounds, takarakuji_draw) never affect
// or observe shared DB 0 state used by other tests or running processes.
func newIsolatedScenarioApp(t *testing.T) *scenarioApp {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	// Schema source for a dedicated DB; close it before creating the scenario app.
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	rows, err := source.QueryContext(context.Background(), "SHOW FULL TABLES WHERE Table_type = 'BASE TABLE'")
	if err != nil {
		_ = source.Close() //lint:ignore error-swallow test setup error takes precedence
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var name, tableType string
		if err := rows.Scan(&name, &tableType); err != nil {
			_ = rows.Close()   //lint:ignore error-swallow test setup error takes precedence
			_ = source.Close() //lint:ignore error-swallow test setup error takes precedence
			t.Fatal(err)
		}
		tables = append(tables, name)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()   //lint:ignore error-swallow test setup error takes precedence
		_ = source.Close() //lint:ignore error-swallow test setup error takes precedence
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		_ = source.Close() //lint:ignore error-swallow test setup error takes precedence
		t.Fatal(err)
	}
	if err := source.Close(); err != nil {
		t.Fatal(err)
	}

	db := database.CreateTestDatabase(t, tables...)
	vkCfg := vk.ConfigFromEnvironment()
	vkCfg.DB = 10 // Isolated from shared DB 0 and casino integration DBs (11-15).
	client, err := vk.NewClientWithConfig(vkCfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	if err := client.Do(context.Background(), client.B().Flushdb().Build()).Error(); err != nil {
		t.Fatalf("flush isolated valkey DB %d: %v", vkCfg.DB, err)
	}
	t.Cleanup(func() {
		if err := client.Do(context.Background(), client.B().Flushdb().Build()).Error(); err != nil {
			t.Errorf("flush isolated valkey DB %d: %v", vkCfg.DB, err)
		}
	})
	return newScenarioAppWithDB(t, db, client)
}

func newScenarioAppWithDB(t *testing.T, db *sql.DB, client valkeygo.Client) *scenarioApp {
	t.Helper()
	uid := id.New()
	// Give each scenario run its own documentation IP so repeated runs do not
	// share Valkey's per-IP public auth rate-limit bucket (192.0.2.1).
	remoteAddr := fmt.Sprintf("[2001:db8:%s:%s:%s:%s]:1234", uid[0:4], uid[4:8], uid[8:12], uid[12:16])
	a := &scenarioApp{t: t, db: db, valkey: client, remoteAddr: remoteAddr}
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
	// Registration caps have dedicated DB coverage; this journey must ignore unrelated players.
	cfg.Player.MaxPlayers = 0
	if a.adminKey != "" {
		cfg.Admin.APIKey = a.adminKey
	}
	app, err := wireApp(a.db, a.valkey, cfg, logging.Nop())
	if err != nil {
		a.t.Fatal(err)
	}
	a.router = app.handler.Router()
	a.worker = app.worker
}

func (a *scenarioApp) call(token, method, path, body string, wantStatus int, out any) {
	a.t.Helper()
	a.callWithHeaders(token, method, path, body, nil, wantStatus, out)
}

func (a *scenarioApp) callWithHeaders(token, method, path, body string, headers map[string]string, wantStatus int, out any) {
	a.t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader(body))
	if a.remoteAddr != "" {
		r.RemoteAddr = a.remoteAddr
	}
	r.Header.Set("Content-Type", "application/json")
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	for k, v := range headers {
		r.Header.Set(k, v)
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
	_, _, token, characterID = a.registerAccount(true)
	return token, characterID
}

func (a *scenarioApp) registerAccount(autoCleanup bool) (playerID, name, token, characterID string) {
	a.t.Helper()
	name = "s" + id.New()[:7]
	credentials := `{"username":"` + name + `","password":"scenario-pass-1"}`
	a.call("", http.MethodPost, "/players", credentials, http.StatusCreated, nil)
	var session struct {
		ID       string `json:"id"`
		PlayerID string `json:"player_id"`
	}
	var character struct{ ID string }
	a.call("", http.MethodPost, "/sessions", credentials, http.StatusCreated, &session)
	if autoCleanup {
		a.t.Cleanup(func() {
			a.call(session.ID, http.MethodDelete, "/players/me", `{"password":"scenario-pass-1"}`, http.StatusOK, nil)
		})
	}
	a.call(session.ID, http.MethodPost, "/characters", `{"name":"`+name+`","job_id":"job-01","gender":"m"}`, http.StatusCreated, &character)
	return session.PlayerID, name, session.ID, character.ID
}

type scenarioContext struct {
	Character struct {
		Gold         int   `json:"gold"`
		Tired        int   `json:"tired"`
		ActionWaitMS int64 `json:"action_wait_ms"`
		HP           int   `json:"hp"`
		MaxHP        int   `json:"max_hp"`
		MP           int   `json:"mp"`
		MaxMP        int   `json:"max_mp"`
		IsDead       bool  `json:"is_dead"`
		IsSleeping   bool  `json:"is_sleeping"`
	} `json:"character"`
	Scene struct {
		LocationID string `json:"location_id"`
		Data       struct {
			Destinations []struct {
				ID          string          `json:"id"`
				EnterParams json.RawMessage `json:"enter_params"`
			} `json:"destinations"`
			Page struct {
				Next json.RawMessage `json:"next"`
			} `json:"page"`
			View struct {
				Private *struct {
					Notices []home.DeliveryNotice `json:"notices"`
				} `json:"private"`
			} `json:"view"`
			Deposit   int64 `json:"deposit"`
			SaleOffer *struct {
				ItemInstanceID   string `json:"item_instance_id"`
				ItemDefinitionID string `json:"item_definition_id"`
			} `json:"sale_offer"`
			PendingRemittances []struct {
				Amount int `json:"amount"`
			} `json:"pending_remittances"`
			Listings      []fleamarket.Listing `json:"listings"`
			OwnedListings []fleamarket.Listing `json:"owned_listings"`
			// Shop wares and Depot contents share the key.
			Items []struct {
				ID             string         `json:"id"`
				RetailPrice    int            `json:"retail_price"`
				PurchaseParams map[string]any `json:"purchase_params"`
				DefinitionID   string         `json:"definition_id"`
				Quantity       int            `json:"quantity"`
			} `json:"items"`
			ItemCount int                           `json:"item_count"`
			RoundID   int                           `json:"round_id"`
			SoldCount int                           `json:"sold_count"`
			DrawDate  time.Time                     `json:"draw_date"`
			Purchased bool                          `json:"purchased"`
			Prizes    []lottery.TakarakujiPrizeItem `json:"prizes"`
			// Casino lobby, selected room and room activity.
			Menu struct {
				Coins int64 `json:"coins"`
			} `json:"menu"`
			Rooms []struct {
				ID           string          `json:"id"`
				SelectParams json.RawMessage `json:"select_params"`
			} `json:"rooms"`
			JoinParams json.RawMessage `json:"join_params"`
			Casino     *struct {
				Coins    int64 `json:"coins"`
				Controls struct {
					Role    string   `json:"role"`
					Actions []string `json:"actions"`
				} `json:"controls"`
			} `json:"casino"`
		} `json:"data"`
	} `json:"scene"`
	AvailableActions []struct {
		Action         string         `json:"action"`
		ParamsTemplate map[string]any `json:"params_template"`
	} `json:"available_actions"`
	OngoingActions []struct {
		ID               string    `json:"id"`
		ActionType       string    `json:"action_type"`
		ExecuteAt        time.Time `json:"execute_at"`
		RemainingSeconds int64     `json:"remaining_seconds"`
		IsReady          bool      `json:"is_ready"`
	} `json:"ongoing_actions"`
}

func (c scenarioContext) offers(action string) bool {
	_, ok := c.template(action)
	return ok
}

// template returns the inputs the observation fixes for an offered command.
func (c scenarioContext) template(action string) (map[string]any, bool) {
	for _, a := range c.AvailableActions {
		if a.Action == action {
			params := map[string]any{}
			for k, v := range a.ParamsTemplate {
				params[k] = v
			}
			return params, true
		}
	}
	return nil, false
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

// offered issues a command the observation offers, with its fixed inputs and
// the caller's remaining ones.
func (a *scenarioApp) offered(token, characterID string, observed scenarioContext, action string, inputs map[string]any, wantStatus int) scenarioOutcome {
	a.t.Helper()
	params, ok := observed.template(action)
	if !ok {
		a.t.Fatalf("%s is not offered: %+v", action, observed.AvailableActions)
	}
	for k, v := range inputs {
		params[k] = v
	}
	return a.command(token, characterID, action, params, wantStatus)
}

// elapseActionWait expires the legacy action wait without a wall-clock sleep.
func (a *scenarioApp) elapseActionWait(characterID string) {
	a.t.Helper()
	if err := timer.NewService(a.valkey).ReleaseLock(context.Background(), timer.CategoryActionWait, characterID); err != nil {
		a.t.Fatal(err)
	}
}

// homeNotices observes the owner's uncleared delivery notices in their Home,
// then returns through town to returnTo so the caller keeps its scene.
func (a *scenarioApp) homeNotices(token, characterID string, town scenarioContext, returnTo string) []home.DeliveryNotice {
	a.t.Helper()
	a.elapseActionWait(characterID)
	observed := a.enter(token, characterID, town, "home")
	if observed.Scene.Data.View.Private == nil {
		a.t.Fatalf("own home lacks private facts: %+v", observed.Scene)
	}
	back := a.command(token, characterID, "scene_back", map[string]any{}, http.StatusOK).Context
	a.enter(token, characterID, back, returnTo)
	return observed.Scene.Data.View.Private.Notices
}

// enter follows a destination the current observation offers, paging hubs when needed.
func (a *scenarioApp) enter(token, characterID string, observed scenarioContext, destination string) scenarioContext {
	a.t.Helper()
	for {
		if !observed.offers("scene_enter") {
			a.t.Fatalf("scene_enter is not offered: %+v", observed.AvailableActions)
		}
		for _, d := range observed.Scene.Data.Destinations {
			if d.ID == destination {
				return a.command(token, characterID, "scene_enter", d.EnterParams, http.StatusOK).Context
			}
		}
		if len(observed.Scene.Data.Page.Next) == 0 || string(observed.Scene.Data.Page.Next) == "null" {
			break
		}
		if !observed.offers("scene_page") {
			a.t.Fatalf("scene_page is not offered: %+v", observed.AvailableActions)
		}
		a.elapseActionWait(characterID)
		observed = a.command(token, characterID, "scene_page", observed.Scene.Data.Page.Next, http.StatusOK).Context
	}
	a.t.Fatalf("destination %s is not offered: %+v", destination, observed.Scene.Data.Destinations)
	return scenarioContext{}
}

func TestGameplayScenario_RegistrationIgnoresUnrelatedPopulation(t *testing.T) {
	t.Setenv("PARTY2_MAX_PLAYERS", "1")
	a := newScenarioApp(t)
	ctx := context.Background()
	unrelated, err := database.CreateTestPlayer(ctx, a.db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := players.Delete(ctx, unrelated.ID); err != nil {
			t.Error(err)
		}
	})
	a.register()
	if _, err := players.FindByID(ctx, unrelated.ID); err != nil {
		t.Fatalf("scenario changed an unrelated player: %v", err)
	}
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
	params := shopPurchaseParams(shop)

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

func shopPurchaseParams(shop scenarioContext) map[string]any {
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
	return params
}

func (a *scenarioApp) assertRejectedTransport(token, method, path, body string, wantStatus int, wantErr string) {
	a.t.Helper()
	var errResp struct {
		Error string `json:"error"`
	}
	a.call(token, method, path, body, wantStatus, &errResp)
	if wantErr != "" && errResp.Error != wantErr {
		a.t.Fatalf("%s %s: want error %q, got %q", method, path, wantErr, errResp.Error)
	}
}

func (a *scenarioApp) assertRejectedSession(token, charID string) {
	a.t.Helper()
	a.assertRejectedTransport(token, http.MethodGet, "/api/v1/characters/"+charID+"/context", "", http.StatusUnauthorized, "invalid session")
	a.assertRejectedTransport(token, http.MethodPost, "/api/v1/characters/"+charID+"/actions", `{"action":"presence_refresh","params":{}}`, http.StatusUnauthorized, "invalid session")
}

func (a *scenarioApp) assertRowCount(query string, arg any, want int) {
	a.t.Helper()
	var count int
	if err := a.db.QueryRowContext(context.Background(), query, arg).Scan(&count); err != nil {
		a.t.Fatalf("%s: %v", query, err)
	}
	if count != want {
		a.t.Fatalf("%s (%v): want %d rows, got %d", query, arg, want, count)
	}
}

func TestGameplayScenario_AccountLogoutBanDeletion(t *testing.T) {
	a := newScenarioApp(t)
	a.adminKey = "scenario-admin-" + id.New()[:8]
	a.restart()

	// Account A holds a bank deposit and a Depot item before logout and self-deletion.
	playerA, nameA, tokenA, charA := a.registerAccount(false)
	townA := a.context(tokenA, charA)
	a.enter(tokenA, charA, townA, "bank")
	deposited := a.command(tokenA, charA, "bank_deposit", map[string]any{"amount": 10}, http.StatusOK).Context
	if deposited.Scene.Data.Deposit != 10 {
		t.Fatalf("expected bank deposit 10, got %+v", deposited.Scene.Data)
	}
	a.elapseActionWait(charA)
	shopA := a.enter(tokenA, charA, townA, "shop_weapon")
	if !shopA.offers("shop_purchase") || len(shopA.Scene.Data.Items) == 0 {
		t.Fatalf("unexpected weapon shop: %+v", shopA)
	}
	buyParams := shopPurchaseParams(shopA)
	for range 2 {
		a.elapseActionWait(charA)
		a.command(tokenA, charA, "shop_purchase", buyParams, http.StatusOK)
	}
	a.elapseActionWait(charA)
	if depot := a.enter(tokenA, charA, townA, "depot"); depot.Scene.Data.ItemCount != 1 {
		t.Fatalf("expected 1 depot item before deletion, got %+v", depot.Scene.Data)
	}

	// Logout invalidates the session for both observation and commands; a new login succeeds.
	a.call(tokenA, http.MethodDelete, "/sessions", "", http.StatusNoContent, nil)
	a.assertRejectedSession(tokenA, charA)

	credsA := `{"username":"` + nameA + `","password":"scenario-pass-1"}`
	var reloginResp struct {
		ID string `json:"id"`
	}
	a.call("", http.MethodPost, "/sessions", credsA, http.StatusCreated, &reloginResp)
	if reloginResp.ID == "" || reloginResp.ID == tokenA {
		t.Fatalf("expected fresh session token on relogin, got %+v", reloginResp)
	}
	tokenA = reloginResp.ID
	if recovered := a.context(tokenA, charA); recovered.Scene.LocationID != "depot" || recovered.Scene.Data.ItemCount != 1 {
		t.Fatalf("relogin did not preserve depot state: %+v", recovered)
	}

	// Register Account B (to ban) and Account C (unaffected observer).
	playerB, nameB, tokenB, charB := a.registerAccount(false)
	t.Cleanup(func() {
		ctx := context.Background()
		charRepo, err := database.NewCharacterRepository(a.db)
		if err != nil {
			t.Error(err)
			return
		}
		playerRepo, err := database.NewPlayerRepository(a.db)
		if err != nil {
			t.Error(err)
			return
		}
		if err := charRepo.Delete(ctx, charB); err != nil {
			t.Error(err)
		}
		if err := playerRepo.Delete(ctx, playerB); err != nil {
			t.Error(err)
		}
	})
	tokenC, charC := a.register()
	a.context(tokenB, charB)

	// Banning Account B with the configured admin key revokes its existing session (401)
	// and blocks new logins (403) while leaving Account C unaffected.
	a.callWithHeaders("", http.MethodPost, "/admin/players/"+playerB+"/ban", "", map[string]string{"X-Admin-Key": a.adminKey}, http.StatusOK, nil)
	a.assertRejectedSession(tokenB, charB)
	credsB := `{"username":"` + nameB + `","password":"scenario-pass-1"}`
	a.assertRejectedTransport("", http.MethodPost, "/sessions", credsB, http.StatusForbidden, "player account is banned")

	a.context(tokenC, charC)
	a.command(tokenC, charC, "presence_refresh", map[string]any{}, http.StatusOK)

	// Before deletion, public reads return charA and its assets exist.
	var profile struct {
		Character struct {
			Name string `json:"name"`
		} `json:"character"`
	}
	a.call("", http.MethodGet, "/characters/"+charA+"/profile", "", http.StatusOK, &profile)
	if profile.Character.Name != nameA {
		t.Fatalf("want profile name %q, got %q", nameA, profile.Character.Name)
	}
	a.call("", http.MethodGet, "/characters/"+charA+"/trajectory", "", http.StatusOK, nil)
	a.call("", http.MethodGet, "/homes/"+charA, "", http.StatusOK, nil)
	a.assertRowCount(`SELECT COUNT(*) FROM characters WHERE id = ? AND deposit = 10`, charA, 1)
	a.assertRowCount(`SELECT COUNT(*) FROM character_depots WHERE character_id = ?`, charA, 1)
	a.assertRowCount(`SELECT COUNT(*) FROM depot_items WHERE character_id = ?`, charA, 1)

	// Self-deletion invalidates the session, removes owned rows/public reads, and frees the character name.
	var deleted struct {
		Deleted  bool   `json:"deleted"`
		PlayerID string `json:"player_id"`
	}
	a.call(tokenA, http.MethodDelete, "/players/me", `{"password":"scenario-pass-1"}`, http.StatusOK, &deleted)
	if !deleted.Deleted || deleted.PlayerID != playerA {
		t.Fatalf("unexpected delete response: %+v", deleted)
	}

	a.assertRejectedSession(tokenA, charA)
	a.assertRejectedTransport("", http.MethodPost, "/sessions", credsA, http.StatusUnauthorized, "authentication failed")

	a.assertRejectedTransport("", http.MethodGet, "/characters/"+charA+"/profile", "", http.StatusNotFound, "character not found")
	a.assertRejectedTransport("", http.MethodGet, "/characters/"+charA+"/trajectory", "", http.StatusNotFound, "character not found")
	a.assertRejectedTransport("", http.MethodGet, "/homes/"+charA, "", http.StatusNotFound, "")
	a.assertRejectedTransport(tokenC, http.MethodGet, "/characters/"+charA, "", http.StatusNotFound, "")

	a.assertRowCount(`SELECT COUNT(*) FROM characters WHERE id = ?`, charA, 0)
	a.assertRowCount(`SELECT COUNT(*) FROM character_depots WHERE character_id = ?`, charA, 0)
	a.assertRowCount(`SELECT COUNT(*) FROM depot_items WHERE character_id = ?`, charA, 0)
	a.assertRowCount(`SELECT COUNT(*) FROM players WHERE id = ?`, playerA, 0)

	// Legacy frees the character name upon directory deletion (new_entry.cgi, delete.cgi).
	var reused struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	a.call(tokenC, http.MethodPost, "/characters", `{"name":"`+nameA+`","job_id":"job-01","gender":"m"}`, http.StatusCreated, &reused)
	if reused.ID == "" || reused.Name != nameA {
		t.Fatalf("expected reused character name %q, got %+v", nameA, reused)
	}
	a.call("", http.MethodGet, "/characters/"+reused.ID+"/profile", "", http.StatusOK, &profile)
	if profile.Character.Name != nameA {
		t.Fatalf("want reused profile name %q, got %q", nameA, profile.Character.Name)
	}
}

type scenarioZeroRollReader struct {
	orig io.Reader
}

func (r *scenarioZeroRollReader) Read(p []byte) (int, error) {
	if len(p) < 16 {
		for i := range p {
			p[i] = 0
		}
		return len(p), nil
	}
	return r.orig.Read(p)
}

func TestGameplayScenario_TakarakujiDraw(t *testing.T) {
	origReader := rand.Reader
	rand.Reader = &scenarioZeroRollReader{orig: origReader}
	defer func() { rand.Reader = origReader }()

	a := newIsolatedScenarioApp(t)
	ctx := context.Background()

	token, hero := a.register()
	// Fund the ticket cost directly (starting gold is 200G; no fast Gateway gold source exists).
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET money = ? WHERE id = ?`, lottery.TakarakujiCostGold, hero); err != nil {
		t.Fatal(err)
	}

	town := a.context(token, hero)
	takScene := a.enter(token, hero, town, "takarakuji")
	if !takScene.offers("takarakuji_buy") {
		t.Fatalf("takarakuji scene did not offer takarakuji_buy: %+v", takScene.AvailableActions)
	}
	roundID := takScene.Scene.Data.RoundID
	if roundID <= 0 || takScene.Scene.Data.Purchased || len(takScene.Scene.Data.Prizes) != 3 {
		t.Fatalf("unexpected initial takarakuji scene: %+v", takScene.Scene.Data)
	}
	wantPrizeID := lottery.IttoPrizeCandidates[0]
	if takScene.Scene.Data.Prizes[0].ItemID != wantPrizeID {
		t.Fatalf("1st prize item = %s, want %s under deterministic roll", takScene.Scene.Data.Prizes[0].ItemID, wantPrizeID)
	}
	wantPrizeName := takScene.Scene.Data.Prizes[0].ItemName
	wantNoticeMsg := fmt.Sprintf("預かり所に%sから%sが届いています", lottery.TakarakujiNPCName, wantPrizeName)

	a.elapseActionWait(hero)
	bought := a.command(token, hero, "takarakuji_buy", map[string]any{}, http.StatusOK)
	var buyResult struct {
		Purchased     bool `json:"purchased"`
		RemainingGold int  `json:"remaining_gold"`
		Ticket        struct {
			RoundID     int    `json:"round_id"`
			CharacterID string `json:"character_id"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal(bought.Result, &buyResult); err != nil {
		t.Fatal(err)
	}
	if !buyResult.Purchased || buyResult.RemainingGold != 0 || buyResult.Ticket.RoundID != roundID || buyResult.Ticket.CharacterID != hero {
		t.Fatalf("unexpected buy result: %s", bought.Result)
	}

	// Expire both the SQL round deadline and the scheduled Valkey action through their stores.
	past := time.Now().UTC().Add(-2 * time.Minute)
	if _, err := a.db.ExecContext(ctx, `UPDATE takarakuji_rounds SET draw_date = ? WHERE round_id = ?`, past, roundID); err != nil {
		t.Fatal(err)
	}
	schedRepo := scheduling.NewValkeyRepository(a.valkey)
	pendingBefore, err := schedRepo.FindPendingByActorID(ctx, "system")
	if err != nil {
		t.Fatal(err)
	}
	var drawAction core_scheduling.ScheduledAction
	foundAction := false
	for _, act := range pendingBefore {
		if act.ActionType == lottery.ActionTypeTakarakujiDraw {
			act.ExecuteAt = past
			if err := schedRepo.Schedule(ctx, act); err != nil {
				t.Fatal(err)
			}
			drawAction = act
			foundAction = true
			break
		}
	}
	if !foundAction {
		t.Fatalf("expected wireApp to schedule %s action under system actor", lottery.ActionTypeTakarakujiDraw)
	}

	// Before the worker runs, GET shows the purchase and settles nothing even past the deadline.
	beforeWorker := a.context(token, hero)
	if beforeWorker.Scene.Data.RoundID != roundID || !beforeWorker.Scene.Data.Purchased || beforeWorker.Scene.Data.SoldCount != 1 {
		t.Fatalf("GET before worker unexpectedly settled or lost purchase: %+v", beforeWorker.Scene.Data)
	}
	noticesBefore := a.homeNotices(token, hero, town, "takarakuji")
	if len(noticesBefore) != 0 {
		t.Fatalf("expected 0 delivery notices before worker runs, got %+v", noticesBefore)
	}

	// Trigger settlement strictly through the production worker returned by wireApp.
	dueActions, err := schedRepo.FetchDue(ctx, time.Now().UTC(), 50)
	if err != nil {
		t.Fatal(err)
	}
	var dueDraw core_scheduling.ScheduledAction
	foundDue := false
	for _, act := range dueActions {
		if act.ID == drawAction.ID {
			dueDraw = act
			foundDue = true
			break
		}
	}
	if !foundDue {
		t.Fatalf("expired takarakuji draw action %s not returned by FetchDue", drawAction.ID)
	}

	beforeRun := time.Now().UTC()
	a.worker.ProcessAction(ctx, dueDraw)
	afterRun := time.Now().UTC()

	// Observe the settled draw and next round through Gateway GET.
	afterWorker := a.context(token, hero)
	nextRoundID := afterWorker.Scene.Data.RoundID
	if nextRoundID != roundID+1 || afterWorker.Scene.Data.Purchased || afterWorker.Scene.Data.SoldCount != 0 {
		t.Fatalf("expected settled round and clean next round in GET context, got %+v", afterWorker.Scene.Data)
	}
	wantNextA := lottery.NextDrawDateJST(beforeRun)
	wantNextB := lottery.NextDrawDateJST(afterRun)
	gotNext := afterWorker.Scene.Data.DrawDate
	if !gotNext.Equal(wantNextA) && !gotNext.Equal(wantNextB) {
		t.Fatalf("next round DrawDate = %v, want %v (or %v)", gotNext, wantNextA, wantNextB)
	}
	jst := time.FixedZone("JST", 9*60*60)
	gotNextJST := gotNext.In(jst)
	if (gotNextJST.Day() != 1 && gotNextJST.Day() != 11 && gotNextJST.Day() != 21) ||
		gotNextJST.Hour() != 0 || gotNextJST.Minute() != 0 || gotNextJST.Second() != 0 || !gotNext.After(beforeRun) {
		t.Fatalf("next round DrawDate %v violates docs/design/lottery.md (1st/11th/21st 00:00 JST in future)", gotNextJST)
	}

	// Verify the prize landed in Depot and produced one @クラゲ Home delivery notice.
	a.elapseActionWait(hero)
	depotCtx := a.enter(token, hero, town, "depot")
	if depotCtx.Scene.Data.ItemCount != 1 || len(depotCtx.Scene.Data.Items) != 1 ||
		depotCtx.Scene.Data.Items[0].DefinitionID != wantPrizeID || depotCtx.Scene.Data.Items[0].Quantity != 1 {
		t.Fatalf("expected 1 prize %s in depot, got %+v", wantPrizeID, depotCtx.Scene.Data)
	}
	noticesAfter := a.homeNotices(token, hero, town, "depot")
	if len(noticesAfter) != 1 || noticesAfter[0].NoticeType != depot.NoticeItemTransfer || noticesAfter[0].Message != wantNoticeMsg {
		t.Fatalf("expected 1 notice %q, got %+v", wantNoticeMsg, noticesAfter)
	}

	// Re-processing the same action through the worker must credit prize and notice only once.
	a.worker.ProcessAction(ctx, dueDraw)

	depotAgain := a.context(token, hero)
	if depotAgain.Scene.Data.ItemCount != 1 || len(depotAgain.Scene.Data.Items) != 1 ||
		depotAgain.Scene.Data.Items[0].DefinitionID != wantPrizeID || depotAgain.Scene.Data.Items[0].Quantity != 1 {
		t.Fatalf("re-processing worker action duplicated depot prize: %+v", depotAgain.Scene.Data)
	}
	noticesAgain := a.homeNotices(token, hero, town, "depot")
	if len(noticesAgain) != 1 {
		t.Fatalf("re-processing worker action duplicated delivery notice: %+v", noticesAgain)
	}

	// Verify the next round draw action remains scheduled in Valkey.
	pendingAfter, err := schedRepo.FindPendingByActorID(ctx, "system")
	if err != nil {
		t.Fatal(err)
	}
	var nextActionFound bool
	for _, act := range pendingAfter {
		if act.ActionType == lottery.ActionTypeTakarakujiDraw && act.State == core_scheduling.StatePending && act.ExecuteAt.Equal(gotNext) {
			nextActionFound = true
			break
		}
	}
	if !nextActionFound {
		t.Fatalf("expected pending next takarakuji draw action at %v in Valkey, got %+v", gotNext, pendingAfter)
	}
}
