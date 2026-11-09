package http

import (
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/validation"
)

// handleNamingHallDialogue returns NPC @マリナン dialogue and pricing information.
func (h *Handler) handleNamingHallDialogue(w http.ResponseWriter, r *http.Request) {
	if h.characters == nil {
		writeJSON(w, http.StatusNotImplemented, map[string]string{"error": "character service not configured"})
		return
	}

	dialogue := h.characters.GetNamingHallDialogue()
	writeJSON(w, http.StatusOK, dialogue)
}

// handleChangeCharacterName handles character renaming at the Naming Hall.
func (h *Handler) handleChangeCharacterName(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	if charID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "character id is required"})
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(player coreplayer.Player, char corecharacter.Character) {
		var req struct {
			Name string `json:"name"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		updated, err := h.characters.ChangeName(r.Context(), charID, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrNameAlreadyTaken):
				writeJSON(w, http.StatusConflict, map[string]string{"error": err.Error()})
			case errors.Is(err, character.ErrInvalidName),
				errors.Is(err, character.ErrSameName),
				errors.Is(err, character.ErrInsufficientGold),
				errors.Is(err, character.ErrInGuildDisallowed),
				errors.Is(err, character.ErrActiveMarketDisallowed),
				errors.Is(err, character.ErrExileInvolvedDisallowed):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, corecharacter.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to change name"})
			}
			return
		}

		writeJSON(w, http.StatusOK, toCharacterResponse(updated))
	})
}

// handleChangeCharacterGender handles character gender change at the Naming Hall.
func (h *Handler) handleChangeCharacterGender(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	if charID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "character id is required"})
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(player coreplayer.Player, char corecharacter.Character) {
		var req struct {
			Gender string `json:"gender"`
		}
		if !decodeJSON(w, r, &req) {
			return
		}

		updated, err := h.characters.ChangeGender(r.Context(), charID, req.Gender)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrInvalidGender),
				errors.Is(err, character.ErrSameGender),
				errors.Is(err, character.ErrInsufficientGold),
				errors.Is(err, character.ErrGenderIncompatibleWithJob):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, corecharacter.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to change gender"})
			}
			return
		}

		writeJSON(w, http.StatusOK, toCharacterResponse(updated))
	})
}

type characterProfileResponse struct {
	Character struct {
		Name string `json:"name"`
	} `json:"character"`
	Profile struct {
		Comment    string            `json:"comment"`
		AvatarURL  string            `json:"avatar_url"`
		AuraEffect int               `json:"aura_effect"`
		BioData    map[string]string `json:"bio_data,omitempty"`
	} `json:"profile"`
}

// handleGetCharacterProfile returns public profile information for a character.
func (h *Handler) handleGetCharacterProfile(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	if charID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "character id is required"})
		return
	}

	profileView, err := h.characters.GetProfile(r.Context(), charID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get character profile"})
		return
	}

	var response characterProfileResponse
	response.Character.Name = profileView.Character.Name
	response.Profile.Comment = profileView.Profile.Comment
	response.Profile.AvatarURL = profileView.Profile.AvatarURL
	response.Profile.AuraEffect = profileView.Profile.AuraEffect
	response.Profile.BioData = profileView.Profile.BioData
	writeJSON(w, http.StatusOK, response)
}

// handleUpdateCharacterProfile updates comment, avatar URL, or bio data for a character.
func (h *Handler) handleUpdateCharacterProfile(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	if charID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "character id is required"})
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(player coreplayer.Player, char corecharacter.Character) {
		var req character.UpdateProfileRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		profile, err := h.characters.UpdateProfile(r.Context(), charID, req)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrCommentTooLong),
				errors.Is(err, character.ErrBioKeyTooLong),
				errors.Is(err, character.ErrBioValueTooLong),
				errors.Is(err, character.ErrInvalidAvatarURL),
				errors.Is(err, character.ErrInvalidAuraEffect),
				errors.Is(err, validation.ErrControlCharacter),
				errors.Is(err, validation.ErrZeroWidth),
				errors.Is(err, validation.ErrBidiOverride),
				errors.Is(err, validation.ErrZalgo):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, corecharacter.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile"})
			}
			return
		}

		writeJSON(w, http.StatusOK, profile)
	})
}
