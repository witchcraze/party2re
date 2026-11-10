package database

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/adventure"
)

type NormalAdventureRepository struct{ db *sql.DB }
type normalAdventureLock struct{}

func NewNormalAdventureRepository(db *sql.DB) (*NormalAdventureRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &NormalAdventureRepository{db: db}, nil
}

// WithRunForUpdate starts before character locks. Only enrollment may create a
// coordination row; disposed runs retain their header and independent receipts.
// Transaction: RunInTx. Lock Order: normal_adventure_runs(0) -> callback assets(2+).
func (r *NormalAdventureRepository) WithRunForUpdate(ctx context.Context, id string, create bool, fn func(context.Context) error) error {
	if ctx.Value(normalAdventureLock{}) == id {
		return fn(ctx)
	}
	if TxFromContext(ctx) != nil {
		return fmt.Errorf("normal run coordination must precede ambient asset locks")
	}
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		e := ExecutorFromContext(ctx, r.db)
		if create {
			if _, err := e.ExecContext(ctx, `INSERT INTO normal_adventure_runs (run_id) VALUES (?) ON DUPLICATE KEY UPDATE run_id=run_id`, id); err != nil {
				return err
			}
		}
		var version int
		err := e.QueryRowContext(ctx, `SELECT version FROM normal_adventure_runs WHERE run_id=? FOR UPDATE`, id).Scan(&version)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		return fn(context.WithValue(ctx, normalAdventureLock{}, id))
	})
}

func (r *NormalAdventureRepository) FindRun(ctx context.Context, id string) (*adventure.NormalRun, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT state_json FROM normal_adventure_runs WHERE run_id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var run adventure.NormalRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	if run.ID != id || run.Encoding != 1 {
		return nil, adventure.ErrRunRecovery
	}
	return &run, nil
}

func (r *NormalAdventureRepository) FindAdmission(ctx context.Context, id string) (*adventure.NormalAdmission, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT admission_json FROM normal_adventure_runs WHERE run_id=?`, id).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && len(data) == 0) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var admission adventure.NormalAdmission
	if err := json.Unmarshal(data, &admission); err != nil {
		return nil, err
	}
	return &admission, nil
}

func (r *NormalAdventureRepository) SaveRun(ctx context.Context, run adventure.NormalRun, expected int) error {
	if ctx.Value(normalAdventureLock{}) != run.ID || run.Version != expected+1 || run.Encoding != 1 {
		return adventure.ErrRunConflict
	}
	data, err := json.Marshal(run)
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
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `UPDATE normal_adventure_runs SET version=?,admission_json=?,state_json=? WHERE run_id=? AND version=? AND (admission_json IS NULL OR BINARY admission_json=BINARY ?)`, run.Version, admission, state, run.ID, expected, admission)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return adventure.ErrRunConflict
	}
	return nil
}

func (r *NormalAdventureRepository) EnrollRun(ctx context.Context, run adventure.NormalRun) error {
	if ctx.Value(normalAdventureLock{}) != run.ID {
		return adventure.ErrRunConflict
	}
	// These are paired enrollment/recipient inserts, not a same-table row-lock pass.
	ids := slices.Clone(run.Roster)
	slices.Sort(ids)
	e := ExecutorFromContext(ctx, r.db)
	for _, id := range ids {
		if _, err := e.ExecContext(ctx, `INSERT INTO normal_adventure_enrollments (character_id,run_id) VALUES (?,?)`, id, run.ID); err != nil {
			var duplicate *mysql.MySQLError
			if errors.As(err, &duplicate) && duplicate.Number == 1062 {
				return adventure.ErrRunBusy
			}
			return err
		}
		data, err := json.Marshal(run.Recipients[id])
		if err != nil {
			return err
		}
		if _, err := e.ExecContext(ctx, `INSERT INTO normal_adventure_recipients (run_id,character_id,roster_order,recipient_json) VALUES (?,?,?,?)`, run.ID, id, slices.Index(run.Roster, id), data); err != nil {
			return err
		}
	}
	return nil
}

func (r *NormalAdventureRepository) SaveRunRecipient(ctx context.Context, id, actor string, recipient adventure.RunRecipient) error {
	if ctx.Value(normalAdventureLock{}) != id {
		return adventure.ErrRunConflict
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
	res, err := e.ExecContext(ctx, `UPDATE normal_adventure_recipients SET disposition=?,penalty_until=?,recipient_json=? WHERE run_id=? AND character_id=?`, recipient.Disposition, until, data, id, actor)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var exists int
		if err := e.QueryRowContext(ctx, `SELECT 1 FROM normal_adventure_recipients WHERE run_id=? AND character_id=?`, id, actor).Scan(&exists); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return adventure.ErrRunRecovery
			}
			return err
		}
	}
	if recipient.Disposition == "exit" || (recipient.Disposition == "timeout" && !recipient.TimerPending) {
		if _, err := e.ExecContext(ctx, `DELETE FROM normal_adventure_enrollments WHERE run_id=? AND character_id=?`, id, actor); err != nil {
			return err
		}
	}
	return nil
}

func (r *NormalAdventureRepository) FindRunReceipt(ctx context.Context, id, key string) (*adventure.RunReceipt, error) {
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT receipt_json FROM normal_adventure_receipts WHERE run_id=? AND command_id=?`, id, key).Scan(&data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var receipt adventure.RunReceipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	return &receipt, nil
}

func (r *NormalAdventureRepository) SaveRunReceipt(ctx context.Context, receipt adventure.RunReceipt) error {
	cmd := receipt.Command
	if ctx.Value(normalAdventureLock{}) != cmd.RunID {
		return adventure.ErrRunConflict
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		return err
	}
	_, err = ExecutorFromContext(ctx, r.db).ExecContext(ctx, `INSERT INTO normal_adventure_receipts (run_id,command_id,actor_id,player_id,receipt_json) VALUES (?,?,?,?,?)`, cmd.RunID, cmd.ID, cmd.ActorID, cmd.PlayerID, data)
	return err
}

func (r *NormalAdventureRepository) FindActorRun(ctx context.Context, actor string) (*adventure.ActorRun, error) {
	var id string
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT e.run_id,r.state_json FROM normal_adventure_enrollments e LEFT JOIN normal_adventure_runs r ON r.run_id=e.run_id WHERE e.character_id=?`, actor).Scan(&id, &data)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if len(data) == 0 {
		return nil, adventure.ErrRunRecovery
	}
	var run adventure.NormalRun
	if err := json.Unmarshal(data, &run); err != nil {
		return nil, err
	}
	if run.ID != id || run.Encoding != 1 {
		return nil, adventure.ErrRunRecovery
	}
	p, ok := run.Recipients[actor]
	if !ok || !slices.Contains(run.Roster, actor) {
		return nil, adventure.ErrRunRecovery
	}
	role := "member"
	if run.LeaderID == actor {
		role = "leader"
	}
	return &adventure.ActorRun{RunID: id, PartyID: run.Admission.PartyID, Version: run.Version, Floor: run.Floor, LeaderID: run.LeaderID, Role: role, LastActivity: run.LastActivity, Recipient: p}, nil
}

// FindRetainedEarnings returns every released timeout ledger row of the actor
// with unpaid unconditional earnings, oldest first; FindPenalty's latest-row read
// cannot see older balances. It is a plain read that precedes settlement; the
// rows are locked and re-verified by SaveRetainedRecipient after the settler.
func (r *NormalAdventureRepository) FindRetainedEarnings(ctx context.Context, actor string) ([]adventure.RetainedEarnings, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `SELECT p.run_id,p.recipient_json FROM normal_adventure_recipients p
WHERE p.character_id=? AND p.disposition='timeout'
AND (JSON_EXTRACT(p.recipient_json,'$.pending.experience')>0 OR JSON_EXTRACT(p.recipient_json,'$.pending.gold')>0 OR JSON_EXTRACT(p.recipient_json,'$.pending.crystals')>0)
AND NOT EXISTS (SELECT 1 FROM normal_adventure_enrollments e WHERE e.character_id=p.character_id AND e.run_id=p.run_id)
ORDER BY p.penalty_until, p.run_id`, actor)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []adventure.RetainedEarnings
	for rows.Next() {
		var entry adventure.RetainedEarnings
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
func (r *NormalAdventureRepository) SaveRetainedRecipient(ctx context.Context, runID, actor string, credited adventure.RunEarnings, recipient adventure.RunRecipient) error {
	if TxFromContext(ctx) == nil {
		return errors.New("retained earnings require the command transaction")
	}
	e := ExecutorFromContext(ctx, r.db)
	var current []byte
	err := e.QueryRowContext(ctx, `SELECT p.recipient_json FROM normal_adventure_recipients p
WHERE p.run_id=? AND p.character_id=? AND p.disposition='timeout'
AND NOT EXISTS (SELECT 1 FROM normal_adventure_enrollments e WHERE e.character_id=p.character_id AND e.run_id=p.run_id)
FOR UPDATE`, runID, actor).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return adventure.ErrRunConflict
	}
	if err != nil {
		return err
	}
	var stored adventure.RunRecipient
	if err := json.Unmarshal(current, &stored); err != nil {
		return err
	}
	if stored.Pending != credited {
		return adventure.ErrRunConflict
	}
	data, err := json.Marshal(recipient)
	if err != nil {
		return err
	}
	_, err = e.ExecContext(ctx, `UPDATE normal_adventure_recipients SET recipient_json=? WHERE run_id=? AND character_id=? AND disposition='timeout'`, data, runID, actor)
	return err
}

func (r *NormalAdventureRepository) FindPenalty(ctx context.Context, actor string) (*adventure.ActorRun, error) {
	var result adventure.ActorRun
	var data []byte
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT run_id,recipient_json FROM normal_adventure_recipients WHERE character_id=? AND disposition='timeout' ORDER BY penalty_until DESC LIMIT 1`, actor).Scan(&result.RunID, &data)
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
