package dungeon

import "testing"

func TestGridFloor_DerivesGeometryAndRejectsRaggedGrid(t *testing.T) {
	f := gridFloor(Floor{FloorNumber: 2}, []string{"010", "0S0"})
	if f.Width != 3 || f.Height != 2 || f.StartX != 1 || f.StartY != 1 || f.FloorNumber != 2 {
		t.Fatalf("floor %+v", f)
	}
	for _, rows := range [][]string{nil, {"000", "00"}} {
		func() {
			defer func() {
				if recover() == nil {
					t.Fatalf("grid %q accepted", rows)
				}
			}()
			gridFloor(Floor{}, rows)
		}()
	}
}

func TestDungeon_FloorNameFallback(t *testing.T) {
	d := Dungeon{Name: "元の迷宮"}
	if got := d.floorName(Floor{}); got != "元の迷宮" {
		t.Fatalf("default floor name = %q, want 元の迷宮", got)
	}
	if got := d.floorName(Floor{Name: "秘密の部屋"}); got != "秘密の部屋" {
		t.Fatalf("override floor name = %q, want 秘密の部屋", got)
	}
}
