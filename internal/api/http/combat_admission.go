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
	default:
		return false
	}
	return true
}
