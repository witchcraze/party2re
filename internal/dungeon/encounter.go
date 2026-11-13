package dungeon

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// ActiveEncounter retains the enemy and destination until combat finishes.
type ActiveEncounter struct {
	Statuses  map[string]string        `json:"statuses,omitempty"`
	CMP       map[string]int           `json:"cmp,omitempty"`
	Field     *corebattle.FieldState   `json:"field,omitempty"`
	Monster   DungeonMonster           `json:"monster"`
	Enemies   []corebattle.Participant `json:"enemies"`
	EventType TileEventType            `json:"event_type"`
	X         int                      `json:"x"`
	Y         int                      `json:"y"`
	// Chests are the enemy-side members of a pending treasure (no Enemies).
	Chests []TreasureChest `json:"chests,omitempty"`
}

// TreasureChest is an unopened chest until actor inspection opens it.
type TreasureChest struct {
	Opened bool `json:"opened,omitempty"`
}

// blocking reports living enemy-side members; they forbid ordinary movement.
func (enc *ActiveEncounter) blocking() bool {
	if enc == nil {
		return false
	}
	if enc.EventType != EventTreasure {
		return true
	}
	return slices.ContainsFunc(enc.Chests, func(c TreasureChest) bool { return !c.Opened })
}

// hostile reports an encounter that Fight resolves; treasure is not combat.
func (enc *ActiveEncounter) hostile() bool { return enc != nil && enc.EventType != EventTreasure }

func pendingEncounter(exp *ActiveExpedition, monster DungeonMonster, event TileEventType, x, y int, result corebattle.Result, field *corebattle.FieldState) *ActiveEncounter {
	enemy := corebattle.MustNewParticipant(monster.ID, monster.HP, monster.Attack, monster.Defense)
	enemy.Name, enemy.Agility, enemy.TeamID = monster.Name, monster.Agility, "enemies"
	if exp.Encounter != nil {
		enemy = exp.Encounter.Enemies[0]
	}
	if hp, ok := result.RemainingHP[enemy.ID]; ok {
		enemy.HP = hp
	}
	if mp, ok := result.RemainingMP[enemy.ID]; ok {
		enemy.MP = mp
	}
	if status, ok := result.RemainingStatus[enemy.ID]; ok {
		enemy.Status = status
	}
	statuses := make(map[string]string)
	cmp := make(map[string]int)
	if exp.Encounter != nil {
		maps.Copy(statuses, exp.Encounter.Statuses)
		maps.Copy(cmp, exp.Encounter.CMP)
		if field == nil {
			field = exp.Encounter.Field
		}
	}
	maps.Copy(statuses, result.RemainingStatus)
	maps.Copy(cmp, result.RemainingCMP)
	if value, ok := cmp[enemy.ID]; ok {
		enemy.CMP = value
	}
	return &ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{enemy}, EventType: event, X: x, Y: y, Statuses: statuses, CMP: cmp, Field: field.Clone()}
}

// Fight resumes the current encounter through the shared battle resolver.
func (s *Service) Fight(ctx context.Context, characterID string) (ExpeditionStepResult, error) {
	if characterID == "" {
		return ExpeditionStepResult{}, ErrCharacterNotFound
	}
	exp, err := s.GetActiveExpedition(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp == nil {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	if exp.CharacterID != characterID {
		return ExpeditionStepResult{}, ErrForbidden
	}
	if exp.Status != StatusExploring {
		return s.resumeSettlement(ctx, exp)
	}
	enc := exp.Encounter
	if !enc.hostile() {
		return ExpeditionStepResult{}, ErrNoActiveCombat
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	return s.resolveMonsterCombat(ctx, exp, &char, enc.Monster, enc.EventType, enc.X, enc.Y, "")
}

func encodeEncounter(enc *ActiveEncounter) (string, error) {
	if enc == nil {
		return "", nil
	}
	b, err := json.Marshal(enc)
	return string(b), err
}

func decodeEncounter(data string) (*ActiveEncounter, error) {
	if data == "" {
		return nil, nil
	}
	var enc ActiveEncounter
	if err := json.Unmarshal([]byte(data), &enc); err != nil {
		return nil, err
	}
	if enc.EventType == EventTreasure {
		if len(enc.Chests) == 0 || len(enc.Enemies) != 0 {
			return nil, corebattle.ErrInvalidParticipant
		}
		return &enc, nil
	}
	if len(enc.Enemies) != 1 || enc.Enemies[0].HP <= 0 {
		return nil, corebattle.ErrInvalidParticipant
	}
	return &enc, nil
}

func cloneEncounter(enc *ActiveEncounter) *ActiveEncounter {
	if enc == nil {
		return nil
	}
	cp := *enc
	cp.Enemies = slices.Clone(enc.Enemies)
	cp.Chests = slices.Clone(enc.Chests)
	cp.Statuses = maps.Clone(enc.Statuses)
	cp.CMP = maps.Clone(enc.CMP)
	cp.Field = enc.Field.Clone()
	return &cp
}

func (enc *ActiveEncounter) restoreParticipant(p *corebattle.Participant) {
	if enc == nil {
		return
	}
	if status, ok := enc.Statuses[p.ID]; ok {
		p.Status = status
	}
	if cmp, ok := enc.CMP[p.ID]; ok {
		p.CMP = cmp
	}
}
