package skill

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

const enhancedEntry = "job-78-entry-0"

type repeatedDraw float64

func (r repeatedDraw) Float64() float64 { return float64(r) }
func (r repeatedDraw) Intn(int) int     { panic("enhanced tools use continuous draws") }
func (r repeatedDraw) IntN(int) int     { panic("enhanced tools use continuous draws") }

// self 500/1000 HP, 80/1000 MP, AT/DF/AG base 100/50/40; a living enemy with
// AT200 and 100/100 MP; a fallen ally with 1000 MaxHP. Submitted args never bind.
func enhancedFixture(held string) (corebattle.LifetimeState, *corebattle.JobAssetContext, corebattle.CallbackContext, BattleDefinitions) {
	s, c, f, d := jobAssetFixture()
	self := &s.Actors[0]
	self.MaxMP, self.Hit = 1000, 50
	self.BaseStats.Attack, self.BaseStats.Defense, self.Defense = 100, 50, 50
	enemy := &s.Actors[1]
	enemy.Attack, enemy.BaseStats.Attack, enemy.MP, enemy.MaxMP = 200, 200, 100, 100
	s.Actors[2].MaxHP, s.Actors[2].MaxMP = 1000, 1000
	*self.HeldToolID = held
	*f.Assets.HeldToolID = held
	c.DefeatContext = &corebattle.DefeatContext{Rewards: &corebattle.DefeatRewards{
		Validate: func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
		Apply:    func(*corebattle.LifetimeState, corebattle.TargetContext, string) error { return nil },
	}}
	c.Roster = &corebattle.RosterContext{EnemyTeamID: "enemy", EnemyPool: []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "pool", Name: "pool", HP: 10, Gear: &corebattle.BattleGearStats{}}},
	}}
	return s, c, f, d
}

func runEnhanced(t *testing.T, s *corebattle.LifetimeState, f *corebattle.CallbackContext, d BattleDefinitions, draw float64) {
	t.Helper()
	ops, err := d.InvocationAction(s, f, []string{enhancedEntry}, noArithmeticDraws{t})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ApplyInvocations(f, d.Invocations, ops, repeatedDraw(draw)); err != nil {
		t.Fatal(err)
	}
}

// Independent vectors: ff97fc5:_skill.cgi:10196–10282 with _heal 9067–9108
// (int(v×.9) at draw0) and _mp_h 9150–9170 (int(v×.9)+1). An absent first shift
// draws the first ally; a fallen helper target falls back to self, except revival.
func TestEnhancedToolEffectLedger(t *testing.T) {
	tests := []struct {
		number int
		draw   float64
		check  func(s *corebattle.LifetimeState) bool
	}{
		{1, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].HP == 608 }},
		{2, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].HP == 770 }},
		{3, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].HP == 1000 }},
		{4, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].HP == 1000 && s.Actors[2].HP == 0 }},
		{5, 0, func(s *corebattle.LifetimeState) bool {
			return s.Actors[0].HP == 1000 && s.Actors[0].MP == 80+91+91 && s.Actors[2].MP == 0
		}},
		{6, .6, func(s *corebattle.LifetimeState) bool {
			return s.Actors[2].HP == 1000 && s.Actors[2].Temporary == corebattle.TemporaryRevival
		}},
		{9, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[2].HP == 500 }},
		{9, .5, func(s *corebattle.LifetimeState) bool { return s.Actors[2].HP == 0 }},
		{11, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].MP == 189 }},
		{12, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].MP == 351 }},
		{13, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].MP == 980 && s.Actors[0].Hit == 95 }},
		{14, 0, func(s *corebattle.LifetimeState) bool {
			return s.Actors[0].Temporary == corebattle.TemporaryStrongDefense
		}},
		{15, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[1].HP == 820 }},
		{18, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].Attack == 200 }},
		{19, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].Defense == 100 }},
		{20, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].Agility == 80 }},
		{41, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[1].HP == 640 }},
		{42, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[1].HP == 0 }},
		{42, .4, func(s *corebattle.LifetimeState) bool { return s.Actors[1].HP == 0 }}, // par>=rand(100) is inclusive.
		{42, .41, func(s *corebattle.LifetimeState) bool { return s.Actors[1].HP == 900 }},
		{78, 0, func(s *corebattle.LifetimeState) bool { return len(s.Actors) == 5 }},
		{78, .5, func(s *corebattle.LifetimeState) bool { return len(s.Actors) == 4 }},
		{80, 0, func(s *corebattle.LifetimeState) bool {
			return s.Actors[0].Temporary == corebattle.TemporaryDefense && s.Actors[2].Temporary == corebattle.TemporaryDefense
		}},
		{81, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[1].Temporary == "" }},
		{84, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[1].MP == 75 && s.Actors[0].MP == 80+22+1 }},
		{87, 0, func(s *corebattle.LifetimeState) bool { return s.Actors[0].Hit == 95 }},
		{87, .9, func(s *corebattle.LifetimeState) bool { return s.Actors[2].Hit == 95 && s.Actors[0].Hit == 50 }},
	}
	seen := map[int]bool{}
	for _, tc := range tests {
		seen[tc.number] = true
		t.Run(fmt.Sprintf("%d/%v", tc.number, tc.draw), func(t *testing.T) {
			held := fmt.Sprintf("item-%03d", tc.number)
			s, _, f, d := enhancedFixture(held)
			if tc.number == 81 {
				s.Actors[1].Temporary = corebattle.TemporaryDefense
			}
			if tc.number == 42 {
				s.Actors[1].HP = 900 // HP above 999 is immune to instant death.
			}
			runEnhanced(t, &s, &f, d, tc.draw)
			if !tc.check(&s) {
				t.Fatalf("effect: %+v", s.Actors)
			}
			if *s.Actors[0].HeldToolID != "" || *f.Assets.HeldToolID != "" {
				t.Fatal("tool retained", *s.Actors[0].HeldToolID, *f.Assets.HeldToolID)
			}
			if f.Assets.Wallet != 100 || !reflect.DeepEqual(f.Arguments, []string{"enemy", "tail"}) {
				t.Fatal("use changed wallet or outer arguments", f.Assets.Wallet, f.Arguments)
			}
		})
	}
	if len(seen) != 22 {
		t.Fatal("ledger covers", len(seen))
	}
}

func TestEnhancedToolConsumptionFollowsTheReturnValue(t *testing.T) {
	retained := func(t *testing.T, s corebattle.LifetimeState, f corebattle.CallbackContext, held string) {
		t.Helper()
		if *s.Actors[0].HeldToolID != held || *f.Assets.HeldToolID != held {
			t.Fatal(*s.Actors[0].HeldToolID, *f.Assets.HeldToolID)
		}
	}
	t.Run("item6 living target returns early", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-006")
		runEnhanced(t, &s, &f, d, 0)
		retained(t, s, f, "item-006")
		if s.Actors[0].Temporary != "" {
			t.Fatal(s.Actors[0])
		}
	})
	t.Run("unsupported ID is an accepted no-effect", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-007")
		runEnhanced(t, &s, &f, d, 0)
		retained(t, s, f, "item-007")
	})
	// Other bodies return1 whatever their helpers did.
	for name, setup := range map[string]func(*corebattle.LifetimeState, *corebattle.CallbackContext){
		"item15 one-hit defense": func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) {
			*s.Actors[0].HeldToolID, *f.Assets.HeldToolID = "item-015", "item-015"
			s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
		},
		"item81 fallen enemy": func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) {
			*s.Actors[0].HeldToolID, *f.Assets.HeldToolID = "item-081", "item-081"
			s.Actors[1].HP = 0
		},
		"item84 stat floor": func(s *corebattle.LifetimeState, f *corebattle.CallbackContext) {
			*s.Actors[0].HeldToolID, *f.Assets.HeldToolID = "item-084", "item-084"
			s.Actors[1].MP = 20
		},
	} {
		t.Run(name, func(t *testing.T) {
			s, _, f, d := enhancedFixture("")
			setup(&s, &f)
			runEnhanced(t, &s, &f, d, 0)
			retained(t, s, f, "")
			if s.Actors[1].HP > 0 && (s.Actors[1].HP != 1000 || s.Actors[0].MP != 80) {
				t.Fatal("no-effect helper applied", s.Actors)
			}
		})
	}
	t.Run("item41 without enemies", func(t *testing.T) {
		s, c, f, d := enhancedFixture("item-041")
		s.Actors = s.Actors[:1:1]
		s.Actors = append(s.Actors, corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "fallen", TeamID: "party", Gear: &corebattle.BattleGearStats{}}})
		c.EnemyIDs, f.EnemyIDs = nil, nil
		runEnhanced(t, &s, &f, d, 0)
		retained(t, s, f, "")
	})
}

// The first shift reads the numeric ID, never the submitted target; only a
// participant actually named that number binds it (_skill.cgi:10196–10282).
func TestEnhancedToolNumericArgumentBinding(t *testing.T) {
	t.Run("item6 numeric-named fallen ally needs no draw", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-006")
		s.Actors[2].Name = "6"
		runEnhanced(t, &s, &f, d, 0)
		if s.Actors[2].HP != 1000 || *s.Actors[0].HeldToolID != "" {
			t.Fatal(s.Actors)
		}
	})
	t.Run("item1 numeric-named enemy falls back to a random ally", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-001")
		s.Actors[1].Name = "1"
		runEnhanced(t, &s, &f, d, 0)
		if s.Actors[1].HP != 1000 || s.Actors[0].HP != 608 {
			t.Fatal(s.Actors)
		}
	})
	t.Run("item84 checks the first argument but steals through fallback", func(t *testing.T) {
		s, c, f, d := enhancedFixture("item-084")
		second := s.Actors[1].Clone()
		second.ID, second.Name = "enemy-84", "84"
		s.Actors = append(s.Actors, second)
		c.EnemyIDs = []string{"enemy", "enemy-84"}
		f.EnemyIDs = []string{"enemy", "enemy-84"}
		runEnhanced(t, &s, &f, d, 0)
		if s.Actors[1].MP != 75 || s.Actors[3].MP != 100 {
			t.Fatal("theft bound the checked target", s.Actors[1].MP, s.Actors[3].MP)
		}
	})
	t.Run("item87 retains numeric-named member or draws all members", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-087")
		s.Actors[1].Name, s.Actors[1].Hit = "87", 50
		runEnhanced(t, &s, &f, d, 0)
		if s.Actors[1].Hit != 95 || s.Actors[0].Hit != 50 {
			t.Fatal(s.Actors)
		}
	})
}

func TestEnhancedPurchaseWeightedPoolAndCost(t *testing.T) {
	pool := []struct {
		id   string
		cost int
	}{
		{"item-001", 75}, {"item-001", 75}, {"item-001", 75}, {"item-001", 75}, {"item-001", 75},
		{"item-002", 250}, {"item-009", 50}, {"item-011", 1250}, {"item-014", 250}, {"item-014", 250},
		{"item-015", 750}, {"item-015", 750}, {"item-041", 375}, {"item-080", 1250}, {"item-080", 1250},
		{"item-081", 3000}, {"item-084", 1750}, {"item-084", 1750},
	}
	for position, want := range pool {
		draw := (float64(position) + .5) / 18
		for _, wallet := range []int{want.cost, want.cost - 1} {
			s, _, f, d := enhancedFixture("")
			f.Assets.Wallet = wallet
			runEnhanced(t, &s, &f, d, draw)
			if wallet == want.cost {
				if *s.Actors[0].HeldToolID != want.id || *f.Assets.HeldToolID != want.id || f.Assets.Wallet != 0 {
					t.Fatal(position, *s.Actors[0].HeldToolID, f.Assets.Wallet)
				}
			} else if *s.Actors[0].HeldToolID != "" || *f.Assets.HeldToolID != "" || f.Assets.Wallet != wallet {
				t.Fatal("underfunded purchase", position, *s.Actors[0].HeldToolID, f.Assets.Wallet)
			}
		}
	}
}

func TestEnhancedPurchaseExclusion(t *testing.T) {
	for _, tc := range []struct {
		name    string
		helpers []int
		draw    float64
		want    string
	}{
		{"exact ID leaves item1 although 11 contains it", []int{11}, 0, "item-001"},
		{"exact ID leaves item1 although 41 contains it", []int{41}, 0, "item-001"},
		{"item1 excluded shifts the draw", []int{1}, 0, "item-002"},
		{"item1 excluded retains later multiplicity", []int{1}, .999, "item-084"},
		{"entire pool", []int{1, 2, 9, 11, 14, 15, 41, 80, 81, 84}, 0, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, f, d := enhancedFixture("")
			f.Assets.Wallet = 5000
			f.Assets.HelperToolIDs = tc.helpers
			runEnhanced(t, &s, &f, d, tc.draw)
			if *s.Actors[0].HeldToolID != tc.want {
				t.Fatal(*s.Actors[0].HeldToolID)
			}
			if tc.want == "" && f.Assets.Wallet != 5000 {
				t.Fatal("empty pool charged", f.Assets.Wallet)
			}
		})
	}
}

// An NPC holds the tool in combat only; its requester still pays.
func TestEnhancedToolNPCActorKeepsDurableHand(t *testing.T) {
	npcFixture := func(held string) (corebattle.LifetimeState, corebattle.CallbackContext, BattleDefinitions) {
		s, c, f, d := enhancedFixture("")
		npcHeld := held
		s.Actors[1].NPC, s.Actors[1].HeldToolID = true, &npcHeld
		s.Actors[1].Hit = 95
		party, enemies := []string{"enemy"}, []string{"self"}
		members := []string{"fallen"}
		c.ActorID, c.PartyIDs, c.EnemyIDs, c.MemberOnlyIDs = "enemy", party, enemies, members
		f.ActorID, f.PartyIDs, f.EnemyIDs, f.MemberOnlyIDs = "enemy", party, enemies, members
		return s, f, d
	}
	t.Run("purchase", func(t *testing.T) {
		s, f, d := npcFixture("")
		runEnhanced(t, &s, &f, d, 0)
		if *s.Actors[1].HeldToolID != "item-001" || *f.Assets.HeldToolID != "" || f.Assets.Wallet != 25 {
			t.Fatal(*s.Actors[1].HeldToolID, *f.Assets.HeldToolID, f.Assets.Wallet)
		}
	})
	t.Run("unknown durable hand is unreachable", func(t *testing.T) {
		s, f, d := npcFixture("")
		f.Assets.HeldToolID = nil
		runEnhanced(t, &s, &f, d, 0)
		if *s.Actors[1].HeldToolID != "item-001" || f.Assets.Wallet != 25 {
			t.Fatal(*s.Actors[1].HeldToolID, f.Assets.Wallet)
		}
	})
	t.Run("use", func(t *testing.T) {
		s, f, d := npcFixture("item-080")
		*f.Assets.HeldToolID = "item-001"
		runEnhanced(t, &s, &f, d, 0)
		if *s.Actors[1].HeldToolID != "" || *f.Assets.HeldToolID != "item-001" || f.Assets.Wallet != 100 {
			t.Fatal(*s.Actors[1].HeldToolID, *f.Assets.HeldToolID, f.Assets.Wallet)
		}
	})
}

// Each case misses only the fact its reachable branch needs; the contrast cases
// prove the same omission does not reject a branch that cannot reach it.
func TestEnhancedToolAdmissionIsInertAndBranchScoped(t *testing.T) {
	for _, tc := range []struct {
		name   string
		held   string
		mutate func(*corebattle.LifetimeState, *corebattle.JobAssetContext, *corebattle.CallbackContext)
		reject bool
	}{
		{"unknown combat hand", "item-001", func(s *corebattle.LifetimeState, _ *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			s.Actors[0].HeldToolID = nil
		}, true},
		{"unknown durable hand", "", func(_ *corebattle.LifetimeState, _ *corebattle.JobAssetContext, f *corebattle.CallbackContext) {
			f.Assets.HeldToolID = nil
		}, true},
		{"hands disagree", "item-001", func(_ *corebattle.LifetimeState, _ *corebattle.JobAssetContext, f *corebattle.CallbackContext) {
			*f.Assets.HeldToolID = "item-002"
		}, true},
		{"missing wallet context", "", func(_ *corebattle.LifetimeState, _ *corebattle.JobAssetContext, f *corebattle.CallbackContext) {
			f.Assets = nil
		}, true},
		{"purchase needs helper facts", "", func(_ *corebattle.LifetimeState, _ *corebattle.JobAssetContext, f *corebattle.CallbackContext) {
			f.Assets.HelperToolIDs = nil
		}, true},
		{"use does not need helper facts", "item-001", func(_ *corebattle.LifetimeState, _ *corebattle.JobAssetContext, f *corebattle.CallbackContext) {
			f.Assets.HelperToolIDs = nil
		}, false},
		{"item13 needs reset owner", "item-013", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.DefeatContext = nil
		}, true},
		{"item87 needs reset owner", "item-087", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.DefeatContext = nil
		}, true},
		{"item42 needs defeat owner", "item-042", func(s *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			s.Actors[1].HP, c.DefeatContext = 900, nil
		}, true},
		{"item42 immune enemy cannot reach defeat", "item-042", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.DefeatContext = nil
		}, false},
		{"item1 does not need reset owner", "item-001", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.DefeatContext = nil
		}, false},
		{"item78 needs roster", "item-078", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.Roster = nil
		}, true},
		{"item14 does not need roster", "item-014", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.Roster = nil
		}, false},
		{"unsupported ID needs no owner", "item-007", func(_ *corebattle.LifetimeState, c *corebattle.JobAssetContext, _ *corebattle.CallbackContext) {
			c.DefeatContext, c.Roster = nil, nil
		}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c, f, d := enhancedFixture(tc.held)
			tc.mutate(&s, c, &f)
			before, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			_, err = d.InvocationAction(&s, &f, []string{enhancedEntry}, noArithmeticDraws{t})
			if tc.reject != errors.Is(err, corebattle.ErrUnsupportedEffect) || (!tc.reject && err != nil) {
				t.Fatal(err)
			}
			after, err := json.Marshal(s)
			if err != nil || string(before) != string(after) {
				t.Fatal("admission mutated state", err)
			}
		})
	}
}

// Scavenging reads the durable hand only for a player actor, and helper facts
// only inside the empty-hand draw (_skill.cgi:6176–6196).
func TestJobAssetScavengingAdmitsWithoutUnreachableFacts(t *testing.T) {
	t.Run("NPC actor with unknown durable hand", func(t *testing.T) {
		s, c, f, d := enhancedFixture("")
		held := ""
		s.Actors[1].NPC, s.Actors[1].HeldToolID, s.Actors[1].Hit = true, &held, 95
		s.Actors[1].BaseStats.Agility, s.Actors[1].Agility = 40, 40
		party, enemies, members := []string{"enemy"}, []string{"self"}, []string{"fallen"}
		c.ActorID, c.PartyIDs, c.EnemyIDs, c.MemberOnlyIDs = "enemy", party, enemies, members
		f.ActorID, f.PartyIDs, f.EnemyIDs, f.MemberOnlyIDs = "enemy", party, enemies, members
		f.Arguments = []string{"self"}
		f.Assets.HeldToolID = nil
		executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0, 0, 0)
		if held != "item-001" || f.Assets.HeldToolID != nil {
			t.Fatal(held, f.Assets.HeldToolID)
		}
	})
	t.Run("held tool without helper facts", func(t *testing.T) {
		s, _, f, d := enhancedFixture("item-015")
		f.Assets.HelperToolIDs = nil
		executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0)
		if *s.Actors[0].HeldToolID != "item-015" {
			t.Fatal(*s.Actors[0].HeldToolID)
		}
	})
}

// Purchase and use across a save/reload must match one uninterrupted operation.
func TestEnhancedToolReloadMatchesUninterrupted(t *testing.T) {
	run := func(t *testing.T, reload bool) corebattle.LifetimeState {
		s, _, f, d := enhancedFixture("")
		runEnhanced(t, &s, &f, d, 0)
		if *f.Assets.HeldToolID != "item-001" {
			t.Fatal(*f.Assets.HeldToolID)
		}
		if reload {
			data, err := json.Marshal(s)
			if err != nil {
				t.Fatal(err)
			}
			s = corebattle.LifetimeState{}
			if err := json.Unmarshal(data, &s); err != nil {
				t.Fatal(err)
			}
		}
		runEnhanced(t, &s, &f, d, 0)
		if f.Assets.Wallet != 25 || *f.Assets.HeldToolID != "" {
			t.Fatal(f.Assets.Wallet, *f.Assets.HeldToolID)
		}
		return s
	}
	straight, reloaded := run(t, false), run(t, true)
	a, err := json.Marshal(straight)
	if err != nil {
		t.Fatal(err)
	}
	b, err := json.Marshal(reloaded)
	if err != nil {
		t.Fatal(err)
	}
	if string(a) != string(b) || straight.Actors[0].HP != 608 {
		t.Fatalf("reload diverged:\n%s\n%s", a, b)
	}
}
