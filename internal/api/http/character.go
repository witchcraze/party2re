package http

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		updated, err := h.characters.ChangeName(r.Context(), charID, req.Name)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrInvalidName),
				errors.Is(err, character.ErrSameName),
				errors.Is(err, character.ErrInsufficientGold),
				errors.Is(err, character.ErrNameAlreadyTaken),
				errors.Is(err, character.ErrInGuildDisallowed),
				errors.Is(err, character.ErrActiveMarketDisallowed):
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		updated, err := h.characters.ChangeGender(r.Context(), charID, req.Gender)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrInvalidGender),
				errors.Is(err, character.ErrSameGender),
				errors.Is(err, character.ErrInsufficientGold):
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

	writeJSON(w, http.StatusOK, profileView)
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
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}

		profile, err := h.characters.UpdateProfile(r.Context(), charID, req)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrCommentTooLong),
				errors.Is(err, character.ErrBioKeyTooLong),
				errors.Is(err, character.ErrBioValueTooLong),
				errors.Is(err, character.ErrInvalidAvatarURL):
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

// handleUploadCharacterAvatar handles avatar image file or data upload.
func (h *Handler) handleUploadCharacterAvatar(w http.ResponseWriter, r *http.Request) {
	charID := r.PathValue("id")
	if charID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "character id is required"})
		return
	}

	h.withAuthenticatedCharacter(w, r, charID, func(player coreplayer.Player, char corecharacter.Character) {
		contentType := r.Header.Get("Content-Type")
		var filename string
		var imageBytes []byte
		var mimeType string

		if strings.HasPrefix(contentType, "multipart/form-data") {
			if err := r.ParseMultipartForm(character.MaxAvatarSizeBytes); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart form data"})
				return
			}
			file, header, err := r.FormFile("avatar")
			if err != nil {
				file, header, err = r.FormFile("image")
			}
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing avatar file in form data"})
				return
			}
			defer file.Close()

			filename = header.Filename
			mimeType = header.Header.Get("Content-Type")
			data, err := io.ReadAll(io.LimitReader(file, character.MaxAvatarSizeBytes+1))
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "failed to read uploaded file"})
				return
			}
			imageBytes = data
		} else {
			var req struct {
				Filename    string `json:"filename"`
				ContentType string `json:"content_type"`
				ImageData   string `json:"image_data"`
				AvatarURL   string `json:"avatar_url"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}

			if req.AvatarURL != "" {
				updatedProfile, err := h.characters.UpdateProfile(r.Context(), charID, character.UpdateProfileRequest{
					AvatarURL: &req.AvatarURL,
				})
				if err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
					return
				}
				writeJSON(w, http.StatusOK, map[string]string{"avatar_url": updatedProfile.AvatarURL})
				return
			}

			filename = req.Filename
			mimeType = req.ContentType
			imageBytes = []byte(req.ImageData)
		}

		avatarURL, err := h.characters.UploadAvatar(r.Context(), charID, filename, mimeType, imageBytes)
		if err != nil {
			switch {
			case errors.Is(err, character.ErrInvalidImageFormat),
				errors.Is(err, character.ErrImageTooLarge),
				errors.Is(err, character.ErrInvalidAvatarURL):
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			case errors.Is(err, corecharacter.ErrNotFound):
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "character not found"})
			default:
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to upload avatar"})
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"avatar_url": avatarURL})
	})
}
