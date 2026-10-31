package battle

import (
	"errors"

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
	BaseStats         BattleBaseStats
	Gear              *BattleGearStats
	OverLevel         bool
	Abilities         []string
	Skills            []ActionSkill
	CustomSkills      []ActionCustomSkill
	ActionItems       []ActionItem
	Defending         bool
	Status            string
	ItemDefinitionIDs []string
	Icon              string
	ExperienceReward  int
	GoldReward        int
}

// BattleBaseStats are the equipment-free stat baselines, never current buffs.
type BattleBaseStats struct {
	Attack, Defense, Agility int
}

// BattleGearStats retains the already evaluated table values. Armor defense is
// a separate term from current DF; weights are used by extension201's reset.
type BattleGearStats struct {
	ArmorDefense, WeaponWeight, ArmorWeight int
	WeaponID, ArmorID, AccessoryID          string
	// WeaponAttack is the evaluated table value before any reset/revision.
	// nil with an equipped weapon is an unrepresented load context.
	WeaponAttack *int
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
	Reward          Reward
	RemainingHP     map[string]int            `json:"remaining_hp,omitempty"`
	RemainingMP     map[string]int            `json:"remaining_mp,omitempty"`
	RemainingCMP    map[string]int            `json:"remaining_cmp,omitempty"`
	RemainingStatus map[string]string         `json:"remaining_status,omitempty"`
	ConsumedItems   map[string][]ConsumedItem `json:"consumed_items,omitempty"`
	Logs            []TurnLog                 `json:"logs,omitempty"`
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

func (e Engine) Resolve(request Request) (Result, error) {
	if len(request.Participants) != 2 {
		return Result{}, ErrInvalidRequest
	}
	first, second := request.Participants[0], request.Participants[1]
	if first.ID == second.ID {
		return Result{}, ErrInvalidRequest
	}

	firstTeam := first.TeamID
	secondTeam := second.TeamID
	if firstTeam == "" {
		firstTeam = "allies"
	}
	if secondTeam == "" || secondTeam == firstTeam {
		if firstTeam == "enemies" {
			secondTeam = "allies"
		} else {
			secondTeam = "enemies"
		}
	}
	if firstTeam == secondTeam {
		firstTeam = "allies"
		secondTeam = "enemies"
	}

	firstCopy := first
	firstCopy.TeamID = firstTeam
	secondCopy := second
	secondCopy.TeamID = secondTeam

	partyRes, err := e.ResolvePartyBattle(PartyBattleRequest{
		Allies:        []Participant{firstCopy},
		Enemies:       []Participant{secondCopy},
		VictoryReward: request.VictoryReward,
		DefeatReward:  request.DefeatReward,
		DrawReward:    request.DrawReward,
		RNG:           request.RNG,
	})
	if err != nil {
		return Result{}, err
	}

	var outcome Outcome
	var winnerID string
	var loserID string

	switch partyRes.Outcome {
	case OutcomeWin:
		outcome = OutcomeWin
		winnerID = first.ID
		loserID = second.ID
	case OutcomeDefeat:
		outcome = OutcomeWin
		winnerID = second.ID
		loserID = first.ID
	default:
		outcome = OutcomeDraw
		winnerID = ""
		loserID = ""
	}

	return Result{
		Outcome:         outcome,
		WinnerID:        winnerID,
		LoserID:         loserID,
		Turns:           partyRes.Turns,
		Reward:          partyRes.TotalReward,
		RemainingHP:     partyRes.RemainingHP,
		RemainingMP:     partyRes.RemainingMP,
		RemainingCMP:    partyRes.RemainingCMP,
		RemainingStatus: partyRes.RemainingStatus,
		ConsumedItems:   partyRes.ConsumedItems,
		Logs:            partyRes.Logs,
	}, nil
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
	for _, sk := range value.Skills {
		if err := ValidateSkillAction(sk); err != nil {
			return err
		}
	}
	for _, cs := range value.CustomSkills {
		if err := ValidateCustomSkillAction(cs); err != nil {
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
