package battle

import "errors"

var (
	ErrInvalidRequest     = errors.New("battle request is invalid")
	ErrInvalidParticipant = errors.New("battle participant is invalid")
)

type Participant struct {
	ID      string
	HP      int
	Attack  int
	Defense int
}

type Request struct {
	Participants []Participant
}

type Outcome string

const (
	OutcomeWin  Outcome = "win"
	OutcomeDraw Outcome = "draw"
)

type Result struct {
	Outcome  Outcome
	WinnerID string
	LoserID  string
	Turns    int
}

type Resolver interface {
	Resolve(request Request) (Result, error)
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

	firstHP, secondHP := first.HP, second.HP
	turns := 0
	for firstHP > 0 && secondHP > 0 {
		turns++
		secondHP -= damage(first.Attack, second.Defense)
		if secondHP <= 0 {
			if firstHP-damage(second.Attack, first.Defense) <= 0 {
				return Result{Outcome: OutcomeDraw, Turns: turns}, nil
			}
			return Result{Outcome: OutcomeWin, WinnerID: first.ID, LoserID: second.ID, Turns: turns}, nil
		}
		firstHP -= damage(second.Attack, first.Defense)
	}
	if firstHP <= 0 {
		return Result{Outcome: OutcomeWin, WinnerID: second.ID, LoserID: first.ID, Turns: turns}, nil
	}
	return Result{Outcome: OutcomeWin, WinnerID: first.ID, LoserID: second.ID, Turns: turns}, nil
}

func validateParticipant(value Participant) error {
	if value.ID == "" || value.HP <= 0 || value.Attack < 0 || value.Defense < 0 {
		return ErrInvalidParticipant
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
