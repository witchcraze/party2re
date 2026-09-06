package dungeon

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
)

const (
	DefaultExpeditionTTL = 2 * time.Hour
)

var (
	ErrExpeditionNotFound   = errors.New("ERR_EXPEDITION_NOT_FOUND: active expedition not found")
	ErrExpeditionNotActive  = errors.New("ERR_EXPEDITION_NOT_ACTIVE: expedition is not in exploring status")
	ErrExpeditionIDMismatch = errors.New("ERR_EXPEDITION_ID_MISMATCH: expedition id mismatch")
)

//go:embed lua/dungeon_step.lua
var dungeonStepLua string

type valkeyConfig struct {
	ttl time.Duration
}

// ValkeyOption configures optional parameters on ValkeyExpeditionRepository.
type ValkeyOption func(*valkeyConfig)

// WithExpeditionTTL overrides the default 2-hour sliding expiration.
func WithExpeditionTTL(ttl time.Duration) ValkeyOption {
	return func(c *valkeyConfig) {
		if ttl > 0 {
			c.ttl = ttl
		}
	}
}

// ValkeyExpeditionRepository manages active expedition buffers in Valkey Master
// using atomic Lua scripts and sliding TTL eviction (Candidate D).
type ValkeyExpeditionRepository struct {
	client     valkeygo.Client
	stepScript *valkeygo.Lua
	ttl        time.Duration
	fallback   *MemoryExpeditionRepository
}

// NewValkeyExpeditionRepository creates a new Valkey-backed expedition store.
// If client is nil, it falls back to a thread-safe in-memory store.
func NewValkeyExpeditionRepository(client valkeygo.Client, opts ...ValkeyOption) (*ValkeyExpeditionRepository, error) {
	cfg := &valkeyConfig{
		ttl: DefaultExpeditionTTL,
	}
	for _, opt := range opts {
		opt(cfg)
	}

	repo := &ValkeyExpeditionRepository{
		client:     client,
		stepScript: valkeygo.NewLuaScript(dungeonStepLua),
		ttl:        cfg.ttl,
		fallback:   NewMemoryExpeditionRepository(opts...),
	}

	return repo, nil
}

func (r *ValkeyExpeditionRepository) stateKey(charID string) string {
	return fmt.Sprintf("party2:dungeon:{char:%s}:state", charID)
}

func (r *ValkeyExpeditionRepository) rewardsKey(charID string) string {
	return fmt.Sprintf("party2:dungeon:{char:%s}:rewards", charID)
}

func mapLuaError(err error) error {
	if err == nil {
		return nil
	}
	errStr := err.Error()
	if strings.Contains(errStr, "ERR_EXPEDITION_NOT_FOUND") {
		return ErrExpeditionNotFound
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_NOT_ACTIVE") {
		return ErrExpeditionNotActive
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_ID_MISMATCH") {
		return ErrExpeditionIDMismatch
	}
	return err
}

func (r *ValkeyExpeditionRepository) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	if r.client == nil {
		return r.fallback.GetActiveExpedition(ctx, characterID)
	}

	sKey := r.stateKey(characterID)
	rKey := r.rewardsKey(characterID)

	cmds := r.client.DoMulti(ctx,
		r.client.B().Hgetall().Key(sKey).Build(),
		r.client.B().Hgetall().Key(rKey).Build(),
	)

	sMap, err := cmds[0].AsStrMap()
	if err != nil {
		if valkeygo.IsValkeyNil(err) {
			return nil, nil
		}
		return nil, err
	}
	if len(sMap) == 0 {
		return nil, nil
	}

	rMap, _ := cmds[1].AsStrMap()

	floor, _ := strconv.Atoi(sMap["current_floor"])
	posX, _ := strconv.Atoi(sMap["pos_x"])
	posY, _ := strconv.Atoi(sMap["pos_y"])
	hp, _ := strconv.Atoi(sMap["current_hp"])
	turns, _ := strconv.Atoi(sMap["turns_remaining"])

	exp, _ := strconv.Atoi(rMap["exp"])
	gold, _ := strconv.Atoi(rMap["gold"])
	medals, _ := strconv.Atoi(rMap["medals"])
	items := DecodeItems(rMap["items"])

	startedAt, _ := time.Parse(time.RFC3339Nano, sMap["started_at"])
	updatedAt, _ := time.Parse(time.RFC3339Nano, sMap["updated_at"])

	active := &ActiveExpedition{
		ID:                sMap["expedition_id"],
		CharacterID:       sMap["character_id"],
		DungeonID:         sMap["dungeon_id"],
		CurrentFloor:      floor,
		PosX:              posX,
		PosY:              posY,
		CurrentHP:         hp,
		TurnsRemaining:    turns,
		AccumulatedExp:    exp,
		AccumulatedGold:   gold,
		AccumulatedMedals: medals,
		AccumulatedItems:  items,
		Status:            ExpeditionStatus(sMap["status"]),
		StartedAt:         startedAt,
		UpdatedAt:         updatedAt,
	}

	return active, nil
}

func (r *ValkeyExpeditionRepository) SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error {
	if r.client == nil {
		return r.fallback.SaveActiveExpedition(ctx, exp)
	}

	sKey := r.stateKey(exp.CharacterID)
	rKey := r.rewardsKey(exp.CharacterID)

	itemsJSON := EncodeItems(exp.AccumulatedItems)
	ttlSec := int64(r.ttl.Seconds())

	cmds := r.client.DoMulti(ctx,
		r.client.B().Hset().Key(sKey).FieldValue().
			FieldValue("expedition_id", exp.ID).
			FieldValue("character_id", exp.CharacterID).
			FieldValue("dungeon_id", exp.DungeonID).
			FieldValue("current_floor", strconv.Itoa(exp.CurrentFloor)).
			FieldValue("pos_x", strconv.Itoa(exp.PosX)).
			FieldValue("pos_y", strconv.Itoa(exp.PosY)).
			FieldValue("current_hp", strconv.Itoa(exp.CurrentHP)).
			FieldValue("turns_remaining", strconv.Itoa(exp.TurnsRemaining)).
			FieldValue("status", string(exp.Status)).
			FieldValue("started_at", exp.StartedAt.Format(time.RFC3339Nano)).
			FieldValue("updated_at", exp.UpdatedAt.Format(time.RFC3339Nano)).
			Build(),
		r.client.B().Hset().Key(rKey).FieldValue().
			FieldValue("exp", strconv.Itoa(exp.AccumulatedExp)).
			FieldValue("gold", strconv.Itoa(exp.AccumulatedGold)).
			FieldValue("medals", strconv.Itoa(exp.AccumulatedMedals)).
			FieldValue("items", itemsJSON).
			Build(),
		r.client.B().Expire().Key(sKey).Seconds(ttlSec).Build(),
		r.client.B().Expire().Key(rKey).Seconds(ttlSec).Build(),
	)

	for _, cmd := range cmds {
		if err := cmd.Error(); err != nil {
			return err
		}
	}

	return nil
}

func (r *ValkeyExpeditionRepository) DeleteActiveExpedition(ctx context.Context, characterID string) error {
	if r.client == nil {
		return r.fallback.DeleteActiveExpedition(ctx, characterID)
	}

	sKey := r.stateKey(characterID)
	rKey := r.rewardsKey(characterID)

	_ = r.client.DoMulti(ctx,
		r.client.B().Del().Key(sKey).Build(),
		r.client.B().Del().Key(rKey).Build(),
	)

	return nil
}

func (r *ValkeyExpeditionRepository) Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error) {
	if r.client == nil {
		return r.fallback.Step(ctx, characterID, params)
	}

	sKey := r.stateKey(characterID)
	rKey := r.rewardsKey(characterID)

	res := r.stepScript.Exec(ctx, r.client, []string{sKey, rKey}, []string{
		params.ExpectedExpeditionID,
		strconv.Itoa(params.NewFloor),
		strconv.Itoa(params.NewX),
		strconv.Itoa(params.NewY),
		strconv.Itoa(params.HPDelta),
		strconv.Itoa(params.TurnsDelta),
		strconv.Itoa(params.ExpDelta),
		strconv.Itoa(params.GoldDelta),
		strconv.Itoa(params.MedalsDelta),
		params.RewardItemID,
		params.Now.Format(time.RFC3339Nano),
		strconv.FormatInt(int64(r.ttl.Seconds()), 10),
	})

	if err := res.Error(); err != nil {
		return StepOutcome{}, mapLuaError(err)
	}

	vals, err := res.AsStrSlice()
	if err != nil || len(vals) < 13 {
		return StepOutcome{}, fmt.Errorf("unexpected lua step response format: %v", err)
	}

	status := ExpeditionStatus(vals[0])
	floor, _ := strconv.Atoi(vals[1])
	posX, _ := strconv.Atoi(vals[2])
	posY, _ := strconv.Atoi(vals[3])
	hp, _ := strconv.Atoi(vals[4])
	turns, _ := strconv.Atoi(vals[5])
	totExp, _ := strconv.Atoi(vals[6])
	totGold, _ := strconv.Atoi(vals[7])
	totMedals, _ := strconv.Atoi(vals[8])
	items := DecodeItems(vals[9])
	updatedAt, _ := time.Parse(time.RFC3339Nano, vals[10])
	dungeonID := vals[11]
	startedAt, _ := time.Parse(time.RFC3339Nano, vals[12])

	exp := ActiveExpedition{
		ID:                params.ExpectedExpeditionID,
		CharacterID:       characterID,
		DungeonID:         dungeonID,
		CurrentFloor:      floor,
		PosX:              posX,
		PosY:              posY,
		CurrentHP:         hp,
		TurnsRemaining:    turns,
		AccumulatedExp:    totExp,
		AccumulatedGold:   totGold,
		AccumulatedMedals: totMedals,
		AccumulatedItems:  items,
		Status:            status,
		StartedAt:         startedAt,
		UpdatedAt:         updatedAt,
	}

	return StepOutcome{
		Expedition: exp,
		Status:     status,
	}, nil
}
