package boss

import (
	"slices"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func stage(t *testing.T, id string) BossStage {
	t.Helper()
	stages, err := DefaultBossCatalog()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(stages, func(s BossStage) bool { return s.ID == id })
	if i < 0 {
		t.Fatalf("stage %s missing", id)
	}
	return stages[i]
}

func count(list []string, id string) int {
	n := 0
	for _, v := range list {
		if v == id {
			n++
		}
	}
	return n
}

// The tables transcribe party2/stage/king*.cgi: repeats are weights, ranges are expanded.
func TestCatalogTreasureTables(t *testing.T) {
	if items := stage(t, "king1").Treasures.Items; len(items) != 6 || count(items, "item-059") != 4 {
		t.Fatalf("king1 items = %v, want [59,59,59,59,71,104]", items)
	}
	if armors := stage(t, "king7").Treasures.Armors; !slices.Equal(armors, []string{"armor-03"}) {
		t.Fatalf("king7 armors = %v", armors)
	}
	items := stage(t, "king10").Treasures.Items
	if len(items) != 66 || items[0] != "item-001" || items[42] != "item-043" || !slices.Contains(items, "item-089") || !slices.Contains(items, "item-103") {
		t.Fatalf("king10 items = %v", items)
	}
}

func TestCatalogBossRows(t *testing.T) {
	king5 := stage(t, "king5")
	if len(king5.Bosses) != 1 {
		t.Fatalf("king5 bosses = %d", len(king5.Bosses))
	}
	a := king5.Bosses[0].actor("b")
	if a.Hit != 1000 || a.MaxMP != 9999999 || a.MP != 999999 || a.Status != "全封" || a.Temporary != "100倍" || a.Jobs.OldJobID != "job-96" {
		t.Fatalf("king5 boss = %+v", a)
	}
	// make_vs_king writes a missing field as 0 and defaults hit and the maxima.
	box := stage(t, "king10").Bosses[0].actor("b")
	if box.Defense != 0 || box.Jobs.OldSP != 0 || box.MaxHP != box.HP || box.Hit != 300 || box.Temporary != corebattle.TemporaryReduceAttack {
		t.Fatalf("king10 boss = %+v", box)
	}
	king1 := stage(t, "king1").Bosses[0].actor("b")
	if king1.Hit != 95 || king1.Tension != 1 || king1.MaxMP != king1.MP {
		t.Fatalf("king1 defaults = %+v", king1)
	}
	for _, s := range []string{"king1", "king2", "king4", "king10"} {
		st := stage(t, s)
		if _, leader := stageRoster(st, nil); leader == "" {
			t.Fatalf("%s has no leader boss %s", s, st.LeaderName)
		}
	}
}

func TestCatalogSummonPool(t *testing.T) {
	st := stage(t, "king10")
	if len(st.Monsters) != 7 || !st.Monsters[0].RandomCrystal {
		t.Fatalf("king10 pool = %+v", st.Monsters)
	}
	ones, zeros := st.summonPool(fixedRNG(0.99)), st.summonPool(fixedRNG(0))
	if ones[0].CrystalReward != 1 || zeros[0].CrystalReward != 0 || ones[0].MaxHP != 0 || ones[0].Name != "ブチスライム" {
		t.Fatalf("pool payload = %+v / %+v", ones[0], zeros[0])
	}
	if len(stage(t, "king1").Monsters) != 0 {
		t.Fatal("king1 has no @monsters")
	}
}

type fixedRNG float64

func (r fixedRNG) Float64() float64 { return float64(r) }
func (r fixedRNG) Intn(n int) int   { return int(float64(r) * float64(n)) }
func (r fixedRNG) IntN(n int) int   { return r.Intn(n) }

// _battle.cgi:152–170: <=600s ×3, <=1800s ×2, <=3600s ×1.5 (floored), otherwise ×1.
func TestScaleCrystals(t *testing.T) {
	for _, tc := range []struct {
		elapsed time.Duration
		want    int
	}{{600 * time.Second, 15}, {601 * time.Second, 10}, {1800 * time.Second, 10}, {1801 * time.Second, 7}, {3600 * time.Second, 7}, {3601 * time.Second, 5}} {
		if got := scaleCrystals(5, tc.elapsed); got != tc.want {
			t.Errorf("%v: got %d, want %d", tc.elapsed, got, tc.want)
		}
	}
}

func TestKing99ClonesTheUnsealers(t *testing.T) {
	unsealers := []corebattle.LifetimeActor{
		{Participant: corebattle.Participant{ID: "a", Name: "Leader", MaxHP: 100, MaxMP: 10, Attack: 50, Defense: 40, Agility: 30, ExperienceReward: 3, GoldReward: 2}, Jobs: corebattle.BattleJobs{JobID: "job-02", SP: 40}},
		{Participant: corebattle.Participant{ID: "b", Name: "Member", MaxHP: 80}},
	}
	roster, leader := stageRoster(stage(t, cloneStageID), unsealers)
	c := roster[0]
	if len(roster) != 2 || leader != c.ID || c.MaxHP != 5000 || c.MP != 500 || c.Attack != 100 || c.ExperienceReward != 90 || c.GoldReward != 60 ||
		c.CrystalReward != 20 || c.Temporary != corebattle.TemporaryStrongDefense || c.Jobs.JobID != "job-02" || c.TeamID != "enemies" || !c.NPC {
		t.Fatalf("king99 clone = %+v (leader %s)", c, leader)
	}
}
