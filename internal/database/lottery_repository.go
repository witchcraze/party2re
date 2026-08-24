package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
	err := r.db.QueryRowContext(ctx, `
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

func (r *LotteryRepository) BuyRaffleTickets(ctx context.Context, characterID string, count int, goldCost int) (int, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Deduct gold
	res, err := tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money - ?
		WHERE id = ? AND money >= ?
	`, goldCost, characterID, goldCost)
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	if affected == 0 {
		return 0, corecharacter.Character{}, lottery.ErrInsufficientGold
	}

	// 2. Upsert raffle tickets
	_, err = tx.ExecContext(ctx, `
		INSERT INTO character_lottery (character_id, raffle_tickets)
		VALUES (?, ?)
		ON DUPLICATE KEY UPDATE raffle_tickets = raffle_tickets + VALUES(raffle_tickets)
	`, characterID, count)
	if err != nil {
		return 0, corecharacter.Character{}, err
	}

	// 3. Query updated values
	var currentTickets int
	if err := tx.QueryRowContext(ctx, `SELECT raffle_tickets FROM character_lottery WHERE character_id = ?`, characterID).Scan(&currentTickets); err != nil {
		return 0, corecharacter.Character{}, err
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, characterID))
	if err != nil {
		return 0, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return 0, corecharacter.Character{}, err
	}
	return currentTickets, char, nil
}

func (r *LotteryRepository) UseRaffleTickets(ctx context.Context, characterID string, count int, rewardGold int) (int, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Deduct raffle tickets
	res, err := tx.ExecContext(ctx, `
		UPDATE character_lottery
		SET raffle_tickets = raffle_tickets - ?
		WHERE character_id = ? AND raffle_tickets >= ?
	`, count, characterID, count)
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, corecharacter.Character{}, err
	}
	if affected == 0 {
		return 0, corecharacter.Character{}, lottery.ErrInsufficientTickets
	}

	// 2. Add reward gold if any
	if rewardGold > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE characters
			SET money = money + ?
			WHERE id = ?
		`, rewardGold, characterID); err != nil {
			return 0, corecharacter.Character{}, err
		}
	}

	// 3. Query updated state
	var currentTickets int
	if err := tx.QueryRowContext(ctx, `SELECT raffle_tickets FROM character_lottery WHERE character_id = ?`, characterID).Scan(&currentTickets); err != nil {
		return 0, corecharacter.Character{}, err
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, characterID))
	if err != nil {
		return 0, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return 0, corecharacter.Character{}, err
	}
	return currentTickets, char, nil
}

func (r *LotteryRepository) PurchaseLotteryTicket(ctx context.Context, ticket lottery.LotteryTicket, goldCost int) (lottery.LotteryTicket, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Deduct gold
	res, err := tx.ExecContext(ctx, `
		UPDATE characters
		SET money = money - ?
		WHERE id = ? AND money >= ?
	`, goldCost, ticket.CharacterID, goldCost)
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	if affected == 0 {
		return lottery.LotteryTicket{}, corecharacter.Character{}, lottery.ErrInsufficientGold
	}

	// 2. Generate ID and insert ticket
	if ticket.ID == "" {
		ticket.ID = generateLotteryID()
	}
	if ticket.PurchasedAt.IsZero() {
		ticket.PurchasedAt = time.Now().UTC()
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO lottery_tickets (id, character_id, round_id, ticket_number, purchased_at, claimed, prize_tier, prize_gold)
		VALUES (?, ?, ?, ?, ?, FALSE, 'NONE', 0)
	`, ticket.ID, ticket.CharacterID, ticket.RoundID, ticket.TicketNumber, ticket.PurchasedAt)
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, ticket.CharacterID))
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	return ticket, char, nil
}

func (r *LotteryRepository) GetLotteryTicket(ctx context.Context, ticketID string) (lottery.LotteryTicket, error) {
	var t lottery.LotteryTicket
	var claimedAt sql.NullTime
	err := r.db.QueryRowContext(ctx, `
		SELECT id, character_id, round_id, ticket_number, purchased_at, claimed, prize_tier, prize_gold, claimed_at
		FROM lottery_tickets
		WHERE id = ?
	`, ticketID).Scan(&t.ID, &t.CharacterID, &t.RoundID, &t.TicketNumber, &t.PurchasedAt, &t.Claimed, &t.PrizeTier, &t.PrizeGold, &claimedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return lottery.LotteryTicket{}, lottery.ErrTicketNotFound
	}
	if err != nil {
		return lottery.LotteryTicket{}, err
	}
	if claimedAt.Valid {
		t.ClaimedAt = &claimedAt.Time
	}
	return t, nil
}

func (r *LotteryRepository) ListLotteryTickets(ctx context.Context, characterID string, roundID int) ([]lottery.LotteryTicket, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, character_id, round_id, ticket_number, purchased_at, claimed, prize_tier, prize_gold, claimed_at
		FROM lottery_tickets
		WHERE character_id = ? AND round_id = ?
		ORDER BY purchased_at ASC
	`, characterID, roundID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tickets []lottery.LotteryTicket
	for rows.Next() {
		var t lottery.LotteryTicket
		var claimedAt sql.NullTime
		if err := rows.Scan(&t.ID, &t.CharacterID, &t.RoundID, &t.TicketNumber, &t.PurchasedAt, &t.Claimed, &t.PrizeTier, &t.PrizeGold, &claimedAt); err != nil {
			return nil, err
		}
		if claimedAt.Valid {
			t.ClaimedAt = &claimedAt.Time
		}
		tickets = append(tickets, t)
	}
	return tickets, rows.Err()
}

func (r *LotteryRepository) SaveDrawing(ctx context.Context, drawing lottery.LotteryDrawing) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO lottery_drawings (round_id, winning_number, drawn_at, is_settled)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE winning_number = VALUES(winning_number), drawn_at = VALUES(drawn_at), is_settled = VALUES(is_settled)
	`, drawing.RoundID, drawing.WinningNumber, drawing.DrawnAt, drawing.IsSettled)
	return err
}

func (r *LotteryRepository) GetDrawing(ctx context.Context, roundID int) (lottery.LotteryDrawing, error) {
	var d lottery.LotteryDrawing
	err := r.db.QueryRowContext(ctx, `
		SELECT round_id, winning_number, drawn_at, is_settled
		FROM lottery_drawings
		WHERE round_id = ?
	`, roundID).Scan(&d.RoundID, &d.WinningNumber, &d.DrawnAt, &d.IsSettled)
	if errors.Is(err, sql.ErrNoRows) {
		return lottery.LotteryDrawing{}, lottery.ErrDrawingNotSettled
	}
	if err != nil {
		return lottery.LotteryDrawing{}, err
	}
	return d, nil
}

func (r *LotteryRepository) ClaimLotteryTicket(ctx context.Context, ticketID string, tier string, prizeGold int) (lottery.LotteryTicket, corecharacter.Character, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	defer func() {
		_ = tx.Rollback()
	}()

	// 1. Mark claimed
	now := time.Now().UTC()
	res, err := tx.ExecContext(ctx, `
		UPDATE lottery_tickets
		SET claimed = TRUE, prize_tier = ?, prize_gold = ?, claimed_at = ?
		WHERE id = ? AND claimed = FALSE
	`, tier, prizeGold, now, ticketID)
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	if affected == 0 {
		return lottery.LotteryTicket{}, corecharacter.Character{}, lottery.ErrTicketAlreadyClaimed
	}

	// 2. Fetch ticket to get characterID
	var t lottery.LotteryTicket
	var claimedAt sql.NullTime
	if err := tx.QueryRowContext(ctx, `
		SELECT id, character_id, round_id, ticket_number, purchased_at, claimed, prize_tier, prize_gold, claimed_at
		FROM lottery_tickets
		WHERE id = ?
	`, ticketID).Scan(&t.ID, &t.CharacterID, &t.RoundID, &t.TicketNumber, &t.PurchasedAt, &t.Claimed, &t.PrizeTier, &t.PrizeGold, &claimedAt); err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	if claimedAt.Valid {
		t.ClaimedAt = &claimedAt.Time
	}

	// 3. Award prize gold if any
	if prizeGold > 0 {
		if _, err := tx.ExecContext(ctx, `
			UPDATE characters
			SET money = money + ?
			WHERE id = ?
		`, prizeGold, t.CharacterID); err != nil {
			return lottery.LotteryTicket{}, corecharacter.Character{}, err
		}
	}

	char, err := scanCharacterRow(tx.QueryRowContext(ctx, `
		SELECT id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, rebirth_count
		FROM characters
		WHERE id = ?
	`, t.CharacterID))
	if err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}

	if err := tx.Commit(); err != nil {
		return lottery.LotteryTicket{}, corecharacter.Character{}, err
	}
	return t, char, nil
}

func generateLotteryID() string {
	bytes := make([]byte, 16)
	_, _ = rand.Read(bytes)
	return hex.EncodeToString(bytes)
}
