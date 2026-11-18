package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
)

func (r *homeSceneRepository) ListDeliveryNoticesPage(_ context.Context, actor string, limit, offset int, before time.Time, beforeID string) ([]home.DeliveryNotice, error) {
	if actor != "hero" {
		r.fixture.t.Fatalf("notice read for %s", actor)
	}
	var notices []home.DeliveryNotice
	for _, n := range r.notices {
		if !n.IsCleared && (before.IsZero() || n.CreatedAt.Before(before) || (n.CreatedAt.Equal(before) && n.ID < beforeID)) {
			notices = append(notices, n)
		}
	}
	return pagination.SlicePage(notices, limit, offset).Items, r.readErr
}

func (f *homeSceneFixture) ListDeliveryNoticesPage(ctx context.Context, actor, player string, limit, offset int, cursor string) (pagination.CursorPage[home.DeliveryNotice], error) {
	return f.viewService.ListDeliveryNoticesPage(ctx, actor, player, limit, offset, cursor)
}

func TestHomeNoticesScene(t *testing.T) {
	for _, count := range []int{0, 50, 51, 121} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			f, _, router := homeSceneRouter(t)
			for i := count; i > 0; i-- {
				f.repo.notices = append(f.repo.notices, home.DeliveryNotice{ID: fmt.Sprintf("n%03d", i), CreatedAt: time.Unix(1, 0)})
			}
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_notices"}}`, nil)
			if status != 200 {
				t.Fatalf("entry: %d %s", status, raw)
			}
			seen := map[string]bool{}
			for {
				status, response := navigationGET(t, router)
				var data HomeNoticesSceneData
				b, err := json.Marshal(response.Scene.Data)
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(b, &data); err != nil {
					t.Fatal(err)
				}
				if status != 200 || data.Notices == nil || len(data.Notices) > 50 || !hasHomeAction(response, "home_notice_ack") {
					t.Fatalf("observation: %d %+v", status, response)
				}
				for _, n := range data.Notices {
					if n.IsCleared || seen[n.ID] {
						t.Fatalf("cleared/duplicate: %+v", n)
					}
					seen[n.ID] = true
				}
				if data.Page.Next == nil {
					if hasHomeAction(response, "scene_page") {
						t.Fatal("offered a page after the end")
					}
					break
				}
				if !hasHomeAction(response, "scene_page") {
					t.Fatal("no continuation command")
				}
				body, err := json.Marshal(map[string]any{"action": "scene_page", "params": data.Page.Next})
				if err != nil {
					t.Fatal(err)
				}
				status, raw = gatewayRequest(t, router, "hero", "session", "application/json", string(body), nil)
				if status != 200 {
					t.Fatalf("page: %d %s", status, raw)
				}
			}
			if len(seen) != count || len(f.living) != 0 || f.executions != 0 || f.updates != 0 {
				t.Fatalf("missing/mutated: seen=%d calls=%v", len(seen), f.living)
			}
			status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"home_notice_ack","params":{"notice_ids":["n001"]}}`, nil)
			if status != 200 || len(f.living) != 1 || f.living[0] != "ack hero n001" {
				t.Fatalf("ack: %d %s %v", status, raw, f.living)
			}
			f.repo.readErr = errors.New("notice read failed")
			if status, _ := navigationGET(t, router); status != 500 {
				t.Fatal("hidden required read failure")
			}
		})
	}
}

func TestHomeNoticesRejectsSpoofing(t *testing.T) {
	_, store, router := homeSceneRouter(t)
	for _, body := range []string{
		`{"action":"scene_enter","params":{"destination":"home_notices","character_id":"other"}}`,
		`{"action":"scene_enter","params":{"destination":"home_notices"}}`,
	} {
		actor := "hero"
		if body == `{"action":"scene_enter","params":{"destination":"home_notices"}}` {
			actor = "other"
		}
		status, raw := gatewayRequest(t, router, actor, "session", "application/json", body, nil)
		if status == 200 || store.writes != 0 {
			t.Fatalf("spoof: %d %s", status, raw)
		}
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"home","target_id":"other"}}`, nil)
	if status != 200 {
		t.Fatalf("visit: %d %s", status, raw)
	}
	// Notices are an own-Home action (home.cgi:26-42): a visited home does not admit them.
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"home_notices"}}`, nil)
	if status != 403 || store.selection.Subject.ID != "other" {
		t.Fatalf("notices from a visited home: %d %s", status, raw)
	}
}
