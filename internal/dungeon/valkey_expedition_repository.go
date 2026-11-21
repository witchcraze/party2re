package dungeon

import (
	"context"
	_ "embed"
	"encoding/json"
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
	ErrExpeditionCorrupted  = errors.New("ERR_EXPEDITION_CORRUPTED: corrupted expedition state")
)

//go:embed lua/dungeon_step.lua
var dungeonStepLua string

//go:embed lua/delete_expedition.lua
var deleteExpeditionLua string

var deleteExpeditionScript = valkeygo.NewLuaScript(deleteExpeditionLua)

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
	if strings.Contains(errStr, "ERR_COMBAT_ACTIVE") {
		return ErrCombatActive
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_STATE_CHANGED") {
		return ErrExpeditionStateChanged
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_NOT_FOUND") {
		return ErrExpeditionNotFound
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_NOT_ACTIVE") {
		return ErrExpeditionNotActive
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_ID_MISMATCH") {
		return ErrExpeditionIDMismatch
	}
	if strings.Contains(errStr, "ERR_EXPEDITION_CORRUPTED") {
		return fmt.Errorf("%w: %w", ErrExpeditionCorrupted, err)
	}
	return err
}

func parseOptionalInt(field, raw string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid %s: %w", ErrExpeditionCorrupted, field, err)
	}
	return v, nil
}

func parseOptionalTime(field, raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid %s: %w", ErrExpeditionCorrupted, field, err)
	}
	return t, nil
}

type rawValkeyExpedition struct {
	id, charID, partyID, partyName, dungeonID, status                 string
	posX, posY, hp, round, limit, variant, revision, exp, gold        string
	items, members, consumedEvents, encounter, mapState, start, updAt string
}

func (raw rawValkeyExpedition) decode() (ActiveExpedition, error) {
	posX, err := parseOptionalInt("pos_x", raw.posX)
	if err != nil {
		return ActiveExpedition{}, err
	}
	posY, err := parseOptionalInt("pos_y", raw.posY)
	if err != nil {
		return ActiveExpedition{}, err
	}
	hp, err := parseOptionalInt("current_hp", raw.hp)
	if err != nil {
		return ActiveExpedition{}, err
	}
	round, err := parseOptionalInt("round", raw.round)
	if err != nil {
		return ActiveExpedition{}, err
	}
	limit, err := parseOptionalInt("round_limit", raw.limit)
	if err != nil {
		return ActiveExpedition{}, err
	}
	variant, err := parseOptionalInt("map_variant", raw.variant)
	if err != nil {
		return ActiveExpedition{}, err
	}
	revision, err := parseOptionalInt("revision", raw.revision)
	if err != nil {
		return ActiveExpedition{}, err
	}
	exp, err := parseOptionalInt("exp", raw.exp)
	if err != nil {
		return ActiveExpedition{}, err
	}
	gold, err := parseOptionalInt("gold", raw.gold)
	if err != nil {
		return ActiveExpedition{}, err
	}
	items, err := decodeItems(raw.items)
	if err != nil {
		return ActiveExpedition{}, fmt.Errorf("%w: invalid items: %w", ErrExpeditionCorrupted, err)
	}
	startedAt, err := parseOptionalTime("started_at", raw.start)
	if err != nil {
		return ActiveExpedition{}, err
	}
	updatedAt, err := parseOptionalTime("updated_at", raw.updAt)
	if err != nil {
		return ActiveExpedition{}, err
	}
	var members []ExpeditionMember
	if raw.members != "" {
		members, err = decodeMembers(raw.members)
		if err != nil {
			return ActiveExpedition{}, fmt.Errorf("%w: invalid members: %w", ErrExpeditionCorrupted, err)
		}
	}
	consumedEvents, err := decodeItems(raw.consumedEvents)
	if err != nil {
		return ActiveExpedition{}, fmt.Errorf("%w: invalid consumed_events: %w", ErrExpeditionCorrupted, err)
	}
	encounter, err := decodeEncounter(raw.encounter)
	if err != nil {
		return ActiveExpedition{}, fmt.Errorf("%w: invalid encounter: %w", ErrExpeditionCorrupted, err)
	}
	return ActiveExpedition{
		Encounter:        encounter,
		Revision:         revision,
		ID:               raw.id,
		CharacterID:      raw.charID,
		PartyID:          raw.partyID,
		PartyName:        raw.partyName,
		Members:          members,
		DungeonID:        raw.dungeonID,
		MapVariant:       variant,
		PosX:             posX,
		PosY:             posY,
		CurrentHP:        hp,
		Round:            round,
		RoundLimit:       limit,
		AccumulatedExp:   exp,
		AccumulatedGold:  gold,
		AccumulatedItems: items,
		ConsumedEvents:   consumedEvents,
		MapState:         rawState(raw.mapState),
		Status:           ExpeditionStatus(raw.status),
		StartedAt:        startedAt,
		UpdatedAt:        updatedAt,
	}, nil
}

func (r *ValkeyExpeditionRepository) GetActiveExpedition(ctx context.Context, characterID string) (*ActiveExpedition, error) {
	return r.getActiveExpedition(ctx, characterID, true)
}

func (r *ValkeyExpeditionRepository) getActiveExpedition(ctx context.Context, characterID string, resolveMember bool) (*ActiveExpedition, error) {
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
		if !resolveMember {
			return nil, nil
		}
		return r.getMemberRun(ctx, characterID)
	}

	rMap, err := cmds[1].AsStrMap()
	if err != nil {
		return nil, err
	}

	active, err := (rawValkeyExpedition{
		id: sMap["expedition_id"], charID: sMap["character_id"], partyID: sMap["party_id"], partyName: sMap["party_name"],
		dungeonID: sMap["dungeon_id"], status: sMap["status"],
		posX: sMap["pos_x"], posY: sMap["pos_y"], hp: sMap["current_hp"], round: sMap["round"],
		limit: sMap["round_limit"], variant: sMap["map_variant"], revision: sMap["revision"],
		exp: rMap["exp"], gold: rMap["gold"], items: rMap["items"],
		members: sMap["members"], consumedEvents: sMap["consumed_events"], encounter: sMap["encounter"],
		mapState: sMap["map_state"], start: sMap["started_at"], updAt: sMap["updated_at"],
	}).decode()
	if err != nil {
		return nil, err
	}
	return &active, nil
}

func (r *ValkeyExpeditionRepository) SaveActiveExpedition(ctx context.Context, exp ActiveExpedition) error {
	if r.client == nil {
		return r.fallback.SaveActiveExpedition(ctx, exp)
	}

	encounterJSON, err := encodeEncounter(exp.Encounter)
	if err != nil {
		return err
	}
	if err := r.saveMemberIndex(ctx, exp); err != nil {
		return err
	}
	sKey := r.stateKey(exp.CharacterID)
	rKey := r.rewardsKey(exp.CharacterID)

	itemsJSON := EncodeItems(exp.AccumulatedItems)
	eventsJSON := EncodeItems(exp.ConsumedEvents)
	membersJSON := EncodeMembers(exp.Members)
	ttlSec := int64(r.ttl.Seconds())

	cmds := r.client.DoMulti(ctx,
		r.client.B().Hset().Key(sKey).FieldValue().
			FieldValue("expedition_id", exp.ID).
			FieldValue("revision", strconv.Itoa(exp.Revision)).
			FieldValue("encounter", encounterJSON).
			FieldValue("character_id", exp.CharacterID).
			FieldValue("party_id", exp.PartyID).
			FieldValue("party_name", exp.PartyName).
			FieldValue("members", membersJSON).
			FieldValue("dungeon_id", exp.DungeonID).
			FieldValue("map_variant", strconv.Itoa(exp.MapVariant)).
			FieldValue("pos_x", strconv.Itoa(exp.PosX)).
			FieldValue("pos_y", strconv.Itoa(exp.PosY)).
			FieldValue("current_hp", strconv.Itoa(exp.CurrentHP)).
			FieldValue("round", strconv.Itoa(exp.Round)).
			FieldValue("round_limit", strconv.Itoa(exp.RoundLimit)).
			FieldValue("status", string(exp.Status)).
			FieldValue("consumed_events", eventsJSON).
			FieldValue("map_state", string(exp.MapState)).
			FieldValue("started_at", exp.StartedAt.Format(time.RFC3339Nano)).
			FieldValue("updated_at", exp.UpdatedAt.Format(time.RFC3339Nano)).
			Build(),
		r.client.B().Hset().Key(rKey).FieldValue().
			FieldValue("exp", strconv.Itoa(exp.AccumulatedExp)).
			FieldValue("gold", strconv.Itoa(exp.AccumulatedGold)).
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

func (r *ValkeyExpeditionRepository) DeleteActiveExpedition(ctx context.Context, characterID, expeditionID string) error {
	if r.client == nil {
		return r.fallback.DeleteActiveExpedition(ctx, characterID, expeditionID)
	}

	run, err := r.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return err
	}
	if run != nil && run.CharacterID != characterID {
		return ErrForbidden
	}
	if run == nil || run.ID != expeditionID {
		return nil
	}
	if err := r.deleteMemberIndex(ctx, *run); err != nil {
		return err
	}

	return deleteExpeditionScript.Exec(ctx, r.client,
		[]string{r.stateKey(characterID), r.rewardsKey(characterID)},
		[]string{characterID, expeditionID}).Error()
}

func (r *ValkeyExpeditionRepository) Step(ctx context.Context, characterID string, params StepParams) (StepOutcome, error) {
	if r.client == nil {
		return r.fallback.Step(ctx, characterID, params)
	}

	sKey := r.stateKey(characterID)
	rKey := r.rewardsKey(characterID)

	memberHPDeltasJSON := EncodeMemberHPDeltas(params.MemberHPDeltas)
	memberMPDeltasJSON := EncodeMemberHPDeltas(params.MemberMPDeltas)
	consumedItemsJSON := EncodeConsumedItems(params.ConsumedItems)
	memberExpDeltasJSON := EncodeMemberHPDeltas(params.MemberExpDeltas)
	memberGoldDeltasJSON := EncodeMemberHPDeltas(params.MemberGoldDeltas)

	encounterJSON, err := encodeEncounter(params.Encounter)
	if err != nil {
		return StepOutcome{}, err
	}
	expectedRevision := ""
	if params.ExpectedRevision != nil {
		expectedRevision = strconv.Itoa(*params.ExpectedRevision)
	}
	res := r.stepScript.Exec(ctx, r.client, []string{sKey, rKey}, []string{
		params.ExpectedExpeditionID,
		strconv.Itoa(params.NewX),
		strconv.Itoa(params.NewY),
		strconv.Itoa(params.HPDelta),
		strconv.Itoa(params.RoundDelta),
		strconv.Itoa(params.ExpDelta),
		strconv.Itoa(params.GoldDelta),
		params.RewardItemID,
		params.Now.Format(time.RFC3339Nano),
		strconv.FormatInt(int64(r.ttl.Seconds()), 10),
		params.EventKey,
		memberHPDeltasJSON,
		memberMPDeltasJSON,
		consumedItemsJSON,
		memberExpDeltasJSON,
		memberGoldDeltasJSON,
		expectedRevision,
		strconv.FormatBool(params.Combat),
		encounterJSON,
		string(params.FinishStatus),
		strconv.Itoa(params.MapVariant),
		strconv.Itoa(params.RoundLimit),
		string(params.MapState),
		strconv.FormatBool(params.MapState != nil),
	})

	if err := res.Error(); err != nil {
		return StepOutcome{}, mapLuaError(err)
	}

	vals, err := res.AsStrSlice()
	if err != nil || len(vals) < 21 {
		return StepOutcome{}, fmt.Errorf("unexpected lua step response format: %v", err)
	}

	exp, err := (rawValkeyExpedition{
		id: params.ExpectedExpeditionID, charID: characterID, partyID: vals[11], partyName: vals[12],
		dungeonID: vals[9], status: vals[0],
		posX: vals[1], posY: vals[2], hp: vals[3], round: vals[4],
		limit: vals[19], variant: vals[18], revision: vals[17],
		exp: vals[5], gold: vals[6], items: vals[7],
		members: vals[13], consumedEvents: vals[14], encounter: vals[16],
		mapState: vals[20], start: vals[10], updAt: vals[8],
	}).decode()
	if err != nil {
		return StepOutcome{}, err
	}

	return StepOutcome{
		Expedition:    exp,
		Status:        exp.Status,
		EventConsumed: vals[15] != "0",
	}, nil
}

// rawState maps an absent or empty map_state field to no state.
func rawState(s string) json.RawMessage {
	if s == "" {
		return nil
	}
	return json.RawMessage(s)
}
