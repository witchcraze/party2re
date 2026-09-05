package http

import (
	"errors"
	"net/http"
	"strings"
	"time"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
)

type CreateAPITokenRequest struct {
	Name      string     `json:"name"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

type APITokenDTO struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

type CreateAPITokenResponse struct {
	APITokenDTO
	Token string `json:"token"`
}

type APITokensResponse struct {
	Tokens []APITokenDTO `json:"tokens"`
}

type RevokeAPITokenResponse struct {
	Revoked bool   `json:"revoked"`
	TokenID string `json:"token_id"`
}

func (h *Handler) handleCreateAPIToken(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	var req CreateAPITokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	token, plaintext, err := h.players.CreateAPIToken(r.Context(), player.ID, req.Name, req.ExpiresAt)
	if err != nil {
		if errors.Is(err, coreplayer.ErrInvalidAPITokenName) || errors.Is(err, coreplayer.ErrInvalidAPITokenExpiration) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusCreated, CreateAPITokenResponse{
		APITokenDTO: APITokenDTO{
			ID:         token.ID,
			Name:       token.Name,
			CreatedAt:  token.CreatedAt,
			LastUsedAt: token.LastUsedAt,
			ExpiresAt:  token.ExpiresAt,
		},
		Token: plaintext,
	})
}

func (h *Handler) handleListAPITokens(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	tokens, err := h.players.ListAPITokens(r.Context(), player.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	dtos := make([]APITokenDTO, 0, len(tokens))
	for _, t := range tokens {
		dtos = append(dtos, APITokenDTO{
			ID:         t.ID,
			Name:       t.Name,
			CreatedAt:  t.CreatedAt,
			LastUsedAt: t.LastUsedAt,
			ExpiresAt:  t.ExpiresAt,
		})
	}

	writeJSON(w, http.StatusOK, APITokensResponse{Tokens: dtos})
}

func (h *Handler) handleRevokeAPIToken(w http.ResponseWriter, r *http.Request) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	tokenID := strings.TrimSpace(r.PathValue("id"))
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, errors.New("token ID is required"))
		return
	}

	if err := h.players.RevokeAPIToken(r.Context(), player.ID, tokenID); err != nil {
		if errors.Is(err, database.ErrAPITokenNotFound) {
			writeError(w, http.StatusNotFound, err)
			return
		}
		if errors.Is(err, database.ErrAPITokenForbidden) {
			writeError(w, http.StatusForbidden, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, RevokeAPITokenResponse{
		Revoked: true,
		TokenID: tokenID,
	})
}
