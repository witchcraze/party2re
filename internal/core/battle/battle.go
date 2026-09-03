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
	ID      string
	Name    string
	HP      int
	Attack  int
	Defense int
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

// ResolvePartyBattle resolves a multi-participant turn-based battle between a party of allies and enemies.
func (Engine) ResolvePartyBattle(req PartyBattleRequest) (PartyBattleResult, error) {
	if len(req.Allies) == 0 || len(req.Enemies) == 0 {
		return PartyBattleResult{}, ErrInvalidRequest
	}
	for _, a := range req.Allies {
		if err := validateParticipant(a); err != nil {
			return PartyBattleResult{}, err
		}
	}
	for _, e := range req.Enemies {
		if err := validateParticipant(e); err != nil {
			return PartyBattleResult{}, err
		}
	}
	for _, reward := range []Reward{req.VictoryReward, req.DefeatReward, req.DrawReward} {
		if err := validateReward(reward); err != nil {
			return PartyBattleResult{}, err
		}
	}

	// Calculate multiplayer synergy bonus
	// 1 ally: 0%, 2 allies: 10%, 3 allies: 20%, 4+ allies: 30%
	bonusPercent := (len(req.Allies) - 1) * 10
	if bonusPercent < 0 {
		bonusPercent = 0
	} else if bonusPercent > 30 {
		bonusPercent = 30
	}

	hpMap := make(map[string]int)
	for _, a := range req.Allies {
		hpMap[a.ID] = a.HP
	}
	for _, e := range req.Enemies {
		hpMap[e.ID] = e.HP
	}

	var logs []TurnLog
	turns := 0
	maxTurns := 100

	for turns < maxTurns {
		turns++

		// 1. Allies turn
		for _, ally := range req.Allies {
			if hpMap[ally.ID] <= 0 {
				continue
			}
			// Find living enemy with lowest HP
			var target *Participant
			for i := range req.Enemies {
				if hpMap[req.Enemies[i].ID] > 0 {
					if target == nil || hpMap[req.Enemies[i].ID] < hpMap[target.ID] {
						target = &req.Enemies[i]
					}
				}
			}
			if target == nil {
				break // All enemies down
			}

			dmg := damage(ally.Attack, target.Defense)
			hpMap[target.ID] -= dmg
			if hpMap[target.ID] < 0 {
				hpMap[target.ID] = 0
			}

			snapshot := make(map[string]int, len(hpMap))
			for k, v := range hpMap {
				snapshot[k] = v
			}

			allyName := ally.Name
			if allyName == "" {
				allyName = ally.ID
			}
			targetName := target.Name
			if targetName == "" {
				targetName = target.ID
			}

			logs = append(logs, TurnLog{
				Turn:        turns,
				ActorID:     ally.ID,
				ActionName:  "こうげき",
				TargetID:    target.ID,
				DamageDealt: dmg,
				Message:     fmt.Sprintf("%s の攻撃！ %s に %d のダメージ！", allyName, targetName, dmg),
				RemainingHP: snapshot,
			})
		}

		// Check if all enemies are defeated
		allEnemiesDown := true
		for _, e := range req.Enemies {
			if hpMap[e.ID] > 0 {
				allEnemiesDown = false
				break
			}
		}
		if allEnemiesDown {
			var survived, fallen []string
			for _, a := range req.Allies {
				if hpMap[a.ID] > 0 {
					survived = append(survived, a.ID)
				} else {
					fallen = append(fallen, a.ID)
				}
			}
			totalReward := req.VictoryReward
			totalReward.Experience = (totalReward.Experience * (100 + bonusPercent)) / 100
			totalReward.Currency = (totalReward.Currency * (100 + bonusPercent)) / 100

			finalSnapshot := make(map[string]int, len(hpMap))
			for k, v := range hpMap {
				finalSnapshot[k] = v
			}

			return PartyBattleResult{
				Outcome:        OutcomeWin,
				WinnerSide:     "allies",
				Turns:          turns,
				BaseReward:     req.VictoryReward,
				BonusPercent:   bonusPercent,
				TotalReward:    totalReward,
				AlliesSurvived: survived,
				AlliesFallen:   fallen,
				RemainingHP:    finalSnapshot,
				Logs:           logs,
			}, nil
		}

		// 2. Enemies turn
		for _, enemy := range req.Enemies {
			if hpMap[enemy.ID] <= 0 {
				continue
			}
			// Find living ally with lowest HP
			var target *Participant
			for i := range req.Allies {
				if hpMap[req.Allies[i].ID] > 0 {
					if target == nil || hpMap[req.Allies[i].ID] < hpMap[target.ID] {
						target = &req.Allies[i]
					}
				}
			}
			if target == nil {
				break // All allies down
			}

			dmg := damage(enemy.Attack, target.Defense)
			hpMap[target.ID] -= dmg
			if hpMap[target.ID] < 0 {
				hpMap[target.ID] = 0
			}

			snapshot := make(map[string]int, len(hpMap))
			for k, v := range hpMap {
				snapshot[k] = v
			}

			enemyName := enemy.Name
			if enemyName == "" {
				enemyName = enemy.ID
			}
			targetName := target.Name
			if targetName == "" {
				targetName = target.ID
			}

			logs = append(logs, TurnLog{
				Turn:        turns,
				ActorID:     enemy.ID,
				ActionName:  "こうげき",
				TargetID:    target.ID,
				DamageDealt: dmg,
				Message:     fmt.Sprintf("%s の攻撃！ %s に %d のダメージ！", enemyName, targetName, dmg),
				RemainingHP: snapshot,
			})
		}

		// Check if all allies are defeated
		allAlliesDown := true
		for _, a := range req.Allies {
			if hpMap[a.ID] > 0 {
				allAlliesDown = false
				break
			}
		}
		if allAlliesDown {
			var fallen []string
			for _, a := range req.Allies {
				fallen = append(fallen, a.ID)
			}
			finalSnapshot := make(map[string]int, len(hpMap))
			for k, v := range hpMap {
				finalSnapshot[k] = v
			}
			return PartyBattleResult{
				Outcome:        OutcomeDefeat,
				WinnerSide:     "enemies",
				Turns:          turns,
				BaseReward:     req.DefeatReward,
				BonusPercent:   0,
				TotalReward:    req.DefeatReward,
				AlliesSurvived: []string{},
				AlliesFallen:   fallen,
				RemainingHP:    finalSnapshot,
				Logs:           logs,
			}, nil
		}
	}

	// Timeout draw
	var survived, fallen []string
	for _, a := range req.Allies {
		if hpMap[a.ID] > 0 {
			survived = append(survived, a.ID)
		} else {
			fallen = append(fallen, a.ID)
		}
	}
	finalSnapshot := make(map[string]int, len(hpMap))
	for k, v := range hpMap {
		finalSnapshot[k] = v
	}
	return PartyBattleResult{
		Outcome:        OutcomeDraw,
		WinnerSide:     "none",
		Turns:          turns,
		BaseReward:     req.DrawReward,
		BonusPercent:   0,
		TotalReward:    req.DrawReward,
		AlliesSurvived: survived,
		AlliesFallen:   fallen,
		RemainingHP:    finalSnapshot,
		Logs:           logs,
	}, nil
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
