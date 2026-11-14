package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Independent vectors: ff97fc5:_skill.cgi:537–548,604–686 bodies. Direct magic
// damage is int(base×0.9) at the lowest _damage:8890 draw; only the damage
// bases take the library ×1.5. Self starts at HP500/1000, the enemy at HP900.
func TestBookOffenseCureManifest(t *testing.T) {
	for _, row := range []struct {
		id, line   string
		mp         int
		library    bool
		status     string // caster status before the body
		damage     int
		enemyState string
		draws      []float64
		party      bool // argument left unconsumed
	}{
		{id: "book-174", line: "537–548", mp: 15},
		{id: "book-174", line: "537–548", mp: 15, library: true},
		{id: "book-218", line: "604", mp: 8, status: corebattle.StatusVirulentPoison},
		{id: "book-219", line: "605", mp: 15, status: corebattle.StatusParalyze, party: true},
		{id: "book-220", line: "606", mp: 12, status: corebattle.StatusSleep, party: true},
		{id: "book-221", line: "607–617", mp: 35, status: corebattle.StatusParalyze},
		{id: "book-221", line: "607–617", mp: 35, library: true, status: corebattle.StatusPhysicalSeal},
		{id: "book-222", line: "618–628", mp: 6, damage: 18, draws: []float64{0}},
		{id: "book-222", line: "618–628", mp: 6, library: true, damage: 27, draws: []float64{0}},
		{id: "book-223", line: "629–639", mp: 17, damage: 63, draws: []float64{0}},
		{id: "book-223", line: "629–639", mp: 17, library: true, damage: 94, draws: []float64{0}},
		{id: "book-224", line: "640–650", mp: 46, damage: 198, draws: []float64{0}},
		{id: "book-224", line: "640–650", mp: 46, library: true, damage: 297, draws: []float64{0}},
		{id: "book-225", line: "651–661", mp: 11, damage: 22, draws: []float64{0}, party: true},
		{id: "book-225", line: "651–661", mp: 11, library: true, damage: 33, draws: []float64{0}, party: true},
		{id: "book-226", line: "662–672", mp: 24, damage: 54, draws: []float64{0}, party: true},
		{id: "book-226", line: "662–672", mp: 24, library: true, damage: 81, draws: []float64{0}, party: true},
		{id: "book-227", line: "673–683", mp: 40, damage: 112, draws: []float64{0}, party: true},
		{id: "book-227", line: "673–683", mp: 40, library: true, damage: 168, draws: []float64{0}, party: true},
		// rand(100) < chance at _st_d:9402; the library does not raise it.
		{id: "book-228", line: "684", mp: 19, enemyState: corebattle.StatusSleep, draws: []float64{.6499}},
		{id: "book-228", line: "684", mp: 19, library: true, draws: []float64{.65}},
		{id: "book-229", line: "685", mp: 24, enemyState: corebattle.StatusConfusion, draws: []float64{.4999}, party: true},
		{id: "book-229", line: "685", mp: 24, library: true, draws: []float64{.5}, party: true},
		{id: "book-230", line: "686", mp: 24, enemyState: corebattle.StatusMagicSeal, draws: []float64{.6999}, party: true},
		{id: "book-230", line: "686", mp: 24, library: true, draws: []float64{.7}, party: true},
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
			s.Actors[0].Status = row.status
			target := "self"
			if row.damage > 0 || row.draws != nil {
				target = "enemy"
			}
			executeBook(t, &s, c, d, row.id, []string{target, "tail"}, row.draws...)
			a, e := s.Actors[0], s.Actors[1]
			if 900-e.HP != row.damage || e.Status != row.enemyState || a.MP != row.mp-1 || a.HP != 500 {
				t.Fatal(row.line, a, e)
			}
			if a.Status != "" && row.status != "" {
				t.Fatal(row.line, "not cured", a.Status)
			}
			if (a.Temporary == corebattle.TemporaryReflectMagic) != (row.id == "book-174") {
				t.Fatal(row.line, "reflection", a.Temporary)
			}
			want := []string{"tail"}
			if row.party {
				want = []string{target, "tail"}
			}
			if !reflect.DeepEqual(c.Arguments, want) {
				t.Fatal(row.line, c.Arguments)
			}
		})
	}
}

// Of these names only the cures are absent from the item249 magician list
// (_battle.cgi:1804–1825): they double, the rest halve with int().
func TestBookOffenseItem249Costs(t *testing.T) {
	for _, row := range []struct {
		id   string
		cost int
	}{{"book-174", 7}, {"book-218", 16}, {"book-219", 30}, {"book-221", 70}, {"book-222", 3}, {"book-223", 8}, {"book-227", 20}, {"book-228", 9}, {"book-230", 12}} {
		s, c, d := bookFixture()
		s.Actors[0].ItemDefinitionIDs = []string{"item-" + row.id[5:], "item-249"}
		s.Actors[0].MP = row.cost
		if entry, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t}); err != nil || entry.MPCost != row.cost {
			t.Fatal(row, entry, err)
		}
		s.Actors[0].MP = row.cost - 1
		if _, err := d.BookAction(&s, c, row.id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(row, err)
		}
	}
}

// The deadly-poison cure clears every poison variant (_st_h:9446–9448); the
// party cures redirect the fallen member to the caster (_check_party:9638),
// and a mismatched status is an accepted no-effect.
func TestBookCureVariantsAndNoEffect(t *testing.T) {
	for _, status := range []string{corebattle.StatusPoison, corebattle.StatusVirulentPoison, corebattle.StatusDeadlyPoison} {
		s, c, d := bookFixture()
		s.Actors[0].Status = status
		executeBook(t, &s, c, d, "book-218", []string{"self"})
		if s.Actors[0].Status != "" {
			t.Fatal(status, s.Actors[0], c)
		}
	}
	s, c, d := bookFixture()
	s.Actors[0].Status = corebattle.StatusSleep
	executeBook(t, &s, c, d, "book-219", nil)
	executeBook(t, &s, c, d, "book-218", []string{"self"})
	if s.Actors[0].Status != corebattle.StatusSleep || !c.AdditionalEffect {
		t.Fatal(s.Actors[0], c)
	}
	executeBook(t, &s, c, d, "book-220", nil)
	if s.Actors[0].Status != "" {
		t.Fatal(s.Actors[0], c)
	}
}

// A sealed caster's magic is blocked before damage, status or cure; book221's
// untyped resolution passes, then the magic cure is blocked (_check_party:9618).
func TestBookOffenseMagicSeal(t *testing.T) {
	for _, row := range []struct {
		id    string
		draws []float64
	}{{"book-222", []float64{.9}}, {"book-225", []float64{.9}}, {"book-229", []float64{.9}}, {"book-219", []float64{.9}}, {"book-221", []float64{.9}}} {
		s, c, d := bookFixture()
		s.Actors[0].Status = corebattle.StatusMagicSeal
		target := "enemy"
		if row.id == "book-221" {
			target = "self"
		}
		executeBook(t, &s, c, d, row.id, []string{target}, row.draws...)
		if s.Actors[0].Status != corebattle.StatusMagicSeal || s.Actors[1].HP != 900 || s.Actors[1].Status != "" || c.DamageCalls != 0 {
			t.Fatal(row.id, s.Actors, c)
		}
	}
}

// Area bodies visit enemies in order: damage decays by 0.85 per target
// (_damages:8822) and each status target draws independently.
func TestBookAreaOrderAndImmunity(t *testing.T) {
	s, c, d := bookFixture()
	other := s.Actors[1]
	other.ID = "other"
	s.Actors = append(s.Actors, other)
	c.EnemyIDs = append(c.EnemyIDs, other.ID)
	executeBook(t, &s, c, d, "book-225", nil, 0, 0)
	executeBook(t, &s, c, d, "book-229", nil, .4, .6)
	if s.Actors[1].HP != 878 || s.Actors[3].HP != 881 || s.Actors[1].Status != corebattle.StatusConfusion || s.Actors[3].Status != "" || c.DamageCalls != 2 || c.AreaDamageCalls != 1 {
		t.Fatal(s.Actors, c)
	}
	// Above 999 HP the target is immune without a probability draw (_st_d:9396).
	s, c, d = bookFixture()
	s.Actors[1].HP = 1000
	executeBook(t, &s, c, d, "book-228", []string{"enemy"})
	if s.Actors[1].Status != "" {
		t.Fatal(s.Actors[1], c)
	}
}

// A possibly lethal hit without an admitted defeat descendant rejects the whole
// entry before cost, RNG or any write.
func TestBookOffenseUnsupportedDefeatRejectsBeforeMutation(t *testing.T) {
	for _, id := range []string{"book-222", "book-227"} {
		s, c, d := bookFixture()
		s.Actors[0].ItemDefinitionIDs = []string{"item-" + id[5:]}
		s.Actors[1].HP = 10
		c.Defeat = nil
		before, _ := json.Marshal(s)
		if _, err := d.BookAction(&s, c, id, noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(id, err)
		}
		if after, _ := json.Marshal(s); string(before) != string(after) || c.DamageCalls != 0 {
			t.Fatal(id, string(after))
		}
	}
}

func TestBookOffenseReloadWithoutReplay(t *testing.T) {
	s, c, d := bookFixture()
	executeBook(t, &s, c, d, "book-228", []string{"enemy"}, .9)
	type checkpoint struct {
		State                        corebattle.LifetimeState
		Target                       corebattle.TargetContext
		DamageCalls, AreaDamageCalls int
		Entry                        string
		Arguments                    []string
	}
	frozen := checkpoint{s, c.TargetContext, c.DamageCalls, c.AreaDamageCalls, "book-230", []string{"enemy"}}
	data, err := json.Marshal(frozen)
	if err != nil {
		t.Fatal(err)
	}
	var loaded checkpoint
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	resumed := &corebattle.BookContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: loaded.Target, DamageCalls: loaded.DamageCalls, AreaDamageCalls: loaded.AreaDamageCalls}, GuaranteedHit: true, Defeat: c.Defeat}}
	var rebuilt BattleDefinitions
	rebuilt.RegisterBooks(resumed)
	executeBook(t, &s, c, d, loaded.Entry, loaded.Arguments, 0)
	executeBook(t, &loaded.State, resumed, rebuilt, loaded.Entry, loaded.Arguments, 0)
	if !reflect.DeepEqual(s, loaded.State) || s.Actors[1].Status != corebattle.StatusMagicSeal || resumed.DamageCalls != c.DamageCalls || resumed.AreaDamageCalls != c.AreaDamageCalls || !reflect.DeepEqual(c.Arguments, resumed.Arguments) {
		t.Fatal(s, loaded.State, c, resumed)
	}
}
