package dungeon

import (
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestDungeonEnemyCombatPayload(t *testing.T) {
	stage6 := *stagePool(6).Boss
	if p := stage6.participant(); p.Status != corebattle.StatusSleep || p.MP != 60 || p.MaxMP != 60 {
		t.Fatalf("stage 6 participant %+v", p)
	}
	if a := stage6.combat; a == nil || a.Hit != 150 || a.Temporary != corebattle.TemporaryDouble || a.CrystalReward != 3 ||
		a.Jobs != (corebattle.BattleJobs{JobID: "job-21", SP: 60, OldJobID: "starter", OldSP: 20}) {
		t.Fatalf("stage 6 payload %+v", a)
	}

	if p := nm9Monsters[0].participant(); p.Status != corebattle.StatusMagicSeal || p.MP != 149 {
		t.Fatalf("map9 participant %+v", p)
	}
	if p := nm10Pool.Monsters[0].participant(); p.Status != corebattle.StatusPhysicalSeal || p.MP != 144 {
		t.Fatalf("map10 participant %+v", p)
	}
	if a := nm7FixedBoss.combat; a == nil || a.Hit != 500 || a.Temporary != corebattle.TemporaryNullAttack || a.MaxMP != 99999 {
		t.Fatalf("map7 payload %+v", a)
	}
	if a := nm14Mimic.combat; a == nil || a.Jobs.JobID != "job-93" || a.Jobs.SP != 10 || a.Temporary != corebattle.TemporaryDouble {
		t.Fatalf("ship mimic payload %+v", a)
	}
	for i, tc := range []struct {
		monsterIndex int
		secondGem    string
		cost         int
	}{
		{4, "", 29},
		{2, "gem_regen_1", 49},
		{4, "gem_crisis_strike", 56},
	} {
		m := nm14Pools[i].Monsters[tc.monsterIndex]
		if m.ID != "nm14-diver-chick" || m.combat == nil || m.combat.NPCLoadout == nil ||
			m.combat.NPCLoadout.Gems != [3]string{"gem_sky_atk_1", tc.secondGem} || m.combat.NPCLoadout.CustomCost != tc.cost {
			t.Fatalf("diver in pool %d: %+v", i, m.combat)
		}
		if p := m.participant(); p.CMP != 80 || p.MaxCMP != 80 {
			t.Fatalf("diver in pool %d: %+v", i, p)
		}
	}
}
