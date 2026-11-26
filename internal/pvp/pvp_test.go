package pvp_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type mockCharRepo struct {
	mu         sync.Mutex
	characters map[string]corecharacter.Character
	prizes     map[string]int
}

func newMockCharRepo() *mockCharRepo {
	return &mockCharRepo{characters: make(map[string]corecharacter.Character), prizes: make(map[string]int)}
}

func (m *mockCharRepo) HoldPrize(_ context.Context, characterID, _ string, amount int) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.prizes[characterID] += amount
	return nil
}

func (m *mockCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c, ok := m.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (m *mockCharRepo) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return m.FindByID(ctx, id)
}

func (m *mockCharRepo) Update(_ context.Context, character corecharacter.Character) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[character.ID] = character
	return nil
}

func (m *mockCharRepo) add(c corecharacter.Character) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.characters[c.ID] = c
}

type mockBattleEngine struct {
	result corebattle.PartyBattleResult
	err    error
}

func (m mockBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return m.result, m.err
}

func createTestCharacter(id, name string, money int) corecharacter.Character {
	char, _ := corecharacter.New(name)
	char.ID = id
	char.Money = money
	char.Stats.HP = 100
	char.Stats.MaxHP = 100
	char.Stats.Attack = 20
	char.Stats.Defense = 20
	char.Stats.Agility = 20
	return char
}

func setupService(battleOutcome corebattle.Outcome) (*pvp.Service, *mockCharRepo, *pvp.MemoryRoomRepository) {
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: battleOutcome,
			Turns:   3,
			Logs:    []corebattle.TurnLog{{Turn: 1, Kind: "damage"}},
		},
	}
	svc, _ := pvp.NewService(roomRepo, charRepo, battleEngine, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()), pvp.WithPrizeRemitter(charRepo))
	return svc, charRepo, roomRepo
}

func TestColosseum_CreateRoom(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin)

	hero := createTestCharacter("char-1", "Hero", 500)
	charRepo.add(hero)

	// Valid creation
	detail, err := svc.CreateRoom(ctx, hero.ID, pvp.CreateRoomRequest{
		Name:       "ArenaGrandMatch",
		Bet:        100,
		MaxMembers: 4,
		TargetWins: 2,
		Speed:      18,
	})
	if err != nil {
		t.Fatalf("unexpected error creating room: %v", err)
	}

	if detail.Room.Name != "ArenaGrandMatch" {
		t.Errorf("expected room name %q, got %q", "ArenaGrandMatch", detail.Room.Name)
	}
	if detail.Room.PrizePool != 100 {
		t.Errorf("expected initial prize pool 100, got %d", detail.Room.PrizePool)
	}
	if len(detail.Members) != 1 || !detail.Members[0].IsLeader {
		t.Fatalf("expected leader member, got %+v", detail.Members)
	}

	// Verify leader wallet deduction
	updatedHero, _ := charRepo.FindByID(ctx, hero.ID)
	if updatedHero.Money != 400 {
		t.Errorf("expected hero money 400 after 100G bet, got %d", updatedHero.Money)
	}

	// Invalid room names (empty, >50 runes, whitespace, fullwidth space, prohibited symbols, unsafe Unicode)
	for _, invalid := range []string{
		"",
		" ",
		" room",
		"room ",
		"room 1",
		"闘技　場",
		"room@1",
		"＠闘技場",
		"room/1",
		"room<1>",
		"room\x001",
		"room\u200B1",
		"room\u202E1",
		"Z\u0300\u0301\u0302",
		strings.Repeat("あ", 51),
	} {
		if _, err := svc.CreateRoom(ctx, hero.ID, pvp.CreateRoomRequest{Name: invalid, Bet: 10}); !errors.Is(err, pvp.ErrInvalidRoomName) {
			t.Errorf("CreateRoom(%q) = %v, want ErrInvalidRoomName", invalid, err)
		}
	}

	// NFC normalization on valid room name
	nfcHero := createTestCharacter("char-nfc", "NFCHero", 500)
	charRepo.add(nfcHero)
	nfcDetail, err := svc.CreateRoom(ctx, nfcHero.ID, pvp.CreateRoomRequest{Name: "Ka\u0301", Bet: 10})
	if err != nil {
		t.Fatalf("CreateRoom NFC failed: %v", err)
	}
	if nfcDetail.Room.Name != "Ká" {
		t.Errorf("expected NFC-normalized room name %q, got %q", "Ká", nfcDetail.Room.Name)
	}

	// Invalid Bet < 10
	_, err = svc.CreateRoom(ctx, hero.ID, pvp.CreateRoomRequest{Name: "BadBet", Bet: 5})
	if !errors.Is(err, pvp.ErrInvalidBet) {
		t.Errorf("expected ErrInvalidBet, got %v", err)
	}

	// Insufficient funds
	poorChar := createTestCharacter("char-poor", "Poor", 10)
	charRepo.add(poorChar)
	_, err = svc.CreateRoom(ctx, poorChar.ID, pvp.CreateRoomRequest{Name: "RichMatch", Bet: 50})
	if !errors.Is(err, pvp.ErrInsufficientBetFunds) {
		t.Errorf("expected ErrInsufficientBetFunds, got %v", err)
	}
}

func TestColosseum_JoinAndLeaveRoom(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin)

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 300)
	charRepo.add(c1)
	charRepo.add(c2)

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "ColosseumDuel",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Join room
	detail, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	if err != nil {
		t.Fatalf("unexpected join error: %v", err)
	}

	if len(detail.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(detail.Members))
	}
	if detail.Room.PrizePool != 200 {
		t.Errorf("expected prize pool 200, got %d", detail.Room.PrizePool)
	}

	updatedC2, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedC2.Money != 200 {
		t.Errorf("expected Bob money 200, got %d", updatedC2.Money)
	}

	// Joining full room fails
	c3 := createTestCharacter("char-3", "Charlie", 500)
	charRepo.add(c3)
	_, err = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	if !errors.Is(err, pvp.ErrRoomFull) {
		t.Errorf("expected ErrRoomFull, got %v", err)
	}

	// Non-leader leaves: bet refunded
	if _, err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatalf("unexpected leave error: %v", err)
	}

	updatedC2, _ = charRepo.FindByID(ctx, c2.ID)
	if updatedC2.Money != 300 {
		t.Errorf("expected Bob money refunded to 300, got %d", updatedC2.Money)
	}

	updatedDetail, _ := svc.GetRoom(ctx, detail.Room.ID)
	if len(updatedDetail.Members) != 1 {
		t.Errorf("expected 1 member remaining, got %d", len(updatedDetail.Members))
	}
	if updatedDetail.Room.PrizePool != 100 {
		t.Errorf("expected prize pool 100 after refund, got %d", updatedDetail.Room.PrizePool)
	}

	// Last remaining leader leaves first room: disbands & refunds Alice
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("unexpected leader leave error: %v", err)
	}
	updatedC1, _ := charRepo.FindByID(ctx, c1.ID)
	if updatedC1.Money != 500 {
		t.Errorf("expected Alice money refunded to 500, got %d", updatedC1.Money)
	}
	_, err = svc.GetRoom(ctx, detail.Room.ID)
	if !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound after disband, got %v", err)
	}

	// Leader leaves while Bob remains: refunds only Alice, promotes Bob to leader.
	detail2, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "HandoverRoom",
		Bet:        100,
		MaxMembers: 3,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, c2.ID, detail2.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, c3.ID, detail2.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, detail2.Room.ID); err != nil {
		t.Fatalf("unexpected leader handover leave error: %v", err)
	}
	updatedC1, _ = charRepo.FindByID(ctx, c1.ID)
	updatedC2, _ = charRepo.FindByID(ctx, c2.ID)
	updatedC3, _ := charRepo.FindByID(ctx, c3.ID)
	if updatedC1.Money != 500 || updatedC2.Money != 200 || updatedC3.Money != 400 {
		t.Fatalf("expected only Alice refunded to 500 (Bob=200, Charlie=400), got %d/%d/%d", updatedC1.Money, updatedC2.Money, updatedC3.Money)
	}
	handed, err := svc.GetRoom(ctx, detail2.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if handed.Room.LeaderCharacterID != c2.ID || handed.Room.LeaderName != c2.Name || handed.Room.PrizePool != 200 || len(handed.Members) != 2 || !handed.Members[0].IsLeader || handed.Members[0].CharacterID != c2.ID {
		t.Fatalf("expected Bob (%s) promoted to leader with 200G pool, got room=%+v members=%+v", c2.Name, handed.Room, handed.Members)
	}
	listed, err := svc.ListRooms(ctx)
	if err != nil || len(listed) != 1 || listed[0].LeaderCharacterID != c2.ID || listed[0].LeaderName != c2.Name || !slices.Equal(listed[0].MemberNames, []string{c2.Name, c3.Name}) {
		t.Fatalf("expected ListRooms to reflect new leader %s (%s), got %+v err=%v", c2.ID, c2.Name, listed, err)
	}
	outsider := createTestCharacter("outsider", "Outsider", 500)
	charRepo.add(outsider)
	if _, err := svc.LeaveRoom(ctx, outsider.ID, detail2.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor, got %v", err)
	}
	// Remaining members leave one by one until last member disbands the room
	if _, err := svc.LeaveRoom(ctx, c2.ID, detail2.Room.ID); err != nil {
		t.Fatal(err)
	}
	handed2, err := svc.GetRoom(ctx, detail2.Room.ID)
	if err != nil || handed2.Room.LeaderCharacterID != c3.ID || handed2.Room.LeaderName != c3.Name {
		t.Fatalf("expected Charlie (%s) promoted to leader, got %+v err=%v", c3.Name, handed2.Room, err)
	}
	if _, err := svc.LeaveRoom(ctx, c3.ID, detail2.Room.ID); err != nil {
		t.Fatal(err)
	}
	_, err = svc.GetRoom(ctx, detail2.Room.ID)
	if !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Errorf("expected ErrRoomNotFound after final disband, got %v", err)
	}
}

func TestColosseum_SelectTeamAndStartMatch(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin)

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	detail, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "TeamClash",
		Bet:        50,
		MaxMembers: 4,
		TargetWins: 2,
	})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")

	// Invalid color fails
	_, err := svc.SelectTeam(ctx, c1.ID, detail.Room.ID, "#111111")
	if !errors.Is(err, pvp.ErrInvalidTeamColor) {
		t.Errorf("expected ErrInvalidTeamColor, got %v", err)
	}

	// Start without selecting teams fails
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if !errors.Is(err, pvp.ErrTeamsNotConfigured) {
		t.Errorf("expected ErrTeamsNotConfigured, got %v", err)
	}

	// Both pick Red: only 1 team, start fails
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorRed)
	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if !errors.Is(err, pvp.ErrNeedAtLeastTwoTeams) {
		t.Errorf("expected ErrNeedAtLeastTwoTeams, got %v", err)
	}

	// Bob switches to Blue
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)

	// Non-leader cannot start
	_, err = svc.StartMatch(ctx, c2.ID, detail.Room.ID)
	if !errors.Is(err, pvp.ErrNotRoomLeader) {
		t.Errorf("expected ErrNotRoomLeader, got %v", err)
	}

	// Leader starts match
	started, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("unexpected start error: %v", err)
	}
	if started.Room.Status != pvp.StatusInProgress || started.Room.Round != 1 {
		t.Errorf("expected in_progress round 1, got status %q round %d", started.Room.Status, started.Room.Round)
	}

	// Cannot select team after start
	_, err = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorGreen)
	if !errors.Is(err, pvp.ErrMatchAlreadyStarted) {
		t.Errorf("expected ErrMatchAlreadyStarted, got %v", err)
	}
}

func TestColosseum_AdvanceRoundAndMatchVictory(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin) // Team 1 (Allies) wins

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	detail, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Championship",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1, // 1 win needed for overall victory
	})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	// Advance Round 1 -> Alice's team (Red) wins
	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("unexpected AdvanceRound error: %v", err)
	}

	if !res.MatchCompleted {
		t.Fatalf("expected match completed on reaching target wins 1")
	}
	if res.OverallWinnerTeam != pvp.ColorRed {
		t.Errorf("expected winner team Red, got %s", res.OverallWinnerTeam)
	}
	if res.PrizePerMember != 200 {
		t.Errorf("expected prize 200G awarded, got %d", res.PrizePerMember)
	}

	// The 200G prize is held separately from Alice's wallet.
	updatedAlice, _ := charRepo.FindByID(ctx, c1.ID)
	if updatedAlice.Money != 400 || charRepo.prizes[c1.ID] != 200 {
		t.Errorf("expected wallet 400 and held prize 200, got %d / %d", updatedAlice.Money, charRepo.prizes[c1.ID])
	}
	if updatedAlice.PvPWins != 0 {
		t.Errorf("expected Alice PvPWins 0 without strong defeats, got %d", updatedAlice.PvPWins)
	}

	// Verify Bob's wallet has 500 - 100 bet = 400G
	updatedBob, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedBob.Money != 400 {
		t.Errorf("expected Bob money 400, got %d", updatedBob.Money)
	}
	if updatedBob.PvPWins != 0 {
		t.Errorf("expected Bob PvPWins 0, got %d", updatedBob.PvPWins)
	}
}

func TestColosseum_MultiRound_2v2TeamBattle(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin) // Leader's team (Red) wins

	c1 := createTestCharacter("char-1", "Alice", 1000)
	c2 := createTestCharacter("char-2", "Bob", 1000)
	c3 := createTestCharacter("char-3", "Charlie", 1000)
	c4 := createTestCharacter("char-4", "Dave", 1000)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)
	charRepo.add(c4)

	// Create room with 100G bet, TargetWins = 2
	detail, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "2v2Championship",
		Bet:        100,
		MaxMembers: 4,
		TargetWins: 2,
	})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c4.ID, detail.Room.ID, "")

	// Red team: Alice (c1) and Charlie (c3)
	// Blue team: Bob (c2) and Dave (c4)
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c3.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.SelectTeam(ctx, c4.ID, detail.Room.ID, pvp.ColorBlue)

	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	// Round 1: Red wins (score: Red 1, Blue 0). Not completed yet (TargetWins = 2)
	res1, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("round 1 failed: %v", err)
	}
	if res1.MatchCompleted {
		t.Fatalf("expected match not completed after 1 win when target is 2")
	}
	if res1.TeamScores[pvp.ColorRed] != 1 {
		t.Errorf("expected Red score 1, got %d", res1.TeamScores[pvp.ColorRed])
	}

	// Round 2: Red wins again (score: Red 2). Match completed!
	res2, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("round 2 failed: %v", err)
	}
	if !res2.MatchCompleted {
		t.Fatalf("expected match completed after 2nd win")
	}
	if res2.OverallWinnerTeam != pvp.ColorRed {
		t.Errorf("expected overall winner Red, got %s", res2.OverallWinnerTeam)
	}

	// Total prize pool: 400G (100G x 4 players).
	// Red team has 2 winners (Alice and Charlie).
	// Prize per winner: 400 / 2 = 200G!
	if res2.PrizePerMember != 200 {
		t.Errorf("expected 200G per winner, got %d", res2.PrizePerMember)
	}

	updatedAlice, _ := charRepo.FindByID(ctx, c1.ID)
	if updatedAlice.Money != 900 || charRepo.prizes[c1.ID] != 200 {
		t.Errorf("expected wallet 900 and held prize 200, got %d / %d", updatedAlice.Money, charRepo.prizes[c1.ID])
	}
	if updatedAlice.PvPWins != 0 {
		t.Errorf("expected Alice PvPWins 0 without strong defeats, got %d", updatedAlice.PvPWins)
	}

	updatedCharlie, _ := charRepo.FindByID(ctx, c3.ID)
	if updatedCharlie.Money != 900 || charRepo.prizes[c3.ID] != 200 {
		t.Errorf("expected wallet 900 and held prize 200, got %d / %d", updatedCharlie.Money, charRepo.prizes[c3.ID])
	}
	if updatedCharlie.PvPWins != 0 {
		t.Errorf("expected Charlie PvPWins 0 without strong defeats, got %d", updatedCharlie.PvPWins)
	}

	// Losers (Bob & Dave) should have 900G each (1000 - 100) and 0 PvPWins
	updatedBob, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedBob.Money != 900 || updatedBob.PvPWins != 0 {
		t.Errorf("expected Bob money 900 and 0 wins, got %d / %d", updatedBob.Money, updatedBob.PvPWins)
	}
}

type scriptedBattleEngine struct {
	results []corebattle.PartyBattleResult
	idx     int
}

func (e *scriptedBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	res := e.results[e.idx]
	e.idx++
	return res, nil
}

func TestColosseum_KillPRecordedPerStrongDefeatAcrossRounds(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	// Alice (strong=170) vs StrongBob (strong=170 > 85) and WeakFoe (strong=50 <= 85)
	alice := createTestCharacter("char-1", "Alice", 500)
	teammate := createTestCharacter("char-2", "Teammate", 500)
	strongBob := createTestCharacter("char-3", "StrongBob", 500)
	weakFoe := createTestCharacter("char-4", "WeakFoe", 500)
	weakFoe.Stats.MaxHP, weakFoe.Stats.HP = 50, 50
	weakFoe.Stats.Attack, weakFoe.Stats.Defense, weakFoe.Stats.Agility = 0, 0, 0
	for _, c := range []corecharacter.Character{alice, teammate, strongBob, weakFoe} {
		charRepo.add(c)
	}

	engine := &scriptedBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				// Round 1: ends in draw, but Alice defeated StrongBob earlier in combat (teammate did not).
				Outcome:     corebattle.OutcomeDraw,
				Turns:       4,
				RemainingHP: map[string]int{"char-1": 50, "char-2": 0, "char-3": 0, "char-4": 50},
				Defeats:     []corebattle.PartyBattleDefeat{{ActorID: "char-1", TargetID: "char-3"}},
			},
			{
				// Round 2: Alice defeats StrongBob and WeakFoe -> only StrongBob counts; teammate gets 0.
				Outcome:     corebattle.OutcomeWin,
				Turns:       3,
				RemainingHP: map[string]int{"char-1": 80, "char-2": 80, "char-3": 0, "char-4": 0},
				Defeats: []corebattle.PartyBattleDefeat{
					{ActorID: "char-1", TargetID: "char-3"},
					{ActorID: "char-1", TargetID: "char-4"},
				},
			},
		},
	}

	svc, err := pvp.NewService(roomRepo, charRepo, engine, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()), pvp.WithPrizeRemitter(charRepo))
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}
	detail, _ := svc.CreateRoom(ctx, alice.ID, pvp.CreateRoomRequest{Name: "StrongKillP", Bet: 100, MaxMembers: 4, TargetWins: 1})
	_, _ = svc.JoinRoom(ctx, teammate.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, strongBob.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, weakFoe.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, alice.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, teammate.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, strongBob.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.SelectTeam(ctx, weakFoe.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, alice.ID, detail.Room.ID)

	if _, err := svc.AdvanceRound(ctx, alice.ID, detail.Room.ID); err != nil {
		t.Fatalf("round 1 failed: %v", err)
	}
	// Non-final round keeps kills pending in room until match completion.
	midAlice, _ := charRepo.FindByID(ctx, alice.ID)
	if midAlice.PvPWins != 0 {
		t.Fatalf("expected pending PvPWins=0 before settlement, got %d", midAlice.PvPWins)
	}

	res2, err := svc.AdvanceRound(ctx, alice.ID, detail.Room.ID)
	if err != nil || !res2.MatchCompleted {
		t.Fatalf("round 2 settlement failed: res=%+v err=%v", res2, err)
	}

	finAlice, _ := charRepo.FindByID(ctx, alice.ID)
	finTeammate, _ := charRepo.FindByID(ctx, teammate.ID)
	finBob, _ := charRepo.FindByID(ctx, strongBob.ID)
	if finAlice.PvPWins != 2 {
		t.Errorf("expected Alice PvPWins 2 (1 per strong defeat across rounds, excluding weak foe), got %d", finAlice.PvPWins)
	}
	if finTeammate.PvPWins != 0 || charRepo.prizes[teammate.ID] != 200 {
		t.Errorf("expected winning teammate with 0 kills to get prize 200 and PvPWins 0, got prize=%d wins=%d", charRepo.prizes[teammate.ID], finTeammate.PvPWins)
	}
	if finBob.PvPWins != 0 {
		t.Errorf("expected StrongBob PvPWins 0, got %d", finBob.PvPWins)
	}
}

func TestColosseum_MaxRoundsDraw(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeDraw) // Always draws

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	detail, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "EndlessDuel",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 3,
	})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	// Simulate 10 rounds of draws
	var lastRes pvp.RoundResolution
	for r := 1; r <= 10; r++ {
		var err error
		lastRes, err = svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
		if err != nil {
			t.Fatalf("round %d failed: %v", r, err)
		}
	}

	if !lastRes.MatchCompleted {
		t.Errorf("expected match completed at round 10")
	}
	if lastRes.Outcome != "match_draw" {
		t.Errorf("expected match_draw outcome, got %s", lastRes.Outcome)
	}

	// Legacy vs_player.cgi gives NO bet refund on undecided max-round draw
	updatedAlice, _ := charRepo.FindByID(ctx, c1.ID)
	if updatedAlice.Money != 400 {
		t.Errorf("expected Alice money to remain 400 (no draw refund), got %d", updatedAlice.Money)
	}
	updatedBob, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedBob.Money != 400 {
		t.Errorf("expected Bob money to remain 400 (no draw refund), got %d", updatedBob.Money)
	}
}

func TestColosseum_LeaveRoom_InProgress_AndForfeit(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	transRepo := pvp.NewMemoryTransitionRepository()
	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{err: errors.New("battle engine must not be called when only one team remains")},
		pvp.WithTransitionRepository(transRepo),
		pvp.WithPrizeRemitter(charRepo),
	)
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 500)
	c3 := createTestCharacter("char-3", "Charlie", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	// Case 1: Leader leaves mid-match -> no refund, Bob becomes leader and wins remaining pool via AdvanceRound
	d1, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "ForfeitWin", Bet: 100, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.JoinRoom(ctx, c2.ID, d1.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, d1.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d1.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d1.Room.ID)

	if _, err := svc.LeaveRoom(ctx, c3.ID, d1.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor in_progress, got %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, d1.Room.ID); err != nil {
		t.Fatalf("unexpected in_progress leader leave error: %v", err)
	}
	// Retry is idempotent
	if _, err := svc.LeaveRoom(ctx, c1.ID, d1.Room.ID); err != nil {
		t.Fatalf("unexpected retry leave error: %v", err)
	}
	aAfter, _ := charRepo.FindByID(ctx, c1.ID)
	if aAfter.Money != 400 {
		t.Fatalf("expected no refund for in_progress leave, got %d", aAfter.Money)
	}
	afterLeave, err := svc.GetRoom(ctx, d1.Room.ID)
	if err != nil || afterLeave.Room.LeaderCharacterID != c2.ID || afterLeave.Room.LeaderName != c2.Name || afterLeave.Room.PrizePool != 200 {
		t.Fatalf("expected Bob (%s) promoted with 200G pool, got %+v err=%v", c2.Name, afterLeave.Room, err)
	}

	res, err := svc.AdvanceRound(ctx, c2.ID, d1.Room.ID)
	if err != nil {
		t.Fatalf("expected AdvanceRound sole surviving team win, got %v", err)
	}
	if !res.MatchCompleted || res.OverallWinnerTeam != pvp.ColorBlue || res.PrizePerMember != 200 || charRepo.prizes[c2.ID] != 200 {
		t.Fatalf("expected Bob to win 200G forfeited pool, got res=%+v prizes=%+v", res, charRepo.prizes)
	}
	if _, err := svc.LeaveRoom(ctx, c2.ID, d1.Room.ID); err != nil {
		t.Fatalf("expected Bob to exit completed room before re-entering: %v", err)
	}

	// Case 2: Both members leave in_progress -> room closes with durable ForfeitedPool
	d2, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "MutualForfeit", Bet: 100, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, c2.ID, d2.Room.ID, ""); err != nil {
		t.Fatalf("JoinRoom d2: %v", err)
	}
	_, _ = svc.SelectTeam(ctx, c1.ID, d2.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d2.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d2.Room.ID)

	if _, err := svc.LeaveRoom(ctx, c2.ID, d2.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, d2.Room.ID); err != nil {
		t.Fatal(err)
	}
	tr, err := transRepo.FindTransition(ctx, d2.Room.ID)
	if err != nil || tr.Kind != "forfeit" || tr.ForfeitedPool != 200 || tr.Detail.Room.Status != pvp.StatusDisbanded {
		t.Fatalf("expected forfeit transition with 200G ForfeitedPool, got %+v err=%v", tr, err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, d2.Room.ID); err != nil {
		t.Fatalf("expected idempotent retry on forfeit, got %v", err)
	}
	if _, err := svc.GetRoom(ctx, d2.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected room deleted after last-member forfeit, got %v", err)
	}
}

func TestColosseum_LeaveRoom_Completed(t *testing.T) {
	ctx := context.Background()
	svc, charRepo, _ := setupService(corebattle.OutcomeWin)

	c1 := createTestCharacter("char-1", "Alice", 500)
	c2 := createTestCharacter("char-2", "Bob", 500)
	c3 := createTestCharacter("char-3", "Outsider", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	d, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "PostMatchExit", Bet: 100, MaxMembers: 2, TargetWins: 1})
	_, _ = svc.JoinRoom(ctx, c2.ID, d.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, d.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d.Room.ID)
	res, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID)
	if err != nil || !res.MatchCompleted {
		t.Fatalf("expected completed match, got %+v err=%v", res, err)
	}

	if _, err := svc.LeaveRoom(ctx, c3.ID, d.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor in completed room, got %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, d.Room.ID); err != nil {
		t.Fatalf("expected leader completed leave success, got %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c1.ID, d.Room.ID); err != nil {
		t.Fatalf("expected idempotent retry after completed leave, got %v", err)
	}
	rem, err := svc.GetRoom(ctx, d.Room.ID)
	if err != nil || rem.Room.LeaderCharacterID != c2.ID || rem.Room.LeaderName != c2.Name || len(rem.Members) != 1 {
		t.Fatalf("expected Bob (%s) promoted on completed leader exit, got %+v err=%v", c2.Name, rem, err)
	}
	if _, err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
		t.Fatalf("expected final completed leave success, got %v", err)
	}
	if _, err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
		t.Fatalf("expected idempotent retry on deleted completed room, got %v", err)
	}
	if _, err := svc.GetRoom(ctx, d.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound after final completed leave, got %v", err)
	}
	if charRepo.prizes[c1.ID] != 200 || charRepo.prizes[c2.ID] != 0 {
		t.Fatalf("expected single 200G prize without duplicates/refunds, got %+v", charRepo.prizes)
	}
}

func TestColosseum_LeaveRoom_Completed_PreservesAdvanceRoundReplay(t *testing.T) {
	ctx := context.Background()
	t.Run("non-leader exits and remaining leader retries AdvanceRound", func(t *testing.T) {
		svc, charRepo, _ := setupService(corebattle.OutcomeWin)
		c1 := createTestCharacter("char-1", "Alice", 500)
		c2 := createTestCharacter("char-2", "Bob", 500)
		charRepo.add(c1)
		charRepo.add(c2)

		d, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "RetryAfterExit", Bet: 100, MaxMembers: 2, TargetWins: 1})
		_, _ = svc.JoinRoom(ctx, c2.ID, d.Room.ID, "")
		_, _ = svc.SelectTeam(ctx, c1.ID, d.Room.ID, pvp.ColorRed)
		_, _ = svc.SelectTeam(ctx, c2.ID, d.Room.ID, pvp.ColorBlue)
		_, _ = svc.StartMatch(ctx, c1.ID, d.Room.ID)
		res, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID)
		if err != nil || !res.MatchCompleted {
			t.Fatalf("expected completed match, got %+v err=%v", res, err)
		}

		if _, err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
			t.Fatalf("expected non-leader completed leave success, got %v", err)
		}

		replayed, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID)
		if err != nil {
			t.Fatalf("expected leader AdvanceRound retry after member exit to replay result, got %v", err)
		}
		if !replayed.MatchCompleted || replayed.WinnerTeam != res.WinnerTeam || replayed.PrizePerMember != res.PrizePerMember {
			t.Fatalf("expected replayed resolution %+v, got %+v", res, replayed)
		}
		if charRepo.prizes[c1.ID] != 200 || charRepo.prizes[c2.ID] != 0 {
			t.Fatalf("expected no duplicate prize on retry, got %+v", charRepo.prizes)
		}
	})

	t.Run("leader exits and promoted remaining leader retries AdvanceRound until last exit", func(t *testing.T) {
		svc, charRepo, _ := setupService(corebattle.OutcomeWin)
		c1 := createTestCharacter("char-1", "Alice", 500)
		c2 := createTestCharacter("char-2", "Bob", 500)
		charRepo.add(c1)
		charRepo.add(c2)

		d, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "PromotedRetry", Bet: 100, MaxMembers: 2, TargetWins: 1})
		_, _ = svc.JoinRoom(ctx, c2.ID, d.Room.ID, "")
		_, _ = svc.SelectTeam(ctx, c1.ID, d.Room.ID, pvp.ColorRed)
		_, _ = svc.SelectTeam(ctx, c2.ID, d.Room.ID, pvp.ColorBlue)
		_, _ = svc.StartMatch(ctx, c1.ID, d.Room.ID)
		res, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID)
		if err != nil || !res.MatchCompleted {
			t.Fatalf("expected completed match, got %+v err=%v", res, err)
		}

		if _, err := svc.LeaveRoom(ctx, c1.ID, d.Room.ID); err != nil {
			t.Fatalf("expected leader completed leave success, got %v", err)
		}
		if _, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID); !errors.Is(err, pvp.ErrNotRoomLeader) {
			t.Fatalf("expected ErrNotRoomLeader for departed leader, got %v", err)
		}
		replayed, err := svc.AdvanceRound(ctx, c2.ID, d.Room.ID)
		if err != nil {
			t.Fatalf("expected promoted leader AdvanceRound retry to succeed, got %v", err)
		}
		if !replayed.MatchCompleted || replayed.WinnerTeam != res.WinnerTeam || replayed.PrizePerMember != res.PrizePerMember {
			t.Fatalf("expected replayed resolution %+v, got %+v", res, replayed)
		}

		if _, err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
			t.Fatalf("expected final completed leave success, got %v", err)
		}
		if _, err := svc.AdvanceRound(ctx, c2.ID, d.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected ErrRoomNotFound after final leave disbanded room, got %v", err)
		}
	})
}
