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
	prizes        map[string]int
}

func newTransactionalCharRepo() *transactionalCharRepo {
	return &transactionalCharRepo{
		characters:    make(map[string]corecharacter.Character),
		failUpdateIDs: make(map[string]error),
		prizes:        make(map[string]int),
	}
}

func (r *transactionalCharRepo) HoldPrize(_ context.Context, characterID, _ string, amount int) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.prizes[characterID] += amount
	return nil
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
	prizes := make(map[string]int, len(r.prizes))
	for k, v := range r.prizes {
		prizes[k] = v
	}
	r.mu.Unlock()

	err := fn(ctx)
	if err != nil {
		r.mu.Lock()
		r.characters = snapshot
		r.prizes = prizes
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
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "ColosseumDisbandTest",
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

	// Leader leaves recruiting room -> refunds only leader (char-3), hands leadership to first remaining member on retry.
	err = svc.LeaveRoom(ctx, c1.ID, detail.Room.ID)
	if err == nil {
		t.Fatal("expected error on LeaveRoom, got nil")
	}
	if !errors.Is(err, errDBFailure) {
		t.Fatalf("expected error wrapping %v, got %v", errDBFailure, err)
	}

	// Verify atomic rollback: no character should have received a refund
	for _, id := range []string{c1.ID, c2.ID, c3.ID} {
		c, _ := charRepo.FindByID(ctx, id)
		if c.Money != 400 {
			t.Fatalf("expected 400G for %s after rollback, got %d", id, c.Money)
		}
	}

	// Verify room is NOT deleted or modified
	roomDetail, err := svc.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatalf("expected room to still exist after failed refund, got %v", err)
	}
	if roomDetail.Room.Status != pvp.StatusRecruiting || roomDetail.Room.LeaderCharacterID != c1.ID {
		t.Fatalf("expected room still recruiting with original leader, got %+v", roomDetail.Room)
	}

	// Only the departing character is locked and refunded
	calls := charRepo.getLockCalls()
	if len(calls) != 1 || calls[0] != "char-3" {
		t.Fatalf("expected single lock call [char-3], got %v", calls)
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
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "ColosseumNonLeaderTest",
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
			Defeats: []corebattle.PartyBattleDefeat{
				{ActorID: c1.ID, TargetID: c3.ID},
				{ActorID: c2.ID, TargetID: c3.ID},
			},
		},
	}

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		battleEngine,
		pvp.WithTransactionProvider(charRepo),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "PrizeRollbackTest",
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

	if len(charRepo.prizes) != 0 {
		t.Fatalf("rollback left held prizes: %v", charRepo.prizes)
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

func TestSettlement_AdvanceRound_MaxRoundsDraw_NoRefund(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()
	transRepo := pvp.NewMemoryTransitionRepository()

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
		pvp.WithTransitionRepository(transRepo),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "MaxRoundsDrawTest",
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
	charRepo.resetLockCalls()

	res, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID)
	if err != nil {
		t.Fatalf("unexpected error on AdvanceRound draw: %v", err)
	}
	if !res.MatchCompleted || res.Outcome != "match_draw" {
		t.Fatalf("expected completed match_draw, got %+v", res)
	}

	// Verify no refunds occur and no wallet updates happen on max-round draw
	for _, id := range []string{"char-1", "char-2"} {
		ch, _ := charRepo.FindByID(ctx, id)
		if ch.Money != 400 {
			t.Fatalf("expected %s money 400 (no refund on draw), got %d", id, ch.Money)
		}
	}
	if calls := charRepo.getLockCalls(); len(calls) != 0 {
		t.Fatalf("expected zero wallet lock calls on draw, got %v", calls)
	}
	tr, err := transRepo.FindTransition(ctx, detail.Room.ID)
	if err != nil || tr.ForfeitedPool != 200 || tr.Detail.Room.PrizePool != 0 {
		t.Fatalf("expected 200G forfeited escrow recorded, got tr=%+v err=%v", tr, err)
	}
}

func TestSettlement_HappyPath_AscendingLockOrder_AndCommit(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-3", "RedLeader", 500)
	c2 := createTestCharacter("char-1", "RedWing", 500)
	c3 := createTestCharacter("char-2", "BlueFoe", 500)
	charRepo.add(c1)
	charRepo.add(c2)
	charRepo.add(c3)

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{
			Outcome:    corebattle.OutcomeWin,
			WinnerSide: pvp.ColorRed,
			WinnerTeam: pvp.ColorRed,
			Turns:      1,
		}},
		pvp.WithTransactionProvider(charRepo),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}

	detail, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{
		Name:       "HappyPathSettlement",
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
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	charRepo.resetLockCalls()

	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}

	// Winners receive 150G held prize each and remain at 400G wallet
	for _, id := range []string{c1.ID, c2.ID} {
		if charRepo.prizes[id] != 150 {
			t.Fatalf("expected 150G held prize for %s, got %d", id, charRepo.prizes[id])
		}
	}

	// Lock calls must be in ascending order (char-1 before char-3)
	calls := charRepo.getLockCalls()
	if len(calls) != 2 || !sort.StringsAreSorted(calls) {
		t.Fatalf("lock calls were not sorted ascending: %v", calls)
	}
}

func TestSettlement_LeaveForfeit_FlushesPendingPvPKills(t *testing.T) {
	ctx := context.Background()
	charRepo := newTransactionalCharRepo()
	roomRepo := pvp.NewMemoryRoomRepository()

	c1 := createTestCharacter("char-b", "LeaderB", 500)
	c2 := createTestCharacter("char-a", "MemberA", 500)
	charRepo.add(c1)
	charRepo.add(c2)

	svc, err := pvp.NewService(
		roomRepo,
		charRepo,
		mockBattleEngine{result: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeDraw,
			Turns:       2,
			RemainingHP: map[string]int{c1.ID: 50, c2.ID: 50},
			Defeats:     []corebattle.PartyBattleDefeat{{ActorID: c2.ID, TargetID: c1.ID}},
		}},
		pvp.WithTransactionProvider(charRepo),
		pvp.WithPrizeRemitter(charRepo),
		pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
	)
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	detail, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "ForfeitKills", Bet: 100, MaxMembers: 2, TargetWins: 2})
	_, _ = svc.JoinRoom(ctx, c2.ID, detail.Room.ID, "")
	_, _ = svc.SelectTeam(ctx, c1.ID, detail.Room.ID, pvp.ColorRed)
	_, _ = svc.SelectTeam(ctx, c2.ID, detail.Room.ID, pvp.ColorBlue)
	_, _ = svc.StartMatch(ctx, c1.ID, detail.Room.ID)

	if _, err := svc.AdvanceRound(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	// Member leaves mid-match (kills stay pending on room), then remaining leader leaves -> disbanded & flushes c2's kill_p.
	if err := svc.LeaveRoom(ctx, c2.ID, detail.Room.ID); err != nil {
		t.Fatalf("member LeaveRoom failed: %v", err)
	}
	if err := svc.LeaveRoom(ctx, c1.ID, detail.Room.ID); err != nil {
		t.Fatalf("leader LeaveRoom failed: %v", err)
	}

	gotC2, _ := charRepo.FindByID(ctx, c2.ID)
	if gotC2.PvPWins != 1 {
		t.Fatalf("expected flushed PvPWins=1 for %s after match disband, got %d", c2.ID, gotC2.PvPWins)
	}
}
