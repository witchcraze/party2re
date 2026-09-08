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
	listDefinitionsFn    func() []corejob.Definition
	changeJobFn          func(ctx context.Context, characterID, targetJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	exchangeJobFn        func(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error)
	saveFutureMemoryFn   func(ctx context.Context, characterID string) (corecharacter.FutureMemory, error)
	recallFutureMemoryFn func(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error)
	listFutureMemoriesFn func(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error)
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

func (s *stubJobService) ExchangeJob(ctx context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.exchangeJobFn != nil {
		return s.exchangeJobFn(ctx, characterID, targetJobID, targetOldJobID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func (s *stubJobService) SaveFutureMemory(ctx context.Context, characterID string) (corecharacter.FutureMemory, error) {
	if s.saveFutureMemoryFn != nil {
		return s.saveFutureMemoryFn(ctx, characterID)
	}
	return corecharacter.FutureMemory{}, nil
}

func (s *stubJobService) RecallFutureMemory(ctx context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error) {
	if s.recallFutureMemoryFn != nil {
		return s.recallFutureMemoryFn(ctx, characterID, memoryID)
	}
	return corecharacter.Character{}, corejob.CharacterJob{}, nil
}

func (s *stubJobService) ListFutureMemories(ctx context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
	if s.listFutureMemoriesFn != nil {
		return s.listFutureMemoriesFn(ctx, characterID)
	}
	return nil, nil
}

func TestJobEndpoints(t *testing.T) {
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
		exchangeJobFn: func(_ context.Context, characterID, targetJobID, targetOldJobID string) (corecharacter.Character, corejob.CharacterJob, error) {
			cj, _ := corejob.NewCharacterJob(characterID, targetJobID)
			return char, cj, nil
		},
		saveFutureMemoryFn: func(_ context.Context, characterID string) (corecharacter.FutureMemory, error) {
			return corecharacter.FutureMemory{ID: "fmem-1", CharacterID: characterID, JobID: "job-01", Level: 50}, nil
		},
		recallFutureMemoryFn: func(_ context.Context, characterID, memoryID string) (corecharacter.Character, corejob.CharacterJob, error) {
			cj, _ := corejob.NewCharacterJob(characterID, "job-01")
			return char, cj, nil
		},
		listFutureMemoriesFn: func(_ context.Context, characterID string) ([]corecharacter.FutureMemory, error) {
			return []corecharacter.FutureMemory{{ID: "fmem-1", CharacterID: characterID, JobID: "job-01", Level: 50}}, nil
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

	t.Run("POST /characters/{id}/rebirth - eliminated (404 Not Found)", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/rebirth", "")
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 Not Found for eliminated rebirth endpoint, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/exchange-job - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/exchange-job", `{"job_id":"job-02","old_job_id":"job-01"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})

	t.Run("GET /characters/{id}/future-memories - success", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/c1/future-memories", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var memories []corecharacter.FutureMemory
		if err := json.NewDecoder(rec.Body).Decode(&memories); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(memories) != 1 {
			t.Fatalf("expected 1 memory, got %d", len(memories))
		}
	})

	t.Run("POST /characters/{id}/future-memories - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/future-memories", `{}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("expected 201 Created, got %d", rec.Code)
		}
	})

	t.Run("POST /characters/{id}/recall-future - success", func(t *testing.T) {
		req := jsonRequest(t, http.MethodPost, "/characters/c1/recall-future", `{"memory_id":"fmem-1"}`)
		req.Header.Set("Authorization", "Bearer valid-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
	})
}
