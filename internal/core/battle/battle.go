package battle

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidRequest     = errors.New("battle request is invalid")
	ErrInvalidParticipant = errors.New("battle participant is invalid")
	ErrInvalidReward      = errors.New("battle reward is invalid")
)

type Participant struct {
	ID                string
	Name              string
	HP                int
	MaxHP             int
	MP                int
	MaxMP             int
	CMP               int
	MaxCMP            int
	Attack            int
	Defense           int
	Agility           int
	Abilities         []string
	Skills            []ActionSkill
	CustomSkills      []ActionCustomSkill
	Defending         bool
	Status            string
	ItemDefinitionIDs []string
}

type Request struct {
	Participants  []Participant
	VictoryReward Reward
	DefeatReward  Reward
	DrawReward    Reward
}

type Outcome string

const (
	OutcomeWin    Outcome = "win"
	OutcomeDefeat Outcome = "defeat"
	OutcomeDraw   Outcome = "draw"
)

type TurnLog struct {
	Turn        int            `json:"turn"`
	ActorID     string         `json:"actor_id"`
	ActionName  string         `json:"action_name"`
	TargetID    string         `json:"target_id"`
	DamageDealt int            `json:"damage_dealt"`
	HealingDone int            `json:"healing_done"`
	IsCritical  bool           `json:"is_critical"`
	Message     string         `json:"message"`
	RemainingHP map[string]int `json:"remaining_hp"`
}

type Result struct {
	Outcome  Outcome
	WinnerID string
	LoserID  string
	Turns    int
	// Reward is selected from the first participant's perspective.
	// VictoryReward applies when the first participant wins, DefeatReward when
	// it loses, and DrawReward for a draw.
	Reward Reward
	Logs   []TurnLog `json:"logs,omitempty"`
}

type Reward struct {
	Experience       int
	Currency         int
	ItemDefinitionID string
	ItemQuantity     int
	SmallMedals      int
}

type Effect struct {
	Kind  string
	Power int
}

type Resolver interface {
	Resolve(request Request) (Result, error)
}

type PartyBattleRequest struct {
	Allies        []Participant
	Enemies       []Participant
	InitialField  *FieldState
	VictoryReward Reward
	DefeatReward  Reward
	DrawReward    Reward
}

type PartyBattleResult struct {
	Outcome        Outcome        `json:"outcome"`
	WinnerSide     string         `json:"winner_side"`
	Turns          int            `json:"turns"`
	BaseReward     Reward         `json:"base_reward"`
	BonusPercent   int            `json:"bonus_percent"`
	TotalReward    Reward         `json:"total_reward"`
	AlliesSurvived []string       `json:"allies_survived"`
	AlliesFallen   []string       `json:"allies_fallen"`
	RemainingHP    map[string]int `json:"remaining_hp"`
	Logs           []TurnLog      `json:"logs,omitempty"`
	FinalField     *FieldState    `json:"final_field,omitempty"`
}

type Engine struct{}

func (Engine) Resolve(request Request) (Result, error) {
	if len(request.Participants) != 2 {
		return Result{}, ErrInvalidRequest
	}
	first, second := request.Participants[0], request.Participants[1]
	if err := validateParticipant(first); err != nil {
		return Result{}, err
	}
	if err := validateParticipant(second); err != nil {
		return Result{}, err
	}
	if first.ID == second.ID {
		return Result{}, ErrInvalidRequest
	}
	for _, reward := range []Reward{request.VictoryReward, request.DefeatReward, request.DrawReward} {
		if err := validateReward(reward); err != nil {
			return Result{}, err
		}
	}

	firstHP, secondHP := first.HP, second.HP
	turns := 0
	var logs []TurnLog
	for firstHP > 0 && secondHP > 0 {
		turns++
		dmg1 := damage(first.Attack, second.Defense)
		secondHP -= dmg1
		if secondHP < 0 {
			secondHP = 0
		}
		logs = append(logs, TurnLog{
			Turn:        turns,
			ActorID:     first.ID,
			ActionName:  "こうげき",
			TargetID:    second.ID,
			DamageDealt: dmg1,
			Message:     fmt.Sprintf("%s の攻撃！ %s に %d のダメージ！", first.ID, second.ID, dmg1),
			RemainingHP: map[string]int{
				first.ID:  firstHP,
				second.ID: secondHP,
			},
		})

		if secondHP <= 0 {
			if firstHP-damage(second.Attack, first.Defense) <= 0 {
				return Result{Outcome: OutcomeDraw, Turns: turns, Reward: request.DrawReward, Logs: logs}, nil
			}
			return Result{Outcome: OutcomeWin, WinnerID: first.ID, LoserID: second.ID, Turns: turns, Reward: request.VictoryReward, Logs: logs}, nil
		}

		dmg2 := damage(second.Attack, first.Defense)
		firstHP -= dmg2
		if firstHP < 0 {
			firstHP = 0
		}
		logs = append(logs, TurnLog{
			Turn:        turns,
			ActorID:     second.ID,
			ActionName:  "こうげき",
			TargetID:    first.ID,
			DamageDealt: dmg2,
			Message:     fmt.Sprintf("%s の攻撃！ %s に %d のダメージ！", second.ID, first.ID, dmg2),
			RemainingHP: map[string]int{
				first.ID:  firstHP,
				second.ID: secondHP,
			},
		})
	}
	if firstHP <= 0 {
		return Result{Outcome: OutcomeWin, WinnerID: second.ID, LoserID: first.ID, Turns: turns, Reward: request.DefeatReward, Logs: logs}, nil
	}
	return Result{Outcome: OutcomeWin, WinnerID: first.ID, LoserID: second.ID, Turns: turns, Reward: request.VictoryReward, Logs: logs}, nil
}

func validateParticipant(value Participant) error {
	if value.ID == "" || value.HP <= 0 || value.Attack < 0 || value.Defense < 0 {
		return ErrInvalidParticipant
	}
	return nil
}

func validateReward(value Reward) error {
	if value.Experience < 0 || value.Currency < 0 ||
		(value.ItemDefinitionID == "" && value.ItemQuantity != 0) ||
		(value.ItemDefinitionID != "" && value.ItemQuantity <= 0) {
		return ErrInvalidReward
	}
	return nil
}

func damage(attack, defense int) int {
	value := attack - defense
	if value < 1 {
		return 1
	}
	return value
}
