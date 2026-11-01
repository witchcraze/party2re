package dungeon

import (
	"context"
	"encoding/json"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// ActiveEncounter retains the enemy and destination until combat finishes.
type ActiveEncounter struct {
	Monster   DungeonMonster           `json:"monster"`
	Enemies   []corebattle.Participant `json:"enemies"`
	EventType TileEventType            `json:"event_type"`
	X         int                      `json:"x"`
	Y         int                      `json:"y"`
}

func pendingEncounter(exp *ActiveExpedition, monster DungeonMonster, event TileEventType, x, y int, result corebattle.Result) *ActiveEncounter {
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
	return &ActiveEncounter{Monster: monster, Enemies: []corebattle.Participant{enemy}, EventType: event, X: x, Y: y}
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
	if exp == nil || exp.Status != StatusExploring {
		return ExpeditionStepResult{}, ErrNoActiveExpedition
	}
	if exp.CharacterID != characterID {
		return ExpeditionStepResult{}, ErrForbidden
	}
	enc := exp.Encounter
	if enc == nil {
		return ExpeditionStepResult{}, ErrNoActiveCombat
	}
	char, err := s.characterRepo.FindByID(ctx, characterID)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	if enc.EventType == EventBoss {
		dungeon, ok := s.dungeonMap[exp.DungeonID]
		if !ok {
			return ExpeditionStepResult{}, ErrDungeonNotFound
		}
		return s.resolveBossCombat(ctx, exp, &char, dungeon, enc.Monster, enc.X, enc.Y)
	}
	return s.resolveMonsterCombat(ctx, exp, &char, enc.Monster, enc.EventType, enc.X, enc.Y)
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
	return &cp
}
