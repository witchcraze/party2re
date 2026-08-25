package replay_test

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestParticipantSnapshot_Constructors(t *testing.T) {
	// 1. Explicit constructor
	snap1 := replay.NewParticipantSnapshot("p1", "Hero", 100, 20, 15, 10, "job-01", 5)
	if snap1.ID != "p1" || snap1.Name != "Hero" || snap1.MaxHP != 100 || snap1.Attack != 20 || snap1.Defense != 15 || snap1.Agility != 10 || snap1.JobID != "job-01" || snap1.Level != 5 {
		t.Errorf("unexpected snap1: %+v", snap1)
	}

	// Fallback name to ID when empty
	snap1EmptyName := replay.NewParticipantSnapshot("p1", "", 100, 20, 15, 10, "job-01", 5)
	if snap1EmptyName.Name != "p1" {
		t.Errorf("expected name to fallback to id %q, got %q", "p1", snap1EmptyName.Name)
	}

	// 2. Character constructor
	char := corecharacter.Character{
		ID:    "char-1",
		Name:  "Warrior",
		Level: 10,
		JobID: "job-01",
		Stats: corecharacter.Stats{
			HP:      150,
			MaxHP:   200,
			Attack:  40,
			Defense: 30,
			Agility: 25,
		},
	}
	snapChar := replay.NewParticipantSnapshotFromCharacter(char)
	if snapChar.ID != "char-1" || snapChar.Name != "Warrior" || snapChar.MaxHP != 200 || snapChar.Attack != 40 || snapChar.Defense != 30 || snapChar.Agility != 25 || snapChar.JobID != "job-01" || snapChar.Level != 10 {
		t.Errorf("unexpected snapChar: %+v", snapChar)
	}

	// Character constructor with MaxHP <= 0 fallback to HP
	charFallbackHP := corecharacter.Character{
		ID:   "char-2",
		Name: "Mage",
		Stats: corecharacter.Stats{
			HP:    80,
			MaxHP: 0,
		},
	}
	snapFallback := replay.NewParticipantSnapshotFromCharacter(charFallbackHP)
	if snapFallback.MaxHP != 80 {
		t.Errorf("expected MaxHP to fallback to HP 80, got %d", snapFallback.MaxHP)
	}

	// 3. Battle Participant constructor
	p := corebattle.Participant{
		ID:      "part-1",
		HP:      120,
		Attack:  35,
		Defense: 18,
	}
	snapPart := replay.NewParticipantSnapshotFromParticipant(p, "Fighter")
	if snapPart.ID != "part-1" || snapPart.Name != "Fighter" || snapPart.MaxHP != 120 || snapPart.Attack != 35 || snapPart.Defense != 18 {
		t.Errorf("unexpected snapPart: %+v", snapPart)
	}

	// 4. Monster constructor
	snapMonster := replay.NewParticipantSnapshotFromMonster("mon-1", "Goblin Chief", 300, 50, 20)
	if snapMonster.ID != "mon-1" || snapMonster.Name != "Goblin Chief" || snapMonster.MaxHP != 300 || snapMonster.Attack != 50 || snapMonster.Defense != 20 {
		t.Errorf("unexpected snapMonster: %+v", snapMonster)
	}
}

func TestRecordMatchFromResult_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	initSnap := replay.NewParticipantSnapshot("hero", "Hero", 100, 25, 10, 12, "warrior", 5)
	oppSnap := replay.NewParticipantSnapshot("slime", "Slime", 60, 15, 5, 8, "", 1)
	battleResult := corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: "hero",
		LoserID:  "slime",
		Turns:    3,
		Logs: []corebattle.TurnLog{
			{Turn: 1, ActorID: "hero", TargetID: "slime", DamageDealt: 25, RemainingHP: map[string]int{"slime": 35}},
			{Turn: 2, ActorID: "slime", TargetID: "hero", DamageDealt: 8, RemainingHP: map[string]int{"hero": 92}},
			{Turn: 3, ActorID: "hero", TargetID: "slime", DamageDealt: 35, RemainingHP: map[string]int{"slime": 0}, IsCritical: true},
		},
	}

	replayID, err := service.RecordMatchFromResult(ctx, replay.CombatTypeAdventure, initSnap, oppSnap, battleResult)
	if err != nil {
		t.Fatalf("RecordMatchFromResult: %v", err)
	}
	if replayID == "" {
		t.Fatal("expected non-empty replay ID")
	}

	rep, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay: %v", err)
	}
	if rep.CombatType != replay.CombatTypeAdventure || rep.WinnerID != "hero" || rep.TotalTurns != 3 || len(rep.TurnLogs) != 3 {
		t.Errorf("unexpected replay: %+v", rep)
	}
}

func TestRecordCharacterVsCharacter_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	c1 := corecharacter.Character{
		ID:    "char1",
		Name:  "Alice",
		Level: 15,
		JobID: "job-01",
		Stats: corecharacter.Stats{HP: 150, MaxHP: 150, Attack: 40, Defense: 20},
	}
	c2 := corecharacter.Character{
		ID:    "char2",
		Name:  "Bob",
		Level: 14,
		JobID: "job-02",
		Stats: corecharacter.Stats{HP: 140, MaxHP: 140, Attack: 38, Defense: 18},
	}
	battleResult := corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: "char1",
		LoserID:  "char2",
		Turns:    2,
	}

	replayID, err := service.RecordCharacterVsCharacter(ctx, replay.CombatTypePvP, c1, c2, battleResult)
	if err != nil {
		t.Fatalf("RecordCharacterVsCharacter: %v", err)
	}

	rep, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay: %v", err)
	}
	if rep.InitiatorName != "Alice" || rep.OpponentName != "Bob" || rep.CombatType != replay.CombatTypePvP {
		t.Errorf("unexpected replay data: %+v", rep)
	}
}

func TestRecordCharacterVsMonster_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	char := corecharacter.Character{
		ID:    "hero-1",
		Name:  "Hero",
		Level: 30,
		JobID: "job-01",
		Stats: corecharacter.Stats{HP: 300, MaxHP: 300, Attack: 80, Defense: 50},
	}
	battleResult := corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: "hero-1",
		LoserID:  "boss-01",
		Turns:    5,
	}

	replayID, err := service.RecordCharacterVsMonster(ctx, replay.CombatTypeBoss, char, "boss-01", "Dragon King", 1200, 150, 70, battleResult)
	if err != nil {
		t.Fatalf("RecordCharacterVsMonster: %v", err)
	}

	rep, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay: %v", err)
	}
	if rep.InitiatorName != "Hero" || rep.OpponentName != "Dragon King" || rep.CombatType != replay.CombatTypeBoss {
		t.Errorf("unexpected replay: %+v", rep)
	}
}

func TestRecordParticipantVsParticipant_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	p1 := corebattle.MustNewParticipant("p1", 100, 30, 15)
	p2 := corebattle.MustNewParticipant("p2", 80, 25, 10)
	battleResult := corebattle.Result{
		Outcome:  corebattle.OutcomeWin,
		WinnerID: "p1",
		LoserID:  "p2",
		Turns:    4,
	}

	replayID, err := service.RecordParticipantVsParticipant(ctx, replay.CombatTypeChallenge, p1, "Challenger", p2, "Wave Boss", battleResult)
	if err != nil {
		t.Fatalf("RecordParticipantVsParticipant: %v", err)
	}

	rep, err := service.GetReplay(ctx, replayID)
	if err != nil {
		t.Fatalf("GetReplay: %v", err)
	}
	if rep.InitiatorName != "Challenger" || rep.OpponentName != "Wave Boss" || rep.CombatType != replay.CombatTypeChallenge {
		t.Errorf("unexpected replay: %+v", rep)
	}
}

func TestReplayRecorder_InterfaceCompliance(t *testing.T) {
	repo := newMockReplayRepo()
	service, err := replay.NewService(repo)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	var _ replay.ReplayRecorder = service
}
