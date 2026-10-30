package skill

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestRangedReloadedContinuation(t *testing.T) {
	for _, job := range []int{11, 42, 54} {
		for _, entry := range corebattle.RangedEntries(job) {
			for _, seed := range []uint64{1459, 42, 54} {
				t.Run(fmt.Sprintf("%s/seed-%d", entry.ID, seed), func(t *testing.T) {
					s, c, d := rangedFixture()
					// Commit actual theft before interruption; retry must use the
					// resulting facts and never replay the completed callback.
					executeRanged(t, &s, c, d, "job-11-entry-4", []string{"enemy"}, 0)
					args := []string{"enemy", "tail"}
					if seed == 42 {
						s.Actors[0].Status, args[0] = corebattle.StatusConfusion, "missing"
					} else if seed == 54 {
						s.Actors[0].Status = corebattle.StatusPhysicalSeal
						s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
					}
					type checkpoint struct {
						State                      corebattle.LifetimeState
						Target                     corebattle.TargetContext
						Entry                      string
						Arguments                  []string
						MPHealCalls, StatDownCalls int
						End                        corebattle.EndProcessing
						Cursor                     corebattle.TargetCursor
					}
					data, err := json.Marshal(checkpoint{s, c.TargetContext, entry.ID, args, c.MPHealCalls, c.StatDownCalls, c.End, c.Cursor})
					if err != nil {
						t.Fatal(err)
					}
					var loaded checkpoint
					if err := json.Unmarshal(data, &loaded); err != nil {
						t.Fatal(err)
					}
					_, resumed, rebuilt := rangedFixture()
					resumed.TargetContext, resumed.MPHealCalls, resumed.StatDownCalls, resumed.End, resumed.Cursor = loaded.Target, loaded.MPHealCalls, loaded.StatDownCalls, loaded.End, loaded.Cursor
					rngs := []random.Generator{random.NewDeterministic(seed), random.NewDeterministic(seed)}
					for i, run := range []struct {
						s *corebattle.LifetimeState
						c *corebattle.RangedContext
						d BattleDefinitions
					}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
						f := corebattle.CallbackContext{TargetContext: loaded.Target, Arguments: loaded.Arguments}
						ops, err := run.d.InvocationAction(run.s, &f, []string{loaded.Entry}, noArithmeticDraws{t})
						if err != nil {
							t.Fatal(err)
						}
						if err := run.s.ApplyInvocations(&f, run.d.Invocations, ops, rngs[i]); err != nil {
							t.Fatal(err)
						}
					}
					// Ports are reconstructed functions; compare all data facts separately.
					left, right := *c, *resumed
					left.Reset, left.Defeat, right.Reset, right.Defeat = nil, nil, nil, nil
					left.Status.Defeat, right.Status.Defeat = nil, nil
					if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(left, right) || rngs[0].Float64() != rngs[1].Float64() {
						t.Fatal(s, loaded.State, left, right)
					}
					if (entry.ID == "job-11-entry-2" || entry.ID == "job-54-entry-2") && (s.Actors[0].MP != 94 || s.Actors[1].MP != 85 || c.MPHealCalls != 1 || c.StatDownCalls != 1) {
						t.Fatal("committed theft replayed", s, c)
					}
				})
			}
		}
	}
}
