package adventure

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

func operationFixture() (*NormalRun, NormalOperationContext) {
	r := &NormalRun{
		Floor: 1, LeaderID: "a", Roster: []string{"a", "b"}, Enemies: []string{"c"},
		Battle: corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{
			{Participant: corebattle.Participant{ID: "a", Name: "Alice", TeamID: "party", HP: 100, MaxHP: 100, MP: 120, MaxMP: 120}},
			{Participant: corebattle.Participant{ID: "b", Name: "Bob", TeamID: "party", HP: 100, MaxHP: 100}},
			{Participant: corebattle.Participant{ID: "c", Name: "Enemy", TeamID: "enemy", HP: 100, MaxHP: 100}, NPC: true},
		}},
	}
	level := 10
	damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{RequesterLevel: &level}}
	ctx := NormalOperationContext{Callback: corebattle.CallbackContext{Damage: &damage}}
	ctx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{
		BaseGuard:       &corebattle.BaseGuardContext{DamageContext: damage},
		BasicMagic:      &corebattle.BasicMagicContext{DamageContext: damage},
		ProtectiveMagic: &corebattle.ProtectiveMagicContext{DamageContext: damage},
		Transformations: &corebattle.TransformationContext{},
		Revival:         &corebattle.RevivalContext{},
	}, 2)
	return r, ctx
}

func TestDiscoverNormalOperationGates(t *testing.T) {
	for _, tc := range []struct {
		name, id string
		change   func(*NormalRun, *NormalOperationContext)
		listed   bool
		cost     int
	}{
		{"attack", "attack", nil, true, 0},
		{"defend", "defend", nil, true, 0},
		{"tension", "tension", nil, true, 0},
		{"previous SP with current SP zero", "job-1-entry-1", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").Jobs = corebattle.BattleJobs{JobID: "job-06", OldJobID: "job-01", OldSP: 5}
		}, true, 5},
		{"current SP cannot pay previous threshold", "job-1-entry-1", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").Jobs = corebattle.BattleJobs{JobID: "job-06", SP: 80, OldJobID: "job-01", OldSP: 4}
		}, false, 0},
		{"item249 truncates odd magician amount", "job-6-entry-3", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.MP, a.ItemDefinitionIDs = "job-06", 8, 2, []string{"item-249"}
		}, true, 2},
		{"item249 doubles non-magician amount", "job-1-entry-1", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.MP, a.ItemDefinitionIDs = "job-01", 5, 10, []string{"item-249"}
		}, true, 10},
		{"item249 doubled amount too expensive", "job-1-entry-1", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.MP, a.ItemDefinitionIDs = "job-01", 5, 9, []string{"item-249"}
		}, false, 0},
		{"magician amulet extended SP zero", "job-6-entry-0", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.ItemDefinitionIDs = "job-06", 1, []string{"item-249"}
		}, true, 0},
		{"warrior amulet wrong job", "job-6-entry-0", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.ItemDefinitionIDs = "job-06", 1, []string{"item-181"}
		}, false, 0},
		{"shift gear via item240", "job-73-gear-previous", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.OldJobID, a.Jobs.OldSP, a.ItemDefinitionIDs = "job-73", 1, []string{"item-240"}
		}, true, 1},
		{"curse hides job but keeps book", "book-172", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").Abilities, r.actor("a").ItemDefinitionIDs = []string{"cursed"}, []string{"item-249", "item-172"}
		}, true, 18},
		{"curse hides job", "job-1-entry-1", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.SP, a.Abilities = "job-01", 5, []string{"cursed"}
		}, false, 0},
		{"equipped extension independent of previous SP", "extension-163", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").ItemDefinitionIDs = []string{"item-163"}
		}, true, 12},
		{"seal requires previous SP", "seal-8-magic", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Gear, a.Jobs.OldSP, a.WeaponSeal = &corebattle.BattleGearStats{WeaponID: "weapon-50"}, 90, 8
			a.ItemDefinitionIDs = []string{"item-249"}
		}, true, 54},
		{"seal cannot use current SP", "seal-8-magic", func(r *NormalRun, _ *NormalOperationContext) {
			a := r.actor("a")
			a.Gear, a.Jobs.SP, a.Jobs.OldSP, a.WeaponSeal = &corebattle.BattleGearStats{WeaponID: "weapon-50"}, 120, 89, 8
		}, false, 0},
		{"category 1 tool at floor zero", "tool", func(r *NormalRun, _ *NormalOperationContext) {
			r.Floor, r.Enemies, r.actor("a").HeldToolID = 0, nil, new("item-001")
		}, true, 0},
		{"category 2 item", "tool", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").HeldToolID = new("item-033")
		}, false, 0},
		{"equipped inventory cannot infer hand", "tool", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("a").ItemDefinitionIDs = []string{"item-001"}
		}, false, 0},
		{"between encounters survival", "between-encounter-survival", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("c").HP, r.actor("a").Jobs.JobID, r.actor("a").Jobs.SP = 0, "job-78", 55
		}, true, 10},
		{"between encounters reverse form wins", "job-300-form", func(r *NormalRun, _ *NormalOperationContext) {
			r.actor("c").HP = 0
			r.actor("a").Jobs = corebattle.BattleJobs{JobID: "job-82", SP: 111, OldJobID: "job-300"}
		}, true, 0},
		{"call at floor zero uses previous SP and item165 override", "job-59-summon", func(r *NormalRun, _ *NormalOperationContext) {
			r.Floor, r.Enemies = 0, nil
			a := r.actor("a")
			a.Jobs.JobID, a.Jobs.OldSP, a.ItemDefinitionIDs = "job-59", 10, []string{"item-165"}
		}, true, 0},
		{"front with two allies", "front", nil, true, 0},
		{"back with solo member", "back", func(r *NormalRun, _ *NormalOperationContext) {
			r.Battle.Actors = slices.Delete(r.Battle.Actors, 1, 2)
			r.Roster = []string{"a"}
		}, false, 0},
		{"stock uses persistent exp at threshold", "stock-level", func(r *NormalRun, ctx *NormalOperationContext) {
			r.Enemies = nil
			ctx.Requester = &corecharacter.Character{Level: 100, OverLevel: true, Experience: 100000}
		}, true, 0},
		{"stock refuses capped ordinary level", "stock-level", func(r *NormalRun, ctx *NormalOperationContext) {
			r.Enemies = nil
			ctx.Requester = &corecharacter.Character{Level: 99, Experience: 100000}
		}, false, 0},
		{"invite for nonleader", "invite", func(r *NormalRun, _ *NormalOperationContext) { r.Floor, r.LeaderID = 0, "b" }, true, 0},
		{"kick for leader", "kick", func(r *NormalRun, _ *NormalOperationContext) { r.Floor = 0 }, true, 0},
		{"kick withheld from nonleader", "kick", func(r *NormalRun, _ *NormalOperationContext) { r.Floor, r.LeaderID = 0, "b" }, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ctx := operationFixture()
			if tc.change != nil {
				tc.change(r, &ctx)
			}
			choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
			if err != nil {
				t.Fatal(err)
			}
			i := slices.IndexFunc(choices, func(c RunOperationChoice) bool { return c.Action == tc.id })
			if (i >= 0) != tc.listed || i >= 0 && choices[i].MPCost != tc.cost {
				t.Fatalf("choice %s, want listed=%t MP=%d, got %+v", tc.id, tc.listed, tc.cost, choices)
			}
		})
	}
}

func TestParseNormalOperationTargetsAndDispatch(t *testing.T) {
	r, ctx := operationFixture()
	r.actor("a").Jobs = corebattle.BattleJobs{JobID: "job-01", SP: 8}
	for _, tc := range []struct {
		name, action, target, input string
		kind                        RunOperationKind
		binding, gotTarget          string
		wantErr                     error
	}{
		{"omission", "defend", "", "", RunOperationBattle, "job-0-entry-4", "", nil},
		{"wrong side remains supplied", "defend", "c", "", RunOperationBattle, "job-0-entry-4", "c", nil},
		{"stale", "defend", "previous-enemy", "", "", "", "", ErrRunConflict},
		{"foreign", "job-1-entry-2", "foreign", "", "", "", "", ErrRunConflict},
		{"plain text", "", "", "a comment", RunOperationComment, "", "", nil},
		{"unknown text command", "comment", "", "＠no-such-skill>foreign", RunOperationComment, "", "", nil},
		{"unknown typed command", "no-such-skill", "", "", "", "", "", ErrRunUnavailable},
		{"whisper outside the encounter", "", "", "＠ささやき&gt;Carol", RunOperationWhisper, "whisper", "Carol", nil},
		{"screenshot", "", "", "＠すくしょ", RunOperationScreenshot, "screenshot", "", nil},
		{"legacy name and wrong-side alias", "", "", "text ＠ぼうぎょ　>Enemy", RunOperationBattle, "job-0-entry-4", "c", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ParseNormalOperation(RunCommand{ActorID: "a", Action: tc.action, TargetID: tc.target, Input: tc.input}, ctx)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v want %v", err, tc.wantErr)
			}
			if err == nil && (got.Kind != tc.kind || got.First.EntryID != tc.binding || got.First.TargetID != tc.gotTarget) {
				t.Fatalf("unexpected operation %+v", got)
			}
		})
	}
}

func TestParseNormalOperationDuplicateAndSecondCost(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.Jobs = corebattle.BattleJobs{JobID: "job-17", SP: 40, OldJobID: "job-33", OldSP: 30}
	a.ItemDefinitionIDs, a.Extended = []string{"item-249"}, corebattle.ExtendedAdditional
	cmd := RunCommand{ActorID: "a", Input: "＠マジックバリア　＠フバーハ>Bob"}
	got, err := r.ParseNormalOperation(cmd, ctx)
	if err != nil || got.First.EntryID != "job-33-entry-0" || got.First.Cost != 12 || !got.First.Whitelisted ||
		got.Second == nil || got.Second.EntryID != "job-33-entry-2" || got.Second.Cost != 7 || got.Second.TargetID != "b" || !got.Second.Whitelisted {
		t.Fatalf("got %+v, second=%+v, err=%v", got, got.Second, err)
	}
	cmd.Action, cmd.Input = "job-17-entry-2", ""
	if _, err := r.ParseNormalOperation(cmd, ctx); !errors.Is(err, ErrRunUnavailable) {
		t.Fatalf("overwritten older entry still selectable: %v", err)
	}
	cmd.Action, cmd.Input = "", "＠マジックバリア ＠フバーハ>stale"
	if _, err := r.ParseNormalOperation(cmd, ctx); !errors.Is(err, ErrRunConflict) {
		t.Fatalf("stale second target not rejected: %v", err)
	}
	a.Extended = ""
	if got, err := r.ParseNormalOperation(cmd, ctx); err != nil || got.Second != nil {
		t.Fatalf("second parsed without additional submission: %+v %v", got, err)
	}
	a.Extended = corebattle.ExtendedAdditional
	cmd.Input = "＠マジックバリア　＠フバーハ>Bob"
	secondDef := ctx.Definitions.Invocations.Entries["job-33-entry-2"]
	secondDef.Validate = func(*corebattle.LifetimeState, *corebattle.CallbackContext) error {
		return errors.New("entry state validator should not run for second parse")
	}
	ctx.Definitions.Invocations.Entries["job-33-entry-2"] = secondDef
	if got, err := r.ParseNormalOperation(cmd, ctx); err != nil || got.Second == nil || got.Second.EntryID != "job-33-entry-2" {
		t.Fatalf("second parse ran state validator: %+v %v", got, err)
	}
	secondDef.Children = []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "unregistered-child"}}
	ctx.Definitions.Invocations.Entries["job-33-entry-2"] = secondDef
	if _, err := r.ParseNormalOperation(cmd, ctx); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatalf("unregistered reachable second descendant not rejected: %v", err)
	}
}

func TestParseNormalOperationCustomAndUnsupportedAdmissionArePure(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.CMP = 20
	a.Custom = &corebattle.CustomLoadout{Name: "Echo", Cost: 20, Gems: [3]string{"test-gem", "test-gem"}}
	validations := 0
	ctx.Definitions.Invocations.Entries["test-gem"] = corebattle.CallbackDefinition{
		Validate: func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
			validations++
			if !f.Custom || f.Arguments[0] != "a" {
				t.Fatalf("expected custom self fallback: %+v", f)
			}
			return nil
		},
		Execute: func(*corebattle.LifetimeState, *corebattle.CallbackContext, random.Generator) error {
			t.Fatal("discovery or parse executed a callback")
			return nil
		},
	}
	cmd := RunCommand{ActorID: "a", Action: "Echo"}
	op, err := r.ParseNormalOperation(cmd, ctx)
	if err != nil || !op.First.Custom || op.First.EntryID != "custom" || op.First.TargetID != "" || validations == 0 {
		t.Fatalf("custom not bound: %+v %v, %d checks", op, err, validations)
	}
	a.Custom.Gems[2] = "unsupported"
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	choices, err := r.DiscoverNormalOperations(cmd, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "custom" && c.Name == "Echo" && c.Gated }) {
		t.Fatalf("missing unsupported choice: %+v %v", choices, err)
	}
	if _, err = r.ParseNormalOperation(cmd, ctx); err == nil {
		t.Fatal("missing descendant admitted")
	}
	after, err := json.Marshal(r)
	if err != nil || string(before) != string(after) {
		t.Fatal("admission changed run", err)
	}
	a.Custom.Name = "こうげき"
	choices, err = r.DiscoverNormalOperations(cmd, ctx)
	if err != nil || slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "custom" }) {
		t.Fatalf("colliding custom was listed: %+v %v", choices, err)
	}
}

func TestDiscoverFallenAndSpectatorOperations(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.HP, a.Jobs.JobID, a.Jobs.SP, a.ItemDefinitionIDs = 0, "job-66", 150, []string{"item-158"}
	r.actor("b").RosterIdentity = "calling-66"
	cmd := RunCommand{ActorID: "a", Action: "job-66-revive", TargetID: "c"}
	got, err := r.ParseNormalOperation(cmd, ctx)
	if err != nil || !got.First.FallenSelf || got.First.TargetID != cmd.TargetID || got.First.Validate == nil || got.First.Execute == nil {
		t.Fatalf("fallen revival: %+v %v", got, err)
	}
	r.actor("b").HP = 0
	if got, err := r.ParseNormalOperation(cmd, ctx); err != nil || !got.First.FallenSelf {
		t.Fatalf("fallen designated helper means the listed callback has no effect: %+v %v", got, err)
	}
	r.actor("b").RosterIdentity = ""
	if got, err := r.ParseNormalOperation(cmd, ctx); err != nil || !got.First.FallenSelf {
		t.Fatalf("missing designated helper means the listed callback has no effect: %+v %v", got, err)
	}
	for _, tc := range []struct {
		name   string
		change func(*NormalRun, *corebattle.LifetimeActor)
	}{
		{"missing amulet", func(_ *NormalRun, a *corebattle.LifetimeActor) { a.ItemDefinitionIDs = nil }},
		{"insufficient sp", func(_ *NormalRun, a *corebattle.LifetimeActor) { a.Jobs.SP = 149 }},
		{"cursed", func(_ *NormalRun, a *corebattle.LifetimeActor) { a.Abilities = []string{"cursed"} }},
		{"banned", func(r *NormalRun, _ *corebattle.LifetimeActor) {
			r.Battle.SkillBans = map[string]int{"job-66-revive": 1}
		}},
		{"no living enemy", func(r *NormalRun, _ *corebattle.LifetimeActor) { r.actor("c").HP = 0 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ctx := operationFixture()
			a := r.actor("a")
			a.HP, a.Jobs.JobID, a.Jobs.SP, a.ItemDefinitionIDs = 0, "job-66", 150, []string{"item-158"}
			tc.change(r, a)
			if _, err := r.ParseNormalOperation(cmd, ctx); !errors.Is(err, ErrRunUnavailable) {
				t.Fatalf("expected ErrRunUnavailable, got %v", err)
			}
		})
	}
	cmd.Action = "attack"
	if _, err := r.ParseNormalOperation(cmd, ctx); !errors.Is(err, ErrUnconsciousCharacter) {
		t.Fatalf("fallen actor allowed ordinary attack: %v", err)
	}
	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "viewer"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	var actions []string
	for _, c := range choices {
		actions = append(actions, c.Action)
	}
	if !slices.Equal(actions, []string{"whisper", "exit", "screenshot"}) {
		t.Fatalf("unexpected spectator actions %v", actions)
	}
}

func TestDiscoverOrdinaryTreasureProjectsUnopenedBoxes(t *testing.T) {
	r, ctx := operationFixture()
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Boxes = []TreasureBox{{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"}}
	ctx.Requester = &corecharacter.Character{Level: 1, Experience: 10}
	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil ||
		!slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "attack" && !c.Gated }) ||
		!slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "chest" && c.Gated }) ||
		slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "stock-level" }) {
		t.Fatalf("treasure discovery with unopened box: %+v %v", choices, err)
	}
	// When the actor has attack > 0 without a defeat port, admission against the projected box still gates attack.
	r.actor("a").Attack = 10
	choices, err = r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "attack" && c.Gated }) {
		t.Fatalf("unrepresented attack against projected box not gated: %+v %v", choices, err)
	}
	// Targeting the unopened box un-gates chest discovery.
	choices, err = r.DiscoverNormalOperations(RunCommand{ActorID: "a", TargetID: "box-1"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "chest" && !c.Gated }) {
		t.Fatalf("targeted chest discovery gated: %+v %v", choices, err)
	}
	// Once opened (or defeated), combat choices disappear and stock-level returns.
	r.Boxes[0].OpenedBy = "a"
	choices, err = r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil ||
		slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "attack" }) ||
		!slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "chest" && c.Gated }) ||
		!slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "stock-level" }) {
		t.Fatalf("post-open treasure discovery: %+v %v", choices, err)
	}
}

func TestParseNormalOperationTreasureChestTargets(t *testing.T) {
	r, ctx := operationFixture()
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Boxes = []TreasureBox{
		{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"},
		{ID: "box-2", Name: "青い宝箱B", OpenedBy: "b", ItemID: "item-002"},
		{ID: "box-3", Name: "黒い宝箱C", Defeated: true, ItemID: "item-003"},
	}
	for _, tc := range []struct {
		name    string
		cmd     RunCommand
		wantID  string
		wantErr error
	}{
		{"typed unopened ID", RunCommand{ActorID: "a", Action: "chest", TargetID: "box-1"}, "box-1", nil},
		{"legacy unopened name", RunCommand{ActorID: "a", Input: "＠しらべる>普通の宝箱A"}, "box-1", nil},
		{"legacy unopened @name", RunCommand{ActorID: "a", Input: "＠しらべる>@普通の宝箱A"}, "box-1", nil},
		{"typed empty target", RunCommand{ActorID: "a", Action: "chest", TargetID: ""}, "", ErrNoUnopenedTreasure},
		{"legacy empty target", RunCommand{ActorID: "a", Input: "＠しらべる"}, "", ErrNoUnopenedTreasure},
		{"typed foreign target", RunCommand{ActorID: "a", Action: "chest", TargetID: "foreign"}, "", ErrNoUnopenedTreasure},
		{"legacy foreign target", RunCommand{ActorID: "a", Input: "＠しらべる>foreign"}, "", ErrNoUnopenedTreasure},
		{"typed opened target", RunCommand{ActorID: "a", Action: "chest", TargetID: "box-2"}, "", ErrNoUnopenedTreasure},
		{"legacy opened target", RunCommand{ActorID: "a", Input: "＠しらべる>青い宝箱B"}, "", ErrNoUnopenedTreasure},
		{"typed defeated target", RunCommand{ActorID: "a", Action: "chest", TargetID: "box-3"}, "", ErrNoUnopenedTreasure},
		{"legacy defeated target", RunCommand{ActorID: "a", Input: "＠しらべる>黒い宝箱C"}, "", ErrNoUnopenedTreasure},
		{"typed display name rejected", RunCommand{ActorID: "a", Action: "chest", TargetID: "普通の宝箱A"}, "", ErrNoUnopenedTreasure},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ParseNormalOperation(tc.cmd, ctx)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err=%v want %v (op=%+v)", err, tc.wantErr, got)
			}
			if err == nil && (got.Kind != RunOperationRoom || got.First.Action != "chest" || got.First.TargetID != tc.wantID || !slices.Equal(got.First.Arguments, []string{tc.wantID})) {
				t.Fatalf("unexpected parsed chest op: %+v", got)
			}
		})
	}
}

func TestNormalOperationTreasureCallbacksAndValuationSearch(t *testing.T) {
	r, ctx := operationFixture()
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Boxes = []TreasureBox{
		{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"},
		{ID: "box-2", Name: "青い宝箱B", OpenedBy: "b", ItemID: "item-002"},
	}
	a := r.actor("a")
	a.Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 200, OldJobID: "job-81", OldSP: 30}
	a.Gear = &corebattle.BattleGearStats{}
	a.HeldToolID = new("item-151")
	stages, err := InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	r.Stage, err = stages.FindByID("stage-00")
	if err != nil {
		t.Fatal(err)
	}
	tools := &corebattle.OrdinaryToolContext{DamageContext: *ctx.Callback.Damage}
	level := 10
	s := &NormalRunService{cfg: NormalRunConfig{Stages: stages, Monsters: monsters}}
	jobEnc, _, err := s.PrepareJobEncounters(context.Background(), r, "a", &corebattle.EquipmentResetContext{TargetContext: corebattle.TargetContext{ActorID: "a"}, RequesterLevel: &level})
	if err != nil {
		t.Fatal(err)
	}
	ctx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{
		BaseGuard:       &corebattle.BaseGuardContext{DamageContext: *ctx.Callback.Damage},
		BasicMagic:      &corebattle.BasicMagicContext{DamageContext: *ctx.Callback.Damage},
		ProtectiveMagic: &corebattle.ProtectiveMagicContext{DamageContext: *ctx.Callback.Damage},
		Transformations: &corebattle.TransformationContext{},
		Revival:         &corebattle.RevivalContext{},
		Tools:           tools,
		JobEncounters:   jobEnc,
	}, 2)
	ctx.Definitions.RegisterOrdinaryTools(tools)

	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"job-78-entry-1", "job-81-entry-1", "job-78-entry-6", "tool"} {
		if !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == action && !c.Gated }) {
			t.Fatalf("expected ungated %s in treasure choices: %+v", action, choices)
		}
	}

	for _, tc := range []struct {
		name, action, target, input, wantEntry, wantTarget string
		wantErr                                            error
	}{
		{"valuation by box ID", "job-78-entry-1", "box-1", "", "job-78-entry-1", "box-1", nil},
		{"valuation by legacy box name", "", "", "＠トレハンのかん>普通の宝箱A", "job-78-entry-1", "box-1", nil},
		{"reward search by item ID", "job-78-entry-1", "item-001", "", "job-78-entry-1", "item-001", nil},
		{"reward search by legacy item name", "", "", "＠トレハンのかん>薬草", "job-78-entry-1", "item-001", nil},
		{"valuation rejects opened box", "job-78-entry-1", "box-2", "", "", "", ErrRunConflict},
		{"valuation rejects unknown ID", "job-78-entry-1", "foreign", "", "", "", ErrRunConflict},
		{"content inspection by legacy box name", "", "", "＠とうぞくのはな>普通の宝箱A", "job-81-entry-1", "box-1", nil},
		{"tool151 inspection", "tool", "", "", "tool-151", "", nil},
		{"job78.6 by legacy box name", "", "", "＠トレハンのいじ>普通の宝箱A", "job-78-entry-6", "box-1", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := r.ParseNormalOperation(RunCommand{ActorID: "a", Action: tc.action, TargetID: tc.target, Input: tc.input}, ctx)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("got err=%v want %v", err, tc.wantErr)
			}
			if err == nil && (got.First.EntryID != tc.wantEntry || got.First.TargetID != tc.wantTarget) {
				t.Fatalf("got %+v want entry=%s target=%s", got.First, tc.wantEntry, tc.wantTarget)
			}
		})
	}
}

func TestNormalStockLevelBindsTypedAndLegacySelection(t *testing.T) {
	r, ctx := operationFixture()
	r.actor("c").HP = 0
	ctx.Requester = &corecharacter.Character{Level: 1, Experience: 10}
	for _, cmd := range []RunCommand{
		{ActorID: "a", Action: stockLevelAction},
		{ActorID: "a", Input: "＠すとっく"},
	} {
		got, err := r.ParseNormalOperation(cmd, ctx)
		if err != nil || got.Kind != RunOperationBattle || got.First.EntryID != stockLevelAction || got.First.Cost != 0 || got.Second != nil {
			t.Fatalf("stock selection: %+v, %v", got, err)
		}
	}
	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == stockLevelAction && !c.Gated }) {
		t.Fatalf("stock discovery: %+v, %v", choices, err)
	}
}

func TestNormalOperationSourceKeysCoverRegisteredTablePositions(t *testing.T) {
	_, ctx := operationFixture()
	for job, table := range ctx.Definitions.Invocations.JobTables {
		for _, variant := range table.Variants {
			for _, entry := range variant {
				if entry.ID != "job-82-entry-0" && skill.EntrySourceKey(entry.ID, false) == "" {
					t.Errorf("job %d entry %s missing its source name key", job, entry.ID)
				}
			}
		}
	}
}

func TestNormalOperationsEXTreasureFloors(t *testing.T) {
	r, ctx := operationFixture()
	r.Stage, r.EXUnlocked, r.Enemies = Stage{ID: "stage-24", StageTraits: StageTraits{EX: &EXStage{Marker: markerBossEncounter, TreasureItems: []string{"item-104"}}}}, true, nil
	r.Floor = EXBossFloor
	if got, err := r.ParseNormalOperation(RunCommand{ActorID: "a", Action: "advance"}, ctx); err != nil || got.First.Action != "advance" {
		t.Fatalf("12→13 advance: %+v %v", got, err)
	}
	r.Floor, r.Boxes = EXTreasureFloor, []TreasureBox{{ID: "box-1", Fixed: true}}
	if got, err := r.ParseNormalOperation(RunCommand{ActorID: "a", Action: "chest", TargetID: "box-1"}, ctx); err != nil || got.First.TargetID != "box-1" {
		t.Fatalf("floor-13 chest: %+v %v", got, err)
	}
	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "chest" }) || slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "advance" }) {
		t.Fatalf("floor-13 discovery: %+v %v", choices, err)
	}
}
