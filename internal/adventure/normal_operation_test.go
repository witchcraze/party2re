package adventure

import (
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
	if err != nil || !got.First.FallenSelf || got.First.TargetID != cmd.TargetID {
		t.Fatalf("fallen revival: %+v %v", got, err)
	}
	r.actor("b").HP = 0
	if got, err := r.ParseNormalOperation(cmd, ctx); err != nil || !got.First.FallenSelf {
		t.Fatalf("fallen designated helper means the listed callback has no effect: %+v %v", got, err)
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

func TestDiscoverOrdinaryTreasureKeepsUnrepresentedCombatGated(t *testing.T) {
	r, ctx := operationFixture()
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Boxes = []TreasureBox{{ID: "first-box"}}
	ctx.Requester = &corecharacter.Character{Level: 1, Experience: 10}
	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "attack" && c.Gated }) ||
		slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "stock-level" }) {
		t.Fatalf("treasure discovery: %+v %v", choices, err)
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
	r.Stage, r.EXUnlocked, r.Enemies = Stage{ID: "stage-24", EXTreasureItems: []string{"item-104"}}, true, nil
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
