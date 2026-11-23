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
	"github.com/witchcraze/party2re/internal/runhost"
)

func treasureCombatRun(t *testing.T) (*NormalRun, corecharacter.Character) {
	t.Helper()
	requester := corecharacter.Character{
		ID:       "a",
		PlayerID: "p1",
		Name:     "Alice",
		Level:    10,
		Stats:    corecharacter.Stats{HP: 500, MaxHP: 500, MP: 120, MaxMP: 120, Attack: 100, Defense: 50, Agility: 50},
	}
	r := &NormalRun{State: runhost.State{ID: "run-treasure", Admission: runhost.Admission{Speed: 3}, Round: TreasureRoomFloor, LeaderID: "a", Roster: []string{"a"}, Recipients: map[string]runhost.Recipient{
		"a": {PlayerID: "p1"},
	}, Battle: corebattle.LifetimeState{
		Actors: []corebattle.LifetimeActor{
			{
				Participant: corebattle.Participant{
					ID:        "a",
					Name:      "Alice",
					TeamID:    "allies",
					HP:        500,
					MaxHP:     500,
					MP:        120,
					MaxMP:     120,
					Attack:    100,
					Defense:   50,
					Agility:   50,
					BaseStats: corebattle.BattleBaseStats{Attack: 100, Defense: 50, Agility: 50},
					Gear:      &corebattle.BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01", WeaponAttack: new(10)},
				},
				Jobs:    corebattle.BattleJobs{JobID: "job-06", SP: 20, OldJobID: "job-78", OldSP: 200},
				Tension: 1,
			},
		},
	}, Boxes: []runhost.TreasureBox{
		{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"},
		{ID: "box-2", Name: "青い宝箱B", OpenedBy: "b", ItemID: "item-002"},
	}}, Stage: Stage{ID: "stage-00", Name: "Test Stage"}, Mode: CrawlModeNormal}
	return r, requester
}

func TestNormalRunTreasureProjectCombatStatsAndReconcile(t *testing.T) {
	r, requester := treasureCombatRun(t)
	r.Boxes = append(r.Boxes, runhost.TreasureBox{ID: "box-3", Name: "金の宝箱A", ItemID: "item-267", Fixed: true})

	state, frame, inspection, err := r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(frame.PartyIDs, []string{"a"}) || !slices.Equal(frame.EnemyIDs, []string{"box-1", "box-3"}) || len(frame.MemberOnlyIDs) != 0 {
		t.Fatalf("unexpected projected target context: %+v", frame)
	}
	if !inspection["box-1"].Treasure || inspection["box-1"].DefinitionID != "item-001" || inspection["box-1"].RewardKind != 3 ||
		!inspection["box-3"].Treasure || inspection["box-3"].DefinitionID != "item-267" || inspection["box-2"].Treasure {
		t.Fatalf("unexpected inspection projection: %+v", inspection)
	}

	b1 := runhost.FindActor(state.Actors, "box-1")
	if b1 == nil || b1.HP != 1000 || b1.MaxHP != 1000 || b1.Defense != 1000 || b1.BaseStats.Defense != 1000 ||
		b1.Temporary != corebattle.TemporaryNullMagic || !b1.NPC || b1.OverLevel || b1.Gear == nil || b1.Tension != 1 {
		t.Fatalf("unexpected ordinary box combat stats: %+v", b1)
	}
	b3 := runhost.FindActor(state.Actors, "box-3")
	if b3 == nil || b3.HP != 2000 || b3.MaxHP != 2000 || b3.Defense != 2000 || b3.BaseStats.Defense != 2000 ||
		b3.Temporary != corebattle.TemporaryNullMagic || !b3.NPC || !b3.OverLevel {
		t.Fatalf("unexpected fixed box combat stats: %+v", b3)
	}

	// Magic attack against unopened box is nullified by TemporaryNullMagic.
	level := requester.Level
	effects := RunActionEffects{}
	rewards := NewRunDefeatRewards(r, requester, "EXP", false, &ordinaryRNG{}, &effects)
	reset := corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: &level}
	defeat := corebattle.DefeatContext{EquipmentResetContext: reset, Rewards: rewards}
	damage := corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level},
		Defeat: func(id string) error {
			_, err := state.ApplyDefeat(&defeat, id, &ordinaryRNG{})
			return err
		},
		Reset: func(id string) error {
			return state.ApplyEquipmentReset(&reset, id, &ordinaryRNG{})
		},
	}
	res, err := state.ApplyDamage(&damage, corebattle.TargetEffect{
		TargetID: "box-1",
		Category: corebattle.CategoryMagic,
		Payload:  corebattle.CallbackPayload{Number: 500},
	}, &ordinaryRNG{})
	if err != nil || res.TargetID != "box-1" || b1.HP != 1000 {
		t.Fatalf("magic should be nullified by box TemporaryNullMagic: res=%+v b1=%+v err=%v", res, b1, err)
	}

	// Non-lethal physical damage persists onto r.Boxes[0].Combat and stays out of r.Battle.Actors.
	b1.HP = 700
	if err := r.ReconcileCombat(state, frame); err != nil {
		t.Fatal(err)
	}
	if len(r.Battle.Actors) != 1 || len(r.Enemies) != 0 || r.Boxes[0].Combat == nil || r.Boxes[0].Combat.HP != 700 || r.Boxes[0].Defeated || r.Boxes[2].Combat != nil {
		t.Fatalf("reconcile failed: actors=%+v boxes=%+v", r.Battle.Actors, r.Boxes)
	}
	if r.NextNPC(time.Unix(1000, 0)) != "" {
		t.Fatal("treasure room must disable NPC response")
	}

	// Reload from JSON preserves partial box HP and rejects lethal prize/reward leaks.
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var reloaded NormalRun
	if err := json.Unmarshal(raw, &reloaded); err != nil {
		t.Fatal(err)
	}
	reloadedState, reloadedFrame, _, err := reloaded.ProjectCombat("a")
	if err != nil || runhost.FindActor(reloadedState.Actors, "box-1").HP != 700 {
		t.Fatalf("reloaded box HP lost: err=%v actor=%+v", err, runhost.FindActor(reloadedState.Actors, "box-1"))
	}

	// Defeating box-1 via ApplyPlayerOperation clears it from next request without rewards or prize.
	reloadedEffects := RunActionEffects{}
	reloadedRewards := NewRunDefeatRewards(&reloaded, requester, "MONSTER", false, &ordinaryRNG{}, &reloadedEffects)
	reloadedReset := corebattle.EquipmentResetContext{TargetContext: reloadedFrame, RequesterLevel: &level}
	reloadedDefeat := corebattle.DefeatContext{EquipmentResetContext: reloadedReset, Rewards: reloadedRewards}
	reloadedDmg := corebattle.DamageContext{
		EffectContext: corebattle.EffectContext{TargetContext: reloadedFrame, RequesterLevel: &level},
		Defeat: func(id string) error {
			_, err := reloadedState.ApplyDefeat(&reloadedDefeat, id, &ordinaryRNG{})
			return err
		},
		Reset: func(id string) error {
			return reloadedState.ApplyEquipmentReset(&reloadedReset, id, &ordinaryRNG{})
		},
	}
	var defs skill.BattleDefinitions
	defs.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{BaseGuard: &corebattle.BaseGuardContext{DamageContext: reloadedDmg}}, 1)
	op := corebattle.PlayerOperation{
		Tail: corebattle.PlayerTailContext{
			DamageContext: reloadedDmg,
			DefeatContext: &reloadedDefeat,
		},
		First: corebattle.PlayerAction{
			EntryID: "attack",
			Validate: func(s *corebattle.LifetimeState) error {
				return s.ValidateDamage(&reloadedDmg, corebattle.TargetEffect{TargetID: "box-1", Category: corebattle.CategoryBreath, Payload: corebattle.CallbackPayload{Number: 5000}, Arguments: []string{"1"}}, &ordinaryRNG{})
			},
			Execute: func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
				d := reloadedDmg
				d.TargetContext = reloadedFrame
				_, err := s.ApplyDamage(&d, corebattle.TargetEffect{TargetID: "box-1", Category: corebattle.CategoryBreath, Payload: corebattle.CallbackPayload{Number: 5000}, Arguments: []string{"1"}}, rng)
				return corebattle.PlayerActionFacts{DamageCalls: 1}, err
			},
		},
		ResponseEnabled: false,
	}
	opRes, err := reloadedState.ApplyPlayerOperation(&op, &ordinaryRNG{})
	if err != nil || !opRes.Accepted || opRes.ResponseReached {
		t.Fatalf("player operation against chest failed: res=%+v err=%v", opRes, err)
	}
	if err := reloaded.ReconcileCombat(reloadedState, opRes.Target); err != nil {
		t.Fatal(err)
	}
	if !reloaded.Boxes[0].Defeated || reloaded.Boxes[0].OpenedBy != "" || reloaded.Boxes[0].Combat != nil {
		t.Fatalf("defeated box state wrong: box=%+v combat=%+v actor=%+v", reloaded.Boxes[0], reloaded.Boxes[0].Combat, runhost.FindActor(reloadedState.Actors, "box-1"))
	}
	if len(reloadedEffects.Earnings) != 0 || len(reloadedEffects.Defeats) != 0 || len(reloadedEffects.CreditedDefeats) != 0 {
		t.Fatalf("defeating box leaked rewards/defeats: %+v", reloadedEffects)
	}
	// box-3 is still unopened; once box-3 is defeated too, livingEnemies is false.
	reloaded.Boxes[2].Defeated = true
	if reloaded.LivingEnemies() {
		t.Fatal("expected livingEnemies=false after all boxes opened or defeated")
	}
	_, nextFrame, _, err := reloaded.ProjectCombat("a")
	if err != nil || len(nextFrame.EnemyIDs) != 0 {
		t.Fatalf("defeated boxes remained in next enemy pool: %+v err=%v", nextFrame, err)
	}
	if _, _, err := reloaded.ClaimChest(nil, nil, "a", "box-1"); !errors.Is(err, ErrNoUnopenedTreasure) {
		t.Fatalf("defeated box still claimable: %v", err)
	}
}

func TestNormalRunTreasureReconcilePreservesOrderAfterMemberRemoval(t *testing.T) {
	r, _ := treasureCombatRun(t)
	r.Boxes = []runhost.TreasureBox{
		{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"},
		{ID: "box-2", Name: "青い宝箱B", OpenedBy: "b", ItemID: "item-002"},
		{ID: "box-3", Name: "黒い宝箱C", ItemID: "item-003"},
	}
	state, frame, _, err := r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate job-79-entry-2 moving box-1 to MemberOnlyIDs while box-3 remains in EnemyIDs.
	frame.EnemyIDs = []string{"box-3"}
	frame.MemberOnlyIDs = []string{"box-1"}
	if err := r.ReconcileCombat(state, frame); err != nil {
		t.Fatal(err)
	}
	if r.Boxes[0].ID != "box-3" || r.Boxes[1].ID != "box-2" || r.Boxes[2].ID != "box-1" {
		t.Fatalf("unexpected box order after reconcile: %+v", r.Boxes)
	}
}

func TestNormalRunMixedEnemyAndTreasureInspectionPreservesEnemyReward(t *testing.T) {
	r, requester := treasureCombatRun(t)
	r.Round = 3
	r.Enemies = []string{"enemy-1"}
	r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{
		Participant: corebattle.Participant{ID: "enemy-1", Name: "Slime", TeamID: "enemies", HP: 100, MaxHP: 100},
		NPC:         true,
	})
	r.Boxes = []runhost.TreasureBox{{ID: "box-1", Name: "普通の宝箱A", ItemID: "item-001"}}

	state, frame, projectionInspection, err := r.ProjectCombat("a")
	if err != nil {
		t.Fatal(err)
	}
	level := requester.Level
	damage := corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame, RequesterLevel: &level}}
	tools := &corebattle.OrdinaryToolContext{
		DamageContext: damage,
		OrdinaryToolInput: corebattle.OrdinaryToolInput{
			Inspection: map[string]corebattle.InspectionContent{
				"a":       {Empty: true},
				"enemy-1": {RewardKind: 1, DefinitionID: "weapon-01", Price: 500},
			},
		},
	}
	var defs skill.BattleDefinitions
	defs.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{Tools: tools}, len(r.Roster))
	defs.RegisterOrdinaryTools(tools)

	// Intentionally leave Callback.Inspection empty so normalOperations overlays ProjectCombat's map.
	opCtx := NormalOperationContext{
		Definitions: defs,
		Callback:    corebattle.CallbackContext{Damage: &damage},
		Requester:   &requester,
	}
	searchCmd, err := r.ParseNormalOperation(runhost.Command{ActorID: "a", Action: "job-78-entry-1", TargetID: "weapon-01"}, opCtx)
	if err != nil {
		t.Fatalf("parse enemy reward search failed: %v", err)
	}
	boxCmd, err := r.ParseNormalOperation(runhost.Command{ActorID: "a", Input: "＠トレハンのかん>普通の宝箱A"}, opCtx)
	if err != nil {
		t.Fatalf("parse box valuation failed: %v", err)
	}

	for _, parsed := range []ParsedRunOperation{searchCmd, boxCmd} {
		cb := corebattle.CallbackContext{
			TargetContext: frame,
			Arguments:     parsed.First.Arguments,
			Inspection:    projectionInspection,
		}
		if err := state.ValidateInvocations(&cb, defs.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, &ordinaryRNG{}); err != nil {
			t.Fatalf("validate %+v: %v", parsed.First, err)
		}
		if err := state.ApplyInvocations(&cb, defs.Invocations, []corebattle.CallbackInvocation{parsed.First.Invocation}, &ordinaryRNG{}); err != nil {
			t.Fatalf("apply %+v: %v", parsed.First, err)
		}
	}

	if len(tools.Results) != 2 || len(tools.Results[0].Facts) != 1 || len(tools.Results[1].Facts) != 1 {
		t.Fatalf("unexpected inspection results: %+v", tools.Results)
	}
	if f := tools.Results[0].Facts[0]; f.Kind != "reward-search" || !f.Found || f.TargetID != "enemy-1" {
		t.Fatalf("ordinary enemy reward lost during mixed inspection: %+v", f)
	}
	if f := tools.Results[1].Facts[0]; f.Kind != "valuation" || f.TargetID != "box-1" {
		t.Fatalf("projected box valuation failed: %+v", f)
	}
	if got := tools.Inspection["enemy-1"]; got.DefinitionID != "weapon-01" || got.Empty {
		t.Fatalf("ordinary enemy inspection entry overwritten: %+v", got)
	}
}
