package battle

import (
	"errors"
	"reflect"
	"strings"
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

func TestEngineResolvePartyBattle(t *testing.T) {
	engine := Engine{}

	// 1. Successful 2-ally victory with 10% bonus
	res, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies: []Participant{
			{ID: "hero1", HP: 50, Attack: 20, Defense: 5},
			{ID: "hero2", HP: 40, Attack: 15, Defense: 5},
		},
		Enemies: []Participant{
			{ID: "goblin", HP: 30, Attack: 10, Defense: 2},
		},
		VictoryReward: Reward{Experience: 100, Currency: 50},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}
	if res.Outcome != OutcomeWin || res.WinnerSide != "allies" {
		t.Fatalf("expected allies win, got outcome=%s side=%s", res.Outcome, res.WinnerSide)
	}
	if res.BonusPercent != 10 {
		t.Fatalf("expected 10%% bonus for 2 allies, got %d%%", res.BonusPercent)
	}
	if res.TotalReward.Experience != 110 || res.TotalReward.Currency != 55 {
		t.Fatalf("expected scaled reward (110 EXP, 55 G), got %+v", res.TotalReward)
	}
	if len(res.AlliesSurvived) != 2 || len(res.Logs) == 0 {
		t.Fatalf("expected 2 survivors and turn logs, got %+v", res)
	}

	// 2. 4-ally party with 30% bonus
	res4, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies: []Participant{
			{ID: "p1", HP: 50, Attack: 20, Defense: 5},
			{ID: "p2", HP: 40, Attack: 15, Defense: 5},
			{ID: "p3", HP: 45, Attack: 18, Defense: 5},
			{ID: "p4", HP: 60, Attack: 22, Defense: 5},
		},
		Enemies: []Participant{
			{ID: "dragon", HP: 100, Attack: 15, Defense: 5},
		},
		VictoryReward: Reward{Experience: 200, Currency: 100},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle 4-player failed: %v", err)
	}
	if res4.BonusPercent != 30 || res4.TotalReward.Experience != 260 {
		t.Fatalf("expected 30%% bonus (260 EXP), got %+v", res4)
	}

	// 3. Allies defeat
	resDefeat, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies: []Participant{
			{ID: "weak1", HP: 5, Attack: 1, Defense: 0},
		},
		Enemies: []Participant{
			{ID: "mega_boss", HP: 1000, Attack: 100, Defense: 50},
		},
		DefeatReward: Reward{Experience: 10},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle defeat failed: %v", err)
	}
	if resDefeat.Outcome != OutcomeDefeat || resDefeat.WinnerSide != "enemies" {
		t.Fatalf("expected enemies win, got outcome=%s side=%s", resDefeat.Outcome, resDefeat.WinnerSide)
	}

	// 4. Duplicate display names with distinct IDs
	resSameName, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies: []Participant{
			{ID: "char-1", Name: "TwinHero", HP: 50, Attack: 25, Defense: 5},
			{ID: "char-2", Name: "TwinHero", HP: 40, Attack: 20, Defense: 5},
		},
		Enemies: []Participant{
			{ID: "mon-1", Name: "Slime", HP: 20, Attack: 5, Defense: 2},
		},
		VictoryReward: Reward{Experience: 50, Currency: 25},
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle with duplicate names failed: %v", err)
	}
	if resSameName.Outcome != OutcomeWin {
		t.Fatalf("expected win, got %s", resSameName.Outcome)
	}
	if len(resSameName.AlliesSurvived) != 2 {
		t.Fatalf("expected 2 survivors, got %d", len(resSameName.AlliesSurvived))
	}
	if resSameName.RemainingHP["char-1"] <= 0 || resSameName.RemainingHP["char-2"] <= 0 {
		t.Fatalf("expected positive remaining HP for both distinct IDs, got %+v", resSameName.RemainingHP)
	}
	if len(resSameName.Logs) == 0 || !strings.Contains(resSameName.Logs[0].Message, "TwinHero") {
		t.Fatalf("expected log message to contain display name TwinHero, got %+v", resSameName.Logs)
	}

	// 5. Invalid requests
	if _, err := engine.ResolvePartyBattle(PartyBattleRequest{}); !errors.Is(err, ErrInvalidRequest) {
		t.Fatalf("expected ErrInvalidRequest, got %v", err)
	}
}
