package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestBasicMagicAdmissionIsInert(t *testing.T) {
	_, _, catalog := basicMagicFixture()
	for id := range catalog.Invocations.Entries {
		for _, reject := range []bool{false, true} {
			s, c, d := basicMagicFixture()
			if reject {
				delete(d.Invocations.Entries, id)
			}
			weapon := 7
			gear := s.Actors[0].Gear
			gear.WeaponAttack = &weapon
			c.Reset = func(string) error { t.Fatal("reset during admission"); return nil }
			c.Defeat = func(string) error { t.Fatal("defeat during admission"); return nil }
			c.Damage = func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
				t.Fatal("stat tail during admission")
				return nil
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}}
			before := basicMagicSnapshot(t, s)
			_, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t})
			if (err != nil) != reject {
				t.Fatal(id, reject, err)
			}
			if before != basicMagicSnapshot(t, s) || f.Arguments[0] != "enemy" || f.PartyIDs[0] != "self" || c.DamageCalls != 0 || c.StatUpCalls != 0 || c.HealCalls != 0 || c.StatusCalls != 0 || c.CureCalls != 0 || c.Arguments != nil || weapon != 7 || s.Actors[0].Gear != gear || gear.WeaponAttack != &weapon {
				t.Fatal(id, s, c, f)
			}
		}
	}
}

func basicMagicSnapshot(t *testing.T, s corebattle.LifetimeState) string {
	t.Helper()
	b, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestBasicMagicRejectsMissingDescendantsBeforeMutation(t *testing.T) {
	for _, row := range []string{"damage", "status-gear", "stat-tension", "reset", "activation", "unknown-later", "missing-representative", "forbidden-item"} {
		t.Run(row, func(t *testing.T) {
			s, c, d := basicMagicFixture()
			ids, target := []string{"job-6-entry-5"}, "enemy"
			switch row {
			case "damage":
				c.Defeat = nil
			case "status-gear":
				ids[0], s.Actors[1].Gear = "job-6-entry-6", nil
			case "stat-tension":
				ids[0], target = "job-5-entry-1", "self"
				s.Actors[0].ItemDefinitionIDs = []string{"item-205"}
			case "reset":
				s.Actors[0].Extended = corebattle.ExtendedPurification
			case "activation":
				ids[0] = "job-5-entry-9"
				other := s.Actors[1]
				other.ID = "other"
				s.Actors = append(s.Actors, other)
				c.EnemyIDs = append(c.EnemyIDs, other.ID)
				s.Actors[1].ItemDefinitionIDs = []string{"item-195"}
				s.Actors[2].HP, s.Actors[2].Gear = 100, nil
			case "unknown-later":
				ids = append(ids, "unknown")
			case "missing-representative":
				ids = append(ids, "job-6-entry-1")
			case "forbidden-item":
				ids = append(ids, "item-110")
			}
			before := basicMagicSnapshot(t, s)
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{target, "tail"}}
			if _, err := d.InvocationAction(&s, &f, ids, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			ops := make([]corebattle.CallbackInvocation, len(ids))
			for i, id := range ids {
				ops[i] = corebattle.CallbackInvocation{Kind: "entry", EntryID: id}
			}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(err)
			}
			if before != basicMagicSnapshot(t, s) || c.Arguments != nil || c.DamageCalls != 0 || f.Arguments[0] != target {
				t.Fatal(s, c, f)
			}
		})
	}
}

func TestBasicMagicDefaultEmptyAndSeparateOwners(t *testing.T) {
	s, c, d := basicMagicFixture()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext}
	var missing BattleDefinitions
	missing.RegisterBasicMagic(nil)
	for _, empty := range []BattleDefinitions{{}, missing} {
		if _, err := empty.InvocationAction(&s, &f, []string{"job-6-entry-5"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"job-5-entry-3", "job-5-entry-7", "job-6-entry-1", "job-6-entry-3", "item-110", "gem-101", "gem-102"} {
		if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	if corebattle.BasicMagicEntries(7) != nil {
		t.Fatal("unexpected job table")
	}
	// A representative owner's binding must survive table assembly unchanged.
	d.Invocations.Entries["job-5-entry-3"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "external-owner"}}
	d.RegisterBasicMagic(c)
	if d.Invocations.Entries["job-5-entry-3"].Invocation.Kind != "external-owner" || len(d.Invocations.JobTables[5].Variants[0]) != 10 || len(d.Invocations.JobTables[6].Variants[0]) != 10 {
		t.Fatal(d.Invocations)
	}
	for _, reverse := range []bool{false, true} {
		var combined BattleDefinitions
		revival := &corebattle.RevivalContext{TargetContext: c.TargetContext}
		if reverse {
			combined.RegisterRevivals(revival)
		}
		combined.RegisterBasicMagic(c)
		if !reverse {
			combined.RegisterRevivals(revival)
		}
		if !reflect.DeepEqual(combined.Invocations.JobTables, d.Invocations.JobTables) || len(combined.Invocations.Entries) != 16+len(corebattle.RevivalEntries()) {
			t.Fatal(combined)
		}
		if combined.Invocations.JobTables[5].Variants[0][7] != corebattle.RevivalEntries()[1] {
			t.Fatal("revival metadata changed")
		}
		s.Actors[2].MaxHP = 101
		executeBasicMagic(t, &s, c, combined, "job-5-entry-7", []string{"fallen"}, 0)
		if s.Actors[2].HP != 50 {
			t.Fatal(s)
		}
		s.Actors[2].HP = 0
		// Full tool/random admission must retain missing representative owners.
		s.Actors[0].Jobs.JobID = "job-05"
		for _, kind := range []string{"tool-84", "random"} {
			if err := s.ValidateInvocations(&f, combined.Invocations, []corebattle.CallbackInvocation{{Kind: kind}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(kind, err)
			}
		}
	}
}

func TestBasicMagicAcceptedNoEffectNeedsNoUnreachableDescendants(t *testing.T) {
	s, c, d := basicMagicFixture()
	s.Actors[0].MP, s.Actors[0].Gear, s.Actors[1].Gear, c.Defeat = 0, nil, nil, nil
	for _, id := range []string{"job-5-entry-0", "job-6-entry-0"} {
		executeBasicMagic(t, &s, c, d, id, []string{"enemy"})
		if c.DamageCalls != 0 || s.Actors[1].HP != 900 {
			t.Fatal(s, c)
		}
	}
	s, c, d = basicMagicFixture()
	s.Actors[0].Defense, s.Actors[0].ItemDefinitionIDs = 200, []string{"item-205"}
	// At the2.5x cap, the requester-dependent205 tail cannot run.
	executeBasicMagic(t, &s, c, d, "job-5-entry-1", []string{"self"})
	if s.Actors[0].Defense != 200 || s.Actors[0].Temporary != "" {
		t.Fatal(s, c)
	}
}

func TestBasicMagicMetadataUsesOrdinaryAdmissionBoundaries(t *testing.T) {
	_, _, d := basicMagicFixture()
	for job, table := range d.Invocations.JobTables {
		for _, entry := range table.Variants[0] {
			if _, own := d.Invocations.Entries[entry.ID]; !own {
				continue
			}
			jobID := "job-05"
			if job == 6 {
				jobID = "job-06"
			}
			def := Definition{ID: entry.ID, RequiredJobIDs: []string{jobID}, RequiredSP: entry.RequiredSP, MPCost: entry.MPCost}
			for _, sp := range []int{entry.RequiredSP - 1, entry.RequiredSP} {
				for _, mp := range []int{entry.MPCost - 1, entry.MPCost} {
					if mp < 0 {
						continue // MP0 is already the minimum valid resource.
					}
					ch := &corecharacter.Character{JobID: jobID, SP: sp}
					ch.Stats.MP = mp
					err := def.CanUse(UseRequest{Character: ch})
					want := sp >= entry.RequiredSP && mp >= entry.MPCost
					if (err == nil) != want || ch.Stats.MP != mp {
						t.Fatal(entry, sp, mp, err)
					}
				}
			}
		}
	}
}
