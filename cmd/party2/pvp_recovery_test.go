package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/rescue"
)

type pvpRecoveryRescueState struct {
	rescue.RescueRepository
	rescue.ActionCleaner
}

func (pvpRecoveryRescueState) FindLatestByCharacterID(context.Context, string) (rescue.RescueRecord, error) {
	return rescue.RescueRecord{}, rescue.ErrNoRescueRecord
}

func (pvpRecoveryRescueState) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func (pvpRecoveryRescueState) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestPvPRecoveryClearsContextAndRescueActivity(t *testing.T) {
	ctx := context.Background()
	chars := &casinoActivityCharStub{chars: map[string]corecharacter.Character{
		"hero": {ID: "hero", PlayerID: "owner", Name: "Hero", Money: 100, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
	}}
	rooms := pvp.NewMemoryRoomRepository()
	service, err := pvp.NewService(rooms, chars, &struct{ pvp.BattleEngine }{}, pvp.WithTransitionRepository(pvp.NewMemoryTransitionRepository()))
	if err != nil {
		t.Fatal(err)
	}
	room, err := service.CreateRoom(ctx, "hero", pvp.CreateRoomRequest{Name: "Lost", Bet: 10})
	if err != nil {
		t.Fatal(err)
	}
	if err := rooms.DeleteRoom(ctx, room.Room.ID); err != nil {
		t.Fatal(err)
	}
	f := &activityFixtures{}
	readers := contextActivityReaders{party: f, pvp: service, gvg: &gvgActivityFixture{}, dungeon: f, challenge: f, casino: f}
	state := pvpRecoveryRescueState{}
	rescues := rescue.NewService(state, chars, state, nil, state)
	rescues.SetActivityReader(readers)
	facts, err := readers.read(ctx, "owner", "hero")
	if err != nil || len(facts) != 1 || facts[0].Phase != "recovery_pending" || !slices.Equal(facts[0].Actions, []string{"pvp_leave"}) {
		t.Fatalf("missing recovery: %+v %v", facts, err)
	}
	if _, err := rescues.EmergencyRescue(ctx, "hero", "stuck", time.Now()); !errors.Is(err, rescue.ErrActivityRemaining) {
		t.Fatalf("rescue bypassed PvP recovery: %v", err)
	}
	if err := service.LeaveRoom(ctx, "hero", room.Room.ID); err != nil {
		t.Fatal(err)
	}
	if facts, err := readers.read(ctx, "owner", "hero"); err != nil || len(facts) != 0 {
		t.Fatalf("context still active: %+v %v", facts, err)
	}
	if record, err := rescues.EmergencyRescue(ctx, "hero", "stuck", time.Now()); err != nil || record.PenaltySeconds != 0 || record.ID != "" {
		t.Fatalf("recovered idle actor blocked or penalized: %+v %v", record, err)
	}
	if chars.chars["hero"].Money != 90 {
		t.Fatal("recovery refunded a lost bet")
	}
}
