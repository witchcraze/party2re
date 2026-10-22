package gvg_test

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/id"
)

// Reject one save command with a real server error without corrupting the shared index.
type saveFaultClient struct {
	valkey.Client
	command, key, wrongTypeKey string
	fail                       bool
	beforeSet                  func()
}

func (c *saveFaultClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	args := cmd.Commands()
	if args[0] == "SET" && args[1] == c.key && c.beforeSet != nil {
		c.beforeSet()
	}
	if c.fail && args[0] == c.command && args[1] == c.key {
		return c.Client.Do(ctx, c.B().Zadd().Key(c.wrongTypeKey).ScoreMember().ScoreMember(1, "fault").Build())
	}
	return c.Client.Do(ctx, cmd)
}

type countingGvGBattle struct {
	gvg.BattleEngine
	calls int
}

func TestGvGRoomListingDuringPublicationIntegration(t *testing.T) {
	f := newRewardFixture(t, 2, 1, corebattle.OutcomeWin)
	ctx := context.Background()
	roomKey := gvg.DefaultRoomKeyPrefix + f.detail.Room.ID
	if err := f.client.Do(ctx, f.client.B().Del().Key(roomKey).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	detail := f.detail
	detail.Room.Status = gvg.StatusRecruiting
	// Keep this test's candidate in the bounded listing even alongside other fixtures.
	detail.Room.UpdatedAt = time.Now().UTC().Add(time.Minute)
	listed := false
	client := &saveFaultClient{Client: f.client, key: roomKey, beforeSet: func() {
		listed = true
		if _, err := f.rooms.ListRooms(ctx); err != nil {
			t.Fatal(err)
		}
	}}
	rooms, err := gvg.NewValkeyRoomRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	if err := rooms.SaveRoom(ctx, detail.Room, detail.Members); err != nil {
		t.Fatal(err)
	}
	if !listed {
		t.Fatal("listing did not interleave with publication")
	}
	if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(detail.Room.ID).Build()).AsFloat64(); err != nil || score != float64(detail.Room.UpdatedAt.Unix()) {
		t.Fatalf("listing erased publishing room's index: %v %v", score, err)
	}
	list, err := rooms.ListRooms(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, room := range list {
		if room.ID == detail.Room.ID {
			return
		}
	}
	t.Fatal("successful publication is absent from room listing")
}

func (b *countingGvGBattle) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	b.calls++
	return b.BattleEngine.ResolvePartyBattle(req)
}

func TestGvGSaveRoomCommandFailureIntegration(t *testing.T) {
	for _, command := range []string{"ZADD", "SET"} {
		for _, target := range []int{1, 2} {
			name := command + map[int]string{1: "/final", 2: "/intermediate"}[target]
			t.Run(name, func(t *testing.T) {
				f := newRewardFixture(t, target, 1, corebattle.OutcomeWin)
				ctx := context.Background()
				roomKey := gvg.DefaultRoomKeyPrefix + f.detail.Room.ID
				key := roomKey
				if command == "ZADD" {
					key = gvg.DefaultRoomsActiveIndexKey
				}
				wrongTypeKey := "party2:test:gvg-save:" + id.New()
				if err := f.client.Do(ctx, f.client.B().Set().Key(wrongTypeKey).Value("wrong type").ExSeconds(60).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				t.Cleanup(func() {
					if err := f.client.Do(ctx, f.client.B().Del().Key(wrongTypeKey).Build()).Error(); err != nil {
						t.Error(err)
					}
				})
				fault := &saveFaultClient{Client: f.client, command: command, key: key, wrongTypeKey: wrongTypeKey, fail: true}
				rooms, err := gvg.NewValkeyRoomRepository(fault)
				if err != nil {
					t.Fatal(err)
				}
				chars, err := database.NewCharacterRepository(f.db)
				if err != nil {
					t.Fatal(err)
				}
				battle := &countingGvGBattle{BattleEngine: f.battle}
				service, err := gvg.NewService(rooms, f.standings, newMockGuildRepo(), chars, battle)
				if err != nil {
					t.Fatal(err)
				}
				before, err := rooms.GetRoom(ctx, f.detail.Room.ID)
				if err != nil {
					t.Fatal(err)
				}
				before.Room.UpdatedAt = before.Room.UpdatedAt.Add(-time.Minute)
				if err := f.rooms.SaveRoom(ctx, before.Room, before.Members); err != nil {
					t.Fatal(err)
				}
				if err := f.client.Do(ctx, f.client.B().Expire().Key(roomKey).Seconds(60).Build()).Error(); err != nil {
					t.Fatal(err)
				}
				if _, err := service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 1); err == nil || !strings.Contains(err.Error(), "WRONGTYPE") {
					t.Fatalf("expected %s server error: %v", command, err)
				}
				after, err := rooms.GetRoom(ctx, before.Room.ID)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(before, after) {
					t.Errorf("failed save advanced room: before=%+v after=%+v", before.Room, after.Room)
				}
				if ttl, err := f.client.Do(ctx, f.client.B().Ttl().Key(roomKey).Build()).AsInt64(); err != nil || ttl <= 0 || ttl > 60 {
					t.Errorf("failed save refreshed room TTL: %d %v", ttl, err)
				}
				gp, opponentGP, wins := int64(3), int64(0), 0
				if target == 1 {
					gp, opponentGP, wins = 7, 4, 1
				}
				f.assertRewards(t, gp, opponentGP, wins, 0)
				saved, found, err := f.standings.FindRoundTransition(ctx, before.Room.ID)
				if err != nil || !found {
					t.Fatalf("SQL commit missing: %v %v", found, err)
				}
				expectedScore := float64(before.Room.UpdatedAt.Unix())
				if command == "SET" {
					expectedScore = float64(saved.Detail.Room.UpdatedAt.Unix())
				}
				if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(before.Room.ID).Build()).AsFloat64(); err != nil || score != expectedScore {
					t.Errorf("index after failed %s: score=%v want=%v err=%v", command, score, expectedScore, err)
				}
				fault.fail = false
				result, err := service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 1)
				if err != nil {
					t.Fatal(err)
				}
				if battle.calls != 1 || !reflect.DeepEqual(result, saved.Result) {
					t.Fatalf("retry ran another battle: calls=%d result=%+v", battle.calls, result)
				}
				if ttl, err := f.client.Do(ctx, f.client.B().Ttl().Key(roomKey).Build()).AsInt64(); err != nil || ttl < 1700 || ttl > 1800 {
					t.Errorf("recovered room TTL: %d %v", ttl, err)
				}
				if score, err := f.client.Do(ctx, f.client.B().Zscore().Key(gvg.DefaultRoomsActiveIndexKey).Member(before.Room.ID).Build()).AsFloat64(); err != nil || score != float64(saved.Detail.Room.UpdatedAt.Unix()) {
					t.Errorf("recovered index: %v %v", score, err)
				}
				f.assertRewards(t, gp, opponentGP, wins, 0)
				if target == 2 {
					result, err = service.AdvanceRound(ctx, before.Room.LeaderCharacterID, before.Room.ID, 2)
					if err != nil {
						t.Fatal(err)
					}
					if battle.calls != 2 || !result.MatchCompleted {
						t.Fatalf("next round did not complete: calls=%d result=%+v", battle.calls, result)
					}
					f.assertRewards(t, 10, 4, 1, 0)
				}
			})
		}
	}
}
