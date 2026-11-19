package dungeon

import (
	"fmt"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/stagedata"
)

func TestStagePool_MirrorsSharedStageTable(t *testing.T) {
	stages, err := stagedata.Stages()
	if err != nil {
		t.Fatal(err)
	}
	for n := 2; n <= 8; n++ {
		p, st := stagePool(n), stages[fmt.Sprintf("stage-%02d", n)]
		ids := make([]string, len(p.Monsters))
		for i, m := range p.Monsters {
			ids[i] = m.ID
		}
		if !slices.Equal(ids, st.MonsterIDs) || !slices.Equal(p.Appears, st.Appears) || !slices.Contains(st.BossRosters[0], p.Boss.ID) {
			t.Errorf("stage %d pool %+v", n, p)
		}
	}
}

// Each map draws the legacy stage range of its map file.
func TestNativeMaps_DrawTheirLegacyStages(t *testing.T) {
	stageOf := map[string]int{}
	for n := 2; n <= 8; n++ {
		stageOf[stagePool(n).Boss.ID] = n
	}
	want := map[string][]int{
		NativeMap1ID: {2},          // map/1/*.cgi
		NativeMap3ID: {5, 6, 7, 8}, // int(rand(4)+5)
		NativeMap4ID: {4, 5, 6, 7, 8},
		NativeMap5ID: {5, 6, 7, 8},
	}
	floor2 := map[int][]int{1: {2, 3, 4}, 2: {3, 4, 5}, 3: {4, 5, 6}, 4: {3, 4, 5, 6}} // map/2 per floor
	for _, d := range DefaultDungeonCatalog() {
		for i, f := range d.Variants {
			exp, ok := want[d.ID]
			if d.ID == NativeMap2ID {
				exp, ok = floor2[f.FloorNumber], true
			}
			if !ok {
				continue
			}
			got := make([]int, len(f.Pools))
			for j, p := range f.Pools {
				got[j] = stageOf[p.Boss.ID]
			}
			if !slices.Equal(got, exp) {
				t.Errorf("%s variant %d stages %v, want %v", d.ID, i+1, got, exp)
			}
		}
	}
}
