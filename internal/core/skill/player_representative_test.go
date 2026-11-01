package skill

import (
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// ff97fc5:_skill.cgi:1239,1266,1268. Reuse the fixed representative
// heal30/magic20/area25 definitions with explicit target and modifier owners.
func TestPlayerRegisteredRepresentativeCallbacks(t *testing.T) {
	for _, row := range []struct {
		id     string
		target string
		hp     int
	}{
		{"job-5-entry-3", "self", 527},
		{"job-6-entry-1", "enemy", 882},
		{"job-6-entry-3", "enemy", 878},
	} {
		for _, sealed := range []bool{false, true} {
			s, c, _ := basicMagicFixture()
			var d BattleDefinitions
			d.RegisterPlayerCallbacks(PlayerCallbackContexts{BasicMagic: c}, 3)
			if sealed {
				s.Actors[0].Status = corebattle.StatusMagicSeal
			}
			f := corebattle.CallbackContext{TargetContext: c.TargetContext, Arguments: []string{row.target, "tail"}}
			ops, err := d.InvocationAction(&s, &f, []string{row.id}, noArithmeticDraws{t})
			if err != nil {
				t.Fatal(row.id, err)
			}
			draws := []float64{0}
			if sealed {
				draws = []float64{.9}
			}
			rng := &damageGemDraws{t: t, draws: draws}
			if err := s.ApplyInvocations(&f, d.Invocations, ops, rng); err != nil || rng.used != len(draws) {
				t.Fatal(row.id, sealed, err, rng.used)
			}
			index, want := 1, row.hp
			if row.target == "self" {
				index = 0
			}
			if sealed {
				want = 900
				if index == 0 {
					want = 500
				}
			}
			if s.Actors[index].HP != want || s.Actors[0].MP != 80 {
				t.Fatal(row.id, sealed, s.Actors)
			}
			args := []string{"tail"}
			if row.id == "job-6-entry-3" {
				args = []string{row.target, "tail"}
			}
			if !reflect.DeepEqual(c.Arguments, args) {
				t.Fatal(row.id, c.Arguments)
			}
		}
	}
}
