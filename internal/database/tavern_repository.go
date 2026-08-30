package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/tavern"
)

type TavernRepository struct {
	db *sql.DB
}

func NewTavernRepository(db *sql.DB) (*TavernRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &TavernRepository{db: db}, nil
}

func (r *TavernRepository) GetCharacterStatus(ctx context.Context, characterID string) (tavern.TavernCharacterStatus, error) {
	var (
		charID      string
		isFull      bool
		lastEatenAt sql.NullTime
		meals       int
		goldSpent   int64
		updatedAt   time.Time
	)

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, is_full, last_eaten_at, total_meals_eaten, total_gold_spent, updated_at
		FROM tavern_character_status
		WHERE character_id = ?
	`, characterID).Scan(&charID, &isFull, &lastEatenAt, &meals, &goldSpent, &updatedAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tavern.TavernCharacterStatus{
				CharacterID: characterID,
				IsFull:      false,
			}, nil
		}
		return tavern.TavernCharacterStatus{}, err
	}

	var eatenPtr *time.Time
	if lastEatenAt.Valid {
		t := lastEatenAt.Time.UTC()
		eatenPtr = &t
	}

	return tavern.TavernCharacterStatus{
		CharacterID:     charID,
		IsFull:          isFull,
		LastEatenAt:     eatenPtr,
		TotalMealsEaten: meals,
		TotalGoldSpent:  goldSpent,
		UpdatedAt:       updatedAt.UTC(),
	}, nil
}

func (r *TavernRepository) UpsertCharacterStatus(ctx context.Context, status tavern.TavernCharacterStatus) error {
	var lastEatenAt sql.NullTime
	if status.LastEatenAt != nil {
		lastEatenAt = sql.NullTime{
			Time:  *status.LastEatenAt,
			Valid: true,
		}
	}

	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO tavern_character_status (
			character_id, is_full, last_eaten_at, total_meals_eaten, total_gold_spent, updated_at
		) VALUES (?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			is_full = VALUES(is_full),
			last_eaten_at = VALUES(last_eaten_at),
			total_meals_eaten = VALUES(total_meals_eaten),
			total_gold_spent = VALUES(total_gold_spent),
			updated_at = VALUES(updated_at)
	`,
		status.CharacterID,
		status.IsFull,
		lastEatenAt,
		status.TotalMealsEaten,
		status.TotalGoldSpent,
		time.Now().UTC(),
	)
	return err
}

func (r *TavernRepository) GetDelivery(ctx context.Context, characterID string) (tavern.DeliveryReservation, error) {
	var (
		charID    string
		itemID    string
		itemName  string
		price     int
		hpHeal    int
		mpHeal    int
		tickets   int
		createdAt time.Time
	)

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT character_id, item_id, item_name, price, hp_heal, mp_heal, tickets, created_at
		FROM tavern_deliveries
		WHERE character_id = ?
	`, characterID).Scan(&charID, &itemID, &itemName, &price, &hpHeal, &mpHeal, &tickets, &createdAt)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return tavern.DeliveryReservation{}, tavern.ErrNoActiveDelivery
		}
		return tavern.DeliveryReservation{}, err
	}

	return tavern.DeliveryReservation{
		CharacterID: charID,
		ItemID:      itemID,
		ItemName:    itemName,
		Price:       price,
		HPHeal:      hpHeal,
		MPHeal:      mpHeal,
		Tickets:     tickets,
		CreatedAt:   createdAt.UTC(),
	}, nil
}

func (r *TavernRepository) SaveDelivery(ctx context.Context, delivery tavern.DeliveryReservation) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO tavern_deliveries (
			character_id, item_id, item_name, price, hp_heal, mp_heal, tickets, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			item_id = VALUES(item_id),
			item_name = VALUES(item_name),
			price = VALUES(price),
			hp_heal = VALUES(hp_heal),
			mp_heal = VALUES(mp_heal),
			tickets = VALUES(tickets),
			created_at = VALUES(created_at)
	`,
		delivery.CharacterID,
		delivery.ItemID,
		delivery.ItemName,
		delivery.Price,
		delivery.HPHeal,
		delivery.MPHeal,
		delivery.Tickets,
		delivery.CreatedAt,
	)
	return err
}

func (r *TavernRepository) DeleteDelivery(ctx context.Context, characterID string) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		DELETE FROM tavern_deliveries
		WHERE character_id = ?
	`, characterID)
	return err
}
