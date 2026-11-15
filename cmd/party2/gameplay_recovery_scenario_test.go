package main

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/rescue"
	"github.com/witchcraze/party2re/internal/scheduling"
)

func TestGameplayScenario_SleepWakeRescue(t *testing.T) {
	a := newScenarioApp(t)
	ctx := context.Background()
	token, hero := a.register()
	town := a.context(token, hero)
	if !town.offers("adventure_start") {
		t.Fatal("healthy character cannot start an adventure")
	}
	a.enter(token, hero, town, "bank")
	a.offered(token, hero, a.context(token, hero), "bank_deposit", map[string]any{"amount": 10}, http.StatusOK)
	a.elapseActionWait(hero)
	a.offered(token, hero, a.context(token, hero), "scene_back", nil, http.StatusOK)

	// Death and depleted vitality have no producer in this recovery journey.
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET hp = 0, max_hp = 30, mp = 1, max_mp = 6, tired = 80 WHERE id = ?`, hero); err != nil {
		t.Fatal(err)
	}
	town = a.context(token, hero)
	if !town.Character.IsDead || town.offers("adventure_start") || !town.offers("scene_enter") {
		t.Fatalf("dead character has wrong eligibility: %+v", town)
	}
	home := a.enter(token, hero, town, "home")

	assertState := func(c scenarioContext, hp, mp, tired int, pending bool) {
		t.Helper()
		var gold, deposit, storedHP, storedMP, storedTired int
		var storedPending bool
		if err := a.db.QueryRowContext(ctx, `SELECT money, deposit, hp, mp, tired, pending_wake FROM characters WHERE id = ?`, hero).
			Scan(&gold, &deposit, &storedHP, &storedMP, &storedTired, &storedPending); err != nil {
			t.Fatal(err)
		}
		if gold != 190 || deposit != 10 || storedHP != hp || storedMP != mp || storedTired != tired || storedPending != pending {
			t.Fatalf("stored state: gold=%d deposit=%d HP=%d MP=%d tired=%d pending=%t; want 190/10/%d/%d/%d/%t", gold, deposit, storedHP, storedMP, storedTired, storedPending, hp, mp, tired, pending)
		}
		if c.Character.Gold != gold || c.Character.HP != hp || c.Character.MP != mp || c.Character.Tired != tired ||
			c.Character.MaxHP != 30 || c.Character.MaxMP != 6 || c.Character.IsDead != (hp == 0) {
			t.Fatalf("observation differs from stored vitality/assets: %+v", c.Character)
		}
	}
	assertSleep := func(c scenarioContext, ready bool) {
		t.Helper()
		assertState(c, 0, 1, 80, true)
		if !c.Character.IsSleeping || c.offers("home_wake") != ready || !c.offers("rescue_request") ||
			c.offers("home_sleep") || c.offers("home_color") || c.offers("scene_back") || len(c.OngoingActions) != 1 {
			t.Fatalf("wrong sleep eligibility/work: %+v", c)
		}
		sleep := c.OngoingActions[0]
		if sleep.ID != "sleep" || sleep.ActionType != "home_sleep" || sleep.ExecuteAt.IsZero() || sleep.IsReady != ready ||
			(ready && sleep.RemainingSeconds != 0) || (!ready && sleep.RemainingSeconds <= 0) {
			t.Fatalf("wrong sleep timer: %+v", sleep)
		}
	}
	deadline := func() time.Time {
		t.Helper()
		ms, err := a.valkey.Do(ctx, a.valkey.B().Pexpiretime().Key(timer.PrefixTimer+timer.CategorySleep+":"+hero).Build()).AsInt64()
		if err != nil || ms <= 0 {
			t.Fatalf("sleep expiry: %d, %v", ms, err)
		}
		return time.UnixMilli(ms)
	}
	observeSleep := func(ready bool) {
		t.Helper()
		for range 2 {
			assertSleep(a.context(token, hero), ready)
		}
	}

	slept := a.offered(token, hero, home, "home_sleep", nil, http.StatusOK)
	var sleepResult struct {
		Sleeping        bool   `json:"sleeping"`
		HomeCharacterID string `json:"home_character_id"`
	}
	if err := json.Unmarshal(slept.Result, &sleepResult); err != nil {
		t.Fatal(err)
	}
	if !sleepResult.Sleeping || sleepResult.HomeCharacterID != hero {
		t.Fatalf("unexpected sleep result: %s", slept.Result)
	}
	assertSleep(slept.Context, false)
	originalDeadline := deadline()
	a.elapseActionWait(hero) // Ensure the sleep guard, rather than the action wait, rejects.
	a.restart()
	observeSleep(false)
	if !deadline().Equal(originalDeadline) {
		t.Fatal("restart or GET changed the sleep deadline")
	}

	// Explicit negative commands test admission even though observation withholds them.
	for _, command := range []struct {
		action string
		params map[string]any
		code   string
	}{
		{"home_wake", map[string]any{}, "HOME_STILL_SLEEPING"},
		{"scene_back", map[string]any{}, "ACTION_UNAVAILABLE"},
	} {
		out := a.command(token, hero, command.action, command.params, http.StatusConflict)
		if out.Error.Code != command.code {
			t.Fatalf("%s: got %s, want %s", command.action, out.Error.Code, command.code)
		}
		assertSleep(out.Context, false)
	}

	// Rescue with sleep alone is idle (rescue.cgi:56-65, adopted scheduled-work rule).
	idle := a.offered(token, hero, a.context(token, hero), "rescue_request", map[string]any{"reason": "sleeping"}, http.StatusOK)
	var idleResult rescue.RescueRecord
	if err := json.Unmarshal(idle.Result, &idleResult); err != nil {
		t.Fatal(err)
	}
	if idleResult.ID != "" || idleResult.PenaltySeconds != 0 || !deadline().Equal(originalDeadline) {
		t.Fatalf("idle rescue charged a penalty: %s", idle.Result)
	}
	assertSleep(idle.Context, false)
	a.assertRowCount(`SELECT COUNT(*) FROM rescue_records WHERE character_id = ?`, hero, 0)

	// No Gateway command creates generic scheduled work; seed only this actor's work.
	scheduler := scheduling.NewService(scheduling.NewValkeyRepository(a.valkey))
	if _, err := scheduler.Schedule(ctx, "scenario:recovery", hero, nil, time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	beforeRescue := a.context(token, hero)
	if len(beforeRescue.OngoingActions) != 2 {
		t.Fatalf("unfinished work not observed: %+v", beforeRescue.OngoingActions)
	}
	rescued := a.offered(token, hero, beforeRescue, "rescue_request", map[string]any{"reason": "unfinished work"}, http.StatusOK)
	var record rescue.RescueRecord
	if err := json.Unmarshal(rescued.Result, &record); err != nil {
		t.Fatal(err)
	}
	// rescue.cgi:58-59 adds ten minutes to remaining sleep; timer TTLs use whole seconds.
	if record.ID == "" || record.CharacterID != hero || record.PenaltySeconds != 600 ||
		deadline().Sub(originalDeadline.Add(600*time.Second)).Abs() > 2*time.Second {
		t.Fatalf("rescue lost the additive penalty: %s; deadline=%v original=%v", rescued.Result, deadline(), originalDeadline)
	}
	a.assertRowCount(`SELECT COUNT(*) FROM rescue_records WHERE character_id = ? AND applied_at IS NOT NULL AND penalty_seconds = 600`, hero, 1)
	assertSleep(rescued.Context, false)
	penaltyDeadline := deadline()
	a.restart()
	observeSleep(false)
	if !deadline().Equal(penaltyDeadline) {
		t.Fatal("GET or restart changed the rescue penalty")
	}

	// Expire the duration through its store; the pending-wake obligation must survive.
	if err := timer.NewService(a.valkey).ReleaseLock(ctx, timer.CategorySleep, hero); err != nil {
		t.Fatal(err)
	}
	a.restart()
	observeSleep(true)
	rejected := a.command(token, hero, "scene_back", map[string]any{}, http.StatusConflict)
	if rejected.Error.Code != "ACTION_UNAVAILABLE" {
		t.Fatalf("pending wake did not guard navigation: %+v", rejected)
	}
	assertSleep(rejected.Context, true)

	woke := a.offered(token, hero, a.context(token, hero), "home_wake", nil, http.StatusOK)
	// sleep.cgi:29-32 restores full HP/MP and clears fatigue; GET must never do it.
	for _, observed := range []scenarioContext{woke.Context, a.context(token, hero), a.context(token, hero)} {
		assertState(observed, 30, 6, 0, false)
		if observed.Character.IsSleeping || len(observed.OngoingActions) != 0 || observed.offers("home_wake") ||
			!observed.offers("home_sleep") || !observed.offers("home_color") || !observed.offers("scene_back") {
			t.Fatalf("wake did not restore eligibility: %+v", observed)
		}
	}
	a.offered(token, hero, woke.Context, "scene_back", nil, http.StatusOK)
	if !a.context(token, hero).offers("adventure_start") {
		t.Fatal("wake did not restore the previously forbidden adventure command")
	}
}
