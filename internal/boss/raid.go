package boss

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"
)

// RaidStatus represents the lifecycle state of a shared world boss encounter.
type RaidStatus string

const (
	RaidStatusActive   RaidStatus = "active"
	RaidStatusDefeated RaidStatus = "defeated"
	RaidStatusSettled  RaidStatus = "settled"
)

// RaidDamageStatus represents the outcome of an atomic damage application.
type RaidDamageStatus string

const (
	RaidDamageHit         RaidDamageStatus = "hit"
	RaidDamageKilled      RaidDamageStatus = "killed"
	RaidDamageAlreadyDead RaidDamageStatus = "already_dead"
	RaidDamageNotFound    RaidDamageStatus = "not_found"
)

var (
	ErrRaidNotFound  = errors.New("raid boss not found")
	ErrRaidDefeated  = errors.New("raid boss has already been defeated")
	ErrRaidSettled   = errors.New("raid boss has already been settled")
	ErrInvalidDamage = errors.New("damage must be greater than zero")
	ErrEmptyBossID   = errors.New("boss id cannot be empty")
	ErrEmptyRunID    = errors.New("run id cannot be empty")
	ErrEmptyAttacker = errors.New("attacker id cannot be empty")
)

// RaidBossState captures the volatile combat state of a World Boss in Valkey Master.
type RaidBossState struct {
	BossID       string         `json:"boss_id"`
	RunID        string         `json:"run_id"`
	InitialHP    int            `json:"initial_hp"`
	CurrentHP    int            `json:"current_hp"`
	Status       RaidStatus     `json:"status"`
	KillerID     string         `json:"killer_id,omitempty"`
	Contributors map[string]int `json:"contributors,omitempty"`
}

// RaidDamageResult captures the result of an atomic attack evaluated by the Lua script.
type RaidDamageResult struct {
	Status       RaidDamageStatus `json:"status"`
	ActualDamage int              `json:"actual_damage"`
	RemainingHP  int              `json:"remaining_hp"`
	KillerID     string           `json:"killer_id,omitempty"`
}

// RaidParticipantReward represents loot granted to a contributor.
type RaidParticipantReward struct {
	CharacterID string `json:"character_id"`
	DamageDealt int    `json:"damage_dealt"`
	ExpReward   int    `json:"exp_reward"`
	GoldReward  int    `json:"gold_reward"`
	IsLastHit   bool   `json:"is_last_hit"`
	IsMVP       bool   `json:"is_mvp"`
}

// RaidSettlement encapsulates the durable transactional settlement of a defeated boss.
type RaidSettlement struct {
	BossID      string                           `json:"boss_id"`
	RunID       string                           `json:"run_id"`
	KillerID    string                           `json:"killer_id"`
	MVPID       string                           `json:"mvp_id"`
	TotalDamage int                              `json:"total_damage"`
	Rewards     map[string]RaidParticipantReward `json:"rewards"`
	SettledAt   time.Time                        `json:"settled_at"`
}

// RaidRepository defines the volatile fast-path operations executed against Valkey Master.
type RaidRepository interface {
	InitializeRaid(ctx context.Context, bossID, runID string, initialHP int, ttl time.Duration) error
	ApplyDamage(ctx context.Context, bossID, attackerID string, incomingDamage int, ttl time.Duration) (RaidDamageResult, error)
	GetRaidState(ctx context.Context, bossID string) (RaidBossState, error)
	GetContributors(ctx context.Context, bossID string) (map[string]int, error)
	MarkSettled(ctx context.Context, bossID, runID string) error
}

// RaidSettlementRepository defines the transactional persistence operations for durable MariaDB settlement.
type RaidSettlementRepository interface {
	IsRunSettled(ctx context.Context, runID string) (bool, error)
	RecordRaidSettlement(ctx context.Context, settlement RaidSettlement) error
}

// RaidCoordinatorOption configures a RaidCoordinator.
type RaidCoordinatorOption func(*RaidCoordinator)

// WithRaidLogger configures structured logging for the coordinator.
func WithRaidLogger(logger *slog.Logger) RaidCoordinatorOption {
	return func(c *RaidCoordinator) {
		if logger != nil {
			c.logger = logger
		}
	}
}

// WithRaidBanquetHook configures victory celebration announcements.
func WithRaidBanquetHook(hook VictoryBanquetHook) RaidCoordinatorOption {
	return func(c *RaidCoordinator) {
		c.banquetHook = hook
	}
}

// RaidCoordinator coordinates real-time shared boss combat and exactly-once MariaDB settlement.
type RaidCoordinator struct {
	raidRepo       RaidRepository
	settlementRepo RaidSettlementRepository
	banquetHook    VictoryBanquetHook
	logger         *slog.Logger
	defaultTTL     time.Duration
}

// NewRaidCoordinator constructs a new RaidCoordinator.
func NewRaidCoordinator(
	raidRepo RaidRepository,
	settlementRepo RaidSettlementRepository,
	opts ...RaidCoordinatorOption,
) *RaidCoordinator {
	c := &RaidCoordinator{
		raidRepo:       raidRepo,
		settlementRepo: settlementRepo,
		logger:         slog.Default(),
		defaultTTL:     2 * time.Hour,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// StartRaid initializes a new World Boss raid session.
func (c *RaidCoordinator) StartRaid(ctx context.Context, bossID, runID string, initialHP int) error {
	if bossID == "" {
		return ErrEmptyBossID
	}
	if runID == "" {
		return ErrEmptyRunID
	}
	if initialHP <= 0 {
		return errors.New("initial hp must be greater than zero")
	}

	return c.raidRepo.InitializeRaid(ctx, bossID, runID, initialHP, c.defaultTTL)
}

// AttackBoss applies damage from an attacker against the shared World Boss.
// If the attacker lands the true killing blow (status: killed), this method
// automatically coordinates the exactly-once MariaDB settlement.
func (c *RaidCoordinator) AttackBoss(ctx context.Context, bossID, attackerID string, incomingDamage int) (RaidDamageResult, *RaidSettlement, error) {
	if bossID == "" {
		return RaidDamageResult{}, nil, ErrEmptyBossID
	}
	if attackerID == "" {
		return RaidDamageResult{}, nil, ErrEmptyAttacker
	}
	if incomingDamage <= 0 {
		return RaidDamageResult{}, nil, ErrInvalidDamage
	}

	res, err := c.raidRepo.ApplyDamage(ctx, bossID, attackerID, incomingDamage, c.defaultTTL)
	if err != nil {
		return RaidDamageResult{}, nil, fmt.Errorf("failed to apply raid damage: %w", err)
	}

	if res.Status == RaidDamageKilled {
		c.logger.InfoContext(ctx, "killer elected by atomic lua evaluation",
			"boss_id", bossID,
			"killer_id", attackerID,
			"actual_damage", res.ActualDamage,
		)

		settlement, settleErr := c.settleDefeatedBoss(ctx, bossID, attackerID)
		if settleErr != nil {
			c.logger.ErrorContext(ctx, "failed to settle defeated boss transactionally",
				"boss_id", bossID,
				"killer_id", attackerID,
				"error", settleErr,
			)
			// Return the kill result; background reconciliation will retry idempotent settlement
			return res, nil, settleErr
		}
		return res, settlement, nil
	}

	return res, nil, nil
}

// settleDefeatedBoss orchestrates durable MariaDB settlement once a killer has been elected.
func (c *RaidCoordinator) settleDefeatedBoss(ctx context.Context, bossID, killerID string) (*RaidSettlement, error) {
	state, err := c.raidRepo.GetRaidState(ctx, bossID)
	if err != nil {
		return nil, fmt.Errorf("failed to get raid state for settlement: %w", err)
	}

	// Idempotency check: has this run already been recorded in durable storage?
	alreadySettled, err := c.settlementRepo.IsRunSettled(ctx, state.RunID)
	if err != nil {
		return nil, fmt.Errorf("failed to check run settlement status: %w", err)
	}
	if alreadySettled {
		// Ensure Valkey status reflects settled state
		_ = c.raidRepo.MarkSettled(ctx, bossID, state.RunID)
		return nil, nil
	}

	contributors, err := c.raidRepo.GetContributors(ctx, bossID)
	if err != nil {
		return nil, fmt.Errorf("failed to get raid contributors: %w", err)
	}

	// Calculate MVP and total damage
	mvpID := killerID
	maxDmg := -1
	totalDmg := 0

	type contrib struct {
		id  string
		dmg int
	}
	list := make([]contrib, 0, len(contributors))
	for id, dmg := range contributors {
		list = append(list, contrib{id: id, dmg: dmg})
		totalDmg += dmg
		if dmg > maxDmg {
			maxDmg = dmg
			mvpID = id
		}
	}

	// Sort contributors deterministically for stable reward mapping
	sort.Slice(list, func(i, j int) bool {
		if list[i].dmg == list[j].dmg {
			return list[i].id < list[j].id
		}
		return list[i].dmg > list[j].dmg
	})

	now := time.Now().UTC()
	settlement := RaidSettlement{
		BossID:      bossID,
		RunID:       state.RunID,
		KillerID:    killerID,
		MVPID:       mvpID,
		TotalDamage: totalDmg,
		Rewards:     make(map[string]RaidParticipantReward, len(list)),
		SettledAt:   now,
	}

	// Base reward formula:
	// - Participation: 10 EXP, 20 Gold per 100 damage dealt (min 5 EXP, 10 Gold)
	// - Last-Hit Bonus: +500 EXP, +1000 Gold
	// - MVP Bonus: +1000 EXP, +2000 Gold
	for _, cEntry := range list {
		exp := (cEntry.dmg / 100) * 10
		if exp < 5 {
			exp = 5
		}
		gold := (cEntry.dmg / 100) * 20
		if gold < 10 {
			gold = 10
		}

		isLastHit := (cEntry.id == killerID)
		isMVP := (cEntry.id == mvpID)

		if isLastHit {
			exp += 500
			gold += 1000
		}
		if isMVP {
			exp += 1000
			gold += 2000
		}

		settlement.Rewards[cEntry.id] = RaidParticipantReward{
			CharacterID: cEntry.id,
			DamageDealt: cEntry.dmg,
			ExpReward:   exp,
			GoldReward:  gold,
			IsLastHit:   isLastHit,
			IsMVP:       isMVP,
		}
	}

	// Durable MariaDB transaction commit
	if err := c.settlementRepo.RecordRaidSettlement(ctx, settlement); err != nil {
		return nil, fmt.Errorf("failed to record durable raid settlement: %w", err)
	}

	// Mark Valkey status as settled
	if err := c.raidRepo.MarkSettled(ctx, bossID, state.RunID); err != nil {
		c.logger.WarnContext(ctx, "failed to mark raid as settled in valkey; state remains defeated",
			"boss_id", bossID,
			"run_id", state.RunID,
			"error", err,
		)
	}

	if c.banquetHook != nil {
		_ = c.banquetHook(ctx, bossID, bossID, killerID, killerID, 1)
	}

	return &settlement, nil
}

// ReconcileUnsettledBoss inspects a World Boss. If it was defeated but unfinalized
// (e.g. due to node crash after killer election), it idempotently performs the MariaDB settlement.
func (c *RaidCoordinator) ReconcileUnsettledBoss(ctx context.Context, bossID string) (*RaidSettlement, error) {
	state, err := c.raidRepo.GetRaidState(ctx, bossID)
	if err != nil {
		return nil, err
	}

	if state.Status != RaidStatusDefeated {
		// Not defeated or already settled; no reconciliation needed
		return nil, nil
	}

	c.logger.InfoContext(ctx, "reconciling unsettled defeated boss",
		"boss_id", bossID,
		"run_id", state.RunID,
		"recorded_killer", state.KillerID,
	)

	return c.settleDefeatedBoss(ctx, bossID, state.KillerID)
}

// GetRaidState returns the current volatile state of a World Boss raid.
func (c *RaidCoordinator) GetRaidState(ctx context.Context, bossID string) (RaidBossState, error) {
	return c.raidRepo.GetRaidState(ctx, bossID)
}
