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

func jobEncounterFixture() (corebattle.LifetimeState, *corebattle.JobEncounterContext, corebattle.CallbackContext, BattleDefinitions) {
	s, base, _ := dualAgilityFixture()
	c := &corebattle.JobEncounterContext{DamageContext: base.DamageContext}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
	var d BattleDefinitions
	d.RegisterPlayerCallbacks(PlayerCallbackContexts{JobEncounters: c, Transformations: &corebattle.TransformationContext{}}, 2)
	return s, c, f, d
}

func executeJobEncounter(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, id string, draws ...float64) {
	t.Helper()
	ops, err := d.InvocationAction(s, f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(f, d.Invocations, ops, rng); err != nil {
		t.Fatal(id, err)
	}
	if rng.used != len(draws) {
		t.Fatal(id, rng.used, len(draws))
	}
}

// Independent source vectors: ff97fc5:_skill.cgi:6119–6133,6147–6174,
// 6201–6234,6258–6275. Enclosing callers retain listed MP costs.
func TestJobEncounterShortcutBoundariesAndNoEffect(t *testing.T) {
	for _, tc := range []struct {
		floor  int
		draw   float64
		called bool
	}{{8, 0, true}, {8, 1.0 / 3, false}, {9, 0, false}} {
		s, c, f, d := jobEncounterFixture()
		calls := 0
		c.Room = &corebattle.JobRoomContext{NormalStage: true, Floor: tc.floor,
			Validate: func(string) error { return nil },
			Apply:    func(string) (corebattle.TargetContext, error) { calls++; return f.TargetContext, nil },
		}
		draws := []float64{tc.draw}
		if tc.floor == 9 {
			c.Room.Apply, c.Room.Validate = nil, nil
			draws = nil
		}
		executeJobEncounter(t, &s, &f, d, "job-78-entry-2", draws...)
		if (calls == 1) != tc.called || s.Actors[0].MP != 80 || len(f.Arguments) != 2 {
			t.Fatal(tc, calls, s)
		}
	}
}

func TestJobEncounterTreasureRetrySPAndTargetResolution(t *testing.T) {
	for _, tc := range []struct {
		target  string
		draw    float64
		success bool
	}{{"box", 0, true}, {"box", .5999, true}, {"box", .6, false}, {"enemy", 0, false}} {
		s, c, f, d := jobEncounterFixture()
		s.Actors[0].Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 240, OldJobID: "job-78", OldSP: 230}
		s.Actors[2].Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 450, OldJobID: "job-78", OldSP: 440}
		f.Arguments[0] = tc.target
		calls := 0
		c.Room = &corebattle.JobRoomContext{NormalStage: true, Floor: 11, TreasureIDs: []string{"box"},
			Validate: func(string) error { return nil },
			Apply:    func(string) (corebattle.TargetContext, error) { calls++; return f.TargetContext, nil },
		}
		draws := []float64{tc.draw}
		if tc.target != "box" {
			draws = nil
			c.Room.Apply, c.Room.Validate = nil, nil
		}
		executeJobEncounter(t, &s, &f, d, "job-78-entry-6", draws...)
		selfSP := 240
		if tc.target == "box" {
			selfSP = 100
		}
		allySP := 450
		if tc.success {
			allySP = 100
		}
		if calls != map[bool]int{true: 1}[tc.success] || s.Actors[0].Jobs.SP != selfSP || s.Actors[2].Jobs.SP != allySP ||
			s.Actors[0].Jobs.OldSP != map[bool]int{true: 100, false: 230}[tc.target == "box"] || s.Actors[2].Jobs.OldSP != map[bool]int{true: 100, false: 440}[tc.success] || len(c.Arguments) != 1 {
			t.Fatal(tc, calls, s.Actors, f.Arguments)
		}
	}
}

func TestJobEncounterOrderingCursorAndTailGuards(t *testing.T) {
	for _, mode := range []string{"hit", "fallback", "self", "ally", "dead", "reflect", "blocked", "immobilize"} {
		t.Run(mode, func(t *testing.T) {
			s, c, f, d := jobEncounterFixture()
			second := s.Actors[1].Clone()
			second.ID = "second"
			s.Actors = append(s.Actors, second)
			f.EnemyIDs = []string{"enemy", "second"}
			f.Arguments[0] = "second" // The submitted target is ignored even through a wrapper.
			c.Cursor = corebattle.TargetCursor{Source: "item159", ActorID: "enemy"}
			draws := []float64{1.0 / 6, 0}
			switch mode {
			case "fallback":
				c.Cursor = corebattle.TargetCursor{}
				draws = []float64{.5, 0, 0}
			case "self":
				c.Cursor.ActorID = "self"
				draws = []float64{.5, 0, 0} // ally cursor triggers ordinary enemy fallback.
			case "ally":
				c.Cursor.ActorID = "self"
				s.Actors[0].Status = corebattle.StatusConfusion
			case "dead":
				s.Actors[1].HP = 0
				draws = []float64{.5}
			case "reflect":
				s.Actors[1].Temporary = corebattle.TemporaryReflectBreath
			case "blocked":
				s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
				draws = []float64{.5}
			case "immobilize":
				s.Actors[0].Tension = 5
				draws = []float64{0}
			}
			executeJobEncounter(t, &s, &f, d, "job-78-entry-4", draws...)
			reordered := mode == "hit" || mode == "fallback" || mode == "self"
			if (f.EnemyIDs[0] == "second") != reordered || f.Arguments[0] != "second" {
				t.Fatal(mode, f, s)
			}
			if mode == "hit" && s.Actors[1].HP != 955 {
				t.Fatal("direct breath base50", s.Actors[1])
			}
			if mode == "immobilize" && (s.Actors[0].Status != corebattle.StatusDofuu || s.Actors[0].Tension != 1) {
				t.Fatal(s.Actors[0])
			}
		})
	}
}

func TestJobEncounterBindingsAndIndirectOrdering(t *testing.T) {
	s, c, f, d := jobEncounterFixture()
	for _, row := range []struct {
		job         string
		pos, sp, mp int
		id          string
	}{
		{"job-78", 2, 30, 10, "job-78-entry-2"}, {"job-78", 4, 70, 18, "job-78-entry-4"},
		{"job-78", 6, 200, 0, "job-78-entry-6"}, {"job-79", 2, 55, 30, "job-79-entry-2"},
	} {
		table, err := d.PlayerCallbackTable(&s, &f, row.job)
		if err != nil || table[row.pos].ID != row.id || table[row.pos].RequiredSP != row.sp || table[row.pos].MPCost != row.mp {
			t.Fatal(row, table, err)
		}
	}
	// Use the complete native table shapes. Other explicit test ports certify
	// only this connection, not the intrinsic native random closure.
	bound := d.Invocations.Entries["job-78-entry-4"]
	for _, table := range d.Invocations.JobTables {
		for _, variant := range table.Variants {
			for _, entry := range variant {
				d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{
					Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
					Execute:  func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error { return nil },
				}
			}
		}
	}
	d.Invocations.Entries["job-78-entry-4"] = bound
	second := s.Actors[1].Clone()
	second.ID = "second"
	s.Actors = append(s.Actors, second)
	f.EnemyIDs = []string{"enemy", "second"}
	f.Arguments[0] = "second"
	c.Cursor = corebattle.TargetCursor{Source: "item159", ActorID: "enemy"}
	rng := &damageGemDraws{t: t, draws: []float64{77.1 / 87, 4.1 / 7, .5, 0}}
	if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "random"}}, rng); err != nil {
		t.Fatal(err)
	}
	if s.Actors[1].HP != 955 || s.Actors[3].HP != 1000 || f.EnemyIDs[0] != "second" || rng.used != 4 {
		t.Fatal(s, f, rng.used)
	}
}

func TestJobEncounterTimePlayerRemovalMembersAndReload(t *testing.T) {
	for _, draw := range []float64{0, .5} {
		s, c, f, d := jobEncounterFixture()
		second := s.Actors[1].Clone()
		second.ID = "second"
		s.Actors = append(s.Actors, second)
		f.EnemyIDs = []string{"enemy", "second"}
		executeJobEncounter(t, &s, &f, d, "job-79-entry-2", draw)
		if !slices.Equal(f.EnemyIDs, []string{"second"}) || !slices.Equal(f.MemberOnlyIDs, []string{"enemy"}) || s.Actors[3].ID != "enemy" || !c.ResponseSuppressed || s.SkillBans["job-79-entry-2"] != 4 || (s.Actors[0].Status == corebattle.StatusSleep) != (draw < .5) {
			t.Fatal(s, c, f)
		}
		s.TickSkillBans()
		data, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		var loaded corebattle.LifetimeState
		if err := json.Unmarshal(data, &loaded); err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(s, loaded) || loaded.SkillBans["job-79-entry-2"] != 3 {
			t.Fatal(loaded)
		}
		// Even with the removed member, later operation helpers retain a valid frame.
		if _, err := d.InvocationAction(&loaded, &f, []string{"job-79-entry-2"}, noArithmeticDraws{t}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestJobEncounterAdmissionRejectsMissingContextBeforeDraw(t *testing.T) {
	for _, mode := range []string{"room", "mode", "cursor", "unknown-later", "unknown-prior", "npc-closure"} {
		s, c, f, d := jobEncounterFixture()
		id := "job-78-entry-2"
		ids := []string{id}
		switch mode {
		case "mode":
			c.Room = &corebattle.JobRoomContext{Floor: 8}
		case "cursor":
			id = "job-78-entry-4"
			ids[0] = id
			c.Cursor = corebattle.TargetCursor{Source: "submitted", ActorID: "enemy"}
		case "unknown-later":
			id = "job-79-entry-2"
			ids = []string{id, "missing"}
		case "unknown-prior":
			ids = []string{"job-79-entry-2", "job-78-entry-4"}
		case "npc-closure":
			id = "job-79-entry-2"
			ids[0] = id
			s.Actors[0].NPC = true
		}
		before, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(mode, err)
		}
		after, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) || c.DamageCalls != 0 || c.Arguments != nil {
			t.Fatal(mode, "admission mutated state")
		}
	}
}

// Full table shapes with stand-in child bodies isolate this wrapper's draw/
// death contract. They do not certify the native descendant bodies for #1733.
func TestJobEncounterTimeNPCDoubleRandomStopsAfterDeath(t *testing.T) {
	for _, die := range []bool{false, true} {
		t.Run(map[bool]string{false: "two-calls", true: "death-after-first"}[die], func(t *testing.T) {
			s, c, f, d := jobEncounterFixture()
			s.Actors[0].NPC = true
			second := s.Actors[1].Clone()
			second.ID = "second"
			s.Actors = append(s.Actors, second)
			f.EnemyIDs = []string{"enemy", "second"}
			f.Command = "outer-command"
			damage := c.DamageContext
			damage.TargetContext, damage.DamageCalls = f.TargetContext, 7
			f.Damage = &damage
			f.Defeat = &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{
				TargetContext: f.TargetContext, Cursor: corebattle.TargetCursor{Source: "item150", ActorID: "self"},
			}}
			timeBody := d.Invocations.Entries["job-79-entry-2"]
			calls := 0
			var lastTarget corebattle.TargetContext
			var lastDamage *corebattle.DamageContext
			var lastDefeat *corebattle.DefeatContext
			for _, table := range d.Invocations.JobTables {
				for _, variant := range table.Variants {
					for _, entry := range variant {
						d.Invocations.Entries[entry.ID] = corebattle.CallbackDefinition{
							Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
							Execute: func(state *corebattle.LifetimeState, frame *corebattle.CallbackContext, _ random.Generator) error {
								if calls > 0 && (!reflect.DeepEqual(frame.TargetContext, lastTarget) || frame.Damage != lastDamage || frame.Defeat != lastDefeat || frame.Damage.DamageCalls != 8 || len(frame.Arguments) != 1) {
									t.Fatal("second random callback lost the first callback's context", frame)
								}
								calls++
								frame.EnemyIDs, frame.MemberOnlyIDs = []string{"second"}, []string{"enemy"}
								if calls == 2 {
									frame.EnemyIDs, frame.MemberOnlyIDs = []string{"enemy"}, []string{"second"}
								}
								frame.PhantomTargetID, frame.Item195Activated = frame.EnemyIDs[0], true
								frame.Arguments = slices.Clone(frame.Arguments[1:])
								damage := *frame.Damage
								damage.TargetContext = frame.TargetContext
								damage.DamageCalls++
								damage.HealCalls += 2
								damage.Cursor = corebattle.TargetCursor{Source: "item159", ActorID: frame.PhantomTargetID}
								frame.Damage = &damage
								defeat := *frame.Defeat
								defeat.TargetContext, defeat.Cursor = frame.TargetContext, damage.Cursor
								frame.Defeat = &defeat
								lastTarget, lastDamage, lastDefeat = frame.TargetContext, frame.Damage, frame.Defeat
								if die {
									state.Actors[0].HP = 0
								}
								return nil
							},
						}
					}
				}
			}
			// Keep the body under test outside its recursive random table closure.
			d.Invocations.Entries["npc-time-probe"] = timeBody
			draws := []float64{0, .2, 0, .2} // Job1 position1, excluding its extended position0.
			want := 2
			if die {
				draws, want = draws[:2], 1
			}
			executeJobEncounter(t, &s, &f, d, "npc-time-probe", draws...)
			if calls != want || c.ResponseSuppressed || len(s.SkillBans) != 0 || s.Actors[0].Status != "" {
				t.Fatal(die, calls, s, c)
			}
			if !reflect.DeepEqual(f.TargetContext, lastTarget) || !reflect.DeepEqual(f.Damage.TargetContext, lastTarget) || !reflect.DeepEqual(f.Defeat.TargetContext, lastTarget) || f.Damage != lastDamage || f.Defeat != lastDefeat ||
				f.Damage.DamageCalls != 7+want || f.Damage.HealCalls != 2*want ||
				f.Damage.Cursor != (corebattle.TargetCursor{Source: "item159", ActorID: lastTarget.PhantomTargetID}) || f.Defeat.Cursor != f.Damage.Cursor {
				t.Fatal("parent overwrote the final random callback's context", die, f, c)
			}
			if !reflect.DeepEqual(c.TargetContext, lastTarget) || c.DamageCalls != 7+want || c.Cursor != f.Defeat.Cursor || c.DefeatContext != lastDefeat || len(c.Arguments) != 2-want {
				t.Fatal("operation context did not retain delegated results", die, c)
			}
		})
	}
}
