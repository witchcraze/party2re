package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/exile"
)

// ExileRepository stores the shared exile board.
type ExileRepository struct {
	db *sql.DB
}

func NewExileRepository(db *sql.DB) (*ExileRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &ExileRepository{db: db}, nil
}

// LockBoard locks the singleton board row; callers must hold a transaction.
func (r *ExileRepository) LockBoard(ctx context.Context) (time.Time, error) {
	var updated sql.NullTime
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `SELECT updated_at FROM exile_board WHERE id = 'global' FOR UPDATE`).Scan(&updated)
	if err != nil {
		return time.Time{}, err
	}
	return updated.Time, nil
}

func (r *ExileRepository) TouchBoard(ctx context.Context, at time.Time) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `UPDATE exile_board SET updated_at = ? WHERE id = 'global'`, at.UTC())
	return err
}

func (r *ExileRepository) ListPetitions(ctx context.Context) ([]exile.Petition, error) {
	exec := ExecutorFromContext(ctx, r.db)
	rows, err := exec.QueryContext(ctx, `
		SELECT id, applicant_id, applicant_name, target_id, target_name, reason, created_at
		FROM exile_petitions ORDER BY seq`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	petitions := []exile.Petition{}
	index := map[string]int{}
	for rows.Next() {
		var p exile.Petition
		if err := rows.Scan(&p.ID, &p.ApplicantID, &p.ApplicantName, &p.TargetID, &p.TargetName, &p.Reason, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.Votes = []exile.Vote{}
		index[p.ID] = len(petitions)
		petitions = append(petitions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	votes, err := exec.QueryContext(ctx, `SELECT petition_id, voter_id, voter_name, approve, voted_at FROM exile_votes ORDER BY seq DESC`)
	if err != nil {
		return nil, err
	}
	defer votes.Close()
	for votes.Next() {
		var petitionID string
		var v exile.Vote
		if err := votes.Scan(&petitionID, &v.VoterID, &v.VoterName, &v.Approve, &v.VotedAt); err != nil {
			return nil, err
		}
		if i, ok := index[petitionID]; ok {
			petitions[i].Votes = append(petitions[i].Votes, v)
		}
	}
	return petitions, votes.Err()
}

// CreatePetition inserts the petition and its recorded votes (oldest last).
func (r *ExileRepository) CreatePetition(ctx context.Context, p exile.Petition) error {
	return RunInTx(ctx, r.db, func(ctx context.Context) error {
		if _, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
			INSERT INTO exile_petitions (id, applicant_id, applicant_name, target_id, target_name, reason, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			p.ID, p.ApplicantID, p.ApplicantName, p.TargetID, p.TargetName, p.Reason, p.CreatedAt.UTC()); err != nil {
			return err
		}
		for i := len(p.Votes) - 1; i >= 0; i-- {
			if err := r.AddVote(ctx, p.ID, p.Votes[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

func (r *ExileRepository) AddVote(ctx context.Context, petitionID string, v exile.Vote) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO exile_votes (petition_id, voter_id, voter_name, approve, voted_at) VALUES (?, ?, ?, ?, ?)`,
		petitionID, v.VoterID, v.VoterName, v.Approve, v.VotedAt.UTC())
	return err
}

func (r *ExileRepository) DeletePetition(ctx context.Context, petitionID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `DELETE FROM exile_petitions WHERE id = ?`, petitionID)
	return err
}

func (r *ExileRepository) IsInvolved(ctx context.Context, characterID string) (bool, error) {
	var involved bool
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT EXISTS (SELECT 1 FROM exile_petitions WHERE applicant_id = ? OR target_id = ?)
		    OR EXISTS (SELECT 1 FROM exile_votes WHERE voter_id = ?)`,
		characterID, characterID, characterID).Scan(&involved)
	return involved, err
}

// SaveSentence records a committed sleep deadline, replacing an earlier one.
func (r *ExileRepository) SaveSentence(ctx context.Context, s exile.Sentence) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO exile_sentences (character_id, sleep_until) VALUES (?, ?)
		ON DUPLICATE KEY UPDATE sleep_until = VALUES(sleep_until)`, s.CharacterID, s.SleepUntil.UTC())
	return err
}

func (r *ExileRepository) ListSentences(ctx context.Context) ([]exile.Sentence, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `SELECT character_id, sleep_until FROM exile_sentences`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []exile.Sentence
	for rows.Next() {
		var s exile.Sentence
		if err := rows.Scan(&s.CharacterID, &s.SleepUntil); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// DeleteSentence removes the row only if it still holds the projected deadline.
func (r *ExileRepository) DeleteSentence(ctx context.Context, s exile.Sentence) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx,
		`DELETE FROM exile_sentences WHERE character_id = ? AND sleep_until = ?`, s.CharacterID, s.SleepUntil.UTC())
	return err
}
