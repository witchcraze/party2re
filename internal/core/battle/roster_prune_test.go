package battle

import (
	"reflect"
	"testing"
)

func TestPruneRosterPreservesSidesAndOrderWhenActorIsRemoved(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		actorNPC               bool
		actorHP                int
		actorTeam              string
		keepActor              bool
		wantParty, wantEnemies []string
	}{
		{"fallen NPC", true, 0, "npc", false, []string{"npc-first", "npc-last"}, []string{"living-player", "fallen-player", "player-last"}},
		{"living NPC", true, 100, "npc", true, []string{"actor", "npc-first", "npc-last"}, []string{"living-player", "fallen-player", "player-last"}},
		{"fallen player", false, 0, "party", true, []string{"actor", "living-player", "fallen-player", "player-last"}, []string{"npc-first", "npc-last"}},
		{"living player", false, 100, "party", true, []string{"actor", "living-player", "fallen-player", "player-last"}, []string{"npc-first", "npc-last"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := LifetimeState{Actors: []LifetimeActor{
				{Participant: Participant{ID: "actor", TeamID: tc.actorTeam, HP: tc.actorHP}, NPC: tc.actorNPC},
				{Participant: Participant{ID: "living-player", TeamID: "party", HP: 100}},
				{Participant: Participant{ID: "npc-first", TeamID: "npc", HP: 100}, NPC: true},
				{Participant: Participant{ID: "fallen-player", TeamID: "party", HP: 0}},
				{Participant: Participant{ID: "fallen-helper", TeamID: "party", HP: 0}, RosterIdentity: "dummy"},
				{Participant: Participant{ID: "fallen-npc", TeamID: "npc", HP: -1}, NPC: true},
				{Participant: Participant{ID: "player-last", TeamID: "party", HP: 100}},
				{Participant: Participant{ID: "npc-last", TeamID: "npc", HP: 100}, NPC: true},
			}}
			c := TargetContext{ActorID: "actor"}
			for _, a := range s.Actors {
				if a.TeamID == tc.actorTeam {
					c.PartyIDs = append(c.PartyIDs, a.ID)
				} else {
					c.EnemyIDs = append(c.EnemyIDs, a.ID)
				}
			}
			s.PruneRoster(&c)
			wantMembers := []string{"living-player", "npc-first", "fallen-player", "player-last", "npc-last"}
			if tc.keepActor {
				wantMembers = append([]string{"actor"}, wantMembers...)
			}
			var memberIDs []string
			for _, a := range s.Actors {
				memberIDs = append(memberIDs, a.ID)
			}
			if !reflect.DeepEqual(memberIDs, wantMembers) || !reflect.DeepEqual(c.PartyIDs, tc.wantParty) || !reflect.DeepEqual(c.EnemyIDs, tc.wantEnemies) {
				t.Fatalf("members=%v party=%v enemies=%v; want members=%v party=%v enemies=%v", memberIDs, c.PartyIDs, c.EnemyIDs, wantMembers, tc.wantParty, tc.wantEnemies)
			}
			if c.ActorID != "actor" || s.actor("fallen-player").HP != 0 {
				t.Fatal("save boundary changed actor identity or fallen player HP")
			}
		})
	}
}
