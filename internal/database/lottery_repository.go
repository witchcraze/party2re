package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/lottery"
)

type LotteryRepository struct {
	db *sql.DB
}

func NewLotteryRepository(db *sql.DB) (*LotteryRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &LotteryRepository{db: db}, nil
}

func (r *LotteryRepository) GetRaffleTickets(ctx context.Context, characterID string) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT raffle_tickets
		FROM character_lottery
		WHERE character_id = ?
	`, characterID).Scan(&count)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (r *LotteryRepository) AddRaffleTickets(ctx context.Context, characterID string, count int) (int, error) {
	if count <= 0 {
		return r.GetRaffleTickets(ctx, characterID)
	}

	executor := ExecutorFromContext(ctx, r.db)
	_, err := executor.ExecContext(ctx, `
		INSERT INTO character_lottery (character_id, raffle_tickets)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE raffle_tickets = raffle_tickets + VALUES(raffle_tickets)
	`, characterID, count)
	if err != nil {
		return 0, err
	}

	return r.GetRaffleTickets(ctx, characterID)
}

func (r *LotteryRepository) UseRaffleTickets(ctx context.Context, characterID string, count int) (int, error) {
	var currentTickets int

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Deduct raffle tickets
		res, err := executor.ExecContext(txCtx, `
			UPDATE character_lottery
			SET raffle_tickets = raffle_tickets - ?
			WHERE character_id = ? AND raffle_tickets >= ?
		`, count, characterID, count)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return lottery.ErrInsufficientTickets
		}

		// 2. Query updated state
		if err := executor.QueryRowContext(txCtx, `SELECT raffle_tickets FROM character_lottery WHERE character_id = ?`, characterID).Scan(&currentTickets); err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return 0, err
	}
	return currentTickets, nil
}

func (r *LotteryRepository) GetActiveTakarakujiRound(ctx context.Context) (lottery.TakarakujiRound, error) {
	var round lottery.TakarakujiRound
	var drawnAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round_id, draw_date, is_drawn, drawn_at,
		       prize_1_item_id, prize_1_amount,
		       prize_2_item_id, prize_2_amount,
		       prize_3_item_id, prize_3_amount,
		       created_at
		FROM takarakuji_rounds
		WHERE is_drawn = FALSE
		ORDER BY round_id ASC
		LIMIT 1
	`).Scan(
		&round.RoundID, &round.DrawDate, &round.IsDrawn, &drawnAt,
		&round.Prize1ItemID, &round.Prize1Amount,
		&round.Prize2ItemID, &round.Prize2Amount,
		&round.Prize3ItemID, &round.Prize3Amount,
		&round.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lottery.TakarakujiRound{}, lottery.ErrRoundNotFound
	}
	if err != nil {
		return lottery.TakarakujiRound{}, err
	}
	if drawnAt.Valid {
		round.DrawnAt = &drawnAt.Time
	}
	return round, nil
}

func (r *LotteryRepository) GetActiveTakarakujiRoundForUpdate(ctx context.Context) (lottery.TakarakujiRound, error) {
	var round lottery.TakarakujiRound
	var drawnAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT round_id, draw_date, is_drawn, drawn_at,
		       prize_1_item_id, prize_1_amount,
		       prize_2_item_id, prize_2_amount,
		       prize_3_item_id, prize_3_amount,
		       created_at
		FROM takarakuji_rounds
		WHERE is_drawn = FALSE
		ORDER BY round_id ASC
		LIMIT 1
		FOR UPDATE
	`).Scan(
		&round.RoundID, &round.DrawDate, &round.IsDrawn, &drawnAt,
		&round.Prize1ItemID, &round.Prize1Amount,
		&round.Prize2ItemID, &round.Prize2Amount,
		&round.Prize3ItemID, &round.Prize3Amount,
		&round.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return lottery.TakarakujiRound{}, lottery.ErrRoundNotFound
	}
	if err != nil {
		return lottery.TakarakujiRound{}, err
	}
	if drawnAt.Valid {
		round.DrawnAt = &drawnAt.Time
	}
	return round, nil
}

func (r *LotteryRepository) CreateTakarakujiRound(ctx context.Context, round lottery.TakarakujiRound) (lottery.TakarakujiRound, error) {
	executor := ExecutorFromContext(ctx, r.db)
	if round.CreatedAt.IsZero() {
		round.CreatedAt = time.Now().UTC()
	}

	res, err := executor.ExecContext(ctx, `
		INSERT INTO takarakuji_rounds (
			draw_date, is_drawn, drawn_at,
			prize_1_item_id, prize_1_amount,
			prize_2_item_id, prize_2_amount,
			prize_3_item_id, prize_3_amount,
			created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		round.DrawDate, round.IsDrawn, round.DrawnAt,
		round.Prize1ItemID, round.Prize1Amount,
		round.Prize2ItemID, round.Prize2Amount,
		round.Prize3ItemID, round.Prize3Amount,
		round.CreatedAt,
	)
	if err != nil {
		return lottery.TakarakujiRound{}, err
	}

	idVal, err := res.LastInsertId()
	if err != nil {
		return lottery.TakarakujiRound{}, err
	}
	round.RoundID = int(idVal)
	return round, nil
}

func (r *LotteryRepository) CountTakarakujiTickets(ctx context.Context, roundID int) (int, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM takarakuji_tickets WHERE round_id = ?
	`, roundID).Scan(&count)
	return count, err
}

func (r *LotteryRepository) HasCharacterPurchasedTakarakuji(ctx context.Context, roundID int, characterID string) (bool, error) {
	var count int
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT COUNT(*) FROM takarakuji_tickets WHERE round_id = ? AND character_id = ?
	`, roundID, characterID).Scan(&count)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *LotteryRepository) PurchaseTakarakujiTicket(ctx context.Context, roundID int, characterID string, goldCost int) (lottery.TakarakujiTicket, corecharacter.Character, error) {
	var ticket lottery.TakarakujiTicket
	var char corecharacter.Character

	err := RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		// 1. Lock round row for update (Rank 0) to serialize concurrent buyers and prevent over-selling
		var isDrawn bool
		err := executor.QueryRowContext(txCtx, `
			SELECT is_drawn FROM takarakuji_rounds WHERE round_id = ? FOR UPDATE
		`, roundID).Scan(&isDrawn)
		if errors.Is(err, sql.ErrNoRows) {
			return lottery.ErrRoundNotFound
		}
		if err != nil {
			return err
		}
		if isDrawn {
			return lottery.ErrAlreadyDrawn
		}

		// 2. Check sold out limit (20 tickets max)
		var soldCount int
		if err := executor.QueryRowContext(txCtx, `
			SELECT COUNT(*) FROM takarakuji_tickets WHERE round_id = ?
		`, roundID).Scan(&soldCount); err != nil {
			return err
		}
		if soldCount >= lottery.TakarakujiMaxTickets {
			return lottery.ErrSoldOut
		}

		// 3. Check duplicate purchase (1 per person)
		var boughtCount int
		if err := executor.QueryRowContext(txCtx, `
			SELECT COUNT(*) FROM takarakuji_tickets WHERE round_id = ? AND character_id = ?
		`, roundID, characterID).Scan(&boughtCount); err != nil {
			return err
		}
		if boughtCount > 0 {
			return lottery.ErrAlreadyPurchased
		}

		// 4. Deduct gold (Rank 2)
		res, err := executor.ExecContext(txCtx, `
			UPDATE characters
			SET money = money - ?
			WHERE id = ? AND money >= ?
		`, goldCost, characterID, goldCost)
		if err != nil {
			return err
		}
		affected, err := res.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return lottery.ErrInsufficientGold
		}

		// 5. Insert ticket
		ticket = lottery.TakarakujiTicket{
			ID:          id.New(),
			RoundID:     roundID,
			CharacterID: characterID,
			PurchasedAt: time.Now().UTC(),
			WonRank:     0,
		}

		_, err = executor.ExecContext(txCtx, `
			INSERT INTO takarakuji_tickets (id, round_id, character_id, purchased_at, won_rank, won_item_id)
			VALUES (?, ?, ?, ?, 0, NULL)
		`, ticket.ID, ticket.RoundID, ticket.CharacterID, ticket.PurchasedAt)
		if err != nil {
			return err
		}

		// 6. Scan updated character
		char, err = scanCharacterRow(executor.QueryRowContext(txCtx, `
			SELECT `+characterColumns+`
			FROM characters
			WHERE id = ?
		`, characterID))
		if err != nil {
			return err
		}

		return nil
	})
	if err != nil {
		return lottery.TakarakujiTicket{}, corecharacter.Character{}, err
	}
	return ticket, char, nil
}

func (r *LotteryRepository) GetCharacterTakarakujiTicket(ctx context.Context, roundID int, characterID string) (lottery.TakarakujiTicket, error) {
	var t lottery.TakarakujiTicket
	var wonItemID sql.NullString

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, round_id, character_id, purchased_at, won_rank, won_item_id
		FROM takarakuji_tickets
		WHERE round_id = ? AND character_id = ?
	`, roundID, characterID).Scan(&t.ID, &t.RoundID, &t.CharacterID, &t.PurchasedAt, &t.WonRank, &wonItemID)
	if errors.Is(err, sql.ErrNoRows) {
		return lottery.TakarakujiTicket{}, errors.New("ticket not found")
	}
	if err != nil {
		return lottery.TakarakujiTicket{}, err
	}
	if wonItemID.Valid {
		t.WonItemID = &wonItemID.String
	}
	return t, nil
}

func (r *LotteryRepository) ListCharacterTakarakujiTickets(ctx context.Context, characterID string) ([]lottery.TakarakujiTicket, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, round_id, character_id, purchased_at, won_rank, won_item_id
		FROM takarakuji_tickets
		WHERE character_id = ?
		ORDER BY purchased_at DESC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []lottery.TakarakujiTicket
	for rows.Next() {
		var t lottery.TakarakujiTicket
		var wonItemID sql.NullString
		if err := rows.Scan(&t.ID, &t.RoundID, &t.CharacterID, &t.PurchasedAt, &t.WonRank, &wonItemID); err != nil {
			return nil, err
		}
		if wonItemID.Valid {
			t.WonItemID = &wonItemID.String
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (r *LotteryRepository) ListRoundTakarakujiTickets(ctx context.Context, roundID int) ([]lottery.TakarakujiTicket, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, round_id, character_id, purchased_at, won_rank, won_item_id
		FROM takarakuji_tickets
		WHERE round_id = ?
		ORDER BY purchased_at ASC
	`, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []lottery.TakarakujiTicket
	for rows.Next() {
		var t lottery.TakarakujiTicket
		var wonItemID sql.NullString
		if err := rows.Scan(&t.ID, &t.RoundID, &t.CharacterID, &t.PurchasedAt, &t.WonRank, &wonItemID); err != nil {
			return nil, err
		}
		if wonItemID.Valid {
			t.WonItemID = &wonItemID.String
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (r *LotteryRepository) SettleTakarakujiRound(ctx context.Context, roundID int, drawnAt time.Time, winningTickets []lottery.TakarakujiTicket) error {
	return RunInTx(ctx, r.db, func(txCtx context.Context) error {
		executor := ExecutorFromContext(txCtx, r.db)

		_, err := executor.ExecContext(txCtx, `
			UPDATE takarakuji_rounds
			SET is_drawn = TRUE, drawn_at = ?
			WHERE round_id = ?
		`, drawnAt, roundID)
		if err != nil {
			return err
		}

		for _, t := range winningTickets {
			_, err := executor.ExecContext(txCtx, `
				UPDATE takarakuji_tickets
				SET won_rank = ?, won_item_id = ?
				WHERE id = ?
			`, t.WonRank, t.WonItemID, t.ID)
			if err != nil {
				return err
			}
		}

		return nil
	})
}
