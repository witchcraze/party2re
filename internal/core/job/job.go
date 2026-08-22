package job

import (
	"errors"
	"strings"
)

var (
	ErrInvalidDefinition = errors.New("job definition is invalid")
	ErrInvalidCharacter  = errors.New("character job state is invalid")
	ErrJobUnavailable    = errors.New("job requirements are not met")
)

type Definition struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	HPGrowth       int    `json:"hp_growth"`
	MPGrowth       int    `json:"mp_growth"`
	AttackGrowth   int    `json:"attack_growth"`
	DefenseGrowth  int    `json:"defense_growth"`
	AgilityGrowth  int    `json:"agility_growth"`
	RequiredGender string `json:"required_gender,omitempty"`
	MinLevel       int    `json:"min_level"`
}

func NewDefinition(id, name string, hp, mp, attack, defense, agility, minLevel int, gender string) (Definition, error) {
	if strings.TrimSpace(id) == "" || strings.TrimSpace(name) == "" || hp < 0 || mp < 0 ||
		attack < 0 || defense < 0 || agility < 0 || minLevel < 1 {
		return Definition{}, ErrInvalidDefinition
	}
	return Definition{
		ID: id, Name: name, HPGrowth: hp, MPGrowth: mp, AttackGrowth: attack,
		DefenseGrowth: defense, AgilityGrowth: agility, RequiredGender: gender, MinLevel: minLevel,
	}, nil
}

type Change struct {
	FromJobID string
	ToJobID   string
}

type CharacterJob struct {
	CharacterID  string
	CurrentJobID string
	History      []Change
}

func NewCharacterJob(characterID, currentJobID string) (CharacterJob, error) {
	if strings.TrimSpace(characterID) == "" || strings.TrimSpace(currentJobID) == "" {
		return CharacterJob{}, ErrInvalidCharacter
	}
	return CharacterJob{CharacterID: characterID, CurrentJobID: currentJobID}, nil
}

func (c *CharacterJob) ChangeTo(target Definition, level int, gender string) error {
	if c == nil || c.CharacterID == "" || c.CurrentJobID == "" {
		return ErrInvalidCharacter
	}
	if target.ID == "" || level < target.MinLevel ||
		(target.RequiredGender != "" && target.RequiredGender != gender) {
		return ErrJobUnavailable
	}
	if target.ID == c.CurrentJobID {
		return ErrJobUnavailable
	}
	c.History = append(c.History, Change{FromJobID: c.CurrentJobID, ToJobID: target.ID})
	c.CurrentJobID = target.ID
	return nil
}
