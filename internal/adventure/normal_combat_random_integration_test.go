package adventure_test

import (
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// scriptRNG scripts the Float64 draws that follow the operation's weapon-seal
// table and action-start draws; later draws fall back to midRNG.
type scriptRNG struct {
	midRNG
	draws []float64
}

func (r *scriptRNG) Float64() float64 {
	r.calls.Add(1)
	if len(r.draws) == 0 {
		return .5
	}
	v := r.draws[0]
	r.draws = r.draws[1:]
	return v
}

// randomDraw selects job then position in an n-entry table in the general
// random helper (_data.cgi:1260–1264), after the two operation-start draws.
func randomDraw(job, position, n int) []float64 {
	return []float64{.5, .5, (float64(job) - .5) / 87, (float64(position) + .5) / float64(n)}
}

// randomFixture serves Command with the production RunCombat and asset
// session, as cmd/party2 does; every random pool entry is then bound.
func randomFixture(t *testing.T, draws ...float64) *normalFixture {
	t.Helper()
	f := newNormalFixture(t, 1)
	f.cfg.RNG = &scriptRNG{draws: draws}
	withAssetSession(t, f)
	installRunCombat(t, f)
	return f
}

// encounterEnemy carries the encounter identity real stage enemies have, so
// room-travel descendants (job78.2) admit as in production.
func encounterEnemy(id string, hp int) corebattle.LifetimeActor {
	e := runEnemy(id, hp)
	e.EncounterIdentity = id + "-identity"
	return e
}

func metronomeRoom(t *testing.T, f *normalFixture, edit func(*adventure.NormalRun, *corebattle.LifetimeActor), enemies ...corebattle.LifetimeActor) adventure.RunCommand {
	t.Helper()
	saveCustom(t, f, "ゆび", [3]string{"gem_metronome", "", ""}, 5)
	return customRoom(t, f, 30, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.MP, a.MaxMP = 50, 50
		if edit != nil {
			edit(r, a)
		}
	}, enemies...)
}

// _skill.cgi:1168–1181 via the random helper (_skill.cgi:9773–9787): the
// sacrifice heals other members fully, defeats the actor through the shared
// defeat owner and sets MP1; the wrapper then refunds the actor's MP decrease.
func TestRunCombatRandomSacrificeRefundsMPDecrease(t *testing.T) {
	f := randomFixture(t, randomDraw(3, 6, 8)...)
	cmd := metronomeRoom(t, f, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
		withAlly(1, "")(r, a)
		runActorByID(r, "ally").HP = 100
	}, encounterEnemy("foe", 900))
	r := accepted(t, f, combatCommand(cmd, "sacrifice", "ゆび", "foe", ""))
	a, ally := runActorByID(r, cmd.ActorID), runActorByID(r, "ally")
	if a.HP != 0 || a.MP != 50 || a.CMP != 25 || ally.HP != ally.MaxHP {
		t.Fatalf("actor hp=%d mp=%d cmp=%d ally hp=%d/%d", a.HP, a.MP, a.CMP, ally.HP, ally.MaxHP)
	}
}

// _skill.cgi:1347–1353: job7.8's area instant death (untyped, 60 ≥ draw .5)
// runs through the status owner before the actor's own defeat.
func TestRunCombatRandomDeathSacrifice(t *testing.T) {
	f := randomFixture(t, randomDraw(7, 8, 9)...)
	cmd := metronomeRoom(t, f, withAlly(1, ""), encounterEnemy("foe", 900), encounterEnemy("other", 900))
	r := accepted(t, f, combatCommand(cmd, "self-destruct", "ゆび", "foe", ""))
	if a := runActorByID(r, cmd.ActorID); a.HP != 0 || a.MP != 50 || len(r.Enemies) != 0 {
		t.Fatalf("actor hp=%d mp=%d enemies=%v", a.HP, a.MP, r.Enemies)
	}
}

// _skill.cgi:5776 (job75.4) converts HP into MP; the random wrapper
// keeps an increase (it restores only a decrease).
func TestRunCombatRandomKeepsMPIncrease(t *testing.T) {
	f := randomFixture(t, randomDraw(75, 4, 7)...)
	cmd := metronomeRoom(t, f, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.MP = 10
	}, encounterEnemy("foe", 900))
	r := accepted(t, f, combatCommand(cmd, "mana", "ゆび", "", ""))
	if a := runActorByID(r, cmd.ActorID); a.MP <= 10 || a.HP >= 500 {
		t.Fatalf("actor hp=%d mp=%d", a.HP, a.MP)
	}
}

// Table variants are chosen against the actor's state when the random helper
// builds the table (77.4: temporary; 79.3: winged form; 84.2: burn strike).
func TestRunCombatRandomSelectsTableVariants(t *testing.T) {
	t.Run("77.4 releases the temporary", func(t *testing.T) {
		for _, temp := range []corebattle.TemporaryEffect{"", corebattle.TemporaryDouble} {
			f := randomFixture(t, randomDraw(77, 4, 5)...)
			cmd := metronomeRoom(t, f, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
				a.Temporary = temp
			}, encounterEnemy("foe", 90000))
			r := accepted(t, f, combatCommand(cmd, "strike", "ゆび", "foe", ""))
			if a := runActorByID(r, cmd.ActorID); a.Temporary != "" || runActorByID(r, "foe").HP >= 90000 {
				t.Fatalf("%q: temporary=%q foe=%d", temp, a.Temporary, runActorByID(r, "foe").HP)
			}
		}
	})
	t.Run("79.3 follows the form", func(t *testing.T) {
		f := randomFixture(t, randomDraw(79, 3, 5)...)
		cmd := metronomeRoom(t, f, nil, encounterEnemy("foe", 900))
		icon := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID).Icon
		r := accepted(t, f, combatCommand(cmd, "wing", "ゆび", "", ""))
		if winged := runActorByID(r, cmd.ActorID).Icon; winged == icon || runActorByID(r, "foe").HP >= 900 {
			t.Fatalf("breath: icon %q→%q foe=%d", icon, winged, runActorByID(r, "foe").HP)
		}
		f.cfg.RNG.(*scriptRNG).draws = randomDraw(79, 3, 5)
		f.clock.add(time.Hour)
		cmd.Version = r.Version
		foe, agility := runActorByID(r, "foe").HP, runActorByID(r, cmd.ActorID).Agility
		r = accepted(t, f, combatCommand(cmd, "unwing", "ゆび", "", ""))
		if a := runActorByID(r, cmd.ActorID); a.Icon != icon || a.Agility <= agility || runActorByID(r, "foe").HP != foe {
			t.Fatalf("speed: icon %q want %q agility %d→%d foe %d→%d", a.Icon, icon, agility, a.Agility, foe, runActorByID(r, "foe").HP)
		}
	})
	t.Run("84.2 burn strike", func(t *testing.T) {
		damage := map[bool]int{}
		for _, burn := range []bool{false, true} {
			f := randomFixture(t, randomDraw(84, 2, 5)...)
			cmd := metronomeRoom(t, f, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
				if burn {
					r.Battle.Fields = corebattle.LifetimeFields{Temporary: "burn", Turns: 5}
					a.Icon = "assets/images/placeholders/battle/forms.svg#job-84-ex-" + f.actors[0].Gender
				}
			}, encounterEnemy("foe", 90000))
			r := accepted(t, f, combatCommand(cmd, "burn", "ゆび", "foe", ""))
			damage[burn] = 90000 - runActorByID(r, "foe").HP
		}
		// AT×2.5 versus AT×2 on the forwarded custom target.
		if damage[false] <= 0 || damage[true] <= damage[false] {
			t.Fatalf("damage %v", damage)
		}
	})
}

// _skill.cgi:5583–5588: job73 reads the table at (AT+DF+AG)%73, with zero
// remapped to job8, from the battle actor's current stats. The full residue
// mapping is exhausted against the same catalog in core/skill.
func TestRunCombatRandomJob73Proxy(t *testing.T) {
	t.Run("residue 3", func(t *testing.T) {
		f := randomFixture(t, randomDraw(73, 2, 8)...)
		cmd := metronomeRoom(t, f, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.Attack, a.Defense, a.Agility, a.BaseStats.Defense = 3, 73, 0, 73
		}, encounterEnemy("foe", 900))
		r := accepted(t, f, combatCommand(cmd, "guard", "ゆび", "", ""))
		if a := runActorByID(r, cmd.ActorID); a.Defense <= 73 || a.Status != "" {
			t.Fatalf("job3.2: df=%d status=%q", a.Defense, a.Status)
		}
	})
	t.Run("residue 0", func(t *testing.T) {
		f := randomFixture(t, randomDraw(73, 1, 9)...)
		cmd := metronomeRoom(t, f, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.Attack, a.Defense, a.Agility, a.HP = 73, 73, 0, 100
		}, encounterEnemy("foe", 900))
		r := accepted(t, f, combatCommand(cmd, "sleep", "ゆび", "", ""))
		if a := runActorByID(r, cmd.ActorID); a.Status != corebattle.StatusSleep || a.HP <= 100 || a.Defense != 73 {
			t.Fatalf("job8.1: status=%q hp=%d df=%d", a.Status, a.HP, a.Defense)
		}
	})
}

// _skill.cgi:6537–6550: job82's generated entry selects table and position by
// the same sum (AT+DF+AG+member count): 90 → job3, position 90%8 = 2. The
// consumer charges the captured child MP.
func TestRunCombatJob82GeneratedEntry(t *testing.T) {
	f := randomFixture(t)
	cmd := customRoom(t, f, 0, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Jobs = corebattle.BattleJobs{JobID: "job-82", SP: 10}
		a.Attack, a.Defense, a.Agility, a.BaseStats.Defense = 48, 40, 0, 40
		a.MP, a.MaxMP = 50, 50
	}, encounterEnemy("foe", 900))
	r := accepted(t, f, combatCommand(cmd, "generated", "job-82-entry-0", "", ""))
	if a := runActorByID(r, cmd.ActorID); a.Defense <= 40 || a.MP != 48 {
		t.Fatalf("df=%d mp=%d", a.Defense, a.MP)
	}
}

// _skill.cgi:5873–5874: job76.5's listed cost 12×party size is fixed when the
// run builds the table for this operation.
func TestRunCombatGroupDanceCostFollowsPartySize(t *testing.T) {
	for _, tc := range []struct {
		ally bool
		mp   int
	}{{false, 38}, {true, 26}} {
		f := randomFixture(t)
		cmd := customRoom(t, f, 0, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.Jobs = corebattle.BattleJobs{JobID: "job-76", SP: 130}
			a.MP, a.MaxMP = 50, 50
			if tc.ally {
				withAlly(1, "")(r, a)
			}
		}, encounterEnemy("foe", 900))
		r := accepted(t, f, combatCommand(cmd, "dance", "job-76-entry-5", "", ""))
		if a := runActorByID(r, cmd.ActorID); a.MP != tc.mp {
			t.Fatalf("ally=%v mp=%d want %d", tc.ally, a.MP, tc.mp)
		}
	}
}

// _skill.cgi:10841–10877: gem86 tries only the current table's last entry; a
// last extended entry that SP/MP cannot run is handled, without fallback.
// Job73 always reports handled (MP0 included). Only an unhandled current job
// falls back to the previous job's last extended entry (job4.8 here).
func TestRunCombatGem86HandledCurrent(t *testing.T) {
	for _, tc := range []struct {
		name    string
		current corebattle.BattleJobs
		mp      int
		attack  int
		wantMP  int
	}{
		{"fallback", corebattle.BattleJobs{JobID: "job-03", SP: 80}, 50, 140, 40},
		{"unaffordable current", corebattle.BattleJobs{JobID: "job-18", SP: 120}, 50, 100, 50},
		{"job73 mp0", corebattle.BattleJobs{JobID: "job-73", SP: 1}, 0, 100, 0},
		{"job73", corebattle.BattleJobs{JobID: "job-73", SP: 1}, 5, 101, 4},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := randomFixture(t)
			saveCustom(t, f, "ひでん", [3]string{"gem_secret_art_sky", "", ""}, 20)
			cmd := customRoom(t, f, 30, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
				a.Jobs = tc.current
				a.Jobs.OldJobID, a.Jobs.OldSP = "job-04", 100
				a.Attack, a.BaseStats.Attack, a.MP, a.MaxMP = 100, 100, tc.mp, 50
			}, encounterEnemy("foe", 900))
			r := accepted(t, f, combatCommand(cmd, "secret", "ひでん", "", ""))
			if a := runActorByID(r, cmd.ActorID); a.Attack != tc.attack || a.MP != tc.wantMP || a.CMP != 10 {
				t.Fatalf("attack=%d mp=%d cmp=%d", a.Attack, a.MP, a.CMP)
			}
		})
	}
}

// An unknown battle hand leaves job86.4 (book power) unsupported, so the
// whole random pool rejects before cost, state or receipt; the pool is never
// narrowed to supported entries.
func TestRunCombatRandomRejectsMissingDescendant(t *testing.T) {
	f := randomFixture(t, randomDraw(3, 2, 8)...)
	cmd := metronomeRoom(t, f, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.HeldToolID = nil
	}, encounterEnemy("foe", 900))
	rejected(t, f, combatCommand(cmd, "unknown-hand", "ゆび", "", ""))
}

// #1733: after a random predecessor only registration is admitted; the later
// slot is validated against the actual state. When job3.3 fells foe, gem66's
// defeat reaches item212's last strike with a fallen listed enemy (an
// unrepresented context), so the whole command restores run, CMP and receipt.
// A harmless draw (job3.2) leaves the same custom accepted.
func TestRunCombatLaterSlotRejectsAfterRandomPredecessor(t *testing.T) {
	for _, tc := range []struct {
		position int
		accept   bool
	}{{3, false}, {2, true}} {
		f := randomFixture(t, randomDraw(3, tc.position, 8)...)
		saveCustom(t, f, "あとで", [3]string{"gem_metronome", "gem_sacrifice_soul", ""}, 11)
		cmd := customRoom(t, f, 30, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
			a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, "item-212")
		}, encounterEnemy("foe", 1), encounterEnemy("other", 90000))
		later := combatCommand(cmd, "later", "あとで", "foe", "")
		if !tc.accept {
			rejected(t, f, later)
			continue
		}
		if a := runActorByID(accepted(t, f, later), cmd.ActorID); a.HP != 0 || a.CMP != 19 {
			t.Fatalf("job3.2 then gem66: hp=%d cmp=%d", a.HP, a.CMP)
		}
	}
}

// Tool151's inspection consumes the held instance once
// and changes no participant; the receipt replays without re-inspecting.
func TestRunCombatTool151ConsumesHeldInstanceOnce(t *testing.T) {
	f := randomFixture(t)
	grantHeldTo(t, f, 0, "item-151")
	cmd := customRoom(t, f, 0, nil, encounterEnemy("foe", 900), encounterEnemy("other", 900))
	before := readStockRoom(t, f, cmd.RunID)
	r := accepted(t, f, combatCommand(cmd, "inspect", "tool", "", ""))
	a := runActorByID(r, cmd.ActorID)
	if a.HeldToolID == nil || *a.HeldToolID != "" || len(heldItems(t, f, 0)) != 0 || len(a.ConsumedItems) != len(runActorByID(before, cmd.ActorID).ConsumedItems)+1 {
		t.Fatalf("held=%v items=%+v consumed=%+v", a.HeldToolID, heldItems(t, f, 0), a.ConsumedItems)
	}
	for _, id := range []string{"foe", "other"} {
		if got, want := runActorByID(r, id), runActorByID(before, id); got.HP != want.HP || got.Status != want.Status {
			t.Fatalf("%s changed: %+v", id, got)
		}
	}
}
