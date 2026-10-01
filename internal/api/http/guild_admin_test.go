package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/guild"
)

func TestAdminDecayGuildPoints(t *testing.T) {
	const adminKey = "secret-admin-key"

	setupHandler := func(guildSvc *stubGuildService) http.Handler {
		opts := []apihttp.Option{
			apihttp.WithAdminAPIKey(adminKey),
		}
		if guildSvc != nil {
			opts = append(opts, apihttp.WithGuild(guildSvc))
		}
		h, err := apihttp.NewHandler(
			&stubPlayerService{},
			&stubCharacterService{},
			&stubAdventureService{},
			&stubShopService{},
			opts...,
		)
		if err != nil {
			t.Fatalf("failed to create handler: %v", err)
		}
		return h.Router()
	}

	t.Run("401 Unauthorized without admin credentials", func(t *testing.T) {
		guildSvc := &stubGuildService{}
		router := setupHandler(guildSvc)

		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("403 Forbidden with invalid admin key", func(t *testing.T) {
		guildSvc := &stubGuildService{}
		router := setupHandler(guildSvc)

		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", nil)
		req.Header.Set("X-Admin-Key", "wrong-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden, got %d", rec.Code)
		}
	})

	t.Run("200 OK with valid factor", func(t *testing.T) {
		var receivedFactor float64
		guildSvc := &stubGuildService{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				receivedFactor = factor
				return nil
			},
		}
		router := setupHandler(guildSvc)

		body := map[string]float64{"factor": 0.75}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", bytes.NewReader(bodyBytes))
		req.Header.Set("X-Admin-Key", adminKey)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if receivedFactor != 0.75 {
			t.Errorf("expected factor 0.75, got %f", receivedFactor)
		}

		var resp map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if resp["status"] != "ok" {
			t.Errorf("expected status ok, got %v", resp["status"])
		}
		if resp["factor"] != 0.75 {
			t.Errorf("expected factor 0.75, got %v", resp["factor"])
		}
	})

	t.Run("200 OK with default factor when body empty", func(t *testing.T) {
		var receivedFactor float64
		guildSvc := &stubGuildService{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				receivedFactor = factor
				return nil
			},
		}
		router := setupHandler(guildSvc)

		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if receivedFactor != guild.DefaultPointDecayFactor {
			t.Errorf("expected default factor %f, got %f", guild.DefaultPointDecayFactor, receivedFactor)
		}
	})

	t.Run("400 Bad Request with factor > 1.0", func(t *testing.T) {
		guildSvc := &stubGuildService{}
		router := setupHandler(guildSvc)

		body := map[string]float64{"factor": 1.5}
		bodyBytes, _ := json.Marshal(body)
		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", bytes.NewReader(bodyBytes))
		req.Header.Set("X-Admin-Key", adminKey)
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("expected 400 Bad Request, got %d", rec.Code)
		}
	})

	t.Run("500 Internal Server Error when service fails", func(t *testing.T) {
		guildSvc := &stubGuildService{
			decayGuildPointsFn: func(ctx context.Context, factor float64) error {
				return errors.New("database error")
			},
		}
		router := setupHandler(guildSvc)

		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusInternalServerError {
			t.Errorf("expected 500 Internal Server Error, got %d", rec.Code)
		}
	})

	t.Run("501 Not Implemented when guild service is nil", func(t *testing.T) {
		router := setupHandler(nil)

		req := httptest.NewRequest(http.MethodPost, "/admin/guilds/decay-points", nil)
		req.Header.Set("X-Admin-Key", adminKey)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotImplemented {
			t.Errorf("expected 501 Not Implemented, got %d", rec.Code)
		}
	})
}
