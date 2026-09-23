package battle

import (
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

var (
	ErrInvalidRequest     = errors.New("battle request is invalid")
	ErrInvalidParticipant = errors.New("battle participant is invalid")
	ErrInvalidReward      = errors.New("battle reward is invalid")
	ErrCannotUseInCombat  = item.ErrCannotUseInCombat
)

type Participant struct {
	ID                string
	Name              string
	TeamID            string
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
	ActionItems       []ActionItem
	Defending         bool
	Status            string
	ItemDefinitionIDs []string
}

type Request struct {
	Participants  []Participant
	VictoryReward Reward
	DefeatReward  Reward
	DrawReward    Reward
	RNG           random.Generator
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
	Crystals         int
}

type Effect struct {
	Kind  string
	Power int
}

type Resolver interface {
	Resolve(request Request) (Result, error)
}

type PartyBattleResolver interface {
	ResolvePartyBattle(req PartyBattleRequest) (PartyBattleResult, error)
}

type PartyBattleRequest struct {
	Allies        []Participant
	Enemies       []Participant
	Teams         map[string][]Participant
	InitialField  *FieldState
	VictoryReward Reward
	DefeatReward  Reward
	DrawReward    Reward
	RNG           random.Generator
}

// ConsumedItem records an item consumed during combat.
type ConsumedItem struct {
	ID         string `json:"id"`
	InstanceID string `json:"instance_id,omitempty"`
	Quantity   int    `json:"quantity"`
}

type PartyBattleResult struct {
	Outcome         Outcome                   `json:"outcome"`
	WinnerSide      string                    `json:"winner_side"`
	WinnerTeam      string                    `json:"winner_team,omitempty"`
	Turns           int                       `json:"turns"`
	BaseReward      Reward                    `json:"base_reward"`
	BonusPercent    int                       `json:"bonus_percent"`
	TotalReward     Reward                    `json:"total_reward"`
	AlliesSurvived  []string                  `json:"allies_survived"`
	AlliesFallen    []string                  `json:"allies_fallen"`
	RemainingHP     map[string]int            `json:"remaining_hp"`
	RemainingMP     map[string]int            `json:"remaining_mp,omitempty"`
	RemainingCMP    map[string]int            `json:"remaining_cmp,omitempty"`
	RemainingStatus map[string]string         `json:"remaining_status,omitempty"`
	ConsumedItems   map[string][]ConsumedItem `json:"consumed_items,omitempty"`
	BanishedIDs     map[string]bool           `json:"banished_ids,omitempty"`
	Logs            []TurnLog                 `json:"logs,omitempty"`
	FinalField      *FieldState               `json:"final_field,omitempty"`
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
	rng := request.RNG
	if rng == nil {
		rng = random.Default()
	}
	for firstHP > 0 && secondHP > 0 {
		turns++
		dmg1 := CalculateDamage(first.Attack, second.Defense, rng, false)
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
			if firstHP-CalculateDamage(second.Attack, first.Defense, rng, false) <= 0 {
				return Result{Outcome: OutcomeDraw, Turns: turns, Reward: request.DrawReward, Logs: logs}, nil
			}
			return Result{Outcome: OutcomeWin, WinnerID: first.ID, LoserID: second.ID, Turns: turns, Reward: request.VictoryReward, Logs: logs}, nil
		}

		dmg2 := CalculateDamage(second.Attack, first.Defense, rng, false)
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
	for _, it := range value.ActionItems {
		if err := ValidateItemAction(it); err != nil {
			return err
		}
	}
	return nil
}

func validateReward(value Reward) error {
	if value.Experience < 0 || value.Currency < 0 || value.Crystals < 0 ||
		(value.ItemDefinitionID == "" && value.ItemQuantity != 0) ||
		(value.ItemDefinitionID != "" && value.ItemQuantity <= 0) {
		return ErrInvalidReward
	}
	return nil
}

// CalculateDamage computes damage according to canonical DQ / legacy Party2 rules:
// - Physical base: int(Atk * 0.5 - Def * 0.3).
// - Direct / Critical strike: int(Atk * 0.75), bypassing defense mitigation.
// - Random variance: multiplied by 0.9..1.2 (rand(0.3) + 0.9).
// - Minimum damage: if resulting damage < 1, deals 1 or 2 (int(rand(2) + 1)).
func CalculateDamage(attack, defense int, rng random.Generator, isDirect bool) int {
	var base float64
	if isDirect {
		base = float64(attack) * 0.75
	} else {
		base = float64(attack)*0.5 - float64(defense)*0.3
	}
	variance := 1.0
	minDmg := 1
	if rng != nil {
		variance = 0.9 + rng.Float64()*0.3
		minDmg = rng.Intn(2) + 1
	}
	dmg := int(base * variance)
	if dmg < 1 {
		return minDmg
	}
	return dmg
}

func damage(attack, defense int) int {
	return CalculateDamage(attack, defense, nil, false)
}
