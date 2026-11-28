package playercontext

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
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
			// ＠さそう sets act_time = 0 whatever the room's pace (_skill.cgi:772, _casino.cgi:127).
			if def.WaitExempt != WaitActivityPacing && !(def.WaitExempt == WaitWhisper && def.LegacyVerb == "さそう") {
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
		case def.ID == "casino_slot":
			if def.WaitExempt != WaitPrivate {
				t.Errorf("%s: suroto's tenth of the wait truncates to none (casino.cgi:552-554, system.cgi:159)", def.ID)
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
		case def.ID == "home_notice_ack":
			if def.WaitExempt != WaitReload {
				t.Errorf("%s: legacy clears notices while rendering the own home (home.cgi:49-68)", def.ID)
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

func TestActionPaceComesFromTheMatchingActivity(t *testing.T) {
	snapshot := Snapshot{Activities: []Activity{{Kind: "pvp"}, {Kind: "casino", Pace: 12 * time.Second}}}
	for id, want := range map[string]time.Duration{"casino_room_action": 12 * time.Second, "casino_room_leave": 12 * time.Second, "pvp_leave": 0, "casino_exchange": 0} {
		def, _ := findAction(id)
		if got := ActionPace(def, snapshot); got != want {
			t.Errorf("%s: pace %v, want %v", id, got, want)
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
func (w *waitTimers) ImposeLockUntil(_ context.Context, category, id string, until time.Time) error {
	w.claims = append(w.claims, "impose:"+category+":"+id+":"+until.UTC().Format(time.RFC3339))
	return w.err
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
	if claim, ok, err := disabled.ClaimActionWait(ctx, "c", 0); err != nil || !ok || claim.Release(ctx) != nil || len(timers.claims) != 0 {
		t.Fatalf("disabled wait must not be written: %v %v %v", ok, err, timers.claims)
	}

	enabled := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, DefaultActionWait))
	r, err := enabled.Query(ctx, "c", "p")
	if err != nil || r.Snapshot.ActionWait != 3*time.Second || !slices.Equal(r.AvailableActions, []string{"home_notice_ack", "place_say", "rescue_request", "presence_refresh"}) {
		t.Fatalf("enabled read: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
	claim, ok, err := enabled.ClaimActionWait(ctx, "c", 0)
	if err != nil || !ok || len(timers.claims) != 1 || timers.claims[0] != "action_wait:c:4s" {
		t.Fatalf("claim: %v %v %v", ok, err, timers.claims)
	}
	if _, ok, err := enabled.ClaimActionWait(ctx, "c", 0); err != nil || ok {
		t.Fatalf("a second claim while held must fail: %v %v", ok, err)
	}
	if err := claim.Release(ctx); err != nil || timers.released != 1 {
		t.Fatalf("release: %v %d", err, timers.released)
	}
	if claim, ok, err := enabled.ClaimActionWait(ctx, "c", 18*time.Second); err != nil || !ok || timers.claims[1] != "action_wait:c:18s" || claim.Release(ctx) != nil {
		t.Fatalf("a pace replaces the default: %v %v %v", ok, err, timers.claims)
	}
	until := time.Date(2026, 11, 16, 12, 0, 0, 0, time.UTC)
	if err := enabled.ImposeActionWaitUntil(ctx, "k", until); err != nil || timers.claims[2] != "impose:action_wait:k:2026-11-16T12:00:00Z" {
		t.Fatalf("impose: %v %v", err, timers.claims)
	}
	if err := disabled.ImposeActionWaitUntil(ctx, "k", until); err != nil || len(timers.claims) != 3 {
		t.Fatalf("disabled wait must not be imposed: %v %v", err, timers.claims)
	}
	timers.err = errors.New("down")
	if _, err := enabled.Query(ctx, "c", "p"); err == nil {
		t.Fatal("a wait read failure must fail the query, not pass silently")
	}
	if _, _, err := enabled.ClaimActionWait(ctx, "c", 0); err == nil {
		t.Fatal("a claim failure must be reported to the caller")
	}
}

func TestActionWaitExpiresWithTimer(t *testing.T) {
	ctx := context.Background()
	timers := timer.NewService(nil)
	s := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, 300*time.Millisecond))
	if _, ok, err := s.ClaimActionWait(ctx, "c", 0); err != nil || !ok {
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
		t.Cleanup(func() {
			if err := timers.ReleaseLock(context.Background(), timer.CategoryActionWait, charID); err != nil {
				t.Errorf("cleanup ReleaseLock action_wait %s: %v", charID, err)
			}
		})
		s := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, time.Minute))
		var wins atomic.Int32
		fns := make([]func() error, 20)
		for i := range fns {
			fns[i] = func() error {
				_, ok, err := s.ClaimActionWait(ctx, charID, 0)
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
		client := testutil.OpenValkey(t)
		run(t, timer.NewService(client))
	})
}
