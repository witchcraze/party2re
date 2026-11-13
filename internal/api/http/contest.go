package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/pagination"
)

type ContestService interface {
	GetOverview(ctx context.Context) (contest.ContestOverview, error)
	SavePhoto(ctx context.Context, characterID, title, location, imageURL, caption, metadata string) (contest.Photo, error)
	ListPhotos(ctx context.Context, characterID string) ([]contest.Photo, error)
	DeletePhoto(ctx context.Context, characterID, photoID string) error
	EnterContest(ctx context.Context, characterID, photoID, title string) (contest.ContestEntry, error)
	Vote(ctx context.Context, voterCharacterID, entryID, comment string) (contest.ContestVote, error)
	GetCurrentEntries(ctx context.Context) ([]contest.ContestEntry, error)
	GetPastResults(ctx context.Context) (*contest.ContestRound, []contest.ContestEntry, error)
	GetLegends(ctx context.Context, limit, offset int) (pagination.Page[contest.ContestLegend], error)
	SettleContest(ctx context.Context, force bool) (contest.SettlementResult, error)
	ReconcileSettlementSchedule(ctx context.Context) error
}

// WithContest configures the ContestService for the Handler.
func WithContest(c ContestService) Option {
	return func(h *Handler) {
		h.contest = c
		h.registerContestCommands()
	}
}

type contestVenueResponse struct {
	ActiveRound    *contest.ContestRound  `json:"active_round,omitempty"`
	PreparingRound *contest.ContestRound  `json:"preparing_round,omitempty"`
	ActiveEntries  []contest.ContestEntry `json:"active_entries,omitempty"`
	EntryCount     int                    `json:"entry_count"`
	MinEntries     int                    `json:"min_entries"`
	IsPostponed    bool                   `json:"is_postponed"`
}

type savePhotoRequest struct {
	Title    string `json:"title"`
	Location string `json:"location"`
	ImageURL string `json:"image_url"`
	Caption  string `json:"caption"`
	Metadata string `json:"metadata"`
}

type settleContestRequest struct {
	Force bool `json:"force"`
}

type settleContestResponse struct {
	contest.SettlementResult
	Warning string `json:"warning,omitempty"`
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

	writeJSON(w, http.StatusOK, contestVenueResponse{
		ActiveRound:    overview.ActiveRound,
		PreparingRound: overview.PreparingRound,
		ActiveEntries:  overview.ActiveEntries,
		EntryCount:     overview.EntryCount,
		MinEntries:     overview.MinEntries,
		IsPostponed:    overview.IsPostponed,
	})
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

	params := pagination.ParseRequest(r)

	legends, err := h.contest.GetLegends(r.Context(), params.Limit, params.Offset)
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
	if !decodeOptionalJSON(w, r, &req) {
		return
	}

	result, err := h.contest.SettleContest(r.Context(), req.Force)
	if err != nil {
		if errors.Is(err, contest.ErrScheduleFailed) {
			writeJSON(w, http.StatusAccepted, settleContestResponse{
				SettlementResult: result,
				Warning:          "contest settled, but settlement scheduling failed: " + err.Error(),
			})
			return
		}
		switch {
		case errors.Is(err, contest.ErrContestNotFound):
			writeError(w, http.StatusNotFound, err)
		case errors.Is(err, contest.ErrContestNotReadyToSettle):
			// Opportunistically reconcile schedule for the unready round to recover missing reservations
			if recErr := h.contest.ReconcileSettlementSchedule(r.Context()); recErr != nil {
				writeError(w, http.StatusInternalServerError, recErr)
				return
			}
			writeError(w, http.StatusBadRequest, err)
		default:
			writeError(w, http.StatusInternalServerError, err)
		}
		return
	}

	writeJSON(w, http.StatusOK, settleContestResponse{SettlementResult: result})
}

func (h *Handler) handleSaveCharacterPhoto(w http.ResponseWriter, r *http.Request) {
	if h.contest == nil {
		writeError(w, http.StatusNotImplemented, errors.New("contest service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedActionCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		var req savePhotoRequest
		if !decodeJSON(w, r, &req) {
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
