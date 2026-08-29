package http_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubJobService struct {
	listDefinitionsFn func() []corejob.Definition
	changeJobFn       func(ctx context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
}

func (s *stubJobService) ListDefinitions() []corejob.Definition {
	if s.listDefinitionsFn != nil {
		return s.listDefinitionsFn()
	}
	return nil
}

func (s *stubJobService) ChangeJob(ctx context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.changeJobFn != nil {
		return s.changeJobFn(ctx, characterID, targetJobID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func TestJobAndRebirthEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero", Level: 50}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
		rebirthFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				c := char
				c.Level = 1
				c.RebirthCount = 1
				return c, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	jService := &stubJobService{
		listDefinitionsFn: func() []corejob.Definition {
			return []corejob.Definition{
				{ID: "job-01", Name: "Fighter"},
				{ID: "job-02", Name: "Mage"},
			}
		},
		changeJobFn: func(_ context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
			if targetJobID == "invalid" {
				return corecharacter.Character{}, corejob.CharacterJob{}, corejob.ErrDefinitionNotFound
			}
			cj, _ := corejob.NewCharacterJob(characterID, targetJobID)
			return char, cj, nil
		},
	}

	h := newTestHandler(
		t,
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithJob(jService),
	)
	router := h.Router()

	t.Run("GET /jobs - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/jobs", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var jobs []corejob.Definition
		if err := json.NewDecoder(rec.Body).Decode(&jobs); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(jobs) != 2 {
			t.Fatalf("expected 2 jobs, got %d", len(jobs))
		}
	})

	t.Run("POST /characters/{id}/change-job - unauthorized", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/change-job", `{"job_id":"job-01"}`)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/change-job - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/change-job", `{"job_id":"job-02"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/rebirth - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/rebirth", "")
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}
