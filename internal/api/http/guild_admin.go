package http

import (
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/guild"
)

type adminDecayGuildPointsRequest struct {
	Factor float64 `json:"factor,omitempty"`
}

type adminDecayGuildPointsResponse struct {
	Status string  `json:"status"`
	Factor float64 `json:"factor"`
}

func (h *Handler) handleAdminDecayGuildPoints(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	if !h.authenticateAdmin(w, r) {
		return
	}

	var req adminDecayGuildPointsRequest
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}

	factor := req.Factor
	if factor <= 0 {
		factor = guild.DefaultPointDecayFactor
	}
	if factor > 1.0 {
		writeError(w, http.StatusBadRequest, guild.ErrInvalidDecayFactor)
		return
	}

	if err := h.guild.DecayGuildPoints(r.Context(), factor); err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, adminDecayGuildPointsResponse{
		Status: "ok",
		Factor: factor,
	})
}
