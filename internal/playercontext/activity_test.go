package playercontext

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/core/scheduling"
)

func TestActivityAuthorityAndContinuationGates(t *testing.T) {
	s, store, readers, _ := navigationFixture(t)
	ctx := readers.ctx
	store.selection = Selection{Destination: "bank"}
	activity := Activity{Kind: "pvp", ID: "room", Role: "leader", Phase: "in_progress", Actions: []string{"pvp_advance"}}
	s.readActivities = func(got context.Context, owner, actor string) ([]Activity, error) {
		if got != ctx || owner != "owner" || actor != "hero" {
			t.Fatal("lost owned identity/context")
		}
		return []Activity{activity}, nil
	}
	r, err := s.Query(ctx, "hero", "owner")
	if err != nil || r.Snapshot.LocationID != "pvp" || r.Navigation != nil || store.reads != 0 {
		t.Fatalf("active authority: %+v %v reads=%d", r, err, store.reads)
	}
	if !slices.Contains(r.AvailableActions, "pvp_advance") || slices.Contains(r.AvailableActions, "bank_deposit") || slices.Contains(r.AvailableActions, "pvp_leave") {
		t.Fatalf("role/phase eligibility: %v", r.AvailableActions)
	}
	// Unfinished work conflicts with the room; only verified recovery is allowed.
	readers.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StateProcessing}}
	activity.Actions = []string{"pvp_advance", "gvg_leave"}
	r, err = s.Query(ctx, "hero", "owner")
	if err != nil || r.Snapshot.LocationID != "activity_conflict" || slices.Contains(r.AvailableActions, "pvp_advance") || slices.Contains(r.AvailableActions, "gvg_leave") {
		t.Fatalf("conflict: %+v %v", r, err)
	}
	activity.Kind, activity.Actions = "gvg", []string{"gvg_advance", "gvg_leave"}
	r, err = s.Query(ctx, "hero", "owner")
	if err != nil || !slices.Contains(r.AvailableActions, "gvg_leave") || !slices.Contains(r.AvailableActions, "rescue_request") || len(r.Snapshot.OngoingActions) != 1 {
		t.Fatalf("verified recovery: %+v %v", r, err)
	}
	if _, err := s.Enter(ctx, "owner", "hero", "bank"); !errors.Is(err, ErrNavigationUnavailable) {
		t.Fatalf("navigation escaped activity: %v", err)
	}
}

func TestActivityReadFailuresAndOwnership(t *testing.T) {
	s, _, readers, _ := navigationFixture(t)
	ctx := readers.ctx
	want := errors.New("activity storage unavailable")
	calls := 0
	s.readActivities = func(context.Context, string, string) ([]Activity, error) { calls++; return nil, want }
	if _, err := s.Query(ctx, "hero", "other"); !errors.Is(err, ErrNavigationForbidden) || calls != 0 {
		t.Fatalf("ownership: %v calls=%d", err, calls)
	}
	if _, err := s.Query(ctx, "hero", "owner"); !errors.Is(err, want) || calls != 1 {
		t.Fatalf("required read: %v calls=%d", err, calls)
	}
}

func TestFacilityAndTerminalWorkEligibility(t *testing.T) {
	s, store, readers, _ := navigationFixture(t)
	ctx := readers.ctx
	readers.character.Money = 100
	store.selection = Selection{Destination: "bank"}
	readers.actions = []scheduling.ScheduledAction{{ID: "completed", State: scheduling.StateCompleted}, {ID: "failed", State: scheduling.StateFailed}}
	r, err := s.Query(ctx, "hero", "owner")
	if err != nil || r.Snapshot.LocationID != "bank" || len(r.Snapshot.OngoingActions) != 0 || !slices.Contains(r.AvailableActions, "bank_deposit") || slices.Contains(r.AvailableActions, "home_sleep") || slices.Contains(r.AvailableActions, "adventure_start") {
		t.Fatalf("facility and terminal work: %+v %v", r, err)
	}
}

func TestLinkedPartyRunIsOneActivity(t *testing.T) {
	snapshot := healthySnapshot()
	snapshot.Activities = []Activity{
		{Kind: "party", ID: "party", Actions: []string{"party_ready", "party_start", "party_leave"}},
		{Kind: "dungeon", ID: "run", PartyID: "party", Actions: []string{"dungeon_move", "dungeon_escape"}},
	}
	if got := activityLocation(snapshot.ActiveActivities()); got != "dungeon" {
		t.Fatalf("linked run misclassified: %s", got)
	}
	actions := Evaluate(snapshot)
	if !slices.Contains(actions, "dungeon_move") || slices.Contains(actions, "party_start") || slices.Contains(actions, "party_ready") || !slices.Contains(actions, "party_leave") {
		t.Fatalf("linked continuation: %v", actions)
	}
	snapshot.Activities = append(snapshot.Activities, Activity{Kind: "challenge", ID: "other-run", PartyID: "party", Actions: []string{"challenge_advance"}})
	if got := activityLocation(snapshot.ActiveActivities()); got != "activity_conflict" {
		t.Fatalf("contradictory linked runs: %s", got)
	}
	if slices.Contains(Evaluate(snapshot), "dungeon_move") {
		t.Fatal("conflict exposed combat continuation")
	}
}
