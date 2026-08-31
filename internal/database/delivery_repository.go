package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/delivery"
)

type DeliveryRepository struct {
	db *sql.DB
}

func NewDeliveryRepository(db *sql.DB) (*DeliveryRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &DeliveryRepository{db: db}, nil
}

func (r *DeliveryRepository) GetAvailableQuests(ctx context.Context, now time.Time) ([]delivery.Quest, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, client_name, client_message, target_item_id, target_item_name,
		       required_quantity, recipient_name, destination, reward_gold, reward_exp,
		       reward_item_id, expires_at, created_at
		FROM delivery_quests
		WHERE expires_at > ?
		ORDER BY created_at ASC
	`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []delivery.Quest
	for rows.Next() {
		var q delivery.Quest
		if err := rows.Scan(
			&q.ID, &q.ClientName, &q.ClientMessage, &q.TargetItemID, &q.TargetItemName,
			&q.RequiredQuantity, &q.RecipientName, &q.Destination, &q.RewardGold,
			&q.RewardExp, &q.RewardItemID, &q.ExpiresAt, &q.CreatedAt,
		); err != nil {
			return nil, err
		}
		list = append(list, q)
	}
	return list, nil
}

func (r *DeliveryRepository) GetQuestByID(ctx context.Context, id string) (*delivery.Quest, error) {
	var q delivery.Quest
	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, client_name, client_message, target_item_id, target_item_name,
		       required_quantity, recipient_name, destination, reward_gold, reward_exp,
		       reward_item_id, expires_at, created_at
		FROM delivery_quests
		WHERE id = ?
	`, id).Scan(
		&q.ID, &q.ClientName, &q.ClientMessage, &q.TargetItemID, &q.TargetItemName,
		&q.RequiredQuantity, &q.RecipientName, &q.Destination, &q.RewardGold,
		&q.RewardExp, &q.RewardItemID, &q.ExpiresAt, &q.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, delivery.ErrQuestNotFound
	}
	if err != nil {
		return nil, err
	}
	return &q, nil
}

func (r *DeliveryRepository) SaveQuest(ctx context.Context, q *delivery.Quest) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO delivery_quests (
			id, client_name, client_message, target_item_id, target_item_name,
			required_quantity, recipient_name, destination, reward_gold, reward_exp,
			reward_item_id, expires_at, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			client_name = VALUES(client_name),
			client_message = VALUES(client_message),
			target_item_id = VALUES(target_item_id),
			target_item_name = VALUES(target_item_name),
			required_quantity = VALUES(required_quantity),
			recipient_name = VALUES(recipient_name),
			destination = VALUES(destination),
			reward_gold = VALUES(reward_gold),
			reward_exp = VALUES(reward_exp),
			reward_item_id = VALUES(reward_item_id),
			expires_at = VALUES(expires_at)
	`, q.ID, q.ClientName, q.ClientMessage, q.TargetItemID, q.TargetItemName,
		q.RequiredQuantity, q.RecipientName, q.Destination, q.RewardGold, q.RewardExp,
		q.RewardItemID, q.ExpiresAt, q.CreatedAt)
	return err
}

func (r *DeliveryRepository) SaveQuests(ctx context.Context, quests []delivery.Quest) error {
	for _, q := range quests {
		if err := r.SaveQuest(ctx, &q); err != nil {
			return err
		}
	}
	return nil
}

func (r *DeliveryRepository) GetCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT cd.id, cd.character_id, cd.quest_id, cd.status, cd.accepted_at, cd.completed_at,
		       dq.id, dq.client_name, dq.client_message, dq.target_item_id, dq.target_item_name,
		       dq.required_quantity, dq.recipient_name, dq.destination, dq.reward_gold,
		       dq.reward_exp, dq.reward_item_id, dq.expires_at, dq.created_at
		FROM character_deliveries cd
		LEFT JOIN delivery_quests dq ON cd.quest_id = dq.id
		WHERE cd.character_id = ?
		ORDER BY cd.accepted_at DESC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []delivery.CharacterDelivery
	for rows.Next() {
		var d delivery.CharacterDelivery
		var statusStr string
		var completedAt sql.NullTime
		var q delivery.Quest
		var qID sql.NullString

		if err := rows.Scan(
			&d.ID, &d.CharacterID, &d.QuestID, &statusStr, &d.AcceptedAt, &completedAt,
			&qID, &q.ClientName, &q.ClientMessage, &q.TargetItemID, &q.TargetItemName,
			&q.RequiredQuantity, &q.RecipientName, &q.Destination, &q.RewardGold,
			&q.RewardExp, &q.RewardItemID, &q.ExpiresAt, &q.CreatedAt,
		); err != nil {
			return nil, err
		}

		d.Status = delivery.DeliveryStatus(statusStr)
		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		if qID.Valid {
			q.ID = qID.String
			d.Quest = &q
		}
		list = append(list, d)
	}
	return list, nil
}

func (r *DeliveryRepository) GetActiveCharacterDeliveries(ctx context.Context, characterID string) ([]delivery.CharacterDelivery, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT cd.id, cd.character_id, cd.quest_id, cd.status, cd.accepted_at, cd.completed_at,
		       dq.id, dq.client_name, dq.client_message, dq.target_item_id, dq.target_item_name,
		       dq.required_quantity, dq.recipient_name, dq.destination, dq.reward_gold,
		       dq.reward_exp, dq.reward_item_id, dq.expires_at, dq.created_at
		FROM character_deliveries cd
		LEFT JOIN delivery_quests dq ON cd.quest_id = dq.id
		WHERE cd.character_id = ? AND cd.status = 'in_progress'
		ORDER BY cd.accepted_at ASC
	`, characterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []delivery.CharacterDelivery
	for rows.Next() {
		var d delivery.CharacterDelivery
		var statusStr string
		var completedAt sql.NullTime
		var q delivery.Quest
		var qID sql.NullString

		if err := rows.Scan(
			&d.ID, &d.CharacterID, &d.QuestID, &statusStr, &d.AcceptedAt, &completedAt,
			&qID, &q.ClientName, &q.ClientMessage, &q.TargetItemID, &q.TargetItemName,
			&q.RequiredQuantity, &q.RecipientName, &q.Destination, &q.RewardGold,
			&q.RewardExp, &q.RewardItemID, &q.ExpiresAt, &q.CreatedAt,
		); err != nil {
			return nil, err
		}

		d.Status = delivery.DeliveryStatus(statusStr)
		if completedAt.Valid {
			d.CompletedAt = &completedAt.Time
		}
		if qID.Valid {
			q.ID = qID.String
			d.Quest = &q
		}
		list = append(list, d)
	}
	return list, nil
}

func (r *DeliveryRepository) GetCharacterDeliveryByID(ctx context.Context, id string) (*delivery.CharacterDelivery, error) {
	var d delivery.CharacterDelivery
	var statusStr string
	var completedAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT id, character_id, quest_id, status, accepted_at, completed_at
		FROM character_deliveries
		WHERE id = ?
	`, id).Scan(
		&d.ID, &d.CharacterID, &d.QuestID, &statusStr, &d.AcceptedAt, &completedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, delivery.ErrDeliveryNotFound
	}
	if err != nil {
		return nil, err
	}

	d.Status = delivery.DeliveryStatus(statusStr)
	if completedAt.Valid {
		d.CompletedAt = &completedAt.Time
	}
	return &d, nil
}

func (r *DeliveryRepository) SaveCharacterDelivery(ctx context.Context, d *delivery.CharacterDelivery) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO character_deliveries (
			id, character_id, quest_id, status, accepted_at, completed_at
		) VALUES (?, ?, ?, ?, ?, ?)
	`, d.ID, d.CharacterID, d.QuestID, string(d.Status), d.AcceptedAt, d.CompletedAt)
	return err
}

func (r *DeliveryRepository) UpdateCharacterDelivery(ctx context.Context, d *delivery.CharacterDelivery) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE character_deliveries
		SET status = ?, completed_at = ?
		WHERE id = ?
	`, string(d.Status), d.CompletedAt, d.ID)
	return err
}

func (r *DeliveryRepository) SaveParcel(ctx context.Context, p *delivery.Parcel) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO delivery_parcels (
			id, sender_character_id, sender_character_name, recipient_character_id,
			item_id, item_name, item_quantity, gold_amount, message, courier_fee,
			status, created_at, claimed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, p.ID, p.SenderCharacterID, p.SenderCharacterName, p.RecipientCharacterID,
		p.ItemID, p.ItemName, p.ItemQuantity, p.GoldAmount, p.Message, p.CourierFee,
		string(p.Status), p.CreatedAt, p.ClaimedAt)
	return err
}

func (r *DeliveryRepository) GetParcelByID(ctx context.Context, id string) (*delivery.Parcel, error) {
	return r.getParcelByIDWithQuery(ctx, `
		SELECT id, sender_character_id, sender_character_name, recipient_character_id,
		       item_id, item_name, item_quantity, gold_amount, message, courier_fee,
		       status, created_at, claimed_at
		FROM delivery_parcels
		WHERE id = ?
	`, id)
}

func (r *DeliveryRepository) GetParcelByIDForUpdate(ctx context.Context, id string) (*delivery.Parcel, error) {
	return r.getParcelByIDWithQuery(ctx, `
		SELECT id, sender_character_id, sender_character_name, recipient_character_id,
		       item_id, item_name, item_quantity, gold_amount, message, courier_fee,
		       status, created_at, claimed_at
		FROM delivery_parcels
		WHERE id = ?
		FOR UPDATE
	`, id)
}

func (r *DeliveryRepository) getParcelByIDWithQuery(ctx context.Context, query string, id string) (*delivery.Parcel, error) {
	var p delivery.Parcel
	var statusStr string
	var claimedAt sql.NullTime

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, query, id).Scan(
		&p.ID, &p.SenderCharacterID, &p.SenderCharacterName, &p.RecipientCharacterID,
		&p.ItemID, &p.ItemName, &p.ItemQuantity, &p.GoldAmount, &p.Message, &p.CourierFee,
		&statusStr, &p.CreatedAt, &claimedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, delivery.ErrParcelNotFound
	}
	if err != nil {
		return nil, err
	}

	p.Status = delivery.ParcelStatus(statusStr)
	if claimedAt.Valid {
		p.ClaimedAt = &claimedAt.Time
	}
	return &p, nil
}

func (r *DeliveryRepository) GetIncomingParcels(ctx context.Context, recipientCharacterID string) ([]delivery.Parcel, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, sender_character_id, sender_character_name, recipient_character_id,
		       item_id, item_name, item_quantity, gold_amount, message, courier_fee,
		       status, created_at, claimed_at
		FROM delivery_parcels
		WHERE recipient_character_id = ? AND status = 'pending'
		ORDER BY created_at DESC
	`, recipientCharacterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []delivery.Parcel
	for rows.Next() {
		var p delivery.Parcel
		var statusStr string
		var claimedAt sql.NullTime

		if err := rows.Scan(
			&p.ID, &p.SenderCharacterID, &p.SenderCharacterName, &p.RecipientCharacterID,
			&p.ItemID, &p.ItemName, &p.ItemQuantity, &p.GoldAmount, &p.Message, &p.CourierFee,
			&statusStr, &p.CreatedAt, &claimedAt,
		); err != nil {
			return nil, err
		}

		p.Status = delivery.ParcelStatus(statusStr)
		if claimedAt.Valid {
			p.ClaimedAt = &claimedAt.Time
		}
		list = append(list, p)
	}
	return list, nil
}

func (r *DeliveryRepository) GetSentParcels(ctx context.Context, senderCharacterID string) ([]delivery.Parcel, error) {
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT id, sender_character_id, sender_character_name, recipient_character_id,
		       item_id, item_name, item_quantity, gold_amount, message, courier_fee,
		       status, created_at, claimed_at
		FROM delivery_parcels
		WHERE sender_character_id = ?
		ORDER BY created_at DESC
	`, senderCharacterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []delivery.Parcel
	for rows.Next() {
		var p delivery.Parcel
		var statusStr string
		var claimedAt sql.NullTime

		if err := rows.Scan(
			&p.ID, &p.SenderCharacterID, &p.SenderCharacterName, &p.RecipientCharacterID,
			&p.ItemID, &p.ItemName, &p.ItemQuantity, &p.GoldAmount, &p.Message, &p.CourierFee,
			&statusStr, &p.CreatedAt, &claimedAt,
		); err != nil {
			return nil, err
		}

		p.Status = delivery.ParcelStatus(statusStr)
		if claimedAt.Valid {
			p.ClaimedAt = &claimedAt.Time
		}
		list = append(list, p)
	}
	return list, nil
}

func (r *DeliveryRepository) UpdateParcel(ctx context.Context, p *delivery.Parcel) error {
	res, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		UPDATE delivery_parcels
		SET status = ?, claimed_at = ?
		WHERE id = ? AND status = 'pending'
	`, string(p.Status), p.ClaimedAt, p.ID)
	if err != nil {
		return err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return delivery.ErrParcelAlreadyClaimed
	}
	return nil
}
