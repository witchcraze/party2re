package adventure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
)

func normalRunID(generation string) string {
	h := sha256.Sum256([]byte("normal-adventure:" + generation))
	return hex.EncodeToString(h[:16])
}

func (s *NormalRunService) authorize(ctx context.Context, player, actor string) error {
	if player == "" || actor == "" {
		return ErrRunForbidden
	}
	c, err := s.cfg.Characters.FindByID(ctx, actor)
	if err != nil {
		return err
	}
	if c.ID != actor || c.PlayerID != player {
		return ErrRunForbidden
	}
	return nil
}

// EnterSolo enrolls without creating an encounter. The caller retains key on retries.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> characters(2, sorted) -> enrollment/recipients(8).
func (s *NormalRunService) EnterSolo(ctx context.Context, player, actor, key, stageID, mode string, speed int) (RunReceipt, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return RunReceipt{}, err
	}
	if strings.TrimSpace(key) == "" || len(key) > 128 {
		return RunReceipt{}, ErrRunConflict
	}
	return s.enter(ctx, player, NormalAdmission{Generation: "solo:" + actor + ":" + key, ActorID: actor, CharacterIDs: []string{actor}, StageID: stageID, Mode: mode, Speed: speed})
}

// EnterParty resolves committed admission before reading the expiring lobby.
// Transaction: WithRunForUpdate, inside waiting admission coordination; SQL commit precedes lobby cleanup.
func (s *NormalRunService) EnterParty(ctx context.Context, player, actor, partyID string) (RunReceipt, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return RunReceipt{}, err
	}
	if partyID == "" || s.cfg.Admission == nil {
		return RunReceipt{}, ErrRunUnavailable
	}
	generation := "party:" + partyID
	a, err := s.cfg.Store.FindAdmission(ctx, normalRunID(generation))
	if err != nil {
		return RunReceipt{}, err
	}
	if a != nil {
		if a.ActorID != actor {
			return RunReceipt{}, ErrRunForbidden
		}
		result, err := s.enter(ctx, player, *a)
		if err == nil {
			err = s.cfg.Admission.CleanupNormalLobby(ctx, *a)
		}
		return result, err
	}
	var result RunReceipt
	err = s.cfg.Admission.WithNormalRoster(ctx, actor, partyID, func(ctx context.Context, a NormalAdmission) error {
		if a.PartyID != partyID || a.ActorID != actor || a.Generation != generation {
			return ErrRunConflict
		}
		var err error
		result, err = s.enter(ctx, player, a)
		return err
	})
	if err != nil {
		committed, readErr := s.cfg.Store.FindAdmission(ctx, normalRunID(generation))
		if readErr != nil {
			return RunReceipt{}, errors.Join(err, readErr)
		}
		if committed == nil {
			return RunReceipt{}, err
		}
		if committed.ActorID != actor {
			return RunReceipt{}, ErrRunForbidden
		}
		result, err = s.enter(ctx, player, *committed)
		if err != nil {
			return RunReceipt{}, err
		}
	}
	a, err = s.cfg.Store.FindAdmission(ctx, normalRunID(generation))
	if err != nil {
		return result, err
	}
	if a == nil {
		return result, ErrRunRecovery
	}
	return result, s.cfg.Admission.CleanupNormalLobby(ctx, *a)
}

func (s *NormalRunService) enter(ctx context.Context, player string, a NormalAdmission) (RunReceipt, error) {
	mode, err := ValidateCrawlMode(a.Mode)
	if err != nil {
		return RunReceipt{}, err
	}
	a.Mode = mode
	a.StageID = strings.TrimSpace(a.StageID)
	if a.Speed != 3 && a.Speed != 18 && a.Speed != 25 {
		return RunReceipt{}, ErrRunConflict
	}
	if len(a.CharacterIDs) < 1 || len(a.CharacterIDs) > 4 || a.CharacterIDs[0] != a.ActorID {
		return RunReceipt{}, ErrNoParticipants
	}
	ids := slices.Clone(a.CharacterIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if id == "" || (i > 0 && ids[i-1] == id) {
			return RunReceipt{}, ErrRunConflict
		}
	}
	input, err := json.Marshal(a)
	if err != nil {
		return RunReceipt{}, err
	}
	cmd := RunCommand{PlayerID: player, ActorID: a.ActorID, RunID: normalRunID(a.Generation), ID: "enroll", Action: "enroll", Input: string(input)}
	var result RunReceipt
	entryLocked := false
	err = s.cfg.Store.WithRunForUpdate(ctx, cmd.RunID, true, func(ctx context.Context) error {
		old, err := s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		if err != nil {
			return err
		}
		if old != nil {
			if old.Command != cmd {
				return ErrRunConflict
			}
			result = *old
			return nil
		}
		r := NormalRun{ID: cmd.RunID, Encoding: 1, Version: 1, Admission: a, Mode: mode, LeaderID: a.ActorID, Roster: slices.Clone(a.CharacterIDs), LastActions: map[string]time.Time{}, Recipients: map[string]RunRecipient{}, StartedAt: s.cfg.Clock.Now(), LastActivity: s.cfg.Clock.Now()}
		for _, id := range ids {
			c, err := s.cfg.Characters.FindByIDForUpdate(ctx, id)
			if err != nil {
				return err
			}
			if c.PendingWake {
				return ErrRunBusy
			}
			if c.Stats.HP <= 0 {
				return ErrCharacterUnconscious
			}
			if c.Tired >= 100 {
				return ErrCharacterExhausted
			}
			if id == a.ActorID && c.PlayerID != player {
				return ErrRunForbidden
			}
			r.Recipients[id] = RunRecipient{PlayerID: c.PlayerID}
		}
		for _, id := range ids {
			active, err := s.cfg.Store.FindActorRun(ctx, id)
			if err != nil {
				return err
			}
			if active != nil {
				return ErrRunBusy
			}
			if s.cfg.CheckEntry != nil {
				if err := s.cfg.CheckEntry(ctx, id); err != nil {
					return err
				}
			}
			c, err := s.cfg.Characters.FindByID(ctx, id)
			if err != nil {
				return err
			}
			if err := s.cfg.Stages.CanAccessStageAt(c, a.StageID, r.StartedAt); err != nil {
				return err
			}
		}
		if a.PartyID == "" && s.cfg.Admission != nil {
			if err := s.cfg.Admission.CheckSoloAdmission(ctx, a.ActorID); err != nil {
				return err
			}
		}
		if a.StageID == "stage-26" && a.PartyID == "" {
			if s.cfg.EntryLocks == nil {
				return ErrRunUnavailable
			}
			// A failed earlier attempt may own this token but no committed receipt.
			// The admission lock and receipt check make only that token releasable.
			if _, err := s.cfg.EntryLocks.ReleaseLockWithToken(ctx, timer.CategoryDungeonOnce, a.ActorID, cmd.RunID); err != nil {
				return err
			}
			ok, err := s.cfg.EntryLocks.TryLockWithToken(ctx, timer.CategoryDungeonOnce, a.ActorID, cmd.RunID, 0)
			if err != nil {
				return err
			}
			if !ok {
				return ErrDailyOnceAlreadyUsed
			}
			entryLocked = true
		}
		stage, err := s.cfg.Stages.FindByID(a.StageID)
		if err != nil {
			return err
		}
		season := a.Season
		if a.PartyID == "" && season == "" {
			// Solo room creation selects its season here; Party selected it at lobby creation.
			season = stage.SelectSeason(s.cfg.RNG.Intn).Season
		}
		r.Stage = stage.OrdinarySeason(season)
		r.RecordSeason = r.Stage.Season
		// quest.cgi:596–600: one EX draw after season selection, never repeated by travel.
		r.EXUnlocked = r.Stage.ID == "stage-26" && s.cfg.RNG.Intn(3) == 0
		for _, id := range a.CharacterIDs {
			actor, err := s.cfg.Builder.BuildLifetimeActor(ctx, id, "")
			if err != nil {
				return err
			}
			actor.TeamID = "allies"
			r.Battle.Actors = append(r.Battle.Actors, actor)
		}
		if err := s.cfg.Store.EnrollRun(ctx, r); err != nil {
			return err
		}
		if err := s.cfg.Store.SaveRun(ctx, r, 0); err != nil {
			return err
		}
		result = RunReceipt{Command: cmd, Version: 1, StageID: r.Stage.ID, Recipient: r.Recipients[a.ActorID]}
		return s.cfg.Store.SaveRunReceipt(ctx, result)
	})
	// The outer waiting-admission transaction must resolve before receipts can prove commit.
	if ctx.Value(normalAdmissionContext{}) == cmd.RunID {
		return result, err
	}
	result, err = s.resolveReceipt(ctx, cmd, result, err)
	if err != nil && entryLocked && !errors.Is(err, ErrRunRecovery) {
		rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if _, releaseErr := s.cfg.EntryLocks.ReleaseLockWithToken(rctx, timer.CategoryDungeonOnce, a.ActorID, cmd.RunID); releaseErr != nil {
			return RunReceipt{}, errors.Join(ErrRunRecovery, err, releaseErr)
		}
	}
	return result, err
}

func (s *NormalRunService) resolveReceipt(ctx context.Context, cmd RunCommand, result RunReceipt, cause error) (RunReceipt, error) {
	if cause == nil {
		return result, nil
	}
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	var saved *RunReceipt
	// Serialize the read against the transaction that may still be committing.
	err := s.cfg.Store.WithRunForUpdate(rctx, cmd.RunID, false, func(ctx context.Context) error {
		var err error
		saved, err = s.cfg.Store.FindRunReceipt(ctx, cmd.RunID, cmd.ID)
		return err
	})
	if err != nil {
		return RunReceipt{}, errors.Join(ErrRunRecovery, cause, err)
	}
	if saved == nil || saved.Command != cmd {
		return RunReceipt{}, cause
	}
	return *saved, nil
}

func (s *NormalRunService) GetActorRun(ctx context.Context, player, actor string) (*ActorRun, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	state, err := s.cfg.Store.FindActorRun(ctx, actor)
	if err != nil || state == nil {
		return state, err
	}
	if state.Recipient.PlayerID != player {
		return nil, ErrRunForbidden
	}
	state.IdleDue = s.cfg.Clock.Now().After(state.LastActivity.Add(1800 * time.Second))
	return state, nil
}

// GetRunReceipt waits for an in-flight commit without advancing any game state.
// Transaction: WithRunForUpdate. Lock Order: run(0), no asset mutation.
func (s *NormalRunService) GetRunReceipt(ctx context.Context, player, actor, runID, key string) (*RunReceipt, error) {
	if err := s.authorize(ctx, player, actor); err != nil {
		return nil, err
	}
	var receipt *RunReceipt
	err := s.cfg.Store.WithRunForUpdate(ctx, runID, false, func(ctx context.Context) error {
		var err error
		receipt, err = s.cfg.Store.FindRunReceipt(ctx, runID, key)
		return err
	})
	if err != nil || receipt == nil {
		return receipt, err
	}
	if receipt.Command.PlayerID != player || receipt.Command.ActorID != actor {
		return nil, ErrRunForbidden
	}
	return receipt, nil
}

// WithWaitingLobby serializes roster writers with the one-way SQL handoff, even
// when the Valkey coordination lease expires during an in-flight admission.
// Transaction: WithRunForUpdate. Lock Order: run(0) -> callback characters/assets(2+).
func (s *NormalRunService) WithWaitingLobby(ctx context.Context, partyID string, fn func(context.Context) error) error {
	id := normalRunID("party:" + partyID)
	return s.cfg.Store.WithRunForUpdate(ctx, id, true, func(ctx context.Context) error {
		a, err := s.cfg.Store.FindAdmission(ctx, id)
		if err != nil {
			return err
		}
		if a != nil {
			return ErrRunBusy
		}
		return fn(context.WithValue(ctx, normalAdmissionContext{}, id))
	})
}

// livingEnemies uses persisted encounter membership, never coordinate-derived IDs.
func (r *NormalRun) livingEnemies() bool {
	for _, box := range r.Boxes {
		if box.OpenedBy == "" {
			return true
		}
	}
	for _, id := range r.Enemies {
		if a := r.actor(id); a != nil && a.HP > 0 {
			return true
		}
	}
	return false
}

func runReward(e RunEarnings) corebattle.Reward {
	return corebattle.Reward{Experience: e.EXP, Currency: e.Gold, Crystals: e.Crystals}
}
