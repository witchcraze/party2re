package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// All adapters and the real CharacterService observe the same persisted actor.
// Reuse the feature fixtures' repositories and deterministic battle resolver;
// only authentication and infrastructure are test doubles.
type commandLoopFixture struct {
	*homeGatewayFixture
	bank      *bankGatewayFixture
	adventure *adventureGatewayFixture
	profiles  *injectableProfileRepo
	router    http.Handler
}

type commandLoopCharacterRepository struct {
	character.Repository
	homes *homeGatewayFixture
}

func (r commandLoopCharacterRepository) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.homes.FindByID(ctx, id)
}

func newCommandLoopFixture(t *testing.T) *commandLoopFixture {
	homes := newHomeGatewayFixture(t)
	homes.char.Level, homes.char.JobID, homes.char.Deposit = 10, "job-01", 1000
	f := &commandLoopFixture{
		homeGatewayFixture: homes,
		bank:               newBankGatewayFixture(t),
		adventure:          newAdventureGatewayFixture(t),
		profiles: &injectableProfileRepo{profiles: map[string]character.Profile{
			"hero": {CharacterID: "hero", AvatarURL: "avatar"},
		}},
	}
	f.bank.gatewayFixture, f.adventure.gatewayFixture = homes.gatewayFixture, homes.gatewayFixture
	characters, err := character.NewService(commandLoopCharacterRepository{homes: homes},
		character.WithProfileRepository(f.profiles))
	if err != nil {
		t.Fatal(err)
	}
	f.adventure.service, err = adventure.NewService(adventureGatewayRecords{adventureGatewayFixture: f.adventure},
		f.adventure, adventureGatewayBattle{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	f.adventure.service.SetTimerService(f.timers)
	h, err := NewHandler(f.gatewayFixture, characters, f.adventure, &struct{ ShopService }{},
		WithBank(f.bank), WithHome(homes),
		WithPlayerContext(playercontext.NewService(homes, f.gatewayFixture, f.timers)))
	if err != nil {
		t.Fatal(err)
	}
	f.router = h.Router()
	return f
}

func (f *commandLoopFixture) command(action, params string, wantStatus int) map[string]json.RawMessage {
	f.t.Helper()
	status, got := f.bank.request(f.router, action, params)
	if status != wantStatus {
		f.t.Fatalf("%s: want status %d, got %d: %s", action, wantStatus, status, got)
	}
	if wantStatus == http.StatusOK && string(got["success"]) != "true" {
		f.t.Fatalf("%s lost success: %s", action, got)
	}
	return got
}

func (f *commandLoopFixture) getContext() PlayerContextResponse {
	f.t.Helper()
	before, deposits, saves, updates := f.executions, f.bank.depositCalls, f.adventure.saves, f.updates
	stored := f.char
	r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(f.expectedContext)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		f.t.Fatalf("GET context: status=%d body=%s", w.Code, w.Body.String())
	}
	if f.executions != before || f.bank.depositCalls != deposits || f.adventure.saves != saves || f.updates != updates || f.char != stored {
		f.t.Fatal("GET context executed a mutation")
	}
	return f.observation(map[string]json.RawMessage{"context": w.Body.Bytes()})
}

func (f *commandLoopFixture) observation(got map[string]json.RawMessage) PlayerContextResponse {
	f.t.Helper()
	if _, exists := got["context_error"]; exists {
		f.t.Fatalf("unexpected context refresh error: %s", got)
	}
	observation := homeObservation(f.t, got)
	c, stored := observation.Character, f.char
	if c.Name != stored.Name || c.JobID != stored.JobID || c.Level != stored.Level ||
		c.Gold != stored.Money || c.HP != stored.Stats.HP || c.MaxHP != stored.Stats.MaxHP ||
		c.MP != stored.Stats.MP || c.MaxMP != stored.Stats.MaxMP || c.Tired != stored.Tired || c.IsDead != (stored.Stats.HP <= 0) {
		f.t.Fatalf("stale character observation: %+v; stored: %+v", c, stored)
	}
	return observation
}

func assertLoopRecovery(t *testing.T, observation PlayerContextResponse, sleeping, ready bool) {
	t.Helper()
	if observation.Character.IsSleeping != sleeping || hasHomeAction(observation, "home_wake") != ready ||
		hasHomeAction(observation, "adventure_start") == sleeping {
		t.Fatalf("wrong recovery/action eligibility: %+v", observation)
	}
	if !sleeping {
		if len(observation.OngoingActions) != 0 {
			t.Fatalf("unexpected unfinished work: %+v", observation.OngoingActions)
		}
		return
	}
	if len(observation.OngoingActions) != 1 {
		t.Fatalf("lost sleep timer: %+v", observation.OngoingActions)
	}
	a := observation.OngoingActions[0]
	if a.ID != "sleep" || a.ActionType != "home_sleep" || a.ExecuteAt.IsZero() || a.IsReady != ready ||
		(ready && a.RemainingSeconds != 0) || (!ready && a.RemainingSeconds <= 0) {
		t.Fatalf("wrong sleep timer: %+v", a)
	}
}

func assertLoopRefreshFailure(t *testing.T, got map[string]json.RawMessage) {
	t.Helper()
	var detail ErrorDetail
	if err := json.Unmarshal(got["context_error"], &detail); err != nil {
		t.Fatal(err)
	}
	if string(got["context"]) != "null" || detail.Code != "CONTEXT_REFRESH_FAILED" {
		t.Fatalf("lost context refresh failure: %s", got)
	}
}

func TestActionGatewayCommandLoop(t *testing.T) {
	for _, refreshFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("refresh_failure=%t", refreshFails), func(t *testing.T) {
			f := newCommandLoopFixture(t)
			initial := f.char
			assertLoopRecovery(t, f.getContext(), false, false)
			if refreshFails {
				// Inject beneath the real CharacterService only after command execution.
				f.afterExecute = func() { f.profiles.getProfileErr = errors.New("profile storage unavailable") }
			}
			got := f.command("bank_deposit", `{"amount":40}`, http.StatusOK)
			var deposit bank.DepositResult
			if err := json.Unmarshal(got["result"], &deposit); err != nil {
				t.Fatal(err)
			}
			if f.bank.depositCalls != 1 || f.char.Money != 60 || f.char.Deposit != 1040 ||
				int64(f.char.Money)+f.char.Deposit != int64(initial.Money)+initial.Deposit ||
				deposit.CharacterID != "hero" || deposit.Amount != 40 || deposit.Money != f.char.Money || deposit.Deposit != f.char.Deposit {
				t.Fatalf("deposit lost result or assets: %+v; stored: %+v", deposit, f.char)
			}
			if refreshFails {
				assertLoopRefreshFailure(t, got)
				f.profiles.getProfileErr = nil
			} else {
				assertLoopRecovery(t, f.observation(got), false, false)
			}
			assertLoopRecovery(t, f.getContext(), false, false)
			if f.bank.depositCalls != 1 || f.char.Money != 60 || f.char.Deposit != 1040 {
				t.Fatal("GET recovery replayed the successful deposit")
			}

			// A real domain rejection also survives a failed profile refresh.
			before := f.char
			got = f.command("bank_deposit", `{"amount":61}`, http.StatusBadRequest)
			assertGatewayError(t, got, "BANK_INSUFFICIENT_FUNDS")
			if f.char != before || f.bank.depositCalls != 2 {
				t.Fatal("rejected deposit changed persisted state")
			}
			if refreshFails {
				assertLoopRefreshFailure(t, got)
				f.profiles.getProfileErr = nil
			} else {
				assertLoopRecovery(t, f.observation(got), false, false)
			}
			f.afterExecute = nil
			assertLoopRecovery(t, f.getContext(), false, false)

			got = f.command("home_sleep", "", http.StatusOK)
			var sleep sleepResponse
			if err := json.Unmarshal(got["result"], &sleep); err != nil {
				t.Fatal(err)
			}
			if !sleep.Sleeping || sleep.DurationSeconds != 60 || sleep.HomeCharacterID != "hero" || f.char != before {
				t.Fatalf("sleep lost service result or recovered early: %+v", sleep)
			}
			assertLoopRecovery(t, f.observation(got), true, false)
			assertLoopRecovery(t, f.getContext(), true, false)
			for _, elapsed := range []bool{false, true} {
				if elapsed {
					// Fixture simulates duration expiry without a wall-clock wait.
					// The independent pending-wake lock must remain intact.
					if err := f.timers.ReleaseLock(f.expectedContext, timer.CategorySleep, "hero"); err != nil {
						t.Fatal(err)
					}
				}
				assertLoopRecovery(t, f.getContext(), true, elapsed)
				executions := f.executions
				got = f.command("adventure_start", `{"stage_id":"stage-01"}`, http.StatusConflict)
				assertGatewayError(t, got, "ACTION_UNAVAILABLE")
				assertLoopRecovery(t, f.observation(got), true, elapsed)
				if f.executions != executions || f.adventure.saves != 0 || f.updates != 0 || f.char != before {
					t.Fatal("sleep/pending wake executed adventure or recovered vitality")
				}
			}
			got = f.command("home_wake", "", http.StatusOK)
			var wake wakeResponse
			if err := json.Unmarshal(got["result"], &wake); err != nil {
				t.Fatal(err)
			}
			if !wake.Success || wake.Character != toCharacterResponse(f.char) || f.updates != 1 ||
				f.char.Stats.HP != initial.Stats.MaxHP || f.char.Stats.MP != initial.Stats.MaxMP || f.char.Tired != 0 ||
				f.char.Money != 60 || f.char.Deposit != 1040 {
				t.Fatalf("wake failed to restore vitality/preserve assets: %+v", wake)
			}
			locked, err := f.timers.IsLocked(f.expectedContext, timer.CategoryAsleep, "hero")
			if err != nil || locked {
				t.Fatalf("wake did not settle recovery: locked=%t err=%v", locked, err)
			}
			assertLoopRecovery(t, f.observation(got), false, false)
			assertLoopRecovery(t, f.getContext(), false, false)

			got = f.command("adventure_start", `{"stage_id":"stage-01"}`, http.StatusOK)
			var crawl adventureResponse
			if err := json.Unmarshal(got["result"], &crawl); err != nil {
				t.Fatal(err)
			}
			if f.adventure.saves != 1 || crawl != toAdventureResponse(f.adventure.saved) ||
				crawl.CharacterID != "hero" || crawl.StageID != "stage-01" || !crawl.Resolved || !crawl.IsCleared ||
				crawl.FloorsCleared != 10 || f.char.Money <= 60 || f.char.Deposit != 1040 {
				t.Fatalf("adventure did not persist/settle its result: %+v; stored: %+v", crawl, f.char)
			}
			// Immediate Adventure resolution must not invent a delayed expedition.
			assertLoopRecovery(t, f.observation(got), false, false)
			assertLoopRecovery(t, f.getContext(), false, false)
		})
	}
}

func TestActionGatewayCommandLoopUnfinishedWork(t *testing.T) {
	f := newCommandLoopFixture(t)
	deadline := time.Now().UTC().Add(time.Hour)
	f.actions = []scheduling.ScheduledAction{
		{ID: "pending", ActorID: "hero", ActionType: "training", State: scheduling.StatePending, ExecuteAt: deadline},
		{ID: "processing", ActorID: "hero", ActionType: "training", State: scheduling.StateProcessing, ExecuteAt: deadline},
	}
	before := f.char
	for _, elapsed := range []bool{false, true} {
		if elapsed {
			deadline = time.Now().UTC().Add(-time.Hour)
			for i := range f.actions {
				f.actions[i].ExecuteAt = deadline
			}
		}
		assertWork := func(observation PlayerContextResponse) {
			t.Helper()
			if len(observation.OngoingActions) != 2 || hasHomeAction(observation, "adventure_start") || hasHomeAction(observation, "bank_deposit") {
				t.Fatalf("lost unfinished work/guard: %+v", observation)
			}
			for i, a := range observation.OngoingActions {
				if a.ID != f.actions[i].ID || a.ActionType != "training" || !a.ExecuteAt.Equal(deadline) || a.IsReady != elapsed ||
					(elapsed && a.RemainingSeconds != 0) || (!elapsed && a.RemainingSeconds <= 0) {
					t.Fatalf("wrong unfinished timer: %+v", a)
				}
			}
		}
		assertWork(f.getContext())
		for _, command := range []struct{ action, params string }{
			{"bank_deposit", `{"amount":1}`}, {"adventure_start", `{"stage_id":"stage-01"}`},
		} {
			got := f.command(command.action, command.params, http.StatusConflict)
			assertGatewayError(t, got, "ACTION_UNAVAILABLE")
			assertWork(f.observation(got))
		}
		if f.executions != 0 || f.adventure.saves != 0 || f.char != before {
			t.Fatal("deadline arrival settled work or allowed a guarded mutation")
		}
	}
	// Model worker settlement: the reader now has no unfinished records.
	f.actions = nil
	assertLoopRecovery(t, f.getContext(), false, false)
	got := f.command("bank_deposit", `{"amount":1}`, http.StatusOK)
	assertLoopRecovery(t, f.observation(got), false, false)
	if f.bank.depositCalls != 1 || f.char.Money != 99 || f.char.Deposit != 1001 {
		t.Fatal("settled work did not restore command eligibility")
	}
}
