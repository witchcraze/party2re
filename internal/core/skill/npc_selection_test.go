package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

type npcActionResult struct {
	NPCChoice
	NextSeed []corebattle.CallbackBinding
}

func applyNPCAction(d BattleDefinitions, s *corebattle.LifetimeState, c *corebattle.CallbackContext, seed []corebattle.CallbackBinding, rng random.Generator) (npcActionResult, error) {
	plan, err := d.npcChoicePool(s, c, seed)
	if err != nil {
		return npcActionResult{}, err
	}
	if err := s.ValidateNPCChoices(c, plan, rng); err != nil {
		return npcActionResult{}, err
	}
	choice, err := plan.SelectChoice(rng)
	if err != nil {
		return npcActionResult{}, err
	}
	if err := s.ApplyNPCChoice(c, plan.Catalog, choice, rng); err != nil {
		return npcActionResult{}, err
	}
	return npcActionResult{NPCChoice: choice, NextSeed: plan.NextSeed}, nil
}

type npcSelectionRNG struct {
	value float64
	calls int
}

func (r *npcSelectionRNG) Float64() float64 { r.calls++; return r.value }
func (r *npcSelectionRNG) Intn(n int) int   { return int(r.Float64() * float64(n)) }
func (r *npcSelectionRNG) IntN(n int) int   { return r.Intn(n) }

func npcSelectionFixture() (corebattle.LifetimeState, *corebattle.BaseGuardContext, corebattle.CallbackContext, BattleDefinitions) {
	s, c, d := baseGuardFixture()
	s.Actors[0].NPC = true
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "starter", OldJobID: "starter"}
	c.ActorNPC = true
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Damage: &c.DamageContext}
	return s, c, f, d
}

func TestNPCDefaultIndependentSPAndDuplicateBinding(t *testing.T) {
	for _, row := range []struct {
		sp, oldSP, count int
		id               string
	}{{0, 0, 1, "npc-normal-attack"}, {10, 0, 4, "job-0-entry-2"}, {0, 10, 4, "job-0-entry-2"}, {10, 9, 6, "job-0-entry-1"}} {
		s, _, f, d := npcSelectionFixture()
		s.Actors[0].Jobs.SP = row.sp
		s.Actors[0].Jobs.OldSP = row.oldSP
		rng := &npcSelectionRNG{}
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng)
		if err != nil || len(choices) != row.count || rng.calls != 0 {
			t.Fatalf("%+v: %v %+v draws%d", row, err, choices, rng.calls)
		}
		for _, choice := range choices {
			if choice.ID != row.id {
				t.Fatalf("last binding/weight: %+v", choices)
			}
		}
	}
	// A new request receives a fresh mode seed, regardless of the old actor/job.
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Jobs.SP = 30
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{}); err != nil {
		t.Fatal(err)
	}
	s.Actors[0].Jobs.SP = 0
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 1 || choices[0].ID != "npc-normal-attack" {
		t.Fatal(choices, err)
	}
	s.Actors[0].Jobs.OldSP = 10
	result, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(result.NextSeed) != 5 {
		t.Fatal(result, err)
	}
	repeat, err := d.NPCChoices(&s, &f, result.NextSeed, &npcSelectionRNG{})
	if err != nil || len(repeat) != 3 {
		t.Fatal("same-request seed was reset", repeat, err)
	}
}

func TestNPCPreviousSPAndExtensionAdmission(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 0, OldJobID: "job-02", OldSP: 5}
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 2 || choices[1].ID != "job-2-entry-1" {
		t.Fatal(choices, err)
	}
	s.Actors[0].ItemDefinitionIDs = []string{"item-158"}
	choices, err = d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 4 {
		t.Fatal(choices, err)
	}
	// MP is listed NPC MP, independent of player item249's modifier.
	s.Actors[0].MP = 2
	s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
	choices, err = d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 1 {
		t.Fatal(choices, err)
	}
}

func TestNPCActualActionLateCostAndReload(t *testing.T) {
	for _, reload := range []bool{false, true} {
		s, c, f, d := npcSelectionFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 25, OldJobID: "starter"}
		if reload {
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			var saved corebattle.LifetimeState
			if err := json.Unmarshal(data, &saved); err != nil {
				t.Fatal(err)
			}
			s = saved
			// Rebuild native closures against this operation's actual NPC context.
			d = BattleDefinitions{}
			d.RegisterBaseGuard(c)
		}
		before := s.Actors[0].MP
		choice, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
		if err != nil || choice.ID != "job-1-entry-3" || s.Actors[0].Attack != 200 || s.Actors[0].MP != before-8 {
			t.Fatal(choice, s.Actors[0], err)
		}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var saved corebattle.LifetimeState
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, saved) {
			t.Fatal("post-response reload changed result")
		}
	}
}

func TestNPCProxyAndGeneratedBindingsUseCurrentRole(t *testing.T) {
	for _, job := range []string{"job-73", "job-82"} {
		s, c, f, d := npcSelectionFixture()
		if job == "job-73" {
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 20
		} else {
			s.Actors[0].Attack, s.Actors[0].Defense, s.Actors[0].Agility = 100, 100, 407
			d.RegisterPlayerCallbacks(PlayerCallbackContexts{BaseGuard: c}, 1)
		}
		sp := 5
		if job == "job-82" {
			sp = 10
		}
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: job, SP: sp, OldJobID: "starter"}
		s.Actors[0].Icon = "assets/images/placeholders/character/character-default.svg"
		s.Actors[0].ItemDefinitionIDs = []string{"item-158"}
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &s); err != nil {
			t.Fatal(err)
		}
		choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
		if err != nil || len(choices) != 2 {
			t.Fatalf("%s %+v %v", job, choices, err)
		}
		binding := choices[1]
		if job == "job-73" && binding.ID != "job-1-entry-1" || job == "job-82" && binding.Invocation.GeneratedChildID != "job-1-entry-1" {
			t.Fatal(job, binding)
		}
		mp := s.Actors[0].MP
		if _, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999}); err != nil {
			t.Fatal(job, err)
		}
		if s.Actors[0].MP != mp-binding.MPCost || !s.Actors[0].NPC {
			t.Fatal(job, s.Actors[0])
		}
	}
}

func TestNPCCustomOrderedRepeatedSlotsAndUnsupportedClosure(t *testing.T) {
	s, c, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 200
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "custom", Gems: [3]string{"gem_guard_1", "gem_guard_1", "gem_guard_1"}, GemCosts: [3]int{20, 20, 20}, CustomCost: 72}
	choice, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
	if err != nil || !choice.Custom || len(choice.Invocations) != 3 || s.Actors[0].CMP != 128 || s.Actors[0].Temporary != corebattle.TemporaryDefense {
		t.Fatal(choice, s.Actors[0], err)
	}
	// A primitive scalar payload cannot admit an unknown explicit descendant.
	s.Actors[0].NPCLoadout.Gems[2] = "gem-101"
	d.Gems = map[string]corebattle.GemEffect{"gem-101": {Kind: corebattle.ActionKindHeal, Power: 100}}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	rng := &npcSelectionRNG{}
	if _, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || rng.calls != 0 {
		t.Fatal("unsupported descendant drew/charged/executed")
	}
	// Empty production registry cannot certify a job table using a primitive.
	d = BattleDefinitions{}
	f.Damage = &c.DamageContext
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestNPCCustomOriginalOrder(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 200
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "ordered", Gems: [3]string{"gem_guard_1", "gem_big_guard_sky", "gem_guard_1"}, GemCosts: [3]int{20, 37, 20}, CustomCost: 92}
	result, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
	if err != nil || !result.Custom || s.Actors[0].Temporary != corebattle.TemporaryDefense || s.Actors[0].CMP != 108 {
		t.Fatal(result, s.Actors[0], err)
	}
}

func TestNPCUnsupportedProducerCannotBypassSelection(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	s.Actors[0].Abilities = []string{"specter"}
	rng := &npcSelectionRNG{}
	before, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) || rng.calls != 0 {
		t.Fatal("unsupported producer mutated/drew")
	}
}

func TestNPCCustomNameOverridesPriorBindingsWithoutLosingWeight(t *testing.T) {
	s, _, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].Jobs.SP = 8
	s.Actors[0].CMP = 100
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{CustomName: "normal-attack", Gems: [3]string{"gem_guard_1"}, GemCosts: [3]int{20}, CustomCost: 20}
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 3 {
		t.Fatal(choices, err)
	}
	for _, choice := range choices {
		if !choice.Custom {
			t.Fatal("last custom binding lost", choices)
		}
	}
	result, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || !result.Custom || s.Actors[0].CMP != 80 || s.Actors[0].Temporary != corebattle.TemporaryDefense || f.Damage.DamageCalls != 0 {
		t.Fatal(result, s.Actors[0], err)
	}
}

// EX-only NPC jobs have no registered tables yet (#1914–#1916); their EX bosses
// must reject the whole response before draws or mutation.
func TestNPCEXOnlyJobsRemainWholeActionGates(t *testing.T) {
	for _, job := range []string{"job-405", "job-406", "job-407"} {
		s, _, f, d := npcSelectionFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: job, SP: 1000, OldJobID: "starter"}
		before, _ := json.Marshal(s)
		rng := &npcSelectionRNG{}
		if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(job, err)
		}
		if after, _ := json.Marshal(s); string(after) != string(before) || rng.calls != 0 {
			t.Fatal(job, "gate mutated or drew")
		}
	}
}

func TestNPCSourceNameLastWriteAcrossDifferentDisplayStrings(t *testing.T) {
	setup := func() (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions) {
		s, c, f, d := npcSelectionFixture()
		d.RegisterProtectiveMagic(&corebattle.ProtectiveMagicContext{DamageContext: c.DamageContext})
		d.RegisterControlClimax(&corebattle.ControlClimaxContext{DamageContext: c.DamageContext})
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-34", SP: 90, OldJobID: "job-70", OldSP: 150}
		s.Actors[0].MP = 40
		return s, f, d
	}

	s, f, d := setup()
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil {
		t.Fatal(err)
	}
	gotIDs := make([]string, len(choices))
	for i, ch := range choices {
		gotIDs[i] = ch.ID
	}
	wantIDs := []string{
		"npc-normal-attack",
		"job-34-entry-0",
		"job-34-entry-1",
		"job-70-entry-0",
		"job-70-entry-2",
		"job-70-entry-0",
		"job-70-entry-1",
		"job-70-entry-2",
	}
	if !slices.Equal(gotIDs, wantIDs) {
		t.Fatalf("choice order/resolution = %v, want %v", gotIDs, wantIDs)
	}
	if choices[3].Name != "meditation" || choices[5].Name != "meditation" ||
		choices[4].Name != "magic-storm" || choices[7].Name != "magic-storm" {
		t.Fatalf("unexpected resolved display names: %+v", choices)
	}

	// Deterministic selection at both duplicate positions (index 4 originally 34.3, index 7 originally 70.2).
	for _, draw := range []float64{4.0 / 8.0, 7.0 / 8.0} {
		s, f, d := setup()
		beforeEnemyHP := s.Actors[1].HP
		res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: draw})
		if err != nil || res.ID != "job-70-entry-2" || res.Name != "magic-storm" || s.Actors[0].MP != 0 || s.Actors[1].HP >= beforeEnemyHP {
			t.Fatalf("draw=%v: res=%+v actor=%+v enemy=%+v err=%v", draw, res, s.Actors[0], s.Actors[1], err)
		}
	}

	// Index 3 (originally 34.2) also resolves to 70.0.
	s, f, d = setup()
	s.Actors[0].HP = 50
	res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: 3.0 / 8.0})
	if err != nil || res.ID != "job-70-entry-0" || res.Name != "meditation" || s.Actors[0].MP != 15 || s.Actors[0].HP <= 50 {
		t.Fatalf("index 3 resolution: res=%+v actor=%+v err=%v", res, s.Actors[0], err)
	}

	// Unavailable-descendant whole-pool gate: removing the overwritten 34.3 succeeds; removing the final 70.2 rejects atomically.
	s, f, d = setup()
	delete(d.Invocations.Entries, "job-34-entry-3")
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{}); err != nil {
		t.Fatalf("overwritten 34.3 gated valid 70.2: %v", err)
	}

	s, f, d = setup()
	delete(d.Invocations.Entries, "job-70-entry-2")
	before, _ := json.Marshal(s)
	rng := &npcSelectionRNG{}
	if _, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatalf("missing 70.2 must gate whole pool, got %v", err)
	}
	if after, _ := json.Marshal(s); string(after) != string(before) || rng.calls != 0 {
		t.Fatal("gate mutated state or consumed RNG")
	}
}

func TestNPCDistinctSourceNamesWithSameDisplayStringStaySeparate(t *testing.T) {
	setup := func() (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions) {
		s, c, f, d := npcSelectionFixture()
		d.RegisterDualAgility(&corebattle.DualAgilityContext{DamageContext: c.DamageContext})
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-02", SP: 5, OldJobID: "job-83", OldSP: 5}
		s.Actors[0].MP = 3
		return s, f, d
	}

	s, f, d := setup()
	choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 3 {
		t.Fatalf("choices=%+v err=%v", choices, err)
	}
	if choices[0].ID != "npc-normal-attack" ||
		choices[1].ID != "job-2-entry-1" || choices[1].Name != "agile-strike" ||
		choices[2].ID != "job-83-entry-0" || choices[2].Name != "agile-strike" {
		t.Fatalf("coincident display names collapsed: %+v", choices)
	}

	for _, tc := range []struct {
		draw   float64
		wantID string
	}{
		{0.4, "job-2-entry-1"},
		{0.7, "job-83-entry-0"},
	} {
		s, f, d := setup()
		beforeEnemyHP := s.Actors[1].HP
		res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: tc.draw})
		if err != nil || res.ID != tc.wantID || s.Actors[0].MP != 0 || s.Actors[1].HP >= beforeEnemyHP {
			t.Fatalf("draw=%v: res=%+v actor=%+v enemy=%+v err=%v", tc.draw, res, s.Actors[0], s.Actors[1], err)
		}
	}

	// Custom skill sharing an ambiguous Go display name must not overwrite either distinct-source job entry.
	s, f, d = setup()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 100
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
		CustomName: "agile-strike",
		Gems:       [3]string{"gem_guard_1"},
		GemCosts:   [3]int{20},
		CustomCost: 20,
	}
	choices, err = d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
	if err != nil || len(choices) != 4 {
		t.Fatalf("ambiguous custom display name choices=%+v err=%v", choices, err)
	}
	if choices[1].ID != "job-2-entry-1" || choices[1].Custom ||
		choices[2].ID != "job-83-entry-0" || choices[2].Custom ||
		!choices[3].Custom {
		t.Fatalf("ambiguous custom display name overwrote job entries: %+v", choices)
	}

	// Late CMP settlement uses source command identity rather than Go display equality.
	for _, tc := range []struct {
		draw       float64
		wantID     string
		wantCustom bool
		wantMP     int
		wantCMP    int
		wantTemp   corebattle.TemporaryEffect
	}{
		{0.30, "job-2-entry-1", false, 0, 100, ""},
		{0.55, "job-83-entry-0", false, 0, 100, ""},
		{0.85, "npc-custom", true, 3, 80, corebattle.TemporaryDefense},
	} {
		s, f, d := setup()
		d.RegisterDefenseGems()
		s.Actors[0].CMP = 100
		s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
			CustomName: "agile-strike",
			Gems:       [3]string{"gem_guard_1"},
			GemCosts:   [3]int{20},
			CustomCost: 20,
		}
		beforeEnemyHP := s.Actors[1].HP
		res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: tc.draw})
		if err != nil || res.ID != tc.wantID || res.Custom != tc.wantCustom ||
			s.Actors[0].MP != tc.wantMP || s.Actors[0].CMP != tc.wantCMP ||
			s.Actors[0].Temporary != tc.wantTemp || (tc.wantCustom == (s.Actors[1].HP < beforeEnemyHP)) {
			t.Fatalf("ApplyNPCAction draw=%v: res=%+v actor=%+v enemyHP=%d err=%v", tc.draw, res, s.Actors[0], s.Actors[1].HP, err)
		}
	}
}

func TestNPCContextDependentSourceNamesAndCustomCollision(t *testing.T) {
	// job-77 temporary variant switches source name and custom-name collision.
	for _, temp := range []corebattle.TemporaryEffect{"", corebattle.TemporaryDefense} {
		for _, customName := range []string{"triple-strike", "release-six-strikes"} {
			s, c, f, d := npcSelectionFixture()
			d.RegisterDualAgility(&corebattle.DualAgilityContext{DamageContext: c.DamageContext})
			d.RegisterDefenseGems()
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-77", SP: 130, OldJobID: "starter"}
			s.Actors[0].MP, s.Actors[0].CMP = 37, 100
			s.Actors[0].Temporary = temp
			s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
				CustomName: customName,
				Gems:       [3]string{"gem_guard_1"},
				GemCosts:   [3]int{20},
				CustomCost: 20,
			}
			choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
			if err != nil || len(choices) != 6 {
				t.Fatalf("temp=%q custom=%q choices=%+v err=%v", temp, customName, choices, err)
			}
			wantCollide := (temp == "" && customName == "triple-strike") || (temp != "" && customName == "release-six-strikes")
			if choices[4].Custom != wantCollide || !choices[5].Custom {
				t.Fatalf("temp=%q custom=%q wantCollide=%v got pos4=%+v pos5=%+v", temp, customName, wantCollide, choices[4], choices[5])
			}
		}
	}

	// job-84 frozen BurnStrike (both false and true) stays consistent across NPCChoices and execution even if state mutates afterward.
	for _, tc := range []struct {
		name, initField, postField, initIcon, postIcon, gender, custom string
		burnOverride                                                   *bool
		wantBurn, wantCollide                                          bool
		wantEnemyHP                                                    int
	}{
		{"frozen false before burn field keeps base name and 2.0x damage", "", "burn", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "m", "ほのおのこぶし", nil, false, true, 892},
		{"frozen false ignores burn name even after burn field", "", "burn", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "m", "ブレイズストライク", nil, false, false, 892},
		{"frozen false by gender mismatch stays false", "burn", "burn", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "f", "ほのおのこぶし", nil, false, true, 892},
		{"frozen true stays burn name and 2.5x damage after field clears", "burn", "", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "changed", "m", "ブレイズストライク", nil, true, true, 888},
		{"frozen true ignores base name after field clears", "burn", "", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "changed", "m", "ほのおのこぶし", nil, true, false, 888},
		{"explicit frozen false overrides active burn state", "burn", "burn", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "assets/images/placeholders/battle/forms.svg#job-84-ex-m", "m", "ブレイズストライク", new(false), false, false, 892},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, f, d := npcSelectionFixture()
			s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = 0, 0
			s.Fields.Temporary = tc.initField
			s.Actors[0].Icon = tc.initIcon
			burn := s.Actors[0].LateBreathSongBurn(s.Fields.Current(), tc.gender)
			if tc.burnOverride != nil {
				burn = *tc.burnOverride
			}
			if burn != tc.wantBurn {
				t.Fatalf("burn=%v want %v", burn, tc.wantBurn)
			}
			d.RegisterLateBreathSong(&corebattle.LateBreathSongContext{
				DamageContext: c.DamageContext,
				BurnStrike:    burn,
				HeldBooks:     map[string]string{"self": "none"},
			})
			d.RegisterDefenseGems()
			// Mutate state after freezing context to verify NPCChoices honors the frozen BurnStrike value.
			s.Fields.Temporary = tc.postField
			s.Actors[0].Icon = tc.postIcon
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-84", SP: 50, OldJobID: "starter"}
			s.Actors[0].MP, s.Actors[0].CMP = 30, 100
			s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
				CustomName: tc.custom,
				Gems:       [3]string{"gem_guard_1"},
				GemCosts:   [3]int{20},
				CustomCost: 20,
			}
			choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
			if err != nil || len(choices) != 5 {
				t.Fatalf("choices=%+v err=%v", choices, err)
			}
			if choices[3].Custom != tc.wantCollide || !choices[4].Custom {
				t.Fatalf("wantCollide=%v got pos3=%+v pos4=%+v", tc.wantCollide, choices[3], choices[4])
			}
			if !tc.wantCollide {
				res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &damageGemDraws{t: t, draws: []float64{3.0 / 5.0, 0, 0}})
				if err != nil || res.ID != "job-84-entry-2" || s.Actors[1].HP != tc.wantEnemyHP {
					t.Fatalf("execution mismatch: res=%+v enemyHP=%d want=%d err=%v", res, s.Actors[1].HP, tc.wantEnemyHP, err)
				}
			}
		})
	}
}

func TestNPCSourceKeysCoverRegisteredTables(t *testing.T) {
	if EntrySourceKey("npc-normal-attack", false) == "" {
		t.Fatal("npc-normal-attack missing source name key")
	}
	for _, job := range []int{88, 90, 91, 92, 93, 94, 95, 97, 98, 99, 402, 403} {
		entries := corebattle.NPCJobEntries(job)
		if len(entries) == 0 {
			t.Fatalf("job %d returned no NPC entries", job)
		}
		for _, entry := range entries {
			if EntrySourceKey(entry.ID, false) == "" {
				t.Errorf("NPC job %d entry %s missing source name key", job, entry.ID)
			}
		}
	}
	if EntrySourceKey("job-77-entry-4-temporary", false) == "" {
		t.Fatal("job-77-entry-4-temporary missing source name key")
	}
	if k1, k2 := EntrySourceKey("job-84-entry-2", false), EntrySourceKey("job-84-entry-2", true); k1 == "" || k2 == "" || k1 == k2 {
		t.Fatalf("unexpected job-84-entry-2 source keys: %q vs %q", k1, k2)
	}
}

func TestNPCLateCMPSourceIdentitySettlement(t *testing.T) {
	// Original-name match deducts CMP even when insufficient CMP prevents custom binding.
	for _, tc := range []struct {
		name, custom string
		burn         bool
		wantCMP      int
		wantEnemyHP  int
	}{
		{"base source match charges CMP without custom binding", "ほのおのこぶし", false, -10, 910},
		{"base non-match leaves CMP unchanged", "ブレイズストライク", false, 10, 910},
		{"burn source match charges CMP without custom binding", "ブレイズストライク", true, -10, 888},
		{"burn non-match leaves CMP unchanged", "ほのおのこぶし", true, 10, 888},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, f, d := npcSelectionFixture()
			s.Actors[1].Defense, s.Actors[1].BaseStats.Defense = 0, 0
			d.RegisterLateBreathSong(&corebattle.LateBreathSongContext{
				DamageContext: c.DamageContext,
				BurnStrike:    tc.burn,
				HeldBooks:     map[string]string{"self": "none"},
			})
			d.RegisterDefenseGems()
			s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-84", SP: 50, OldJobID: "starter"}
			s.Actors[0].MP, s.Actors[0].CMP = 30, 10
			s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
				CustomName: tc.custom,
				Gems:       [3]string{"gem_guard_1"},
				GemCosts:   [3]int{20},
				CustomCost: 20,
			}
			choices, err := d.NPCChoices(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
			if err != nil || len(choices) != 4 || choices[3].Custom {
				t.Fatalf("choices=%+v err=%v", choices, err)
			}
			res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &damageGemDraws{t: t, draws: []float64{3.0 / 4.0, 0, 0}})
			if err != nil || res.ID != "job-84-entry-2" || res.Custom ||
				s.Actors[0].MP != 4 || s.Actors[0].CMP != tc.wantCMP || s.Actors[1].HP != tc.wantEnemyHP {
				t.Fatalf("res=%+v actor=%+v enemyHP=%d wantCMP=%d err=%v", res, s.Actors[0], s.Actors[1].HP, tc.wantCMP, err)
			}
		})
	}

	// Unambiguous display compatibility retains CMP deduction without custom binding.
	s, _, f, d := npcSelectionFixture()
	d.RegisterDefenseGems()
	s.Actors[0].CMP = 10
	s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
		CustomName: "normal-attack",
		Gems:       [3]string{"gem_guard_1"},
		GemCosts:   [3]int{20},
		CustomCost: 20,
	}
	res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: 0})
	if err != nil || res.ID != "npc-normal-attack" || res.Custom || s.Actors[0].CMP != -10 {
		t.Fatalf("unambiguous display fallback: res=%+v actor=%+v err=%v", res, s.Actors[0], err)
	}

	// Ambiguous display name without custom binding leaves CMP unchanged on both entries.
	for _, draw := range []float64{0.4, 0.7} {
		s, c, f, d := npcSelectionFixture()
		d.RegisterDualAgility(&corebattle.DualAgilityContext{DamageContext: c.DamageContext})
		d.RegisterDefenseGems()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-02", SP: 5, OldJobID: "job-83", OldSP: 5}
		s.Actors[0].MP, s.Actors[0].CMP = 3, 10
		s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
			CustomName: "agile-strike",
			Gems:       [3]string{"gem_guard_1"},
			GemCosts:   [3]int{20},
			CustomCost: 20,
		}
		res, err := applyNPCAction(d, &s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: draw})
		if err != nil || res.Custom || s.Actors[0].MP != 0 || s.Actors[0].CMP != 10 {
			t.Fatalf("draw=%v: res=%+v actor=%+v err=%v", draw, res, s.Actors[0], err)
		}
	}

	// Full ApplyNPCOperation path uses the same resolved source-identity CMP settlement.
	registerDualAgility := func(d *BattleDefinitions, f *corebattle.CallbackContext) {
		d.RegisterNPCCallbacks(PlayerCallbackContexts{
			BaseGuard:   &corebattle.BaseGuardContext{DamageContext: *f.Damage},
			DualAgility: &corebattle.DualAgilityContext{DamageContext: *f.Damage},
		}, f)
	}
	for _, tc := range []struct {
		draw    float64
		wantID  string
		wantMP  int
		wantCMP int
	}{
		{0.30, "job-2-entry-1", 0, 100},
		{0.55, "job-83-entry-0", 0, 100},
		{0.85, "npc-custom", 3, 80},
	} {
		s, op := runNPCFixture(t)
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-02", SP: 5, OldJobID: "job-83", OldSP: 5}
		s.Actors[0].MP, s.Actors[0].CMP = 3, 100
		s.Actors[0].NPCLoadout = &corebattle.NPCLoadout{
			CustomName: "agile-strike",
			Gems:       [3]string{"gem_guard_1"},
			GemCosts:   [3]int{20},
			CustomCost: 20,
		}
		got, err := ApplyNPCOperation(&s, op, NPCDefaultSeed(), registerDualAgility, &npcSelectionRNG{value: tc.draw})
		if err != nil || !got.NormalAction || got.Choice.ID != tc.wantID || s.Actors[0].MP != tc.wantMP || s.Actors[0].CMP != tc.wantCMP {
			t.Fatalf("ApplyNPCOperation draw=%v: got=%+v actor=%+v err=%v", tc.draw, got, s.Actors[0], err)
		}
	}
}
