package gvg_test

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

func TestMedalPromotion(t *testing.T) {
	standing := gvg.GvGStanding{
		BronzeMedals: 26, // 26 Bronze -> 5 Silver + 1 Bronze -> 1 Gold + 0 Silver + 1 Bronze
	}
	standing.PromoteMedals()

	if standing.BronzeMedals != 1 {
		t.Errorf("expected 1 bronze medal, got %d", standing.BronzeMedals)
	}
	if standing.SilverMedals != 0 {
		t.Errorf("expected 0 silver medals, got %d", standing.SilverMedals)
	}
	if standing.GoldMedals != 1 {
		t.Errorf("expected 1 gold medal, got %d", standing.GoldMedals)
	}

	// Test up to Champion Cup
	standing2 := gvg.GvGStanding{
		BronzeMedals: 5 * 5 * 5 * 5 * 5 * 2, // 2 Champion Cups
	}
	standing2.PromoteMedals()
	if standing2.ChampionCups != 2 {
		t.Errorf("expected 2 champion cups, got %d", standing2.ChampionCups)
	}
}

func TestCalculateEloDelta(t *testing.T) {
	tests := []struct {
		name           string
		cRating        int
		dRating        int
		cScore         int
		dScore         int
		wantCDeltaSign int // 1 for positive, -1 for negative, 0 for zero
		wantDDeltaSign int
	}{
		{
			name:           "Equal ratings challenger wins",
			cRating:        1000,
			dRating:        1000,
			cScore:         3,
			dScore:         1,
			wantCDeltaSign: 1,
			wantDDeltaSign: -1,
		},
		{
			name:           "Equal ratings defender wins",
			cRating:        1000,
			dRating:        1000,
			cScore:         1,
			dScore:         3,
			wantCDeltaSign: -1,
			wantDDeltaSign: 1,
		},
		{
			name:           "Equal ratings draw",
			cRating:        1000,
			dRating:        1000,
			cScore:         2,
			dScore:         2,
			wantCDeltaSign: 0,
			wantDDeltaSign: 0,
		},
		{
			name:           "Much higher rated challenger wins (minimum +1)",
			cRating:        3000,
			dRating:        500,
			cScore:         3,
			dScore:         0,
			wantCDeltaSign: 1,
			wantDDeltaSign: -1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cDelta, dDelta := gvg.CalculateEloDelta(tt.cRating, tt.dRating, tt.cScore, tt.dScore)
			if tt.wantCDeltaSign > 0 && cDelta <= 0 {
				t.Errorf("expected positive cDelta, got %d", cDelta)
			}
			if tt.wantCDeltaSign < 0 && cDelta >= 0 {
				t.Errorf("expected negative cDelta, got %d", cDelta)
			}
			if tt.wantCDeltaSign == 0 && cDelta != 0 {
				t.Errorf("expected zero cDelta, got %d", cDelta)
			}
			if cDelta+dDelta != 0 {
				t.Errorf("expected cDelta + dDelta = 0, got %d + %d = %d", cDelta, dDelta, cDelta+dDelta)
			}
		})
	}
}

func TestCalculateGuildRewards(t *testing.T) {
	cExp, dExp, cVP, dVP, cMedal, dMedal := gvg.CalculateGuildRewards(3, 1)
	if cExp != 100 || dExp != 20 || cVP != 10 || dVP != 1 || !cMedal || dMedal {
		t.Errorf("unexpected challenger win rewards: cExp=%d, dExp=%d, cVP=%d, dVP=%d, cMedal=%v, dMedal=%v",
			cExp, dExp, cVP, dVP, cMedal, dMedal)
	}

	cExp, dExp, cVP, dVP, cMedal, dMedal = gvg.CalculateGuildRewards(1, 3)
	if cExp != 20 || dExp != 100 || cVP != 1 || dVP != 10 || cMedal || !dMedal {
		t.Errorf("unexpected defender win rewards: cExp=%d, dExp=%d, cVP=%d, dVP=%d, cMedal=%v, dMedal=%v",
			cExp, dExp, cVP, dVP, cMedal, dMedal)
	}

	cExp, dExp, cVP, dVP, cMedal, dMedal = gvg.CalculateGuildRewards(2, 2)
	if cExp != 50 || dExp != 50 || cVP != 3 || dVP != 3 || cMedal || dMedal {
		t.Errorf("unexpected draw rewards: cExp=%d, dExp=%d, cVP=%d, dVP=%d, cMedal=%v, dMedal=%v",
			cExp, dExp, cVP, dVP, cMedal, dMedal)
	}
}

// Mock repositories for service testing
type mockGvGRepo struct {
	standings map[string]gvg.GvGStanding
	matches   map[string]gvg.MatchRecord
}

func newMockGvGRepo() *mockGvGRepo {
	return &mockGvGRepo{
		standings: make(map[string]gvg.GvGStanding),
		matches:   make(map[string]gvg.MatchRecord),
	}
}

func (m *mockGvGRepo) GetOrCreateStanding(ctx context.Context, guildID string) (gvg.GvGStanding, error) {
	st, ok := m.standings[guildID]
	if !ok {
		st = gvg.GvGStanding{
			GuildID:   guildID,
			Rating:    gvg.DefaultRating,
			UpdatedAt: time.Now(),
		}
		m.standings[guildID] = st
	}
	return st, nil
}

func (m *mockGvGRepo) FindOpponentGuilds(ctx context.Context, challengerGuildID string, limit int) ([]gvg.GuildCandidate, error) {
	return []gvg.GuildCandidate{
		{GuildID: "g_defender", GuildName: "Defender Guild", Rating: 1000},
	}, nil
}

func (m *mockGvGRepo) GetLeaderboard(ctx context.Context, limit int) ([]gvg.GvGStanding, error) {
	var list []gvg.GvGStanding
	for _, st := range m.standings {
		list = append(list, st)
	}
	return list, nil
}

func (m *mockGvGRepo) GetMatchHistory(ctx context.Context, guildID string, limit int) ([]gvg.MatchRecord, error) {
	var list []gvg.MatchRecord
	for _, match := range m.matches {
		if match.ChallengerGuildID == guildID || match.DefenderGuildID == guildID {
			list = append(list, match)
		}
	}
	return list, nil
}

func (m *mockGvGRepo) GetMatchDetail(ctx context.Context, matchID string) (gvg.MatchRecord, error) {
	match, ok := m.matches[matchID]
	if !ok {
		return gvg.MatchRecord{}, gvg.ErrMatchNotFound
	}
	return match, nil
}

func (m *mockGvGRepo) RecordMatchAndUpdateStandings(
	ctx context.Context,
	match gvg.MatchRecord,
	challengerDelta, defenderDelta int,
	challengerExp, defenderExp int64,
	challengerVP, defenderVP int64,
	challengerMedal, defenderMedal bool,
	memberRewards map[string]gvg.MemberReward,
) error {
	m.matches[match.ID] = match

	cSt, _ := m.GetOrCreateStanding(ctx, match.ChallengerGuildID)
	cSt.Rating += challengerDelta
	cSt.VictoryPoints += challengerVP
	if challengerMedal {
		cSt.BronzeMedals++
		cSt.Wins++
	} else if defenderMedal {
		cSt.Losses++
	} else {
		cSt.Draws++
	}
	cSt.PromoteMedals()
	m.standings[match.ChallengerGuildID] = cSt

	dSt, _ := m.GetOrCreateStanding(ctx, match.DefenderGuildID)
	dSt.Rating += defenderDelta
	dSt.VictoryPoints += defenderVP
	if defenderMedal {
		dSt.BronzeMedals++
		dSt.Wins++
	} else if challengerMedal {
		dSt.Losses++
	} else {
		dSt.Draws++
	}
	dSt.PromoteMedals()
	m.standings[match.DefenderGuildID] = dSt

	return nil
}

type mockGuildRepo struct {
	guilds map[string]guild.Detail
}

func (m *mockGuildRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	d, ok := m.guilds[guildID]
	if !ok {
		return guild.Guild{}, nil, guild.ErrGuildNotFound
	}
	return d.Guild, d.Members, nil
}

func (m *mockGuildRepo) GetGuildByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error) {
	for _, d := range m.guilds {
		for _, mem := range d.Members {
			if mem.CharacterID == characterID {
				return d.Guild, mem, nil
			}
		}
	}
	return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
}

type mockCharRepo struct {
	chars map[string]corecharacter.Character
}

func (m *mockCharRepo) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.chars[id]
	if !ok {
		return corecharacter.Character{}, gvg.ErrCharacterNotFound
	}
	return c, nil
}

type mockBattleEngine struct{}

func (m *mockBattleEngine) Resolve(req corebattle.Request) (corebattle.Result, error) {
	winnerID := req.Participants[0].ID
	loserID := req.Participants[1].ID
	if req.Participants[1].Attack > req.Participants[0].Attack {
		winnerID = req.Participants[1].ID
		loserID = req.Participants[0].ID
	}
	return corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: winnerID,
		LoserID:  loserID,
		Turns:    5,
	}, nil
}

func TestServiceDeclareMatch(t *testing.T) {
	ctx := context.Background()

	gvgRepo := newMockGvGRepo()
	guildRepo := &mockGuildRepo{
		guilds: map[string]guild.Detail{
			"guild_a": {
				Guild: guild.Guild{ID: "guild_a", Name: "Alpha Guild", Level: 1},
				Members: []guild.Member{
					{GuildID: "guild_a", CharacterID: "char_leader_a", Role: guild.RoleLeader},
					{GuildID: "guild_a", CharacterID: "char_mem_a2", Role: guild.RoleMember},
				},
			},
			"guild_b": {
				Guild: guild.Guild{ID: "guild_b", Name: "Beta Guild", Level: 1},
				Members: []guild.Member{
					{GuildID: "guild_b", CharacterID: "char_leader_b", Role: guild.RoleLeader},
					{GuildID: "guild_b", CharacterID: "char_mem_b2", Role: guild.RoleMember},
				},
			},
		},
	}

	charRepo := &mockCharRepo{
		chars: map[string]corecharacter.Character{
			"char_leader_a": {ID: "char_leader_a", Name: "Leader A", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100, Attack: 50, Defense: 30}},
			"char_mem_a2":   {ID: "char_mem_a2", Name: "Member A2", Level: 8, Stats: corecharacter.Stats{HP: 80, MaxHP: 80, Attack: 40, Defense: 20}},
			"char_leader_b": {ID: "char_leader_b", Name: "Leader B", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 20, Defense: 10}},
			"char_mem_b2":   {ID: "char_mem_b2", Name: "Member B2", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50, Attack: 20, Defense: 10}},
		},
	}

	battleEngine := &mockBattleEngine{}

	svc, err := gvg.NewService(gvgRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatalf("failed to create gvg service: %v", err)
	}

	// 1. Validation: Member cannot declare match
	_, err = svc.DeclareMatch(ctx, "char_mem_a2", "guild_b")
	if !errors.Is(err, gvg.ErrUnauthorized) {
		t.Errorf("expected ErrUnauthorized for regular member, got %v", err)
	}

	// 2. Validation: Cannot challenge own guild
	_, err = svc.DeclareMatch(ctx, "char_leader_a", "guild_a")
	if !errors.Is(err, gvg.ErrCannotChallengeOwnGuild) {
		t.Errorf("expected ErrCannotChallengeOwnGuild, got %v", err)
	}

	// 3. Success declaration by leader
	res, err := svc.DeclareMatch(ctx, "char_leader_a", "guild_b")
	if err != nil {
		t.Fatalf("DeclareMatch failed: %v", err)
	}

	if res.Match.WinnerGuildID != "guild_a" {
		t.Errorf("expected winner guild_a, got %s", res.Match.WinnerGuildID)
	}
	if res.Match.ChallengerScore != 2 || res.Match.DefenderScore != 0 {
		t.Errorf("expected score 2-0, got %d-%d", res.Match.ChallengerScore, res.Match.DefenderScore)
	}
	if res.ChallengerRatingDelta <= 0 || res.DefenderRatingDelta >= 0 {
		t.Errorf("unexpected rating deltas: cDelta=%d, dDelta=%d", res.ChallengerRatingDelta, res.DefenderRatingDelta)
	}
	if res.ChallengerGuildExp != 100 || res.ChallengerVictoryPoints != 10 || !res.ChallengerMedalAwarded {
		t.Errorf("unexpected challenger guild rewards: exp=%d, vp=%d, medal=%v",
			res.ChallengerGuildExp, res.ChallengerVictoryPoints, res.ChallengerMedalAwarded)
	}

	// 4. Query standing & match history
	stA, err := svc.GetStanding(ctx, "guild_a")
	if err != nil || stA.Wins != 1 || stA.BronzeMedals != 1 {
		t.Errorf("unexpected guild A standing: %#v, err=%v", stA, err)
	}

	history, err := svc.GetMatchHistory(ctx, "guild_a", 10)
	if err != nil || len(history) != 1 {
		t.Errorf("expected 1 match in history, got %d, err=%v", len(history), err)
	}

	detail, err := svc.GetMatchDetail(ctx, res.Match.ID)
	if err != nil || detail.ID != res.Match.ID {
		t.Errorf("unexpected match detail: %#v, err=%v", detail, err)
	}
}
