package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
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
	if err != nil || s.actor(second.HelperID).RosterIdentity != "species-Z" {
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

// Legacy $win (arena, guild battle, a record-holding Challenge) makes the
// summon a no-op even without a stage pool (_skill.cgi:9835).
func TestEnemySummonDoesNothingUnderWin(t *testing.T) {
	s, c := rosterFixture()
	c.NoEnemySummon = true
	before := len(s.Actors)
	if err := s.ValidateRoster(&c, "gem_summon_sky", "", &targetRNG{t: t}); err != nil {
		t.Fatal(err)
	}
	if r, err := s.ApplyRoster(&c, "gem_summon_sky", "", &targetRNG{t: t}); err != nil || r.Applied || len(s.Actors) != before {
		t.Fatal(r, err)
	}
}

func TestEnemySummonSizeGemGuardAndNPCSide(t *testing.T) {
	s, c := rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	r, err := s.ApplyRoster(&c, "gem_summon_sky", "", &targetRNG{t: t, draws: []float64{.5, 0}})
	if err != nil || !r.Applied {
		t.Fatal(r, err)
	}
	c.EnemyPool = nil
	if r, err := s.ApplyRoster(&c, "gem_summon_sky", "", &targetRNG{t: t}); err != nil || r.Applied {
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
	if err != nil || s.actor(r.HelperID).RosterIdentity != "second-P" {
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

	// Negative NPCLoadout costs reject before any summon RNG draw or mutation.
	for _, loadout := range []*NPCLoadout{
		{CustomCost: -1},
		{GemCosts: [3]int{10, 0, -2}},
	} {
		s, c = rosterFixture()
		enemy := enemySummonPayload()
		enemy.NPCLoadout = loadout
		c.EnemyPool = []LifetimeActor{enemy}
		before, _ = json.Marshal(s)
		rng := &targetRNG{t: t}
		if _, err := s.ApplyRoster(&c, "tool-78", "", rng); !errors.Is(err, ErrInvalidParticipant) {
			t.Fatalf("loadout=%+v: err=%v", loadout, err)
		}
		after, _ = json.Marshal(s)
		if string(before) != string(after) {
			t.Fatalf("loadout=%+v mutated state", loadout)
		}
		rng.done()
	}
}

func TestEnemySummonStageReloadDrawsOnceAndRebuildsPool(t *testing.T) {
	s, c := rosterFixture()
	s.Actors[0].MaxHP = 333
	stale := enemySummonPayload()
	stale.ID = "stale"
	c.EnemyPool = []LifetimeActor{stale}
	c.ReloadDraws = 1
	var loaded bool
	c.StageLoaded = &loaded
	rebuilds := 0
	c.RebuildPool = func(summoner LifetimeActor, rng random.Generator) []LifetimeActor {
		rebuilds++
		rng.Float64()
		rebuilt := enemySummonPayload()
		rebuilt.ID = "rebuilt"
		rebuilt.MaxHP = summoner.MaxHP
		return []LifetimeActor{rebuilt}
	}

	// Reload draws and RebuildPool run before suffix and position draws, once.
	rng := &targetRNG{t: t, draws: []float64{.75, .25, .5, 0}}
	r1, err := s.ApplyRoster(&c, "gem_summon_sky", "", rng)
	if err != nil || !r1.Applied || !loaded || rebuilds != 1 || s.actor(r1.HelperID).RosterIdentity != "rebuilt-R" || s.actor(r1.HelperID).MaxHP != 333 {
		t.Fatalf("first summon: err=%v r=%+v loaded=%v rebuilds=%d actor=%+v", err, r1, loaded, rebuilds, s.actor(r1.HelperID))
	}
	rng.done()

	// A second summon sharing StageLoaded skips reload and reuses the rebuilt pool.
	rng = &targetRNG{t: t, draws: []float64{0, 0}}
	r2, err := s.ApplyRoster(&c, "tool-78", "", rng)
	if err != nil || !r2.Applied || rebuilds != 1 || s.actor(r2.HelperID).RosterIdentity != "rebuilt-I" || s.actor(r2.HelperID).MaxHP != 333 {
		t.Fatalf("second summon: err=%v r=%+v rebuilds=%d actor=%+v", err, r2, rebuilds, s.actor(r2.HelperID))
	}
	rng.done()

	// Early return (gem_summon_sky with >1 enemy) skips reload and keeps loaded false.
	s, c = rosterFixture()
	c.EnemyPool = []LifetimeActor{stale}
	c.ReloadDraws = 1
	loaded = false
	c.StageLoaded = &loaded
	s.Actors = append(s.Actors, LifetimeActor{Participant: Participant{ID: "enemy-2", TeamID: "npc", HP: 10}})
	c.EnemyIDs = append(c.EnemyIDs, "enemy-2")
	rng = &targetRNG{t: t}
	if r, err := s.ApplyRoster(&c, "gem_summon_sky", "", rng); err != nil || r.Applied || loaded {
		t.Fatalf("early return loaded stage: err=%v r=%+v loaded=%v", err, r, loaded)
	}
	rng.done()

	// Negative ReloadDraws rejects in validation before RNG or mutation.
	s, c = rosterFixture()
	c.EnemyPool = []LifetimeActor{stale}
	c.ReloadDraws = -1
	if err := s.ValidateRoster(&c, "tool-78", "", rng); !errors.Is(err, ErrUnsupportedEffect) {
		t.Fatalf("negative ReloadDraws: err=%v", err)
	}

	// NPC summon with opposing EnemyTeamID (as runhost passes during npc_turn)
	// still spawns onto the NPC's own team and PartyIDs (#2696).
	s, c = rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	c.ActorID = "enemy"
	c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
	c.EnemyTeamID = "party"
	r, err := s.ApplyRoster(&c, "npc-job-95-position-0", "", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil || !r.Applied || !reflect.DeepEqual(c.PartyIDs, []string{"enemy", r.HelperID}) || s.actor(r.HelperID).TeamID != "npc" {
		t.Fatalf("npc summon with opposing EnemyTeamID: err=%v r=%+v c=%+v actor=%+v", err, r, c, s.actor(r.HelperID))
	}
}

func TestEnemySummonCollisionByEncounterIdentityFamily(t *testing.T) {
	s, c := rosterFixture()
	first := enemySummonPayload()
	first.ID, first.EncounterIdentity = "pos-0", "ordinary-25-boss-22"
	second := enemySummonPayload()
	second.ID, second.EncounterIdentity = "pos-1", "ordinary-25-boss-22"
	distinct := enemySummonPayload()
	distinct.ID, distinct.EncounterIdentity, distinct.Name = "pos-2", "other-family", "別の敵"
	c.EnemyPool = []LifetimeActor{first, second, distinct}

	// Suffix I from pos 0, then suffix I from pos 1 collides and retries Z.
	r1, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, 0}})
	if err != nil || !r1.Applied || s.actor(r1.HelperID).RosterIdentity != "ordinary-25-boss-22-I" {
		t.Fatalf("first summon: err=%v r=%+v actor=%+v", err, r1, s.actor(r1.HelperID))
	}
	r2, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, .5}})
	if err != nil || !r2.Applied || s.actor(r2.HelperID).RosterIdentity != "ordinary-25-boss-22-Z" {
		t.Fatalf("second summon: err=%v r=%+v actor=%+v", err, r2, s.actor(r2.HelperID))
	}

	// Reload keeps the family identities; a third draw for suffix I is a no-op,
	// while a distinct family on suffix I still coexists.
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	var loaded LifetimeState
	if err := json.Unmarshal(raw, &loaded); err != nil {
		t.Fatal(err)
	}
	r3, err := loaded.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, .5}})
	if err != nil || r3.Applied {
		t.Fatalf("third same-family summon should be no-op: err=%v r=%+v", err, r3)
	}
	r4, err := loaded.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{0, .9}})
	if err != nil || !r4.Applied || loaded.actor(r4.HelperID).RosterIdentity != "other-family-I" {
		t.Fatalf("distinct family should coexist: err=%v r=%+v", err, r4)
	}

	// Collides with a same-family member created by another producer (job407).
	s, c = rosterFixture()
	c.EnemyPool = []LifetimeActor{first, second}
	s.Actors = append(s.Actors, LifetimeActor{
		Participant:       Participant{ID: "job407-successor", TeamID: "npc", HP: 500},
		NPC:               true,
		EncounterIdentity: "ordinary-25-boss-22",
		RosterIdentity:    "ordinary-25-boss-22-J",
	})
	c.EnemyIDs = append(c.EnemyIDs, "job407-successor")
	r, err := s.ApplyRoster(&c, "tool-78", "", &targetRNG{t: t, draws: []float64{1.0 / 18.0, .8}})
	if err != nil || !r.Applied || s.actor(r.HelperID).RosterIdentity != "ordinary-25-boss-22-I" {
		t.Fatalf("collision against job407 successor should fall back to I: err=%v r=%+v actor=%+v", err, r, s.actor(r.HelperID))
	}
}
