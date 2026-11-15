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
