package replay

import (
	"context"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// ReplayRecorder defines the standardized replay recording interface that can be used
// by any game mode (PvP, GvG, Boss, Dungeon, Challenge, Adventure) to record battle results.
type ReplayRecorder interface {
	RecordBattle(ctx context.Context, req SaveReplayRequest) (string, error)
	RecordMatchFromResult(ctx context.Context, combatType string, initiator ParticipantSnapshot, opponent ParticipantSnapshot, result corebattle.Result) (string, error)
	RecordCharacterVsCharacter(ctx context.Context, combatType string, initiator corecharacter.Character, opponent corecharacter.Character, result corebattle.Result) (string, error)
	RecordCharacterVsMonster(ctx context.Context, combatType string, initiator corecharacter.Character, monsterID, monsterName string, monsterHP, monsterAtk, monsterDef int, result corebattle.Result) (string, error)
	RecordParticipantVsParticipant(ctx context.Context, combatType string, initiator corebattle.Participant, initiatorName string, opponent corebattle.Participant, opponentName string, result corebattle.Result) (string, error)
}

// NewParticipantSnapshot creates a ParticipantSnapshot from explicit stats and attributes.
func NewParticipantSnapshot(id, name string, maxHP, attack, defense, agility int, jobID string, level int) ParticipantSnapshot {
	if name == "" {
		name = id
	}
	return ParticipantSnapshot{
		ID:      id,
		Name:    name,
		MaxHP:   maxHP,
		Attack:  attack,
		Defense: defense,
		Agility: agility,
		JobID:   jobID,
		Level:   level,
	}
}

// NewParticipantSnapshotFromCharacter creates a ParticipantSnapshot from a Core Character entity.
func NewParticipantSnapshotFromCharacter(c corecharacter.Character) ParticipantSnapshot {
	maxHP := c.Stats.MaxHP
	if maxHP <= 0 {
		maxHP = c.Stats.HP
	}
	name := c.Name
	if name == "" {
		name = c.ID
	}
	return ParticipantSnapshot{
		ID:      c.ID,
		Name:    name,
		MaxHP:   maxHP,
		Attack:  c.Stats.Attack,
		Defense: c.Stats.Defense,
		Agility: c.Stats.Agility,
		JobID:   c.JobID,
		Level:   c.Level,
	}
}

// NewParticipantSnapshotFromParticipant creates a ParticipantSnapshot from a Core Battle Participant.
func NewParticipantSnapshotFromParticipant(p corebattle.Participant, name string) ParticipantSnapshot {
	if name == "" {
		name = p.ID
	}
	return ParticipantSnapshot{
		ID:      p.ID,
		Name:    name,
		MaxHP:   p.HP,
		Attack:  p.Attack,
		Defense: p.Defense,
	}
}

// NewParticipantSnapshotFromMonster creates a ParticipantSnapshot representing a PvE monster or boss.
func NewParticipantSnapshotFromMonster(id, name string, hp, attack, defense int) ParticipantSnapshot {
	if name == "" {
		name = id
	}
	return ParticipantSnapshot{
		ID:      id,
		Name:    name,
		MaxHP:   hp,
		Attack:  attack,
		Defense: defense,
	}
}

// RecordMatchFromResult records a battle replay from a completed corebattle.Result and participant snapshots.
func (s *Service) RecordMatchFromResult(
	ctx context.Context,
	combatType string,
	initiator ParticipantSnapshot,
	opponent ParticipantSnapshot,
	result corebattle.Result,
) (string, error) {
	return s.RecordBattle(ctx, SaveReplayRequest{
		CombatType:   combatType,
		Initiator:    initiator,
		Opponent:     opponent,
		BattleResult: result,
	})
}

// RecordCharacterVsCharacter records a battle replay for PvP combat between two characters.
func (s *Service) RecordCharacterVsCharacter(
	ctx context.Context,
	combatType string,
	initiator corecharacter.Character,
	opponent corecharacter.Character,
	result corebattle.Result,
) (string, error) {
	return s.RecordMatchFromResult(
		ctx,
		combatType,
		NewParticipantSnapshotFromCharacter(initiator),
		NewParticipantSnapshotFromCharacter(opponent),
		result,
	)
}

// RecordCharacterVsMonster records a battle replay for PvE combat between a character and a monster/boss.
func (s *Service) RecordCharacterVsMonster(
	ctx context.Context,
	combatType string,
	initiator corecharacter.Character,
	monsterID, monsterName string,
	monsterHP, monsterAtk, monsterDef int,
	result corebattle.Result,
) (string, error) {
	return s.RecordMatchFromResult(
		ctx,
		combatType,
		NewParticipantSnapshotFromCharacter(initiator),
		NewParticipantSnapshotFromMonster(monsterID, monsterName, monsterHP, monsterAtk, monsterDef),
		result,
	)
}

// RecordParticipantVsParticipant records a battle replay directly from corebattle.Participant inputs and names.
func (s *Service) RecordParticipantVsParticipant(
	ctx context.Context,
	combatType string,
	initiator corebattle.Participant,
	initiatorName string,
	opponent corebattle.Participant,
	opponentName string,
	result corebattle.Result,
) (string, error) {
	return s.RecordMatchFromResult(
		ctx,
		combatType,
		NewParticipantSnapshotFromParticipant(initiator, initiatorName),
		NewParticipantSnapshotFromParticipant(opponent, opponentName),
		result,
	)
}
