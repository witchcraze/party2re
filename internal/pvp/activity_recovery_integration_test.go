package pvp_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/testutil"
)

type selectiveSetRooms struct {
	pvp.RoomRepository
	targetCharID string
	armed        atomic.Bool
	err          error
}

func (r *selectiveSetRooms) SetCharacterRoom(ctx context.Context, characterID, roomID string) error {
	if characterID == r.targetCharID && r.armed.CompareAndSwap(true, false) {
		return r.err
	}
	return r.RoomRepository.SetCharacterRoom(ctx, characterID, roomID)
}

type activityFixture struct {
	*transitionFixture
	timers    timer.Service
	clock     atomic.Int64
	battleSpy *countingDefeatBattle
}

type countingDefeatBattle struct {
	calls   atomic.Int64
	defeats []corebattle.PartyBattleDefeat
}

func (b *countingDefeatBattle) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	n := b.calls.Add(1)
	rem := make(map[string]int, len(req.Allies)+len(req.Enemies))
	for _, p := range append(req.Allies, req.Enemies...) {
		rem[p.ID] = max(1, p.HP)
	}
	// First round awards leader's team (draw=false) on win? Return OutcomeWin with leader alive only so Round 1 gives 1 point.
	for _, e := range req.Enemies {
		rem[e.ID] = 0
	}
	return corebattle.PartyBattleResult{
		Outcome:     corebattle.OutcomeWin,
		Turns:       int(n) + 1,
		RemainingHP: rem,
		Defeats:     b.defeats,
	}, nil
}

func newActivityFixture(t *testing.T, start time.Time) *activityFixture {
	t.Helper()
	f := newTransitionFixture(t)
	timers := timer.NewService(f.client)
	cleanupSleepTimers(t, timers, f.leader.ID, f.member.ID)
	af := &activityFixture{
		transitionFixture: f,
		timers:            timers,
		battleSpy: &countingDefeatBattle{
			defeats: []corebattle.PartyBattleDefeat{{ActorID: f.leader.ID, TargetID: f.member.ID}},
		},
	}
	af.clock.Store(start.UnixNano())
	af.restartActivity(t)
	return af
}

func (af *activityFixture) now() time.Time {
	return time.Unix(0, af.clock.Load()).UTC()
}

func (af *activityFixture) setNow(t time.Time) {
	af.clock.Store(t.UTC().UnixNano())
}

func (af *activityFixture) restartActivity(t *testing.T) {
	t.Helper()
	store, err := database.NewPvPTransitionRepository(af.db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := pvp.NewService(
		af.rooms,
		af.chars,
		af.battleSpy,
		pvp.WithTransactionProvider(database.NewTransactionProvider(af.db)),
		pvp.WithTransitionRepository(store),
		pvp.WithPrizeRemitter(newSQLPrizeRemitter(t, af.db)),
		pvp.WithTimerService(af.timers),
		pvp.WithClock(af.now),
	)
	if err != nil {
		t.Fatal(err)
	}
	af.service = svc
}

func (af *activityFixture) createAndJoin(t *testing.T, targetWins int) pvp.RoomDetail {
	t.Helper()
	ctx := context.Background()
	room, err := af.service.CreateRoom(ctx, af.leader.ID, pvp.CreateRoomRequest{
		Name:       "ActivityRecovery",
		Bet:        100,
		MaxMembers: 2,
		TargetWins: targetWins,
	})
	if err != nil {
		t.Fatal(err)
	}
	af.room = room
	af.roomIDs[room.Room.ID] = true
	joined, err := af.service.JoinRoom(ctx, af.member.ID, room.Room.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	return joined
}

func (af *activityFixture) keyTTL(t *testing.T, key string) int64 {
	t.Helper()
	ttl, err := af.client.Do(context.Background(), af.client.B().Ttl().Key(key).Build()).AsInt64()
	if err != nil {
		t.Fatalf("TTL %s: %v", key, err)
	}
	return ttl
}

func (af *activityFixture) indexScore(t *testing.T, roomID string) int64 {
	t.Helper()
	score, err := af.client.Do(context.Background(), af.client.B().Zscore().Key(pvp.DefaultRoomsActiveIndexKey).Member(roomID).Build()).AsInt64()
	if err != nil {
		t.Fatalf("ZSCORE %s: %v", roomID, err)
	}
	return score
}

func (af *activityFixture) assertConverged(t *testing.T, roomID string, wantActivity time.Time, wantVersion int, wantTTL int64) {
	t.Helper()
	ctx := context.Background()
	tr, err := af.store.FindTransition(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if tr.Pending || !tr.LastActivityAt.Equal(wantActivity) || tr.Version != wantVersion {
		t.Fatalf("SQL transition=%+v; want Pending=false LastActivityAt=%s Version=%d", tr, wantActivity, wantVersion)
	}
	live, err := af.rooms.GetRoom(ctx, roomID)
	if err != nil {
		t.Fatal(err)
	}
	if !live.Room.LastActivityAt.Equal(wantActivity) || live.Room.FinancialVersion != wantVersion {
		t.Fatalf("Valkey room=%+v; want LastActivityAt=%s FinancialVersion=%d", live.Room, wantActivity, wantVersion)
	}
	if score := af.indexScore(t, roomID); score != wantActivity.Unix() {
		t.Fatalf("active index score=%d, want %d", score, wantActivity.Unix())
	}
	roomTTL := af.keyTTL(t, pvp.DefaultRoomKeyPrefix+roomID)
	leaderTTL := af.keyTTL(t, pvp.DefaultCharacterKeyPrefix+af.leader.ID)
	memberTTL := af.keyTTL(t, pvp.DefaultCharacterKeyPrefix+af.member.ID)
	for name, got := range map[string]int64{"room": roomTTL, "leader": leaderTTL, "member": memberTTL} {
		if got < wantTTL-10 || got > wantTTL {
			t.Fatalf("%s TTL=%d, want within [%d, %d]", name, got, wantTTL-10, wantTTL)
		}
	}
	if diff := roomTTL - memberTTL; diff < -2 || diff > 2 {
		t.Fatalf("room TTL (%d) and member mapping TTL (%d) did not converge", roomTTL, memberTTL)
	}
}

func TestPvPActivityPublicationAndRecoveryIntegration(t *testing.T) {
	t0 := time.Now().UTC().Truncate(time.Second)

	for _, op := range []string{"select", "start", "advance"} {
		for _, mode := range []string{"healthy", "before-room", "after-room-zadd", "mapping"} {
			op, mode := op, mode
			t.Run(op+"/"+mode, func(t *testing.T) {
				ctx := context.Background()
				af := newActivityFixture(t, t0)
				af.createAndJoin(t, 2)
				roomID := af.room.Room.ID

				switch op {
				case "start":
					if _, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed); err != nil {
						t.Fatal(err)
					}
					if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); err != nil {
						t.Fatal(err)
					}
				case "advance":
					if _, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed); err != nil {
						t.Fatal(err)
					}
					if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); err != nil {
						t.Fatal(err)
					}
					if _, err := af.service.StartMatch(ctx, af.leader.ID, roomID); err != nil {
						t.Fatal(err)
					}
				}

				beforeTr, err := af.store.FindTransition(ctx, roomID)
				if err != nil {
					t.Fatal(err)
				}

				activityAt := t0.Add(5 * time.Minute)
				af.setNow(activityAt)
				// Mirror 5 minutes of Valkey TTL decay on existing keys before the activity write.
				for _, key := range []string{
					pvp.DefaultRoomKeyPrefix + roomID,
					pvp.DefaultCharacterKeyPrefix + af.leader.ID,
					pvp.DefaultCharacterKeyPrefix + af.member.ID,
				} {
					if err := af.client.Do(ctx, af.client.B().Expire().Key(key).Seconds(1500).Build()).Error(); err != nil {
						t.Fatal(err)
					}
				}

				var wrongTypeKey string
				var baseRooms pvp.RoomRepository
				switch mode {
				case "before-room":
					af.rooms.fail = "save"
					af.rooms.after = false
					af.rooms.armed.Store(true)
				case "after-room-zadd":
					wrongTypeKey = pvp.DefaultRoomKeyPrefix + roomID + ":zaddfault"
					if err := af.client.Do(ctx, af.client.B().Set().Key(wrongTypeKey).Value("wrongtype").Ex(pvp.DefaultLobbyTTL).Build()).Error(); err != nil {
						t.Fatal(err)
					}
					t.Cleanup(func() {
						if err := af.client.Do(context.Background(), af.client.B().Del().Key(wrongTypeKey).Build()).Error(); err != nil {
							t.Errorf("cleanup DEL %s: %v", wrongTypeKey, err)
						}
					})
					baseRooms = af.rooms.RoomRepository
					faultClient := &pvpCommandFailure{
						Client:       af.client,
						command:      "ZADD",
						key:          pvp.DefaultRoomsActiveIndexKey,
						wrongTypeKey: wrongTypeKey,
					}
					faultValkey, err := pvp.NewValkeyRoomRepository(faultClient)
					if err != nil {
						t.Fatal(err)
					}
					af.rooms.RoomRepository = faultValkey
				case "mapping":
					baseRooms = af.rooms.RoomRepository
					sel := &selectiveSetRooms{
						RoomRepository: baseRooms,
						targetCharID:   af.member.ID,
						err:            errPublication,
					}
					sel.armed.Store(true)
					af.rooms.RoomRepository = sel
				}

				invoke := func() (pvp.RoomDetail, pvp.RoundResolution, error) {
					switch op {
					case "select":
						d, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed)
						return d, pvp.RoundResolution{}, err
					case "start":
						d, err := af.service.StartMatch(ctx, af.leader.ID, roomID)
						return d, pvp.RoundResolution{}, err
					default:
						r, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID)
						return pvp.RoomDetail{}, r, err
					}
				}

				detail, roundRes, err := invoke()
				if mode == "healthy" {
					if err != nil {
						t.Fatalf("healthy %s failed: %v", op, err)
					}
					af.assertConverged(t, roomID, activityAt, beforeTr.Version+1, 1800)
					if op == "advance" && (roundRes.MatchCompleted || roundRes.Round != 2 || af.battleSpy.calls.Load() != 1) {
						t.Fatalf("unexpected healthy advance result: %+v calls=%d", roundRes, af.battleSpy.calls.Load())
					}
					_ = detail
					return
				}

				if err == nil {
					t.Fatalf("expected publication error for %s/%s", op, mode)
				}

				// Inspect persisted SQL and Valkey state immediately after partial failure.
				midTr, err := af.store.FindTransition(ctx, roomID)
				if err != nil {
					t.Fatal(err)
				}
				if !midTr.Pending || !midTr.LastActivityAt.Equal(activityAt) || midTr.Version != beforeTr.Version+1 {
					t.Fatalf("SQL state after failure=%+v; want Pending=true LastActivityAt=%s Version=%d", midTr, activityAt, beforeTr.Version+1)
				}
				if op == "advance" && (midTr.Result == nil || midTr.Result.Round != 2 || midTr.PendingPvPKills[af.leader.ID] != 1) {
					t.Fatalf("SQL did not retain non-final advance result/kills: %+v", midTr)
				}

				midRoom, err := af.rooms.GetRoom(ctx, roomID)
				if err != nil {
					t.Fatal(err)
				}
				switch mode {
				case "before-room":
					if !midRoom.Room.LastActivityAt.Equal(t0) || midRoom.Room.FinancialVersion != beforeTr.Version {
						t.Fatalf("before-room modified Valkey room unexpectedly: %+v", midRoom.Room)
					}
					if op == "select" && midRoom.Members[0].TeamColor != "" {
						t.Fatalf("before-room modified live team color: %+v", midRoom.Members[0])
					}
					if score := af.indexScore(t, roomID); score != t0.Unix() {
						t.Fatalf("before-room modified active index score: %d want %d", score, t0.Unix())
					}
					if roomTTL := af.keyTTL(t, pvp.DefaultRoomKeyPrefix+roomID); roomTTL > 1500 {
						t.Fatalf("before-room refreshed room TTL unexpectedly: %d", roomTTL)
					}
				case "after-room-zadd":
					if !midRoom.Room.LastActivityAt.Equal(activityAt) || midRoom.Room.FinancialVersion != beforeTr.Version+1 {
						t.Fatalf("after-room-zadd did not write room key: %+v", midRoom.Room)
					}
					if score := af.indexScore(t, roomID); score != t0.Unix() {
						t.Fatalf("after-room-zadd updated index unexpectedly: %d want %d", score, t0.Unix())
					}
					if leaderTTL := af.keyTTL(t, pvp.DefaultCharacterKeyPrefix+af.leader.ID); leaderTTL > 1500 {
						t.Fatalf("after-room-zadd refreshed leader TTL unexpectedly: %d", leaderTTL)
					}
				case "mapping":
					if !midRoom.Room.LastActivityAt.Equal(activityAt) || af.indexScore(t, roomID) != activityAt.Unix() {
						t.Fatalf("mapping failure expected room and index updated first: room=%+v", midRoom.Room)
					}
					leaderTTL := af.keyTTL(t, pvp.DefaultCharacterKeyPrefix+af.leader.ID)
					memberTTL := af.keyTTL(t, pvp.DefaultCharacterKeyPrefix+af.member.ID)
					if leaderTTL <= 1750 || memberTTL > 1500 {
						t.Fatalf("expected leader mapping refreshed (%d) while member mapping lagged (%d)", leaderTTL, memberTTL)
					}
				}

				// Clear fault, restart service, and advance clock 2m to verify remaining-TTL convergence on retry.
				if baseRooms != nil {
					af.rooms.RoomRepository = baseRooms
				}
				retryAt := activityAt.Add(2 * time.Minute)
				af.setNow(retryAt)
				af.restartActivity(t)

				retryDetail, retryRound, err := invoke()
				if err != nil {
					t.Fatalf("retry %s/%s failed: %v", op, mode, err)
				}
				af.assertConverged(t, roomID, activityAt, beforeTr.Version+1, 1680)

				switch op {
				case "select":
					if retryDetail.Members[0].TeamColor != pvp.ColorRed {
						t.Fatalf("select retry lost team color: %+v", retryDetail.Members[0])
					}
					if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); err != nil {
						t.Fatalf("subsequent member SelectTeam failed: %v", err)
					}
				case "start":
					if retryDetail.Room.Status != pvp.StatusInProgress || retryDetail.Room.Round != 1 {
						t.Fatalf("start retry returned wrong room state: %+v", retryDetail.Room)
					}
					if _, err := af.service.StartMatch(ctx, af.leader.ID, roomID); !errors.Is(err, pvp.ErrMatchAlreadyStarted) {
						t.Fatalf("expected ErrMatchAlreadyStarted after recovered start, got %v", err)
					}
				case "advance":
					if retryRound.MatchCompleted || retryRound.Round != 2 || retryRound.WinnerTeam != pvp.ColorRed || retryRound.Turns != 2 {
						t.Fatalf("advance retry did not preserve round 1 result: %+v", retryRound)
					}
					if af.battleSpy.calls.Load() != 1 {
						t.Fatalf("advance retry triggered second battle draw: calls=%d", af.battleSpy.calls.Load())
					}
					// Next AdvanceRound advances round 2 cleanly.
					round2, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID)
					if err != nil {
						t.Fatalf("round 2 after recovery failed: %v", err)
					}
					if !round2.MatchCompleted || af.battleSpy.calls.Load() != 2 {
						t.Fatalf("expected round 2 completion on call 2, got res=%+v calls=%d", round2, af.battleSpy.calls.Load())
					}
				}
			})
		}
	}
}

func TestPvPActivityStaleVersionAndConcurrentCleanupIntegration(t *testing.T) {
	t0 := time.Now().UTC().Truncate(time.Second)

	t.Run("stale cached version rejected for all three activity writers", func(t *testing.T) {
		ctx := context.Background()
		af := newActivityFixture(t, t0)
		af.createAndJoin(t, 2)
		roomID := af.room.Room.ID

		// Stale SelectTeam
		staleJoin, err := af.rooms.GetRoom(ctx, roomID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed); err != nil {
			t.Fatal(err)
		}
		if err := af.rooms.SaveRoom(ctx, staleJoin.Room, staleJoin.Members); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); !errors.Is(err, pvp.ErrTransitionConflict) {
			t.Fatalf("expected ErrTransitionConflict for stale SelectTeam, got %v", err)
		}

		// Restore and complete team selection, then test stale StartMatch.
		if _, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed); err == nil {
			t.Fatal("expected conflict on stale room")
		}
		freshTr, err := af.store.FindTransition(ctx, roomID)
		if err != nil {
			t.Fatal(err)
		}
		if err := af.rooms.SaveRoom(ctx, freshTr.Detail.Room, freshTr.Detail.Members); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); err != nil {
			t.Fatal(err)
		}
		stalePreStart, err := af.rooms.GetRoom(ctx, roomID)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.StartMatch(ctx, af.leader.ID, roomID); err != nil {
			t.Fatal(err)
		}
		if err := af.rooms.SaveRoom(ctx, stalePreStart.Room, stalePreStart.Members); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.StartMatch(ctx, af.leader.ID, roomID); !errors.Is(err, pvp.ErrTransitionConflict) {
			t.Fatalf("expected ErrTransitionConflict for stale StartMatch, got %v", err)
		}

		// Restore started room and test stale AdvanceRound.
		startedTr, err := af.store.FindTransition(ctx, roomID)
		if err != nil {
			t.Fatal(err)
		}
		if err := af.rooms.SaveRoom(ctx, startedTr.Detail.Room, startedTr.Detail.Members); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID); err != nil {
			t.Fatal(err)
		}
		if err := af.rooms.SaveRoom(ctx, startedTr.Detail.Room, startedTr.Detail.Members); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID); !errors.Is(err, pvp.ErrTransitionConflict) {
			t.Fatalf("expected ErrTransitionConflict for stale AdvanceRound, got %v", err)
		}
		afterTr, err := af.store.FindTransition(ctx, roomID)
		if err != nil {
			t.Fatal(err)
		}
		if afterTr.PendingPvPKills[af.leader.ID] != 1 {
			t.Fatalf("stale AdvanceRound mutated PendingPvPKills: %+v", afterTr.PendingPvPKills)
		}
	})

	t.Run("concurrent cleanup and pending activity mutation preserve escrow claims and penalty", func(t *testing.T) {
		ctx := context.Background()
		af := newActivityFixture(t, t0)
		af.createAndJoin(t, 2)
		roomID := af.room.Room.ID
		if _, err := af.service.SelectTeam(ctx, af.leader.ID, roomID, pvp.ColorRed); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.SelectTeam(ctx, af.member.ID, roomID, pvp.ColorBlue); err != nil {
			t.Fatal(err)
		}
		if _, err := af.service.StartMatch(ctx, af.leader.ID, roomID); err != nil {
			t.Fatal(err)
		}

		// Inject mapping failure on non-final AdvanceRound at T0 + 5m.
		activityAt := t0.Add(5 * time.Minute)
		af.setNow(activityAt)
		baseRooms := af.rooms.RoomRepository
		sel := &selectiveSetRooms{RoomRepository: baseRooms, targetCharID: af.member.ID, err: errPublication}
		sel.armed.Store(true)
		af.rooms.RoomRepository = sel
		if _, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID); !errors.Is(err, errPublication) {
			t.Fatalf("expected mapping publication error, got %v", err)
		}
		af.rooms.RoomRepository = baseRooms

		// Advance clock past inactivity timeout and race LeaveRoom cleanup against AdvanceRound.
		timeoutAt := activityAt.Add(pvp.InactivityTimeout + time.Second)
		af.setNow(timeoutAt)
		result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
			if worker%2 == 0 {
				_, err := af.service.LeaveRoom(ctx, af.member.ID, roomID)
				return err
			}
			_, err := af.service.AdvanceRound(ctx, af.leader.ID, roomID)
			if errors.Is(err, pvp.ErrRoomNotFound) {
				return nil
			}
			return err
		})
		if result.Failures != 0 || result.Successes == 0 {
			t.Fatalf("concurrent cleanup + advance failed: %+v", result)
		}

		tr, err := af.store.FindTransition(ctx, roomID)
		wantUntil := timeoutAt.Add(pvp.InactivitySleepDuration)
		if err != nil || tr.Pending || tr.Kind != "timeout" || tr.Detail.Room.Status != pvp.StatusDisbanded || tr.ForfeitedPool != 200 || !tr.PenaltyUntil.Equal(wantUntil) {
			t.Fatalf("unexpected timeout transition: %+v err=%v", tr, err)
		}
		gotLeader, _ := af.chars.FindByID(ctx, af.leader.ID)
		gotMember, _ := af.chars.FindByID(ctx, af.member.ID)
		if gotLeader.Money != 900 || gotMember.Money != 900 || gotLeader.PvPWins != 1 || gotMember.PvPWins != 0 || !gotLeader.PendingWake || !gotMember.PendingWake {
			t.Fatalf("unexpected character state after timeout: leader=%+v member=%+v", gotLeader, gotMember)
		}
		for _, actor := range []string{af.leader.ID, af.member.ID} {
			if claim, err := af.service.GetCharacterClaim(ctx, actor); err != nil || claim != "" {
				t.Fatalf("actor %s retained claim: %q %v", actor, claim, err)
			}
			if exp := pexpireAt(t, af.client, timer.PrefixTimer+timer.CategorySleep+":"+actor); exp != wantUntil.UnixMilli() {
				t.Fatalf("actor %s sleep=%d want %d", actor, exp, wantUntil.UnixMilli())
			}
		}
	})
}
