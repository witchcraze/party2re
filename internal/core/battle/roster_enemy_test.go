package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"
)

func enemySummonPayload() LifetimeActor {
	return LifetimeActor{Participant: Participant{ID: "species", Name: "敵", HP: 101, MP: 51, Attack: 41, Defense: 31, Agility: 21, CMP: 11, MaxHP: 777, BaseStats: BattleBaseStats{Attack: 222}, Gear: &BattleGearStats{}, ExperienceReward: 11, GoldReward: 7, Status: StatusPoison, CustomSkills: []ActionCustomSkill{{ID: "custom", CMPCost: 47, Gems: []GemEffect{{Kind: "unresolved"}}}}}, Jobs: BattleJobs{JobID: "job-2", SP: 13, OldJobID: "job-3", OldSP: 9}, Hit: 80, Tension: 2, Extended: ExtendedFlame, ExtendedTurns: 4, CrystalReward: 3}
}

func TestEnemySummonScalingPayloadAndTwoSuffixAttempts(t *testing.T) {
	s, c := rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	c.AppearancePositions = []int{999, 0, 0}
	c.Hard = true
	before, _ := json.Marshal(c.EnemyPool)
	r, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil {
		t.Fatal(err)
	}
	a := s.actor(r.HelperID)
	if [6]int{a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP} != ([6]int{143, 72, 57, 43, 29, 15}) || a.MaxHP != 777 || a.MaxMP != 72 || a.MaxCMP != 15 || a.BaseStats.Attack != 222 || a.BaseStats.Defense != 43 || a.Jobs.SP != 19 || a.Jobs.OldSP != 13 || a.Hit != 80 || a.Tension != 2 || a.Status != StatusPoison || a.ExtendedTurns != 4 || a.ExperienceReward != 16 || a.GoldReward != 10 || a.CrystalReward != 4 || a.CustomSkills[0].CMPCost != 47 {
		t.Fatal(a)
	}
	if !reflect.DeepEqual(c.EnemyIDs, []string{"enemy", r.HelperID}) {
		t.Fatal(c.EnemyIDs)
	}
	second, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, .9}})
	if err != nil || s.actor(second.HelperID).RosterIdentity != "enemy-species-Z" {
		t.Fatal(second, err)
	}
	if r, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, .5}}); err != nil || r.Applied {
		t.Fatal("two occupied suffixes", r, err)
	}
	// Live incarnations must not mutate the server-owned payload on use/reload.
	a.CustomSkills[0].Gems[0].Kind = "changed"
	a.Gear.WeaponID = "changed"
	after, _ := json.Marshal(c.EnemyPool)
	if string(before) != string(after) {
		t.Fatal("mutated payload")
	}
}

func TestEnemySummonSizeGemGuardAndNPCSide(t *testing.T) {
	s, c := rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	r, err := s.ApplyRoster(&c, "gem-82", "", &targetRNG{t: t, draws: []float64{.5, 0}})
	if err != nil || !r.Applied {
		t.Fatal(r, err)
	}
	c.EnemyPool = nil
	if r, err := s.ApplyRoster(&c, "gem-82", "", &targetRNG{t: t}); err != nil || r.Applied {
		t.Fatal(r, err)
	}
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	for len(s.Actors) < 10 {
		a := LifetimeActor{Participant: Participant{ID: string(rune('a' + len(s.Actors))), TeamID: "party", HP: 1}}
		s.Actors = append(s.Actors, a)
		c.PartyIDs = append(c.PartyIDs, a.ID)
	}
	c.EnemyPool = nil
	if r, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t}); err != nil || r.Applied {
		t.Fatal(r, err)
	}
	// NPC invocation adds another NPC to its own ordered party.
	s, c = rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	c.ActorID = "enemy"
	c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	r, err = s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil || !reflect.DeepEqual(c.PartyIDs, []string{"enemy", r.HelperID}) || s.actor(r.HelperID).TeamID != "npc" {
		t.Fatal(r, err, c)
	}
}

func TestEnemySummonWeightsFallbackAndReloadContinuation(t *testing.T) {
	s, c := rosterFixture()
	one, two := enemySummonPayload(), enemySummonPayload()
	two.ID = "second"
	two.Name = "別の敵"
	c.EnemyPool = []LifetimeActor{one, two}
	c.AppearancePositions = []int{1, 1, 0}
	r, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{.4, .5}})
	if err != nil || s.actor(r.HelperID).RosterIdentity != "enemy-second-P" {
		t.Fatal(r, err)
	}
	data, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatal(err)
	}
	c2 := c
	c2.EnemyIDs = append([]string(nil), c.EnemyIDs...)
	r1, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{.4, .5}})
	if err != nil {
		t.Fatal(err)
	}
	r2, err := loaded.ApplyRoster(&c2, "tool-78", "", &targetRNG{t: t, draws: []float64{.4, .5}})
	if err != nil || !reflect.DeepEqual(r1, r2) || !reflect.DeepEqual(s, loaded) {
		t.Fatal(r1, r2, err)
	}
	// Missing certification/payload rejects before suffix/selection draws.
	s, c = rosterFixture()
	before, _ := json.Marshal(s)
	if _, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatal(err)
	}
	after, _ := json.Marshal(s)
	if string(before) != string(after) {
		t.Fatal("rejected summon mutated state")
	}
}
