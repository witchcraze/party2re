package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/replay"
)

// Staging locks only the run's source row, after the caller's run/character locks. Publication runs
// after commit and serializes per recipient on the slot row.

func (r *ReplayRepository) AppendScene(ctx context.Context, runID, commandID, actorID, encoded string, limit int64) (bool, error) {
	appended := false
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		if _, err := exec.ExecContext(txCtx, `INSERT IGNORE INTO replay_scene_sources (run_id) VALUES (?)`, runID); err != nil {
			return err
		}
		total, count, err := r.lockSceneSource(txCtx, runID)
		if err != nil {
			return err
		}
		var seq int
		err = exec.QueryRowContext(txCtx, `SELECT seq FROM replay_scenes WHERE run_id = ? AND command_id = ?`, runID, commandID).Scan(&seq)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return err // duplicate command identity keeps its original scene
		}
		if total > limit {
			return nil
		}
		if _, err := exec.ExecContext(txCtx,
			`INSERT INTO replay_scenes (run_id, seq, command_id, actor_id, scene_json, encoded_bytes) VALUES (?, ?, ?, ?, ?, ?)`,
			runID, count+1, commandID, actorID, encoded, len(encoded)); err != nil {
			return err
		}
		if _, err := exec.ExecContext(txCtx,
			`UPDATE replay_scene_sources SET encoded_bytes = encoded_bytes + ?, scene_count = scene_count + 1 WHERE run_id = ?`,
			len(encoded), runID); err != nil {
			return err
		}
		appended = true
		return nil
	})
	return appended, err
}

func (r *ReplayRepository) StageCopy(ctx context.Context, runID, commandID, recipientID string) (string, error) {
	var copyID string
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		_, count, err := r.lockSceneSource(txCtx, runID)
		if errors.Is(err, sql.ErrNoRows) {
			return nil // absent source preserves the previous slot
		}
		if err != nil {
			return err
		}
		err = exec.QueryRowContext(txCtx, `SELECT id FROM replay_archive_copies WHERE run_id = ? AND command_id = ?`, runID, commandID).Scan(&copyID)
		if err == nil || !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if count == 0 {
			return nil // empty source preserves the previous slot
		}
		copyID = id.New()
		_, err = exec.ExecContext(txCtx,
			`INSERT INTO replay_archive_copies (id, recipient_id, run_id, command_id, prefix_len) VALUES (?, ?, ?, ?, ?)`,
			copyID, recipientID, runID, commandID, count)
		return err
	})
	if err != nil {
		return "", err
	}
	return copyID, nil
}

func (r *ReplayRepository) lockSceneSource(ctx context.Context, runID string) (total int64, count int, err error) {
	err = ExecutorFromContext(ctx, r.db).QueryRowContext(ctx,
		`SELECT encoded_bytes, scene_count FROM replay_scene_sources WHERE run_id = ? FOR UPDATE`, runID).Scan(&total, &count)
	return total, count, err
}

func (r *ReplayRepository) PublishPending(ctx context.Context, recipientID string) error {
	// Create the per-recipient mutex row before locking: duplicate-key S locks inside one tx deadlock.
	if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`INSERT IGNORE INTO replay_latest_slots (recipient_id, copy_id, copy_seq) VALUES (?, '', 0)`, recipientID); err != nil {
		return err
	}
	var unreadable error
	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		exec := ExecutorFromContext(txCtx, r.db)
		unreadable = nil
		var slotSeq int64
		if err := exec.QueryRowContext(txCtx,
			`SELECT copy_seq FROM replay_latest_slots WHERE recipient_id = ? FOR UPDATE`, recipientID).Scan(&slotSeq); err != nil {
			return err
		}
		list, err := r.pendingCopies(txCtx, recipientID)
		if err != nil || len(list) == 0 {
			return err
		}
		// Like the original per-exit copy, each copy stands alone: the newest readable copy newer than
		// the slot wins, and newer unreadable copies stay pending without blocking it.
		chosen := -1
		for i := len(list) - 1; i >= 0 && list[i].seq > slotSeq; i-- {
			if err := r.checkPrefix(txCtx, list[i]); err != nil {
				if !errors.Is(err, replay.ErrArchiveSourceUnavailable) {
					return err
				}
				if unreadable == nil {
					unreadable = err
				}
				continue
			}
			chosen = i
			break
		}
		superseded := list
		if chosen >= 0 {
			c := list[chosen]
			if _, err := exec.ExecContext(txCtx,
				`UPDATE replay_latest_slots SET copy_id = ?, copy_seq = ? WHERE recipient_id = ?`, c.id, c.seq, recipientID); err != nil {
				return err
			}
			if err := r.setCopyStatus(txCtx, "published", list[chosen:chosen+1]); err != nil {
				return err
			}
			superseded = list[:chosen]
		} else {
			n := 0
			for n < len(list) && list[n].seq <= slotSeq {
				n++
			}
			superseded = list[:n] // late older work never replaces a newer published copy
		}
		return r.setCopyStatus(txCtx, "superseded", superseded)
	})
	if err != nil {
		return err
	}
	return unreadable
}

// checkPrefix verifies that every frozen scene of the copy decodes as a supported version.
func (r *ReplayRepository) checkPrefix(ctx context.Context, p pendingCopy) error {
	scenes, err := r.framePrefix(ctx, p.runID, p.prefixLen)
	if err != nil {
		return fmt.Errorf("copy %s: %w", p.id, err)
	}
	for _, raw := range scenes {
		if _, err := replay.DecodeStoredScene(raw); err != nil {
			return fmt.Errorf("copy %s: %w", p.id, err)
		}
	}
	return nil
}

func (r *ReplayRepository) setCopyStatus(ctx context.Context, status string, copies []pendingCopy) error {
	if len(copies) == 0 {
		return nil
	}
	args := []any{status}
	for _, c := range copies {
		args = append(args, c.id)
	}
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`UPDATE replay_archive_copies SET status = ? WHERE id IN (?`+strings.Repeat(",?", len(copies)-1)+`)`, args...)
	return err
}

type pendingCopy struct {
	seq       int64
	id, runID string
	prefixLen int
}

// pendingCopies runs under the slot lock, so a plain read cannot race another publisher.
func (r *ReplayRepository) pendingCopies(ctx context.Context, recipientID string) ([]pendingCopy, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx,
		`SELECT seq, id, run_id, prefix_len FROM replay_archive_copies
		 WHERE recipient_id = ? AND status = 'pending' ORDER BY seq`, recipientID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []pendingCopy
	for rows.Next() {
		var p pendingCopy
		if err := rows.Scan(&p.seq, &p.id, &p.runID, &p.prefixLen); err != nil {
			return nil, err
		}
		list = append(list, p)
	}
	return list, rows.Err()
}

func (r *ReplayRepository) FindLatest(ctx context.Context, recipientID string) (replay.StoredArchive, error) {
	exec := ExecutorFromContext(ctx, r.db)
	var out replay.StoredArchive
	if err := exec.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM replay_archive_copies c
		   WHERE c.recipient_id = ? AND c.status = 'pending'
		     AND c.seq > COALESCE((SELECT copy_seq FROM replay_latest_slots WHERE recipient_id = ?), 0))`,
		recipientID, recipientID).Scan(&out.Pending); err != nil {
		return replay.StoredArchive{}, err
	}
	var runID string
	var prefixLen int
	err := exec.QueryRowContext(ctx,
		`SELECT c.id, c.run_id, c.prefix_len FROM replay_latest_slots s
		 JOIN replay_archive_copies c ON c.id = s.copy_id WHERE s.recipient_id = ?`, recipientID).Scan(&out.CopyID, &runID, &prefixLen)
	if errors.Is(err, sql.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return replay.StoredArchive{}, err
	}
	if out.Scenes, err = r.framePrefix(ctx, runID, prefixLen); err != nil {
		return replay.StoredArchive{}, err
	}
	return out, nil
}

// framePrefix reads a copy's frozen scenes oldest first; scenes are immutable, so no snapshot is needed.
func (r *ReplayRepository) framePrefix(ctx context.Context, runID string, prefixLen int) ([]string, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx,
		`SELECT scene_json FROM replay_scenes WHERE run_id = ? AND seq <= ? ORDER BY seq`, runID, prefixLen)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var scenes []string
	for rows.Next() {
		var scene string
		if err := rows.Scan(&scene); err != nil {
			return nil, err
		}
		scenes = append(scenes, scene)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(scenes) != prefixLen {
		return nil, replay.ErrArchiveSourceUnavailable
	}
	return scenes, nil
}
