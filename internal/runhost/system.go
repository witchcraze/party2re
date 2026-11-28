package runhost

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
)

// idleTimeout is legacy $auto_delete_quest_time (quest.cgi:86–90).
const idleTimeout = 1800 * time.Second

// Open opens a system-owned run with no members and no character leader
// (vs_monster.cgi make_vs_king): init sets the room's own state, such as its
// enemies. While a live run holds slot, Open returns that run instead.
// Transaction: WithRunForUpdate. Lock Order: runs(0); no character locks.
func (s *Service[R, P]) Open(ctx context.Context, slot string, a Admission, init func(*R)) (string, error) {
	if s.cfg.SystemRoom == nil || strings.TrimSpace(slot) == "" || len(slot) > 128 {
		return "", ErrUnavailable
	}
	if live, err := s.cfg.Store.FindSlot(ctx, slot); err != nil || live != "" {
		return live, err
	}
	// Every opening is a new identity, so a reopened room never inherits receipts or recipients.
	a.Generation = "system:" + slot + ":" + id.New()
	a.PartyID, a.ActorID, a.CharacterIDs, a.Actors = "", "", nil, nil
	runID := s.ID(a.Generation)
	err := s.cfg.Store.WithRunForUpdate(ctx, runID, true, func(ctx context.Context) error {
		var run R
		now := s.cfg.Clock.Now()
		*P(&run).Host() = State{ID: runID, Encoding: 1, Version: 1, Kind: s.cfg.Kind, Admission: a, LastActions: map[string]time.Time{}, Recipients: map[string]Recipient{}, StartedAt: now, LastActivity: now, Retained: true}
		if init != nil {
			init(&run)
		}
		return s.cfg.Store.OpenRun(ctx, run, slot)
	})
	if errors.Is(err, ErrConflict) {
		// A concurrent opening committed first; its run is the room.
		live, findErr := s.cfg.Store.FindSlot(ctx, slot)
		if findErr != nil || live == "" {
			return "", errors.Join(err, findErr)
		}
		return live, nil
	}
	if err != nil {
		return "", err
	}
	return runID, nil
}

// FindSlot reads the live run holding slot without changing it.
func (s *Service[R, P]) FindSlot(ctx context.Context, slot string) (*R, error) {
	runID, err := s.cfg.Store.FindSlot(ctx, slot)
	if err != nil || runID == "" {
		return nil, err
	}
	return s.cfg.Store.FindRun(ctx, runID)
}

// Join admits actor into runID's running battle as a new ally (quest.cgi
// add_member, type 6). key is the caller-retained request identity: a retry
// returns the committed receipt and never repeats the room's join effects.
// Joining is not room activity; legacy add_member never writes the room log.
// Transaction: WithRunForUpdate. Lock Order: runs(0) -> characters(2) -> enrollment/recipients(8).
func (s *Service[R, P]) Join(ctx context.Context, player, actor, runID, key string) (Receipt, error) {
	if s.cfg.SystemRoom == nil {
		return Receipt{}, ErrUnavailable
	}
	if err := s.authorize(ctx, player, actor); err != nil {
		return Receipt{}, err
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 || runID == "" {
		return Receipt{}, ErrConflict
	}
	cmd := Command{PlayerID: player, ActorID: actor, RunID: runID, ID: key, Action: "join"}
	var result Receipt
	err := s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		old, err := s.cfg.Store.FindRunReceipt(ctx, runID, key)
		if err != nil {
			return err
		}
		if old != nil {
			if old.Command.PlayerID != player || old.Command.ActorID != actor {
				return ErrForbidden
			}
			if old.Command != cmd {
				return ErrConflict
			}
			result = *old
			return nil
		}
		run, err := s.cfg.Store.FindRun(ctx, runID)
		if err != nil {
			return err
		}
		if run == nil {
			return ErrNotFound
		}
		r := P(run).Host()
		if r.Encoding != 1 || r.Kind != s.cfg.Kind || r.TimedOut {
			return ErrRecovery
		}
		if slices.Contains(r.Roster, actor) {
			return ErrBusy
		}
		if len(r.Roster) >= s.maxMembers() {
			return ErrRoomFull
		}
		// A recipient left by exit or Dejon is replaced; JoinRun carries its
		// unreleased earnings over (legacy stock.cgi outlives the room).
		if left, ok := r.Recipients[actor]; ok && (left.Disposition != "exit" && left.Disposition != "dejon" || left.Conditional != (Earnings{})) {
			return ErrConflict
		}
		c, err := s.cfg.Characters.FindByIDForUpdate(ctx, actor)
		if err != nil {
			return err
		}
		switch {
		case c.PlayerID != player:
			return ErrForbidden
		case c.PendingWake:
			return ErrBusy
		case c.Stats.HP <= 0:
			return ErrCharacterUnconscious
		case c.Tired >= 100:
			return ErrCharacterExhausted
		}
		active, err := s.cfg.Store.FindActorRun(ctx, actor)
		if err != nil {
			return err
		}
		if active != nil {
			return ErrBusy
		}
		if s.cfg.CheckEntry != nil {
			if err := s.cfg.CheckEntry(ctx, actor); err != nil {
				return err
			}
		}
		if err := s.cfg.SystemRoom.Join(run, &c); err != nil {
			return err
		}
		if err := s.cfg.Characters.Update(ctx, c); err != nil {
			return err
		}
		a, err := s.cfg.Builder.BuildLifetimeActor(ctx, actor, "")
		if err != nil {
			return err
		}
		a.TeamID = "allies"
		r.Battle.Actors = append(r.Battle.Actors, a)
		r.Roster = append(r.Roster, actor)
		joined, err := s.cfg.Store.JoinRun(ctx, runID, actor, len(r.Roster)-1, Recipient{PlayerID: player})
		if err != nil {
			return err
		}
		r.Recipients[actor] = joined
		// A joiner's battle row is new (get_battle_line), so no earlier action time carries over.
		delete(r.LastActions, actor)
		version := r.Version
		r.Version++
		if err := s.cfg.Store.SaveRun(ctx, *run, version); err != nil {
			return err
		}
		result = Receipt{Command: cmd, Version: r.Version, Round: r.Round, Recipient: r.Recipients[actor]}
		return s.cfg.Store.SaveRunReceipt(ctx, result)
	})
	return s.resolveReceipt(ctx, cmd, result, err)
}

// disposeIdle deletes a retained run that has had no members and no room
// activity for the idle timeout (quest.cgi:86–90). A run with members keeps
// its member-requested timeout, which applies their penalties.
// Transaction: WithRunForUpdate. Lock Order: runs(0); no character locks.
func (s *Service[R, P]) disposeIdle(ctx context.Context, runID string) error {
	return s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		run, err := s.cfg.Store.FindRun(ctx, runID)
		if err != nil || run == nil {
			return err
		}
		r := P(run).Host()
		if len(r.Roster) > 0 || !s.cfg.Clock.Now().After(r.LastActivity.Add(idleTimeout)) {
			return ErrConflict
		}
		version := r.Version
		r.Version++
		r.Retained = false
		return s.cfg.Store.SaveRun(ctx, *run, version)
	})
}

// sweepBatchLimit bounds the rooms one scheduled sweep tick cleans up.
const sweepBatchLimit = 50

// SweepIdle cleans up this kind's idle rooms without a member command, standing in
// for legacy's cleanup on every quest-list view (quest.cgi:80–91, auto_delete_quest
// 172–184) because GET stays read-only. It also finishes timeouts whose penalty
// projection failed. Each room is re-checked under its run lock, so racing commands
// and repeated sweeps are safe.
func (s *Service[R, P]) SweepIdle(ctx context.Context) error {
	runs, err := s.cfg.Store.ListRuns(ctx, s.cfg.Kind)
	if err != nil {
		return err
	}
	cutoff := s.cfg.Clock.Now().Add(-idleTimeout)
	var errs []error
	swept := 0
	for _, r := range runs {
		if swept == sweepBatchLimit {
			break
		}
		var err error
		switch {
		case len(r.Roster) > 0 && (r.TimedOut || r.LastActivity.Before(cutoff)):
			err = s.timeoutIdle(ctx, r.ID)
		case len(r.Roster) == 0 && r.Retained && r.LastActivity.Before(cutoff):
			err = s.disposeIdle(ctx, r.ID)
		default:
			continue
		}
		swept++
		// ErrConflict: a racing command renewed or finished the room first.
		if err != nil && !errors.Is(err, ErrConflict) {
			errs = append(errs, fmt.Errorf("run %s: %w", r.ID, err))
		}
	}
	return errors.Join(errs...)
}

// timeoutIdle commits the idle timeout a member's cleanup command would, then
// projects its fixed penalties.
// Transaction: WithRunForUpdate. Lock Order: runs(0) -> characters(2, sorted) -> recipients(8).
func (s *Service[R, P]) timeoutIdle(ctx context.Context, runID string) error {
	err := s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		run, err := s.cfg.Store.FindRun(ctx, runID)
		if err != nil || run == nil {
			return err
		}
		r := P(run).Host()
		if r.TimedOut || len(r.Roster) == 0 {
			return nil
		}
		now := s.cfg.Clock.Now()
		if !now.After(r.LastActivity.Add(idleTimeout)) {
			return ErrConflict
		}
		ctx = commandContext(ctx, r.ID)
		if err := economy.LockInOrder(ctx, r.Roster, func(ctx context.Context, id string) error {
			_, err := s.cfg.Characters.FindByIDForUpdate(ctx, id)
			return err
		}); err != nil {
			return err
		}
		if err := s.timeout(ctx, run, now); err != nil {
			return err
		}
		version := r.Version
		r.Version++
		for _, id := range r.Roster {
			if err := s.cfg.Store.SaveRunRecipient(ctx, r.ID, id, r.Recipients[id]); err != nil {
				return err
			}
		}
		return s.cfg.Store.SaveRun(ctx, *run, version)
	})
	if err != nil {
		return err
	}
	return s.recoverTimeout(ctx, runID, nil)
}
