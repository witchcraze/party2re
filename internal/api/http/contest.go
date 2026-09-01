package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type ContestService interface {
	GetDialogue() contest.Dialogue
	GetOverview(ctx context.Context) (contest.ContestOverview, error)
	SavePhoto(ctx context.Context, characterID, title, location, imageURL, caption, metadata string) (contest.Photo, error)
	ListPhotos(ctx context.Context, characterID string) ([]contest.Photo, error)
	DeletePhoto(ctx context.Context, characterID, photoID string) error
	EnterContest(ctx context.Context, characterID, photoID, title string) (contest.ContestEntry, error)
	Vote(ctx context.Context, voterCharacterID, entryID, comment string) (contest.ContestVote, error)
	GetCurrentEntries(ctx context.Context) ([]contest.ContestEntry, error)
	GetPastResults(ctx context.Context) (*contest.ContestRound, []contest.ContestEntry, error)
	GetLegends(ctx context.Context, limit, offset int) ([]contest.ContestLegend, error)
	SettleContest(ctx context.Context, force bool) (contest.SettlementResult, error)
}

// WithContest configures the ContestService for the Handler.
func WithContest(c ContestService) Option {
	return func(h *Handler) {
		h.contest = c
	}
}

type savePhotoRequest struct {
	Title    string `json:"title"`
	Location string `json:"location"`
	ImageURL string `json:"image_url"`
	Caption  string `json:"caption"`
	Metadata string `json:"metadata"`
}

type enterContestRequest struct {
	PhotoID string `json:"photo_id"`
	Title   string `json:"title"`
}

type voteContestRequest struct {
	EntryID string `json:"entry_id"`
	Comment string `json:"comment"`
}

type settleContestRequest struct {
	Force bool `json:"force"`
}

func (h *Handler) handleGetContestVenue(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	overview, err := h.contest.GetOverview(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, overview)
}

func (h *Handler) handleGetContestCurrent(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	entries, err := h.contest.GetCurrentEntries(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, entries)
}

func (h *Handler) handleGetContestPast(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	round, entries, err := h.contest.GetPastResults(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	res := map[string]any{
		"round":   round,
		"entries": entries,
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *Handler) handleGetContestLegends(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))

	legends, err := h.contest.GetLegends(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}

	writeJSON(w, http.StatusOK, legends)
}

func (h *Handler) handleSettleContest(w http.ResponseWriter, r *http.Request) {
	if !h.authenticateAdmin(w, r) {
		return
	}

	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	var req settleContestRequest
	if r.Body != nil && r.ContentLength > 0 {
		_ = json.NewDecoder(r.Body).Decode(&req)
	}

	result, err := h.contest.SettleContest(r.Context(), req.Force)
	if err != nil {
		switch {
		case errors.Is(err, contest.ErrContestNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, contest.ErrContestNotReadyToSettle):
			writeError(w, http.StatusBadRequest, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

func (h *Handler) handleGetCharacterPhotos(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		photos, err := h.contest.ListPhotos(r.Context(), char.ID)
		if err != nil {
			if errors.Is(err, contest.ErrCharacterNotFound) {
				writeError(w, http.StatusNotFound, err)
				return
			}
			writeError(w, http.StatusInternalServerError, err)
			return
		}

		writeJSON(w, http.StatusOK, photos)
	})
}

func (h *Handler) handleSaveCharacterPhoto(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req savePhotoRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		photo, err := h.contest.SavePhoto(r.Context(), char.ID, req.Title, req.Location, req.ImageURL, req.Caption, req.Metadata)
		if err != nil {
			switch {
			case errors.Is(err, contest.ErrInvalidTitle),
				errors.Is(err, contest.ErrTitleTooLong),
				errors.Is(err, contest.ErrMaxPhotosReached):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, contest.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusCreated, photo)
	})
}

func (h *Handler) handleDeleteCharacterPhoto(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		photoID := r.PathValue("photoId")
		if photoID == "" {
			writeError(w, http.StatusBadRequest, errors.New("photoId is required"))
			return
		}

		err := h.contest.DeletePhoto(r.Context(), char.ID, photoID)
		if err != nil {
			switch {
			case errors.Is(err, contest.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, contest.ErrPhotoNotFound),
				errors.Is(err, contest.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, map[string]string{"message": "photo deleted successfully"})
	})
}

func (h *Handler) handleEnterContest(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req enterContestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		entry, err := h.contest.EnterContest(r.Context(), char.ID, req.PhotoID, req.Title)
		if err != nil {
			switch {
			case errors.Is(err, contest.ErrInvalidTitle),
				errors.Is(err, contest.ErrTitleTooLong),
				errors.Is(err, contest.ErrConsecutiveEntryDisallowed),
				errors.Is(err, contest.ErrAlreadyEntered),
				errors.Is(err, contest.ErrDuplicateTitle):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, contest.ErrForbidden):
				writeError(w, http.StatusForbidden, err)
			case errors.Is(err, contest.ErrPhotoNotFound),
				errors.Is(err, contest.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusCreated, entry)
	})
}

func (h *Handler) handleVoteContest(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req voteContestRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, errors.New("invalid request body"))
			return
		}

		vote, err := h.contest.Vote(r.Context(), char.ID, req.EntryID, req.Comment)
		if err != nil {
			switch {
			case errors.Is(err, contest.ErrContestNotActive),
				errors.Is(err, contest.ErrAlreadyVoted),
				errors.Is(err, contest.ErrSelfVoteDisallowed),
				errors.Is(err, contest.ErrCommentTooLong):
				writeError(w, http.StatusBadRequest, err)
			case errors.Is(err, contest.ErrEntryNotFound),
				errors.Is(err, contest.ErrCharacterNotFound):
				writeError(w, http.StatusNotFound, err)
			default:
				writeError(w, http.StatusInternalServerError, err)
			}
			return
		}

		writeJSON(w, http.StatusOK, vote)
	})
}
