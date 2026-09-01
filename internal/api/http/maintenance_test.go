package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/maintenance"
)

type stubMaintenanceService struct {
	status   maintenance.Status
	getErr   error
	setErr   error
	isEnable bool
}

func (s *stubMaintenanceService) GetStatus(ctx context.Context) (maintenance.Status, error) {
	if s.getErr != nil {
		return maintenance.Status{}, s.getErr
	}
	return s.status, nil
}

func (s *stubMaintenanceService) SetMaintenance(ctx context.Context, enabled bool, message string, estimatedEndTime *time.Time) (maintenance.Status, error) {
	if s.setErr != nil {
		return maintenance.Status{}, s.setErr
	}
	s.status = maintenance.Status{
		Enabled:          enabled,
		Message:          message,
		EstimatedEndTime: estimatedEndTime,
		UpdatedAt:        time.Now().UTC(),
	}
	s.isEnable = enabled
	return s.status, nil
}

func (s *stubMaintenanceService) IsEnabled(ctx context.Context) bool {
	return s.isEnable
}

func TestMaintenanceEndpoints(t *testing.T) {
	maintSvc := &stubMaintenanceService{
		status: maintenance.Status{
			Enabled:   false,
			Message:   "Normal operation",
			UpdatedAt: time.Now().UTC(),
		},
		isEnable: false,
	}

	h, err := apihttp.NewHandler(
		&stubPlayerService{
			authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
				return coreplayer.Player{ID: "player-1"}, nil
			},
		},
		&stubCharacterService{
			getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
				return corecharacter.Character{ID: id, PlayerID: "player-1"}, nil
			},
		},
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithMaintenance(maintSvc),
		apihttp.WithAdminAPIKey("secret-admin-key"),
	)
	if err != nil {
		t.Fatalf("failed to create handler: %v", err)
	}
	router := h.Router()

	t.Run("GET /maintenance returns current status", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/maintenance", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d", rec.Code)
		}
		var st maintenance.Status
		if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if st.Enabled {
			t.Errorf("expected maintenance disabled, got true")
		}
	})

	t.Run("POST /admin/maintenance rejects unauthorized request", func(t *testing.T) {
		body := `{"enabled": true, "message": "Scheduled maintenance"}`
		req := httptest.NewRequest(http.MethodPost, "/admin/maintenance", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401 Unauthorized, got %d", rec.Code)
		}
	})

	t.Run("POST /admin/maintenance enables maintenance with valid key", func(t *testing.T) {
		body := `{"enabled": true, "message": "Emergency maintenance"}`
		req := httptest.NewRequest(http.MethodPost, "/admin/maintenance", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "secret-admin-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}
		if !maintSvc.isEnable {
			t.Errorf("expected maintenance to be enabled in service")
		}
	})

	t.Run("maintenanceMiddleware blocks normal routes with 503 when enabled", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/characters/char-1", nil)
		req.Header.Set("Authorization", "Bearer session-token")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Fatalf("expected 503 Service Unavailable, got %d: %s", rec.Code, rec.Body.String())
		}
	})

	t.Run("maintenanceMiddleware permits /health during maintenance", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /health, got %d", rec.Code)
		}
	})

	t.Run("maintenanceMiddleware permits /openapi.json during maintenance", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/openapi.json", nil)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for /openapi.json, got %d", rec.Code)
		}
	})

	t.Run("maintenanceMiddleware permits admin requests during maintenance", func(t *testing.T) {
		body := `{"enabled": false, "message": "Resumed"}`
		req := httptest.NewRequest(http.MethodPut, "/admin/maintenance", bytes.NewReader([]byte(body)))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-Admin-Key", "secret-admin-key")
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK for admin request, got %d: %s", rec.Code, rec.Body.String())
		}
		if maintSvc.isEnable {
			t.Errorf("expected maintenance disabled after update")
		}
	})
}
