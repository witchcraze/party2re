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

// mockStandingRepo implements gvg.StandingRepository for tests.
type mockStandingRepo struct {
	standings   map[string]gvg.GvGStanding
	settlements []gvg.MatchSettlement
	roundWins   map[string]int
}

func newMockStandingRepo() *mockStandingRepo {
	return &mockStandingRepo{
		standings: make(map[string]gvg.GvGStanding),
		roundWins: make(map[string]int),
	}
}

func (m *mockStandingRepo) GetOrCreateStanding(_ context.Context, guildID string) (gvg.GvGStanding, error) {
	if st, ok := m.standings[guildID]; ok {
		return st, nil
	}
	st := gvg.GvGStanding{
		GuildID:   guildID,
		UpdatedAt: time.Now(),
	}
	m.standings[guildID] = st
	return st, nil
}

func (m *mockStandingRepo) GetLeaderboard(_ context.Context, limit int) ([]gvg.GvGStanding, error) {
	var list []gvg.GvGStanding
	for _, st := range m.standings {
		list = append(list, st)
	}
	if len(list) > limit {
		list = list[:limit]
	}
	return list, nil
}

func (m *mockStandingRepo) AddRoundWinGP(_ context.Context, guildID string, gp int) error {
	st, _ := m.GetOrCreateStanding(context.Background(), guildID)
	st.VictoryPoints += int64(gp)
	m.standings[guildID] = st
	m.roundWins[guildID] += gp
	return nil
}

func (m *mockStandingRepo) RecordMatchSettlement(_ context.Context, settlement gvg.MatchSettlement) error {
	m.settlements = append(m.settlements, settlement)
	for _, gID := range settlement.GuildIDs {
		st, _ := m.GetOrCreateStanding(context.Background(), gID)
		extraGP := settlement.ParticipantGP[gID]
		if settlement.IsDraw {
			st.Draws++
			st.VictoryPoints += int64(extraGP)
		} else if gID == settlement.WinnerGuildID {
			st.Wins++
			st.VictoryPoints += int64(settlement.WinnerPrizeGP + extraGP)
			st.BronzeMedals++
			st.PromoteMedals()
		} else {
			st.Losses++
			st.VictoryPoints += int64(extraGP)
		}
		m.standings[gID] = st
	}
	return nil
}

// mockGuildRepo implements gvg.GuildRepository for tests.
type mockGuildRepo struct {
	guilds     map[string]guild.Guild
	charGuilds map[string]string
	members    map[string][]guild.Member
}

func newMockGuildRepo() *mockGuildRepo {
	return &mockGuildRepo{
		guilds:     make(map[string]guild.Guild),
		charGuilds: make(map[string]string),
		members:    make(map[string][]guild.Member),
	}
}

func (m *mockGuildRepo) GetGuild(_ context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, ok := m.guilds[guildID]
	if !ok {
		return guild.Guild{}, nil, guild.ErrGuildNotFound
	}
	return g, m.members[guildID], nil
}

func (m *mockGuildRepo) GetGuildByCharacter(_ context.Context, characterID string) (guild.Guild, guild.Member, error) {
	gID, ok := m.charGuilds[characterID]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
	}
	g, ok := m.guilds[gID]
	if !ok {
		return guild.Guild{}, guild.Member{}, guild.ErrGuildNotFound
	}
	return g, guild.Member{GuildID: gID, CharacterID: characterID, Role: guild.RoleMember}, nil
}

// mockCharRepo implements gvg.CharacterRepository for tests.
type mockCharRepo struct {
	characters map[string]corecharacter.Character
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{characters: make(map[string]corecharacter.Character)}
}

func (m *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, gvg.ErrCharacterNotFound
	}
	return c, nil
}

func (m *mockCharRepo) Update(_ context.Context, character corecharacter.Character) error {
	m.characters[character.ID] = character
	return nil
}

// mockBattleEngine implements gvg.BattleEngine for tests.
type mockBattleEngine struct {
	outcome corebattle.Outcome
}

func (m *mockBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return corebattle.PartyBattleResult{
		Outcome: m.outcome,
		Turns:   3,
	}, nil
}

func createTestCharacter(id, name string, hp int, tired int) corecharacter.Character {
	return corecharacter.Character{
		ID:    id,
		Name:  name,
		JobID: "warrior",
		Level: 10,
		Stats: corecharacter.Stats{
			HP:    hp,
			MaxHP: 100,
		},
		Tired: tired,
	}
}

func TestTrophyMedalPromotion(t *testing.T) {
	// Test 7-tier cascading promotion:
	// Bronze (5) -> Silver (5) -> Gold (5) -> Order (5) -> Trophy (5) -> Championship Cup (5) -> Champion Cup
	st := gvg.GvGStanding{
		BronzeMedals: 26, // 26 -> 1 Bronze + 5 Silver -> 1 Bronze + 0 Silver + 1 Gold
	}
	st.PromoteMedals()
	if st.BronzeMedals != 1 || st.SilverMedals != 0 || st.GoldMedals != 1 {
		t.Fatalf("expected 1 bronze, 0 silver, 1 gold, got B=%d S=%d G=%d", st.BronzeMedals, st.SilverMedals, st.GoldMedals)
	}

	// 5^6 = 15625 Bronze Medals = 1 Champion Cup
	st2 := gvg.GvGStanding{
		BronzeMedals: 15625 * 2,
	}
	st2.PromoteMedals()
	if st2.ChampionCups != 2 || st2.BronzeMedals != 0 {
		t.Fatalf("expected 2 champion cups, got %d (bronze=%d)", st2.ChampionCups, st2.BronzeMedals)
	}

	// Test Orders tier promotion
	st3 := gvg.GvGStanding{
		Orders: 5,
	}
	st3.PromoteMedals()
	if st3.Orders != 0 || st3.Trophies != 1 {
		t.Fatalf("expected 0 orders, 1 trophy, got O=%d T=%d", st3.Orders, st3.Trophies)
	}
}

func TestCreateRoom(t *testing.T) {
	ctx := context.Background()
	roomRepo := gvg.NewMemoryRoomRepository()
	standingRepo := newMockStandingRepo()
	guildRepo := newMockGuildRepo()
	charRepo := newMockCharRepo()
	battleEngine := &mockBattleEngine{outcome: corebattle.OutcomeWin}

	svc, err := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)
	if err != nil {
		t.Fatal(err)
	}

	// Register characters
	charRepo.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	charRepo.characters["c_dead"] = createTestCharacter("c_dead", "Dead", 0, 0)
	charRepo.characters["c_tired"] = createTestCharacter("c_tired", "Tired", 100, 100)
	charRepo.characters["c_friendly"] = createTestCharacter("c_friendly", "Friendly", 100, 0)

	// Register guilds
	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guildRepo.charGuilds["c1"] = "g1"

	guildRepo.guilds["g_white"] = guild.Guild{ID: "g_white", Name: "WhiteGuild", Color: "#FFFFFF"}
	guildRepo.charGuilds["c_friendly"] = "g_white"

	t.Run("fails if character not in guild", func(t *testing.T) {
		charRepo.characters["c_no_guild"] = createTestCharacter("c_no_guild", "NoGuild", 100, 0)
		_, err := svc.CreateRoom(ctx, "c_no_guild", gvg.CreateRoomRequest{Name: "GvG Arena"})
		if !errors.Is(err, gvg.ErrActorNotInGuild) {
			t.Fatalf("expected ErrActorNotInGuild, got %v", err)
		}
	})

	t.Run("fails if guild has friendly color #FFFFFF", func(t *testing.T) {
		_, err := svc.CreateRoom(ctx, "c_friendly", gvg.CreateRoomRequest{Name: "GvG Arena"})
		if !errors.Is(err, gvg.ErrFriendlyGuildCannotBattle) {
			t.Fatalf("expected ErrFriendlyGuildCannotBattle, got %v", err)
		}
	})

	t.Run("fails if unconscious or exhausted", func(t *testing.T) {
		guildRepo.charGuilds["c_dead"] = "g1"
		guildRepo.charGuilds["c_tired"] = "g1"

		_, err := svc.CreateRoom(ctx, "c_dead", gvg.CreateRoomRequest{Name: "GvG Arena"})
		if !errors.Is(err, gvg.ErrCharacterUnconscious) {
			t.Fatalf("expected ErrCharacterUnconscious, got %v", err)
		}

		_, err = svc.CreateRoom(ctx, "c_tired", gvg.CreateRoomRequest{Name: "GvG Arena"})
		if !errors.Is(err, gvg.ErrCharacterExhausted) {
			t.Fatalf("expected ErrCharacterExhausted, got %v", err)
		}
	})

	t.Run("successfully creates room with initial 2 GP prize pool", func(t *testing.T) {
		detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
			Name:       "Crimson Clan War",
			MaxMembers: 8,
			TargetWins: 2,
		})
		if err != nil {
			t.Fatal(err)
		}

		if detail.Room.PrizePool != 2 {
			t.Errorf("expected initial prize pool 2 GP, got %d", detail.Room.PrizePool)
		}
		if detail.Room.Status != gvg.StatusRecruiting {
			t.Errorf("expected status recruiting, got %s", detail.Room.Status)
		}
		if len(detail.Members) != 1 {
			t.Fatalf("expected 1 member, got %d", len(detail.Members))
		}
		if detail.Members[0].GuildColor != "#FF3333" {
			t.Errorf("expected leader guild color #FF3333, got %s", detail.Members[0].GuildColor)
		}

		// Character is mapped to active room
		inRoom, err := svc.GetCharacterRoom(ctx, "c1")
		if err != nil || inRoom.Room.ID != detail.Room.ID {
			t.Fatalf("expected character mapped to room %s, got %v", detail.Room.ID, err)
		}
	})

	t.Run("fails if character already in active room", func(t *testing.T) {
		_, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Duplicate"})
		if !errors.Is(err, gvg.ErrAlreadyInRoom) {
			t.Fatalf("expected ErrAlreadyInRoom, got %v", err)
		}
	})
}

func TestJoinAndLeaveRoom(t *testing.T) {
	ctx := context.Background()
	roomRepo := gvg.NewMemoryRoomRepository()
	standingRepo := newMockStandingRepo()
	guildRepo := newMockGuildRepo()
	charRepo := newMockCharRepo()
	battleEngine := &mockBattleEngine{outcome: corebattle.OutcomeWin}

	svc, _ := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)

	charRepo.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	charRepo.characters["c2"] = createTestCharacter("c2", "Joiner", 100, 0)
	charRepo.characters["c3"] = createTestCharacter("c3", "Third", 100, 0)

	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guildRepo.charGuilds["c1"] = "g1"

	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds["c2"] = "g2"
	guildRepo.charGuilds["c3"] = "g2"

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
		Name:       "Test GvG",
		Password:   "secret",
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("fails join with wrong password", func(t *testing.T) {
		_, err := svc.JoinRoom(ctx, "c2", detail.Room.ID, "wrong")
		if !errors.Is(err, gvg.ErrInvalidPassword) {
			t.Fatalf("expected ErrInvalidPassword, got %v", err)
		}
	})

	t.Run("successful join adds 1 GP to prize pool and adopts guild color", func(t *testing.T) {
		joined, err := svc.JoinRoom(ctx, "c2", detail.Room.ID, "secret")
		if err != nil {
			t.Fatal(err)
		}
		if joined.Room.PrizePool != 3 { // 2 initial + 1 joiner = 3 GP
			t.Errorf("expected prize pool 3 GP, got %d", joined.Room.PrizePool)
		}
		if len(joined.Members) != 2 {
			t.Fatalf("expected 2 members, got %d", len(joined.Members))
		}
		if joined.Members[1].GuildColor != "#33CCFF" {
			t.Errorf("expected joiner guild color #33CCFF, got %s", joined.Members[1].GuildColor)
		}
	})

	t.Run("fails join when room is full", func(t *testing.T) {
		_, err := svc.JoinRoom(ctx, "c3", detail.Room.ID, "secret")
		if !errors.Is(err, gvg.ErrRoomFull) {
			t.Fatalf("expected ErrRoomFull, got %v", err)
		}
	})

	t.Run("regular member leaves room and reduces 1 GP", func(t *testing.T) {
		err := svc.LeaveRoom(ctx, "c2", detail.Room.ID)
		if err != nil {
			t.Fatal(err)
		}
		afterLeave, err := svc.GetRoom(ctx, detail.Room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if len(afterLeave.Members) != 1 {
			t.Fatalf("expected 1 member left, got %d", len(afterLeave.Members))
		}
		if afterLeave.Room.PrizePool != 2 {
			t.Errorf("expected prize pool reduced to 2 GP, got %d", afterLeave.Room.PrizePool)
		}
	})

	t.Run("leader leaves and disbands room", func(t *testing.T) {
		err := svc.LeaveRoom(ctx, "c1", detail.Room.ID)
		if err != nil {
			t.Fatal(err)
		}
		_, err = svc.GetRoom(ctx, detail.Room.ID)
		if !errors.Is(err, gvg.ErrRoomNotFound) {
			t.Fatalf("expected ErrRoomNotFound after disband, got %v", err)
		}
	})
}

func TestGvGMatchFlow(t *testing.T) {
	ctx := context.Background()
	roomRepo := gvg.NewMemoryRoomRepository()
	standingRepo := newMockStandingRepo()
	guildRepo := newMockGuildRepo()
	charRepo := newMockCharRepo()
	battleEngine := &mockBattleEngine{outcome: corebattle.OutcomeWin}

	svc, _ := gvg.NewService(roomRepo, standingRepo, guildRepo, charRepo, battleEngine)

	charRepo.characters["c1"] = createTestCharacter("c1", "Leader Crimson", 50, 0)
	charRepo.characters["c2"] = createTestCharacter("c2", "Ally Crimson", 60, 0)
	charRepo.characters["c3"] = createTestCharacter("c3", "Enemy Azure", 70, 0)

	guildRepo.guilds["g_crimson"] = guild.Guild{ID: "g_crimson", Name: "Crimson", Color: "#FF3333"}
	guildRepo.charGuilds["c1"] = "g_crimson"
	guildRepo.charGuilds["c2"] = "g_crimson"

	guildRepo.guilds["g_azure"] = guild.Guild{ID: "g_azure", Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds["c3"] = "g_azure"

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
		Name:       "War of the Guilds",
		MaxMembers: 4,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cannot start with only 1 participant", func(t *testing.T) {
		_, err := svc.StartMatch(ctx, "c1", detail.Room.ID)
		if !errors.Is(err, gvg.ErrNotEnoughParticipants) {
			t.Fatalf("expected ErrNotEnoughParticipants, got %v", err)
		}
	})

	// Add ally with same color
	_, err = svc.JoinRoom(ctx, "c2", detail.Room.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("cannot start with only 1 guild color", func(t *testing.T) {
		_, err := svc.StartMatch(ctx, "c1", detail.Room.ID)
		if !errors.Is(err, gvg.ErrNeedAtLeastTwoGuilds) {
			t.Fatalf("expected ErrNeedAtLeastTwoGuilds, got %v", err)
		}
	})

	// Add opponent with different color
	_, err = svc.JoinRoom(ctx, "c3", detail.Room.ID, "")
	if err != nil {
		t.Fatal(err)
	}

	t.Run("start match recovers HP and sets round 1 in progress", func(t *testing.T) {
		started, err := svc.StartMatch(ctx, "c1", detail.Room.ID)
		if err != nil {
			t.Fatal(err)
		}
		if started.Room.Status != gvg.StatusInProgress || started.Room.Round != 1 {
			t.Fatalf("expected in_progress round 1, got status=%s round=%d", started.Room.Status, started.Room.Round)
		}
		// Participant HP should be restored to MaxHP (100)
		for _, m := range started.Members {
			if m.HP != 100 {
				t.Errorf("expected HP restored to 100, got %d for %s", m.HP, m.CharacterName)
			}
		}
	})

	t.Run("advance round resolves combat and completes match on target wins", func(t *testing.T) {
		// Prize pool: 2 initial + 2 joiners (c2, c3) = 4 GP
		res, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID)
		if err != nil {
			t.Fatal(err)
		}

		if !res.MatchCompleted {
			t.Fatal("expected match to be completed on reaching 1 target win")
		}
		if res.Outcome != "match_won" {
			t.Errorf("expected outcome match_won, got %s", res.Outcome)
		}
		if res.OverallWinnerGuildID != "g_crimson" {
			t.Errorf("expected winner g_crimson, got %s", res.OverallWinnerGuildID)
		}

		// Verify Standings:
		// 1. Round win: g_crimson got +3 GP
		// 2. Match settlement:
		//    - g_crimson (winner): +4 GP prize pool + (4 * 2 participants) = 12 GP, +1 Win, +1 Bronze Medal
		//    - Total GP for g_crimson: 3 (round) + 12 (match) = 15 GP
		//    - g_azure (loser): +1 Loss, + (4 * 1 participant) = 4 GP
		stCrimson, err := standingRepo.GetOrCreateStanding(ctx, "g_crimson")
		if err != nil {
			t.Fatal(err)
		}
		if stCrimson.Wins != 1 {
			t.Errorf("expected 1 win for Crimson, got %d", stCrimson.Wins)
		}
		if stCrimson.BronzeMedals != 1 {
			t.Errorf("expected 1 bronze medal for Crimson, got %d", stCrimson.BronzeMedals)
		}
		if stCrimson.VictoryPoints != 15 {
			t.Errorf("expected 15 GP for Crimson, got %d", stCrimson.VictoryPoints)
		}

		stAzure, err := standingRepo.GetOrCreateStanding(ctx, "g_azure")
		if err != nil {
			t.Fatal(err)
		}
		if stAzure.Losses != 1 {
			t.Errorf("expected 1 loss for Azure, got %d", stAzure.Losses)
		}
		if stAzure.VictoryPoints != 4 {
			t.Errorf("expected 4 GP for Azure, got %d", stAzure.VictoryPoints)
		}
	})
}
