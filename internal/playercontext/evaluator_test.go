package playercontext

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
)

func healthySnapshot() Snapshot {
	return Snapshot{Character: character.Character{Stats: character.Stats{HP: 1}, Money: 1}, LocationID: LocationTown}
}

func TestGates_BoundariesAndFlags(t *testing.T) {
	healthy := healthySnapshot()
	for _, tc := range []struct {
		name  string
		gate  Evaluator
		flag  GateFlags
		block func(*Snapshot)
	}{
		{"dead zero", DeadGate{}, GateDeadCheck, func(s *Snapshot) { s.Character.Stats.HP = 0 }},
		{"dead negative", DeadGate{}, GateDeadCheck, func(s *Snapshot) { s.Character.Stats.HP = -1 }},
		{"fatigue 100", FatigueGate{}, GateFatigueCheck, func(s *Snapshot) { s.Character.Tired = 100 }},
		{"fatigue above 100", FatigueGate{}, GateFatigueCheck, func(s *Snapshot) { s.Character.Tired = 101 }},
		{"sleep", SleepGate{}, GateSleepCheck, func(s *Snapshot) { s.Sleeping = true }},
		{"sleep timer", SleepGate{}, GateSleepCheck, func(s *Snapshot) { s.SleepRemaining = time.Second }},
		{"pending", CooldownGate{}, GateCooldownCheck, func(s *Snapshot) { s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StatePending}} }},
		{"processing", CooldownGate{}, GateCooldownCheck, func(s *Snapshot) {
			s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StateProcessing}}
		}},
		{"money zero", CurrencyGate{}, GateCurrencyCheck, func(s *Snapshot) { s.Character.Money = 0 }},
		{"money negative", CurrencyGate{}, GateCurrencyCheck, func(s *Snapshot) { s.Character.Money = -1 }},
		{"location unknown", LocationGate{}, GateLocationCheck, func(s *Snapshot) { s.LocationID = "" }},
		{"location non-town", LocationGate{}, GateLocationCheck, func(s *Snapshot) { s.LocationID = "dungeon" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			blocked := healthy
			tc.block(&blocked)
			action := ActionDefinition{ID: "ordinary", RequiredGates: tc.flag}
			if tc.gate.Allows(action, &blocked) {
				t.Error("gate allowed a blocked action")
			}
			if !tc.gate.Allows(action, &healthy) {
				t.Error("gate blocked a healthy action")
			}
			action.RequiredGates = 0
			if !tc.gate.Allows(action, &blocked) {
				t.Error("gate blocked an exempt action")
			}
		})
	}
	belowLimit := healthy
	belowLimit.Character.Tired = 99
	if !((FatigueGate{}).Allows(ActionDefinition{RequiredGates: GateFatigueCheck}, &belowLimit)) {
		t.Error("fatigue 99 must be allowed")
	}
	for _, terminal := range []scheduling.State{scheduling.StateCompleted, scheduling.StateFailed} {
		state := healthy
		state.OngoingActions = []scheduling.ScheduledAction{{State: terminal}}
		if !((CooldownGate{}).Allows(ActionDefinition{RequiredGates: GateCooldownCheck}, &state)) {
			t.Errorf("terminal state %s blocked an action", terminal)
		}
	}
}

func TestEvaluate_RecoveryAndCatalogOrder(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*Snapshot)
		want   []string
	}{
		{"dead", func(s *Snapshot) { s.Character.Stats.HP = 0 }, []string{"home_sleep", "chapel_pray", "rescue_request"}},
		{"dead without money", func(s *Snapshot) { s.Character.Stats.HP = 0; s.Character.Money = 0 }, []string{"home_sleep", "chapel_pray", "rescue_request"}},
		{"sleeping", func(s *Snapshot) { s.Sleeping = true; s.SleepRemaining = time.Minute }, []string{"rescue_request"}},
		{"wakeable", func(s *Snapshot) { s.Sleeping = true; s.CanWake = true }, []string{"home_wake", "rescue_request"}},
		{"sleep timer only", func(s *Snapshot) { s.SleepRemaining = time.Minute }, []string{"rescue_request"}},
		{"overdue pending", func(s *Snapshot) {
			s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StatePending, ExecuteAt: time.Now().Add(-time.Hour)}}
		}, []string{"rescue_request"}},
		{"processing", func(s *Snapshot) {
			s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StateProcessing}}
		}, []string{"rescue_request"}},
		{"non-town", func(s *Snapshot) { s.LocationID = "dungeon" }, []string{"rescue_request"}},
		{"all constraints", func(s *Snapshot) {
			s.Character.Stats.HP = 0
			s.Character.Tired = 100
			s.Character.Money = 0
			s.Sleeping = true
			s.LocationID = ""
		}, []string{"rescue_request"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snapshot := healthySnapshot()
			tc.change(&snapshot)
			if got := Evaluate(snapshot); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	healthy := Evaluate(healthySnapshot())
	if len(healthy) != 41 || slices.Contains(healthy, "home_wake") {
		t.Fatalf("healthy catalog: %v", healthy)
	}
	zeroGold := healthySnapshot()
	zeroGold.Character.Money = 0
	gotZeroGold := Evaluate(zeroGold)
	if !slices.Contains(gotZeroGold, "chapel_pray") {
		t.Errorf("zero wallet gold incorrectly suppressed chapel_pray")
	}
	if !slices.Contains(gotZeroGold, "blacksmith_seal") {
		t.Errorf("zero wallet gold incorrectly suppressed blacksmith_seal")
	}
	for _, id := range []string{"casino_slot", "casino_highlow", "casino_doppel"} {
		if !slices.Contains(gotZeroGold, id) {
			t.Errorf("zero wallet gold incorrectly suppressed %s", id)
		}
	}
	exhausted := healthySnapshot()
	exhausted.Character.Tired = 100
	got := Evaluate(exhausted)
	if len(got) != 33 {
		t.Fatalf("exhausted action count = %d, want 33", len(got))
	}
	for _, action := range []string{"home_sleep", "bank_deposit", "shop_purchase", "tavern_order", "rescue_request"} {
		if !slices.Contains(got, action) {
			t.Errorf("exhausted character lost %s", action)
		}
	}
	for _, action := range []string{"adventure_start", "dungeon_start", "casino_slot"} {
		if slices.Contains(got, action) {
			t.Errorf("exhausted character can %s", action)
		}
	}
}

type recordingGate struct {
	name    string
	allow   bool
	visited *[]string
}

func (g recordingGate) Allows(_ ActionDefinition, _ *Snapshot) bool {
	*g.visited = append(*g.visited, g.name)
	return g.allow
}

func TestEvaluation_OrderAndShortCircuit(t *testing.T) {
	wantOrder := []Evaluator{DeadGate{}, FatigueGate{}, SleepGate{}, CooldownGate{}, CurrencyGate{}, LocationGate{}}
	if !reflect.DeepEqual(defaultEvaluators(), wantOrder) {
		t.Fatal("default gate order changed")
	}
	for stop := 0; stop < len(wantOrder); stop++ {
		var visited []string
		gates := make([]Evaluator, len(wantOrder))
		var want []string
		for i := range gates {
			name := reflect.TypeOf(wantOrder[i]).Name()
			gates[i] = recordingGate{name, i != stop, &visited}
			if i <= stop {
				want = append(want, name)
			}
		}
		snapshot := healthySnapshot()
		got := evaluate([]ActionDefinition{{ID: "test"}}, &snapshot, gates)
		if len(got) != 0 || !reflect.DeepEqual(visited, want) {
			t.Fatalf("stop %d: actions=%v visited=%v want=%v", stop, got, visited, want)
		}
	}
}
