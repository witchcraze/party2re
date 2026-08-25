package battle

import (
	"errors"
	"reflect"
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
	if !reflect.DeepEqual(first, second) || first.Outcome != OutcomeWin || first.WinnerID != "first" ||
		first.LoserID != "second" || first.Turns != 3 || first.Reward.Experience != 20 || len(first.Logs) == 0 {
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

func TestEngineResolvesSecondParticipantWinner(t *testing.T) {
	request := Request{
		Participants: []Participant{
			{ID: "hero", HP: 5, Attack: 1, Defense: 0},
			{ID: "boss", HP: 100, Attack: 50, Defense: 10},
		},
		DefeatReward: Reward{Experience: 0, Currency: 0},
	}
	result, err := (Engine{}).Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWin || result.WinnerID != "boss" || result.LoserID != "hero" || result.Turns != 1 {
		t.Fatalf("Resolve() = %#v, want boss to win on turn 1", result)
	}
}

func TestEngineResolvesMinimumDamage(t *testing.T) {
	request := Request{
		Participants: []Participant{
			{ID: "weak", HP: 3, Attack: 1, Defense: 100},
			{ID: "tank", HP: 2, Attack: 1, Defense: 100},
		},
	}
	result, err := (Engine{}).Resolve(request)
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != OutcomeWin || result.WinnerID != "weak" || result.Turns != 2 {
		t.Fatalf("Resolve() = %#v, want weak to win in 2 turns with minimum damage 1", result)
	}
}

func TestEngineRejectsInvalidRewardFields(t *testing.T) {
	tests := []Reward{
		{Experience: -1},
		{Currency: -1},
		{ItemDefinitionID: "", ItemQuantity: 5},
		{ItemDefinitionID: "potion", ItemQuantity: 0},
		{ItemDefinitionID: "potion", ItemQuantity: -1},
	}
	for _, reward := range tests {
		req := Request{
			Participants: []Participant{
				{ID: "a", HP: 10, Attack: 1, Defense: 0},
				{ID: "b", HP: 10, Attack: 1, Defense: 0},
			},
			VictoryReward: reward,
		}
		if _, err := (Engine{}).Resolve(req); !errors.Is(err, ErrInvalidReward) {
			t.Errorf("Resolve(invalid reward %#v) error = %v, want %v", reward, err, ErrInvalidReward)
		}
	}
}
