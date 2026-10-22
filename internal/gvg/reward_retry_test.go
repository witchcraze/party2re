package gvg_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/gvg"
)

var errRoomPublication = errors.New("room publication failed")

type failingPublicationRepo struct {
	gvg.RoomRepository
	fail          bool
	failDelete    bool
	failAfterSave bool
	getErr        error
}

func (r *failingPublicationRepo) GetRoom(ctx context.Context, roomID string) (gvg.RoomDetail, error) {
	if r.getErr != nil {
		return gvg.RoomDetail{}, r.getErr
	}
	return r.RoomRepository.GetRoom(ctx, roomID)
}

func (r *failingPublicationRepo) DeleteRoom(ctx context.Context, roomID string) error {
	if r.failDelete {
		return errRoomPublication
	}
	return r.RoomRepository.DeleteRoom(ctx, roomID)
}

func (r *failingPublicationRepo) SaveRoom(ctx context.Context, room gvg.GvGRoom, members []gvg.GvGMember) error {
	if r.fail {
		return errRoomPublication
	}
	if err := r.RoomRepository.SaveRoom(ctx, room, members); err != nil {
		return err
	}
	if r.failAfterSave {
		return errRoomPublication
	}
	return nil
}

func TestAdvanceRound_RetryRoomPublication(t *testing.T) {
	for _, target := range []int{1, 2} {
		t.Run(map[int]string{1: "final", 2: "intermediate"}[target], func(t *testing.T) {
			ctx := context.Background()
			standings := newMockStandingRepo()
			_, rooms, leader, roomID := setupSettlementTest(t, standings)
			detail, err := rooms.GetRoom(ctx, roomID)
			if err != nil {
				t.Fatal(err)
			}
			detail.Room.TargetWins = target
			if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
				t.Fatal(err)
			}
			chars := newMockCharRepo()
			chars.characters["c1"] = createTestCharacter("c1", "Leader", 100, 0)
			chars.characters["c2"] = createTestCharacter("c2", "Opponent", 100, 0)
			publication := &failingPublicationRepo{RoomRepository: rooms, fail: true}
			battle := &mockBattleEngine{outcome: corebattle.OutcomeWin}
			svc, err := gvg.NewService(publication, standings, newMockGuildRepo(), chars, battle)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AdvanceRound(ctx, leader, roomID, 1); !errors.Is(err, errRoomPublication) {
				t.Fatalf("publication error: %v", err)
			}
			// Recovery must reuse the committed winner even if another battle would differ.
			battle.outcome = corebattle.OutcomeDefeat
			publication.fail = false
			result, err := svc.AdvanceRound(ctx, leader, roomID, 1)
			if err != nil {
				t.Fatal(err)
			}
			expectedRoundGP := 0
			if target > 1 {
				expectedRoundGP = gvg.RoundWinGP
			}
			if result.WinnerGuildID != "g1" || result.GuildScores["g1"] != 1 || standings.roundWins["g1"] != expectedRoundGP || standings.roundWins["g2"] != 0 {
				t.Fatalf("round was rerun: result=%+v rewards=%v", result, standings.roundWins)
			}
			if target == 1 && len(standings.settlements) != 1 {
				t.Fatalf("settlements=%d", len(standings.settlements))
			}
			replayed, err := svc.AdvanceRound(ctx, leader, roomID, 1)
			if err != nil || !reflect.DeepEqual(replayed, result) {
				t.Fatalf("published result replay: result=%+v err=%v", replayed, err)
			}
		})
	}
}
