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

	"github.com/witchcraze/party2re/internal/chapel"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type chapelReadRepository struct {
	chapel.Repository
	fixture       *gatewayFixture
	blessing      chapel.CharacterBlessing
	reads, failAt int
}

func (r *chapelReadRepository) GetBlessing(ctx context.Context, actor string) (chapel.CharacterBlessing, error) {
	r.fixture.checkContext(ctx)
	if actor != "hero" {
		r.fixture.t.Fatal("selected target replaced Chapel observation actor")
	}
	r.reads++
	if r.reads == r.failAt {
		return chapel.CharacterBlessing{}, errors.New("blessing read failed")
	}
	return r.blessing, nil
}

func chapelReadRouter(t *testing.T, configured bool) (*gatewayFixture, *chapelReadRepository, *gatewayNavigationStore, http.Handler) {
	t.Helper()
	f := newGatewayFixture(t)
	f.char.Name = "Hero"
	repo := &chapelReadRepository{fixture: f, blessing: chapel.CharacterBlessing{CharacterID: "hero", ActiveBlessing: chapel.BlessingNone}}
	var service ChapelService
	if configured {
		var err error
		service, err = chapel.NewService(repo)
		if err != nil {
			t.Fatal(err)
		}
	}
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "chapel"}}
	pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town"}, playercontext.SceneDefinition{ID: "chapel", Parent: "town"}))
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithChapel(service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	return f, repo, store, h.Router()
}

func TestRetiredChapelRoutesDoNotReadOrExecute(t *testing.T) {
	for _, configured := range []bool{false, true} {
		f, repo, store, router := chapelReadRouter(t, configured)
		for _, route := range []struct{ method, suffix string }{{http.MethodGet, ""}, {http.MethodPost, "/pray"}, {http.MethodPost, "/donate"}} {
			for _, actor := range []struct{ id, token string }{{"hero", ""}, {"hero", "session"}, {"other", "session"}, {"missing", "session"}} {
				r := httptest.NewRequest(route.method, "/characters/"+actor.id+"/chapel"+route.suffix, strings.NewReader(`{"blessing":"GOLD"}`))
				r.Header.Set("Authorization", "Bearer "+actor.token)
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				router.ServeHTTP(w, r)
				if w.Code != 404 || repo.reads != 0 || f.characterCalls != 0 || f.executions != 0 || store.writes != 0 {
					t.Fatalf("retired GET reached a service: configured=%t actor=%s status=%d reads=%d body=%s", configured, actor.id, w.Code, repo.reads, w.Body.String())
				}
			}
		}
	}
}

func TestChapelContextPreservesStatusFieldsWithoutMutation(t *testing.T) {
	stamp := time.Date(2026, 10, 9, 23, 30, 0, 0, chapel.JST).UTC()
	for _, kind := range []chapel.BlessingType{chapel.BlessingNone, chapel.BlessingGold, chapel.BlessingExp, chapel.BlessingMonster, chapel.BlessingDrop, chapel.BlessingCasino} {
		for _, prayed := range []bool{false, true} {
			if kind != chapel.BlessingNone && !prayed {
				continue
			}
			t.Run(string(kind)+"/"+map[bool]string{false: "absent", true: "retained record"}[prayed], func(t *testing.T) {
				f, repo, store, router := chapelReadRouter(t, true)
				repo.blessing.ActiveBlessing = kind
				if prayed {
					repo.blessing.PrayedAt, repo.blessing.UpdatedAt = stamp, stamp.Add(time.Minute)
				}
				before := repo.blessing
				svc, err := chapel.NewService(repo)
				if err != nil {
					t.Fatal(err)
				}
				want, err := svc.GetStatus(context.Background(), "hero", f.char.Name)
				if err != nil {
					t.Fatal(err)
				}
				repo.reads = 0
				f.expectedContext = context.WithValue(context.Background(), struct{}{}, "Chapel read")
				for range 2 {
					r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/hero/context", nil).WithContext(f.expectedContext)
					r.Header.Set("Authorization", "Bearer session")
					w := httptest.NewRecorder()
					router.ServeHTTP(w, r)
					if w.Code != 200 {
						t.Fatalf("context status %d: %s", w.Code, w.Body.String())
					}
					assertGatewayContext(t, map[string]json.RawMessage{"context": w.Body.Bytes()})
					var observed PlayerContextResponse
					if err := json.Unmarshal(w.Body.Bytes(), &observed); err != nil {
						t.Fatal(err)
					}
					data := decodeShopScene[ChapelSceneData](t, observed)
					if data.Parent != "town" || !reflect.DeepEqual(data.Blessing, before) || !reflect.DeepEqual(data.Status, want) {
						t.Fatalf("lost Chapel fields: %+v; blessing=%+v status=%+v", data, before, want)
					}
				}
				if repo.reads != 4 || repo.blessing != before || store.writes != 0 || f.executions != 0 {
					t.Fatal("GET mutated prayer, quota or navigation")
				}
			})
		}
	}
}

func TestChapelContextReadBoundary(t *testing.T) {
	for _, condition := range []string{"anonymous", "foreign", "missing", "snapshot owner", "profile owner", "blessing error", "status error", "unconfigured", "sleep", "work"} {
		t.Run(condition, func(t *testing.T) {
			f, repo, store, router := chapelReadRouter(t, condition != "unconfigured")
			actor, token, want, reads := "hero", "session", 403, 0
			switch condition {
			case "anonymous":
				token, want = "", 401
			case "foreign":
				actor = "other"
			case "missing":
				actor, want = "missing", 404
			case "snapshot owner":
				f.queryOwner = "other"
			case "profile owner":
				f.profileOwner = "other"
				want = 500
			case "blessing error":
				repo.failAt, want, reads = 1, 500, 1
			case "status error":
				repo.failAt, want, reads = 2, 500, 2
			case "unconfigured":
				want = 501
			case "sleep":
				f.char.PendingWake = true
				want = 200
			case "work":
				f.actions = []scheduling.ScheduledAction{{ID: "work", State: scheduling.StateProcessing, ExecuteAt: time.Now().Add(-time.Hour)}}
				want = 200
			}
			r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+actor+"/context", nil)
			r.Header.Set("Authorization", "Bearer "+token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, r)
			if w.Code != want || repo.reads != reads || f.executions != 0 || store.writes != 0 {
				t.Fatalf("read boundary: status=%d reads=%d body=%s", w.Code, repo.reads, w.Body.String())
			}
			if want != 200 && strings.Contains(w.Body.String(), `"scene"`) {
				t.Fatalf("partial scene: %s", w.Body.String())
			}
		})
	}
}

func TestOpenAPIChapelRetirementKeepsGateway(t *testing.T) {
	var spec struct {
		Paths      map[string]map[string]json.RawMessage
		Components struct{ Schemas map[string]json.RawMessage }
	}
	if err := json.Unmarshal(OpenAPISpec(), &spec); err != nil {
		t.Fatal(err)
	}
	if _, exists := spec.Paths["/characters/{id}/chapel"]; exists {
		t.Error("retired Chapel GET remains in OpenAPI")
	}
	if _, exists := spec.Paths["/characters/{id}/chapel/pray"]; exists {
		t.Error("retired Chapel POST remains in OpenAPI")
	}
	if _, exists := spec.Components.Schemas["GetChapelResponse"]; exists {
		t.Error("orphan Chapel GET schema remains")
	}
	var result struct {
		Required    []string
		Description string
	}
	if err := json.Unmarshal(spec.Components.Schemas["ChapelBlessingResponse"], &result); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(result.Required, []string{"blessing", "message"}) || !strings.Contains(result.Description, "chapel_pray") {
		t.Fatalf("missing Gateway confirmation contract: %+v", result)
	}
	for _, route := range []struct{ method, path string }{{"get", "/api/v1/characters/{id}/context"}, {"post", "/api/v1/characters/{id}/actions"}} {
		if len(spec.Paths[route.path][route.method]) == 0 {
			t.Errorf("required route missing: %s %s", route.method, route.path)
		}
	}
}
