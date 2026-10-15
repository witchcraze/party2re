package battle

import (
	"encoding/json"
	"reflect"
	"testing"
)

func rosterFixture() (LifetimeState, RosterContext) {
	level := 70
	s := LifetimeState{Actors: []LifetimeActor{
		{Participant: Participant{ID: "player", TeamID: "party", HP: 100, MaxHP: 100, MP: 100, MaxMP: 100, Attack: 100, Defense: 80, Agility: 60, BaseStats: BattleBaseStats{100, 80, 60}, Gear: &BattleGearStats{}}, Tension: 1, Hit: 95},
		{Participant: Participant{ID: "enemy", TeamID: "npc", HP: 1000, MaxHP: 1000, Gear: &BattleGearStats{}}, NPC: true, Tension: 1, Hit: 95},
	}}
	c := RosterContext{DamageContext: DamageContext{EffectContext: EffectContext{TargetContext: TargetContext{ActorID: "player", PartyIDs: []string{"player"}, EnemyIDs: []string{"enemy"}}, RequesterLevel: &level}}, EnemyTeamID: "npc"}
	return s, c
}

func TestHelperIdentityDuplicateRemovalAndReload(t *testing.T) {
	s, c := rosterFixture()
	rng := &targetRNG{t: t, draws: []float64{0, .2, .4, .6, .999}}
	r, err := s.ApplyRoster(&c, "tool-72", "", rng)
	if err != nil || r.HelperID == "" {
		t.Fatalf("summon: %+v %v", r, err)
	}
	a := s.actor(r.HelperID)
	if a.HP != 135 || a.MP != 959 || a.Attack != 408 || a.Defense != 54 || a.Agility != 179 || a.Tension != 1 || a.Hit != 95 || a.TeamID != "party" || a.ExperienceReward != 35 || a.GoldReward != 17 {
		t.Fatalf("helper: %+v", a)
	}
	before, _ := json.Marshal(s)
	if r, err := s.ApplyRoster(&c, "tool-73", "", &targetRNG{t: t}); err != nil || r.Applied {
		t.Fatalf("shared duplicate: %+v %v", r, err)
	}
	a.HP = 0
	s.Actors[0].HP = 0
	s.PruneRoster(&c.TargetContext)
	if s.actor(r.HelperID) != nil || s.actor("player") == nil {
		t.Fatal("only fallen NPC/helpers should be removed")
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	newHelper, err := loaded.ApplyRoster(&c, "tool-74", "", &targetRNG{t: t, draws: []float64{0, 0, 0, 0, 0}})
	if err != nil || newHelper.HelperID == r.HelperID {
		t.Fatalf("recall needs a new stable identity: %+v %v; before=%s", newHelper, err, before)
	}
	if !reflect.DeepEqual(c.PartyIDs, []string{"player", newHelper.HelperID}) {
		t.Fatal(c.PartyIDs)
	}
}
