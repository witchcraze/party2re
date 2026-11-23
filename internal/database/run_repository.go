package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/runhost"
)

// RunRepository is the run host store for room library snapshot R. Every
// room kind shares the tables, so enrollment stays unique across kinds.
type RunRepository[R any, P runhost.Snapshot[R]] struct{ db *sql.DB }
type runLock struct{}

func NewRunRepository[R any, P runhost.Snapshot[R]](db *sql.DB) (*RunRepository[R, P], error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &RunRepository[R, P]{db: db}, nil
}

// WithRunForUpdate starts before character locks. Only enrollment may create a
// coordination row; disposed runs retain their header and independent receipts.
// Transaction: RunInTx. Lock Order: runs(0) -> callback assets(2+).
func (r *RunRepository[R, P]) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	if ctx.Value(runLock{}) == id {
		return fn(ctx)
	}
	if TxFromContext(ctx) != nil {
		return fmt.Errorf("run coordination must precede ambient asset locks")
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		e := ExecutorFromContext(ctx, r.db)
		if create {
			if _, err := e.ExecContext(ctx, `INSERT INTO runs (run_id) VALUES (?) ON DUPLICATE KEY UPDATE run_id=run_id`, id); err != nil {
				return err
			}
		}
		var version int
		err := e.QueryRowContext(ctx, `SELECT version FROM runs WHERE run_id=? FOR UPDATE`, id).Scan(&version)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return fn(context.WithValue(ctx, runLock{}, id))
	})
}

func (r *RunRepository[R, P]) FindRun(ctx context.Context, id string) (*R, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT state_json FROM runs WHERE run_id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var run R
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	if h := P(&run).Host(); h.ID != id || h.Encoding != 1 {
		return nil, runhost.ErrRecovery
	}
	return &run, nil
}

func (r *RunRepository[R, P]) FindAdmission(ctx context.Context, id string) (*runhost.Admission, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT admission_json FROM runs WHERE run_id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var admission runhost.Admission
	if err := json.Unmarshal(data, &admission); err != nil {
		return nil, err
	}
	return &admission, nil
}

func (r *RunRepository[R, P]) SaveRun(ctx context.Context, value R, expected int) error {
	run := P(&value).Host()
	if ctx.Value(runLock{}) != run.ID || run.Version != expected+1 || run.Encoding != 1 {
		return runhost.ErrConflict
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var state any = data
	if len(run.Roster) == 0 {
		state = nil
	}
	admission, err := json.Marshal(run.Admission)
	if err != nil {
		return err
	}
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `UPDATE runs SET version=?,admission_json=?,state_json=? WHERE run_id=? AND version=? AND (admission_json IS NULL OR BINARY admission_json=BINARY ?)`, run.Version, admission, state, run.ID, expected, admission)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return runhost.ErrConflict
	}
	return nil
}

func (r *RunRepository[R, P]) EnrollRun(ctx context.Context, run runhost.State) error {
	if ctx.Value(runLock{}) != run.ID {
		return runhost.ErrConflict
	}
	// These are paired enrollment/recipient inserts, not a same-table row-lock pass.
	ids := slices.Clone(run.Roster)
	slices.Sort(ids)
	e := ExecutorFromContext(ctx, r.db)
	for _, id := range ids {
		if _, err := e.ExecContext(ctx, `INSERT INTO run_enrollments (character_id,run_id) VALUES (?,?)`, id, run.ID); err != nil {
			var duplicate *mysql.MySQLError
			if errors.As(err, &duplicate) && duplicate.Number == 1062 {
				return runhost.ErrBusy
			}
			return err
		}
		data, err := json.Marshal(run.Recipients[id])
		if err != nil {
			return err
		}
		if _, err := e.ExecContext(ctx, `INSERT INTO run_recipients (run_id,character_id,roster_order,recipient_json) VALUES (?,?,?,?)`, run.ID, id, slices.Index(run.Roster, id), data); err != nil {
			return err
		}
	}
	return nil
}

func (r *RunRepository[R, P]) SaveRunRecipient(ctx context.Context, id, actor string, recipient runhost.Recipient) error {
	if ctx.Value(runLock{}) != id {
		return runhost.ErrConflict
	}
	data, err := json.Marshal(recipient)
	if err != nil {
		return err
	}
	var until any
	if !recipient.PenaltyUntil.IsZero() {
		until = recipient.PenaltyUntil
	}
	e := ExecutorFromContext(ctx, r.db)
	res, err := e.ExecContext(ctx, `UPDATE run_recipients SET disposition=?,penalty_until=?,recipient_json=? WHERE run_id=? AND character_id=?`, recipient.Disposition, until, data, id, actor)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists int
		if err := e.QueryRowContext(ctx, `SELECT 1 FROM run_recipients WHERE run_id=? AND character_id=?`, id, actor).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return runhost.ErrRecovery
			}
			return err
		}
	}
	if recipient.Disposition == "exit" || (recipient.Disposition == "timeout" && !recipient.TimerPending) {
		if _, err := e.ExecContext(ctx, `DELETE FROM run_enrollments WHERE run_id=? AND character_id=?`, id, actor); err != nil {
			return err
		}
	}
	return nil
}

func (r *RunRepository[R, P]) FindRunReceipt(ctx context.Context, id, key string) (*runhost.Receipt, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT receipt_json FROM run_receipts WHERE run_id=? AND command_id=?`, id, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt runhost.Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *RunRepository[R, P]) SaveRunReceipt(ctx context.Context, receipt runhost.Receipt) error {
	cmd := receipt.Command
	if ctx.Value(runLock{}) != cmd.RunID {
		return runhost.ErrConflict
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = ExecutorFromContext(ctx, r.db).ExecContext(ctx, `INSERT INTO run_receipts (run_id,command_id,actor_id,player_id,receipt_json) VALUES (?,?,?,?,?)`, cmd.RunID, cmd.ID, cmd.ActorID, cmd.PlayerID, data)
	return err
}

func (r *RunRepository[R, P]) FindActorRun(ctx context.Context, actor string) (*runhost.ActorRun, error) {
	var id string
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT e.run_id,r.state_json FROM run_enrollments e LEFT JOIN runs r ON r.run_id=e.run_id WHERE e.character_id=?`, actor).Scan(&id, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, runhost.ErrRecovery
	}
	var run runhost.State
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	if run.ID != id || run.Encoding != 1 {
		return nil, runhost.ErrRecovery
	}
	p, ok := run.Recipients[actor]
	if !ok || !slices.Contains(run.Roster, actor) {
		return nil, runhost.ErrRecovery
	}
	role := "member"
	if run.LeaderID == actor {
		role = "leader"
	}
	return &runhost.ActorRun{RunID: id, PartyID: run.Admission.PartyID, Version: run.Version, Round: run.Round, LeaderID: run.LeaderID, Role: role, LastActivity: run.LastActivity, Recipient: p}, nil
}

// FindRetainedEarnings returns every released timeout ledger row of the actor
// with unpaid unconditional earnings, oldest first; FindPenalty's latest-row read
// cannot see older balances. It is a plain read that precedes settlement; the
// rows are locked and re-verified by SaveRetainedRecipient after the settler.
func (r *RunRepository[R, P]) FindRetainedEarnings(ctx context.Context, actor string) ([]runhost.RetainedEarnings, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `SELECT p.run_id,p.recipient_json FROM run_recipients p
WHERE p.character_id=? AND p.disposition='timeout'
AND (JSON_EXTRACT(p.recipient_json,'$.pending.experience')>0 OR JSON_EXTRACT(p.recipient_json,'$.pending.gold')>0 OR JSON_EXTRACT(p.recipient_json,'$.pending.crystals')>0)
AND NOT EXISTS (SELECT 1 FROM run_enrollments e WHERE e.character_id=p.character_id AND e.run_id=p.run_id)
ORDER BY p.penalty_until, p.run_id`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []runhost.RetainedEarnings
	for rows.Next() {
		var entry runhost.RetainedEarnings
		var data []byte
		if err := rows.Scan(&entry.RunID, &data); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(data, &entry.Recipient); err != nil {
			return nil, err
		}
		found = append(found, entry)
	}
	return found, rows.Err()
}

// SaveRetainedRecipient rewrites only a released timeout ledger row, preserving
// its disposition and committed penalty deadline. It locks the row and requires
// the unpaid balance that was credited, so a changed ledger fails the command
// instead of paying twice or losing stock.
// Transaction: ambient command transaction. Lock Order: characters(2) -> settler(3/4/5) -> recipients(8).
func (r *RunRepository[R, P]) SaveRetainedRecipient(ctx context.Context, runID, actor string, credited runhost.Earnings, recipient runhost.Recipient) error {
	if TxFromContext(ctx) == nil {
		return errors.New("retained earnings require the command transaction")
	}
	e := ExecutorFromContext(ctx, r.db)
	var current []byte
	err := e.QueryRowContext(ctx, `SELECT p.recipient_json FROM run_recipients p
WHERE p.run_id=? AND p.character_id=? AND p.disposition='timeout'
AND NOT EXISTS (SELECT 1 FROM run_enrollments e WHERE e.character_id=p.character_id AND e.run_id=p.run_id)
FOR UPDATE`, runID, actor).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return runhost.ErrConflict
	}
	if err != nil {
		return err
	}
	var stored runhost.Recipient
	if err := json.Unmarshal(current, &stored); err != nil {
		return err
	}
	if stored.Pending != credited {
		return runhost.ErrConflict
	}
	data, err := json.Marshal(recipient)
	if err != nil {
		return err
	}
	_, err = e.ExecContext(ctx, `UPDATE run_recipients SET recipient_json=? WHERE run_id=? AND character_id=? AND disposition='timeout'`, data, runID, actor)
	return err
}

func (r *RunRepository[R, P]) FindPenalty(ctx context.Context, actor string) (*runhost.ActorRun, error) {
	var result runhost.ActorRun
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT run_id,recipient_json FROM run_recipients WHERE character_id=? AND disposition='timeout' ORDER BY penalty_until DESC LIMIT 1`, actor).Scan(&result.RunID, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, &result.Recipient); err != nil {
		return nil, err
	}
	return &result, nil
}
