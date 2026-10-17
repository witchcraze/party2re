package http

import (
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/party"
)

func writeCombatAdmissionError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, party.ErrNotFound), errors.Is(err, party.ErrCharacterNotInParty):
		writeError(w, http.StatusBadRequest, err)
	case errors.Is(err, party.ErrNotPartyLeader), errors.Is(err, dungeon.ErrForbidden):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, party.ErrPartyNotRecruiting), errors.Is(err, party.ErrPartyNotReady),
		errors.Is(err, party.ErrMemberBusy), errors.Is(err, party.ErrCharacterUnconscious),
		errors.Is(err, party.ErrCharacterExhausted), errors.Is(err, dungeon.ErrActiveExpeditionExists),
		errors.Is(err, challenge.ErrActiveSessionExists):
		writeError(w, http.StatusUnprocessableEntity, err)
	case errors.Is(err, dungeon.ErrStartOutcomeUnknown):
		// Only the stable sentinel is exposed; joined causes may carry internals.
		writeError(w, http.StatusInternalServerError, dungeon.ErrStartOutcomeUnknown)
	case errors.Is(err, challenge.ErrStartOutcomeUnknown):
		writeError(w, http.StatusInternalServerError, challenge.ErrStartOutcomeUnknown)
	default:
		return false
	}
	return true
}
