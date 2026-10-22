package gvg_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Only authentication and character reads are used by these retained HTTP routes.
type retryHTTPServices struct {
	apihttp.PlayerService
	apihttp.CharacterService
	chars *database.CharacterRepository
}

func (s retryHTTPServices) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	return coreplayer.Player{ID: token}, nil
}
func (s retryHTTPServices) Get(ctx context.Context, id string) (corecharacter.Character, error) {
	return s.chars.FindByID(ctx, id)
}

func TestGvGExpectedRoundHTTPReplayIntegration(t *testing.T) {
	for _, tc := range []struct {
		name          string
		target, round int
		outcome       corebattle.Outcome
		gp, otherGP   int64
		wins, draws   int
	}{
		{"intermediate", 2, 1, corebattle.OutcomeWin, 3, 0, 0, 0},
		{"final_win", 1, 1, corebattle.OutcomeWin, 7, 4, 1, 0},
		{"final_draw", 3, gvg.MaxRounds, corebattle.OutcomeDraw, 0, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRewardFixture(t, tc.target, tc.round, tc.outcome)
			ctx := context.Background()
			chars, err := database.NewCharacterRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			deps := retryHTTPServices{chars: chars}
			h, err := apihttp.NewHandler(deps, deps, struct{ apihttp.AdventureService }{}, struct{ apihttp.ShopService }{}, apihttp.WithGvG(f.service))
			if err != nil {
				t.Fatal(err)
			}
			router := h.Router()
			leader, err := chars.FindByID(ctx, f.detail.Room.LeaderCharacterID)
			if err != nil {
				t.Fatal(err)
			}
			opponent, err := chars.FindByID(ctx, f.detail.Members[1].CharacterID)
			if err != nil {
				t.Fatal(err)
			}
			request := func(actor, owner string, round int) *httptest.ResponseRecorder {
				req := httptest.NewRequest(http.MethodPost, fmt.Sprintf("/characters/%s/gvg/rooms/%s/advance", actor, f.detail.Room.ID), bytes.NewBufferString(fmt.Sprintf(`{"expected_round":%d}`, round)))
				req.Header.Set("Content-Type", "application/json")
				req.Header.Set("Authorization", "Bearer "+owner)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)
				return rec
			}
			checkForbidden := func() {
				t.Helper()
				for _, ids := range [][2]string{{leader.ID, opponent.PlayerID}, {opponent.ID, opponent.PlayerID}} {
					if rec := request(ids[0], ids[1], tc.round); rec.Code != 403 {
						t.Fatalf("ownership/leader: %d %s", rec.Code, rec.Body.String())
					}
				}
			}
			checkForbidden()
			// Deliberately discard a completed HTTP response; this is not a network-fault simulation.
			if rec := request(leader.ID, leader.PlayerID, tc.round); rec.Code != 200 {
				t.Fatalf("advance: %d %s", rec.Code, rec.Body.String())
			}
			saved, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID)
			if err != nil || !found || saved.ResolvedRound != tc.round {
				t.Fatalf("receipt: %+v %v %v", saved, found, err)
			}
			key := gvg.DefaultRoomKeyPrefix + f.detail.Room.ID
			if err := f.client.Do(ctx, f.client.B().Expire().Key(key).Seconds(60).Build()).Error(); err != nil {
				t.Fatal(err)
			}
			if _, err := f.service.GetRoom(ctx, f.detail.Room.ID); err != nil {
				t.Fatal(err)
			}
			rec := request(leader.ID, leader.PlayerID, tc.round)
			var result gvg.RoundResolution
			if err := json.Unmarshal(rec.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if rec.Code != 200 || !reflect.DeepEqual(result, saved.Result) || f.battle.calls != 1 {
				t.Fatalf("replay: status=%d result=%+v calls=%d", rec.Code, result, f.battle.calls)
			}
			checkForbidden()
			if ttl, err := f.client.Do(ctx, f.client.B().Ttl().Key(key).Build()).AsInt64(); err != nil || ttl <= 0 || ttl > 60 {
				t.Fatalf("GET/replay renewed TTL: %d %v", ttl, err)
			}
			f.assertRewards(t, tc.gp, tc.otherGP, tc.wins, tc.draws)
			for _, expired := range []bool{true, false} {
				if expired {
					if err := f.client.Do(ctx, f.client.B().Expire().Key(key).Seconds(0).Build()).Error(); err != nil {
						t.Fatal(err)
					}
				} else {
					if err := f.rooms.SaveRoom(ctx, saved.Detail.Room, saved.Detail.Members); err != nil {
						t.Fatal(err)
					}
					if err := f.service.LeaveRoom(ctx, leader.ID, f.detail.Room.ID); err != nil {
						t.Fatal(err)
					}
				}
				if rec := request(leader.ID, leader.PlayerID, tc.round); rec.Code != 404 {
					t.Fatalf("missing room resurrected: %d %s", rec.Code, rec.Body.String())
				}
				if _, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID); err != nil || !found {
					t.Fatalf("missing room erased SQL guard: %v %v", found, err)
				}
			}
			if f.battle.calls != 1 {
				t.Fatal("replay or authorization reran combat")
			}
		})
	}
}

func TestGvGExpectedRoundConcurrentIntegration(t *testing.T) {
	for _, tc := range []struct {
		name          string
		target, round int
		outcome       corebattle.Outcome
		gp, otherGP   int64
		wins, draws   int
	}{
		{"intermediate", 3, 1, corebattle.OutcomeWin, 3, 0, 0, 0},
		{"final_win", 1, 1, corebattle.OutcomeWin, 7, 4, 1, 0},
		{"final_draw", 3, gvg.MaxRounds, corebattle.OutcomeDraw, 0, 0, 0, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRewardFixture(t, tc.target, tc.round, tc.outcome)
			ctx := context.Background()
			stress := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
				result, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, tc.round)
				if err != nil {
					return err
				}
				saved, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID)
				if err != nil {
					return err
				}
				if !found || !reflect.DeepEqual(result, saved.Result) {
					return fmt.Errorf("concurrent replies disagree: %+v", result)
				}
				return nil
			})
			if stress.Successes != stress.TotalOps || f.battle.calls != 1 {
				t.Fatalf("retry stress: %+v calls=%d", stress, f.battle.calls)
			}
			f.assertRewards(t, tc.gp, tc.otherGP, tc.wins, tc.draws)
			if tc.target != 3 || tc.round != 1 {
				return
			}
			first, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 1)
			if err != nil {
				t.Fatal(err)
			}
			stress = testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, _ int) error {
				expected := 1 + worker%2
				result, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, expected)
				if expected == 1 {
					if errors.Is(err, gvg.ErrRoundMismatch) {
						return nil
					}
					if err == nil && !reflect.DeepEqual(first, result) {
						return fmt.Errorf("old retry executed the next round: %+v", result)
					}
				} else if err == nil && result.Round != 3 {
					return fmt.Errorf("next request got preceding result: %+v", result)
				}
				return err
			})
			if stress.Successes != stress.TotalOps || f.battle.calls != 2 {
				t.Fatalf("old/next race: %+v calls=%d", stress, f.battle.calls)
			}
			f.assertRewards(t, 6, 0, 0, 0)
		})
	}
}

type uncertainStandingRepo struct {
	gvg.StandingRepository
	commitErr, readErr error
}

func (r uncertainStandingRepo) FindRoundTransition(ctx context.Context, roomID string) (gvg.RoundTransition, bool, error) {
	if r.readErr != nil {
		return gvg.RoundTransition{}, false, r.readErr
	}
	return r.StandingRepository.FindRoundTransition(ctx, roomID)
}
func (r uncertainStandingRepo) CommitRoundTransition(ctx context.Context, roomID string, round int, next gvg.RoundTransition) (gvg.RoundTransition, error) {
	saved, err := r.StandingRepository.CommitRoundTransition(ctx, roomID, round, next)
	if err != nil {
		return gvg.RoundTransition{}, err
	}
	return saved, r.commitErr
}

func TestGvGExpectedRoundUncertainCommitIntegration(t *testing.T) {
	for _, stage := range []string{"sql_ack", "room_write", "room_ack"} {
		t.Run(stage, func(t *testing.T) {
			f := newRewardFixture(t, 2, 1, corebattle.OutcomeWin)
			ctx := context.Background()
			chars, err := database.NewCharacterRepository(f.db)
			if err != nil {
				t.Fatal(err)
			}
			standings := uncertainStandingRepo{StandingRepository: f.standings}
			switch stage {
			case "sql_ack":
				standings.commitErr = errRoomPublication
			case "room_write":
				f.rooms.fail = true
			case "room_ack":
				f.rooms.failAfterSave = true
			}
			svc, err := gvg.NewService(f.rooms, standings, newMockGuildRepo(), chars, f.battle)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := svc.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 1); !errors.Is(err, errRoomPublication) {
				t.Fatalf("uncertain result: %v", err)
			}
			saved, found, err := f.standings.FindRoundTransition(ctx, f.detail.Room.ID)
			if err != nil || !found || saved.ResolvedRound != 1 || saved.Result.Round != 2 {
				t.Fatalf("committed identity: %+v %v %v", saved, found, err)
			}
			f.assertRewards(t, 3, 0, 0, 0)
			if stage == "room_write" {
				if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 2); !errors.Is(err, errRoomPublication) {
					t.Fatalf("next bypassed failed publication: %v", err)
				}
				if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 3); !errors.Is(err, gvg.ErrRoundMismatch) {
					t.Fatalf("future request: %v", err)
				}
				f.rooms.fail = false
				if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 2); !errors.Is(err, gvg.ErrRoundMismatch) {
					t.Fatalf("recovery satisfied a different request: %v", err)
				}
				room, err := f.rooms.GetRoom(ctx, f.detail.Room.ID)
				if err != nil || !reflect.DeepEqual(room, saved.Detail) {
					t.Fatalf("pending publication not recovered: %+v %v", room, err)
				}
			}
			// For room_ack the write still returns an error: replay must not write again.
			result, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 1)
			if err != nil || !reflect.DeepEqual(result, saved.Result) || f.battle.calls != 1 {
				t.Fatalf("unknown outcome recovery: %+v %v calls=%d", result, err, f.battle.calls)
			}
			f.assertRewards(t, 3, 0, 0, 0)
			// Existing payloads contain no operation number; SQL supplies it without migration.
			var payload []byte
			if err := f.db.QueryRowContext(ctx, "SELECT transition_json FROM gvg_match_transitions WHERE room_id = ?", f.detail.Room.ID).Scan(&payload); err != nil {
				t.Fatal(err)
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(payload, &fields); err != nil {
				t.Fatal(err)
			}
			if _, exists := fields["ResolvedRound"]; exists {
				t.Fatal("identity was stored in JSON instead of reading existing SQL round_number")
			}
			f.rooms.getErr = errRoomPublication
			if _, err := f.service.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 1); !errors.Is(err, errRoomPublication) {
				t.Fatalf("room storage failure: %v", err)
			}
			f.rooms.getErr = nil
			broken, err := gvg.NewService(f.rooms, uncertainStandingRepo{StandingRepository: f.standings, readErr: errRoomPublication}, newMockGuildRepo(), chars, f.battle)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := broken.AdvanceRound(ctx, f.detail.Room.LeaderCharacterID, f.detail.Room.ID, 1); !errors.Is(err, errRoomPublication) {
				t.Fatalf("SQL storage failure: %v", err)
			}
			f.assertRewards(t, 3, 0, 0, 0)
		})
	}
}
