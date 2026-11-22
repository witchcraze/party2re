package adventure_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func mpEnemy(id string, hp, mp int) corebattle.LifetimeActor {
	e := runEnemy(id, hp)
	e.MP, e.MaxMP = mp, mp
	return e
}

// accepted runs cmd, then checks the result survives reload and replays from
// its receipt without executing again.
func accepted(t *testing.T, f *normalFixture, cmd adventure.RunCommand) *adventure.NormalRun {
	t.Helper()
	receipt, err := f.svc.Command(t.Context(), cmd)
	if err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	f.restart(t)
	if again, err := f.svc.Command(t.Context(), cmd); err != nil || !reflect.DeepEqual(again, receipt) {
		t.Fatalf("replay: %+v %v", again, err)
	}
	if replayed := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(replayed, r) {
		t.Fatal("replay re-executed the operation")
	}
	return r
}

// rejected checks a whole-action rejection stores neither run state nor receipt.
func rejected(t *testing.T, f *normalFixture, cmd adventure.RunCommand) {
	t.Helper()
	before := readStockRoom(t, f, cmd.RunID)
	if _, err := f.svc.Command(t.Context(), cmd); err == nil {
		t.Fatal("accepted")
	}
	if got, err := f.svc.GetRunReceipt(t.Context(), cmd.PlayerID, cmd.ActorID, cmd.RunID, cmd.ID); err != nil || got != nil {
		t.Fatalf("stored receipt: %+v %v", got, err)
	}
	if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
		t.Fatal("rejected operation mutated the run")
	}
}

// _skill.cgi skill_58 SP110: _st_down(0.4, magic, mp) → int(100×0.4×tension 1).
func TestRunCombatControlMPDrain(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := customRoom(t, f, 0, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Jobs = corebattle.BattleJobs{JobID: "job-58", SP: 110}
		a.MP, a.MaxMP = 50, 50
	}, mpEnemy("foe", 900, 100))
	r := accepted(t, f, combatCommand(cmd, "drain", "job-58-entry-3", "foe", ""))
	if foe, a := runActorByID(r, "foe"), runActorByID(r, cmd.ActorID); foe.MP != 60 || a.MP != 44 {
		t.Fatalf("foe mp=%d actor mp=%d", foe.MP, a.MP)
	}
}

// _skill.cgi skill_79 form branch: _damages(150, breath, direct) on every
// enemy in order, then the winged form; an invalid requester gender gates the
// action.
func TestRunCombatTransformationBreath(t *testing.T) {
	for _, gender := range []string{"", "m"} {
		f := combatFixture(t, 1)
		f.actors[0].Gender = gender
		cmd := customRoom(t, f, 0, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.Jobs = corebattle.BattleJobs{JobID: "job-79", SP: 77}
			a.MP, a.MaxMP = 50, 50
		}, runEnemy("foe", 900), runEnemy("other", 900))
		breath := combatCommand(cmd, "breath", "job-79-breath", "", "")
		if gender == "" {
			rejected(t, f, breath)
			continue
		}
		icon := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID).Icon
		r := accepted(t, f, breath)
		foe, other, a := runActorByID(r, "foe"), runActorByID(r, "other"), runActorByID(r, cmd.ActorID)
		// Direct ×(0.9+rand .3) at .5, and ×0.85 per later position: 157, 133.
		if foe.HP != 743 || other.HP != 767 || a.MP != 29 || a.Icon == icon {
			t.Fatalf("foe=%d other=%d mp=%d icon %q→%q", foe.HP, other.HP, a.MP, icon, a.Icon)
		}
	}
}

// _skill.cgi:5450–5581: the divisor wrapper runs the last discovered entry
// named by its ＠× child on every member whose MP divides; an unknown name is
// a failed cast and a non-arithmetic last match gates the whole action.
func TestRunCombatArithmeticSelector(t *testing.T) {
	room := func(t *testing.T, f *normalFixture) adventure.RunCommand {
		return customRoom(t, f, 9, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.Jobs = corebattle.BattleJobs{JobID: "job-72", SP: 150}
			a.MP, a.MaxMP = 50, 50
		}, mpEnemy("foe", 900, 8), mpEnemy("other", 900, 9))
	}
	t.Run("reflect", func(t *testing.T) {
		f := combatFixture(t, 1)
		cmd := room(t, f)
		r := accepted(t, f, combatCommand(cmd, "reflect", "", "", "＠ＭＰ４ ＠×リフレク"))
		if runActorByID(r, "foe").Temporary != corebattle.TemporaryReflectMagic || runActorByID(r, "other").Temporary != "" || runActorByID(r, cmd.ActorID).MP != 36 {
			t.Fatalf("foe=%q other=%q mp=%d", runActorByID(r, "foe").Temporary, runActorByID(r, "other").Temporary, runActorByID(r, cmd.ActorID).MP)
		}
	})
	t.Run("no match", func(t *testing.T) {
		f := combatFixture(t, 1)
		cmd := room(t, f)
		r := accepted(t, f, combatCommand(cmd, "miss", "", "", "＠ＭＰ４ ＠×なし"))
		if runActorByID(r, "foe").Temporary != "" || runActorByID(r, cmd.ActorID).MP != 36 {
			t.Fatalf("foe=%q mp=%d", runActorByID(r, "foe").Temporary, runActorByID(r, cmd.ActorID).MP)
		}
	})
	t.Run("custom last match", func(t *testing.T) {
		f := combatFixture(t, 1)
		saveCustom(t, f, "×ぎじ", [3]string{"gem_atk_1", "", ""}, 6)
		rejected(t, f, combatCommand(room(t, f), "custom", "", "", "＠ＭＰ４ ＠×ぎじ"))
	})
}

// _skill.cgi:2653–2713: job31's fixed divisors quarter or defeat members at
// HP ≤ 999 whose MP divides, through the shared defeat owner.
func TestRunCombatArithmeticJob31(t *testing.T) {
	for _, tc := range []struct {
		id      string
		foeMP   int
		alive   bool
		foeHP   int
		otherHP int
	}{
		{"job-31-gravity", 8, true, 225, 999},
		{"job-31-death", 10, false, 0, 999},
	} {
		t.Run(tc.id, func(t *testing.T) {
			f := combatFixture(t, 1)
			cmd := customRoom(t, f, 0, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
				a.Jobs = corebattle.BattleJobs{JobID: "job-31", SP: 155}
				a.MP, a.MaxMP = 51, 51
			}, mpEnemy("foe", 900, tc.foeMP), mpEnemy("other", 999, 9))
			r := accepted(t, f, combatCommand(cmd, tc.id, tc.id, "", ""))
			foe := runActorByID(r, "foe")
			if (foe != nil) != tc.alive || foe != nil && foe.HP != tc.foeHP || runActorByID(r, "other").HP != tc.otherHP {
				t.Fatalf("foe=%+v other=%d", foe, runActorByID(r, "other").HP)
			}
		})
	}
}

// _skill.cgi:2689–2712 (extended via item249): the supplied participant's
// two/three HP digits reverse directly, without a MaxHP clamp.
func TestRunCombatArithmeticDigitReversal(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := customRoom(t, f, 0, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Jobs = corebattle.BattleJobs{JobID: "job-31", SP: 155}
		a.MP, a.MaxMP, a.ItemDefinitionIDs = 99, 99, []string{"item-249"}
	}, runEnemy("foe", 123))
	r := accepted(t, f, combatCommand(cmd, "reverse", "job-31-reverse", "foe", ""))
	if foe := runActorByID(r, "foe"); foe.HP != 321 {
		t.Fatalf("foe hp=%d", foe.HP)
	}
}

// _skill.cgi:5459–5571: `$_mp_num` is a request global, so the NPC answering
// the same command (_battle.cgi:1170) runs a direct job72 effect with the
// player's divisor; the next command starts without one.
func TestRunCombatArithmeticDivisorReachesSameCommandResponse(t *testing.T) {
	f := combatFixture(t, 1)
	foe := mpEnemy("foe", 900, 8)
	// Pool [normal attack, ×フレア, ×フレア]: midRNG draws position 1.
	foe.Jobs = corebattle.BattleJobs{JobID: "job-72", SP: 20, OldJobID: "job-72", OldSP: 20}
	cmd := combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, f.actors[0].ID)
		a.Jobs = corebattle.BattleJobs{JobID: "job-72", SP: 150}
		a.MP, a.MaxMP = 50, 50
	}, foe)
	// 273 = int(260×(0.9+0.3×.5)) on every member with MP%4 == 0: the actor
	// after its 14 MP cost (36) and the foe (8).
	r := accepted(t, f, combatCommand(cmd, "reflect", "", "", "＠ＭＰ４ ＠×リフレク"))
	if a, foe := runActorByID(r, cmd.ActorID), runActorByID(r, "foe"); a.MP != 36 || a.HP != 227 || foe.HP != 627 {
		t.Fatalf("carried: actor hp=%d mp=%d foe hp=%d", a.HP, a.MP, foe.HP)
	}
	f.clock.add(time.Hour)
	cmd.Version = r.Version
	r = accepted(t, f, combatCommand(cmd, "strike", "attack", "foe", ""))
	if a := runActorByID(r, cmd.ActorID); a.HP != 227 || !r.LastActions["foe"].Equal(f.clock.Now()) {
		t.Fatalf("next command: actor hp=%d foe acted=%v", a.HP, r.LastActions["foe"])
	}
}
