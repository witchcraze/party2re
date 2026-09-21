package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type adminPlayerResponse struct {
	ID        string     `json:"id"`
	Username  string     `json:"username"`
	CreatedAt time.Time  `json:"created_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	LastIP    string     `json:"last_ip"`
	BannedAt  *time.Time `json:"banned_at,omitempty"`
	IsBanned  bool       `json:"is_banned"`
}

type adminPlayersListResponse struct {
	Players []adminPlayerResponse `json:"players"`
	Total   int                   `json:"total"`
}

type adminBanPlayerResponse struct {
	Message  string `json:"message"`
	PlayerID string `json:"player_id"`
}

func (h *Handler) handleAdminListPlayers(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateAdmin(w, r) {
		return
	}

	sort := r.URL.Query().Get("sort")
	players, err := h.players.ListPlayers(r.Context(), sort)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	res := adminPlayersListResponse{
		Players: make([]adminPlayerResponse, 0, len(players)),
		Total:   len(players),
	}
	for _, p := range players {
		res.Players = append(res.Players, adminPlayerResponse{
			ID:        p.ID,
			Username:  p.Username,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
			LastIP:    p.LastIP,
			BannedAt:  p.BannedAt,
			IsBanned:  p.IsBanned(),
		})
	}

	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleAdminBanPlayer(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateAdmin(w, r) {
		return
	}

	playerID := strings.TrimSpace(r.PathValue("id"))
	if playerID == "" {
		writeError(w, http.StatusBadRequest, errors.New("player id is required"))
		return
	}

	if err := h.players.BanPlayer(r.Context(), playerID); err != nil {
		if errors.Is(err, coreplayer.ErrInvalidPlayer) || errors.Is(err, coreplayer.ErrPlayerNotFound) || strings.Contains(strings.ToLower(err.Error()), "not found") {
			writeError(w, http.StatusNotFound, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, adminBanPlayerResponse{
		Message:  "player banned successfully",
		PlayerID: playerID,
	})
}
