package pvp_test

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

func TestCalculateEloDelta(t *testing.T) {
	tests := []struct {
		name           string
		attackerRating int
		defenderRating int
		outcome        pvp.MatchOutcome
		wantAttacker   int
		wantDefender   int
	}{
		{
			name:           "Equal rating win",
			attackerRating: 1000,
			defenderRating: 1000,
			outcome:        pvp.OutcomeWin,
			wantAttacker:   16,
			wantDefender:   -16,
		},
		{
			name:           "Equal rating loss",
			attackerRating: 1000,
			defenderRating: 1000,
			outcome:        pvp.OutcomeLoss,
			wantAttacker:   -16,
			wantDefender:   16,
		},
		{
			name:           "Equal rating draw",
			attackerRating: 1000,
			defenderRating: 1000,
			outcome:        pvp.OutcomeDraw,
			wantAttacker:   0,
			wantDefender:   0,
		},
		{
			name:           "Higher rating beating lower rating gets small positive delta",
			attackerRating: 1400,
			defenderRating: 1000,
			outcome:        pvp.OutcomeWin,
			wantAttacker:   3,
			wantDefender:   -3,
		},
		{
			name:           "Lower rating beating higher rating gets large positive delta",
			attackerRating: 1000,
			defenderRating: 1400,
			outcome:        pvp.OutcomeWin,
			wantAttacker:   29,
			wantDefender:   -29,
		},
		{
			name:           "Win always guarantees at least +1 delta",
			attackerRating: 2500,
			defenderRating: 500,
			outcome:        pvp.OutcomeWin,
			wantAttacker:   1,
			wantDefender:   -1,
		},
		{
			name:           "Loss always guarantees at least -1 delta",
			attackerRating: 500,
			defenderRating: 2500,
			outcome:        pvp.OutcomeLoss,
			wantAttacker:   -1,
			wantDefender:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotA, gotD := pvp.CalculateEloDelta(tt.attackerRating, tt.defenderRating, tt.outcome)
			if gotA != tt.wantAttacker || gotD != tt.wantDefender {
				t.Errorf("CalculateEloDelta(%d, %d, %s) = (%d, %d), want (%d, %d)",
					tt.attackerRating, tt.defenderRating, tt.outcome, gotA, gotD, tt.wantAttacker, tt.wantDefender)
			}
		})
	}
}

func TestCalculateRewards(t *testing.T) {
	defender := corecharacter.Character{
		ID:    "def-1",
		Level: 10,
	}

	// Win
	exp, gold := pvp.CalculateRewards(defender, pvp.OutcomeWin)
	if exp != 150 || gold != 300 {
		t.Errorf("CalculateRewards win = (%d, %d), want (150, 300)", exp, gold)
	}

	// Loss
	exp, gold = pvp.CalculateRewards(defender, pvp.OutcomeLoss)
	if exp != 10 || gold != 0 {
		t.Errorf("CalculateRewards loss = (%d, %d), want (10, 0)", exp, gold)
	}

	// Draw
	exp, gold = pvp.CalculateRewards(defender, pvp.OutcomeDraw)
	if exp != 25 || gold != 50 {
		t.Errorf("CalculateRewards draw = (%d, %d), want (25, 50)", exp, gold)
	}
}

type mockCharacterRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharacterRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

type mockPvPRepo struct {
	ratings     map[string]pvp.ArenaRating
	matches     []pvp.MatchRecord
	opponents   []pvp.OpponentCandidate
	leaderboard []pvp.OpponentCandidate
}

func (m *mockPvPRepo) GetOrCreateRating(ctx context.Context, characterID string) (pvp.ArenaRating, error) {
	r, ok := m.ratings[characterID]
	if !ok {
		r = pvp.ArenaRating{
			CharacterID: characterID,
			Rating:      pvp.DefaultRating,
			Wins:        0,
			Losses:      0,
			Draws:       0,
		}
		m.ratings[characterID] = r
	}
	return r, nil
}

func (m *mockPvPRepo) RecordMatchAndUpdateRatings(ctx context.Context, match pvp.MatchRecord, attackerRating, defenderRating pvp.ArenaRating, attacker corecharacter.Character) error {
	m.ratings[attackerRating.CharacterID] = attackerRating
	m.ratings[defenderRating.CharacterID] = defenderRating
	m.matches = append(m.matches, match)
	return nil
}

func (m *mockPvPRepo) FindOpponents(ctx context.Context, characterID string, limit int) ([]pvp.OpponentCandidate, error) {
	return m.opponents, nil
}

func (m *mockPvPRepo) GetMatchHistory(ctx context.Context, characterID string, limit int) ([]pvp.MatchRecord, error) {
	var result []pvp.MatchRecord
	for _, match := range m.matches {
		if match.AttackerID == characterID || match.DefenderID == characterID {
			result = append(result, match)
		}
	}
	return result, nil
}

func (m *mockPvPRepo) GetDefenseLogs(ctx context.Context, characterID string, limit int) ([]pvp.MatchRecord, error) {
	var result []pvp.MatchRecord
	for _, match := range m.matches {
		if match.DefenderID == characterID {
			result = append(result, match)
		}
	}
	return result, nil
}

func (m *mockPvPRepo) GetLeaderboard(ctx context.Context, limit int) ([]pvp.OpponentCandidate, error) {
	return m.leaderboard, nil
}

type fixedBattleResolver struct {
	result corebattle.Result
}

func (f fixedBattleResolver) Resolve(request corebattle.Request) (corebattle.Result, error) {
	return f.result, nil
}

func TestServiceChallenge(t *testing.T) {
	ctx := context.Background()

	attacker := corecharacter.Character{
		ID:       "att-1",
		PlayerID: "player-1",
		Name:     "Attacker",
		Level:    5,
		Money:    100,
		Stats: corecharacter.Stats{
			HP:      50,
			MaxHP:   50,
			Attack:  15,
			Defense: 10,
		},
	}
	defender := corecharacter.Character{
		ID:       "def-1",
		PlayerID: "player-2",
		Name:     "Defender",
		Level:    5,
		Money:    100,
		Stats: corecharacter.Stats{
			HP:      50,
			MaxHP:   50,
			Attack:  12,
			Defense: 8,
		},
	}

	charRepo := &mockCharacterRepo{
		chars: map[string]corecharacter.Character{
			attacker.ID: attacker,
			defender.ID: defender,
		},
	}
	pvpRepo := &mockPvPRepo{
		ratings: make(map[string]pvp.ArenaRating),
	}

	battleResolver := fixedBattleResolver{
		result: corebattle.Result{
			Outcome:  corebattle.OutcomeWin,
			WinnerID: attacker.ID,
			LoserID:  defender.ID,
			Turns:    3,
		},
	}

	service, err := pvp.NewService(pvpRepo, charRepo, battleResolver)
	if err != nil {
		t.Fatalf("NewService error = %v", err)
	}

	// 1. Cannot challenge self
	_, err = service.Challenge(ctx, attacker.ID, attacker.ID)
	if !errors.Is(err, pvp.ErrCannotChallengeSelf) {
		t.Errorf("expected ErrCannotChallengeSelf, got %v", err)
	}

	// 2. Cannot challenge if attacker has 0 HP
	charRepo.chars[attacker.ID] = corecharacter.Character{
		ID:       attacker.ID,
		PlayerID: attacker.PlayerID,
		Stats: corecharacter.Stats{
			HP: 0,
		},
	}
	_, err = service.Challenge(ctx, attacker.ID, defender.ID)
	if !errors.Is(err, pvp.ErrCharacterDefeated) {
		t.Errorf("expected ErrCharacterDefeated, got %v", err)
	}
	charRepo.chars[attacker.ID] = attacker

	// 3. Successful challenge
	res, err := service.Challenge(ctx, attacker.ID, defender.ID)
	if err != nil {
		t.Fatalf("Challenge error = %v", err)
	}

	if res.Match.Outcome != pvp.OutcomeWin {
		t.Errorf("match outcome = %v, want %v", res.Match.Outcome, pvp.OutcomeWin)
	}
	if res.Match.AttackerRatingBefore != 1000 || res.Match.AttackerRatingAfter != 1016 {
		t.Errorf("attacker rating before=%d, after=%d; want 1000 -> 1016", res.Match.AttackerRatingBefore, res.Match.AttackerRatingAfter)
	}
	if res.Match.DefenderRatingBefore != 1000 || res.Match.DefenderRatingAfter != 984 {
		t.Errorf("defender rating before=%d, after=%d; want 1000 -> 984", res.Match.DefenderRatingBefore, res.Match.DefenderRatingAfter)
	}
	if res.Match.RewardExp != 100 || res.Match.RewardGold != 200 {
		t.Errorf("match rewards exp=%d, gold=%d; want 100, 200", res.Match.RewardExp, res.Match.RewardGold)
	}

	// Verify ratings in repo
	attRating, err := service.GetRating(ctx, attacker.ID)
	if err != nil || attRating.Rating != 1016 || attRating.Wins != 1 {
		t.Errorf("unexpected attacker rating in repo: %#v", attRating)
	}
	defRating, err := service.GetRating(ctx, defender.ID)
	if err != nil || defRating.Rating != 984 || defRating.Losses != 1 {
		t.Errorf("unexpected defender rating in repo: %#v", defRating)
	}

	// Verify match history
	hist, err := service.GetMatchHistory(ctx, attacker.ID, 10)
	if err != nil || len(hist) != 1 {
		t.Fatalf("unexpected match history: %#v", hist)
	}

	// Verify defense logs for defender
	defLogs, err := service.GetDefenseLogs(ctx, defender.ID, 10)
	if err != nil || len(defLogs) != 1 {
		t.Fatalf("unexpected defense logs: %#v", defLogs)
	}
}

func TestServiceFindOpponentsAndLeaderboard(t *testing.T) {
	ctx := context.Background()
	charRepo := &mockCharacterRepo{}
	pvpRepo := &mockPvPRepo{
		opponents: []pvp.OpponentCandidate{
			{CharacterID: "opp-1", Name: "Warrior", Level: 5, Rating: 1050},
		},
		leaderboard: []pvp.OpponentCandidate{
			{CharacterID: "top-1", Name: "Hero", Level: 50, Rating: 2100},
		},
	}

	service, err := pvp.NewService(pvpRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	opps, err := service.FindOpponents(ctx, "char-1", 5)
	if err != nil || len(opps) != 1 || opps[0].CharacterID != "opp-1" {
		t.Errorf("FindOpponents unexpected: %#v", opps)
	}

	leaders, err := service.GetLeaderboard(ctx, 10)
	if err != nil || len(leaders) != 1 || leaders[0].CharacterID != "top-1" {
		t.Errorf("GetLeaderboard unexpected: %#v", leaders)
	}
}

func TestService_VictoryHook(t *testing.T) {
	ctx := context.Background()
	attacker := corecharacter.Character{
		ID:       "att-hero",
		PlayerID: "player-1",
		Level:    5,
		Stats:    corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 50, Defense: 20, Agility: 30},
	}
	defender := corecharacter.Character{
		ID:       "def-victim",
		PlayerID: "player-2",
		Level:    5,
		Stats:    corecharacter.Stats{HP: 10, MaxHP: 10, Attack: 5, Defense: 5, Agility: 5},
	}
	charRepo := &mockCharacterRepo{
		chars: map[string]corecharacter.Character{
			attacker.ID: attacker,
			defender.ID: defender,
		},
	}
	pvpRepo := &mockPvPRepo{
		ratings: make(map[string]pvp.ArenaRating),
	}
	service, err := pvp.NewService(pvpRepo, charRepo, corebattle.Engine{})
	if err != nil {
		t.Fatal(err)
	}

	var hookedWinnerID, hookedLoserID string
	service.SetVictoryHook(func(ctx context.Context, winnerID, loserID string) error {
		hookedWinnerID = winnerID
		hookedLoserID = loserID
		return nil
	})

	res, err := service.Challenge(ctx, attacker.ID, defender.ID)
	if err != nil {
		t.Fatalf("Challenge failed: %v", err)
	}

	if res.Match.Outcome != pvp.OutcomeWin {
		t.Fatalf("expected win, got %v", res.Match.Outcome)
	}
	if hookedWinnerID != attacker.ID {
		t.Errorf("expected hookedWinnerID %s, got %s", attacker.ID, hookedWinnerID)
	}
	if hookedLoserID != defender.ID {
		t.Errorf("expected hookedLoserID %s, got %s", defender.ID, hookedLoserID)
	}
}
