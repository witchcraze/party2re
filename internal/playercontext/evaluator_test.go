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
		{"dead", func(s *Snapshot) { s.Character.Stats.HP = 0 }, []string{
			"scene_enter", "scene_select", "scene_page", "scene_back",
			"home_sleep", "chapel_pray",
			"bank_deposit", "bank_withdraw", "depot_deposit", "depot_withdraw", "auction_send", "fleamarket_list", "fleamarket_purchase",
			"shop_purchase", "shop_sell", "shop_accessory_buy", "gemstore_buy", "secretshop_purchase", "blackmarket_trade",
			"blacksmith_seal", "alchemy_synthesize", "custom_skill_set", "plantation_sow", "plantation_harvest",
			"casino_slot", "lottery_raffle", "lottery_takarakuji_buy", "tavern_order", "contest_enter",
			"wishingwell_exchange", "altar_pray", "god_wish", "job_change", "medal_claim",
			"monster_tame", "helper_complete", "park_post", "rescue_request",
			"casino_exchange", "casino_prize_exchange", "casino_room_create", "casino_room_join", "casino_room_spectate",
		}},
		{"dead without money", func(s *Snapshot) { s.Character.Stats.HP = 0; s.Character.Money = 0 }, []string{
			"scene_enter", "scene_select", "scene_page", "scene_back",
			"home_sleep", "chapel_pray",
			"bank_withdraw", "depot_deposit", "depot_withdraw", "auction_send", "fleamarket_list",
			"shop_sell", "blackmarket_trade",
			"blacksmith_seal", "alchemy_synthesize", "custom_skill_set", "plantation_sow", "plantation_harvest",
			"casino_slot", "lottery_raffle", "contest_enter",
			"wishingwell_exchange", "altar_pray", "god_wish", "job_change", "medal_claim",
			"monster_tame", "helper_complete", "park_post", "rescue_request",
			"casino_exchange", "casino_prize_exchange", "casino_room_create", "casino_room_join", "casino_room_spectate",
		}},
		{"sleeping", func(s *Snapshot) { s.Sleeping = true; s.SleepRemaining = time.Minute }, []string{"rescue_request"}},
		{"wakeable", func(s *Snapshot) { s.Sleeping = true; s.CanWake = true }, []string{"home_wake", "rescue_request"}},
		{"sleep timer only", func(s *Snapshot) { s.SleepRemaining = time.Minute }, []string{"rescue_request"}},
		{"overdue pending", func(s *Snapshot) {
			s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StatePending, ExecuteAt: time.Now().Add(-time.Hour)}}
		}, []string{"rescue_request"}},
		{"processing", func(s *Snapshot) {
			s.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StateProcessing}}
		}, []string{"rescue_request"}},
		{"non-town", func(s *Snapshot) { s.LocationID = "dungeon" }, []string{"scene_enter", "scene_select", "scene_page", "scene_back", "dungeon_start", "rescue_request"}},
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
	if len(healthy) != 48 || slices.Contains(healthy, "home_wake") {
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
	for _, id := range []string{"casino_slot", "casino_prize_exchange", "casino_room_join"} {
		if !slices.Contains(gotZeroGold, id) {
			t.Errorf("zero wallet gold incorrectly suppressed %s", id)
		}
	}
	exhausted := healthySnapshot()
	exhausted.Character.Tired = 100
	got := Evaluate(exhausted)
	if len(got) != 41 {
		t.Fatalf("exhausted action count = %d, want 41", len(got))
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

func TestEvaluate_DeadNonCombatActions(t *testing.T) {
	s := Snapshot{Character: character.Character{Money: 100}, LocationID: LocationTown}
	for _, id := range []string{"bank_withdraw", "park_post", "shop_purchase"} {
		t.Run(id, func(t *testing.T) {
			if !slices.Contains(Evaluate(s), id) {
				t.Errorf("noncombat legacy action hidden solely by HP=0: %s", id)
			}
		})
	}
}

func TestEvaluate_LivingVsDeadCombatContrast(t *testing.T) {
	combatActions := []string{
		"adventure_start", "challenge_start", "dungeon_start", "boss_fight", "pvp_room_create",
	}
	sampleNonCombatActions := []string{
		"bank_withdraw", "bank_deposit", "shop_purchase", "shop_sell",
		"blacksmith_seal", "casino_slot", "job_change", "park_post",
	}

	living := healthySnapshot()
	living.Character.Stats.HP = 100
	living.Character.Money = 100
	gotLiving := Evaluate(living)

	dead := healthySnapshot()
	dead.Character.Stats.HP = 0
	dead.Character.Money = 100
	gotDead := Evaluate(dead)

	// Combat actions must be available to living characters, but blocked for dead characters
	for _, act := range combatActions {
		t.Run("combat/"+act, func(t *testing.T) {
			if !slices.Contains(gotLiving, act) {
				t.Errorf("living character missing combat action: %s", act)
			}
			if slices.Contains(gotDead, act) {
				t.Errorf("dead character must NOT be offered combat action: %s", act)
			}
		})
	}

	// Noncombat actions must be available to both living and dead characters
	for _, act := range sampleNonCombatActions {
		t.Run("noncombat/"+act, func(t *testing.T) {
			if !slices.Contains(gotLiving, act) {
				t.Errorf("living character missing noncombat action: %s", act)
			}
			if !slices.Contains(gotDead, act) {
				t.Errorf("dead character incorrectly blocked from noncombat action: %s", act)
			}
		})
	}

	// When dead, SleepGate and CooldownGate must still be enforced
	deadAsleep := dead
	deadAsleep.Sleeping = true
	deadAsleep.SleepRemaining = time.Minute
	gotDeadAsleep := Evaluate(deadAsleep)
	if !reflect.DeepEqual(gotDeadAsleep, []string{"rescue_request"}) {
		t.Errorf("dead and sleeping character should only have rescue_request, got %v", gotDeadAsleep)
	}

	deadPending := dead
	deadPending.OngoingActions = []scheduling.ScheduledAction{{State: scheduling.StatePending}}
	gotDeadPending := Evaluate(deadPending)
	if !reflect.DeepEqual(gotDeadPending, []string{"rescue_request"}) {
		t.Errorf("dead character with pending action should only have rescue_request, got %v", gotDeadPending)
	}
}
