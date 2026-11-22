package gvg_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
)

type failingBattleEngine struct {
	calls atomic.Int64
	err   error
}

func (e *failingBattleEngine) ResolvePartyBattle(_ corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls.Add(1)
	return corebattle.PartyBattleResult{}, e.err
}

type statefulMockBuilder struct {
	chars    *mockCharRepo
	consumed [][]corebattle.ConsumedItem
}

func (b *statefulMockBuilder) BuildParticipantWithCurrentHP(ctx context.Context, id string, hp int) (corebattle.Participant, error) {
	return b.BuildParticipantWithState(ctx, id, hp, nil, nil)
}

func (b *statefulMockBuilder) BuildBattleMaxima(_ context.Context, char corecharacter.Character) (int, int, error) {
	return char.Stats.MaxHP, char.Stats.MaxMP, nil
}

func (b *statefulMockBuilder) BuildParticipantWithState(ctx context.Context, id string, hp int, mp *int, consumed []corebattle.ConsumedItem) (corebattle.Participant, error) {
	if id == "c1" {
		b.consumed = append(b.consumed, append([]corebattle.ConsumedItem(nil), consumed...))
	}
	c, err := b.chars.FindByID(ctx, id)
	if err != nil {
		return corebattle.Participant{}, err
	}
	p := corebattle.MustNewParticipant(c.ID, hp, c.Stats.Attack, c.Stats.Defense)
	p.MaxHP = c.Stats.MaxHP
	p.MaxMP = c.Stats.MaxMP
	p.MP = c.Stats.MP
	if mp != nil {
		p.MP = *mp
	}
	return p, nil
}

type mockExitClaimRepo struct {
	claims map[string]gvg.ExitClaimState
}

func newMockExitClaimRepo() *mockExitClaimRepo {
	return &mockExitClaimRepo{claims: make(map[string]gvg.ExitClaimState)}
}

func (r *mockExitClaimRepo) ClaimRoomExitForUpdate(_ context.Context, roomID, charID, joinID string) (gvg.ExitClaimState, error) {
	key := roomID + ":" + charID + ":" + joinID
	if st, ok := r.claims[key]; ok {
		st.AlreadyClaimed = true
		return st, nil
	}
	r.claims[key] = gvg.ExitClaimState{}
	return gvg.ExitClaimState{AlreadyClaimed: false, DeliveryDone: false}, nil
}

func (r *mockExitClaimRepo) MarkExitDeliveryDone(_ context.Context, roomID, charID, joinID string) error {
	key := roomID + ":" + charID + ":" + joinID
	st := r.claims[key]
	st.DeliveryDone = true
	r.claims[key] = st
	return nil
}

type stubPostBattleSettler struct {
	calls int
	reqs  []battle.ApplyPostBattleRequest
	err   error
}

func (s *stubPostBattleSettler) ApplyPostBattleResult(_ context.Context, req battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	s.calls++
	s.reqs = append(s.reqs, req)
	if s.err != nil {
		return battle.ApplyPostBattleResponse{}, s.err
	}
	return battle.ApplyPostBattleResponse{}, nil
}

type failingDeleteCharRoomRepo struct {
	gvg.RoomRepository
	failDeleteCharRoom atomic.Bool
	err                error
}

func (r *failingDeleteCharRoomRepo) DeleteCharacterRoom(ctx context.Context, characterID, roomID string) error {
	if r.failDeleteCharRoom.CompareAndSwap(true, false) {
		return r.err
	}
	return r.RoomRepository.DeleteCharacterRoom(ctx, characterID, roomID)
}

func setupTwoGuildRoom(t *testing.T, targetWins int, engine gvg.BattleEngine, opts ...gvg.Option) (*gvg.Service, gvg.RoomRepository, *mockCharRepo, *mockStandingRepo, string) {
	t.Helper()
	ctx := context.Background()
	roomRepo := gvg.NewMemoryRoomRepository()
	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()

	c1 := createTestCharacter("c1", "Leader", 80, 0)
	c1.Stats.MaxHP, c1.Stats.MP, c1.Stats.MaxMP = 100, 30, 50
	c2 := createTestCharacter("c2", "Ally", 90, 0)
	c2.Stats.MaxHP, c2.Stats.MP, c2.Stats.MaxMP = 100, 20, 40
	c3 := createTestCharacter("c3", "Enemy", 100, 0)
	c3.Stats.MaxHP, c3.Stats.MP, c3.Stats.MaxMP = 100, 25, 40
	chars.characters["c1"], chars.characters["c2"], chars.characters["c3"] = c1, c2, c3

	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"], guilds.charGuilds["c3"] = "g1", "g1", "g2"

	svc, err := gvg.NewService(roomRepo, standings, guilds, chars, engine, opts...)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{
		Name:       "Test War",
		MaxMembers: 4,
		TargetWins: targetWins,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "c2", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "c3", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	return svc, roomRepo, chars, standings, detail.Room.ID
}

func TestLeaveRoom_PreStartLeaderHandoverAndUnclampedGP(t *testing.T) {
	ctx := context.Background()
	svc, _, chars, standings, roomID := setupTwoGuildRoom(t, 1, &mockBattleEngine{outcome: corebattle.OutcomeWin})

	// Initial 2 + c2(+1) + c3(+1) = 4 GP. Leader c1 leaves -> transfers leadership to c2, deducts 2 GP -> 2 GP.
	if err := svc.LeaveRoom(ctx, "c1", roomID); err != nil {
		t.Fatalf("leader leave failed: %v", err)
	}
	if _, err := svc.GetCharacterRoom(ctx, "c1"); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected c1 unmapped, got %v", err)
	}
	if chars.characters["c1"].Tired != 1 || chars.characters["c1"].Stats.HP != 80 {
		t.Fatalf("pre-start exit side-effects mismatch: %+v", chars.characters["c1"])
	}

	detail, err := svc.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("room should remain open after leader handover: %v", err)
	}
	if detail.Room.LeaderCharacterID != "c2" || detail.Room.LeaderName != "Ally" || !detail.Members[0].IsLeader {
		t.Fatalf("leadership did not transfer to c2: %+v", detail)
	}
	if detail.Room.PrizePool != 2 {
		t.Fatalf("PrizePool = %d, want 2 (4 - 2)", detail.Room.PrizePool)
	}

	// Repeated join (+1) / leave (-2) drives PrizePool negative per legacy system.cgi:1357.
	for range 4 {
		if _, err := svc.JoinRoom(ctx, "c1", roomID, ""); err != nil {
			t.Fatalf("rejoin failed: %v", err)
		}
		if err := svc.LeaveRoom(ctx, "c1", roomID); err != nil {
			t.Fatalf("re-leave failed: %v", err)
		}
	}
	detail, err = svc.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Room.PrizePool != -2 {
		t.Fatalf("PrizePool after 4 join/leave cycles = %d, want -2", detail.Room.PrizePool)
	}

	// New leader c2 starts and wins; negative PrizePool (-2) combines with +4 ParticipantGP = +2 GP.
	if _, err := svc.StartMatch(ctx, "c2", roomID); err != nil {
		t.Fatalf("new leader StartMatch failed: %v", err)
	}
	res, err := svc.AdvanceRound(ctx, "c2", roomID, 1)
	if err != nil || !res.MatchCompleted || res.PrizeGP != -2 {
		t.Fatalf("AdvanceRound = %+v, err=%v", res, err)
	}
	st, err := standings.GetOrCreateStanding(ctx, "g1")
	if err != nil || st.VictoryPoints != 2 {
		t.Fatalf("g1 VictoryPoints = %d (err=%v), want 2 (-2 prize + 4 participant)", st.VictoryPoints, err)
	}
}

func TestLeaveRoom_MidMatchDepartureAndSoleRemainingGuildWin(t *testing.T) {
	ctx := context.Background()
	engine := &failingBattleEngine{err: errors.New("battle engine must not be called when 1 guild remains")}
	svc, _, _, _, roomID := setupTwoGuildRoom(t, 2, engine)

	if _, err := svc.StartMatch(ctx, "c1", roomID); err != nil {
		t.Fatal(err)
	}

	// Non-leader member c3 (only member of g2) leaves mid-match.
	if err := svc.LeaveRoom(ctx, "c3", roomID); err != nil {
		t.Fatalf("non-leader mid-match leave failed: %v", err)
	}
	// Leader c1 leaves mid-match -> transfers leadership to c2.
	if err := svc.LeaveRoom(ctx, "c1", roomID); err != nil {
		t.Fatalf("leader mid-match leave failed: %v", err)
	}

	detail, err := svc.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Room.LeaderCharacterID != "c2" || len(detail.Members) != 1 {
		t.Fatalf("unexpected room state after mid-match departures: %+v", detail)
	}

	// Old leader cannot advance; new leader c2 wins round 1 without invoking battle engine.
	if _, err := svc.AdvanceRound(ctx, "c1", roomID, 1); !errors.Is(err, gvg.ErrNotRoomLeader) {
		t.Fatalf("old leader advance = %v, want ErrNotRoomLeader", err)
	}
	res, err := svc.AdvanceRound(ctx, "c2", roomID, 1)
	if err != nil {
		t.Fatalf("new leader AdvanceRound failed: %v", err)
	}
	if res.Outcome != "round_win" || res.WinnerGuildID != "g1" || engine.calls.Load() != 0 {
		t.Fatalf("expected sole-guild round_win without engine call: res=%+v calls=%d", res, engine.calls.Load())
	}
}

func TestLeaveRoom_PostMatchIndividualExitAndFinalRoundHP(t *testing.T) {
	ctx := context.Background()
	engine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{"c1": 35, "c2": 100, "c3": 0},
			RemainingMP: map[string]int{"c1": 12, "c2": 20, "c3": 0},
		},
	}
	svc, _, chars, _, roomID := setupTwoGuildRoom(t, 1, engine)
	if _, err := svc.StartMatch(ctx, "c1", roomID); err != nil {
		t.Fatal(err)
	}
	res, err := svc.AdvanceRound(ctx, "c1", roomID, 1)
	if err != nil || !res.MatchCompleted {
		t.Fatalf("AdvanceRound = %+v, err=%v", res, err)
	}

	// c1 leaves completed room: room stays open for c2 and c3, leadership transfers to c2.
	if err := svc.LeaveRoom(ctx, "c1", roomID); err != nil {
		t.Fatalf("c1 post-match leave failed: %v", err)
	}
	detail, err := svc.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatalf("room should stay open until last member leaves: %v", err)
	}
	if detail.Room.LeaderCharacterID != "c2" || len(detail.Members) != 2 {
		t.Fatalf("unexpected state after c1 completed leave: %+v", detail)
	}
	if got := chars.characters["c1"]; got.Stats.HP != 35 || got.Stats.MP != 12 || got.Tired != 4 {
		t.Fatalf("c1 exit vitality/fatigue mismatch: %+v", got)
	}

	// c3 leaves defeated (0 HP -> 1 HP survival floor, 0 MP, tired = 4).
	if err := svc.LeaveRoom(ctx, "c3", roomID); err != nil {
		t.Fatalf("c3 post-match leave failed: %v", err)
	}
	if got := chars.characters["c3"]; got.Stats.HP != 1 || got.Stats.MP != 0 || got.Tired != 4 {
		t.Fatalf("c3 exit vitality/fatigue mismatch: %+v", got)
	}

	// Last member c2 leaves -> disbands room.
	if err := svc.LeaveRoom(ctx, "c2", roomID); err != nil {
		t.Fatalf("last member leave failed: %v", err)
	}
	if _, err := svc.GetRoom(ctx, roomID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected room deleted after last member leaves, got %v", err)
	}
}

func TestLeaveRoom_ConsumedItemsCarryAcrossRoundsAndRetryIdempotency(t *testing.T) {
	ctx := context.Background()
	builder := &statefulMockBuilder{}
	settler := &stubPostBattleSettler{}
	claims := newMockExitClaimRepo()
	baseRepo := gvg.NewMemoryRoomRepository()
	roomRepo := &failingDeleteCharRoomRepo{RoomRepository: baseRepo, err: errors.New("valkey unavailable")}

	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:       corebattle.OutcomeWin,
				RemainingHP:   map[string]int{"c1": 40, "c2": 100, "c3": 0},
				RemainingMP:   map[string]int{"c1": 10, "c2": 20, "c3": 0},
				ConsumedItems: map[string][]corebattle.ConsumedItem{"c1": {{ID: "item-193", Quantity: 1}}},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 55, "c2": 100, "c3": 0},
				RemainingMP: map[string]int{"c1": 5, "c2": 20, "c3": 0},
			},
		},
	}

	var hookCalls int
	var failHook atomic.Bool
	failHook.Store(true)
	hook := func(_ context.Context, _ string) (string, error) {
		hookCalls++
		if failHook.CompareAndSwap(true, false) {
			return "", errors.New("tavern temporary error")
		}
		return "ok", nil
	}

	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()
	builder.chars = chars

	c1 := createTestCharacter("c1", "Leader", 100, 0)
	c2 := createTestCharacter("c2", "Ally", 100, 0)
	c3 := createTestCharacter("c3", "Enemy", 100, 0)
	chars.characters["c1"], chars.characters["c2"], chars.characters["c3"] = c1, c2, c3
	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"], guilds.charGuilds["c3"] = "g1", "g1", "g2"

	svc, err := gvg.NewService(
		roomRepo, standings, guilds, chars, engine,
		gvg.WithParticipantBuilder(builder),
		gvg.WithPostBattleSettler(settler),
		gvg.WithExitClaimRepository(claims),
		gvg.WithPostExitHook(hook),
	)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Consumed", MaxMembers: 4, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = svc.JoinRoom(ctx, "c2", detail.Room.ID, "")
	_, _ = svc.JoinRoom(ctx, "c3", detail.Room.ID, "")
	_, _ = svc.StartMatch(ctx, "c1", detail.Room.ID)
	if _, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 2); err != nil {
		t.Fatal(err)
	}
	if len(builder.consumed) != 2 || len(builder.consumed[0]) != 0 || len(builder.consumed[1]) != 1 || builder.consumed[1][0].ID != "item-193" {
		t.Fatalf("round 2 did not receive round 1 ConsumedItems: %+v", builder.consumed)
	}

	// Attempt 1: settler succeeds, postExitHook fails -> retry must NOT re-run settler.
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err == nil {
		t.Fatal("expected postExitHook failure")
	}
	// Attempt 2: postExitHook succeeds, Valkey DeleteCharacterRoom fails -> retry must NOT re-run settler or postExitHook.
	roomRepo.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err == nil {
		t.Fatal("expected Valkey DeleteCharacterRoom failure")
	}
	// Attempt 3: succeeds cleanly.
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err != nil {
		t.Fatalf("final LeaveRoom retry failed: %v", err)
	}

	if settler.calls != 1 {
		t.Fatalf("settler called %d times, want 1", settler.calls)
	}
	if hookCalls != 2 {
		t.Fatalf("postExitHook called %d times (1 fail + 1 success), want 2", hookCalls)
	}
	req := settler.reqs[0]
	if req.ExitFatigue != 7 || req.ExitBonusRounds != 2 || req.BattleResult.RemainingHP["c1"] != 55 || req.BattleResult.RemainingMP["c1"] != 5 || len(req.BattleResult.ConsumedItems["c1"]) != 1 {
		t.Fatalf("unexpected settler request: %+v", req)
	}
}

func TestLeaveRoom_RejoinSameRoomAppliesNewExitSideEffectsAndRetryIdempotency(t *testing.T) {
	ctx := context.Background()
	settler := &stubPostBattleSettler{}
	claims := newMockExitClaimRepo()
	roomRepo := &failingDeleteCharRoomRepo{
		RoomRepository: gvg.NewMemoryRoomRepository(),
		err:            errors.New("valkey unavailable"),
	}
	var hookCalls int
	hook := func(_ context.Context, _ string) (string, error) {
		hookCalls++
		return "ok", nil
	}

	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()
	chars.characters["c1"] = createTestCharacter("c1", "Leader", 80, 0)
	chars.characters["c2"] = createTestCharacter("c2", "Enemy", 90, 0)
	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"] = "g1", "g2"

	engine := &customGvGBattleEngine{
		result: corebattle.PartyBattleResult{
			Outcome:     corebattle.OutcomeWin,
			RemainingHP: map[string]int{"c1": 33, "c2": 0},
			RemainingMP: map[string]int{"c1": 7, "c2": 0},
		},
	}
	svc, err := gvg.NewService(
		roomRepo, standings, guilds, chars, engine,
		gvg.WithPostBattleSettler(settler),
		gvg.WithExitClaimRepository(claims),
		gvg.WithPostExitHook(hook),
	)
	if err != nil {
		t.Fatal(err)
	}

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Rejoin Test", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	firstJoinID := detail.Members[0].JoinID
	if firstJoinID == "" {
		t.Fatal("expected non-empty initial JoinID")
	}
	if _, err := svc.JoinRoom(ctx, "c2", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	// 1st exit (pre-start).
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err != nil {
		t.Fatalf("first leave failed: %v", err)
	}

	// Rejoin same room -> distinct JoinID.
	rejoined, err := svc.JoinRoom(ctx, "c1", detail.Room.ID, "")
	if err != nil {
		t.Fatalf("rejoin failed: %v", err)
	}
	var secondJoinID string
	for _, m := range rejoined.Members {
		if m.CharacterID == "c1" {
			secondJoinID = m.JoinID
		}
	}
	if secondJoinID == "" || secondJoinID == firstJoinID {
		t.Fatalf("expected fresh JoinID on rejoin, got %q (first %q)", secondJoinID, firstJoinID)
	}

	// Advance round 1 (new leader c2 starts and advances), then c1 leaves with transient Valkey failure + retry.
	if _, err := svc.StartMatch(ctx, "c2", detail.Room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, "c2", detail.Room.ID, 1); err != nil {
		t.Fatal(err)
	}

	roomRepo.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err == nil {
		t.Fatal("expected transient Valkey DeleteCharacterRoom failure")
	}
	if err := svc.LeaveRoom(ctx, "c1", detail.Room.ID); err != nil {
		t.Fatalf("second leave retry failed: %v", err)
	}

	if settler.calls != 2 {
		t.Fatalf("settler called %d times, want 2 (once per membership join)", settler.calls)
	}
	if hookCalls != 2 {
		t.Fatalf("postExitHook called %d times, want 2", hookCalls)
	}
	if settler.reqs[0].ExitFatigue != 1 || settler.reqs[0].ExitBonusRounds != 0 {
		t.Fatalf("1st exit request mismatch: %+v", settler.reqs[0])
	}
	if settler.reqs[1].ExitFatigue != 4 || settler.reqs[1].ExitBonusRounds != 1 || settler.reqs[1].BattleResult.RemainingHP["c1"] != 33 {
		t.Fatalf("2nd exit request mismatch: %+v", settler.reqs[1])
	}
}

type faultRoomRepo struct {
	gvg.RoomRepository
	getCharErr   error
	saveErr      error
	deleteErr    error
	saveCalls    int
	setCharCalls int
}

func (r *faultRoomRepo) GetCharacterRoom(ctx context.Context, characterID string) (string, error) {
	if r.getCharErr != nil {
		return "", r.getCharErr
	}
	return r.RoomRepository.GetCharacterRoom(ctx, characterID)
}

func (r *faultRoomRepo) SaveRoom(ctx context.Context, room gvg.GvGRoom, members []gvg.GvGMember) error {
	if r.saveErr != nil {
		return r.saveErr
	}
	r.saveCalls++
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func (r *faultRoomRepo) SetCharacterRoom(ctx context.Context, characterID string, roomID string) error {
	r.setCharCalls++
	return r.RoomRepository.SetCharacterRoom(ctx, characterID, roomID)
}

func (r *faultRoomRepo) DeleteRoom(ctx context.Context, id string) error {
	if r.deleteErr != nil {
		return r.deleteErr
	}
	return r.RoomRepository.DeleteRoom(ctx, id)
}

func TestAdmissionAbortsOnGetCharacterRoomError(t *testing.T) {
	ctx := context.Background()
	lookupErr := errors.New("valkey lookup timeout")
	base := gvg.NewMemoryRoomRepository()
	repo := &faultRoomRepo{RoomRepository: base, getCharErr: lookupErr}
	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()
	chars.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	chars.characters["c2"] = createTestCharacter("c2", "Joiner", 100, 0)
	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"] = "g1", "g2"

	svc, err := gvg.NewService(repo, standings, guilds, chars, &mockBattleEngine{outcome: corebattle.OutcomeWin})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Should Abort"}); !errors.Is(err, lookupErr) {
		t.Fatalf("CreateRoom error = %v, want %v", err, lookupErr)
	}
	if repo.saveCalls != 0 || repo.setCharCalls != 0 {
		t.Fatalf("CreateRoom wrote room/mapping on lookup failure: save=%d setChar=%d", repo.saveCalls, repo.setCharCalls)
	}

	// Valid room creation succeeds when GetCharacterRoom returns ErrRoomNotFound.
	repo.getCharErr = nil
	room, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Valid Room", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatalf("CreateRoom after recovery: %v", err)
	}
	beforeSaves := repo.saveCalls
	beforeSets := repo.setCharCalls

	repo.getCharErr = lookupErr
	if _, err := svc.JoinRoom(ctx, "c2", room.Room.ID, ""); !errors.Is(err, lookupErr) {
		t.Fatalf("JoinRoom error = %v, want %v", err, lookupErr)
	}
	if repo.saveCalls != beforeSaves || repo.setCharCalls != beforeSets {
		t.Fatalf("JoinRoom wrote room/mapping on lookup failure: save=%d->%d setChar=%d->%d", beforeSaves, repo.saveCalls, beforeSets, repo.setCharCalls)
	}
}

func TestLeaveRoom_SaveFailurePreservesMappingAndBlocksSecondRoom(t *testing.T) {
	ctx := context.Background()
	base := gvg.NewMemoryRoomRepository()
	repo := &faultRoomRepo{RoomRepository: base}
	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()
	settler := &stubPostBattleSettler{}
	claims := newMockExitClaimRepo()

	chars.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	chars.characters["c2"] = createTestCharacter("c2", "Member", 100, 0)
	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"] = "g1", "g2"

	svc, err := gvg.NewService(
		repo, standings, guilds, chars, &mockBattleEngine{outcome: corebattle.OutcomeWin},
		gvg.WithPostBattleSettler(settler),
		gvg.WithExitClaimRepository(claims),
	)
	if err != nil {
		t.Fatal(err)
	}

	room1, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Room 1", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "c2", room1.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	repo.saveErr = errors.New("valkey save failure")
	if err := svc.LeaveRoom(ctx, "c1", room1.Room.ID); err == nil {
		t.Fatal("expected SaveRoom failure on LeaveRoom")
	}
	repo.saveErr = nil

	// Mapping must remain intact so c1 cannot enter a second room while still in room1.
	if _, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Room 2"}); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("CreateRoom while partial leave pending = %v, want ErrAlreadyInRoom", err)
	}

	// Retry LeaveRoom finishes room publication and mapping removal without repeating exit effects.
	if err := svc.LeaveRoom(ctx, "c1", room1.Room.ID); err != nil {
		t.Fatalf("LeaveRoom retry failed: %v", err)
	}
	if settler.calls != 1 {
		t.Fatalf("settler calls = %d, want 1", settler.calls)
	}
	detail, err := svc.GetRoom(ctx, room1.Room.ID)
	if err != nil {
		t.Fatalf("GetRoom failed: %v", err)
	}
	if detail.Room.LeaderCharacterID != "c2" || len(detail.Members) != 1 {
		t.Fatalf("unexpected room1 state after retry: %+v", detail)
	}
	if _, err := svc.GetCharacterRoom(ctx, "c1"); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected c1 unmapped after retry, got %v", err)
	}
}

func TestLeaveRoom_DeleteFailureAndMappingRetrySafety(t *testing.T) {
	ctx := context.Background()
	base := gvg.NewMemoryRoomRepository()
	delCharErr := errors.New("valkey delete char room failure")
	delCharWrap := &failingDeleteCharRoomRepo{RoomRepository: base, err: delCharErr}
	repo := &faultRoomRepo{RoomRepository: delCharWrap}
	standings := newMockStandingRepo()
	guilds := newMockGuildRepo()
	chars := newMockCharRepo()
	settler := &stubPostBattleSettler{}
	claims := newMockExitClaimRepo()

	chars.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
	chars.characters["c2"] = createTestCharacter("c2", "Member", 100, 0)
	guilds.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guilds.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guilds.charGuilds["c1"], guilds.charGuilds["c2"] = "g1", "g2"

	svc, err := gvg.NewService(
		repo, standings, guilds, chars, &mockBattleEngine{outcome: corebattle.OutcomeWin},
		gvg.WithPostBattleSettler(settler),
		gvg.WithExitClaimRepository(claims),
	)
	if err != nil {
		t.Fatal(err)
	}

	room1, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Room 1", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "c2", room1.Room.ID, ""); err != nil {
		t.Fatal(err)
	}

	// 1. DeleteCharacterRoom fails after SaveRoom removes c1; c2 also leaves (disbanding room1) before c1 retries.
	delCharWrap.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, "c1", room1.Room.ID); !errors.Is(err, delCharErr) {
		t.Fatalf("c1 LeaveRoom = %v, want %v", err, delCharErr)
	}
	// DeleteRoom fails for last member c2 -> mapping preserved -> retry succeeds.
	repo.deleteErr = errors.New("valkey delete room failure")
	if err := svc.LeaveRoom(ctx, "c2", room1.Room.ID); err == nil {
		t.Fatal("expected DeleteRoom failure")
	}
	repo.deleteErr = nil
	if _, err := svc.CreateRoom(ctx, "c2", gvg.CreateRoomRequest{Name: "Blocked"}); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("c2 CreateRoom while DeleteRoom pending = %v, want ErrAlreadyInRoom", err)
	}
	if err := svc.LeaveRoom(ctx, "c2", room1.Room.ID); err != nil {
		t.Fatalf("c2 LeaveRoom retry failed: %v", err)
	}

	// Room1 is gone, c1 mapping still points to room1 -> c1 retry cleans mapping without ErrRoomNotFound.
	if err := svc.LeaveRoom(ctx, "c1", room1.Room.ID); err != nil {
		t.Fatalf("c1 LeaveRoom mapping retry after disband failed: %v", err)
	}
	if settler.calls != 2 {
		t.Fatalf("settler calls = %d, want 2 (once each)", settler.calls)
	}

	// 2. c1 creates room2; stale DeleteCharacterRoom or LeaveRoom for room1 cannot delete room2's mapping.
	room2, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Room 2", MaxMembers: 4, TargetWins: 1})
	if err != nil {
		t.Fatalf("c1 CreateRoom(room2) failed: %v", err)
	}
	if err := base.DeleteCharacterRoom(ctx, "c1", room1.Room.ID); err != nil {
		t.Fatalf("stale DeleteCharacterRoom(room1): %v", err)
	}
	if err := svc.LeaveRoom(ctx, "c1", room1.Room.ID); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("stale LeaveRoom(room1) = %v, want ErrRoomNotFound", err)
	}
	if got, err := base.GetCharacterRoom(ctx, "c1"); err != nil || got != room2.Room.ID {
		t.Fatalf("room2 mapping corrupted: got=%q err=%v want=%q", got, err, room2.Room.ID)
	}

	// 3. Last member DeleteCharacterRoom failure after DeleteRoom -> retry succeeds cleanly.
	delCharWrap.failDeleteCharRoom.Store(true)
	if err := svc.LeaveRoom(ctx, "c1", room2.Room.ID); !errors.Is(err, delCharErr) {
		t.Fatalf("c1 LeaveRoom(room2) = %v, want %v", err, delCharErr)
	}
	if _, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Blocked 3"}); !errors.Is(err, gvg.ErrAlreadyInRoom) {
		t.Fatalf("c1 CreateRoom while mapping cleanup pending = %v, want ErrAlreadyInRoom", err)
	}
	if err := svc.LeaveRoom(ctx, "c1", room2.Room.ID); err != nil {
		t.Fatalf("c1 LeaveRoom(room2) mapping retry failed: %v", err)
	}
	if _, err := svc.GetCharacterRoom(ctx, "c1"); !errors.Is(err, gvg.ErrRoomNotFound) {
		t.Fatalf("expected c1 unmapped, got %v", err)
	}
	if settler.calls != 3 {
		t.Fatalf("settler calls = %d, want 3", settler.calls)
	}
}
