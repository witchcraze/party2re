package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/runhost"
	"github.com/witchcraze/party2re/internal/testutil"
)

type stockRunRNG struct{ normalRNG }

func (r *stockRunRNG) Float64() float64 { r.calls.Add(1); return .99 }

func stockRun(t *testing.T, level, exp int, over bool) (*normalFixture, corecharacter.Character, runhost.Command, *stockRunRNG) {
	t.Helper()
	f := newNormalFixture(t, 1)
	jobs, err := job.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	rng := &stockRunRNG{}
	f.cfg.Combat, f.cfg.Jobs, f.cfg.RNG = nil, jobs, rng
	f.restart(t)
	c := f.actors[0]
	c.Level, c.Experience, c.OverLevel, c.SP = level, exp, over, 30
	c.JobID, c.OldJobID, c.OldSP = "job-01", "job-05", 12
	c.Stats = corecharacter.Stats{HP: 41, MP: 23, MaxHP: 998, MaxMP: 998, Attack: 254, Defense: 254, Agility: 254}
	if err := f.chars.Update(t.Context(), c); err != nil {
		t.Fatal(err)
	}
	entry := f.enter(t, adventure.CrawlModeNormal)
	cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: entry.Command.RunID, ID: "stock", Action: "stock-level", Version: entry.Version}
	return f, c, cmd, rng
}

func editStockRoom(t *testing.T, f *normalFixture, cmd *runhost.Command, edit func(*adventure.NormalRun)) *adventure.NormalRun {
	t.Helper()
	if err := f.store.WithRunForUpdate(t.Context(), cmd.RunID, false, func(ctx context.Context) error {
		r, err := f.store.FindRun(ctx, cmd.RunID)
		if err != nil {
			return err
		}
		edit(r)
		r.Version++
		return f.store.SaveRun(ctx, *r, cmd.Version)
	}); err != nil {
		t.Fatal(err)
	}
	cmd.Version++
	return readStockRoom(t, f, cmd.RunID)
}

func readStockRoom(t *testing.T, f *normalFixture, runID string) *adventure.NormalRun {
	t.Helper()
	r, err := f.store.FindRun(t.Context(), runID)
	if err != nil || r == nil {
		t.Fatal(r, err)
	}
	return r
}

func TestNormalStockLevelPersistsAndReplays(t *testing.T) {
	for _, tc := range []struct {
		name                                  string
		level, exp, wantLevel                 int
		over                                  bool
		wantDurableHP, wantRoomHP, wantRoomAT int
	}{
		{"ordinary cap", 98, 100000, 99, false, 999, 1005, 257},
		{"crossing 99 clamps only the room at the intermediate level", 98, 98010, 100, true, 1012, 1006, 258},
		{"over-level cap", 149, 222010, 150, true, 1005, 1005, 257},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, cmd, rng := stockRun(t, tc.level, tc.exp, tc.over)
			before := editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
				a := &r.Battle.Actors[0]
				a.Jobs.JobID, a.Jobs.SP, a.Attack, a.Defense, a.Agility = "job-06", 6, 378, 419, 286
				a.Extended, a.ExtendedTurns = corebattle.ExtendedIncense, 4
				r.LastActions[c.ID] = f.clock.Now()
				r.Round = 1
				r.Enemies = []string{"fallen-foe"}
				r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{NPC: true, Participant: corebattle.Participant{ID: "fallen-foe", TeamID: "foes", MaxHP: 30}})
			})
			f.clock.add(3 * time.Second)
			f.store.failWrite.Store(true)
			if _, err := f.svc.Command(t.Context(), cmd); err == nil {
				t.Fatal("receipt write should fail")
			}
			f.store.failWrite.Store(false)
			assertStockUnchanged(t, f, cmd, c, before)
			f.store.loseCommit.Store(true)
			receipt, err := f.svc.Command(t.Context(), cmd)
			if err != nil {
				t.Fatal(err)
			}
			draws := rng.calls.Load()
			f.restart(t)
			errs := testutil.RunRace(func() error {
				got, err := f.svc.Command(t.Context(), cmd)
				if err == nil && !reflect.DeepEqual(receipt, got) {
					return errors.New("replay returned a different receipt")
				}
				return err
			}, func() error {
				_, err := f.svc.Command(t.Context(), cmd)
				return err
			})
			for _, err := range errs {
				if err != nil {
					t.Fatal(err)
				}
			}
			if rng.calls.Load() != draws {
				t.Fatal("committed request rerolled")
			}
			r := readStockRoom(t, f, cmd.RunID)
			a := r.Battle.Actors[0]
			got, err := f.chars.FindByID(t.Context(), c.ID)
			levels := tc.wantLevel - tc.level
			wantStats := corecharacter.Stats{HP: 41, MP: 23, MaxHP: tc.wantDurableHP, MaxMP: 998 + levels, Attack: 254 + levels*3, Defense: 254 + levels*5, Agility: 254 + levels*2}
			if !tc.over {
				wantStats = corecharacter.Stats{HP: 41, MP: 23, MaxHP: 999, MaxMP: 999, Attack: 255, Defense: 255, Agility: 255}
			}
			if err != nil || got.Level != tc.wantLevel || got.SP != c.SP+levels || got.Stats != wantStats || got.Experience != c.Experience || got.JobID != c.JobID || got.OldJobID != c.OldJobID || got.OldSP != c.OldSP {
				t.Fatalf("unexpected saved progression: level=%d exp=%d sp=%d stats=%+v err=%v", got.Level, got.Experience, got.SP, got.Stats, err)
			}
			wantJobs := corebattle.BattleJobs{JobID: "job-06", SP: 6 + levels, OldJobID: c.OldJobID, OldSP: c.OldSP}
			if a.Jobs != wantJobs || a.MaxHP != tc.wantRoomHP || a.MaxMP != 998+levels || a.BaseStats.Attack != tc.wantRoomAT ||
				a.Attack != 378 || a.Defense != 419 || a.Agility != 286 || a.HP != 41 || a.MP != 23 || a.ExtendedTurns != 4 || a.CMP != before.Battle.Actors[0].CMP {
				t.Fatalf("unexpected room projection: %+v", a)
			}
			if r.Version != cmd.Version+1 || receipt.Version != r.Version || !r.LastActivity.Equal(f.clock.Now()) || !r.LastActions[c.ID].Equal(f.clock.Now()) {
				t.Fatalf("unexpected receipt, version or activity: %+v", receipt)
			}
		})
	}
}

func TestNormalStockLevelCommandGates(t *testing.T) {
	for _, tc := range []struct {
		name       string
		level, exp int
		over       bool
		edit       func(*adventure.NormalRun)
		want       error
	}{
		{"below threshold", 1, 9, false, nil, runhost.ErrUnavailable},
		{"ordinary cap", 99, 100000, false, nil, runhost.ErrUnavailable},
		{"over-level cap", 150, 225000, true, nil, runhost.ErrUnavailable},
		{"fallen", 1, 10, false, func(r *adventure.NormalRun) { r.Battle.Actors[0].HP = 0 }, adventure.ErrUnconsciousCharacter},
		{"enemies", 1, 10, false, func(r *adventure.NormalRun) {
			r.Round, r.Enemies = 1, []string{"foe"}
			r.Battle.Actors = append(r.Battle.Actors, corebattle.LifetimeActor{NPC: true, Participant: corebattle.Participant{ID: "foe", TeamID: "foes", HP: 30, MaxHP: 30}})
		}, runhost.ErrUnavailable},
		{"unopened chest", 1, 10, false, func(r *adventure.NormalRun) {
			r.Round, r.Boxes = adventure.TreasureRoomFloor, []runhost.TreasureBox{{ID: "chest"}}
		}, runhost.ErrUnavailable},
		{"cooldown", 1, 10, false, nil, runhost.ErrCooldown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, c, cmd, rng := stockRun(t, tc.level, tc.exp, tc.over)
			before := readStockRoom(t, f, cmd.RunID)
			if tc.edit != nil {
				before = editStockRoom(t, f, &cmd, tc.edit)
			}
			if tc.want == runhost.ErrCooldown {
				before = editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
					r.LastActions[c.ID] = f.clock.Now()
				})
			} else {
				f.clock.add(3 * time.Second)
			}
			draws := rng.calls.Load()
			if _, err := f.svc.Command(t.Context(), cmd); !errors.Is(err, tc.want) || rng.calls.Load() != draws {
				t.Fatalf("got %v, want %v without draws", err, tc.want)
			}
			assertStockUnchanged(t, f, cmd, c, before)
		})
	}
}

func TestNormalStockLevelIncapacitationUsesOrdinaryTail(t *testing.T) {
	f, c, cmd, _ := stockRun(t, 1, 40, false)
	editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
		r.Battle.Actors[0].Status = corebattle.StatusDofuu
		r.Battle.Fields.AntiTurns = 2
	})
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	got, err := f.chars.FindByID(t.Context(), c.ID)
	if err != nil || got.Level != 2 || got.SP != c.SP+1 || r.Battle.Actors[0].Status != "" || r.Battle.Fields.AntiTurns != 1 {
		t.Fatalf("ordinary tail did not run: level=%d SP=%d battle=%+v err=%v", got.Level, got.SP, r.Battle, err)
	}
}

func assertStockUnchanged(t *testing.T, f *normalFixture, cmd runhost.Command, c corecharacter.Character, before *adventure.NormalRun) {
	t.Helper()
	got, err := f.chars.FindByID(t.Context(), c.ID)
	if err != nil || got.Stats != c.Stats || got.Level != c.Level || got.SP != c.SP || got.Experience != c.Experience {
		t.Fatalf("stored character changed: %+v, %v", got, err)
	}
	if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
		t.Fatalf("stored room changed: %+v", after)
	}
	if receipt, err := f.store.FindRunReceipt(t.Context(), cmd.RunID, cmd.ID); err != nil || receipt != nil {
		t.Fatalf("unexpected receipt: %+v, %v", receipt, err)
	}
}
