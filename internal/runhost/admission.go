package runhost

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

func (s *Service[R, P]) authorize(ctx context.Context, player, actor string) error {
	if player == "" || actor == "" {
		return ErrForbidden
	}
	c, err := s.cfg.Characters.FindByID(ctx, actor)
	if err != nil {
		return err
	}
	if c.ID != actor || c.PlayerID != player {
		return ErrForbidden
	}
	return nil
}

// Enroll enrolls a.ActorID alone without creating an encounter. The caller
// retains key on retries.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> characters(2, sorted) -> enrollment/recipients(8).
func (s *Service[R, P]) Enroll(ctx context.Context, player, key string, a Admission) (Receipt, error) {
	if err := s.authorize(ctx, player, a.ActorID); err != nil {
		return Receipt{}, err
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return Receipt{}, ErrConflict
	}
	a.Generation, a.PartyID, a.CharacterIDs = "solo:"+a.ActorID+":"+key, "", []string{a.ActorID}
	return s.enter(ctx, player, a)
}

// EnterParty resolves committed admission before reading the expiring lobby.
// Transaction: WithRunForUpdate, inside waiting admission coordination; SQL commit precedes lobby cleanup.
func (s *Service[R, P]) EnterParty(ctx context.Context, player, actor, partyID string) (Receipt, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return Receipt{}, err
	}
	if partyID == "" || s.cfg.Lobby == nil {
		return Receipt{}, ErrUnavailable
	}
	generation := "party:" + partyID
	a, err := s.cfg.Store.FindAdmission(ctx, s.ID(generation))
	if err != nil {
		return Receipt{}, err
	}
	if a != nil {
		if a.ActorID != actor {
			return Receipt{}, ErrForbidden
		}
		result, err := s.enter(ctx, player, *a)
		if err == nil {
			err = s.cfg.Lobby.CleanupLobby(ctx, *a)
		}
		return result, err
	}
	var result Receipt
	err = s.cfg.Lobby.WithRoster(ctx, actor, partyID, func(ctx context.Context, a Admission) error {
		if a.PartyID != partyID || a.ActorID != actor || a.Generation != generation {
			return ErrConflict
		}
		var err error
		result, err = s.enter(ctx, player, a)
		return err
	})
	if err != nil {
		committed, readErr := s.cfg.Store.FindAdmission(ctx, s.ID(generation))
		if readErr != nil {
			return Receipt{}, errors.Join(err, readErr)
		}
		if committed == nil {
			return Receipt{}, err
		}
		if committed.ActorID != actor {
			return Receipt{}, ErrForbidden
		}
		result, err = s.enter(ctx, player, *committed)
		if err != nil {
			return Receipt{}, err
		}
	}
	a, err = s.cfg.Store.FindAdmission(ctx, s.ID(generation))
	if err != nil {
		return result, err
	}
	if a == nil {
		return result, ErrRecovery
	}
	return result, s.cfg.Lobby.CleanupLobby(ctx, *a)
}

func (s *Service[R, P]) enter(ctx context.Context, player string, a Admission) (Receipt, error) {
	if err := s.cfg.Room.Prepare(&a); err != nil {
		return Receipt{}, err
	}
	if a.Speed != 3 && a.Speed != 18 && a.Speed != 25 {
		return Receipt{}, ErrConflict
	}
	if len(a.CharacterIDs) < 1 || len(a.CharacterIDs) > s.maxMembers() || a.CharacterIDs[0] != a.ActorID {
		return Receipt{}, ErrNoParticipants
	}
	if len(a.Actors) != 0 && len(a.Actors) != len(a.CharacterIDs) {
		return Receipt{}, ErrConflict
	}
	// Admission must reject duplicate/empty participants rather than deduplicate them.
	ids := slices.Clone(a.CharacterIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if id == "" || (i > 0 && ids[i-1] == id) {
			return Receipt{}, ErrConflict
		}
	}
	input, err := json.Marshal(a)
	if err != nil {
		return Receipt{}, err
	}
	cmd := Command{PlayerID: player, ActorID: a.ActorID, RunID: s.ID(a.Generation), ID: "enroll", Action: "enroll", Input: string(input)}
	var result Receipt
	var release func(context.Context) error
	err = s.cfg.Store.WithRunForUpdate(ctx, cmd.RunID, true, func(ctx context.Context) error {
		old, err := s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		if err != nil {
			return err
		}
		if old != nil {
			if old.Command != cmd {
				return ErrConflict
			}
			result = *old
			return nil
		}
		var run R
		r := P(&run).Host()
		*r = State{ID: cmd.RunID, Encoding: 1, Version: 1, Kind: s.cfg.Kind, Admission: a, LeaderID: a.ActorID, Roster: slices.Clone(a.CharacterIDs), LastActions: map[string]time.Time{}, Recipients: map[string]Recipient{}, StartedAt: s.cfg.Clock.Now(), LastActivity: s.cfg.Clock.Now()}
		if err := economy.LockInOrder(ctx, ids, func(ctx context.Context, id string) error {
			c, err := s.cfg.Characters.FindByIDForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if c.PendingWake {
				return ErrBusy
			}
			if c.Stats.HP <= 0 {
				return ErrCharacterUnconscious
			}
			if c.Tired >= 100 {
				return ErrCharacterExhausted
			}
			if id == a.ActorID && c.PlayerID != player {
				return ErrForbidden
			}
			r.Recipients[id] = Recipient{PlayerID: c.PlayerID}
			return nil
		}); err != nil {
			return err
		}
		for _, id := range ids {
			active, err := s.cfg.Store.FindActorRun(ctx, id)
			if err != nil {
				return err
			}
			if active != nil {
				return ErrBusy
			}
			if s.cfg.CheckEntry != nil {
				if err := s.cfg.CheckEntry(ctx, id); err != nil {
					return err
				}
			}
			if err := s.cfg.Room.AdmitMember(ctx, &run, id); err != nil {
				return err
			}
		}
		if a.PartyID == "" && s.cfg.Lobby != nil {
			if err := s.cfg.Lobby.CheckSoloAdmission(ctx, a.ActorID); err != nil {
				return err
			}
		}
		for i, id := range a.CharacterIDs {
			var actor corebattle.LifetimeActor
			if i < len(a.Actors) && a.Actors[i].ID != "" {
				if a.Actors[i].ID != id {
					return ErrConflict
				}
				actor = a.Actors[i].Clone()
			} else {
				var err error
				actor, err = s.cfg.Builder.BuildLifetimeActor(ctx, id, "")
				if err != nil {
					return err
				}
			}
			actor.TeamID = "allies"
			r.Battle.Actors = append(r.Battle.Actors, actor)
		}
		if release, err = s.cfg.Room.Open(ctx, &run); err != nil {
			return err
		}
		if err := s.cfg.Store.EnrollRun(ctx, *r); err != nil {
			return err
		}
		if err := s.cfg.Store.SaveRun(ctx, run, 0); err != nil {
			return err
		}
		result = Receipt{Command: cmd, Version: 1, Recipient: r.Recipients[a.ActorID]}
		return s.cfg.Store.SaveRunReceipt(ctx, result)
	})
	// The outer waiting-admission transaction must resolve before receipts can prove commit.
	if ctx.Value(admissionContext{}) == cmd.RunID {
		return result, err
	}
	result, err = s.resolveReceipt(ctx, cmd, result, err)
	if err != nil && release != nil && !errors.Is(err, ErrRecovery) {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if releaseErr := release(rctx); releaseErr != nil {
			return Receipt{}, errors.Join(ErrRecovery, err, releaseErr)
		}
	}
	return result, err
}

func (s *Service[R, P]) resolveReceipt(ctx context.Context, cmd Command, result Receipt, cause error) (Receipt, error) {
	if cause == nil {
		return result, nil
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved *Receipt
	// Serialize the read against the transaction that may still be committing.
	err := s.cfg.Store.WithRunForUpdate(rctx, cmd.RunID, false, func(ctx context.Context) error {
		var err error
		saved, err = s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		return err
	})
	if err != nil {
		return Receipt{}, errors.Join(ErrRecovery, cause, err)
	}
	if saved == nil || saved.Command != cmd {
		return Receipt{}, cause
	}
	return *saved, nil
}

func (s *Service[R, P]) GetActorRun(ctx context.Context, player, actor string) (*ActorRun, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	state, err := s.cfg.Store.FindActorRun(ctx, actor)
	if err != nil || state == nil {
		return state, err
	}
	if state.Recipient.PlayerID != player {
		return nil, ErrForbidden
	}
	state.IdleDue = s.cfg.Clock.Now().After(state.LastActivity.Add(idleTimeout))
	return state, nil
}

// HasActiveRun exposes admission only; full run observations still require ownership.
func (s *Service[R, P]) HasActiveRun(ctx context.Context, actor string) (bool, error) {
	run, err := s.cfg.Store.FindActorRun(ctx, actor)
	return run != nil, err
}

// GetRunReceipt waits for an in-flight commit without advancing any game state.
// Transaction: WithRunForUpdate. Lock Order: run(0), no asset mutation.
func (s *Service[R, P]) GetRunReceipt(ctx context.Context, player, actor, runID, key string) (*Receipt, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	var receipt *Receipt
	err := s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		var err error
		receipt, err = s.cfg.Store.FindRunReceipt(ctx, runID, key)
		return err
	})
	if err != nil || receipt == nil {
		return receipt, err
	}
	if receipt.Command.PlayerID != player || receipt.Command.ActorID != actor {
		return nil, ErrForbidden
	}
	return receipt, nil
}

// WithWaitingLobby serializes roster writers with the one-way SQL handoff, even
// when the Valkey coordination lease expires during an in-flight admission.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> callback characters/assets(2+).
func (s *Service[R, P]) WithWaitingLobby(ctx context.Context, partyID string, fn func(context.Context) error) error {
	id := s.ID("party:" + partyID)
	return s.cfg.Store.WithRunForUpdate(ctx, id, true, func(ctx context.Context) error {
		a, err := s.cfg.Store.FindAdmission(ctx, id)
		if err != nil {
			return err
		}
		if a != nil {
			return ErrBusy
		}
		return fn(context.WithValue(ctx, admissionContext{}, id))
	})
}
