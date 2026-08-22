package battle

import (
	"errors"
	"testing"
)

func TestEngineResolvesDeterministicWinner(t *testing.T) {
	request := Request{Participants: []Participant{
		{ID: "first", HP: 10, Attack: 5, Defense: 1},
		{ID: "second", HP: 10, Attack: 2, Defense: 1},
	}, VictoryReward: Reward{Experience: 20}}

	first, err := (Engine{}).Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := (Engine{}).Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if first != second || first.Outcome != OutcomeWin || first.WinnerID != "first" ||
		first.LoserID != "second" || first.Turns != 3 || first.Reward.Experience != 20 {
		t.Fatalf("Resolve() = %#v and %#v", first, second)
	}
}

func TestEngineReturnsDrawWhenBothParticipantsFall(t *testing.T) {
	result, err := (Engine{}).Resolve(Request{Participants: []Participant{
		{ID: "first", HP: 5, Attack: 5, Defense: 0},
		{ID: "second", HP: 5, Attack: 5, Defense: 0},
	}, DrawReward: Reward{Currency: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeDraw || result.WinnerID != "" || result.LoserID != "" ||
		result.Reward.Currency != 1 {
		t.Fatalf("Resolve() = %#v, want draw", result)
	}
}

func TestEngineRejectsInvalidRequests(t *testing.T) {
	tests := []Request{
		{},
		{Participants: []Participant{{ID: "first", HP: 1}}},
		{Participants: []Participant{
			{ID: "same", HP: 1},
			{ID: "same", HP: 1},
		}},
		{Participants: []Participant{
			{ID: "first", HP: 0},
			{ID: "second", HP: 1},
		}},
		{Participants: []Participant{
			{ID: "first", HP: 1},
			{ID: "second", HP: 1},
		}, VictoryReward: Reward{ItemDefinitionID: "potion"}},
	}
	for _, request := range tests {
		if _, err := (Engine{}).Resolve(request); !errors.Is(err, ErrInvalidRequest) &&
			!errors.Is(err, ErrInvalidParticipant) && !errors.Is(err, ErrInvalidReward) {
			t.Errorf("Resolve(%#v) error = %v", request, err)
		}
	}
}
