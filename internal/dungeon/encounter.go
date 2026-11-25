package dungeon

import (
	"context"
	"encoding/json"
	"maps"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// ActiveEncounter retains the spawned group and destination until combat
// finishes. Monsters[i] is the rolled source of Enemies[i]; defeated members
// stay until victory.
type ActiveEncounter struct {
	Statuses  map[string]string        `json:"statuses,omitempty"`
	CMP       map[string]int           `json:"cmp,omitempty"`
	Field     *corebattle.FieldState   `json:"field,omitempty"`
	Monsters  []DungeonMonster         `json:"monsters"`
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

// pendingEncounter keeps the group after a draw or defeat, or nil once no
// enemy survives.
func pendingEncounter(exp *ActiveExpedition, monsters []DungeonMonster, enemies []corebattle.Participant, event TileEventType, x, y int, result corebattle.Result, field *corebattle.FieldState) *ActiveEncounter {
	enemies = slices.Clone(enemies)
	for i := range enemies {
		enemy := &enemies[i]
		if hp, ok := result.RemainingHP[enemy.ID]; ok {
			enemy.HP = hp
		}
		if mp, ok := result.RemainingMP[enemy.ID]; ok {
			enemy.MP = mp
		}
		if status, ok := result.RemainingStatus[enemy.ID]; ok {
			enemy.Status = status
		}
		if value, ok := result.RemainingCMP[enemy.ID]; ok {
			enemy.CMP = value
		}
	}
	if !slices.ContainsFunc(enemies, func(e corebattle.Participant) bool { return e.HP > 0 }) {
		return nil
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
	return &ActiveEncounter{Monsters: monsters, Enemies: enemies, EventType: event, X: x, Y: y, Statuses: statuses, CMP: cmp, Field: field.Clone()}
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
	if err := s.settleCredits(ctx, exp); err != nil {
		return ExpeditionStepResult{}, err
	}
	if exp.Status != StatusExploring {
		return s.resumeSettlement(ctx, exp)
	}
	enc := exp.Encounter
	if !enc.hostile() {
		return ExpeditionStepResult{}, ErrNoActiveCombat
	}
	if !exp.anyAlive() {
		return ExpeditionStepResult{}, ErrActorFallen
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	// A fallen leader's command never applies its stock (_battle.cgi:654–664).
	var carry StepParams
	if exp.isAlive(characterID) {
		carry.StockDrain = characterID
	}
	return s.resolveMonsterCombat(ctx, exp, &char, enc.Monsters, enc.EventType, enc.X, enc.Y, "", "", nil, carry, nil)
}

// encounterRecord adds each enemy's private combat data, by index, only to
// the stored buffer.
type encounterRecord struct {
	*ActiveEncounter
	EnemyCombat []*corebattle.LifetimeActor `json:"enemy_combat,omitempty"`
}

func encodeEncounter(enc *ActiveEncounter) (string, error) {
	if enc == nil {
		return "", nil
	}
	record := encounterRecord{ActiveEncounter: enc}
	if len(enc.Monsters) > 0 {
		record.EnemyCombat = make([]*corebattle.LifetimeActor, len(enc.Monsters))
		for i, m := range enc.Monsters {
			record.EnemyCombat[i] = m.combat
		}
	}
	b, err := json.Marshal(record)
	return string(b), err
}

func decodeEncounter(data string) (*ActiveEncounter, error) {
	if data == "" {
		return nil, nil
	}
	var enc ActiveEncounter
	record := encounterRecord{ActiveEncounter: &enc}
	if err := json.Unmarshal([]byte(data), &record); err != nil {
		return nil, err
	}
	if enc.EventType == EventTreasure {
		if len(enc.Chests) == 0 || len(enc.Enemies) != 0 || len(enc.Monsters) != 0 {
			return nil, corebattle.ErrInvalidParticipant
		}
		return &enc, nil
	}
	n := len(enc.Enemies)
	if n == 0 || n > maxEncounterEnemies || len(enc.Monsters) != n || len(record.EnemyCombat) != n ||
		!slices.ContainsFunc(enc.Enemies, func(e corebattle.Participant) bool { return e.HP > 0 }) {
		return nil, corebattle.ErrInvalidParticipant
	}
	for i := range enc.Monsters {
		enc.Monsters[i].combat = record.EnemyCombat[i]
	}
	return &enc, nil
}

func cloneEncounter(enc *ActiveEncounter) *ActiveEncounter {
	if enc == nil {
		return nil
	}
	cp := *enc
	cp.Monsters = slices.Clone(enc.Monsters)
	for i, m := range cp.Monsters {
		if m.combat != nil {
			combat := m.combat.Clone()
			cp.Monsters[i].combat = &combat
		}
	}
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
