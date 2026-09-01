package http

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/maintenance"
)

// MaintenanceService defines the operations exposed by the maintenance domain for HTTP endpoints.
type MaintenanceService interface {
	GetStatus(ctx context.Context) (maintenance.Status, error)
	SetMaintenance(ctx context.Context, enabled bool, message string, estimatedEndTime *time.Time) (maintenance.Status, error)
	IsEnabled(ctx context.Context) bool
}

// setMaintenanceRequest represents the payload for enabling/disabling maintenance mode.
type setMaintenanceRequest struct {
	Enabled          bool       `json:"enabled"`
	Message          string     `json:"message"`
	EstimatedEndTime *time.Time `json:"estimated_end_time,omitempty"`
}

// handleGetMaintenance returns the current public maintenance status.
func (h *Handler) handleGetMaintenance(w http.ResponseWriter, r *http.Request) {
	if h.maintenance == nil {
		writeJSON(w, http.StatusOK, maintenance.Status{
			Enabled:   false,
			Message:   "System is operating normally.",
			UpdatedAt: time.Now().UTC(),
		})
		return
	}

	status, err := h.maintenance.GetStatus(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, status)
}

// handleAdminSetMaintenance configures the maintenance mode state (requires admin API key).
func (h *Handler) handleAdminSetMaintenance(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateAdmin(w, r) {
		return
	}

	if h.maintenance == nil {
		writeError(w, http.StatusNotImplemented, errors.New("maintenance service not configured"))
		return
	}

	var req setMaintenanceRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	status, err := h.maintenance.SetMaintenance(r.Context(), req.Enabled, req.Message, req.EstimatedEndTime)
	if err != nil {
		if errors.Is(err, maintenance.ErrInvalidMessage) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, status)
}

// isAdminRequest checks if the request includes valid administrator credentials without writing an error response.
func (h *Handler) isAdminRequest(r *http.Request) bool {
	if strings.TrimSpace(h.adminAPIKey) == "" {
		return false
	}

	providedKey := r.Header.Get("X-Admin-Key")
	if providedKey == "" {
		if authHeader := r.Header.Get("Authorization"); strings.HasPrefix(authHeader, "Bearer ") {
			providedKey = strings.TrimPrefix(authHeader, "Bearer ")
		}
	}
	providedKey = strings.TrimSpace(providedKey)
	if providedKey == "" {
		return false
	}

	return subtle.ConstantTimeCompare([]byte(providedKey), []byte(h.adminAPIKey)) == 1
}

// maintenanceMiddleware intercepts requests during active maintenance and responds with 503 Service Unavailable,
// bypassing health, openapi, maintenance status, and authorized administrator requests.
func (h *Handler) maintenanceMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if h.maintenance == nil || !h.maintenance.IsEnabled(r.Context()) {
			next.ServeHTTP(w, r)
			return
		}

		path := r.URL.Path
		// Whitelist public system endpoints
		if path == "/health" || path == "/openapi.json" || path == "/maintenance" {
			next.ServeHTTP(w, r)
			return
		}

		// Whitelist admin requests
		if h.isAdminRequest(r) {
			next.ServeHTTP(w, r)
			return
		}

		st, err := h.maintenance.GetStatus(r.Context())
		msg := "The system is currently undergoing maintenance. Please try again later."
		if err == nil && st.Message != "" {
			msg = st.Message
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"error":              msg,
			"code":               "MAINTENANCE_MODE",
			"estimated_end_time": st.EstimatedEndTime,
		})
	})
}
