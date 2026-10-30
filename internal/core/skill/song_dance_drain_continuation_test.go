package skill

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestSongDanceDrainReloadedContinuation(t *testing.T) {
	for _, job := range []int{13, 14, 20} {
		for _, entry := range corebattle.SongDanceDrainEntries(job) {
			if entry.ID == "job-14-entry-4" {
				continue
			}
			for _, seed := range []uint64{1460, 13, 20} {
				t.Run(fmt.Sprintf("%s/seed-%d", entry.ID, seed), func(t *testing.T) {
					s, c, d := songDanceFixture()
					// Complete a real drain before interruption. Rebuild the next
					// operation from facts; replay would damage/heal/charge again.
					executeSongDance(t, &s, c, d, "job-20-entry-1", []string{"enemy"}, 0, 0)
					args := []string{"enemy", "tail"}
					switch seed {
					case 1460:
						s.Actors[0].Status = corebattle.StatusDanceSeal
					case 13:
						s.Actors[0].Status = corebattle.StatusConfusion
						args[0] = "fallen"
					case 20:
						s.Actors[0].Status = corebattle.StatusPhysicalSeal
						s.Actors[1].Temporary = corebattle.TemporaryOneHitDefense
					}
					type checkpoint struct {
						State                    corebattle.LifetimeState
						Target                   corebattle.TargetContext
						Entry                    string
						Arguments                []string
						DamageCalls, MPHealCalls int
						End                      corebattle.EndProcessing
						Cursor                   corebattle.TargetCursor
					}
					input := checkpoint{s, c.TargetContext, entry.ID, args, c.DamageCalls, c.MPHealCalls, c.End, c.Cursor}
					data, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					var loaded checkpoint
					if err := json.Unmarshal(data, &loaded); err != nil {
						t.Fatal(err)
					}
					_, resumed, rebuilt := songDanceFixture()
					resumed.TargetContext, resumed.DamageCalls, resumed.MPHealCalls, resumed.End, resumed.Cursor = loaded.Target, loaded.DamageCalls, loaded.MPHealCalls, loaded.End, loaded.Cursor
					rngs := []random.Generator{random.NewDeterministic(seed), random.NewDeterministic(seed)}
					for i, run := range []struct {
						s *corebattle.LifetimeState
						c *corebattle.SongDanceDrainContext
						d BattleDefinitions
					}{{&s, c, d}, {&loaded.State, resumed, rebuilt}} {
						f := corebattle.CallbackContext{TargetContext: run.c.TargetContext, Arguments: loaded.Arguments}
						ops, err := run.d.InvocationAction(run.s, &f, []string{loaded.Entry}, noArithmeticDraws{t})
						if err != nil {
							t.Fatal(err)
						}
						if err := run.s.ApplyInvocations(&f, run.d.Invocations, ops, rngs[i]); err != nil {
							t.Fatal(err)
						}
					}
					if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(c.EffectContext, resumed.EffectContext) || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || c.End != resumed.End || c.Cursor != resumed.Cursor || c.Status.StatusCalls != resumed.Status.StatusCalls || c.Status.DeathCalls != resumed.Status.DeathCalls || c.Status.CureCalls != resumed.Status.CureCalls || c.Status.AreaStatusCalls != resumed.Status.AreaStatusCalls || c.Status.PartyCureCalls != resumed.Status.PartyCureCalls || c.Status.AreaDeathCalls != resumed.Status.AreaDeathCalls || rngs[0].Float64() != rngs[1].Float64() {
						t.Fatal(s, loaded.State, c, resumed)
					}
					if (entry.ID == "job-14-entry-2" || entry.ID == "job-20-entry-2") && (s.Actors[0].MP != 84 || s.Actors[1].HP != 860 || c.DamageCalls != 1 || c.MPHealCalls != 1) {
						t.Fatal("committed drain replayed", s, c)
					}
				})
			}
		}
	}
}
