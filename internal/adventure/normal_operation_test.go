package adventure

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

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
	tools := &corebattle.OrdinaryToolContext{DamageContext: damage}
	ctx := NormalOperationContext{Callback: corebattle.CallbackContext{Damage: &damage}}
	ctx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{
		BaseGuard:       &corebattle.BaseGuardContext{DamageContext: damage},
		BasicMagic:      &corebattle.BasicMagicContext{DamageContext: damage},
		ProtectiveMagic: &corebattle.ProtectiveMagicContext{DamageContext: damage},
		Transformations: &corebattle.TransformationContext{},
		Revival:         &corebattle.RevivalContext{},
		Tools:           tools,
	}, 2)
	ctx.Definitions.RegisterOrdinaryTools(tools)
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

func TestOrdinaryTreasureDiscoveryAndTargetBinding(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.Attack = 100
	a.Gear = &corebattle.BattleGearStats{}
	a.Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 20, OldJobID: "job-16", OldSP: 10}
	a.HeldToolID = new("item-151")
	r.actor("b").Gear = &corebattle.BattleGearStats{}
	r.Admission.Speed = 3
	r.Stage = Stage{ID: "stage-00", TreasureItems: []string{"item-001", "item-005"}}
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Battle.Actors = r.Battle.Actors[:2]
	svc := &NormalRunService{cfg: NormalRunConfig{Clock: &testClock{now: time.Date(2026, 2, 2, 0, 0, 0, 0, time.UTC)}, RNG: &ordinaryRNG{}}}
	if err := svc.createTreasureEncounter(r, "a", ""); err != nil {
		t.Fatal(err)
	}
	if len(r.Boxes) != 3 {
		t.Fatalf("expected 3 generated boxes with job-78, got %+v", r.Boxes)
	}
	r.Boxes[1].OpenedBy = "b"
	r.Boxes[2].Defeated = true
	unopened, opened, defeated := r.Boxes[0], r.Boxes[1], r.Boxes[2]
	ctx.Requester = &corecharacter.Character{Level: 1, Experience: 10}
	defeatCtx := &corebattle.DefeatContext{}
	ctx.Callback.Damage.Defeat = func(string) error { return nil }
	ctx.Callback.Defeat = defeatCtx

	if !r.livingEnemies() {
		t.Fatal("expected livingEnemies true with unopened box")
	}
	if npc := r.NextNPC(svc.cfg.Clock.Now().Add(time.Minute)); npc != "" {
		t.Fatalf("expected no NPC response on treasure floor, got %q", npc)
	}
	state, frame, inspection, err := r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(frame.EnemyIDs, []string{unopened.ID}) || len(frame.MemberOnlyIDs) != 0 {
		t.Fatalf("unexpected projected enemies: %+v", frame)
	}
	if len(r.Battle.Actors) != 2 {
		t.Fatalf("ProjectCombat leaked boxes into durable Battle.Actors: %+v", r.Battle.Actors)
	}
	var boxActor *corebattle.LifetimeActor
	for i := range state.Actors {
		if state.Actors[i].ID == unopened.ID {
			boxActor = &state.Actors[i]
		}
	}
	if boxActor == nil || boxActor.HP != 1000 || boxActor.MaxHP != 1000 || boxActor.Defense != 1000 || boxActor.BaseStats.Defense != 1000 ||
		boxActor.Tension != 1 || boxActor.Temporary != corebattle.TemporaryNullMagic || !boxActor.NPC {
		t.Fatalf("unexpected projected box actor: %+v", boxActor)
	}
	if content := inspection[unopened.ID]; !content.Treasure || content.DefinitionID != "item-001" {
		t.Fatalf("unexpected box inspection content: %+v", content)
	}

	choices, err := r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, action := range []string{"attack", "defend", "tension", "job-78-entry-1", "job-16-entry-1", "tool", "chest"} {
		if !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == action && !c.Gated }) {
			t.Fatalf("expected ungated choice %s, got %+v", action, choices)
		}
	}
	for _, hidden := range []string{"stock-level", "between-encounter-survival", "advance"} {
		if slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == hidden }) {
			t.Fatalf("unexpected between-encounter/advance choice %s while box unopened: %+v", hidden, choices)
		}
	}

	for _, cmd := range []RunCommand{
		{ActorID: "a", Action: "attack", TargetID: unopened.ID},
		{ActorID: "a", Action: "job-16-entry-1", TargetID: unopened.ID},
		{ActorID: "a", Action: "job-78-entry-1", TargetID: unopened.ID},
		{ActorID: "a", Action: "job-78-entry-1", Input: unopened.ItemID},
		{ActorID: "a", Input: "＠こうげき>" + unopened.Name},
		{ActorID: "a", Input: "＠トレハンのかん>@" + unopened.Name},
		{ActorID: "a", Input: "＠トレハンのかん>薬草"},
		{ActorID: "a", Action: "chest", TargetID: unopened.ID},
		{ActorID: "a", Input: "＠しらべる>" + unopened.Name},
	} {
		parsed, err := r.ParseNormalOperation(cmd, ctx)
		if err != nil {
			t.Fatalf("ParseNormalOperation(%+v) failed: %v", cmd, err)
		}
		if cmd.Action == "job-78-entry-1" && cmd.Input != "" && !slices.Equal(parsed.First.Arguments, []string{unopened.ItemID}) {
			t.Fatalf("expected reward-search argument %s, got %+v", unopened.ItemID, parsed.First)
		}
	}

	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	for _, rejected := range []struct {
		cmd     RunCommand
		wantErr error
	}{
		{RunCommand{ActorID: "a", Action: "attack", TargetID: opened.ID}, ErrRunConflict},
		{RunCommand{ActorID: "a", Action: "attack", TargetID: defeated.ID}, ErrRunConflict},
		{RunCommand{ActorID: "a", Action: "job-16-entry-1", TargetID: opened.ID}, ErrRunConflict},
		{RunCommand{ActorID: "a", Action: "job-78-entry-1", TargetID: opened.ID}, ErrRunConflict},
		{RunCommand{ActorID: "a", Action: "job-78-entry-1", TargetID: defeated.ID}, ErrRunConflict},
		{RunCommand{ActorID: "a", Input: "＠こうげき>" + opened.Name}, ErrRunConflict},
		{RunCommand{ActorID: "a", Input: "＠トレハンのかん>" + opened.Name}, ErrRunConflict},
		{RunCommand{ActorID: "a", Action: "chest", TargetID: opened.ID}, ErrNoUnopenedTreasure},
		{RunCommand{ActorID: "a", Action: "chest", TargetID: defeated.ID}, ErrNoUnopenedTreasure},
		{RunCommand{ActorID: "a", Input: "＠しらべる>" + opened.Name}, ErrNoUnopenedTreasure},
	} {
		if _, err := r.ParseNormalOperation(rejected.cmd, ctx); !errors.Is(err, rejected.wantErr) {
			t.Fatalf("ParseNormalOperation(%+v) = %v, want %v", rejected.cmd, err, rejected.wantErr)
		}
	}
	after, err := json.Marshal(r)
	if err != nil || string(before) != string(after) {
		t.Fatalf("rejected parse mutated run: err=%v", err)
	}

	r.Boxes[0].Defeated = true
	if r.livingEnemies() {
		t.Fatal("expected livingEnemies false after all boxes opened or defeated")
	}
	state, frame, _, err = r.ProjectCombat("a")
	if err != nil || len(frame.EnemyIDs) != 0 || len(state.Actors) != 2 {
		t.Fatalf("expected empty enemy projection after all boxes cleared: frame=%+v actors=%+v err=%v", frame, state.Actors, err)
	}
	choices, err = r.DiscoverNormalOperations(RunCommand{ActorID: "a"}, ctx)
	if err != nil || !slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == stockLevelAction && !c.Gated }) {
		t.Fatalf("expected stock-level after boxes cleared, got %+v %v", choices, err)
	}
}

func TestOrdinaryTreasureInspectionAndHostileCombatLifecycle(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.Attack = 200
	a.Gear = &corebattle.BattleGearStats{}
	a.Jobs = corebattle.BattleJobs{JobID: "job-78", SP: 20, OldJobID: "job-16", OldSP: 10}
	r.actor("b").Gear = &corebattle.BattleGearStats{}
	r.Floor, r.Enemies = TreasureRoomFloor, nil
	r.Battle.Actors = r.Battle.Actors[:2]
	r.Boxes = []TreasureBox{
		{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-005"},
		{ID: "box-2", Name: "青い宝箱B", ItemID: "item-001", OpenedBy: "b"},
	}

	// Register a minimal defeat/rewards port alongside tools and magic.
	level := 10
	tools := &corebattle.OrdinaryToolContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{RequesterLevel: &level}}}
	magic := &corebattle.BasicMagicContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{RequesterLevel: &level}}}
	ctx.Definitions.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{
		BasicMagic: magic,
		Tools:      tools,
	}, 2)
	ctx.Definitions.RegisterOrdinaryTools(tools)
	ctx.Callback.Damage = &magic.DamageContext

	runInspection := func(entryID string, args ...string) corebattle.OrdinaryToolResult {
		t.Helper()
		state, frame, inspection, err := r.ProjectCombat("a")
		if err != nil {
			t.Fatal(err)
		}
		f := corebattle.CallbackContext{TargetContext: frame, Arguments: args, Inspection: inspection}
		ops, err := ctx.Definitions.InvocationAction(&state, &f, []string{entryID}, random.NewDeterministic(1))
		if err != nil {
			t.Fatal(entryID, err)
		}
		if err := state.ApplyInvocations(&f, ctx.Definitions.Invocations, ops, random.NewDeterministic(1)); err != nil {
			t.Fatal(entryID, err)
		}
		r.ReconcileCombat(state, &f.TargetContext)
		return tools.Results[len(tools.Results)-1]
	}

	statFact := runInspection("job-16-entry-1", "box-1").Facts[0]
	if statFact.Kind != "stats" || statFact.TargetID != "box-1" || statFact.HP != 1000 || statFact.MaxHP != 1000 || statFact.BaseStats.Defense != 1000 {
		t.Fatalf("unexpected stat inspection fact: %+v", statFact)
	}
	valFact := runInspection("job-78-entry-1", "box-1").Facts[0]
	if valFact.Kind != "valuation" || valFact.TargetID != "box-1" || valFact.Band != 2 {
		t.Fatalf("unexpected valuation fact: %+v", valFact)
	}
	foundFact := runInspection("job-78-entry-1", "item-005").Facts[0]
	if foundFact.Kind != "reward-search" || !foundFact.Found || foundFact.TargetID != "box-1" {
		t.Fatalf("unexpected reward-search hit fact: %+v", foundFact)
	}
	missFact := runInspection("job-78-entry-1", "item-001").Facts[0]
	if missFact.Kind != "reward-search" || missFact.Found {
		t.Fatalf("opened box item leaked into reward search: %+v", missFact)
	}
	toolFact := runInspection("tool-151").Facts
	if len(toolFact) != 1 || toolFact[0].Kind != "content" || toolFact[0].DefinitionID != "item-005" || toolFact[0].RewardKind != 3 {
		t.Fatalf("unexpected tool-151 facts: %+v", toolFact)
	}
	if r.Boxes[0].Combat != nil || r.Boxes[0].OpenedBy != "" || r.Boxes[0].Defeated {
		t.Fatalf("inspection mutated box state: %+v", r.Boxes[0])
	}

	// Hostile magic is blocked by initial TemporaryNullMagic.
	state, frame, inspection, err := r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	rewardApplies := 0
	defeatCtx := corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: &level},
		Rewards: &corebattle.DefeatRewards{
			Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
			Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { rewardApplies++; return nil },
		},
	}
	magic.TargetContext = frame
	magic.Defeat = func(id string) error {
		_, err := state.ApplyDefeat(&defeatCtx, id, random.NewDeterministic(1))
		return err
	}
	f := corebattle.CallbackContext{TargetContext: frame, Arguments: []string{"box-1"}, Inspection: inspection, Damage: &magic.DamageContext, Defeat: &defeatCtx}
	ops, err := ctx.Definitions.InvocationAction(&state, &f, []string{"job-6-entry-1"}, random.NewDeterministic(1))
	if err != nil {
		t.Fatal(err)
	}
	if err := state.ApplyInvocations(&f, ctx.Definitions.Invocations, ops, random.NewDeterministic(1)); err != nil {
		t.Fatal(err)
	}
	r.ReconcileCombat(state, &f.TargetContext)
	if r.Boxes[0].Combat != nil || r.Boxes[0].Defeated {
		t.Fatalf("magic should be nullified by TemporaryNullMagic, got %+v", r.Boxes[0])
	}

	// Non-lethal physical damage persists across serialize/reload in Boxes[0].Combat.
	state, frame, _, err = r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	defeatCtx.TargetContext = frame
	dmgCtx := corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level},
		GuaranteedHit: true,
		Defeat: func(id string) error {
			_, err := state.ApplyDefeat(&defeatCtx, id, random.NewDeterministic(1))
			return err
		},
	}
	if _, err := state.ApplyDamage(&dmgCtx, corebattle.TargetEffect{TargetID: "box-1", Payload: corebattle.CallbackPayload{Number: 1200}, Category: corebattle.CategoryPhysical}, &ordinaryRNG{}); err != nil {
		t.Fatal(err)
	}
	r.ReconcileCombat(state, &dmgCtx.TargetContext)
	if len(r.Battle.Actors) != 2 || r.Boxes[0].Combat == nil || r.Boxes[0].Combat.HP >= 1000 || r.Boxes[0].Combat.HP <= 0 || r.Boxes[0].Defeated {
		t.Fatalf("expected persisted partial box damage without leaking Battle.Actors: box=%+v actors=%+v", r.Boxes[0], r.Battle.Actors)
	}
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded NormalRun
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	*r = reloaded
	state, frame, _, err = r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	if idx := slices.IndexFunc(state.Actors, func(x corebattle.LifetimeActor) bool { return x.ID == "box-1" }); idx < 0 || state.Actors[idx].HP != r.Boxes[0].Combat.HP {
		t.Fatalf("reloaded projection lost damaged box HP: %+v", state.Actors)
	}

	// Lethal damage marks the box defeated without prize delivery and excludes it from later frames/claims.
	defeatCtx.TargetContext = frame
	dmgCtx = corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level},
		GuaranteedHit: true,
		Defeat: func(id string) error {
			_, err := state.ApplyDefeat(&defeatCtx, id, random.NewDeterministic(1))
			return err
		},
	}
	if _, err := state.ApplyDamage(&dmgCtx, corebattle.TargetEffect{TargetID: "box-1", Payload: corebattle.CallbackPayload{Number: 3000}, Category: corebattle.CategoryBreath}, &ordinaryRNG{}); err != nil {
		t.Fatal(err)
	}
	r.ReconcileCombat(state, &dmgCtx.TargetContext)
	if !r.Boxes[0].Defeated || r.Boxes[0].Combat != nil || r.Boxes[0].OpenedBy != "" || r.Boxes[0].DeliveredTo != "" || r.livingEnemies() || len(r.Battle.Actors) != 2 {
		t.Fatalf("expected defeated box with no delivery: box=%+v actors=%+v", r.Boxes[0], r.Battle.Actors)
	}
	svc := &NormalRunService{}
	if _, _, err := svc.claimChest(t.Context(), r, "a", "box-1"); !errors.Is(err, ErrNoUnopenedTreasure) {
		t.Fatalf("expected claim on defeated box to fail with ErrNoUnopenedTreasure, got %v", err)
	}
	if rewardApplies != 1 {
		t.Fatalf("expected defeat reward hook reached once on defeat, got %d", rewardApplies)
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
