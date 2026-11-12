package playercontext

import (
	"context"
	"errors"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

func TestActionWaitGateBlocksAllButRecovery(t *testing.T) {
	waiting := Snapshot{ActionWait: time.Second, Sleeping: true, CanWake: true, Character: character.Character{Stats: character.Stats{HP: 0}}}
	got := Evaluate(waiting)
	for _, id := range got {
		def, _ := findAction(id)
		if def.WaitExempt != WaitRecovery && def.WaitExempt != WaitReload {
			t.Fatalf("%s offered during the wait", id)
		}
	}
	if !slices.Contains(got, "home_wake") || !slices.Contains(got, "rescue_request") {
		t.Fatalf("recovery actions must stay available during the wait: %v", got)
	}
	// Continuations are gated too: legacy rejects every registered action.
	party := Snapshot{ActionWait: time.Second, Activities: []Activity{{Kind: "party", Actions: []string{"party_leave"}}}}
	if slices.Contains(Evaluate(party), "party_leave") {
		t.Fatal("continuation offered during the wait")
	}
}

func findAction(id string) (ActionDefinition, bool) {
	for _, def := range DefaultCatalog {
		if def.ID == id {
			return def, true
		}
	}
	return ActionDefinition{}, false
}

func TestCatalogWaitDispositions(t *testing.T) {
	for _, def := range DefaultCatalog {
		switch {
		case def.ActivityKind != "":
			if def.WaitExempt != WaitActivityPacing {
				t.Errorf("%s: continuation pacing belongs to its activity owner", def.ID)
			}
		case def.Category == "navigation":
			if def.WaitExempt != WaitNavigation {
				t.Errorf("%s: navigation must not start the wait", def.ID)
			}
		case def.ID == "npc_inspect" || def.ID == "player_inspect":
			if def.WaitExempt != WaitPrivate {
				t.Errorf("%s: legacy shiraberu answers with a private $mes", def.ID)
			}
		case def.ID == "place_say":
			if def.WaitExempt != WaitSpeech {
				t.Errorf("%s: plain speech is not a ＠ action", def.ID)
			}
		case def.ID == "place_whisper":
			if def.WaitExempt != WaitWhisper {
				t.Errorf("%s: sasayaki sets act_time = 0", def.ID)
			}
		case def.ID == "alchemy_recipes":
			if def.WaitExempt != WaitNavigation {
				t.Errorf("%s: the reshipi list view sets act_time = 0", def.ID)
			}
		case def.ID == "presence_refresh":
			if def.WaitExempt != WaitReload {
				t.Errorf("%s: a page reload carries no ＠ action", def.ID)
			}
		case def.ID == "home_wake" || def.ID == "rescue_request":
			if def.WaitExempt != WaitRecovery {
				t.Errorf("%s: recovery runs outside party.cgi", def.ID)
			}
		default:
			if def.WaitExempt != "" {
				t.Errorf("%s: unexpected exemption %q; record its legacy evidence first", def.ID, def.WaitExempt)
			}
		}
	}
}

type waitTimers struct {
	remaining map[string]time.Duration
	err       error
	held      bool
	claims    []string
	released  int
}

func (w *waitTimers) GetRemainingLock(_ context.Context, category, _ string) (time.Duration, error) {
	if category == timer.CategoryActionWait && w.err != nil {
		return 0, w.err
	}
	return w.remaining[category], nil
}
func (w *waitTimers) IsLocked(context.Context, string, string) (bool, error) { return false, nil }
func (w *waitTimers) TryLockWithToken(_ context.Context, category, id, token string, d time.Duration) (bool, error) {
	if w.err != nil {
		return false, w.err
	}
	if w.held {
		return false, nil
	}
	w.held = true
	w.claims = append(w.claims, category+":"+id+":"+d.String())
	return token != "", nil
}
func (w *waitTimers) ReleaseLockWithToken(context.Context, string, string, string) (bool, error) {
	w.released++
	w.held = false
	return true, w.err
}

type waitChars struct{}

func (waitChars) FindByID(_ context.Context, id string) (character.Character, error) {
	return character.Character{ID: id, PlayerID: "p", Stats: character.Stats{HP: 10}, Money: 10}, nil
}

type waitActions struct{}

func (waitActions) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func TestQueryReadsAndClaimsActionWait(t *testing.T) {
	ctx := context.Background()
	timers := &waitTimers{remaining: map[string]time.Duration{timer.CategoryActionWait: 3 * time.Second}}

	disabled := NewService(waitChars{}, waitActions{}, timers)
	if r, err := disabled.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait != 0 {
		t.Fatalf("disabled wait must not be read: %v %v", r.Snapshot.ActionWait, err)
	}
	if claim, ok, err := disabled.ClaimActionWait(ctx, "c"); err != nil || !ok || claim.Release(ctx) != nil || len(timers.claims) != 0 {
		t.Fatalf("disabled wait must not be written: %v %v %v", ok, err, timers.claims)
	}

	enabled := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, DefaultActionWait))
	r, err := enabled.Query(ctx, "c", "p")
	if err != nil || r.Snapshot.ActionWait != 3*time.Second || !slices.Equal(r.AvailableActions, []string{"place_say", "rescue_request", "presence_refresh"}) {
		t.Fatalf("enabled read: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
	claim, ok, err := enabled.ClaimActionWait(ctx, "c")
	if err != nil || !ok || len(timers.claims) != 1 || timers.claims[0] != "action_wait:c:4s" {
		t.Fatalf("claim: %v %v %v", ok, err, timers.claims)
	}
	if _, ok, err := enabled.ClaimActionWait(ctx, "c"); err != nil || ok {
		t.Fatalf("a second claim while held must fail: %v %v", ok, err)
	}
	if err := claim.Release(ctx); err != nil || timers.released != 1 {
		t.Fatalf("release: %v %d", err, timers.released)
	}
	timers.err = errors.New("down")
	if _, err := enabled.Query(ctx, "c", "p"); err == nil {
		t.Fatal("a wait read failure must fail the query, not pass silently")
	}
	if _, _, err := enabled.ClaimActionWait(ctx, "c"); err == nil {
		t.Fatal("a claim failure must be reported to the caller")
	}
}

func TestActionWaitExpiresWithTimer(t *testing.T) {
	ctx := context.Background()
	timers := timer.NewService(nil)
	s := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, 300*time.Millisecond))
	if _, ok, err := s.ClaimActionWait(ctx, "c"); err != nil || !ok {
		t.Fatal(ok, err)
	}
	if r, err := s.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait <= 0 || slices.Contains(r.AvailableActions, "scene_enter") {
		t.Fatalf("inside the wait: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
	time.Sleep(400 * time.Millisecond)
	if r, err := s.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait != 0 || !slices.Contains(r.AvailableActions, "scene_enter") {
		t.Fatalf("after the wait: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
}

// Concurrent commands race only on the atomic claim: exactly one wins.
func TestConcurrentActionWaitClaimsAdmitOne(t *testing.T) {
	run := func(t *testing.T, timers timer.Service) {
		ctx := context.Background()
		charID := id.New()
		t.Cleanup(func() { _ = timers.ReleaseLock(context.Background(), timer.CategoryActionWait, charID) })
		s := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, time.Minute))
		var wins atomic.Int32
		fns := make([]func() error, 20)
		for i := range fns {
			fns[i] = func() error {
				_, ok, err := s.ClaimActionWait(ctx, charID)
				if ok {
					wins.Add(1)
				}
				return err
			}
		}
		for _, err := range testutil.RunRace(fns...) {
			if err != nil {
				t.Fatal(err)
			}
		}
		if wins.Load() != 1 {
			t.Fatalf("%d concurrent claims won, want 1", wins.Load())
		}
	}
	t.Run("memory", func(t *testing.T) { run(t, timer.NewService(nil)) })
	t.Run("valkey", func(t *testing.T) {
		if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
			t.Skip("PARTY2_VALKEY_ADDR is not configured")
		}
		client, err := vk.NewClient()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(client.Close)
		run(t, timer.NewService(client))
	})
}
