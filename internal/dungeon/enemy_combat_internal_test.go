package dungeon

import (
	"math"
	"slices"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// drawRNG replays floats and ints; exhausted draws return 0.
type drawRNG struct {
	floats []float64
	ints   []int
}

func (r *drawRNG) Float64() float64 {
	if len(r.floats) == 0 {
		return 0
	}
	v := r.floats[0]
	r.floats = r.floats[1:]
	return v
}

func (r *drawRNG) Intn(n int) int {
	if len(r.ints) == 0 {
		return 0
	}
	v := r.ints[0]
	r.ints = r.ints[1:]
	return v % n
}

// rollMax is the largest rand() value.
var rollMax = math.Nextafter(1, 0)

func TestDungeonEnemyCombatPayload(t *testing.T) {
	stage6 := stagePool(6).Bosses[0]
	if a := stage6.combat; a == nil || a.Hit != 150 || a.Temporary != corebattle.TemporaryDouble || a.CrystalReward != 3 ||
		a.Jobs != (corebattle.BattleJobs{JobID: "job-21", SP: 60, OldJobID: "starter", OldSP: 20}) {
		t.Fatalf("stage 6 payload %+v", a)
	}
	// Party 2 at rand 0 scales by 0.9.
	spawned := func(m DungeonMonster) corebattle.Participant {
		return participants([]DungeonMonster{m.spawn(2, &drawRNG{})})[0]
	}
	if p := spawned(stage6); p.Status != corebattle.StatusSleep || p.MP != 54 || p.MaxMP != 54 {
		t.Fatalf("stage 6 participant %+v", p)
	}
	if p := spawned(nm9Monsters[0]); p.Status != corebattle.StatusMagicSeal || p.MP != 134 {
		t.Fatalf("map9 participant %+v", p)
	}
	if p := spawned(nm10Pool.Monsters[0]); p.Status != corebattle.StatusPhysicalSeal || p.MP != 129 {
		t.Fatalf("map10 participant %+v", p)
	}
	if a := nm7FixedBoss[0].combat; a == nil || a.Hit != 500 || a.Temporary != corebattle.TemporaryNullAttack || a.MaxMP != 99999 {
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
		if p := spawned(m); p.CMP != 72 || p.MaxCMP != 72 {
			t.Fatalf("diver in pool %d: %+v", i, p)
		}
	}
}

// add_monster's count int(rand(2.5 + party × 0.6)) + 1 at both rand bounds.
func TestEncounterGroupSizeBounds(t *testing.T) {
	pool := EncounterPool{Monsters: []DungeonMonster{{ID: "m", Name: "スライム", HP: 10}}}
	for party, want := range map[int][2]int{1: {1, 4}, 2: {1, 4}, 3: {1, 5}, 4: {1, 5}} {
		for i, r := range []float64{0, rollMax} {
			group := pool.group(party, &drawRNG{floats: []float64{r}})
			if len(group) != want[i] {
				t.Errorf("party %d rand %v: %d members, want %d", party, r, len(group), want[i])
			}
			for j, m := range group {
				if m.Name != "スライム"+string(rune('A'+j)) {
					t.Errorf("party %d member %d named %q", party, j, m.Name)
				}
			}
		}
	}
}

// Each member draws through @appears; an index without a monster falls back to 0.
func TestEncounterGroupDrawsWeightedMembers(t *testing.T) {
	pool := EncounterPool{Monsters: []DungeonMonster{{ID: "a", HP: 10}, {ID: "b", HP: 10}}, Appears: []int{1, 7}}
	group := pool.group(4, &drawRNG{floats: []float64{0.5}, ints: []int{0, 1, 0}})
	var ids []string
	for _, m := range group {
		ids = append(ids, m.ID)
	}
	if !slices.Equal(ids, []string{"b", "a", "b"}) {
		t.Fatalf("members %v", ids)
	}
	ps := participants(group)
	if ps[0].ID == ps[2].ID {
		t.Fatalf("repeated monsters share participant ID %q", ps[0].ID)
	}
}

// The party-size roll at its bounds for each stat; source maxima survive.
func TestEnemySpawnRollBounds(t *testing.T) {
	src := DungeonMonster{HP: 1000, Attack: 100, Defense: 200, Agility: 50,
		combat: enemyCombat(corebattle.LifetimeActor{Participant: corebattle.Participant{MP: 300, CMP: 40, MaxMP: 900}})}
	for _, tc := range []struct {
		party int
		r     float64
		want  [6]int // hp mp at df ag c_mp
	}{
		{1, 0, [6]int{800, 240, 80, 160, 40, 32}},
		{2, 0, [6]int{900, 270, 90, 180, 45, 36}},
		{4, rollMax, [6]int{1400, 420, 140, 280, 70, 56}}, // the sum rounds up to 1.4, as in Perl
	} {
		draws := make([]float64, 6)
		for i := range draws {
			draws[i] = tc.r
		}
		m := src.spawn(tc.party, &drawRNG{floats: draws})
		got := [6]int{m.HP, m.combat.MP, m.Attack, m.Defense, m.Agility, m.combat.CMP}
		if got != tc.want || m.combat.MaxHP != m.HP || m.combat.MaxMP != 900 || m.combat.MaxCMP != m.combat.CMP {
			t.Errorf("party %d rand %v: %v max %d/%d/%d, want %v", tc.party, tc.r, got, m.combat.MaxHP, m.combat.MaxMP, m.combat.MaxCMP, tc.want)
		}
	}
	if src.HP != 1000 || src.combat.MP != 300 || src.combat.MaxHP != 0 {
		t.Fatalf("spawn mutated its source %+v %+v", src, src.combat)
	}
}

// Map17 w rolls its 6000 HP once and keeps the preset 12000 maximum.
func TestMap17NecroRollKeepsPresetMaximum(t *testing.T) {
	for _, tc := range []struct {
		party int
		r     float64
		hp    int
	}{{1, 0, 4800}, {4, rollMax, 8400}} {
		p := participants(spawnRoster([]DungeonMonster{nm17Necro}, tc.party, &drawRNG{floats: []float64{tc.r}}))[0]
		if p.HP != tc.hp || p.MaxHP != 12000 {
			t.Errorf("party %d: HP %d/%d, want %d/12000", tc.party, p.HP, p.MaxHP, tc.hp)
		}
	}
}
