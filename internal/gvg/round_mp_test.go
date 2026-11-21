package gvg_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/gvg"
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
	if char.ID == "c1" {
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
	gvg.RoomRepository
	failSave atomic.Bool
	err      error
}

func (r *failingSaveRoomRepo) SaveRoom(ctx context.Context, room gvg.GvGRoom, members []gvg.GvGMember) error {
	if r.failSave.CompareAndSwap(true, false) {
		return r.err
	}
	return r.RoomRepository.SaveRoom(ctx, room, members)
}

func TestGvG_CarryCurrentMPBetweenRounds(t *testing.T) {
	ctx := context.Background()
	guildRepo, charRepo := newMockGuildRepo(), newMockCharRepo()
	standings := newMockStandingRepo()
	roomRepo := gvg.NewMemoryRoomRepository()
	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 35, "c2": 0},
				RemainingMP: map[string]int{"c1": 0, "c2": 0},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 150, "c2": 0},
				RemainingMP: map[string]int{"c1": 0, "c2": 0},
			},
		},
	}

	svc, err := gvg.NewService(roomRepo, standings, guildRepo, charRepo, engine, gvg.WithParticipantBuilder(mpSnapshotBuilder{chars: charRepo}))
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("c1", "Leader", 100, 0)
	c1.Stats.HP, c1.Stats.MaxHP = 70, 100
	c1.Stats.MP, c1.Stats.MaxMP = 40, 40

	c2 := createTestCharacter("c2", "Joiner", 100, 0)
	c2.Stats.HP, c2.Stats.MaxHP = 50, 100
	c2.Stats.MP, c2.Stats.MaxMP = 0, 20

	charRepo.characters["c1"] = c1
	charRepo.characters["c2"] = c2
	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds["c1"], guildRepo.charGuilds["c2"] = "g1", "g2"

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "GvG MP", MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	if detail, err = svc.JoinRoom(ctx, "c2", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	if detail.Members[0].MP == nil || *detail.Members[0].MP != 40 {
		t.Fatalf("leader entry MP = %v, want 40", detail.Members[0].MP)
	}
	if detail.Members[1].MP == nil || *detail.Members[1].MP != 0 {
		t.Fatalf("joiner entry MP = %v, want 0", detail.Members[1].MP)
	}

	// Mutate DB MP after room entry; rounds must use room member state.
	c1.Stats.MP = 99
	c2.Stats.MP = 88
	charRepo.characters["c1"] = c1
	charRepo.characters["c2"] = c2

	started, err := svc.StartMatch(ctx, "c1", detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if started.Members[0].HP != 200 || *started.Members[0].MP != 40 || started.Members[1].HP != 100 || *started.Members[1].MP != 0 {
		t.Fatalf("unexpected start state: %+v", started.Members)
	}

	res1, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 1)
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
		t.Fatalf("after round 1 leader = %+v (MP=%v), want HP=200 MP=0", afterR1.Members[0], afterR1.Members[0].MP)
	}
	if afterR1.Members[1].HP != 100 || afterR1.Members[1].MP == nil || *afterR1.Members[1].MP != 0 {
		t.Fatalf("after round 1 joiner = %+v (MP=%v), want HP=100 MP=0", afterR1.Members[1], afterR1.Members[1].MP)
	}

	res2, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 2)
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
	if r1Ally.HP != 200 || r1Ally.MaxHP != 200 || r1Ally.MP != 40 || r1Ally.MaxMP != 120 {
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

	// Stored character MP must not be mutated during GvG combat rounds.
	if charRepo.characters["c1"].Stats.MP != 99 || charRepo.characters["c2"].Stats.MP != 88 {
		t.Fatalf("characters MP mutated during GvG: c1=%d c2=%d", charRepo.characters["c1"].Stats.MP, charRepo.characters["c2"].Stats.MP)
	}
}

func TestGvG_MP_LegacyCompatibility_And_RetryIdempotency(t *testing.T) {
	ctx := context.Background()

	// Verify 0 MP survives JSON round-trip while omitted MP remains nil.
	zero := 0
	raw, err := json.Marshal([]gvg.GvGMember{
		{CharacterID: "zero", MP: &zero},
		{CharacterID: "legacy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	var decoded []gvg.GvGMember
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded[0].MP == nil || *decoded[0].MP != 0 {
		t.Fatalf("zero MP lost in JSON round-trip: %+v", decoded[0].MP)
	}
	if decoded[1].MP != nil {
		t.Fatalf("legacy member MP should remain nil, got %+v", decoded[1].MP)
	}

	guildRepo, charRepo := newMockGuildRepo(), newMockCharRepo()
	standings := newMockStandingRepo()
	baseRooms := gvg.NewMemoryRoomRepository()
	roomRepo := &failingSaveRoomRepo{RoomRepository: baseRooms, err: errors.New("valkey save failed")}
	engine := &scriptedMPBattleEngine{
		results: []corebattle.PartyBattleResult{
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 80, "c2": 0},
				RemainingMP: map[string]int{"c1": 15, "c2": 3},
			},
			{
				Outcome:     corebattle.OutcomeWin,
				RemainingHP: map[string]int{"c1": 140, "c2": 0},
				RemainingMP: map[string]int{"c1": 5, "c2": 0},
			},
		},
	}

	svc, err := gvg.NewService(roomRepo, standings, guildRepo, charRepo, engine, gvg.WithParticipantBuilder(mpSnapshotBuilder{chars: charRepo}))
	if err != nil {
		t.Fatal(err)
	}

	c1 := createTestCharacter("c1", "Leader", 100, 0)
	c1.Stats.MP, c1.Stats.MaxMP = 25, 40
	c2 := createTestCharacter("c2", "Joiner", 100, 0)
	c2.Stats.MP, c2.Stats.MaxMP = 10, 20
	charRepo.characters["c1"] = c1
	charRepo.characters["c2"] = c2
	guildRepo.guilds["g1"] = guild.Guild{ID: "g1", Name: "Crimson", Color: "#FF3333"}
	guildRepo.guilds["g2"] = guild.Guild{ID: "g2", Name: "Azure", Color: "#33CCFF"}
	guildRepo.charGuilds["c1"], guildRepo.charGuilds["c2"] = "g1", "g2"

	detail, err := svc.CreateRoom(ctx, "c1", gvg.CreateRoomRequest{Name: "Legacy Retry", MaxMembers: 2, TargetWins: 2})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.JoinRoom(ctx, "c2", detail.Room.ID, ""); err != nil {
		t.Fatal(err)
	}
	started, err := svc.StartMatch(ctx, "c1", detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Simulate an older room snapshot without MP stored.
	for i := range started.Members {
		started.Members[i].MP = nil
	}
	if err := baseRooms.SaveRoom(ctx, started.Room, started.Members); err != nil {
		t.Fatal(err)
	}

	// Round 1 commits SQL transition with RemainingMP (15/3), then fails Valkey SaveRoom.
	roomRepo.failSave.Store(true)
	if _, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 1); err == nil {
		t.Fatal("expected SaveRoom failure after SQL commit")
	}
	if engine.requests[0].Allies[0].MP != 25 || engine.requests[0].Enemies[0].MP != 10 {
		t.Fatalf("legacy fallback MP mismatch: ally=%d enemy=%d", engine.requests[0].Allies[0].MP, engine.requests[0].Enemies[0].MP)
	}

	// Mutate DB MP before retry and Round 2.
	c1.Stats.MP = 40
	c2.Stats.MP = 20
	charRepo.characters["c1"] = c1
	charRepo.characters["c2"] = c2

	// Retry Round 1 must replay committed transition, republish MP=15/3, and NOT rerun combat.
	res1, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 1)
	if err != nil {
		t.Fatalf("round 1 replay failed: %v", err)
	}
	if engine.calls.Load() != 1 || res1.Round != 2 {
		t.Fatalf("round 1 replay reran combat or wrong round: calls=%d res=%+v", engine.calls.Load(), res1)
	}

	republished, err := svc.GetRoom(ctx, detail.Room.ID)
	if err != nil {
		t.Fatal(err)
	}
	if republished.Members[0].MP == nil || *republished.Members[0].MP != 15 || republished.Members[1].MP == nil || *republished.Members[1].MP != 3 {
		t.Fatalf("republished room did not restore RemainingMP (15/3): %+v", republished.Members)
	}

	// Round 2 must use Round 1's RemainingMP (15/3) without double-applying Round 1.
	res2, err := svc.AdvanceRound(ctx, "c1", detail.Room.ID, 2)
	if err != nil {
		t.Fatalf("round 2 failed: %v", err)
	}
	if !res2.MatchCompleted || engine.calls.Load() != 2 {
		t.Fatalf("expected round 2 completion with 2 total engine calls, got calls=%d res=%+v", engine.calls.Load(), res2)
	}
	if engine.requests[1].Allies[0].MP != 15 || engine.requests[1].Enemies[0].MP != 3 {
		t.Fatalf("round 2 MP mismatch: ally=%d enemy=%d", engine.requests[1].Allies[0].MP, engine.requests[1].Enemies[0].MP)
	}
}
