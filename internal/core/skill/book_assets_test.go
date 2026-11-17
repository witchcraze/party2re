package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

// Vectors: ff97fc5:_skill.cgi:6806–6899,10489–10557,10831–10839;
// _battle.cgi:668–675,1137–1150. Listed MP is paid by the outer consumer.
func heldBookFixture(held string, rows ...corebattle.StoredTool) (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions) {
	s, _, f, d := jobAssetFixture()
	*s.Actors[0].HeldToolID, *f.Assets.HeldToolID = held, held
	f.Assets.StoredTools = append([]corebattle.StoredTool{}, rows...)
	return s, f, d
}

func stored(instance, definition string) corebattle.StoredTool {
	return corebattle.StoredTool{InstanceID: instance, DefinitionID: definition, Category: item.UsageCategoryCombatPassive}
}

func rejectBook(t *testing.T, s corebattle.LifetimeState, f corebattle.CallbackContext, d BattleDefinitions, id string) {
	t.Helper()
	before, err := json.Marshal([]any{s, f.Assets})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.InvocationAction(&s, &f, []string{id}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("admitted", id, err)
	}
	if after, _ := json.Marshal([]any{s, f.Assets}); string(after) != string(before) {
		t.Fatal("admission mutated", string(after))
	}
}

func TestBookWithdrawalSwapsFirstMatchingBook(t *testing.T) {
	for _, held := range []string{"item-169", "item-092", "item-196"} {
		s, f, d := heldBookFixture(held, stored("tool", "item-001"), stored("first", "item-170"), stored("second", "item-170"))
		f.Arguments = []string{"item-170"}
		executeJobAsset(t, &s, &f, d, "job-86-entry-0")
		want := []corebattle.StoredToolOutcome{{InstanceID: "first", Result: corebattle.StoredToolExchanged}}
		if !reflect.DeepEqual(f.Assets.StoredOutcomes, want) || f.Assets.SelectionError || *s.Actors[0].HeldToolID != "item-170" || *f.Assets.HeldToolID != "item-170" {
			t.Fatal(held, f.Assets, s.Actors[0])
		}
		if s.Actors[0].HP != 500 || len(s.SkillBans) != 0 {
			t.Fatal("withdrawal had a release side effect", s)
		}
	}
}

func TestBookWithdrawalFailureAndIneligibleHands(t *testing.T) {
	for name, tc := range map[string]struct {
		held, field, arg string
		selection        bool
	}{
		"no match":            {"item-169", "", "item-171", true},
		"tool row same name":  {"item-169", "", "item-001", true},
		"no book":             {"item-001", "", "item-170", false},
		"library empty hand":  {"", "library", "item-170", false},
		"empty hand no field": {"", "", "item-170", false},
	} {
		for _, id := range []string{"job-86-entry-0", "job-86-entry-3"} {
			s, f, d := heldBookFixture(tc.held, stored("tool", "item-001"), stored("book", "item-170"))
			s.Fields.Temporary = tc.field
			f.Arguments = []string{tc.arg}
			executeJobAsset(t, &s, &f, d, id)
			if name == "library empty hand" && id == "job-86-entry-0" {
				if *f.Assets.HeldToolID != "item-170" || len(f.Assets.StoredOutcomes) != 1 {
					t.Fatal("library exception did not withdraw into the empty hand", f.Assets)
				}
				continue
			}
			if f.Assets.SelectionError != tc.selection || len(f.Assets.StoredOutcomes) != 0 || *f.Assets.HeldToolID != tc.held {
				t.Fatal(name, id, f.Assets)
			}
			if s.Actors[0].HP != 500 || len(s.SkillBans) != 0 || f.Assets.StoredTools == nil {
				t.Fatal(name, id, "failure released", s)
			}
		}
	}
}

func TestBookReleaseRecoilBanLifetimeAndReload(t *testing.T) {
	s, c, f, d := jobAssetFixture()
	*s.Actors[0].HeldToolID, *f.Assets.HeldToolID = "item-169", "item-169"
	f.Assets.StoredTools = []corebattle.StoredTool{stored("book", "item-224")}
	f.Arguments = []string{"item-224"}
	executeJobAsset(t, &s, &f, d, "job-86-entry-3")
	// _risk: int(1000×.15+1)=151 immediate HP loss.
	if !c.ResponseSuppressed || s.Actors[0].HP != 349 || s.SkillBans["job-86-entry-3"] != 2 || *s.Actors[0].HeldToolID != "item-224" {
		t.Fatal(c.ResponseSuppressed, s)
	}
	s.TickSkillBans() // The same action's tail.
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded corebattle.LifetimeState
	if err := json.Unmarshal(raw, &loaded); err != nil || loaded.SkillBans["job-86-entry-3"] != 1 || *loaded.Actors[0].HeldToolID != "item-224" {
		t.Fatal(loaded, err)
	}
	loaded.TickSkillBans() // An intervening qualifying action.
	if _, ok := loaded.SkillBans["job-86-entry-3"]; ok {
		t.Fatal("ban outlived the intervening tail", loaded.SkillBans)
	}
}

func TestBookReturnRecoversMPThenClearsBothHands(t *testing.T) {
	s, f, d := heldBookFixture("item-170")
	s.Actors[0].MaxMP = 200
	executeJobAsset(t, &s, &f, d, "job-86-entry-2", 0)
	// _mp_h: int(30×.9)+1=28.
	if s.Actors[0].MP != 108 || *s.Actors[0].HeldToolID != "" || *f.Assets.HeldToolID != "" {
		t.Fatal(s.Actors[0], f.Assets)
	}
	if !reflect.DeepEqual(f.Assets.StoredOutcomes, []corebattle.StoredToolOutcome{{Result: corebattle.BookReturned}}) {
		t.Fatal(f.Assets.StoredOutcomes)
	}
	s, f, d = heldBookFixture("item-001")
	executeJobAsset(t, &s, &f, d, "job-86-entry-2")
	if *f.Assets.HeldToolID != "item-001" || len(f.Assets.StoredOutcomes) != 0 {
		t.Fatal("non-book returned", f.Assets)
	}
}

func TestBookAssetsRejectUnrepresentedIdentities(t *testing.T) {
	for name, prepare := range map[string]func(*corebattle.LifetimeState, *corebattle.CallbackContext){
		"npc actor": func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) { s.Actors[0].NPC = true },
		"mismatched hands": func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) {
			held := "item-171"
			f.Assets.HeldToolID = &held
		},
		"unknown durable hand": func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) { f.Assets.HeldToolID = nil },
		"unknown combat hand":  func(s *corebattle.LifetimeState, _ *corebattle.CallbackContext) { s.Actors[0].HeldToolID = nil },
		"missing depot":        func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) { f.Assets.StoredTools = nil },
		"actor not requester":  func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) { f.Assets.RequesterID = "fallen" },
	} {
		for _, id := range []string{"job-86-entry-0", "job-86-entry-2", "job-86-entry-3", "job-86-entry-5"} {
			s, f, d := heldBookFixture("item-170", stored("book", "item-170"))
			f.Arguments = []string{"enemy", "enemy"}
			prepare(&s, &f)
			t.Run(name+"/"+id, func(t *testing.T) { rejectBook(t, s, f, d, id) })
		}
	}
	// Without a held book the NPC branch is an accepted no-effect.
	s, f, d := heldBookFixture("")
	s.Actors[0].NPC = true
	executeJobAsset(t, &s, &f, d, "job-86-entry-2")
}

// doubleChild is a synthetic projected child: 300 damage to its target and a
// 5-MP actor cost, with an optional hand change on "fallen".
func doubleChild(calls *[]string) corebattle.CallbackDefinition {
	apply := func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) {
		for i := range s.Actors {
			a := &s.Actors[i]
			if a.ID == f.Arguments[0] {
				a.HP = max(0, a.HP-300)
			}
			if a.ID == f.ActorID {
				a.MP -= 5
				if f.Arguments[0] == "fallen" {
					*a.HeldToolID, *f.Assets.HeldToolID = "", ""
				}
			}
		}
	}
	return corebattle.CallbackDefinition{
		Validate: func(*corebattle.LifetimeState, *corebattle.CallbackContext) error { return nil },
		Project: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) (bool, error) {
			apply(s, f)
			return true, nil
		},
		Execute: func(s *corebattle.LifetimeState, f *corebattle.CallbackContext, _ random.Generator) error {
			*calls = append(*calls, f.Arguments[0])
			apply(s, f)
			return nil
		},
	}
}

func TestBookDoubleUseCostDeathAndReturnOrder(t *testing.T) {
	for _, tc := range []struct {
		name, held       string
		mp, selfHP       int
		targets          []string
		calls            []string
		wantMP, wantSelf int
		returned         bool
	}{
		// book169 listed6: floor(9); MP8 skips both children and the return.
		{"insufficient", "item-169", 8, 500, []string{"enemy", "enemy"}, nil, 8, 500, false},
		// Charge uses current MP after children, without a local clamp.
		{"same target twice", "item-169", 9, 500, []string{"enemy", "enemy"}, []string{"enemy", "enemy"}, -10, 500, true},
		{"unlisted book uses position0", "item-092", 9, 500, []string{"enemy", "enemy"}, []string{"enemy", "enemy"}, -10, 500, true},
		{"first child death", "item-169", 20, 300, []string{"self", "enemy"}, []string{"self"}, 15, 0, false},
		{"second child death", "item-169", 20, 300, []string{"enemy", "self"}, []string{"enemy", "self"}, 1, 0, true},
		{"child emptied hand", "item-169", 20, 500, []string{"fallen", "enemy"}, []string{"fallen", "enemy"}, 1, 500, false},
		// book170 listed20: floor(30); odd book175 listed9: floor(13.5)=13.
		{"listed cost per book", "item-170", 29, 500, []string{"enemy", "enemy"}, nil, 29, 500, false},
		{"odd cost below", "item-175", 12, 500, []string{"enemy", "enemy"}, nil, 12, 500, false},
		{"odd cost truncated", "item-175", 13, 500, []string{"enemy", "enemy"}, []string{"enemy", "enemy"}, -10, 500, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, f, d := heldBookFixture(tc.held)
			s.Actors[0].MP, s.Actors[0].HP = tc.mp, tc.selfHP
			var calls []string
			d.Invocations.Entries["book-169"] = doubleChild(&calls)
			d.Invocations.Entries["book-170"] = doubleChild(&calls)
			d.Invocations.Entries["book-175"] = doubleChild(&calls)
			f.Arguments = tc.targets
			executeJobAsset(t, &s, &f, d, "job-86-entry-5")
			if !reflect.DeepEqual(calls, tc.calls) || s.Actors[0].MP != tc.wantMP || s.Actors[0].HP != tc.wantSelf {
				t.Fatal(calls, s.Actors[0])
			}
			if got := len(f.Assets.StoredOutcomes) == 1; got != tc.returned {
				t.Fatal("return", f.Assets)
			}
			kept := len(tc.calls) < 2
			if (*f.Assets.HeldToolID == tc.held) != kept || (*s.Actors[0].HeldToolID == tc.held) != kept {
				t.Fatal("hands", kept, *f.Assets.HeldToolID, *s.Actors[0].HeldToolID)
			}
		})
	}
}

func TestBookDoubleUseGatesBeforeMutation(t *testing.T) {
	for name, prepare := range map[string]func(*corebattle.CallbackContext, *BattleDefinitions){
		"one target":     func(f *corebattle.CallbackContext, _ *BattleDefinitions) { f.Arguments = []string{"enemy"} },
		"foreign target": func(f *corebattle.CallbackContext, _ *BattleDefinitions) { f.Arguments = []string{"enemy", "stale"} },
		// Concrete books have no projection yet, so the second use is unadmitted.
		"concrete child": func(_ *corebattle.CallbackContext, d *BattleDefinitions) {
			d.RegisterBooks(&corebattle.BookContext{})
		},
		"unregistered child": func(*corebattle.CallbackContext, *BattleDefinitions) {},
	} {
		s, f, d := heldBookFixture("item-169")
		f.Arguments = []string{"enemy", "enemy"}
		prepare(&f, &d)
		t.Run(name, func(t *testing.T) { rejectBook(t, s, f, d, "job-86-entry-5") })
	}
}
