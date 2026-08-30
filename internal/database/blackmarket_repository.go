package database

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/blackmarket"
)

type BlackMarketRepository struct {
	db *sql.DB
}

func NewBlackMarketRepository(db *sql.DB) (*BlackMarketRepository, error) {
	if db == nil {
		return nil, errors.New("database is nil")
	}
	return &BlackMarketRepository{db: db}, nil
}

func (r *BlackMarketRepository) GetDailyPurchases(ctx context.Context, characterID string, date time.Time) (map[string]int, error) {
	dateStr := date.UTC().Format("2006-01-02")
	rows, err := ExecutorFromContext(ctx, r.db).QueryContext(ctx, `
		SELECT item_id, quantity
		FROM blackmarket_character_purchases
		WHERE character_id = ? AND purchase_date = ?
	`, characterID, dateStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[string]int)
	for rows.Next() {
		var itemID string
		var quantity int
		if err := rows.Scan(&itemID, &quantity); err != nil {
			return nil, err
		}
		result[itemID] = quantity
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func (r *BlackMarketRepository) RecordPurchase(ctx context.Context, characterID string, itemID string, date time.Time, quantity int) error {
	dateStr := date.UTC().Format("2006-01-02")
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO blackmarket_character_purchases (character_id, item_id, purchase_date, quantity)
		VALUES (?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE quantity = quantity + VALUES(quantity)
	`, characterID, itemID, dateStr, quantity)
	return err
}

func (r *BlackMarketRepository) GetMarketState(ctx context.Context) (blackmarket.MarketState, error) {
	var (
		condName  string
		priceMult float64
		sellMult  float64
		riskLevel string
	)

	err := ExecutorFromContext(ctx, r.db).QueryRowContext(ctx, `
		SELECT condition_name, price_multiplier, sell_multiplier, risk_level
		FROM blackmarket_market_state
		WHERE id = 1
	`).Scan(&condName, &priceMult, &sellMult, &riskLevel)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return blackmarket.DefaultMarketStates[blackmarket.ConditionQuiet], nil
		}
		return blackmarket.MarketState{}, err
	}

	cond := blackmarket.MarketCondition(condName)
	desc := ""
	if def, ok := blackmarket.DefaultMarketStates[cond]; ok {
		desc = def.Description
	}

	return blackmarket.MarketState{
		Condition:       cond,
		PriceMultiplier: priceMult,
		SellMultiplier:  sellMult,
		RiskLevel:       riskLevel,
		Description:     desc,
	}, nil
}

func (r *BlackMarketRepository) SaveMarketState(ctx context.Context, state blackmarket.MarketState) error {
	_, err := ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO blackmarket_market_state (id, condition_name, price_multiplier, sell_multiplier, risk_level)
		VALUES (1, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			condition_name = VALUES(condition_name),
			price_multiplier = VALUES(price_multiplier),
			sell_multiplier = VALUES(sell_multiplier),
			risk_level = VALUES(risk_level)
	`, string(state.Condition), state.PriceMultiplier, state.SellMultiplier, state.RiskLevel)
	return err
}
