package http

import (
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// withAuthenticatedActionCharacter validates player authentication, character ownership,
// and ensures the character is not sleeping before invoking fn.
func (h *Handler) withAuthenticatedActionCharacter(
	w http.ResponseWriter,
	r *http.Request,
	characterID string,
	fn func(player coreplayer.Player, char corecharacter.Character),
) {
	h.withAuthenticatedCharacter(w, r, characterID, func(player coreplayer.Player, char corecharacter.Character) {
		if !h.ensureNotSleeping(w, r, char.ID) {
			return
		}
		fn(player, char)
	})
}

// withAuthenticatedActionCharacterAndJSON decodes JSON, validates player authentication,
// character ownership, and ensures the character is not sleeping before invoking fn.
func withAuthenticatedActionCharacterAndJSON[Req any](
	h *Handler,
	w http.ResponseWriter,
	r *http.Request,
	getCharID func(req *Req) string,
	fn func(player coreplayer.Player, char corecharacter.Character, req Req),
) {
	withAuthenticatedCharacterAndJSON(h, w, r, getCharID, func(player coreplayer.Player, char corecharacter.Character, req Req) {
		if !h.ensureNotSleeping(w, r, char.ID) {
			return
		}
		fn(player, char, req)
	})
}
