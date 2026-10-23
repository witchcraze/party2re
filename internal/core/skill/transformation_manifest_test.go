package skill

import (
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// Independent source vectors at ff97fc5:_skill.cgi. Each row binds the full
// intrinsic body, not merely availability of its damage/stat descendant.
func TestTransformationManifestIntrinsicResults(t *testing.T) {
	for _, tc := range []struct {
		id, span         string
		job, pos, sp, mp int
		draws            []float64
	}{
		{"job-36-copy", "2946–2964", 36, 4, 100, 50, nil},
		{"job-79-breath", "6279–6287", 79, 3, 77, 21, nil},
		{"job-79-speed", "6288–6292", 79, 3, 77, 21, nil},
		{"job-82-form", "6596–6610", 82, 6, 111, 10, nil},
		{"job-300-tune", "7470–7475", 300, 0, 0, 30, []float64{0, .99}},
		{"job-300-strike", "7480–7486", 300, 1, 0, 48, nil},
		{"job-300-burst", "7491–7515", 300, 2, 0, 99, []float64{0, 0}},
		{"job-300-form", "7520–7530", 300, 3, 0, 0, nil},
	} {
		t.Run(tc.id, func(t *testing.T) {
			s, c, d := transformationRegistryFixture()
			a := &s.Actors[0]
			if tc.job == 300 {
				a.Jobs.JobID = "job-300"
			}
			s.Actors[2].Temporary = corebattle.TemporaryStrongDefense
			calls := 0
			c.AreaDamage = func(ctx corebattle.TargetContext, power float64, category corebattle.CallbackCategory, direct bool, _ random.Generator) error {
				calls++
				if ctx.ActorID != "self" || power != 150 || category != corebattle.CategoryBreath || !direct {
					t.Fatal(tc.span, ctx, power, category, direct)
				}
				return nil
			}
			c.Damage = func(ctx corebattle.TargetContext, e corebattle.TargetEffect, _ random.Generator) error {
				calls++
				power, category := 200.0, corebattle.CategoryBreath
				if tc.id == "job-300-strike" {
					power, category = 100, corebattle.CategoryPhysical
					if s.Actors[2].Temporary != "" {
						t.Fatal("strike did not clear temporary")
					}
					s.Actors[2].Temporary = corebattle.TemporaryDefense
				}
				if e.TargetID != "enemy" || e.Category != category || e.Payload.Number != power || !reflect.DeepEqual(e.Arguments, []string{"1"}) || ctx.ActorID != "self" {
					t.Fatal(tc.span, ctx, e)
				}
				c.DamageCalls++
				return nil
			}
			d.RegisterTransformations(&c)
			target := "enemy"
			if tc.id == "job-36-copy" {
				target = "ally"
			}
			entry, err := d.TransformationAction(&s, &c, tc.id, target, noArithmeticDraws{t})
			if err != nil || entry.RequiredSP != tc.sp || entry.MPCost != tc.mp {
				t.Fatal(tc.span, entry, err)
			}
			variant := 0
			if tc.id == "job-79-speed" {
				variant = 1
			}
			if d.Invocations.JobTables[tc.job].Variants[variant][tc.pos].ID != tc.id {
				t.Fatal("wrong source position", tc.span)
			}
			frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{target, "trailing"}}
			rng := &damageGemDraws{t: t, draws: tc.draws}
			if err := s.ApplyInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: tc.id}}, rng); err != nil || rng.used != len(tc.draws) {
				t.Fatal(tc.span, err, rng)
			}
			switch tc.id {
			case "job-36-copy":
				if a.Jobs != s.Actors[1].Jobs || a.MP != 50 || a.MaxMP != 20 || a.Icon != "ally.svg" || a.BaseStats != s.Actors[1].BaseStats {
					t.Fatal(a)
				}
			case "job-79-breath":
				if calls != 1 || a.Icon != "assets/images/placeholders/battle/forms.svg#winged-m" {
					t.Fatal(calls, a)
				}
			case "job-79-speed":
				if a.Agility != 250 || a.Icon != c.RequesterIcon || c.StatUpCalls != 1 {
					t.Fatal(a, c)
				}
			case "job-82-form":
				if a.Jobs.JobID != "job-300" || a.Jobs.OldJobID != "job-36" || a.Icon != "assets/images/placeholders/battle/forms.svg#mechanical-m" {
					t.Fatal(a)
				}
			case "job-300-tune":
				if a.Agility != 150 || a.Attack != 150 || a.Defense != 100 || c.StatUpCalls != 2 {
					t.Fatal(a, c)
				}
			case "job-300-strike":
				if calls != 1 || c.DamageCalls != 1 || s.Actors[2].Temporary != corebattle.TemporaryStrongDefense {
					t.Fatal(calls, c, s)
				}
			case "job-300-burst":
				if calls != 2 || c.DamageCalls != 2 || a.Jobs.JobID != "job-82" || a.Status != corebattle.StatusDofuu || a.Tension != 1 {
					t.Fatal(calls, a, c)
				}
			case "job-300-form":
				if a.Jobs.JobID != "job-82" || a.Icon != c.RequesterIcon {
					t.Fatal(a)
				}
			}
			if tc.id != "job-36-copy" && a.MP != 200 || a.CMP != 0 {
				t.Fatal("intrinsic callback charged an outer cost", a)
			}
		})
	}
}

func TestTransformationMissingDescendantsAndEmptyOuterTargetRejectBeforeExecution(t *testing.T) {
	for _, id := range []string{"job-79-breath", "job-79-speed", "job-300-tune", "job-300-strike", "job-300-burst"} {
		s, c, d := transformationRegistryFixture()
		d.RegisterTransformations(&c)
		if id == "job-79-speed" || id == "job-300-tune" {
			s.Actors[0].ItemDefinitionIDs = []string{"item-201"}
		}
		before := append([]corebattle.LifetimeActor(nil), s.Actors...)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		ops := []corebattle.CallbackInvocation{{Kind: "entry", EntryID: "job-82-form"}, {Kind: "entry", EntryID: id}}
		if err := s.ApplyInvocations(&frame, d.Invocations, ops, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) || !reflect.DeepEqual(before, s.Actors) || c.StatUpCalls != 0 || c.DamageCalls != 0 {
			t.Fatal(id, err, s, c)
		}
	}
	for _, id := range []string{"job-300-strike", "job-300-burst"} {
		s, c, d := transformationRegistryFixture()
		s.Actors[2].HP = 0
		c.Damage = func(corebattle.TargetContext, corebattle.TargetEffect, random.Generator) error {
			t.Fatal("unrepresented target executed")
			return nil
		}
		d.RegisterTransformations(&c)
		frame := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy"}}
		if err := s.ValidateInvocations(&frame, d.Invocations, []corebattle.CallbackInvocation{{Kind: "entry", EntryID: id}}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
}
