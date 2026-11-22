package adventure_test

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/customskill"
)

// saveCustom stores the actor's custom skill with persisted gem catalog IDs.
func saveCustom(t *testing.T, f *normalFixture, name string, gems [3]string, cost int) {
	t.Helper()
	id := f.actors[0].ID
	if err := f.customs.SaveCustomSkill(t.Context(), customskill.CustomSkill{CharacterID: id, Name: name, CMP: cost, Gems: gems, UpdatedAt: time.Now()}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.db.ExecContext(context.Background(), "DELETE FROM character_custom_skills WHERE character_id = ?", id); err != nil {
			t.Error(err)
		}
	})
}

func withAlly(tension float64, temp corebattle.TemporaryEffect) func(*adventure.NormalRun, *corebattle.LifetimeActor) {
	return func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
		ally := runEnemy("ally", 500)
		ally.TeamID, ally.Tension, ally.Temporary, ally.Hit = a.TeamID, tension, temp, 40
		r.Battle.Actors = append(r.Battle.Actors, ally)
	}
}

// customRoom freezes the stored custom skill at entry and leaves the NPC idle.
func customRoom(t *testing.T, f *normalFixture, cmp int, edit func(*adventure.NormalRun, *corebattle.LifetimeActor), enemies ...corebattle.LifetimeActor) adventure.RunCommand {
	t.Helper()
	return combatRoom(t, f, f.actors[0], func(r *adventure.NormalRun) {
		a := runActorByID(r, f.actors[0].ID)
		// No tail recharge while CMPCharge < CMPNeedCharge.
		a.CMP, a.MaxCMP, a.CMPCharge, a.CMPNeedCharge = cmp, cmp, 0, 9
		for _, e := range enemies {
			r.LastActions[e.ID] = f.clock.Now().Add(time.Hour)
		}
		if edit != nil {
			edit(r, a)
		}
	}, enemies...)
}

// #1436/#2338: entry freezes name, catalog gem IDs and cost; later edits to
// the stored skill do not change the run, across reload.
func TestRunCombatCustomGemsFreezeAtEntry(t *testing.T) {
	f := combatFixture(t, 1)
	gems := [3]string{"gem_buff_atk_1", "gem_buff_atk_1", ""}
	saveCustom(t, f, "ふたつ", gems, 12)
	cmd := customRoom(t, f, 30, func(_ *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Attack, a.BaseStats.Attack = 100, 100
	}, runEnemy("foe", 900))
	frozen := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID)
	if !reflect.DeepEqual(frozen.Custom, &corebattle.CustomLoadout{Name: "ふたつ", Gems: gems, Cost: 12}) {
		t.Fatalf("entry loadout: %+v", frozen.Custom)
	}
	saveCustom(t, f, "べつ", [3]string{"gem_mind_eye", "", ""}, 1)
	f.restart(t)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "renamed", "べつ", "", "")); err == nil {
		t.Fatalf("stored edit leaked into the run: %v", err)
	}
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "custom", "ふたつ", "", "")); err != nil {
		t.Fatal(err)
	}
	// _data.cgi gem28 → _st_up (_skill.cgi:9307–9340): each slot adds
	// floor(base 100×1.0×tension 1), clamped to int(base×2.5): 100→200→250.
	// The frozen cost 12 is charged once, not the stored 1.
	a := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID)
	if a.Attack != 250 || a.CMP != 18 || a.MP != frozen.MP || !reflect.DeepEqual(a.Custom, frozen.Custom) {
		t.Fatalf("custom: attack %d→%d cmp=%d mp=%d custom=%+v", frozen.Attack, a.Attack, a.CMP, a.MP, a.Custom)
	}
}

// #1733: caster death stops later slots but keeps the late CMP charge; an
// unregistered later slot rejects before any slot, cost or receipt.
func TestRunCombatCustomLaterSlots(t *testing.T) {
	t.Run("death stops later slots", func(t *testing.T) {
		f := combatFixture(t, 1)
		saveCustom(t, f, "すてみ", [3]string{"gem_kamikaze_sky", "gem_mind_eye", ""}, 12)
		cmd := customRoom(t, f, 30, withAlly(1, ""), runEnemy("foe", 900000))
		if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "custom", "すてみ", "foe", "")); err != nil {
			t.Fatal(err)
		}
		r := readStockRoom(t, f, cmd.RunID)
		// gem72 strikes for the caster's HP and defeats the caster; gem54 never runs.
		a, ally := runActorByID(r, cmd.ActorID), runActorByID(r, "ally")
		if a.HP != 0 || ally.Hit != 40 || a.CMP != 18 || runActorByID(r, "foe").HP >= 900000 {
			t.Fatalf("hp=%d ally hit=%d cmp=%d foe=%d", a.HP, ally.Hit, a.CMP, runActorByID(r, "foe").HP)
		}
	})
	t.Run("unregistered later slot", func(t *testing.T) {
		f := combatFixture(t, 1)
		saveCustom(t, f, "なぞ", [3]string{"gem_buff_atk_1", "missing", ""}, 12)
		cmd := customRoom(t, f, 30, nil, runEnemy("foe", 900))
		before := readStockRoom(t, f, cmd.RunID)
		if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "custom", "なぞ", "", "")); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
			t.Fatal(err)
		}
		if after := readStockRoom(t, f, cmd.RunID); !reflect.DeepEqual(before, after) {
			t.Fatal("rejected custom mutated the run")
		}
		if got, err := f.svc.GetRunReceipt(t.Context(), cmd.PlayerID, cmd.ActorID, cmd.RunID, "custom"); err != nil || got != nil {
			t.Fatalf("rejected custom stored a receipt: %+v %v", got, err)
		}
	})
}

// #1451: sequence 94/28/74 applies gem74's tension once to its one resolved
// ally (_data.cgi gem74; _check_party picks a random ally for an enemy target).
func TestRunCombatRecoveryGemAlternativesApplyOnce(t *testing.T) {
	f := combatFixture(t, 1)
	saveCustom(t, f, "こぶ", [3]string{"gem_debuff_def", "gem_buff_atk_1", "gem_inspire_sky"}, 12)
	cmd := customRoom(t, f, 30, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Status, a.Temporary, a.Tension = corebattle.StatusPhysicalSeal, corebattle.TemporaryCover, 8
		withAlly(1.7, corebattle.TemporaryCovering)(r, a)
	}, runEnemy("foe", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "custom", "こぶ", "foe", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	self, ally := runActorByID(r, cmd.ActorID), runActorByID(r, "ally")
	// _check_party draws partys[int(rand 2)] = ally at .5; Covering does not
	// redirect. One tenshon 1.7→3, never 1.7→3→5; self keeps 8.
	if self.Tension != 8 || ally.Tension != 3 {
		t.Fatalf("self=%v ally=%v", self.Tension, ally.Tension)
	}
}

// #1452: 54/68/67 against an ally target resets accuracy, lets gem68's
// fallback enemy reflect the reset onto the caster, then raises it twice.
func TestRunCombatRecoveryGemReflectedResetSequence(t *testing.T) {
	f := combatFixture(t, 1)
	saveCustom(t, f, "しずか", [3]string{"gem_mind_eye", "gem_calm_orb", "gem_divine_tension_sky"}, 12)
	foe := runEnemy("foe", 900)
	foe.Temporary, foe.Tension = corebattle.TemporaryReflectAttack, 5
	cmd := customRoom(t, f, 30, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
		a.Tension, a.Hit = 3, 50
		withAlly(1, "")(r, a)
	}, foe)
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "custom", "しずか", "ally", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	self, ally, got := runActorByID(r, cmd.ActorID), runActorByID(r, "ally"), runActorByID(r, "foe")
	if self.Hit != 95 || ally.Hit != 95 || self.Tension != 3 || got.Tension != 5 {
		t.Fatalf("self hit=%d ten=%v ally hit=%d foe ten=%v", self.Hit, self.Tension, ally.Hit, got.Tension)
	}
}

// #1678: _skill.cgi skill_79 base-220 bite raises tension once, and once more
// only when the returned target fell. At .5, item145's rand(6)<1 recursion misses.
func TestRunCombatJob79BiteTension(t *testing.T) {
	for _, tc := range []struct {
		name string
		hp   int
		want float64
	}{
		{"nonlethal", 100000, 1.7},
		{"lethal", 5, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := combatFixture(t, 1)
			cmd := customRoom(t, f, 0, func(r *adventure.NormalRun, a *corebattle.LifetimeActor) {
				a.Jobs = corebattle.BattleJobs{JobID: "job-79", SP: 99}
				a.MP, a.MaxMP, a.Tension = 50, 50, 1
				a.ItemDefinitionIDs = []string{"item-145"}
			}, runEnemy("foe", tc.hp), runEnemy("other", 900))
			if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "bite", "job-79-entry-4", "foe", "")); err != nil {
				t.Fatal(err)
			}
			a := runActorByID(readStockRoom(t, f, cmd.RunID), cmd.ActorID)
			if a.Tension != tc.want || a.MP != 23 {
				t.Fatalf("tension=%v mp=%d", a.Tension, a.MP)
			}
		})
	}
}

// #1419: admission and execution use the same explicit target, so a normal
// attack on a detached actor of another team strikes it instead of the
// one-hit-defended enemy.
func TestRunCombatAttackOnDetachedTarget(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := customRoom(t, f, 0, func(r *adventure.NormalRun, _ *corebattle.LifetimeActor) {
		stray := runEnemy("stray", 900)
		stray.TeamID, stray.RosterIdentity, stray.RosterDetached = "npc", "job-60", true
		r.Battle.Actors = append(r.Battle.Actors, stray)
		runActorByID(r, "foe").Temporary = corebattle.TemporaryOneHitDefense
	}, runEnemy("foe", 900))
	if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "strike", "attack", "stray", "")); err != nil {
		t.Fatal(err)
	}
	r := readStockRoom(t, f, cmd.RunID)
	if stray, foe := runActorByID(r, "stray"), runActorByID(r, "foe"); stray.HP >= 900 || foe.HP != 900 || foe.Temporary != corebattle.TemporaryOneHitDefense {
		t.Fatalf("stray=%d foe=%d %q", stray.HP, foe.HP, foe.Temporary)
	}
}
