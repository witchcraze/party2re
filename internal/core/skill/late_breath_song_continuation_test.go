package skill

import (
	"encoding/json"
	"fmt"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

func TestLateSongReloadedContinuation(t *testing.T) {
	for _, job := range []int{84, 85, 86} {
		for _, entry := range corebattle.LateBreathSongEntries(job) {
			_, _, definitions := lateSongFixture()
			if _, ok := definitions.Invocations.Entries[entry.ID]; !ok {
				continue
			}
			for _, seed := range []uint64{1476, 20261029, 84} {
				t.Run(fmt.Sprintf("%s/seed-%d", entry.ID, seed), func(t *testing.T) {
					s, c, d := lateSongFixture()
					s.Actors[0].Icon = "assets/images/placeholders/battle/forms.svg#job-84-ex-m"
					c.BurnStrike = s.Actors[0].LateBreathSongBurn("burn", "m")
					c.HeldBooks["self"] = "forbidden"
					// A completed ritual precedes the interruption. Replaying it
					// would debit HP again; only the frozen next command runs.
					executeLateSong(t, &s, c, d, "job-85-entry-0", nil, 0, 0)
					s.Actors[1].HP = 900
					args := []string{"enemy", "tail"}
					switch seed {
					case 1476:
						s.Actors[1].Status = corebattle.StatusSabaku
					case 20261029:
						s.Actors[0].Status = corebattle.StatusConfusion
						args[0] = "fallen"
					case 84:
						s.Actors[0].Status = corebattle.StatusDanceSeal
						s.Actors[1].Temporary = corebattle.TemporaryNullMagic
					}
					type checkpoint struct {
						State       corebattle.LifetimeState
						Target      corebattle.TargetContext
						Entry       string
						Arguments   []string
						Burn        bool
						Books       map[string]string
						MPHealCalls int
					}
					input := checkpoint{s, c.TargetContext, entry.ID, args, c.BurnStrike, c.HeldBooks, c.MPHealCalls}
					data, err := json.Marshal(input)
					if err != nil {
						t.Fatal(err)
					}
					var loaded checkpoint
					if err := json.Unmarshal(data, &loaded); err != nil {
						t.Fatal(err)
					}
					_, resumed, rebuilt := lateSongFixture()
					resumed.TargetContext, resumed.BurnStrike, resumed.HeldBooks, resumed.MPHealCalls = loaded.Target, loaded.Burn, loaded.Books, loaded.MPHealCalls
					rngs := []random.Generator{random.NewDeterministic(seed), random.NewDeterministic(seed)}
					for i, run := range []struct {
						s *corebattle.LifetimeState
						c *corebattle.LateBreathSongContext
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
					if !reflect.DeepEqual(s, loaded.State) || !reflect.DeepEqual(c.TargetContext, resumed.TargetContext) || !reflect.DeepEqual(c.Arguments, resumed.Arguments) || c.DamageCalls != resumed.DamageCalls || c.TensionCalls != resumed.TensionCalls || c.MPHealCalls != resumed.MPHealCalls || c.End != resumed.End || c.Death.DeathCalls != resumed.Death.DeathCalls || rngs[0].Float64() != rngs[1].Float64() {
						t.Fatal(s, loaded.State, c, resumed)
					}
					if (entry.ID == "job-84-entry-0" || entry.ID == "job-84-entry-3" || entry.ID == "job-85-entry-2" || entry.ID == "job-85-entry-3") && s.Actors[0].HP != 230 {
						t.Fatal("completed ritual replayed", s)
					}
				})
			}
		}
	}
}
