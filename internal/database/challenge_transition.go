package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/challenge"
)

type challengeRunLock struct{}

// WithRunLock serializes computation, settlement and recovery of one run.
// Transaction: RunInTx. Lock Order: challenge_run_transitions(0), callback assets(2+).
func (r *ChallengeRepository) WithRunLock(ctx context.Context, id string, fn func(context.Context) error) error {
	if held, _ := ctx.Value(challengeRunLock{}).(string); held == id {
		return fn(ctx)
	}
	if TxFromContext(ctx) != nil {
		return fmt.Errorf("Challenge coordination must precede ambient asset locks")
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		e := ExecutorFromContext(ctx, r.db)
		if _, err := e.ExecContext(ctx, `INSERT INTO challenge_run_transitions (run_id) VALUES (?) ON DUPLICATE KEY UPDATE run_id=run_id`, id); err != nil {
			return err
		}
		var rev int
		if err := e.QueryRowContext(ctx, `SELECT revision FROM challenge_run_transitions WHERE run_id=? FOR UPDATE`, id).Scan(&rev); err != nil {
			return err
		}
		return fn(context.WithValue(ctx, challengeRunLock{}, id))
	})
}

func (r *ChallengeRepository) FindTransition(ctx context.Context, id string) (challenge.Transition, error) {
	var t challenge.Transition
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT revision,pending,publication_json FROM challenge_run_transitions WHERE run_id=?`, id).Scan(&t.Revision, &t.Pending, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return t, nil
	}
	if err != nil {
		return t, err
	}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &t); err != nil {
			return t, err
		}
	}
	t.RunID = id
	return t, nil
}
func (r *ChallengeRepository) FindClaim(ctx context.Context, actor string) (string, error) {
	var id string
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT run_id FROM challenge_character_claims WHERE character_id=?`, actor).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		err = ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT s.id FROM challenge_sessions s
            LEFT JOIN challenge_run_transitions t ON t.run_id=s.id
            WHERE s.status='active' AND COALESCE(t.revision,0)=0
            AND (s.character_id=? OR JSON_CONTAINS(s.members_json,JSON_OBJECT('character_id',?))) LIMIT 1`, actor, actor).Scan(&id)
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
	}
	return id, err
}
func (r *ChallengeRepository) FindCommandResult(ctx context.Context, id, actor string, revision int) (*challenge.CommandResult, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT result_json FROM challenge_command_results WHERE run_id=? AND actor_id=? AND expected_revision=?`, id, actor, revision).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var result challenge.CommandResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

func (r *ChallengeRepository) EnrollRun(ctx context.Context, s challenge.ChallengeSession) error {
	if ctx.Value(challengeRunLock{}) != s.ID {
		return challenge.ErrRevisionConflict
	}
	e := ExecutorFromContext(ctx, r.db)
	for _, m := range s.Members {
		if _, err := e.ExecContext(ctx, `INSERT INTO challenge_character_claims (character_id,run_id) VALUES (?,?)`, m.CharacterID, s.ID); err != nil {
			var duplicate *mysql.MySQLError
			if errors.As(err, &duplicate) && duplicate.Number == 1062 {
				return challenge.ErrActiveSessionExists
			}
			return err
		}
		if _, err := e.ExecContext(ctx, `INSERT INTO challenge_participants (run_id,character_id) VALUES (?,?)`, s.ID, m.CharacterID); err != nil {
			return err
		}
	}
	return nil
}

func (r *ChallengeRepository) SaveTransition(ctx context.Context, t challenge.Transition, result *challenge.CommandResult) error {
	if ctx.Value(challengeRunLock{}) != t.RunID || t.Revision < 1 || !t.Pending || t.Publication == nil {
		return challenge.ErrRevisionConflict
	}
	data, err := json.Marshal(t)
	if err != nil {
		return err
	}
	e := ExecutorFromContext(ctx, r.db)
	updated, err := e.ExecContext(ctx, `UPDATE challenge_run_transitions SET revision=?,pending=TRUE,publication_json=? WHERE run_id=? AND revision=? AND pending=FALSE`, t.Revision, data, t.RunID, t.Revision-1)
	if err != nil {
		return err
	}
	n, err := updated.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return challenge.ErrRevisionConflict
	}
	if result != nil {
		data, err := json.Marshal(result)
		if err != nil {
			return err
		}
		if _, err := e.ExecContext(ctx, `INSERT INTO challenge_command_results (run_id,actor_id,expected_revision,result_json) VALUES (?,?,?,?)`, t.RunID, result.ActorID, result.ExpectedRevision, data); err != nil {
			return err
		}
	}
	updated, err = e.ExecContext(ctx, `UPDATE challenge_sessions SET status=IF(?='active',status,?),write_protocol=write_protocol+1 WHERE id=?`, t.Publication.Status, t.Publication.Status, t.RunID)
	if err != nil {
		return err
	}
	n, err = updated.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return challenge.ErrSessionNotFound
	}
	return nil
}

// AcknowledgePublication releases only this run's departed claims after publication.
// Transaction: WithRunLock / RunInTx. Lock Order: coordination(0) -> claims(8).
func (r *ChallengeRepository) AcknowledgePublication(ctx context.Context, id string, rev int) error {
	if ctx.Value(challengeRunLock{}) != id {
		return challenge.ErrRevisionConflict
	}
	t, err := r.FindTransition(ctx, id)
	if err != nil {
		return err
	}
	if t.Revision != rev {
		return challenge.ErrRevisionConflict
	}
	e := ExecutorFromContext(ctx, r.db)
	for _, actor := range t.RemovedIDs {
		if _, err := e.ExecContext(ctx, `DELETE FROM challenge_character_claims WHERE character_id=? AND run_id=?`, actor, id); err != nil {
			return err
		}
	}
	_, err = e.ExecContext(ctx, `UPDATE challenge_run_transitions SET pending=FALSE,publication_json=NULL WHERE run_id=? AND revision=?`, id, rev)
	return err
}
