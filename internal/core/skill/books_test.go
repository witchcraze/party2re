package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func bookFixture() (corebattle.LifetimeState, *corebattle.BookContext, BattleDefinitions) {
	s, base, _ := timeMagicFixture()
	s.Actors[0].ItemDefinitionIDs = nil
	c := &corebattle.BookContext{DamageContext: base.DamageContext}
	var d BattleDefinitions
	d.RegisterBooks(c)
	return s, c, d
}

func executeBook(t *testing.T, s *corebattle.LifetimeState, c *corebattle.BookContext, d BattleDefinitions, id string, args []string, draws ...float64) {
	t.Helper()
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: args}
	ops, err := d.InvocationAction(s, &f, []string{id}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(id, err)
	}
	rng := &damageGemDraws{t: t, draws: draws}
	if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
		t.Fatal(id, err, rng.used, len(draws))
	}
}

// Independent vectors: ff97fc5:_skill.cgi:479–735 bodies with _heal:9067–9135
// (int(v×0.9) at the lowest variance draw) and _st_up:9307–9354 (int(base×v)).
// Self starts at HP500/1000, defense80, agility40 and attack100 base stats.
// Party bodies visit self, then the fallen member, which _check_party:9638
// redirects to the caster, so self receives both applications.
func TestBookManifest(t *testing.T) {
	for _, row := range []struct {
		id, line                     string
		mp                           int
		library                      bool
		hp, defense, agility, attack int
		draws                        []float64
	}{
		{"book-169", "480–488", 6, false, 527, 80, 40, 100, []float64{0}},
		{"book-169", "480–488", 6, true, 540, 80, 40, 100, []float64{0}},
		{"book-170", "489–499", 20, false, 581, 80, 40, 100, []float64{0}},
		{"book-170", "489–499", 20, true, 621, 80, 40, 100, []float64{0}},
		{"book-171", "500–510", 50, false, 716, 80, 40, 100, []float64{0, 0}},
		{"book-171", "500–510", 50, true, 824, 80, 40, 100, []float64{0, 0}},
		{"book-172", "511–521", 36, false, 662, 80, 40, 100, []float64{0}},
		{"book-172", "511–521", 36, true, 743, 80, 40, 100, []float64{0}},
		{"book-175", "549–560", 9, false, 500, 112, 40, 100, nil},
		{"book-175", "549–560", 9, true, 500, 128, 40, 100, nil},
		{"book-176", "561–572", 21, false, 500, 120, 40, 100, nil},
		{"book-176", "561–572", 21, true, 500, 140, 40, 100, nil},
		{"book-177", "573–582", 9, false, 500, 80, 56, 100, nil},
		{"book-177", "573–582", 9, true, 500, 80, 64, 100, nil},
		{"book-178", "583–592", 18, false, 500, 80, 60, 100, nil},
		{"book-178", "583–592", 18, true, 500, 80, 70, 100, nil},
		{"book-179", "593–603", 48, false, 500, 80, 40, 200, nil},
		{"book-179", "593–603", 48, true, 500, 80, 40, 250, nil},
		// Book250 has no library multiplier: 999×0.9 caps at MaxHP either way.
		{"book-250", "726–735", 120, false, 1000, 80, 40, 100, []float64{0, 0}},
		{"book-250", "726–735", 120, true, 1000, 80, 40, 100, []float64{0, 0}},
	} {
		t.Run(row.id, func(t *testing.T) {
			s, c, d := bookFixture()
			if row.library {
				s.Fields.Create("library", 11)
			}
			s.Actors[0].ItemDefinitionIDs = []string{"item-" + row.id[5:]}
			s.Actors[0].MP = row.mp
			entry, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t})
			if err != nil || entry.MPCost != row.mp || entry.RequiredSP != 0 || entry.Extended {
				t.Fatal(row.line, entry, err)
			}
			s.Actors[0].MP = row.mp - 1
			if _, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal(row.line, "MP below listed cost", err)
			}
			executeBook(t, &s, c, d, row.id, []string{"self", "tail"}, row.draws...)
			a := s.Actors[0]
			if a.HP != row.hp || a.Defense != row.defense || a.Agility != row.agility || a.Attack != row.attack || a.MP != row.mp-1 {
				t.Fatal(row.line, a)
			}
			sealed := row.id == "book-250" && !row.library
			if (a.Status == corebattle.StatusMagicSeal) != sealed {
				t.Fatal(row.line, "seal", a.Status)
			}
			party := row.id == "book-171" || row.id == "book-176" || row.id == "book-178" || row.id == "book-250"
			want := []string{"tail"}
			if party {
				want = []string{"self", "tail"}
			}
			if !reflect.DeepEqual(c.Arguments, want) {
				t.Fatal(row.line, c.Arguments)
			}
		})
	}
}

func TestBookDiscoveryGates(t *testing.T) {
	var empty BattleDefinitions
	s, c, d := bookFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-169"}
	if _, err := empty.BookAction(&s, c, "book-169", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("unregistered", err)
	}
	var nilContext BattleDefinitions
	nilContext.RegisterBooks(nil)
	if _, err := nilContext.InvocationAction(&s, &corebattle.CallbackContext{TargetContext: c.TargetContext}, []string{"book-169"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal("nil context", err)
	}
	if len(d.Invocations.JobTables) != 0 || len(d.Invocations.Entries) != len(corebattle.BookEntries()) {
		t.Fatal(d.Invocations)
	}
	// Revival, copy and the double-use wrapper keep their owners.
	for _, id := range []string{"book-173", "book-196", "book-211", "job-86-entry-5"} {
		if _, err := d.BookAction(&s, c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
	}
	for name, mutate := range map[string]func(*corebattle.LifetimeState, *corebattle.BookContext){
		"other book": func(s *corebattle.LifetimeState, _ *corebattle.BookContext) {
			s.Actors[0].ItemDefinitionIDs = []string{"item-170"}
		},
		"no enemies":  func(_ *corebattle.LifetimeState, c *corebattle.BookContext) { c.EnemyIDs = nil },
		"npc actor":   func(s *corebattle.LifetimeState, _ *corebattle.BookContext) { s.Actors[0].NPC = true },
		"npc context": func(_ *corebattle.LifetimeState, c *corebattle.BookContext) { c.ActorNPC = true },
	} {
		s, c, d := bookFixture()
		s.Actors[0].ItemDefinitionIDs = []string{"item-169"}
		mutate(&s, c)
		if _, err := d.BookAction(&s, c, "book-169", noArithmeticDraws{t}); err == nil {
			t.Fatal(name)
		}
	}
	// Neither SP nor curse gates the listing (_skill.cgi:34,136–145,260–283).
	s, c, d = bookFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-169"}
	s.Actors[0].Jobs = corebattle.BattleJobs{}
	s.Actors[0].Abilities = []string{"cursed"}
	if _, err := d.BookAction(&s, c, "book-169", noArithmeticDraws{t}); err != nil {
		t.Fatal("sp/curse", err)
	}
}

// Only book172's name is on the item249 magician list (_battle.cgi:1804–1825).
func TestBookItem249Costs(t *testing.T) {
	for _, row := range []struct {
		id   string
		cost int
	}{{"book-169", 12}, {"book-171", 100}, {"book-172", 18}, {"book-179", 96}, {"book-250", 240}} {
		s, c, d := bookFixture()
		s.Actors[0].ItemDefinitionIDs = []string{"item-" + row.id[5:], "item-249"}
		s.Actors[0].MP = row.cost
		entry, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t})
		if err != nil || entry.MPCost != row.cost {
			t.Fatal(row, entry, err)
		}
		s.Actors[0].MP = row.cost - 1
		if _, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(row, err)
		}
	}
}

// Indirect invocation (job86's double use) runs the body without the ordinary
// held-book, item249 or listed MP gate; its wrapper owns floor(cost×1.5).
func TestBookIndirectBodyHasNoOrdinaryCost(t *testing.T) {
	s, c, d := bookFixture()
	s.Actors[0].MP = 0
	s.Actors[0].ItemDefinitionIDs = []string{"item-249"}
	executeBook(t, &s, c, d, "book-172", []string{"self", "self"}, 0)
	executeBook(t, &s, c, d, "book-172", c.Arguments, 0)
	executeBook(t, &s, c, d, "book-222", []string{"enemy"}, 0)
	if a := s.Actors[0]; a.HP != 824 || a.MP != 0 || s.Actors[1].HP != 882 || len(c.Arguments) != 0 {
		t.Fatal(a, s.Actors[1], c.Arguments)
	}
}

func TestBookFallenAndPartyOrder(t *testing.T) {
	s, c, d := bookFixture()
	// A fallen recipient is redirected to the caster (_check_party:9638).
	executeBook(t, &s, c, d, "book-169", []string{"fallen"}, 0)
	if s.Actors[0].HP != 527 || s.Actors[2].HP != 0 || c.HealCalls != 1 || len(c.Arguments) != 0 {
		t.Fatal("fallen single target", s.Actors, c)
	}
	// Living allies receive ordered applications: self takes the first draw.
	s.Actors[2].HP, s.Actors[2].MaxHP = 100, 1000
	s.Actors[2].BaseStats, s.Actors[2].Defense = s.Actors[0].BaseStats, 80
	executeBook(t, &s, c, d, "book-171", nil, 0, .5)
	executeBook(t, &s, c, d, "book-176", nil)
	if s.Actors[0].HP != 635 || s.Actors[2].HP != 226 || s.Actors[0].Defense != 100 || s.Actors[2].Defense != 100 || c.PartyHealCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	// A capped caster returns without a write; the later ally still rises.
	s.Actors[0].Defense = 200
	executeBook(t, &s, c, d, "book-176", nil)
	if s.Actors[0].Defense != 200 || s.Actors[2].Defense != 120 {
		t.Fatal(s.Actors)
	}
}

// Book250 seals the caster outside the library even when healing has no effect
// or the caster's magic seal blocks it (_skill.cgi:726–735).
func TestBook250SealsAfterNoEffectOrBlockedHeal(t *testing.T) {
	s, c, d := bookFixture()
	s.Actors[0].HP = 1000
	executeBook(t, &s, c, d, "book-250", nil, 0, 0)
	if a := s.Actors[0]; a.HP != 1000 || a.Status != corebattle.StatusMagicSeal {
		t.Fatal("no-effect heal", a)
	}
	s.Actors[0].HP = 500
	executeBook(t, &s, c, d, "book-250", nil, .9)
	if a := s.Actors[0]; a.HP != 500 || a.Status != corebattle.StatusMagicSeal {
		t.Fatal("blocked heal", a)
	}
	s.Fields.Create("library", 11)
	s.Actors[0].Status = ""
	executeBook(t, &s, c, d, "book-250", nil, 0, 0)
	if a := s.Actors[0]; a.HP != 1000 || a.Status != "" {
		t.Fatal("library", a)
	}
}

// The caster's item186 heals the recipient by int(int(80×0.4)×0.9) before the raise.
func TestBookStatItem186IncidentalHeal(t *testing.T) {
	s, c, d := bookFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-186"}
	executeBook(t, &s, c, d, "book-175", []string{"self"}, 0)
	if a := s.Actors[0]; a.Defense != 112 || a.HP != 528 {
		t.Fatal(a)
	}
}

// An unsupported item201 tail rejects the whole entry before any write or draw.
func TestBookUnsupportedDescendantRejectsBeforeMutation(t *testing.T) {
	s, c, d := bookFixture()
	s.Actors[0].ItemDefinitionIDs = []string{"item-177", "item-201"}
	c.Damage = nil
	before, _ := json.Marshal(s)
	if _, err := d.BookAction(&s, c, "book-177", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"self"}}
	if _, err := d.InvocationAction(&s, &f, []string{"book-178"}, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	if after, _ := json.Marshal(s); string(before) != string(after) {
		t.Fatal(string(after))
	}
}

func TestBookReloadFrozenInputWithoutReplay(t *testing.T) {
	s, c, d := bookFixture()
	s.Fields.Create("library", 11)
	executeBook(t, &s, c, d, "book-175", []string{"self"})
	type checkpoint struct {
		State     corebattle.LifetimeState
		Target    corebattle.TargetContext
		HealCalls int
		Entry     string
		Arguments []string
	}
	frozen := checkpoint{s, c.TargetContext, c.HealCalls, "book-170", []string{"self", "tail"}}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.BookContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, HealCalls: loaded.HealCalls}}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterBooks(resumed)
	executeBook(t, &s, c, d, loaded.Entry, loaded.Arguments, 0)
	executeBook(t, &loaded.State, resumed, rebuilt, loaded.Entry, loaded.Arguments, 0)
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[0].Defense != 128 || s.Actors[0].HP != 621 || resumed.HealCalls != 1 || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || loaded.Arguments[0] != "self" {
		t.Fatal(s, loaded.State, c, resumed)
	}
}
