package battle

import (
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
)

func TestEngine_FirstStrikeDefeatIsWin(t *testing.T) {
	// If first deals lethal damage to second on turn 1, but second's counterattack
	// WOULD have been lethal if second had survived, first must still WIN.
	// The defeated opponent's unexecuted attack must not force a Draw.
	req := Request{
		Participants: []Participant{
			{ID: "first", Name: "First", HP: 10, Attack: 50, Defense: 0, Agility: 20},
			{ID: "second", Name: "Second", HP: 10, Attack: 50, Defense: 0, Agility: 10},
		},
		VictoryReward: Reward{Experience: 100},
		DefeatReward:  Reward{Experience: 10},
		DrawReward:    Reward{Currency: 1},
		RNG:           random.NewDeterministic(42),
	}

	res, err := (Engine{}).Resolve(req)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if res.Outcome != OutcomeWin {
		t.Fatalf("expected OutcomeWin, got %v (draw bug triggered)", res.Outcome)
	}
	if res.WinnerID != "first" {
		t.Fatalf("expected WinnerID first, got %v", res.WinnerID)
	}
	if res.LoserID != "second" {
		t.Fatalf("expected LoserID second, got %v", res.LoserID)
	}
	if res.Turns != 1 {
		t.Fatalf("expected 1 turn, got %d", res.Turns)
	}
	if res.Reward.Experience != 100 {
		t.Fatalf("expected VictoryReward Experience 100, got %d", res.Reward.Experience)
	}
}

func TestEngine_DrawOnTurnLimit(t *testing.T) {
	// When neither participant can defeat the other within 100 turns,
	// the shared battle rules produce a draw.
	req := Request{
		Participants: []Participant{
			{ID: "tank-1", Name: "Tank 1", HP: 10000, Attack: 0, Defense: 1000},
			{ID: "tank-2", Name: "Tank 2", HP: 10000, Attack: 0, Defense: 1000},
		},
		DrawReward: Reward{Currency: 50},
		RNG:        random.NewDeterministic(1),
	}

	res, err := (Engine{}).Resolve(req)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if res.Outcome != OutcomeDraw {
		t.Fatalf("expected OutcomeDraw, got %v", res.Outcome)
	}
	if res.WinnerID != "" || res.LoserID != "" {
		t.Fatalf("expected empty winner/loser for draw, got winner=%q loser=%q", res.WinnerID, res.LoserID)
	}
	if res.Turns != 100 {
		t.Fatalf("expected 100 turns, got %d", res.Turns)
	}
	if res.Reward.Currency != 50 {
		t.Fatalf("expected DrawReward Currency 50, got %d", res.Reward.Currency)
	}
}

func TestEngineResolve_MatchesPartyBattle(t *testing.T) {
	// Compare Engine.Resolve vs ResolvePartyBattle on a 1v1 encounter with skills and MP.
	seed := uint64(12345)
	p1 := Participant{
		ID:      "hero",
		Name:    "Hero",
		HP:      100,
		MaxHP:   100,
		MP:      30,
		MaxMP:   30,
		Attack:  20,
		Defense: 10,
		Agility: 15,
		Skills: []ActionSkill{
			{ID: "skill-mera", Name: "メラ", MPCost: 4, Power: 25, Kind: "attack", TargetScope: "single_enemy", Element: "fire"},
		},
	}
	p2 := Participant{
		ID:      "slime",
		Name:    "Slime",
		HP:      80,
		MaxHP:   80,
		MP:      10,
		MaxMP:   10,
		Attack:  15,
		Defense: 5,
		Agility: 8,
	}

	reward := Reward{Experience: 250, Currency: 120}

	engine := Engine{}

	// 1. Resolve via Engine.Resolve
	resSingle, err := engine.Resolve(Request{
		Participants:  []Participant{p1, p2},
		VictoryReward: reward,
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	// 2. Resolve via ResolvePartyBattle
	resParty, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies:        []Participant{p1},
		Enemies:       []Participant{p2},
		VictoryReward: reward,
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	// 3. Verify parity
	if resSingle.Outcome != resParty.Outcome {
		t.Fatalf("Outcome mismatch: single=%v, party=%v", resSingle.Outcome, resParty.Outcome)
	}
	if resSingle.Turns != resParty.Turns {
		t.Fatalf("Turns mismatch: single=%d, party=%d", resSingle.Turns, resParty.Turns)
	}
	if resSingle.WinnerID != "hero" {
		t.Fatalf("expected WinnerID hero, got %s", resSingle.WinnerID)
	}
	if resSingle.Reward != resParty.TotalReward {
		t.Fatalf("Reward mismatch: single=%+v, party=%+v", resSingle.Reward, resParty.TotalReward)
	}
	if len(resSingle.Logs) != len(resParty.Logs) {
		t.Fatalf("Logs length mismatch: single=%d, party=%d", len(resSingle.Logs), len(resParty.Logs))
	}
	for i := range resSingle.Logs {
		if !reflect.DeepEqual(resSingle.Logs[i], resParty.Logs[i]) {
			t.Fatalf("Log[%d] mismatch:\nsingle: %+v\nparty:  %+v", i, resSingle.Logs[i], resParty.Logs[i])
		}
	}
}

func TestEngineResolve_TeamIDVariants(t *testing.T) {
	testCases := []struct {
		name       string
		firstTeam  string
		secondTeam string
	}{
		{name: "first_enemies_second_empty", firstTeam: "enemies", secondTeam: ""},
		{name: "first_empty_second_allies", firstTeam: "", secondTeam: "allies"},
		{name: "first_allies_second_allies", firstTeam: "allies", secondTeam: "allies"},
		{name: "first_enemies_second_enemies", firstTeam: "enemies", secondTeam: "enemies"},
		{name: "first_same_custom_second_same_custom", firstTeam: "guild-1", secondTeam: "guild-1"},
		{name: "first_custom_second_empty", firstTeam: "team-a", secondTeam: ""},
		{name: "first_empty_second_custom", firstTeam: "", secondTeam: "team-b"},
		{name: "first_custom_second_custom", firstTeam: "team-a", secondTeam: "team-b"},
	}

	engine := Engine{}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := Request{
				Participants: []Participant{
					{ID: "p1", Name: "Player 1", HP: 50, Attack: 30, Defense: 5, Agility: 20, TeamID: tc.firstTeam},
					{ID: "p2", Name: "Player 2", HP: 50, Attack: 10, Defense: 5, Agility: 10, TeamID: tc.secondTeam},
				},
				VictoryReward: Reward{Experience: 100},
				RNG:           random.NewDeterministic(1),
			}
			res, err := engine.Resolve(req)
			if err != nil {
				t.Fatalf("Resolve with teams (%q, %q) failed: %v", tc.firstTeam, tc.secondTeam, err)
			}
			if res.Outcome != OutcomeWin || res.WinnerID != "p1" || res.LoserID != "p2" {
				t.Fatalf("expected p1 to win, got outcome=%v winner=%q loser=%q", res.Outcome, res.WinnerID, res.LoserID)
			}
		})
	}
}

func TestEngineResolve_StatusAilmentParity(t *testing.T) {
	seed := uint64(999)
	p1 := Participant{
		ID:      "attacker",
		Name:    "Attacker",
		HP:      100,
		MaxHP:   100,
		Attack:  30,
		Defense: 10,
		Agility: 20,
	}
	p2 := Participant{
		ID:      "defender",
		Name:    "Defender",
		HP:      40,
		MaxHP:   40,
		Attack:  15,
		Defense: 5,
		Agility: 10,
		Status:  "paralyze", // Has chance to skip turn
	}

	engine := Engine{}
	resSingle, err := engine.Resolve(Request{
		Participants:  []Participant{p1, p2},
		VictoryReward: Reward{Experience: 50},
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	resParty, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies:        []Participant{p1},
		Enemies:       []Participant{p2},
		VictoryReward: Reward{Experience: 50},
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	if resSingle.Outcome != resParty.Outcome {
		t.Fatalf("Outcome mismatch: single=%v, party=%v", resSingle.Outcome, resParty.Outcome)
	}
	if resSingle.Turns != resParty.Turns {
		t.Fatalf("Turns mismatch: single=%d, party=%d", resSingle.Turns, resParty.Turns)
	}
	if resSingle.WinnerID != "attacker" {
		t.Fatalf("expected WinnerID attacker, got %s", resSingle.WinnerID)
	}
	if len(resSingle.Logs) != len(resParty.Logs) {
		t.Fatalf("Logs length mismatch: single=%d, party=%d", len(resSingle.Logs), len(resParty.Logs))
	}
	for i := range resSingle.Logs {
		if !reflect.DeepEqual(resSingle.Logs[i], resParty.Logs[i]) {
			t.Fatalf("Log[%d] mismatch:\nsingle: %+v\nparty:  %+v", i, resSingle.Logs[i], resParty.Logs[i])
		}
	}
}

func TestEngineResolve_RevivalParity(t *testing.T) {
	seed := uint64(777)
	p1 := Participant{
		ID:        "pharaoh-hero",
		Name:      "Pharaoh",
		HP:        15,
		MaxHP:     100,
		MP:        50,
		MaxMP:     100,
		Attack:    50,
		Defense:   0,
		Agility:   10,
		Abilities: []string{"pharaoh"}, // Revives once upon lethal blow
	}
	p2 := Participant{
		ID:      "boss",
		Name:    "Boss",
		HP:      20,
		MaxHP:   20,
		Attack:  25,
		Defense: 0,
		Agility: 20, // Boss strikes first and triggers hero's revival
	}

	engine := Engine{}
	resSingle, err := engine.Resolve(Request{
		Participants:  []Participant{p1, p2},
		VictoryReward: Reward{Experience: 100},
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	resParty, err := engine.ResolvePartyBattle(PartyBattleRequest{
		Allies:        []Participant{p1},
		Enemies:       []Participant{p2},
		VictoryReward: Reward{Experience: 100},
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("ResolvePartyBattle failed: %v", err)
	}

	if resSingle.Outcome != resParty.Outcome {
		t.Fatalf("Outcome mismatch: single=%v, party=%v", resSingle.Outcome, resParty.Outcome)
	}
	if resSingle.WinnerID != "pharaoh-hero" {
		t.Fatalf("expected WinnerID pharaoh-hero after revival, got %s", resSingle.WinnerID)
	}
	if resSingle.Turns != resParty.Turns {
		t.Fatalf("Turns mismatch: single=%d, party=%d", resSingle.Turns, resParty.Turns)
	}
	if len(resSingle.Logs) != len(resParty.Logs) {
		t.Fatalf("Logs length mismatch: single=%d, party=%d", len(resSingle.Logs), len(resParty.Logs))
	}
	for i := range resSingle.Logs {
		if !reflect.DeepEqual(resSingle.Logs[i], resParty.Logs[i]) {
			t.Fatalf("Log[%d] mismatch:\nsingle: %+v\nparty:  %+v", i, resSingle.Logs[i], resParty.Logs[i])
		}
	}
}

func TestEngineResolve_ResultFieldsPopulated(t *testing.T) {
	seed := uint64(54321)
	p1 := Participant{
		ID:      "mage",
		Name:    "Mage",
		HP:      50,
		MaxHP:   50,
		MP:      20,
		MaxMP:   20,
		Attack:  10,
		Defense: 5,
		Agility: 25,
		Skills: []ActionSkill{
			{ID: "skill-fire", Name: "メラ", MPCost: 4, Power: 30, Kind: "attack", TargetScope: "single_enemy"},
		},
	}
	p2 := Participant{
		ID:      "slime",
		Name:    "Slime",
		HP:      20,
		MaxHP:   20,
		Attack:  5,
		Defense: 2,
		Agility: 5,
	}

	engine := Engine{}
	res, err := engine.Resolve(Request{
		Participants:  []Participant{p1, p2},
		VictoryReward: Reward{Experience: 30},
		RNG:           random.NewDeterministic(seed),
	})
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}

	if res.RemainingHP == nil {
		t.Fatalf("expected non-nil RemainingHP on Result")
	}
	if res.RemainingHP["mage"] <= 0 {
		t.Errorf("expected mage RemainingHP > 0, got %d", res.RemainingHP["mage"])
	}
	if res.RemainingMP == nil {
		t.Fatalf("expected non-nil RemainingMP on Result")
	}
	if res.RemainingMP["mage"] != 16 { // 20 - 4 = 16
		t.Errorf("expected mage RemainingMP == 16 after 1 skill use, got %d", res.RemainingMP["mage"])
	}
}
