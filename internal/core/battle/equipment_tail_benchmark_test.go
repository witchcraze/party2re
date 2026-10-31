package battle

import (
	"fmt"
	"testing"
)

func BenchmarkEquipmentTail208Admission(b *testing.B) {
	for _, enemies := range []int{1, 2, 6} {
		b.Run(fmt.Sprintf("enemies%d", enemies), func(b *testing.B) {
			s, c := equipmentTailFixture(false, 6)
			s.actor("self").ItemDefinitionIDs = []string{"item-208"}
			for i := 1; i < enemies; i++ {
				a := *s.actor("enemy")
				a.ID = fmt.Sprintf("enemy-%d", i)
				s.Actors = append(s.Actors, a)
				c.EnemyIDs = append(c.EnemyIDs, a.ID)
			}
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				if err := s.ValidateEquipmentTail(&c, "item-208-combat-tail", resetBoundary(0)); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
