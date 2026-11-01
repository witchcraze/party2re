package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestSummonGemRegisteredSkippedAndMissingPool(t *testing.T) {
	for _, count := range []int{1, 2, 9, 10} {
		s, c, tc, d := summonAppearanceFixture()
		if count == 2 {
			a := s.Actors[1]
			a.ID, a.HP = "fallen-enemy", 0
			s.Actors = append(s.Actors, a)
			c.EnemyIDs = append(c.EnemyIDs, a.ID)
		} else {
			for len(s.Actors) < count {
				a := s.Actors[0]
				a.ID = "member-" + strconv.Itoa(len(s.Actors))
				s.Actors = append(s.Actors, a)
				c.PartyIDs = append(c.PartyIDs, a.ID)
			}
		}
		c.EnemyPool, c.AppearancePositions, c.RequesterLevel, c.EnemyTeamID = nil, nil, nil, ""
		d.RegisterSummonAppearanceGems(&c, &tc)
		d.RegisterDefenseGems()
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}}
		ids := []string{"gem-82", "gem_guard_1"}
		ops, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t})
		if count == 1 || count == 9 {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(count, "reachable summon lost stage prerequisite", err)
			}
			continue
		}
		if err != nil {
			t.Fatal(count, err)
		}
		if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); err != nil || s.Actors[0].Temporary != corebattle.TemporaryDefense || s.RosterSequence != 0 || c.DamageCalls != 0 {
			t.Fatal(count, "skipped summon lost unchanged continuation", err, s, c)
		}
	}
}

func TestSummonAppearanceRegisteredProjectionNeverRunsPorts(t *testing.T) {
	s, c, tc, d := summonAppearanceFixture()
	apply := func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
		t.Fatal("projection executed external Apply port")
		return nil
	}
	c.Damage = apply
	c.Defeat = func(string) error { t.Fatal("projection executed defeat"); return nil }
	tc.Damage = apply
	tc.AreaDamage = func(corebattle.TargetContext, float64, corebattle.CallbackCategory, bool, random.Generator) error {
		t.Fatal("projection executed area damage")
		return nil
	}
	c.DamageCalls, c.TensionCalls = 7, 3
	d.RegisterSummonAppearanceGems(&c, &tc)
	d.RegisterDefenseGems()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self", "tail"}, Damage: &c.DamageContext, ArithmeticDivisor: 11}
	before, _ := json.Marshal(s)
	poolBefore, _ := json.Marshal(c.EnemyPool)
	for _, id := range []string{"gem-82", "gem-100-appearance"} {
		if _, err := d.InvocationAction(&s, &f, []string{id, "gem_guard_1"}, noArithmeticDraws{t}); err != nil {
			t.Fatal(id, err)
		}
		branches, err := d.Invocations.Entries[id].ProjectBranches(&s, &f)
		if err != nil || len(branches) < 2 {
			t.Fatal(id, err)
		}
		first := &branches[0]
		if first.Context.Damage.DamageCalls != 7 || first.Context.Damage.TensionCalls != 3 || first.Context.ArithmeticDivisor != 11 {
			t.Fatal("lost enclosing operation counters", id, first)
		}
		if id == "gem-82" {
			a := &first.State.Actors[2]
			*a.Gear.WeaponAttack = 999
			*a.JobHistory["job-7"] = 999
			a.CustomSkills[0].Gems[0].Kind = "changed"
			if *branches[1].State.Actors[2].Gear.WeaponAttack != 12 || *branches[1].State.Actors[2].JobHistory["job-7"] != 17 || branches[1].State.Actors[2].CustomSkills[0].Gems[0].Kind != "unresolved" {
				t.Fatal("sibling shared payload referents")
			}
		}
		*first.Context.Damage.RequesterLevel = 999
		first.Context.Arguments[0] = "changed"
		first.Context.EnemyIDs[0] = "changed"
		if *branches[1].Context.Damage.RequesterLevel != 70 || branches[1].Context.Arguments[0] != "self" || branches[1].Context.EnemyIDs[0] != "enemy" {
			t.Fatal("sibling shared frame referents")
		}
	}
	after, _ := json.Marshal(s)
	poolAfter, _ := json.Marshal(c.EnemyPool)
	if string(before) != string(after) || string(poolBefore) != string(poolAfter) || c.DamageCalls != 7 || c.TensionCalls != 3 || *c.RequesterLevel != 70 || f.Arguments[0] != "self" || c.EnemyIDs[0] != "enemy" {
		t.Fatal("registered projection changed caller facts", s, c, f)
	}
}

func TestSummonAppearanceFrozenPayloadReloadAndRebuiltPorts(t *testing.T) {
	type frozenInput struct {
		Pool              []corebattle.LifetimeActor
		Positions         []int
		Hard              bool
		EnemyTeamID       string
		Icons             [108]string
		Target            corebattle.TargetContext
		Arguments         []string
		Next, DamageCalls int
	}
	type checkpoint struct {
		State corebattle.LifetimeState
		Input frozenInput
	}
	var finals []checkpoint
	for _, reload := range []bool{false, true} {
		s, c, tc, _ := summonAppearanceFixture()
		saved := checkpoint{s, frozenInput{Pool: c.EnemyPool, Positions: c.AppearancePositions, Hard: c.Hard, EnemyTeamID: c.EnemyTeamID, Icons: tc.MonsterIcons, Target: c.TargetContext, Arguments: []string{"self", "tail"}, DamageCalls: 7}}
		ids := []string{"gem-82", "gem_guard_1", "gem-100-appearance"}
		for saved.Input.Next < len(ids) {
			in := &saved.Input
			// Only resulting facts and frozen input are persisted, never closures.
			roster := corebattle.RosterContext{EnemyPool: in.Pool, AppearancePositions: in.Positions, Hard: in.Hard, EnemyTeamID: in.EnemyTeamID}
			appearance := corebattle.TransformationContext{MonsterIcons: in.Icons}
			roster.TargetContext, roster.DamageCalls = in.Target, in.DamageCalls
			var rebuilt BattleDefinitions
			rebuilt.RegisterSummonAppearanceGems(&roster, &appearance)
			rebuilt.RegisterDefenseGems()
			f := corebattle.CallbackContext{TargetContext: in.Target, Arguments: in.Arguments, Damage: &roster.DamageContext}
			ops, err := rebuilt.InvocationAction(&saved.State, &f, ids[in.Next:], noArithmeticDraws{t})
			if err != nil {
				t.Fatal(reload, in.Next, err)
			}
			draws := [][]float64{{.999, 0}, nil, {.999}}[in.Next]
			rng := &damageGemDraws{t: t, draws: draws}
			if err := saved.State.ApplyInvocations(&f, rebuilt.Invocations, ops[:1], rng); err != nil || rng.used != len(draws) {
				t.Fatal(reload, in.Next, err, rng)
			}
			in.Target, in.Arguments, in.DamageCalls, in.Next = f.TargetContext, f.Arguments, f.Damage.DamageCalls, in.Next+1
			if reload {
				data, err := json.Marshal(saved)
				if err != nil {
					t.Fatal(err)
				}
				saved = checkpoint{}
				if err := json.Unmarshal(data, &saved); err != nil {
					t.Fatal(err)
				}
			}
		}
		if saved.State.RosterSequence != 1 || len(saved.State.Actors) != 3 || saved.State.Actors[2].RosterIdentity != "enemy-species-Z" || saved.State.Actors[0].Icon != tc.MonsterIcons[107] || saved.State.Actors[0].Temporary != corebattle.TemporaryDefense || saved.Input.DamageCalls != 7 || *saved.Input.Pool[0].Gear.WeaponAttack != 12 || *saved.Input.Pool[0].JobHistory["job-7"] != 17 {
			t.Fatal(saved)
		}
		finals = append(finals, saved)
	}
	if !reflect.DeepEqual(finals[0], finals[1]) {
		t.Fatal("reload changed roster, payload or frozen facts", finals)
	}
}
