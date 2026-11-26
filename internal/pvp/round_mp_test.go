package pvp_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type mpSnapshotBuilder struct {
	chars *mockCharRepo
}

func (b mpSnapshotBuilder) BuildParticipantWithCurrentHP(ctx context.Context, id string, hp int) (corebattle.Participant, error) {
	c, err := b.chars.FindByID(ctx, id)
	if err != nil {
		return corebattle.Participant{}, err
	}
	return corebattle.Participant{
		ID:      c.ID,
		Name:    c.Name,
		HP:      hp,
		MaxHP:   c.Stats.MaxHP,
		MP:      c.Stats.MP,
		MaxMP:   c.Stats.MaxMP,
		Attack:  c.Stats.Attack,
		Defense: c.Stats.Defense,
		Agility: c.Stats.Agility,
	}, nil
}

func (mpSnapshotBuilder) BuildBattleMaxima(_ context.Context, char corecharacter.Character) (int, int, error) {
	if char.ID == "char-1" {
		return 200, 120, nil
	}
	return 100, 50, nil
}

type scriptedMPBattleEngine struct {
	calls    atomic.Int64
	requests []corebattle.PartyBattleRequest
	results  []corebattle.PartyBattleResult
}

func (e *scriptedMPBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	idx := int(e.calls.Add(1)) - 1
	e.requests = append(e.requests, req)
	if idx < len(e.results) {
		return e.results[idx], nil
	}
	return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}, nil
}

type failingSaveRoomRepo struct {
	pvp.RoomRepository
	failSave atomic.Bool
	err      error
}

func (r *failingSaveRoomRepo) SaveRoom(ctx context.Context, room pvp.ColosseumRoom, members []pvp.RoomMember) error {
	if r.failSave.CompareAndSwap(true, false) {
		return r.err
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func TestColosseum_CarryCurrentMPBetweenRounds(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 45, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 0, "char-2": 0},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 120, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 0, "char-2": 0},
			},
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		engine,
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithParticipantBuilder(mpSnapshotBuilder{chars: charRepo}),
	)
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("char-1", "Alice", 500)
	c1.Stats.HP, c1.Stats.MaxHP = 80, 100
	c1.Stats.MP, c1.Stats.MaxMP = 30, 40

	c2 := createTestCharacter("char-2", "Bob", 500)
	c2.Stats.HP, c2.Stats.MaxHP = 60, 100
	c2.Stats.MP, c2.Stats.MaxMP = 0, 20

	charRepo.add(c1)
	charRepo.add(c2)

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "MPCarry", Bet: 50, MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if detail.Members[0].MP == nil || *detail.Members[0].MP != 30 {
		t.Fatalf("leader entry MP = %v, want 30", detail.Members[0].MP)
	}
	if detail.Members[1].MP == nil || *detail.Members[1].MP != 0 {
		t.Fatalf("joiner entry MP = %v, want 0", detail.Members[1].MP)
	}

	// Mutate stored character MP after entry to prove rounds read room state rather than characters.
	c1Mut := c1
	c1Mut.Stats.MP = 99
	c2Mut := c2
	c2Mut.Stats.MP = 77
	_ = charRepo.Update(ctx, c1Mut)
	_ = charRepo.Update(ctx, c2Mut)

	if _, err := svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Members[0].HP != 200 || *started.Members[0].MP != 30 || started.Members[1].HP != 100 || *started.Members[1].MP != 0 {
		t.Fatalf("unexpected start state: %+v", started.Members)
	}

	res1, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if res1.MatchCompleted {
		t.Fatalf("round 1 unexpectedly completed match")
	}

	afterR1, err := svc.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterR1.Members[0].HP != 200 || afterR1.Members[0].MP == nil || *afterR1.Members[0].MP != 0 {
		t.Fatalf("after round 1 leader state = %+v (MP=%v), want HP=200 MP=0", afterR1.Members[0], afterR1.Members[0].MP)
	}
	if afterR1.Members[1].HP != 100 || afterR1.Members[1].MP == nil || *afterR1.Members[1].MP != 0 {
		t.Fatalf("after round 1 joiner state = %+v (MP=%v), want HP=100 MP=0", afterR1.Members[1], afterR1.Members[1].MP)
	}

	res2, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !res2.MatchCompleted {
		t.Fatalf("expected match completion on round 2")
	}
	if len(engine.requests) != 2 {
		t.Fatalf("expected 2 battle requests, got %d", len(engine.requests))
	}

	r1Ally, r1Enemy := engine.requests[0].Allies[0], engine.requests[0].Enemies[0]
	if r1Ally.HP != 200 || r1Ally.MaxHP != 200 || r1Ally.MP != 30 || r1Ally.MaxMP != 120 {
		t.Fatalf("round 1 ally mismatch: %+v", r1Ally)
	}
	if r1Enemy.HP != 100 || r1Enemy.MaxHP != 100 || r1Enemy.MP != 0 || r1Enemy.MaxMP != 50 {
		t.Fatalf("round 1 enemy mismatch: %+v", r1Enemy)
	}

	r2Ally, r2Enemy := engine.requests[1].Allies[0], engine.requests[1].Enemies[0]
	if r2Ally.HP != 200 || r2Ally.MaxHP != 200 || r2Ally.MP != 0 || r2Ally.MaxMP != 120 {
		t.Fatalf("round 2 ally mismatch (0 MP must persist and HP must heal): %+v", r2Ally)
	}
	if r2Enemy.HP != 100 || r2Enemy.MaxHP != 100 || r2Enemy.MP != 0 || r2Enemy.MaxMP != 50 {
		t.Fatalf("round 2 enemy mismatch: %+v", r2Enemy)
	}

	// Stored character MP must not be modified by PvP combat rounds.
	stored1, _ := charRepo.FindByID(ctx, c1.ID)
	stored2, _ := charRepo.FindByID(ctx, c2.ID)
	if stored1.Stats.MP != 99 || stored2.Stats.MP != 77 {
		t.Fatalf("characters MP mutated during PvP: c1=%d c2=%d", stored1.Stats.MP, stored2.Stats.MP)
	}
}

func TestColosseum_MPCarryOver_JSONAndLegacyRoomCompatibility(t *testing.T) {
	ctx := context.Background()

	// Verify 0 MP survives JSON round-trip while omitted MP remains nil.
	zero := 0
	raw, err := json.Marshal([]pvp.RoomMember{
		{CharacterID: "zero", MP: &zero},
		{CharacterID: "legacy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []pvp.RoomMember
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0].MP == nil || *decoded[0].MP != 0 {
		t.Fatalf("zero MP lost in JSON round-trip: %+v", decoded[0].MP)
	}
	if decoded[1].MP != nil {
		t.Fatalf("legacy member MP should remain nil, got %+v", decoded[1].MP)
	}

	charRepo := newMockCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 80, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 15, "char-2": 5},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 90, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 10, "char-2": 5},
			},
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		engine,
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithParticipantBuilder(mpSnapshotBuilder{chars: charRepo}),
	)
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("char-1", "Alice", 500)
	c1.Stats.MP, c1.Stats.MaxMP = 25, 40
	c2 := createTestCharacter("char-2", "Bob", 500)
	c2.Stats.MP, c2.Stats.MaxMP = 10, 20
	charRepo.add(c1)
	charRepo.add(c2)

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Legacy", Bet: 50, MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate an older pre-MP Valkey room snapshot where MP was not recorded.
	for i := range started.Members {
		started.Members[i].MP = nil
	}
	if err := roomRepo.SaveRoom(ctx, started.Room, started.Members); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if engine.requests[0].Allies[0].MP != 25 || engine.requests[0].Enemies[0].MP != 10 {
		t.Fatalf("legacy fallback MP mismatch: ally=%d enemy=%d", engine.requests[0].Allies[0].MP, engine.requests[0].Enemies[0].MP)
	}

	// Change DB MP before Round 2; Round 2 must now use Round 1's RemainingMP (15 / 5).
	c1.Stats.MP = 40
	c2.Stats.MP = 20
	_ = charRepo.Update(ctx, c1)
	_ = charRepo.Update(ctx, c2)

	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if engine.requests[1].Allies[0].MP != 15 || engine.requests[1].Enemies[0].MP != 5 {
		t.Fatalf("round 2 did not carry RemainingMP from legacy round 1: ally=%d enemy=%d", engine.requests[1].Allies[0].MP, engine.requests[1].Enemies[0].MP)
	}
}

func TestColosseum_MPCarryOver_FailureRecoveryDoesNotDoubleApply(t *testing.T) {
	ctx := context.Background()
	charRepo := newMockCharRepo()
	baseRooms := pvp.NewMemoryRoomRepository()
	roomRepo := &failingSaveRoomRepo{RoomRepository: baseRooms, err: errors.New("valkey down")}
	transRepo := pvp.NewMemoryTransitionRepository()

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			// Round 1 commits transition and fails at SaveRoom -> retry recovers Round 1 without rerunning battle.
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 50, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 20, "char-2": 0},
			},
			// Round 2 commits settle transition, fails at SaveRoom, then replays without rerunning battle.
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"char-1": 70, "char-2": 0},
				RemainingMP: map[string]int{"char-1": 5, "char-2": 0},
			},
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		engine,
		pvp.WithTransitionRepository(transRepo),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithParticipantBuilder(mpSnapshotBuilder{chars: charRepo}),
	)
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("char-1", "Alice", 500)
	c1.Stats.MP, c1.Stats.MaxMP = 30, 40
	c2 := createTestCharacter("char-2", "Bob", 500)
	c2.Stats.MP, c2.Stats.MaxMP = 0, 20
	charRepo.add(c1)
	charRepo.add(c2)

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "RetrySafe", Bet: 50, MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}

	// Simulate JoinRoom SQL commit succeeding while Valkey SaveRoom fails; recovery must preserve 0 MP.
	roomRepo.failSave.Store(true)
	if _, err := svc.JoinRoom(ctx, c2.ID, detail.Room.ID, ""); err == nil {
		t.Fatal("expected JoinRoom SaveRoom failure")
	}
	if _, err := svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.StartMatch(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatal(err)
	}

	// Non-final Round 1 SaveRoom failure must recover Round 1 and preserve RemainingMP=20/0 without rerunning battle.
	roomRepo.failSave.Store(true)
	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected round 1 SaveRoom failure")
	}
	res1, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("round 1 retry failed: %v", err)
	}
	if res1.MatchCompleted || res1.Round != 2 || engine.calls.Load() != 1 {
		t.Fatalf("expected recovered round 1 without extra combat call: calls=%d res=%+v", engine.calls.Load(), res1)
	}

	// Final Round 2 fails on Valkey publication after SQL settle commit.
	roomRepo.failSave.Store(true)
	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err == nil {
		t.Fatal("expected round 2 publish failure")
	}
	if engine.requests[1].Allies[0].MP != 20 || engine.requests[1].Enemies[0].MP != 0 {
		t.Fatalf("round 2 did not receive round 1 RemainingMP (20/0): got %d/%d", engine.requests[1].Allies[0].MP, engine.requests[1].Enemies[0].MP)
	}

	// Retry replays SQL result without rerunning combat or double-applying MP.
	replayed, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("round 2 replay failed: %v", err)
	}
	if !replayed.MatchCompleted || engine.calls.Load() != 2 {
		t.Fatalf("expected replayed completion without extra combat call: calls=%d res=%+v", engine.calls.Load(), replayed)
	}
	tr, err := transRepo.FindTransition(ctx, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Detail.Members[0].MP == nil || *tr.Detail.Members[0].MP != 5 {
		t.Fatalf("final transition did not preserve round 2 RemainingMP=5: %+v", tr.Detail.Members[0].MP)
	}
}
