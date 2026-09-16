package pvp_test

import (
	"context"
	"errors"
	"sort"
	"sync"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type transactionalCharRepo struct {
	mu            sync.Mutex
	characters    map[string]corecharacter.Character
	failUpdateIDs map[string]error
	lockCalls     []string
}

func newTransactionalCharRepo() *transactionalCharRepo {
	return &transactionalCharRepo{
		characters:    make(map[string]corecharacter.Character),
		failUpdateIDs: make(map[string]error),
	}
}

func (r *transactionalCharRepo) add(c corecharacter.Character) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.characters[c.ID] = c
}

func (r *transactionalCharRepo) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *transactionalCharRepo) FindByIDForUpdate(_ context.Context, id string) (corecharacter.Character, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lockCalls = append(r.lockCalls, id)
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (r *transactionalCharRepo) Update(_ context.Context, c corecharacter.Character) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err, ok := r.failUpdateIDs[c.ID]; ok {
		return err
	}
	r.characters[c.ID] = c
	return nil
}

func (r *transactionalCharRepo) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	r.mu.Lock()
	snapshot := make(map[string]corecharacter.Character, len(r.characters))
	for k, v := range r.characters {
		snapshot[k] = v
	}
	r.mu.Unlock()

	err := fn(ctx)
	if err != nil {
		r.mu.Lock()
		r.characters = snapshot
		r.mu.Unlock()
		return err
	}
	return nil
}

func (r *transactionalCharRepo) resetLockCalls() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lockCalls = nil
}

func (r *transactionalCharRepo) getLockCalls() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	copied := make([]string, len(r.lockCalls))
	copy(copied, r.lockCalls)
	return copied
}

func TestSettlement_LeaveRoom_LeaderDisband_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-3", "Leader", 500)
	c2 := createTestCharacter("char-1", "MemberA", 500)
	c3 := createTestCharacter("char-2", "MemberB", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}},
		pvp.WithTransactionProvider(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Colosseum Disband Test",
		Bet:        100,
		MaxMembers: 3,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom c2 failed: %v", err)
	}
	_, err = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom c3 failed: %v", err)
	}

	// Each character should have 400G remaining
	for _, id := range []string{c1.ID, c2.ID, c3.ID} {
		c, _ := charRepo.FindByID(ctx, id)
		if c.Money != 400 {
			t.Fatalf("expected 400G for %s, got %d", id, c.Money)
		}
	}

	// Simulate persistence failure when updating char-3 (Leader)
	errDBFailure := errors.New("simulated database disk error")
	charRepo.failUpdateIDs["char-3"] = errDBFailure
	charRepo.resetLockCalls()

	// Leader leaves room -> triggers disband and refund
	err = svc.LeaveRoom(ctx, c1.ID, detail.Room.ID)
	if err == nil {
		t.Fatal("expected error on LeaveRoom, got nil")
	}
	if !errors.Is(err, errDBFailure) {
		t.Fatalf("expected error wrapping %v, got %v", errDBFailure, err)
	}

	// Verify atomic rollback: no character should have received a partial refund
	for _, id := range []string{c1.ID, c2.ID, c3.ID} {
		c, _ := charRepo.FindByID(ctx, id)
		if c.Money != 400 {
			t.Fatalf("expected 400G for %s after rollback, got %d", id, c.Money)
		}
	}

	// Verify room is NOT deleted or disbanded
	roomDetail, err := svc.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatalf("expected room to still exist after failed refund, got %v", err)
	}
	if roomDetail.Room.Status != pvp.StatusRecruiting {
		t.Fatalf("expected room to still be recruiting, got %v", roomDetail.Room.Status)
	}

	// Verify Rank 2 lock ordering: characters locked ascending by ID
	calls := charRepo.getLockCalls()
	expectedOrder := []string{"char-1", "char-2", "char-3"}
	if len(calls) < len(expectedOrder) {
		t.Fatalf("expected at least %d lock calls, got %v", len(expectedOrder), calls)
	}
	for i, exp := range expectedOrder {
		if calls[i] != exp {
			t.Fatalf("expected lock call %d to be %s, got %s (full calls: %v)", i, exp, calls[i], calls)
		}
	}
}

func TestSettlement_LeaveRoom_NonLeader_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-1", "Leader", 500)
	c2 := createTestCharacter("char-2", "Member", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}},
		pvp.WithTransactionProvider(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Colosseum NonLeader Test",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, err = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	if err != nil {
		t.Fatalf("JoinRoom c2 failed: %v", err)
	}

	// Fail update for c2 on leave
	errDB := errors.New("simulated lock timeout")
	charRepo.failUpdateIDs[c2.ID] = errDB

	err = svc.LeaveRoom(ctx, c2.ID, detail.Room.ID)
	if err == nil {
		t.Fatal("expected error on LeaveRoom, got nil")
	}

	// Money must still be 400G (not refunded)
	c2After, _ := charRepo.FindByID(ctx, c2.ID)
	if c2After.Money != 400 {
		t.Fatalf("expected 400G after rollback, got %d", c2After.Money)
	}

	// Room must still retain c2 as member and prize pool 200G
	r, err := svc.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatalf("failed to get room: %v", err)
	}
	if len(r.Members) != 2 {
		t.Fatalf("expected 2 members still in room, got %d", len(r.Members))
	}
	if r.Room.PrizePool != 200 {
		t.Fatalf("expected prize pool 200, got %d", r.Room.PrizePool)
	}
}

func TestSettlement_AdvanceRound_PrizeDistribution_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-b", "RedLeader", 500)
	c2 := createTestCharacter("char-a", "RedMember", 500)
	c3 := createTestCharacter("char-c", "BlueMember", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome:    corebattle.OutcomeWin,
			WinnerSide: pvp.ColorRed,
			WinnerTeam: pvp.ColorRed,
			Turns:      1,
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		battleEngine,
		pvp.WithTransactionProvider(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Prize Rollback Test",
		Bet:        100,
		MaxMembers: 3,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, c3.ID, detail.Room.ID, "")

	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c3.ID, detail.Room.ID, pvp.ColorBlue)

	_, err = svc.StartMatch(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("StartMatch failed: %v", err)
	}

	// Fail update on winner char-b
	errDeadlock := errors.New("simulated transaction deadlock")
	charRepo.failUpdateIDs["char-b"] = errDeadlock
	charRepo.resetLockCalls()

	_, err = svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err == nil {
		t.Fatal("expected error on AdvanceRound, got nil")
	}

	// Assert atomic rollback: winner char-a must not have received prize or PvPWins
	charA, _ := charRepo.FindByID(ctx, "char-a")
	if charA.Money != 400 {
		t.Fatalf("expected char-a money 400 after rollback, got %d", charA.Money)
	}
	if charA.PvPWins != 0 {
		t.Fatalf("expected char-a PvPWins 0 after rollback, got %d", charA.PvPWins)
	}

	// Room should not be marked as completed, prize pool must remain 300
	r, _ := svc.GetRoom(ctx, detail.Room.ID)
	if r.Room.Status == pvp.StatusCompleted {
		t.Fatal("room should not be completed after transaction failure")
	}
	if r.Room.PrizePool != 300 {
		t.Fatalf("expected prize pool 300, got %d", r.Room.PrizePool)
	}

	// Verify ascending lock order for winners: char-a before char-b
	calls := charRepo.getLockCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 lock calls, got %v", calls)
	}
	if calls[0] != "char-a" || calls[1] != "char-b" {
		t.Fatalf("expected lock order [char-a, char-b], got %v", calls)
	}
}

func TestSettlement_AdvanceRound_DrawRefund_RollbackOnFailure(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-2", "MemberA", 500)
	c2 := createTestCharacter("char-1", "MemberB", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	// Draw result
	battleEngine := mockBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome: corebattle.OutcomeDraw,
			Turns:   1,
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		battleEngine,
		pvp.WithTransactionProvider(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Draw Rollback Test",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	// Advance room round directly to MaxRounds (10)
	r, _ := svc.GetRoom(ctx, detail.Room.ID)
	r.Room.Round = pvp.MaxRounds
	_ = roomRepo.SaveRoom(ctx, r.Room, r.Members)

	// Fail update on char-2
	errDrawFail := errors.New("simulated draw failure")
	charRepo.failUpdateIDs["char-2"] = errDrawFail
	charRepo.resetLockCalls()

	_, err = svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err == nil {
		t.Fatal("expected error on AdvanceRound draw, got nil")
	}

	// Verify rollback: char-1 money unchanged
	char1, _ := charRepo.FindByID(ctx, "char-1")
	if char1.Money != 400 {
		t.Fatalf("expected char-1 money 400 after rollback, got %d", char1.Money)
	}

	// Verify lock order was ascending: char-1 before char-2
	calls := charRepo.getLockCalls()
	if len(calls) < 2 {
		t.Fatalf("expected at least 2 lock calls, got %v", calls)
	}
	if calls[0] != "char-1" || calls[1] != "char-2" {
		t.Fatalf("expected lock order [char-1, char-2], got %v", calls)
	}
}

func TestSettlement_HappyPath_AscendingLockOrder_AndCommit(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-3", "Leader", 500)
	c2 := createTestCharacter("char-1", "MemberA", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin}},
		pvp.WithTransactionProvider(charRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "Happy Path Disband",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: 1,
	})
	if err != nil {
		t.Fatalf("CreateRoom failed: %v", err)
	}

	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")

	charRepo.resetLockCalls()

	// Leader leaves -> refunds both members (100G each)
	err = svc.LeaveRoom(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("LeaveRoom failed: %v", err)
	}

	// Both characters should have 500G restored
	c1After, _ := charRepo.FindByID(ctx, c1.ID)
	c2After, _ := charRepo.FindByID(ctx, c2.ID)
	if c1After.Money != 500 || c2After.Money != 500 {
		t.Fatalf("expected both characters to have 500G, got c1=%d, c2=%d", c1After.Money, c2After.Money)
	}

	// Room should be deleted
	_, err = svc.GetRoom(ctx, detail.Room.ID)
	if !errors.Is(err, pvp.ErrRoomNotFound) {
		t.Fatalf("expected ErrRoomNotFound, got %v", err)
	}

	// Lock calls must be in ascending order
	calls := charRepo.getLockCalls()
	if !sort.StringsAreSorted(calls) {
		t.Fatalf("lock calls were not sorted ascending: %v", calls)
	}
}
