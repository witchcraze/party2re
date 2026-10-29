package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestTreatmentChanceAdmissionPurity(t *testing.T) {
	for _, job := range []int{43, 45, 46} {
		for _, entry := range corebattle.TreatmentChanceEntries(job) {
			if entry.ID == "job-43-entry-6" {
				continue
			}
			for _, reject := range []bool{false, true} {
				s, c, d := chanceFixture()
				if reject {
					delete(d.Invocations.Entries, entry.ID)
				}
				before, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				original := *c
				defeat := *c.Status.Defeat
				c.Defeat = func(string) error { t.Fatal("external defeat during admission"); return nil }
				c.Status.Defeat.Rewards.Apply = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
					t.Fatal("external reward during admission")
					return nil
				}
				f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
				frame := f
				_, err = d.InvocationAction(&s, &f, []string{entry.ID}, noArithmeticDraws{t})
				if (err != nil) != reject {
					t.Fatal(entry.ID, reject, err)
				}
				after, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				if string(before) != string(after) || !reflect.DeepEqual(f, frame) || !reflect.DeepEqual(c.Arguments, original.Arguments) || c.DamageCalls != 0 || c.Status.DeathCalls != 0 || c.Status.CureCalls != 0 || !reflect.DeepEqual(c.Status.Defeat.EquipmentResetContext, defeat.EquipmentResetContext) {
					t.Fatal(entry.ID, "admission changed caller facts")
				}
			}
		}
	}
}

func TestTreatmentChanceMissingDescendantsRejectWholeAction(t *testing.T) {
	for _, id := range []string{"job-46-entry-0", "job-46-entry-1", "job-46-entry-3", "job-46-entry-5"} {
		s, c, d := chanceFixture()
		c.Status.Defeat = nil
		before := s.Actors[0].Participant
		f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		if err := s.ApplyInvocations(&f, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if !reflect.DeepEqual(s.Actors[0].Participant, before) || c.DamageCalls != 0 || c.Status.DeathCalls != 0 {
			t.Fatal(id, s.Actors, c)
		}
	}
	// An unsupported unchosen triple still rejects before any symbol is drawn.
	s, c, d := chanceFixture()
	c.Defeat = nil
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	if _, err := d.InvocationAction(&s, &f, []string{"job-46-entry-0"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}

func TestTreatmentChanceCoinProjectsEarlierDefeatBeforeLaterBranch(t *testing.T) {
	for _, changed := range []bool{false, true} {
		s, c, d := chanceFixture()
		other := s.Actors[1]
		other.ID, other.HP, other.BaseStats.Defense = "later", 500, 1000
		s.Actors = append(s.Actors, other)
		c.EnemyIDs = append(c.EnemyIDs, "later")
		// Initially raw halving never reaches this unsupported reset accessory.
		s.Actors[3].Gear = &corebattle.BattleGearStats{AccessoryID: "item-110"}
		project := func(p *corebattle.LifetimeState, _ corebattle.TargetContext, id string) error {
			if changed && id == "enemy" {
				p.Actors[3].BaseStats.Defense = 999
			}
			return nil
		}
		c.Status.Defeat.Rewards.Project = project
		c.Status.Defeat.Rewards.Apply = project
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		_, err := d.InvocationAction(&s, &f, []string{"job-46-entry-5"}, noArithmeticDraws{t})
		if changed {
			if !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
		} else {
			if err != nil {
				t.Fatal(err)
			}
			executeChance(t, &s, c, d, "job-46-entry-5", nil, 0, .99, 0)
			if s.Actors[3].HP != 250 {
				t.Fatal(s.Actors)
			}
		}
		if s.Actors[3].BaseStats.Defense != 1000 || changed && (s.Actors[1].HP != 900 || c.Status.DeathCalls != 0) {
			t.Fatal(s.Actors, c)
		}
	}
}

func TestTreatmentChanceCoinNativeDamageContinuation(t *testing.T) {
	for _, mode := range []string{"missing", "neutral", "changed", "supported-change", "error"} {
		s, c, d := chanceFixture()
		s.Actors[1].HP, s.Actors[1].MaxHP = 1100, 3600
		later := s.Actors[1]
		later.ID, later.HP, later.BaseStats.Defense = "later", 500, 1000
		later.Gear = &corebattle.BattleGearStats{AccessoryID: "item-110"}
		if mode == "supported-change" {
			later.Gear = &corebattle.BattleGearStats{}
		}
		s.Actors = append(s.Actors, later)
		c.EnemyIDs = append(c.EnemyIDs, "later")
		project := func(p *corebattle.LifetimeState, _ corebattle.TargetContext, id string) error {
			if (mode == "changed" || mode == "supported-change") && id == "enemy" {
				p.Actors[3].BaseStats.Defense = 999
			}
			return nil
		}
		c.Status.Defeat.Rewards.Project, c.Status.Defeat.Rewards.Apply = project, project
		if mode == "missing" {
			c.Status.Defeat = nil
		}
		if mode == "error" {
			c.Status.Defeat.Rewards.Validate = func(*corebattle.LifetimeState, corebattle.TargetContext, string) error {
				return errors.New("reward unavailable")
			}
		}
		f := corebattle.CallbackContext{TargetContext: c.TargetContext}
		_, err := d.InvocationAction(&s, &f, []string{"job-46-entry-5"}, noArithmeticDraws{t})
		if mode == "neutral" || mode == "supported-change" {
			if err != nil {
				t.Fatal(mode, err)
			}
			executeChance(t, &s, c, d, "job-46-entry-5", nil, 0, 0, 0, 0)
			want := 250
			if mode == "supported-change" {
				want = 0
			}
			if s.Actors[1].HP != 0 || s.Actors[3].HP != want || c.DamageCalls != 1 {
				t.Fatal(s.Actors, c)
			}
		} else if err == nil || s.Actors[1].HP != 1100 || s.Actors[3].HP != 500 || c.DamageCalls != 0 {
			t.Fatal(mode, err, s.Actors, c)
		}
	}
}

func TestTreatmentChanceDefaultEmptyRevivalAndIndirectAdmission(t *testing.T) {
	s, c, d := chanceFixture()
	var own, nilContext BattleDefinitions
	own.RegisterTreatmentChance(c)
	nilContext.RegisterTreatmentChance(nil)
	if len(own.Invocations.Entries) != 20 {
		t.Fatal(len(own.Invocations.Entries))
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	for _, empty := range []BattleDefinitions{{}, nilContext} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-43-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for range 2 {
		d.RegisterTreatmentChance(c)
		executeChance(t, &s, c, d, "job-43-entry-6", []string{"fallen"})
		if s.Actors[2].HP != 101 {
			t.Fatal(s.Actors)
		}
		s.Actors[2].HP = 0
		d.RegisterRevivals(&corebattle.RevivalContext{TargetContext: c.TargetContext})
	}
	for job, count := range map[int]int{43: 7, 45: 8, 46: 6} {
		if len(d.Invocations.JobTables[job].Variants[0]) != count {
			t.Fatal(job)
		}
	}
	// Ordinary intrinsic execution leaves listed MP to the consumer. Delegation
	// admits every entry and waives a selected child's listed cost/SP requirement.
	s.Actors[0].Jobs.JobID, s.Actors[0].Jobs.SP, s.Actors[0].MP = "job-43", 0, 0
	d.Invocations.Entries["delegate"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "tool-84"}}
	executeChance(t, &s, c, d, "delegate", nil, 5.1/7, .9, 0)
	if s.Actors[0].HP != 860 || s.Actors[0].MP != 0 {
		t.Fatal(s.Actors)
	}
	delete(d.Invocations.Entries, "job-43-entry-6")
	if _, err := d.InvocationAction(&s, &f, []string{"delegate"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
}
