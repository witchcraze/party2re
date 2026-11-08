package playercontext

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func TestActionWaitGateBlocksAllButRecovery(t *testing.T) {
	waiting := Snapshot{ActionWait: time.Second, Sleeping: true, CanWake: true, Character: character.Character{Stats: character.Stats{HP: 0}}}
	got := Evaluate(waiting)
	for _, id := range got {
		def, _ := findAction(id)
		if def.WaitExempt != WaitRecovery {
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
	set       []string
}

func (w *waitTimers) GetRemainingLock(_ context.Context, category, _ string) (time.Duration, error) {
	if category == timer.CategoryActionWait && w.err != nil {
		return 0, w.err
	}
	return w.remaining[category], nil
}
func (w *waitTimers) IsLocked(context.Context, string, string) (bool, error) { return false, nil }
func (w *waitTimers) SetLock(_ context.Context, category, id string, d time.Duration) error {
	w.set = append(w.set, category+":"+id+":"+d.String())
	return w.err
}

type waitChars struct{}

func (waitChars) FindByID(_ context.Context, id string) (character.Character, error) {
	return character.Character{ID: id, PlayerID: "p", Stats: character.Stats{HP: 10}, Money: 10}, nil
}

type waitActions struct{}

func (waitActions) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func TestQueryReadsAndStartsActionWait(t *testing.T) {
	ctx := context.Background()
	timers := &waitTimers{remaining: map[string]time.Duration{timer.CategoryActionWait: 3 * time.Second}}

	disabled := NewService(waitChars{}, waitActions{}, timers)
	if r, err := disabled.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait != 0 {
		t.Fatalf("disabled wait must not be read: %v %v", r.Snapshot.ActionWait, err)
	}
	if err := disabled.StartActionWait(ctx, "c"); err != nil || len(timers.set) != 0 {
		t.Fatalf("disabled wait must not be written: %v %v", err, timers.set)
	}

	enabled := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, DefaultActionWait))
	r, err := enabled.Query(ctx, "c", "p")
	if err != nil || r.Snapshot.ActionWait != 3*time.Second || !slices.Equal(r.AvailableActions, []string{"rescue_request"}) {
		t.Fatalf("enabled read: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
	if err := enabled.StartActionWait(ctx, "c"); err != nil || len(timers.set) != 1 || timers.set[0] != "action_wait:c:4s" {
		t.Fatalf("start: %v %v", err, timers.set)
	}
	timers.err = errors.New("down")
	if _, err := enabled.Query(ctx, "c", "p"); err == nil {
		t.Fatal("a wait read failure must fail the query, not pass silently")
	}
	if err := enabled.StartActionWait(ctx, "c"); err == nil {
		t.Fatal("a wait write failure must be reported to the caller")
	}
}

func TestActionWaitExpiresWithTimer(t *testing.T) {
	ctx := context.Background()
	timers := timer.NewService(nil)
	s := NewService(waitChars{}, waitActions{}, timers, WithActionWait(timers, 300*time.Millisecond))
	if err := s.StartActionWait(ctx, "c"); err != nil {
		t.Fatal(err)
	}
	if r, err := s.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait <= 0 || slices.Contains(r.AvailableActions, "scene_enter") {
		t.Fatalf("inside the wait: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
	time.Sleep(400 * time.Millisecond)
	if r, err := s.Query(ctx, "c", "p"); err != nil || r.Snapshot.ActionWait != 0 || !slices.Contains(r.AvailableActions, "scene_enter") {
		t.Fatalf("after the wait: %v %v %v", r.Snapshot.ActionWait, r.AvailableActions, err)
	}
}
