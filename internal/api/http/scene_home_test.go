package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type homeSceneFixture struct {
	*homeGatewayFixture
	viewService *home.Service
	repo        *homeSceneRepository
}

type homeSceneRepository struct {
	homeGatewayRepository
	readErr      error
	privateReads int
	letters      []home.Letter
}

func (r *homeSceneRepository) GetHome(ctx context.Context, id string) (home.CharacterHome, error) {
	if r.readErr != nil {
		return home.CharacterHome{}, r.readErr
	}
	return home.CharacterHome{CharacterID: id, CompanionName: "ペット"}, nil
}
func (r *homeSceneRepository) GetUnreadLetterCount(context.Context, string) (int, error) {
	r.privateReads++
	return 1, nil
}
func (r *homeSceneRepository) ListCompanionPhrases(context.Context, string) ([]home.CompanionPhrase, error) {
	r.privateReads++
	return nil, nil
}
func (r *homeSceneRepository) ListDeliveryNotices(context.Context, string, bool) ([]home.DeliveryNotice, error) {
	r.privateReads++
	return nil, nil
}
func (r *homeSceneRepository) ListInboxLetters(ctx context.Context, actor string, limit, offset int) ([]home.Letter, int, error) {
	if actor != "hero" {
		r.fixture.t.Fatalf("mailbox read for target %s", actor)
	}
	page := pagination.SlicePage(r.letters, limit, offset)
	return page.Items, page.Total, r.readErr
}
func (r *homeSceneRepository) ListOutboxLetters(ctx context.Context, actor string, limit, offset int) ([]home.Letter, int, error) {
	return r.ListInboxLetters(ctx, actor, limit, offset)
}
func (r *homeSceneRepository) ListInboxLettersByCursor(ctx context.Context, actor string, limit int, before time.Time, beforeID string) ([]home.Letter, error) {
	if actor != "hero" {
		r.fixture.t.Fatalf("cursor read for target %s", actor)
	}
	start := 0
	for i, l := range r.letters {
		if l.ID == beforeID {
			start = i + 1
		}
	}
	return r.letters[start:min(start+limit, len(r.letters))], r.readErr
}
func (r *homeSceneRepository) ListOutboxLettersByCursor(ctx context.Context, actor string, limit int, before time.Time, beforeID string) ([]home.Letter, error) {
	return r.ListInboxLettersByCursor(ctx, actor, limit, before, beforeID)
}

func (f *homeSceneFixture) GetHomeView(ctx context.Context, target, viewer, player string) (home.HomeView, error) {
	return f.viewService.GetHomeView(ctx, target, viewer, player)
}
func (f *homeSceneFixture) ListInbox(ctx context.Context, actor string, limit, offset int) (home.LetterListResult, error) {
	return f.viewService.ListInbox(ctx, actor, limit, offset)
}
func (f *homeSceneFixture) ListOutbox(ctx context.Context, actor string, limit, offset int) (home.LetterListResult, error) {
	return f.viewService.ListOutbox(ctx, actor, limit, offset)
}
func (f *homeSceneFixture) ListInboxByCursor(ctx context.Context, actor string, limit int, cursor string) (pagination.CursorPage[home.Letter], error) {
	return f.viewService.ListInboxByCursor(ctx, actor, limit, cursor)
}
func (f *homeSceneFixture) ListOutboxByCursor(ctx context.Context, actor string, limit int, cursor string) (pagination.CursorPage[home.Letter], error) {
	return f.viewService.ListOutboxByCursor(ctx, actor, limit, cursor)
}

func homeSceneRouter(t *testing.T) (*homeSceneFixture, *gatewayNavigationStore, http.Handler) {
	f := &homeSceneFixture{homeGatewayFixture: newHomeGatewayFixture(t)}
	f.expectedContext = nil
	f.repo = &homeSceneRepository{homeGatewayRepository: homeGatewayRepository{fixture: f.homeGatewayFixture}, letters: []home.Letter{
		{ID: "letter-2", Content: "private letter", CreatedAt: time.Unix(2, 0)}, {ID: "letter-1", Content: "older letter", CreatedAt: time.Unix(1, 0)},
	}}
	var err error
	f.viewService, err = home.NewService(f.repo, f.homeGatewayFixture)
	if err != nil {
		t.Fatal(err)
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "home"}}
	service := playercontext.NewService(f.homeGatewayFixture, f.gatewayFixture, f.timers, playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"},
		playercontext.SceneDefinition{ID: "home", Parent: "town", SubjectKind: "home", SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
			_, err := f.viewService.GetHomeView(ctx, target, "", "")
			if errors.Is(err, home.ErrCharacterNotFound) {
				return false, nil
			}
			return err == nil, err
		}},
		playercontext.SceneDefinition{ID: "home_inbox", Parent: "home", Pageable: true, CursorPageable: true},
		playercontext.SceneDefinition{ID: "home_outbox", Parent: "home", Pageable: true, CursorPageable: true},
	))
	h, err := NewHandler(f.gatewayFixture, f.homeGatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithHome(f), WithPlayerContext(service))
	if err != nil {
		t.Fatal(err)
	}
	return f, store, h.Router()
}

func decodeHomeData(t *testing.T, response PlayerContextResponse) HomeSceneData {
	t.Helper()
	raw, err := json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data HomeSceneData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}
func decodeMailboxData(t *testing.T, response PlayerContextResponse) HomeMailboxSceneData {
	t.Helper()
	raw, err := json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	var data HomeMailboxSceneData
	if err := json.Unmarshal(raw, &data); err != nil {
		t.Fatal(err)
	}
	return data
}

func TestHomeScenesPublicOwnedTargetsAndRecovery(t *testing.T) {
	f, store, router := homeSceneRouter(t)
	status, response := navigationGET(t, router)
	data := decodeHomeData(t, response)
	if status != 200 || !data.View.IsOwner || data.View.Private == nil || len(data.Destinations) != 2 || response.Scene.Support.Observation != "details" {
		t.Fatalf("owned: %d %+v", status, response)
	}
	before := f.repo.privateReads
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, nil)
	if status != 200 {
		t.Fatalf("select: %d %s", status, raw)
	}
	refreshed := homeObservation(t, raw)
	status, response = navigationGET(t, router)
	data = decodeHomeData(t, response)
	if status != 200 || !reflect.DeepEqual(refreshed.Scene, response.Scene) || response.Character.ID != "hero" || data.View.Owner.ID != "other" || data.View.IsOwner || data.View.Private != nil || len(data.Destinations) != 0 || f.repo.privateReads != before {
		t.Fatalf("public: %+v", response)
	}
	for _, a := range response.AvailableActions {
		if a.Action == "home_sleep" && a.ParamsTemplate["target_home_id"] != "other" {
			t.Fatal("target became actor")
		}
	}
	// A saved target remains explicit when it disappears; GET does not fallback.
	store.selection.Subject.ID = "missing"
	status, response = navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "selection_unavailable" || hasHomeAction(response, "home_sleep") {
		t.Fatalf("missing: %+v", response)
	}
	store.selection = playercontext.Selection{}
	status, response = navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "town" {
		t.Fatal("expired selection did not return town")
	}
	store.afterSave = func() { f.repo.readErr = errors.New("required home read") }
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" {
		t.Fatalf("known outcome: %d %s", status, raw)
	}
	writes := store.writes
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatal("hidden enrichment failure")
	}
	f.repo.readErr = nil
	if status, _ := navigationGET(t, router); status != 200 || store.writes != writes || f.executions != 0 || f.updates != 0 {
		t.Fatal("GET recovery mutated/replayed")
	}
}

func TestHomeRESTVisitorSpoofing(t *testing.T) {
	f, _, router := homeSceneRouter(t)
	for _, tc := range []struct {
		path, token string
		status      int
	}{
		{"/homes/other", "", 200}, {"/homes/other?visitor_id=other", "", 401}, {"/homes/other?visitor_id=other", "session", 403}, {"/homes/hero?visitor_id=hero", "session", 200},
	} {
		r := httptest.NewRequest("GET", tc.path, nil)
		if tc.token != "" {
			r.Header.Set("Authorization", "Bearer "+tc.token)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if tc.path == "/homes/other" && (strings.Contains(w.Body.String(), "private") || strings.Contains(w.Body.String(), "player_id")) {
			t.Fatal("public leaked")
		}
	}
	f.repo.readErr = home.ErrForbidden
	r := httptest.NewRequest("GET", "/homes/hero?visitor_id=hero", nil)
	r.Header.Set("Authorization", "Bearer session")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("service authorization: %d %s", w.Code, w.Body.String())
	}
}

func TestHomeMailboxRejectsSpoofedPaging(t *testing.T) {
	_, store, router := homeSceneRouter(t)
	store.selection.Destination = "home_inbox"
	for _, params := range []string{
		`{"destination":"home_inbox","offset":0,"cursor":""}`,
		`{"destination":"home_inbox","cursor":null}`,
		`{"destination":"home_inbox","cursor":"` + strings.Repeat("a", 513) + `"}`,
		`{"destination":"home_inbox","cursor":"invalid cursor"}`,
		`{"destination":"home_inbox","offset":0,"visitor_id":"other"}`,
		`{"destination":"home_inbox","offset":0,"character_id":"other"}`,
	} {
		status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":`+params+`}`, nil)
		if status != 400 || store.writes != 0 {
			t.Fatalf("spoof: %d %s", status, raw)
		}
	}
	status, raw := gatewayRequest(t, router, "other", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_inbox"}}`, nil)
	if status != 403 || store.writes != 0 {
		t.Fatalf("owner: %d %s", status, raw)
	}
}

func TestHomeMailboxPagingUsesActorAndNeverReadsLetters(t *testing.T) {
	for _, destination := range []string{"home_inbox", "home_outbox"} {
		for _, mode := range []string{"offset", "cursor"} {
			t.Run(destination+mode, func(t *testing.T) {
				f, store, router := homeSceneRouter(t)
				store.selection = playercontext.Selection{Destination: destination, Limit: 1}
				if mode == "cursor" {
					status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":{"destination":"`+destination+`","cursor":"","limit":1}}`, nil)
					if status != 200 || store.selection.Cursor == nil {
						t.Fatalf("cursor entry: %d %s", status, raw)
					}
				}
				status, response := navigationGET(t, router)
				data := decodeMailboxData(t, response)
				if status != 200 || len(data.Letters) != 1 || data.Page.Mode != mode || data.Page.Next == nil || data.Letters[0].IsRead {
					t.Fatalf("first: %+v", data)
				}
				body, err := json.Marshal(map[string]any{"action": "scene_page", "params": data.Page.Next})
				if err != nil {
					t.Fatal(err)
				}
				status, raw := gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil)
				if status != 200 {
					t.Fatalf("next: %d %s", status, raw)
				}
				refreshed := homeObservation(t, raw)
				status, response = navigationGET(t, router)
				data = decodeMailboxData(t, response)
				wantWrites := 1
				if mode == "cursor" {
					wantWrites = 2
				}
				if status != 200 || !reflect.DeepEqual(response.Scene, refreshed.Scene) || len(data.Letters) != 1 || data.Letters[0].ID != "letter-1" || data.Page.Next != nil || store.writes != wantWrites {
					t.Fatalf("second: %+v", data)
				}
				f.repo.letters = nil
				status, response = navigationGET(t, router)
				data = decodeMailboxData(t, response)
				if status != 200 || data.Letters == nil || len(data.Letters) != 0 || f.executions != 0 || f.updates != 0 {
					t.Fatal("empty/mutation")
				}
				f.repo.readErr = errors.New("mailbox failure")
				if status, _ := navigationGET(t, router); status != 500 {
					t.Fatal("hidden mailbox failure")
				}
			})
		}
	}
}
