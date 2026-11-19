package adventure

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func ordinaryTestOperation(t *testing.T, r *NormalRun, ctx NormalOperationContext, cmd RunCommand) (*corebattle.PlayerOperation, error) {
	t.Helper()
	ops, err := r.normalOperations(cmd.ActorID, ctx)
	if err != nil {
		return nil, err
	}
	parsed, err := ops.parse(cmd)
	if err != nil {
		return nil, err
	}
	if parsed.Kind != RunOperationBattle {
		t.Fatalf("expected battle action, got %+v", parsed)
	}
	targets := ops.ctx.Callback.TargetContext
	damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: targets, RequesterLevel: ctx.Callback.Damage.RequesterLevel}}
	defeat := &corebattle.DefeatContext{EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: targets}}
	return &corebattle.PlayerOperation{First: parsed.First.PlayerAction, Tail: corebattle.PlayerTailContext{DamageContext: damage, DefeatContext: defeat}, ResponseEnabled: true}, nil
}

func applyOrdinaryTestAction(t *testing.T, r *NormalRun, ctx NormalOperationContext, cmd RunCommand, rng random.Generator) (corebattle.PlayerOperationResult, error) {
	t.Helper()
	op, err := ordinaryTestOperation(t, r, ctx, cmd)
	if err != nil {
		return corebattle.PlayerOperationResult{}, err
	}
	return r.applyPlayerOperation(op, rng)
}

func TestOrdinaryFormationUsesMemberScanAndSurvivesReload(t *testing.T) {
	r, ctx := operationFixture()
	r.Roster = []string{"a", "d", "b"}
	r.Enemies = []string{"c", "e"}
	d := r.actor("b").Clone()
	d.ID = "d"
	e, fallen := r.actor("c").Clone(), r.actor("c").Clone()
	e.ID, fallen.ID, fallen.HP, fallen.NPC = "e", "fallen", 0, false
	r.Battle.Actors = slices.Insert(r.Battle.Actors, 1, d, fallen, e)
	r.Battle.SkillBans = map[string]int{"unrelated": 2}
	r.actor("d").CMPNeedCharge = 3
	r.Admission.Speed = 10
	now := time.Unix(100, 0)
	r.LastActions = map[string]time.Time{"e": now}
	rng := &ordinaryRNG{value: .5}

	result, err := applyOrdinaryTestAction(t, r, ctx, RunCommand{ActorID: "d", Input: "＠まえ"}, rng)
	if err != nil || !result.Accepted || !result.ResponseReached || r.Battle.SkillBans["unrelated"] != 1 {
		t.Fatalf("formation did not reach the ordinary tail: %+v %v", result, err)
	}
	if !slices.Equal(r.Roster, []string{"d", "a", "b"}) || !slices.Equal(r.Enemies, []string{"fallen", "e", "c"}) || r.NextNPC(now) != "c" {
		t.Fatalf("first formation: party %v enemies %v next %s", r.Roster, r.Enemies, r.NextNPC(now))
	}
	if err := r.RecordNPCResponse("c", now); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var loaded NormalRun
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	result, err = applyOrdinaryTestAction(t, &loaded, ctx, RunCommand{ActorID: "a", Action: "back"}, rng)
	var members []string
	for _, a := range loaded.Battle.Actors {
		members = append(members, a.ID)
	}
	if err != nil || !result.Accepted || !slices.Equal(loaded.Roster, []string{"d", "b", "a"}) ||
		!slices.Equal(members, []string{"d", "b", "a", "e", "c", "fallen"}) || loaded.LeaderID != "a" {
		t.Fatalf("second formation: party %v members %v result %+v error %v", loaded.Roster, members, result, err)
	}
	if !slices.Equal(loaded.Enemies, []string{"e", "c", "fallen"}) || loaded.NextNPC(now.Add(10*time.Second)) != "e" {
		t.Fatalf("reloaded NPC order: %v", loaded.Enemies)
	}
}

func TestOrdinarySurvivalUsesSharedHealingAndLateCost(t *testing.T) {
	for _, tc := range []struct {
		name            string
		jobs            corebattle.BattleJobs
		encounter       bool
		mp, hp, df, pay int
	}{
		{"current", corebattle.BattleJobs{JobID: "job-78", SP: 55}, false, 10, 181, 100, 10},
		{"previous", corebattle.BattleJobs{OldJobID: "job-78", OldSP: 55}, false, 10, 181, 100, 10},
		{"combat", corebattle.BattleJobs{JobID: "job-78", SP: 55}, true, 14, 408, 80, 14},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, ctx := operationFixture()
			ctx.Definitions.RegisterTreatmentMischief(&corebattle.TreatmentMischiefContext{})
			a := r.actor("a")
			a.Jobs, a.MP, a.HP, a.MaxHP, a.Defense, a.Tension = tc.jobs, tc.mp, 30, 500, 100, 1
			a.Status, a.ItemDefinitionIDs = corebattle.StatusMagicSeal, []string{"item-146"}
			a.CMPNeedCharge = 3
			if !tc.encounter {
				r.actor("c").HP = 0
			}
			cmd := RunCommand{ActorID: "a", Input: "＠サバイバル>Bob"}
			parsed, err := r.ParseNormalOperation(cmd, ctx)
			if err != nil || parsed.First.Cost != tc.pay || (parsed.First.EntryID == "job-78-entry-3") != tc.encounter {
				t.Fatalf("survival binding: %+v %v", parsed, err)
			}
			if tc.encounter {
				// The existing job callback has its own port and identity.
				frame := corebattle.CallbackContext{TargetContext: corebattle.TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}, EnemyIDs: []string{"c"}}}
				err = r.Battle.ApplyInvocations(&frame, ctx.Definitions.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, &ordinaryRNG{value: .5})
			} else {
				op, err := ordinaryTestOperation(t, r, ctx, cmd)
				if err != nil {
					t.Fatal(err)
				}
				execute := op.First.Execute
				op.First.Execute = func(state *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
					facts, err := execute(state, rng)
					if r.actor("a").MP != tc.pay || facts.HealCalls != 1 {
						t.Errorf("callback must return before the late charge, MP %d facts %+v", r.actor("a").MP, facts)
					}
					return facts, err
				}
				result, applyErr := r.applyPlayerOperation(op, &ordinaryRNG{value: .5})
				if applyErr != nil || !result.Accepted || result.ResponseReached {
					t.Fatalf("survival result: %+v %v", result, applyErr)
				}
			}
			a = r.actor("a")
			if err != nil || a.HP != tc.hp || a.Defense != tc.df || !tc.encounter && a.MP != 0 || r.actor("b").HP != 100 {
				t.Fatalf("survival result: HP %d DF %d MP %d ally HP %d error %v", a.HP, a.Defense, a.MP, r.actor("b").HP, err)
			}
		})
	}
}

func TestOrdinarySeparatorIsHiddenButRunsTheTail(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.Jobs.OldJobID, a.Jobs.OldSP, a.CMPNeedCharge = "job-01", 1, 3
	a.Custom = &corebattle.CustomLoadout{Name: "br", Gems: [3]string{"missing"}}
	a.Temporary = corebattle.TemporaryDefense
	cmd := RunCommand{ActorID: "a", Input: "＠br"}
	choices, err := r.DiscoverNormalOperations(cmd, ctx)
	if err != nil || slices.ContainsFunc(choices, func(c RunOperationChoice) bool { return c.Action == "br" || c.Action == "custom" }) {
		t.Fatalf("separator is visible: %+v %v", choices, err)
	}
	comment, err := r.ParseNormalOperation(RunCommand{ActorID: "a", Input: "＠unknown"}, ctx)
	if err != nil || comment.Kind != RunOperationComment {
		t.Fatalf("unknown command: %+v %v", comment, err)
	}
	result, err := applyOrdinaryTestAction(t, r, ctx, cmd, &ordinaryRNG{value: .5})
	if err != nil || !result.Accepted || !result.NormalAction || !result.ResponseReached || r.actor("a").Temporary != "" || r.actor("a").CMPCharge != 1 {
		t.Fatalf("separator did not reach the ordinary tail: %+v %v", result, err)
	}
	a = r.actor("a")
	a.Abilities = []string{"cursed"}
	comment, err = r.ParseNormalOperation(cmd, ctx)
	if err != nil || comment.Kind != RunOperationComment {
		t.Fatalf("unlisted separator: %+v %v", comment, err)
	}
}

func TestOrdinaryActionsRejectWithoutChangingTheRun(t *testing.T) {
	r, ctx := operationFixture()
	a := r.actor("a")
	a.Jobs.JobID, a.Jobs.SP, a.MP = "job-78", 55, 9
	r.actor("c").HP = 0
	r.Battle.SkillBans = map[string]int{"front": 2}
	before, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	rng := &ordinaryRNG{}
	for _, action := range []string{"front", "between-encounter-survival"} {
		if _, err := applyOrdinaryTestAction(t, r, ctx, RunCommand{ActorID: "a", Action: action}, rng); !errors.Is(err, ErrRunUnavailable) {
			t.Fatalf("%s rejected with %v", action, err)
		}
	}
	after, err := json.Marshal(r)
	if err != nil || !slices.Equal(before, after) || rng.calls != 0 {
		t.Fatalf("rejected action mutated the run or drew randomness: %v, draws %d", err, rng.calls)
	}

	r.actor("c").HP = 100
	delete(r.Battle.SkillBans, "front")
	op, err := ordinaryTestOperation(t, r, ctx, RunCommand{ActorID: "b", Action: "front"})
	if err != nil {
		t.Fatal(err)
	}
	failure := errors.New("test tail failure")
	op.LevelUp = func(state *corebattle.LifetimeState, _ random.Generator) (bool, error) {
		if state.Actors[0].ID != "b" {
			t.Error("tail ran before the formation")
		}
		return false, failure
	}
	before, err = json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = r.applyPlayerOperation(op, rng); !errors.Is(err, failure) {
		t.Fatalf("operation returned %v", err)
	}
	after, err = json.Marshal(r)
	if err != nil || !slices.Equal(before, after) {
		t.Fatalf("failed tail changed saved formation: %v", err)
	}
}
