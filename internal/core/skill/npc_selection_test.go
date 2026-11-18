package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

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
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
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
		choice, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
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
		if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999}); err != nil {
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
	choice, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
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
	if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
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
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: .999})
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
	if _, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), rng); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
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
	result, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{})
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
		res, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: draw})
		if err != nil || res.ID != "job-70-entry-2" || res.Name != "magic-storm" || s.Actors[0].MP != 0 || s.Actors[1].HP >= beforeEnemyHP {
			t.Fatalf("draw=%v: res=%+v actor=%+v enemy=%+v err=%v", draw, res, s.Actors[0], s.Actors[1], err)
		}
	}

	// Index 3 (originally 34.2) also resolves to 70.0.
	s, f, d = setup()
	s.Actors[0].HP = 50
	res, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: 3.0 / 8.0})
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
		res, err := d.ApplyNPCAction(&s, &f, NPCDefaultSeed(), &npcSelectionRNG{value: tc.draw})
		if err != nil || res.ID != tc.wantID || s.Actors[0].MP != 0 || s.Actors[1].HP >= beforeEnemyHP {
			t.Fatalf("draw=%v: res=%+v actor=%+v enemy=%+v err=%v", tc.draw, res, s.Actors[0], s.Actors[1], err)
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

	// job-84 burn-strike context switches source name key and custom-name collision.
	for _, tc := range []struct {
		name, field, gender, custom string
		burnFlag                    bool
		wantCollide                 bool
	}{
		{"non-burn gender mismatch collides base", "burn", "f", "ほのおのこぶし", false, true},
		{"non-burn gender mismatch ignores burn name", "burn", "f", "ブレイズストライク", false, false},
		{"burn active collides burn name", "burn", "m", "ブレイズストライク", false, true},
		{"burn active ignores base name", "burn", "m", "ほのおのこぶし", false, false},
		{"frozen burn flag overrides empty field", "", "f", "ブレイズストライク", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, f, d := npcSelectionFixture()
			d.RegisterTransformations(&corebattle.TransformationContext{RequesterGender: tc.gender})
			d.RegisterLateBreathSong(&corebattle.LateBreathSongContext{
				DamageContext: c.DamageContext,
				BurnStrike:    tc.burnFlag,
				HeldBooks:     map[string]string{"self": "none"},
			})
			d.RegisterDefenseGems()
			s.Fields.Temporary = tc.field
			s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#job-84-ex-m"
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
