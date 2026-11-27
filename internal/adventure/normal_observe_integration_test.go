package adventure_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/runhost"
)

// Observation discovers the operations Command would admit, reveals only
// public encounter facts and leaves the stored run byte-identical.
func TestObserveRunDiscoversWithoutMutation(t *testing.T) {
	f := combatFixture(t, 1)
	cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
	before, err := json.Marshal(readStockRoom(t, f, cmd.RunID))
	if err != nil {
		t.Fatal(err)
	}
	obs, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID)
	if err != nil || obs == nil {
		t.Fatal(obs, err)
	}
	usable := func(action string) bool {
		return slices.ContainsFunc(obs.Choices, func(c runhost.OperationChoice) bool { return c.Action == action && !c.Gated })
	}
	if obs.RunID != cmd.RunID || obs.Version != cmd.Version || !usable("attack") || !usable("exit") || usable("advance") {
		t.Fatalf("observation %+v", obs)
	}
	if !slices.ContainsFunc(obs.Actors, func(a runhost.ObservedActor) bool { return a.ID == "foe" && a.Enemy && a.HP == 900 }) || obs.ReadyAt.After(f.clock.Now()) {
		t.Fatalf("actors %+v ready %v now %v", obs.Actors, obs.ReadyAt, f.clock.Now())
	}
	after, err := json.Marshal(readStockRoom(t, f, cmd.RunID))
	if err != nil || string(before) != string(after) {
		t.Fatalf("observation changed the run: %v", err)
	}
	if _, err := f.svc.ObserveRun(t.Context(), "foreign-player", cmd.ActorID); !errors.Is(err, runhost.ErrForbidden) {
		t.Fatalf("foreign observation: %v", err)
	}

	r := accepted(t, f, combatCommand(cmd, "strike", "attack", "foe", ""))
	if obs, err = f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID); err != nil || obs.Version != r.Version || !obs.ReadyAt.After(f.clock.Now()) {
		t.Fatalf("after command: %+v %v", obs, err)
	}
}

type observeHookStore struct {
	runhost.Store[adventure.NormalRun]
	findRunErr error
	findErr    error
}

func (s *observeHookStore) FindRun(ctx context.Context, runID string) (*adventure.NormalRun, error) {
	if s.findRunErr != nil {
		return nil, s.findRunErr
	}
	return s.Store.FindRun(ctx, runID)
}

func (s *observeHookStore) FindActorRun(ctx context.Context, actor string) (*adventure.NormalRun, error) {
	if s.findErr != nil {
		return nil, s.findErr
	}
	return s.Store.FindActorRun(ctx, actor)
}

type observeHookChars struct {
	runhost.Characters
	calls   int
	onSetup func()
}

func (c *observeHookChars) FindByID(ctx context.Context, actor string) (corecharacter.Character, error) {
	c.calls++
	// Authorize reads the character first; the second read occurs during combat setup after the run snapshot is loaded.
	if c.calls == 2 && c.onSetup != nil {
		fn := c.onSetup
		c.onSetup = nil
		fn()
	}
	return c.Characters.FindByID(ctx, actor)
}

func hasUsableChoice(obs *runhost.Observation, action string) bool {
	return slices.ContainsFunc(obs.Choices, func(c runhost.OperationChoice) bool {
		return c.Action == action && !c.Gated && c.Submittable
	})
}

func hasObservedActor(obs *runhost.Observation, actorID string) bool {
	return slices.ContainsFunc(obs.Actors, func(a runhost.ObservedActor) bool { return a.ID == actorID })
}

func TestObserveRunSingleVersionConsistency(t *testing.T) {
	t.Run("after command", func(t *testing.T) {
		f := combatFixture(t, 1)
		c := f.actors[0]
		c.JobID = "job-01"
		if err := f.chars.Update(t.Context(), c); err != nil {
			t.Fatal(err)
		}
		f.actors[0] = c
		entry := f.enter(t, adventure.CrawlModeTry)
		cmd := runhost.Command{PlayerID: c.PlayerID, ActorID: c.ID, RunID: entry.Command.RunID, Version: entry.Version}
		foe := runEnemy("foe", 5)
		foe.GoldReward = 12
		editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
			r.Round, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
			r.Battle.Actors = append(r.Battle.Actors, foe)
			r.Enemies = append(r.Enemies, foe.ID)
			a := runActorByID(r, c.ID)
			a.Attack, a.HP, a.MaxHP = 400, 500, 500
			r.LastActions[c.ID] = f.clock.Now()
		})
		f.clock.add(3 * time.Second)
		v1 := cmd.Version
		var storeHook *observeHookStore
		commitStrike := func() {
			storeHook.findRunErr = nil
			accepted(t, f, combatCommand(cmd, "strike", "attack", "foe", ""))
		}

		storeHook = &observeHookStore{Store: f.store, findRunErr: errors.New("unexpected FindRun during ObserveRun")}
		charsHook := &observeHookChars{Characters: f.chars, onSetup: commitStrike}
		f.cfg.Store = storeHook
		f.cfg.Characters = charsHook
		f.restart(t)

		// Advance past idleTimeout only on the clock while v1 still has an older LastActivity,
		// and verify ObserveRun derives IdleDue and IdleAt from the same v1 snapshot.
		f.clock.add(1800 * time.Second)
		obs, err := f.svc.ObserveRun(t.Context(), c.PlayerID, c.ID)
		if err != nil || obs == nil {
			t.Fatalf("ObserveRun: obs=%+v err=%v", obs, err)
		}
		if obs.Version != v1 || obs.Role != "leader" || obs.Recipient.Conditional.Gold != 0 || !obs.IdleDue || !hasObservedActor(obs, "foe") || !hasUsableChoice(obs, "attack") {
			t.Fatalf("pre-commit observation mixed versions: %+v", obs)
		}

		// Re-observe v2: strike renewed LastActivity, so IdleDue cleared with the new version.
		storeHook.findRunErr = errors.New("unexpected FindRun during ObserveRun")
		after, err := f.svc.ObserveRun(t.Context(), c.PlayerID, c.ID)
		if err != nil || after == nil {
			t.Fatalf("post-commit ObserveRun: obs=%+v err=%v", after, err)
		}
		if after.Version != v1+1 || after.Role != "leader" || after.Recipient.Conditional.Gold != 12 || after.IdleDue || hasObservedActor(after, "foe") || !after.ReadyAt.After(after.Now) {
			t.Fatalf("post-commit observation mismatch: %+v", after)
		}
	})

	t.Run("after leader exit with remaining member", func(t *testing.T) {
		f := combatFixture(t, 2)
		for i := range f.actors {
			f.actors[i].JobID = "job-01"
			if err := f.chars.Update(t.Context(), f.actors[i]); err != nil {
				t.Fatal(err)
			}
		}
		_, lobby := f.waitingParty(t, adventure.CrawlModeTry)
		leader, member := f.actors[0], f.actors[1]
		entry, err := f.svc.EnterParty(t.Context(), leader.PlayerID, leader.ID, lobby)
		if err != nil {
			t.Fatal(err)
		}
		cmd := runhost.Command{PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: entry.Command.RunID, Version: entry.Version}
		editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
			r.Round, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
			foe := runEnemy("foe", 900)
			r.Battle.Actors = append(r.Battle.Actors, foe)
			r.Enemies = append(r.Enemies, foe.ID)
			p := r.Recipients[leader.ID]
			p.Pending.Gold = 12
			r.Recipients[leader.ID] = p
			r.LastActions[leader.ID] = f.clock.Now()
		})
		f.clock.add(3 * time.Second)
		v1 := cmd.Version
		var storeHook *observeHookStore
		commitLeaderExit := func() {
			storeHook.findRunErr = nil
			accepted(t, f, combatCommand(cmd, "leader-exit", "exit", "", ""))
		}

		// Observe the remaining member while leader-exit commits after the run read.
		storeHook = &observeHookStore{Store: f.store, findRunErr: errors.New("unexpected FindRun during ObserveRun")}
		charsHook := &observeHookChars{Characters: f.chars, onSetup: commitLeaderExit}
		f.cfg.Store = storeHook
		f.cfg.Characters = charsHook
		f.restart(t)

		prePromotion, err := f.svc.ObserveRun(t.Context(), member.PlayerID, member.ID)
		if err != nil || prePromotion == nil {
			t.Fatalf("member ObserveRun during leader exit: obs=%+v err=%v", prePromotion, err)
		}
		if prePromotion.Version != v1 || prePromotion.Role != "member" || prePromotion.LeaderID != leader.ID || !hasObservedActor(prePromotion, leader.ID) {
			t.Fatalf("member pre-exit observation incoherent: %+v", prePromotion)
		}

		// Departed leader observes no active run; promoted member observes coherent v2 leadership.
		storeHook.findRunErr = errors.New("unexpected FindRun during ObserveRun")
		if gone, err := f.svc.ObserveRun(t.Context(), leader.PlayerID, leader.ID); err != nil || gone != nil {
			t.Fatalf("departed leader still observed: obs=%+v err=%v", gone, err)
		}

		promoted, err := f.svc.ObserveRun(t.Context(), member.PlayerID, member.ID)
		if err != nil || promoted == nil {
			t.Fatalf("promoted member ObserveRun: obs=%+v err=%v", promoted, err)
		}
		if promoted.Version != v1+1 || promoted.Role != "leader" || promoted.LeaderID != member.ID || hasObservedActor(promoted, leader.ID) || !hasObservedActor(promoted, member.ID) {
			t.Fatalf("promoted member observation incoherent: %+v", promoted)
		}
	})

	t.Run("departing leader observes pre-exit snapshot or nil", func(t *testing.T) {
		f := combatFixture(t, 2)
		for i := range f.actors {
			f.actors[i].JobID = "job-01"
			if err := f.chars.Update(t.Context(), f.actors[i]); err != nil {
				t.Fatal(err)
			}
		}
		_, lobby := f.waitingParty(t, adventure.CrawlModeTry)
		leader := f.actors[0]
		entry, err := f.svc.EnterParty(t.Context(), leader.PlayerID, leader.ID, lobby)
		if err != nil {
			t.Fatal(err)
		}
		cmd := runhost.Command{PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: entry.Command.RunID, Version: entry.Version}
		editStockRoom(t, f, &cmd, func(r *adventure.NormalRun) {
			r.Round, r.EncounterFloor, r.EncounterStageID = 1, 1, r.Stage.ID
			foe := runEnemy("foe", 900)
			r.Battle.Actors = append(r.Battle.Actors, foe)
			r.Enemies = append(r.Enemies, foe.ID)
			p := r.Recipients[leader.ID]
			p.Pending.Gold = 12
			r.Recipients[leader.ID] = p
			r.LastActions[leader.ID] = f.clock.Now()
		})
		f.clock.add(3 * time.Second)
		v1 := cmd.Version
		var storeHook *observeHookStore
		commitLeaderExit := func() {
			storeHook.findRunErr = nil
			accepted(t, f, combatCommand(cmd, "leader-exit", "exit", "", ""))
		}

		storeHook = &observeHookStore{Store: f.store, findRunErr: errors.New("unexpected FindRun during ObserveRun")}
		charsHook := &observeHookChars{Characters: f.chars, onSetup: commitLeaderExit}
		f.cfg.Store = storeHook
		f.cfg.Characters = charsHook
		f.restart(t)

		obs, err := f.svc.ObserveRun(t.Context(), leader.PlayerID, leader.ID)
		if err != nil || obs == nil {
			t.Fatalf("leader ObserveRun during exit: obs=%+v err=%v", obs, err)
		}
		if obs.Version != v1 || obs.Role != "leader" || obs.LeaderID != leader.ID || obs.Recipient.Pending.Gold != 12 || !hasObservedActor(obs, leader.ID) || !hasUsableChoice(obs, "exit") {
			t.Fatalf("leader observation incoherent: %+v", obs)
		}
		storeHook.findRunErr = errors.New("unexpected FindRun during ObserveRun")
		if gone, err := f.svc.ObserveRun(t.Context(), leader.PlayerID, leader.ID); err != nil || gone != nil {
			t.Fatalf("departed leader still observed: obs=%+v err=%v", gone, err)
		}
	})

	t.Run("after last-member exit disposal", func(t *testing.T) {
		f := combatFixture(t, 1)
		cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
		v1 := cmd.Version
		var storeHook *observeHookStore
		commitLastExit := func() {
			storeHook.findRunErr = nil
			if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "last-exit", "exit", "", "")); err != nil {
				t.Fatal(err)
			}
		}

		storeHook = &observeHookStore{Store: f.store, findRunErr: errors.New("unexpected FindRun during ObserveRun")}
		charsHook := &observeHookChars{Characters: f.chars, onSetup: commitLastExit}
		f.cfg.Store = storeHook
		f.cfg.Characters = charsHook
		f.restart(t)

		obs, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID)
		if err != nil || obs == nil {
			t.Fatalf("ObserveRun during disposal: obs=%+v err=%v", obs, err)
		}
		if obs.Version != v1 || obs.Role != "leader" || len(obs.Actors) == 0 || !hasUsableChoice(obs, "exit") {
			t.Fatalf("disposal observation incoherent: %+v", obs)
		}
		if gone, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID); err != nil || gone != nil {
			t.Fatalf("disposed run still observed: obs=%+v err=%v", gone, err)
		}
	})

	t.Run("pending cleanup ownership and storage failure", func(t *testing.T) {
		f := combatFixture(t, 1)
		cmd := combatRoom(t, f, f.actors[0], nil, runEnemy("foe", 900))
		f.clock.add(1801 * time.Second)
		f.penalty.fail.Store(true)
		if _, err := f.svc.Command(t.Context(), combatCommand(cmd, "idle-cleanup", "cleanup", "", "")); err == nil {
			t.Fatal("expected cleanup penalty projection failure")
		}

		obs, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID)
		if err != nil || obs == nil {
			t.Fatalf("pending cleanup ObserveRun: obs=%+v err=%v", obs, err)
		}
		if !obs.Recipient.TimerPending || !obs.IdleDue || len(obs.Choices) != 0 || !hasObservedActor(obs, cmd.ActorID) {
			t.Fatalf("pending cleanup observation incoherent: %+v", obs)
		}
		if _, err := f.svc.ObserveRun(t.Context(), "other-player", cmd.ActorID); !errors.Is(err, runhost.ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}

		storeHook := &observeHookStore{Store: f.store, findErr: errors.New("db unavailable")}
		f.cfg.Store = storeHook
		f.restart(t)
		if _, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID); !errors.Is(err, storeHook.findErr) {
			t.Fatalf("expected storage error propagation, got %v", err)
		}

		f.cfg.Store = f.store
		f.penalty.fail.Store(false)
		f.restart(t)
		if err := f.svc.RecoverTimeout(t.Context(), cmd.PlayerID, cmd.ActorID, cmd.RunID); err != nil {
			t.Fatal(err)
		}
		if gone, err := f.svc.ObserveRun(t.Context(), cmd.PlayerID, cmd.ActorID); err != nil || gone != nil {
			t.Fatalf("recovered timeout still observed: obs=%+v err=%v", gone, err)
		}
	})
}
