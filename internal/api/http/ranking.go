package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/witchcraze/party2re/internal/ranking"
)

// RankingService defines leaderboard and ranking queries exposed over HTTP.
type RankingService interface {
	GetLevelRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetPlayerWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.PlayerWealthRankingEntry], error)
	GetCharacterWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetBattleVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetPvPVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetBossDefeatRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetAdventureVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetJobMasteryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetJobPopularityRanking(ctx context.Context, useSnapshot bool) (ranking.RankingPage[ranking.JobPopularityEntry], error)
	GetHelperRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetRebirthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetSmallMedalRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetRankingByType(ctx context.Context, rankingType ranking.RankingType, limit, offset int, useSnapshot bool) (any, error)
	RefreshSnapshot(ctx context.Context, rankingType ranking.RankingType) error
	RefreshAllSnapshots(ctx context.Context) error
}

// WithRanking configures the RankingService for the Handler.
func WithRanking(r RankingService) Option {
	return func(h *Handler) {
		h.rankings = r
	}
}

func parsePaginationAndSnapshotParams(r *http.Request) (limit int, offset int, useSnapshot bool) {
	q := r.URL.Query()
	limit = ranking.DefaultLimit
	if l := q.Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	offset = 0
	if o := q.Get("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}
	limit, offset = ranking.NormalizePagination(limit, offset)

	useSnapshot = true
	if s := q.Get("snapshot"); s == "false" || s == "0" {
		useSnapshot = false
	}
	return limit, offset, useSnapshot
}

func (h *Handler) handleGetLevelRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetLevelRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetPlayerWealthRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetPlayerWealthRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetCharacterWealthRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetCharacterWealthRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetBattleRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetBattleVictoryRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetJobMasteryRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetJobMasteryRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetJobPopularityRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	_, _, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetJobPopularityRanking(r.Context(), useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetHelperRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetHelperRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetRebirthRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetRebirthRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetSmallMedalRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetSmallMedalRanking(r.Context(), limit, offset, useSnapshot)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetRankingByType(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	typeStr := r.PathValue("type")
	rankingType := ranking.RankingType(typeStr)
	if !ranking.IsValidRankingType(rankingType) {
		writeError(w, http.StatusBadRequest, ranking.ErrInvalidRankingType)
		return
	}

	limit, offset, useSnapshot := parsePaginationAndSnapshotParams(r)
	page, err := h.rankings.GetRankingByType(r.Context(), rankingType, limit, offset, useSnapshot)
	if err != nil {
		if errors.Is(err, ranking.ErrInvalidRankingType) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

type refreshRankingsRequest struct {
	RankingType string `json:"ranking_type,omitempty"`
}

func (h *Handler) handleRefreshRankings(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}

	if !h.authenticateAdmin(w, r) {
		return
	}

	var req refreshRankingsRequest
	if r.ContentLength > 0 {
		if !decodeJSON(w, r, &req) {
			return
		}
	}

	if req.RankingType != "" {
		rt := ranking.RankingType(req.RankingType)
		if !ranking.IsValidRankingType(rt) {
			writeError(w, http.StatusBadRequest, ranking.ErrInvalidRankingType)
			return
		}
		if err := h.rankings.RefreshSnapshot(r.Context(), rt); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	} else {
		if err := h.rankings.RefreshAllSnapshots(r.Context()); err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "refreshed"})
}
