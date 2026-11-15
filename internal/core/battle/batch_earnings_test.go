package battle

import (
	"reflect"
	"testing"
)

func TestPartyBattle_DefeatEarningsCaptureLivingRecipients(t *testing.T) {
	a := MustNewParticipant("a", 10000, 100, 0)
	a.Agility = 100
	b := MustNewParticipant("b", 5, 0, 0)
	b.Agility = 50
	e1 := MustNewParticipant("e1", 10, 1, 0)
	e1.ExperienceReward, e1.GoldReward = 7, 11
	e2 := MustNewParticipant("e2", 20, 100, 0)
	e2.Agility = 75
	e2.ExperienceReward, e2.GoldReward = 13, 19
	r, err := (Engine{}).ResolvePartyBattle(PartyBattleRequest{
		Allies: []Participant{a, b}, Enemies: []Participant{e1, e2},
		VictoryReward: Reward{Experience: 20, Currency: 30}, RNG: fixedFloatRNG{val: 0},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []DefeatEarnings{
		{Reward: Reward{Experience: 7, Currency: 11, Crystals: 1}, RecipientIDs: []string{"a", "b"}},
		{Reward: Reward{Experience: 13, Currency: 19, Crystals: 1}, RecipientIDs: []string{"a"}},
	}
	if r.Outcome != OutcomeWin || r.RemainingHP["b"] != 0 || !reflect.DeepEqual(r.DefeatEarnings, want) {
		t.Fatalf("outcome=%s HP=%v earnings=%+v, want %+v", r.Outcome, r.RemainingHP, r.DefeatEarnings, want)
	}
	if r.TotalReward != (Reward{Experience: 22, Currency: 33, Crystals: 2}) {
		t.Fatal("aggregate reward changed", r.TotalReward)
	}
}

func TestDefeatEarningsResumeAfterRevival(t *testing.T) {
	a, b := MustNewParticipant("a", 100, 100, 0), MustNewParticipant("b", 100, 0, 0)
	e := MustNewParticipant("e", 10, 0, 0)
	e.ExperienceReward, e.GoldReward = 7, 11
	c := &battleContext{
		req: PartyBattleRequest{Allies: []Participant{a, b}}, rng: fixedFloatRNG{val: 0},
		field: &FieldState{}, hpMap: map[string]int{"a": 100, "b": 100, "e": 10},
		mpMap:      map[string]int{},
		teamMap:    map[string]string{"a": "allies", "b": "allies", "e": "enemies"},
		allyTeamID: "allies", allParticipants: []Participant{a, b, e},
		crystalDroppedMap: map[string]bool{},
	}
	for i, hp := range []int{100, 0, 50} {
		e.ID = []string{"e1", "e2", "e3"}[i]
		c.teamMap[e.ID] = "enemies"
		c.hpMap["b"], c.hpMap[e.ID] = hp, 10
		c.applyDamage(a, e, 100, "", "attack", false, "", false)
		want := []string{"a", "b"}
		if i == 1 {
			want = []string{"a"}
		}
		if len(c.defeatEarnings) != i+1 || !reflect.DeepEqual(c.defeatEarnings[i].RecipientIDs, want) {
			t.Fatalf("step %d: %+v, want recipients %v", i, c.defeatEarnings, want)
		}
	}
	for _, e := range c.defeatEarnings {
		if e.Reward.Crystals != 1 {
			t.Fatal("missing crystal earnings", c.defeatEarnings)
		}
	}
}

func TestPartyBattle_PreventedDefeatDoesNotEarn(t *testing.T) {
	a := MustNewParticipant("a", 100, 100, 0)
	a.Agility = 100
	e := MustNewParticipant("e", 10, 0, 0)
	e.Abilities = []string{"revive"}
	e.ExperienceReward, e.GoldReward = 7, 11
	r, err := (Engine{}).ResolvePartyBattle(PartyBattleRequest{Allies: []Participant{a}, Enemies: []Participant{e}, RNG: fixedFloatRNG{val: 0}})
	if err != nil {
		t.Fatal(err)
	}
	if r.Outcome != OutcomeWin || r.Turns != 2 || len(r.DefeatEarnings) != 1 || r.DefeatEarnings[0].Reward != (Reward{Experience: 7, Currency: 11, Crystals: 1}) {
		t.Fatalf("prevented defeat earned rewards: turns=%d earnings=%+v", r.Turns, r.DefeatEarnings)
	}
}

func TestPartyBattle_SequentialAttackGemsEarnOnlyOnDefeat(t *testing.T) {
	for _, tc := range []struct {
		name, scope string
		powers      []int
		revive      bool
	}{
		{"two area gems", TargetScopeAllEnemies, []int{100, 100}, false},
		{"nonlethal then lethal area gems", TargetScopeAllEnemies, []int{10, 100}, false},
		{"two single-target gems", TargetScopeSingleEnemy, []int{100, 100}, false},
		{"revival then defeat then dead-target hit", TargetScopeAllEnemies, []int{100, 100, 100}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := MustNewParticipant("a", 100, 0, 0)
			a.Agility, a.CMP = 100, 5
			cs := ActionCustomSkill{ID: "sequential-attacks", Name: "連続攻撃", CMPCost: 5}
			for _, power := range tc.powers {
				cs.Gems = append(cs.Gems, GemEffect{Kind: ActionKindAttack, Power: power, TargetScope: tc.scope})
			}
			if err := ValidateCustomSkillAction(cs); err != nil {
				t.Fatal("invalid regression fixture", err)
			}
			a.CustomSkills = []ActionCustomSkill{cs}
			b := MustNewParticipant("b", 100, 0, 0)
			e1, e2 := MustNewParticipant("e1", 10, 0, 0), MustNewParticipant("e2", 10, 0, 0)
			e1.ExperienceReward, e1.GoldReward = 7, 11
			e2.ExperienceReward, e2.GoldReward = 13, 19
			if tc.revive {
				e1.Abilities = []string{"revive"}
			}
			r, err := (Engine{}).ResolvePartyBattle(PartyBattleRequest{
				Allies: []Participant{a, b}, Enemies: []Participant{e1, e2},
				VictoryReward: Reward{Experience: 20, Currency: 30}, RNG: fixedFloatRNG{val: 0},
			})
			if err != nil {
				t.Fatal(err)
			}
			if r.Outcome != OutcomeWin || r.Turns != 1 || r.RemainingCMP[a.ID] != 0 {
				t.Fatalf("custom skill did not execute as expected: outcome=%s turns=%d CMP=%v", r.Outcome, r.Turns, r.RemainingCMP)
			}
			var exp, gold, crystals int
			for _, e := range r.DefeatEarnings {
				exp, gold, crystals = exp+e.Reward.Experience, gold+e.Reward.Currency, crystals+e.Reward.Crystals
				if !reflect.DeepEqual(e.RecipientIDs, []string{"a", "b"}) {
					t.Fatal("incorrect recipients", e.RecipientIDs)
				}
			}
			if len(r.DefeatEarnings) != 2 || exp != 20 || gold != 30 || crystals != 2 {
				t.Fatalf("earnings repeated without a new defeat: count=%d EXP=%d gold=%d crystals=%d", len(r.DefeatEarnings), exp, gold, crystals)
			}
			if r.TotalReward != (Reward{Experience: 22, Currency: 33, Crystals: 2}) {
				t.Fatal("aggregate reward or cooperative bonus changed", r.TotalReward)
			}
		})
	}
}

func TestDefeatEarnings_EnemyRevivedAfterDefeatEarnsAgain(t *testing.T) {
	a, e := MustNewParticipant("a", 100, 100, 0), MustNewParticipant("e", 10, 0, 0)
	e.ExperienceReward, e.GoldReward = 7, 11
	c := &battleContext{
		req: PartyBattleRequest{Allies: []Participant{a}}, rng: fixedFloatRNG{val: 0},
		field: &FieldState{}, hpMap: map[string]int{"a": 100, "e": 10}, mpMap: map[string]int{},
		teamMap:    map[string]string{"a": "allies", "e": "enemies"},
		allyTeamID: "allies", allParticipants: []Participant{a, e}, crystalDroppedMap: map[string]bool{},
	}
	for i, hp := range []int{10, 0, 5} {
		// Restoring HP represents a later revival, not a new enemy identity.
		c.hpMap[e.ID] = hp
		c.applyDamage(a, e, 100, "", "attack", false, "", false)
		if want := []int{1, 1, 2}[i]; len(c.defeatEarnings) != want {
			t.Fatalf("step %d: earned %d times, want %d", i, len(c.defeatEarnings), want)
		}
	}
	for _, earnings := range c.defeatEarnings {
		if earnings.Reward.Experience != 7 || earnings.Reward.Currency != 11 || !reflect.DeepEqual(earnings.RecipientIDs, []string{"a"}) {
			t.Fatal("legitimate re-defeat lost earnings", c.defeatEarnings)
		}
	}
}
