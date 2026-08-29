package http

import (
	"errors"
	"net/http"
	"strings"

	"crypto/subtle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// authenticateAdmin extracts administrator credentials from the request and validates them.
// Supported credential headers:
// 1. "X-Admin-Key: <key>"
// 2. "Authorization: Bearer <key>"
// Returns true if authenticated, or writes 401/403 and returns false.
func (h *Handler) authenticateAdmin(w http.ResponseWriter, r *http.Request) bool {
	if strings.TrimSpace(h.adminAPIKey) == "" {
		writeError(w, http.StatusForbidden, errors.New("forbidden: admin operations disabled (no admin key configured)"))
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
		writeError(w, http.StatusUnauthorized, errors.New("missing admin authorization credentials"))
		return false
	}

	if subtle.ConstantTimeCompare([]byte(providedKey), []byte(h.adminAPIKey)) != 1 {
		writeError(w, http.StatusForbidden, errors.New("forbidden: invalid admin credentials"))
		return false
	}

	return true
}

// authenticatePlayer extracts the session from the request and authenticates the player.
// If authentication fails, it writes an appropriate 401 Unauthorized response and returns false.
func (h *Handler) authenticatePlayer(w http.ResponseWriter, r *http.Request) (coreplayer.Player, bool) {
	sessionID := sessionIDFromRequest(r)
	if sessionID == "" {
		writeError(w, http.StatusUnauthorized, errors.New("missing session"))
		return coreplayer.Player{}, false
	}
	player, err := h.players.Authenticate(r.Context(), sessionID)
	if err != nil {
		writeError(w, http.StatusUnauthorized, errors.New("invalid session"))
		return coreplayer.Player{}, false
	}
	return player, true
}

// authorizeCharacter fetches the character by ID and ensures it belongs to the authenticated player.
// If validation fails, it writes the appropriate HTTP error (400, 404, 500, or 403) and returns false.
func (h *Handler) authorizeCharacter(w http.ResponseWriter, r *http.Request, playerID, characterID string) (corecharacter.Character, bool) {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		writeError(w, http.StatusBadRequest, errors.New("missing character_id parameter"))
		return corecharacter.Character{}, false
	}

	char, err := h.characters.Get(r.Context(), characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeError(w, http.StatusNotFound, err)
			return corecharacter.Character{}, false
		}
		writeError(w, http.StatusInternalServerError, err)
		return corecharacter.Character{}, false
	}
	if char.PlayerID != playerID {
		writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
		return corecharacter.Character{}, false
	}
	return char, true
}

// withAuthenticatedCharacter validates player authentication and character ownership before invoking the callback.
func (h *Handler) withAuthenticatedCharacter(
	w http.ResponseWriter,
	r *http.Request,
	characterID string,
	fn func(player coreplayer.Player, char corecharacter.Character),
) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}
	char, ok := h.authorizeCharacter(w, r, player.ID, characterID)
	if !ok {
		return
	}
	fn(player, char)
}

// withAuthenticatedCharacterAndJSON decodes a JSON request body and validates player authentication
// and character ownership before invoking the callback.
func withAuthenticatedCharacterAndJSON[Req any](
	h *Handler,
	w http.ResponseWriter,
	r *http.Request,
	getCharID func(req *Req) string,
	fn func(player coreplayer.Player, char corecharacter.Character, req Req),
) {
	player, ok := h.authenticatePlayer(w, r)
	if !ok {
		return
	}

	var req Req
	if !decodeJSON(w, r, &req) {
		return
	}

	charID := getCharID(&req)
	char, ok := h.authorizeCharacter(w, r, player.ID, charID)
	if !ok {
		return
	}

	fn(player, char, req)
}
