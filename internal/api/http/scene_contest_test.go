package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type contestFixture struct {
	*gatewayFixture
	overview    contest.ContestOverview
	overviewErr error
	photos      []contest.Photo
	photosErr   error
	entries     []contest.ContestEntry
	entryErr    error
}

func (f *contestFixture) GetOverview(context.Context) (contest.ContestOverview, error) {
	return f.overview, f.overviewErr
}
func (f *contestFixture) SavePhoto(context.Context, string, string, string, string, string, string) (contest.Photo, error) {
	return contest.Photo{}, nil
}
func (f *contestFixture) ListPhotos(context.Context, string) ([]contest.Photo, error) {
	return f.photos, f.photosErr
}
func (f *contestFixture) DeletePhoto(ctx context.Context, actor, _ string) error {
	_, err := f.execute(ctx, actor, 0)
	return err
}
func (f *contestFixture) EnterContest(ctx context.Context, actor, photoID, title string) (contest.ContestEntry, error) {
	_, err := f.execute(ctx, actor, 0)
	if err != nil && !errors.Is(err, contest.ErrScheduleFailed) {
		return contest.ContestEntry{}, err
	}
	return contest.ContestEntry{ID: "e1", Round: 4, PhotoID: photoID, Title: title}, f.entryErr
}
func (f *contestFixture) Vote(ctx context.Context, actor, entryID, comment string) (contest.ContestVote, error) {
	_, err := f.execute(ctx, actor, 0)
	return contest.ContestVote{ID: "v1", Round: 3, EntryID: entryID, Comment: comment}, err
}
func (f *contestFixture) GetCurrentEntries(context.Context) ([]contest.ContestEntry, error) {
	return f.entries, nil
}
func (f *contestFixture) GetPastResults(context.Context) (*contest.ContestRound, []contest.ContestEntry, error) {
	return nil, nil, nil
}
func (f *contestFixture) GetLegends(context.Context, int, int) (pagination.Page[contest.ContestLegend], error) {
	return pagination.Page[contest.ContestLegend]{}, nil
}
func (f *contestFixture) SettleContest(context.Context, bool) (contest.SettlementResult, error) {
	return contest.SettlementResult{}, nil
}
func (f *contestFixture) ReconcileSettlementSchedule(context.Context) error { return nil }

func (f *contestFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "contest", Parent: "town"}))
	opts := append([]Option{WithContest(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newContestFixture(t *testing.T) *contestFixture {
	return &contestFixture{gatewayFixture: newGatewayFixture(t),
		overview: contest.ContestOverview{ActiveRound: &contest.ContestRound{Round: 3, Status: contest.StatusActive}, MinEntries: 5},
		photos:   []contest.Photo{{ID: "p-new", Title: "新"}, {ID: "p-old", Title: "旧"}},
		entries:  []contest.ContestEntry{{ID: "e9", Round: 3, Title: "夕焼け"}}}
}

func TestContestSceneObservesRoundAndPhotos(t *testing.T) {
	f := newContestFixture(t)
	store, router := f.router(t, "contest", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[ContestSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "フォトコン会場" || observation.Scene.Support.Observation != "details" ||
		data.Parent != "town" || data.ActiveRound == nil || data.ActiveRound.Round != 3 || len(data.Photos) != 2 {
		t.Fatalf("contest: %d %+v", status, data)
	}
	for _, action := range []string{"contest_enter", "contest_vote", "contest_photo_delete", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s", action)
		}
	}
	for _, fail := range []func(){
		func() { f.overviewErr = errors.New("private round read") },
		func() { f.overviewErr, f.photosErr = nil, errors.New("private photo read") },
	} {
		fail()
		if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
			t.Fatalf("partial observation: %d", status)
		}
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestContestCommandReplies(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		entryErr       error
		want           string
		warning        bool
	}{
		{"contest_enter", `{"photo_id":"p-new","title":"春光"}`, nil, "第4回フォトコンテストに『春光』という題名でエントリーしました", false},
		{"contest_enter", `{"photo_id":"p-new","title":"春光"}`, fmt.Errorf("%w: queue", contest.ErrScheduleFailed), "第4回フォトコンテストに『春光』という題名でエントリーしました", true},
		{"contest_vote", `{"entry_id":"e9","comment":"すてき"}`, nil, "夕焼けに投票しました", false},
		{"contest_vote", `{"entry_id":"e9"}`, nil, "夕焼けに投票しました", false},
		{"contest_photo_delete", `{"photo_id":"p-old"}`, nil, "2枚目のスクリーンショットを削除しましたわよ", false},
	} {
		t.Run(tc.action+tc.want, func(t *testing.T) {
			f := newContestFixture(t)
			f.entryErr = tc.entryErr
			_, router := f.router(t, "contest")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			got := tavernResult[ContestResult](t, raw)
			if status != 200 || got.Message != tc.want || (got.Warning != "") != tc.warning {
				t.Fatalf("%s: %d %+v", tc.action, status, got)
			}
		})
	}
}

func TestContestCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		err            error
		code, text     string
		status         int
	}{
		{"contest_enter", `{"photo_id":"p","title":""}`, contest.ErrInvalidTitle, "CONTEST_REFUSED", "題名を記入してください", 409},
		{"contest_enter", `{"photo_id":"p","title":"春 光"}`, contest.ErrInvalidTitle, "CONTEST_REFUSED", "題名に不正な空白が含まれています", 409},
		{"contest_enter", `{"photo_id":"p","title":" 春光"}`, contest.ErrInvalidTitle, "CONTEST_REFUSED", "題名に不正な空白が含まれています", 409},
		{"contest_enter", `{"photo_id":"p","title":"春＠光"}`, contest.ErrInvalidTitle, "CONTEST_REFUSED", "題名に不正な文字( ＠ )が含まれています", 409},
		{"contest_enter", `{"photo_id":"p","title":"a<b"}`, contest.ErrInvalidTitle, "CONTEST_REFUSED", `題名に不正な文字( ,;"'&<>@ )が含まれています`, 409},
		{"contest_enter", `{"photo_id":"p","title":"` + strings.Repeat("あ", 41) + `"}`, contest.ErrTitleTooLong, "CONTEST_REFUSED", "題名は40文字までです", 409},
		{"contest_enter", `{"photo_id":"p","title":"夏"}`, contest.AlreadyEnteredError{Title: "春光"}, "CONTEST_ALREADY_ENTERED", "【春光】という作品で、すでにエントリー済みです", 409},
		{"contest_enter", `{"photo_id":"p","title":"夏"}`, contest.ErrDuplicateTitle, "CONTEST_DUPLICATE_TITLE", "同じタイトルの作品がすでにエントリーされています", 409},
		{"contest_enter", `{"photo_id":"p","title":"夏"}`, contest.ErrConsecutiveEntryDisallowed, "CONTEST_CONSECUTIVE_ENTRY", "連続でコンテストにエントリーすることはできません", 409},
		{"contest_vote", `{"entry_id":"e9"}`, contest.ErrAlreadyVoted, "CONTEST_REFUSED", "第3回のフォトコンテストには、すでに投票済みです", 409},
		{"contest_vote", `{"entry_id":"x"}`, contest.ErrEntryNotFound, "CONTEST_ENTRY_NOT_FOUND", "", 400},
		{"contest_photo_delete", `{"photo_id":"x"}`, contest.ErrPhotoNotFound, "CONTEST_PHOTO_NOT_FOUND", "", 400},
		{"contest_enter", `{"photo_id":"p","title":"夏"}`, errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.action+tc.code+tc.params, func(t *testing.T) {
			f := newContestFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, "contest")
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			var detail ErrorDetail
			if err := json.Unmarshal(got["error"], &detail); err != nil {
				t.Fatal(err)
			}
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(detail.Message, tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestContestCommandsRequireTheHall(t *testing.T) {
	for _, scene := range []string{"bank", "town"} {
		f := newContestFixture(t)
		_, router := f.router(t, scene)
		if status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("contest_vote", `{"entry_id":"e9"}`), nil); status != 409 || f.executions != 0 {
			t.Fatalf("vote from %s: %d %s", scene, status, got)
		}
	}
}
