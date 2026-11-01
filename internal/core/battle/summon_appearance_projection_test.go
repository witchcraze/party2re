package battle

import (
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"testing"
)

func TestEnemySummonProjectionAllPositionsSuffixesAndOrder(t *testing.T) {
	for _, weighted := range []bool{false, true} {
		s, c := rosterFixture()
		// State order differs from the operation's ordered rosters. An occupied
		// generated ID must be skipped without reordering either representation.
		s.Actors[0], s.Actors[1] = s.Actors[1], s.Actors[0]
		s.actor(c.ActorID).ID, c.ActorID, c.PartyIDs[0] = "summoned-1", "summoned-1", "summoned-1"
		for i := range 4 {
			a := enemySummonPayload()
			a.ID = "species" + strconv.Itoa(i)
			c.EnemyPool = append(c.EnemyPool, a)
		}
		if weighted {
			c.AppearancePositions = []int{3, 3, -1, 99, 2, 1}
		}
		f := CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}, ArithmeticDivisor: 7, Damage: &c.DamageContext}
		branches, err := s.ProjectEnemySummon(&c, &f)
		if err != nil || len(branches) != 4*18 {
			t.Fatal(weighted, err, len(branches))
		}
		seen := map[string]bool{}
		for _, b := range branches {
			a := b.State.Actors[2]
			seen[a.RosterIdentity] = true
			if a.ID != "summoned-2" || b.State.RosterSequence != 2 || b.State.Actors[0].ID != "enemy" || !reflect.DeepEqual(b.Context.PartyIDs, []string{"summoned-1"}) || !reflect.DeepEqual(b.Context.EnemyIDs, []string{"enemy", "summoned-2"}) || b.Context.ArithmeticDivisor != 7 || !reflect.DeepEqual(b.Context.Arguments, f.Arguments) {
				t.Fatal("lost order, sequence or context", b)
			}
		}
		for i := range 4 {
			for suffix := range 18 {
				if !seen["enemy-species"+strconv.Itoa(i)+"-"+string(rune('I'+suffix))] {
					t.Fatal("omitted source candidate", i, suffix)
				}
			}
		}
	}
}

func TestEnemySummonProjectionDuplicateAndSkippedContexts(t *testing.T) {
	s, c := rosterFixture()
	c.EnemyPool = []LifetimeActor{enemySummonPayload()}
	for _, identity := range []string{"enemy-species-I", "enemy-species-Z"} {
		a := LifetimeActor{Participant: Participant{ID: identity, TeamID: "party", HP: 0}, RosterIdentity: identity}
		s.Actors = append(s.Actors, a)
		c.PartyIDs = append(c.PartyIDs, a.ID)
	}
	f := CallbackContext{TargetContext: c.TargetContext}
	branches, err := s.ProjectEnemySummon(&c, &f)
	if err != nil || len(branches) != 18 || !reflect.DeepEqual(branches[0].State, s) || !reflect.DeepEqual(branches[0].Context.TargetContext, f.TargetContext) || branches[1].State.Actors[4].RosterIdentity != "enemy-species-J" || branches[17].State.Actors[4].RosterIdentity != "enemy-species-Y" {
		t.Fatal("two occupied suffixes are unchanged; fallback is previous suffix", err, branches)
	}
	for _, reason := range []string{"enemy-count", "no-room"} {
		s, c = rosterFixture()
		c.EnemyPool, c.EnemyTeamID, c.RequesterLevel = nil, "", nil
		if reason == "enemy-count" {
			a := LifetimeActor{Participant: Participant{ID: "fallen", TeamID: "npc", HP: 0}}
			s.Actors = append(s.Actors, a)
			c.EnemyIDs = append(c.EnemyIDs, a.ID)
		} else {
			for len(s.Actors) < 10 {
				a := LifetimeActor{Participant: Participant{ID: strconv.Itoa(len(s.Actors)), TeamID: "party", HP: 1}}
				s.Actors = append(s.Actors, a)
				c.PartyIDs = append(c.PartyIDs, a.ID)
			}
		}
		f.TargetContext = c.TargetContext
		branches, err = s.ProjectEnemySummon(&c, &f)
		if err != nil || len(branches) != 1 || !reflect.DeepEqual(branches[0].State, s) {
			t.Fatal(reason, err, branches)
		}
		if _, err := s.ApplyRoster(&c, "gem-82", "", &targetRNG{t: t}); err != nil {
			t.Fatal(reason, err)
		}
	}
	for _, count := range []int{0, 1} {
		s, c = rosterFixture()
		if count == 0 {
			s.Actors = s.Actors[:1]
			c.EnemyIDs = nil
		}
		f.TargetContext = c.TargetContext
		if _, err := s.ProjectEnemySummon(&c, &f); !errors.Is(err, ErrUnsupportedEffect) {
			t.Fatal("reachable summon needs stage payload", count, err)
		}
	}
}

func TestEnemySummonProjectionScalingMaximaAndNPCSide(t *testing.T) {
	// Independent vectors from ff97fc5:_skill.cgi:9885–9906. Hard truncation
	// happens first; explicit maxima/base stats never acquire party scaling.
	for _, tc := range []struct {
		party     int
		hard, npc bool
		stats     [6]int
	}{
		{1, false, false, [6]int{95, 48, 38, 29, 19, 10}},
		{2, false, false, [6]int{101, 51, 41, 31, 21, 11}},
		{4, true, true, [6]int{166, 83, 67, 50, 34, 17}},
	} {
		s, c := rosterFixture()
		if tc.npc {
			c.ActorID = "enemy"
			c.PartyIDs, c.EnemyIDs = c.EnemyIDs, c.PartyIDs
		}
		for len(c.PartyIDs) < tc.party {
			a := *s.actor(c.ActorID)
			a.ID = "member-" + strconv.Itoa(len(c.PartyIDs))
			s.Actors = append(s.Actors, a)
			c.PartyIDs = append(c.PartyIDs, a.ID)
		}
		c.EnemyPool, c.Hard = []LifetimeActor{enemySummonPayload()}, tc.hard
		f := CallbackContext{TargetContext: c.TargetContext}
		branches, err := s.ProjectEnemySummon(&c, &f)
		if err != nil || len(branches) != 18 {
			t.Fatal(tc, err)
		}
		b := branches[0]
		a := b.State.Actors[len(s.Actors)]
		if [6]int{a.HP, a.MP, a.Attack, a.Defense, a.Agility, a.CMP} != tc.stats || a.MaxHP != 777 || a.MaxMP != tc.stats[1] || a.MaxCMP != tc.stats[5] || a.BaseStats.Attack != 222 || a.BaseStats.Defense != tc.stats[3] || !a.NPC || a.TeamID != "npc" {
			t.Fatal(tc, a)
		}
		if tc.npc && (!reflect.DeepEqual(b.Context.EnemyIDs, c.EnemyIDs) || !reflect.DeepEqual(b.Context.PartyIDs, append(append([]string(nil), c.PartyIDs...), a.ID))) {
			t.Fatal("NPC summon must extend its own party in order", b.Context)
		}
	}
}

func TestSummonAppearanceProjectionIsolatesNestedReferents(t *testing.T) {
	for _, appearance := range []bool{false, true} {
		s, c := rosterFixture()
		weapon, sp, debit := 51, 17, 8
		payload := enemySummonPayload()
		payload.Gear.WeaponAttack = &weapon
		payload.JobHistory = map[string]*int{"job-1": &sp}
		payload.ConsumedItems = []ConsumedItem{{ID: "owned", Quantity: 1}}
		c.EnemyPool = []LifetimeActor{payload}
		s.actor(c.ActorID).Gear.WeaponAttack = &weapon
		s.actor(c.ActorID).JobHistory = payload.JobHistory
		c.End.DarknessDebit = &debit
		f := CallbackContext{TargetContext: c.TargetContext, Arguments: []string{"enemy", "tail"}, Damage: &c.DamageContext}
		before, _ := json.Marshal(s)
		pool, _ := json.Marshal(c.EnemyPool)
		var branches []CallbackContinuation
		var err error
		if appearance {
			branches, err = s.ProjectAppearanceGem(&TransformationContext{}, &f)
		} else {
			branches, err = s.ProjectEnemySummon(&c, &f)
		}
		if err != nil || len(branches) < 2 {
			t.Fatal(err, len(branches))
		}
		a := &branches[0].State.Actors[0]
		if !appearance {
			a = &branches[0].State.Actors[2]
			a.CustomSkills[0].Gems[0].Kind = "changed"
			a.ConsumedItems[0].ID = "changed"
		}
		*a.Gear.WeaponAttack = 999
		*a.JobHistory["job-1"] = 999
		*branches[0].Context.Damage.RequesterLevel = 999
		*branches[0].Context.Damage.End.DarknessDebit = 999
		branches[0].Context.EnemyIDs[0] = "changed"
		branches[0].Context.Arguments[0] = "changed"
		after, _ := json.Marshal(s)
		poolAfter, _ := json.Marshal(c.EnemyPool)
		index := 0
		if !appearance {
			index = 2
		}
		if string(before) != string(after) || string(pool) != string(poolAfter) || weapon != 51 || sp != 17 || debit != 8 || *branches[1].State.Actors[index].Gear.WeaponAttack != 51 || *branches[1].State.Actors[index].JobHistory["job-1"] != 17 || branches[1].Context.EnemyIDs[0] != "enemy" || branches[1].Context.Arguments[0] != "enemy" || *branches[1].Context.Damage.RequesterLevel != *c.RequesterLevel {
			t.Fatal("caller, payload or sibling shared a referent", appearance, branches[1])
		}
	}
}
