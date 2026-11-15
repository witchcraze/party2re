package pvp_test

import (
	"context"
	"errors"
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
			Logs:    []corebattle.TurnLog{{Turn: 1, Message: "Round combat concluded."}},
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
		Name:       "Arena Grand Match",
		Bet:        100,
		MaxMembers: 4,
		TargetWins: 2,
		Speed:      18,
	})
	if err != nil {
		t.Fatalf("unexpected error creating room: %v", err)
	}

	if detail.Room.Name != "Arena Grand Match" {
		t.Errorf("expected room name %q, got %q", "Arena Grand Match", detail.Room.Name)
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

	// Invalid Bet < 10
	_, err = svc.CreateRoom(ctx, hero.ID, pvp.CreateRoomRequest{Name: "Bad Bet", Bet: 5})
	if !errors.Is(err, pvp.ErrInvalidBet) {
		t.Errorf("expected ErrInvalidBet, got %v", err)
	}

	// Insufficient funds
	poorChar := createTestCharacter("char-poor", "Poor", 10)
	charRepo.add(poorChar)
	_, err = svc.CreateRoom(ctx, poorChar.ID, pvp.CreateRoomRequest{Name: "Rich Match", Bet: 50})
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
		Name:       "Colosseum Duel",
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
	if err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
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
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
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
		Name:       "Handover Room",
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
	if err := svc.LeaveRoom(ctx, c1.ID, detail2.Room.ID); err != nil {
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
	if err != nil || len(listed) != 1 || listed[0].LeaderCharacterID != c2.ID || listed[0].LeaderName != c2.Name {
		t.Fatalf("expected ListRooms to reflect new leader %s (%s), got %+v err=%v", c2.ID, c2.Name, listed, err)
	}
	outsider := createTestCharacter("outsider", "Outsider", 500)
	charRepo.add(outsider)
	if err := svc.LeaveRoom(ctx, outsider.ID, detail2.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor, got %v", err)
	}
	// Remaining members leave one by one until last member disbands the room
	if err := svc.LeaveRoom(ctx, c2.ID, detail2.Room.ID); err != nil {
		t.Fatal(err)
	}
	handed2, err := svc.GetRoom(ctx, detail2.Room.ID)
	if err != nil || handed2.Room.LeaderCharacterID != c3.ID || handed2.Room.LeaderName != c3.Name {
		t.Fatalf("expected Charlie (%s) promoted to leader, got %+v err=%v", c3.Name, handed2.Room, err)
	}
	if err := svc.LeaveRoom(ctx, c3.ID, detail2.Room.ID); err != nil {
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
		Name:       "Team Clash",
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

	var victoryCalledWithWinner string
	svc.SetVictoryHook(func(_ context.Context, winnerID, _ string) error {
		victoryCalledWithWinner = winnerID
		return nil
	})

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
	if updatedAlice.PvPWins != 1 {
		t.Errorf("expected Alice PvPWins 1, got %d", updatedAlice.PvPWins)
	}

	// Verify Bob's wallet has 500 - 100 bet = 400G
	updatedBob, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedBob.Money != 400 {
		t.Errorf("expected Bob money 400, got %d", updatedBob.Money)
	}
	if updatedBob.PvPWins != 0 {
		t.Errorf("expected Bob PvPWins 0, got %d", updatedBob.PvPWins)
	}

	// Verify VictoryHook was executed
	if victoryCalledWithWinner != "char-1" {
		t.Errorf("expected VictoryHook called with char-1, got %q", victoryCalledWithWinner)
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
		Name:       "2v2 Championship",
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
	if updatedAlice.PvPWins != 1 {
		t.Errorf("expected Alice PvPWins 1, got %d", updatedAlice.PvPWins)
	}

	updatedCharlie, _ := charRepo.FindByID(ctx, c3.ID)
	if updatedCharlie.Money != 900 || charRepo.prizes[c3.ID] != 200 {
		t.Errorf("expected wallet 900 and held prize 200, got %d / %d", updatedCharlie.Money, charRepo.prizes[c3.ID])
	}
	if updatedCharlie.PvPWins != 1 {
		t.Errorf("expected Charlie PvPWins 1, got %d", updatedCharlie.PvPWins)
	}

	// Losers (Bob & Dave) should have 900G each (1000 - 100) and 0 PvPWins
	updatedBob, _ := charRepo.FindByID(ctx, c2.ID)
	if updatedBob.Money != 900 || updatedBob.PvPWins != 0 {
		t.Errorf("expected Bob money 900 and 0 wins, got %d / %d", updatedBob.Money, updatedBob.PvPWins)
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
		Name:       "Endless Duel",
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
	d1, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Forfeit Win", Bet: 100, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.JoinRoom(ctx, c2.ID, d1.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, d1.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d1.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d1.Room.ID)

	if err := svc.LeaveRoom(ctx, c3.ID, d1.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor in_progress, got %v", err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, d1.Room.ID); err != nil {
		t.Fatalf("unexpected in_progress leader leave error: %v", err)
	}
	// Retry is idempotent
	if err := svc.LeaveRoom(ctx, c1.ID, d1.Room.ID); err != nil {
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

	// Case 2: Both members leave in_progress -> room closes with durable ForfeitedPool
	d2, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Mutual Forfeit", Bet: 100, MaxMembers: 2, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.JoinRoom(ctx, c2.ID, d2.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, d2.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d2.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d2.Room.ID)

	if err := svc.LeaveRoom(ctx, c2.ID, d2.Room.ID); err != nil {
		t.Fatal(err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, d2.Room.ID); err != nil {
		t.Fatal(err)
	}
	tr, err := transRepo.FindTransition(ctx, d2.Room.ID)
	if err != nil || tr.Kind != "forfeit" || tr.ForfeitedPool != 200 || tr.Detail.Room.Status != pvp.StatusDisbanded {
		t.Fatalf("expected forfeit transition with 200G ForfeitedPool, got %+v err=%v", tr, err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, d2.Room.ID); err != nil {
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

	d, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Post-Match Exit", Bet: 100, MaxMembers: 2, TargetWins: 1})
	_, _ = svc.JoinRoom(ctx, c2.ID, d.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, d.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, d.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, d.Room.ID)
	res, err := svc.AdvanceRound(ctx, c1.ID, d.Room.ID)
	if err != nil || !res.MatchCompleted {
		t.Fatalf("expected completed match, got %+v err=%v", res, err)
	}

	if err := svc.LeaveRoom(ctx, c3.ID, d.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
		t.Fatalf("expected ErrCharacterNotInRoom for unrelated actor in completed room, got %v", err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, d.Room.ID); err != nil {
		t.Fatalf("expected leader completed leave success, got %v", err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, d.Room.ID); err != nil {
		t.Fatalf("expected idempotent retry after completed leave, got %v", err)
	}
	rem, err := svc.GetRoom(ctx, d.Room.ID)
	if err != nil || rem.Room.LeaderCharacterID != c2.ID || rem.Room.LeaderName != c2.Name || len(rem.Members) != 1 {
		t.Fatalf("expected Bob (%s) promoted on completed leader exit, got %+v err=%v", c2.Name, rem, err)
	}
	if err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
		t.Fatalf("expected final completed leave success, got %v", err)
	}
	if err := svc.LeaveRoom(ctx, c2.ID, d.Room.ID); err != nil {
		t.Fatalf("expected idempotent retry on deleted completed room, got %v", err)
	}
	if _, err := svc.GetRoom(ctx, d.Room.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound after final completed leave, got %v", err)
	}
	if charRepo.prizes[c1.ID] != 200 || charRepo.prizes[c2.ID] != 0 {
		t.Fatalf("expected single 200G prize without duplicates/refunds, got %+v", charRepo.prizes)
	}
}
