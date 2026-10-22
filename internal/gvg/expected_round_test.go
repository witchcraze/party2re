package gvg_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/gvg"
)

func TestAdvanceRoundExpectedIdentity(t *testing.T) {
	ctx := context.Background()
	standings := newMockStandingRepo()
	svc, rooms, leader, roomID := setupSettlementTest(t, standings)
	detail, err := rooms.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	detail.Room.TargetWins = 3
	if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	for _, round := range []int{-1, 0, gvg.MaxRounds + 1} {
		if _, err := svc.AdvanceRound(ctx, leader, roomID, round); !errors.Is(err, gvg.ErrInvalidExpectedRound) {
			t.Fatalf("invalid %d: %v", round, err)
		}
	}
	for _, round := range []int{2, gvg.MaxRounds} {
		if _, err := svc.AdvanceRound(ctx, leader, roomID, round); !errors.Is(err, gvg.ErrRoundMismatch) {
			t.Fatalf("future %d: %v", round, err)
		}
	}
	if len(standings.transitions) != 0 {
		t.Fatal("invalid request committed a round")
	}
	first, err := svc.AdvanceRound(ctx, leader, roomID, 1)
	if err != nil {
		t.Fatal(err)
	}
	if first.Round != 2 {
		t.Fatalf("result round changed meaning: %+v", first)
	}
	replay, err := svc.AdvanceRound(ctx, leader, roomID, 1)
	if err != nil || !reflect.DeepEqual(replay, first) {
		t.Fatalf("replay: %+v %v", replay, err)
	}
	if _, err := svc.AdvanceRound(ctx, "c2", roomID, 1); !errors.Is(err, gvg.ErrNotRoomLeader) {
		t.Fatalf("replay leader: %v", err)
	}
	if _, err := svc.AdvanceRound(ctx, leader, roomID, 2); err != nil {
		t.Fatal(err)
	}
	before, err := rooms.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	// The result of round 2 also says 3: response round is never an operation receipt.
	for _, round := range []int{1, 4} {
		if _, err := svc.AdvanceRound(ctx, leader, roomID, round); !errors.Is(err, gvg.ErrRoundMismatch) {
			t.Fatalf("conflict %d: %v", round, err)
		}
	}
	after, err := rooms.GetRoom(ctx, roomID)
	if err != nil || !reflect.DeepEqual(before, after) || standings.roundWins["g1"] != 6 {
		t.Fatalf("conflict mutated room/rewards: %+v %v", after, err)
	}
	// A delayed request must not republish an old room even when Valkey is behind.
	if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, leader, roomID, 1); !errors.Is(err, gvg.ErrRoundMismatch) {
		t.Fatal(err)
	}
	after, err = rooms.GetRoom(ctx, roomID)
	if err != nil || !reflect.DeepEqual(detail, after) {
		t.Fatalf("old request published a newer receipt: %+v %v", after, err)
	}
}

func TestAdvanceRoundPhaseAndCurrentLeader(t *testing.T) {
	for _, phase := range []string{gvg.StatusRecruiting, gvg.StatusCompleted} {
		t.Run(phase, func(t *testing.T) {
			ctx := context.Background()
			svc, rooms, leader, roomID := setupSettlementTest(t, newMockStandingRepo())
			detail, err := rooms.GetRoom(ctx, roomID)
			if err != nil {
				t.Fatal(err)
			}
			detail.Room.Status = phase
			if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AdvanceRound(ctx, leader, roomID, 1); !errors.Is(err, gvg.ErrMatchNotInProgress) {
				t.Fatalf("phase rejection: %v", err)
			}
		})
	}
	ctx := context.Background()
	svc, rooms, leader, roomID := setupSettlementTest(t, newMockStandingRepo())
	if _, err := svc.AdvanceRound(ctx, leader, roomID, 1); err != nil {
		t.Fatal(err)
	}
	detail, err := rooms.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	detail.Room.LeaderCharacterID = "c2"
	if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AdvanceRound(ctx, leader, roomID, 1); !errors.Is(err, gvg.ErrNotRoomLeader) {
		t.Fatalf("old leader replay: %v", err)
	}
	if _, err := svc.AdvanceRound(ctx, "c2", roomID, 1); err != nil {
		t.Fatalf("current leader replay: %v", err)
	}
}
