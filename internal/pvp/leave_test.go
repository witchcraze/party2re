package pvp_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/pvp"
)

type recordingSettler struct {
	requests []battle.ApplyPostBattleRequest
	err      error
}

func (r *recordingSettler) ApplyPostBattleResult(_ context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	if r.err != nil {
		return battle.ApplyPostBattleResponse{}, r.err
	}
	r.requests = append(r.requests, req)
	return battle.ApplyPostBattleResponse{}, nil
}

type statefulParticipantMock struct {
	calls []struct {
		id       string
		hp       int
		mp       *int
		consumed []corebattle.ConsumedItem
	}
}

func (m *statefulParticipantMock) BuildParticipantWithCurrentHP(_ context.Context, characterID string, currentHP int) (corebattle.Participant, error) {
	return corebattle.MustNewParticipant(characterID, currentHP, 20, 10), nil
}

func (m *statefulParticipantMock) BuildBattleMaxima(_ context.Context, char corecharacter.Character) (int, int, error) {
	return char.Stats.MaxHP, char.Stats.MaxMP, nil
}

func (m *statefulParticipantMock) BuildParticipantWithState(_ context.Context, characterID string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error) {
	m.calls = append(m.calls, struct {
		id       string
		hp       int
		mp       *int
		consumed []corebattle.ConsumedItem
	}{characterID, hp, mp, slices.Clone(consumed)})
	p := corebattle.MustNewParticipant(characterID, hp, 20, 10)
	p.MaxHP = 100
	p.MaxMP = 50
	if mp != nil {
		p.MP = *mp
	}
	return p, nil
}

type recordingActionWait struct {
	waits map[string]time.Time
	err   error
}

func (r *recordingActionWait) ImposeActionWaitUntil(_ context.Context, characterID string, until time.Time) error {
	if r.err != nil {
		return r.err
	}
	if r.waits == nil {
		r.waits = make(map[string]time.Time)
	}
	r.waits[characterID] = until
	return nil
}

type failingRoomRepo struct {
	pvp.RoomRepository
	failDeleteRoom bool
	failDeleteChar bool
	failSaveRoom   bool
}

func (r *failingRoomRepo) SaveRoom(ctx context.Context, room pvp.ColosseumRoom, members []pvp.RoomMember) error {
	if r.failSaveRoom {
		return errors.New("save room failure")
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func (r *failingRoomRepo) DeleteRoom(ctx context.Context, id string) error {
	if r.failDeleteRoom {
		return errors.New("delete room failure")
	}
	return r.RoomRepository.DeleteRoom(ctx, id)
}

func (r *failingRoomRepo) DeleteCharacterRoom(ctx context.Context, characterID, roomID string) error {
	if r.failDeleteChar {
		return errors.New("delete character room failure")
	}
	return r.RoomRepository.DeleteCharacterRoom(ctx, characterID, roomID)
}

type leaveScriptedBattleEngine struct {
	fn func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error)
}

func (e leaveScriptedBattleEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	return e.fn(req)
}

func TestPvPLeaveSideEffects(t *testing.T) {
	ctx := context.Background()

	t.Run("pre-start leave calls settler with round 0 fatigue and postExitHook", func(t *testing.T) {
		charRepo := newMockCharRepo()
		roomRepo := pvp.NewMemoryRoomRepository()
		settler := &recordingSettler{}
		var hookCalls []string
		svc, err := pvp.NewService(
			roomRepo, charRepo, mockBattleEngine{},
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithPostBattleSettler(settler),
			pvp.WithPostExitHook(func(_ context.Context, characterID string) (*pvp.DeliveryOutcome, error) {
				hookCalls = append(hookCalls, characterID)
				return &pvp.DeliveryOutcome{ItemName: "soup", Paid: true}, nil
			}),
		)
		if err != nil {
			t.Fatal(err)
		}
		c1 := createTestCharacter("c1", "Alice", 500)
		c1.Stats.HP, c1.Stats.MP, c1.Stats.MaxMP = 80, 12, 30
		charRepo.add(c1)

		room, err := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "PreStart", Bet: 50})
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.LeaveRoom(ctx, c1.ID, room.Room.ID)
		if err != nil {
			t.Fatalf("LeaveRoom: %v", err)
		}
		if res.RefundedBet != 50 || res.Delivery == nil || res.Delivery.ItemName != "soup" || !res.Delivery.Paid {
			t.Fatalf("LeaveRoom result = %+v", res)
		}
		if len(settler.requests) != 1 {
			t.Fatalf("settler calls = %d, want 1", len(settler.requests))
		}
		req := settler.requests[0]
		if req.ExitFatigue != 1 || req.ExitBonusRounds != 0 || !req.PreserveEarnedRewards {
			t.Fatalf("unexpected settler request: %+v", req)
		}
		if req.BattleResult.RemainingHP[c1.ID] != 80 || req.BattleResult.RemainingMP[c1.ID] != 12 {
			t.Fatalf("unexpected vitality copyback: HP=%v MP=%v", req.BattleResult.RemainingHP, req.BattleResult.RemainingMP)
		}
		if !slices.Equal(hookCalls, []string{c1.ID}) {
			t.Fatalf("postExitHook calls = %v", hookCalls)
		}
		got, _ := charRepo.FindByID(ctx, c1.ID)
		if got.Money != 500 {
			t.Fatalf("expected bet refunded to 500, got %d", got.Money)
		}
	})

	t.Run("multi-round consumed items and final-round 0 HP copyback reach LeaveRoom", func(t *testing.T) {
		charRepo := newMockCharRepo()
		roomRepo := pvp.NewMemoryRoomRepository()
		builder := &statefulParticipantMock{}
		settler := &recordingSettler{}
		roundCalls := 0
		engine := leaveScriptedBattleEngine{fn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			roundCalls++
			if roundCalls == 1 {
				return corebattle.PartyBattleResult{
					Outcome:     corebattle.OutcomeDraw,
					RemainingHP: map[string]int{"c1": 50, "c2": 60},
					RemainingMP: map[string]int{"c1": 15, "c2": 20},
					ConsumedItems: map[string][]corebattle.ConsumedItem{
						"c1": {{ID: "item-101", Quantity: 1}},
					},
				}, nil
			}
			return corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 70, "c2": 0},
				RemainingMP: map[string]int{"c1": 10, "c2": 5},
			}, nil
		}}
		svc, err := pvp.NewService(
			roomRepo, charRepo, engine,
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithParticipantBuilder(builder),
			pvp.WithPostBattleSettler(settler),
			pvp.WithPrizeRemitter(charRepo),
		)
		if err != nil {
			t.Fatal(err)
		}
		c1 := createTestCharacter("c1", "Alice", 500)
		c1.Stats.MP, c1.Stats.MaxMP = 30, 50
		c2 := createTestCharacter("c2", "Bob", 500)
		c2.Stats.MP, c2.Stats.MaxMP = 40, 50
		charRepo.add(c1)
		charRepo.add(c2)

		room, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Multi", Bet: 50, TargetWins: 1})
		_, _ = svc.JoinRoom(ctx, c2.ID, room.Room.ID, "")
		_, _ = svc.SelectTeam(ctx, c1.ID, room.Room.ID, pvp.ColorRed)
		_, _ = svc.SelectTeam(ctx, c2.ID, room.Room.ID, pvp.ColorBlue)
		_, _ = svc.StartMatch(ctx, c1.ID, room.Room.ID)

		if _, err := svc.AdvanceRound(ctx, c1.ID, room.Room.ID); err != nil {
			t.Fatalf("round 1 draw: %v", err)
		}
		res2, err := svc.AdvanceRound(ctx, c1.ID, room.Room.ID)
		if err != nil {
			t.Fatalf("round 2 win: %v", err)
		}
		if !res2.MatchCompleted {
			t.Fatalf("expected match completed on round 2")
		}
		// Round 2 builder call for c1 must see round 1 consumed items.
		var sawRound2Consumed bool
		for _, call := range builder.calls {
			if call.id == "c1" && len(call.consumed) == 1 && call.consumed[0].ID == "item-101" {
				sawRound2Consumed = true
			}
		}
		if !sawRound2Consumed {
			t.Fatalf("expected round 2 builder to receive c1 consumed items, got %+v", builder.calls)
		}

		if res, err := svc.LeaveRoom(ctx, c1.ID, room.Room.ID); err != nil || res.RefundedBet != 0 {
			t.Fatalf("c1 LeaveRoom = %+v, %v; want no refund after the start", res, err)
		}
		if _, err := svc.LeaveRoom(ctx, c2.ID, room.Room.ID); err != nil {
			t.Fatalf("c2 LeaveRoom: %v", err)
		}
		if len(settler.requests) != 2 {
			t.Fatalf("settler calls = %d, want 2", len(settler.requests))
		}
		r1 := settler.requests[0]
		if r1.ExitFatigue != 7 || r1.ExitBonusRounds != 2 || r1.BattleResult.RemainingHP["c1"] != 70 || r1.BattleResult.RemainingMP["c1"] != 10 || len(r1.BattleResult.ConsumedItems["c1"]) != 1 {
			t.Fatalf("unexpected c1 exit settler request: %+v", r1)
		}
		r2 := settler.requests[1]
		if r2.ExitFatigue != 7 || r2.ExitBonusRounds != 2 || r2.BattleResult.RemainingHP["c2"] != 0 || r2.BattleResult.RemainingMP["c2"] != 5 {
			t.Fatalf("unexpected c2 exit settler request: %+v", r2)
		}
	})

	t.Run("timeout and missing-room abandon do not apply exit fatigue or hooks", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0).UTC()
		charRepo := newMockCharRepo()
		roomRepo := pvp.NewMemoryRoomRepository()
		settler := &recordingSettler{}
		hookCalls := 0
		timers := &stubTimerService{}
		svc, err := pvp.NewService(
			roomRepo, charRepo, mockBattleEngine{},
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithPostBattleSettler(settler),
			pvp.WithPostExitHook(func(context.Context, string) (*pvp.DeliveryOutcome, error) { hookCalls++; return nil, nil }),
			pvp.WithTimerService(timers),
			pvp.WithClock(func() time.Time { return now }),
		)
		if err != nil {
			t.Fatal(err)
		}
		c1 := createTestCharacter("c1", "Alice", 500)
		charRepo.add(c1)
		room, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Lost", Bet: 50})
		_ = roomRepo.DeleteRoom(ctx, room.Room.ID)
		if _, err := svc.LeaveRoom(ctx, c1.ID, room.Room.ID); err != nil {
			t.Fatalf("abandonLostRoom: %v", err)
		}
		if len(settler.requests) != 0 || hookCalls != 0 {
			t.Fatalf("abandonLostRoom must not run exit side-effects: settler=%d hook=%d", len(settler.requests), hookCalls)
		}

		c2 := createTestCharacter("c2", "Bob", 500)
		charRepo.add(c2)
		room2, _ := svc.CreateRoom(ctx, c2.ID, pvp.CreateRoomRequest{Name: "Timeout", Bet: 50})
		now = now.Add(pvp.InactivityTimeout + time.Second)
		if _, err := svc.LeaveRoom(ctx, c2.ID, room2.Room.ID); err != nil {
			t.Fatalf("timeout LeaveRoom: %v", err)
		}
		if len(settler.requests) != 0 || hookCalls != 0 {
			t.Fatalf("timeout must not run exit side-effects: settler=%d hook=%d", len(settler.requests), hookCalls)
		}
	})
}

func TestPvPKickMember(t *testing.T) {
	ctx := context.Background()

	t.Run("leader kick removes target without bet refund or exit hooks and sets 60s wait", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0).UTC()
		charRepo := newMockCharRepo()
		roomRepo := pvp.NewMemoryRoomRepository()
		settler := &recordingSettler{}
		waits := &recordingActionWait{}
		hookCalls := 0
		svc, err := pvp.NewService(
			roomRepo, charRepo, mockBattleEngine{},
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithPostBattleSettler(settler),
			pvp.WithPostExitHook(func(context.Context, string) (*pvp.DeliveryOutcome, error) { hookCalls++; return nil, nil }),
			pvp.WithActionWait(waits),
			pvp.WithClock(func() time.Time { return now }),
		)
		if err != nil {
			t.Fatal(err)
		}
		leader := createTestCharacter("c1", "Alice", 500)
		peer := createTestCharacter("c2", "Bob", 500)
		outsider := createTestCharacter("c3", "Carol", 500)
		charRepo.add(leader)
		charRepo.add(peer)
		charRepo.add(outsider)

		room, _ := svc.CreateRoom(ctx, leader.ID, pvp.CreateRoomRequest{Name: "Lobby", Bet: 100})
		_, _ = svc.JoinRoom(ctx, peer.ID, room.Room.ID, "")

		if _, err := svc.KickMember(ctx, room.Room.ID, peer.ID, leader.ID); !errors.Is(err, pvp.ErrNotRoomLeader) {
			t.Fatalf("non-leader kick = %v, want ErrNotRoomLeader", err)
		}
		if _, err := svc.KickMember(ctx, room.Room.ID, leader.ID, leader.ID); !errors.Is(err, pvp.ErrCannotKickSelf) {
			t.Fatalf("self-kick = %v, want ErrCannotKickSelf", err)
		}
		if _, err := svc.KickMember(ctx, room.Room.ID, leader.ID, outsider.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
			t.Fatalf("outsider kick = %v, want ErrCharacterNotInRoom", err)
		}

		res, err := svc.KickMember(ctx, room.Room.ID, leader.ID, peer.ID)
		if err != nil {
			t.Fatalf("KickMember: %v", err)
		}
		if res.RoomName != "Lobby" || res.LeaderName != "Alice" || res.TargetName != "Bob" {
			t.Fatalf("KickResult = %+v, want RoomName=Lobby LeaderName=Alice TargetName=Bob", res)
		}

		gotPeer, _ := charRepo.FindByID(ctx, peer.ID)
		if gotPeer.Money != 400 || gotPeer.Tired != 0 {
			t.Fatalf("kicked peer must keep no refund and 0 fatigue, got money=%d tired=%d", gotPeer.Money, gotPeer.Tired)
		}
		if len(settler.requests) != 0 || hookCalls != 0 {
			t.Fatalf("kick must not trigger exit side-effects: settler=%d hook=%d", len(settler.requests), hookCalls)
		}
		if !waits.waits[peer.ID].Equal(now.Add(pvp.KickActionWait)) {
			t.Fatalf("peer wait = %v, want %v", waits.waits[peer.ID], now.Add(pvp.KickActionWait))
		}
		detail, err := svc.GetRoom(ctx, room.Room.ID)
		if err != nil || len(detail.Members) != 1 || detail.Room.PrizePool != 200 {
			t.Fatalf("expected prize pool 200 and 1 member, got %+v err=%v", detail, err)
		}
		if claim, err := svc.GetCharacterClaim(ctx, peer.ID); err != nil || claim != "" {
			t.Fatalf("expected peer claim released, got %q err=%v", claim, err)
		}
		if _, err := svc.GetCharacterRoom(ctx, peer.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("expected peer GetCharacterRoom ErrRoomNotFound, got %v", err)
		}
		if _, err := svc.LeaveRoom(ctx, peer.ID, room.Room.ID); !errors.Is(err, pvp.ErrCharacterNotInRoom) {
			t.Fatalf("kicked peer LeaveRoom = %v, want ErrCharacterNotInRoom", err)
		}
	})

	t.Run("cannot kick after match start", func(t *testing.T) {
		charRepo := newMockCharRepo()
		roomRepo := pvp.NewMemoryRoomRepository()
		svc, _ := pvp.NewService(roomRepo, charRepo, mockBattleEngine{}, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()))
		c1 := createTestCharacter("c1", "Alice", 500)
		c2 := createTestCharacter("c2", "Bob", 500)
		charRepo.add(c1)
		charRepo.add(c2)
		room, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Started", Bet: 50})
		_, _ = svc.JoinRoom(ctx, c2.ID, room.Room.ID, "")
		_, _ = svc.SelectTeam(ctx, c1.ID, room.Room.ID, pvp.ColorRed)
		_, _ = svc.SelectTeam(ctx, c2.ID, room.Room.ID, pvp.ColorBlue)
		_, _ = svc.StartMatch(ctx, c1.ID, room.Room.ID)

		if _, err := svc.KickMember(ctx, room.Room.ID, c1.ID, c2.ID); !errors.Is(err, pvp.ErrMatchAlreadyStarted) {
			t.Fatalf("post-start kick = %v, want ErrMatchAlreadyStarted", err)
		}
	})

	t.Run("kick retry after a pending wait failure replays the target name", func(t *testing.T) {
		charRepo := newMockCharRepo()
		waits := &recordingActionWait{err: errors.New("wait store down")}
		svc, _ := pvp.NewService(
			pvp.NewMemoryRoomRepository(), charRepo, mockBattleEngine{},
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithActionWait(waits),
		)
		c1 := createTestCharacter("c1", "Alice", 500)
		c2 := createTestCharacter("c2", "Bob", 500)
		charRepo.add(c1)
		charRepo.add(c2)
		room, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Replay", Bet: 50})
		_, _ = svc.JoinRoom(ctx, c2.ID, room.Room.ID, "")
		if _, err := svc.KickMember(ctx, room.Room.ID, c1.ID, c2.ID); err == nil {
			t.Fatal("expected KickMember error when wait write fails")
		}
		waits.err = nil
		res, err := svc.KickMember(ctx, room.Room.ID, c1.ID, c2.ID)
		if err != nil || res.TargetName != "Bob" || res.RoomName != "Replay" {
			t.Fatalf("replayed KickMember = %+v, %v", res, err)
		}
	})

	t.Run("pending kick wait failure preserves claim and recovers via RecoverCharacterDeparture", func(t *testing.T) {
		now := time.Unix(1_700_000_000, 0).UTC()
		charRepo := newMockCharRepo()
		baseRooms := pvp.NewMemoryRoomRepository()
		rooms := &failingRoomRepo{RoomRepository: baseRooms}
		waits := &recordingActionWait{err: errors.New("wait store down")}
		svc, _ := pvp.NewService(
			rooms, charRepo, mockBattleEngine{},
			pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()),
			pvp.WithActionWait(waits),
			pvp.WithClock(func() time.Time { return now }),
		)
		c1 := createTestCharacter("c1", "Alice", 500)
		c2 := createTestCharacter("c2", "Bob", 500)
		charRepo.add(c1)
		charRepo.add(c2)
		room, _ := svc.CreateRoom(ctx, c1.ID, pvp.CreateRoomRequest{Name: "Recover", Bet: 50})
		_, _ = svc.JoinRoom(ctx, c2.ID, room.Room.ID, "")

		if _, err := svc.KickMember(ctx, room.Room.ID, c1.ID, c2.ID); err == nil {
			t.Fatal("expected KickMember error when wait write fails")
		}
		if claim, err := svc.GetCharacterClaim(ctx, c2.ID); err != nil || claim != room.Room.ID {
			t.Fatalf("claim must remain reserved until wait write succeeds: %q %v", claim, err)
		}
		if pending, err := svc.HasPendingKickWait(ctx, c2.ID); err != nil || !pending {
			t.Fatalf("HasPendingKickWait = %v, %v; want true", pending, err)
		}
		// Simulate stale reverse mapping (DeleteCharacterRoom hadn't completed or raced): GetCharacterRoom must not fail membership mismatch.
		if _, err := svc.GetCharacterRoom(ctx, c2.ID); !errors.Is(err, pvp.ErrRoomNotFound) {
			t.Fatalf("GetCharacterRoom after SaveRoom = %v, want ErrRoomNotFound", err)
		}

		waits.err = nil
		if err := svc.RecoverCharacterDeparture(ctx, c2.ID); err != nil {
			t.Fatalf("RecoverCharacterDeparture: %v", err)
		}
		if !waits.waits[c2.ID].Equal(now.Add(pvp.KickActionWait)) {
			t.Fatalf("recovered wait = %v, want %v", waits.waits[c2.ID], now.Add(pvp.KickActionWait))
		}
		if claim, err := svc.GetCharacterClaim(ctx, c2.ID); err != nil || claim != "" {
			t.Fatalf("expected claim released after recovery, got %q err=%v", claim, err)
		}
	})
}

type stubTimerService struct{}

func (stubTimerService) ExtendLockUntil(context.Context, string, string, time.Time) error {
	return nil
}
func (stubTimerService) IsLocked(context.Context, string, string) (bool, error) { return false, nil }
