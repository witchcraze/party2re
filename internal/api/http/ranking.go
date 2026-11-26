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
	GetMonsterKillsRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetMaoCountRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetHeroCountRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetPvPVictoryRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetJobPopularityRanking(ctx context.Context) (ranking.RankingPage[ranking.JobPopularityEntry], error)
	GetHelperRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetCasinoWinsRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetAlchemyRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetWeeklyJobChangeRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error)
	GetLegends(ctx context.Context) ([]ranking.LegendCategoryInfo, error)
	GetLegendCategory(ctx context.Context, category ranking.LegendCategory) (ranking.LegendCategoryPage, error)
	GetRankingByType(ctx context.Context, rankingType ranking.RankingType, limit, offset int) (any, error)
	RefreshSnapshot(ctx context.Context, rankingType ranking.RankingType) error
	RefreshAllSnapshots(ctx context.Context) error
}

// WithRanking configures the RankingService for the Handler.
func WithRanking(r RankingService) Option {
	return func(h *Handler) {
		h.rankings = r
	}
}

func parsePaginationParams(r *http.Request) (limit int, offset int) {
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
	return ranking.NormalizePagination(limit, offset)
}

func (h *Handler) handleGetMonsterKillsRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetMonsterKillsRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetMaoCountRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetMaoCountRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetHeroCountRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetHeroCountRanking(r.Context(), limit, offset)
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
	page, err := h.rankings.GetJobPopularityRanking(r.Context())
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
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetHelperRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetCasinoWinsRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetCasinoWinsRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetAlchemyRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetAlchemyRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetWeeklyJobChangeRanking(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetWeeklyJobChangeRanking(r.Context(), limit, offset)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (h *Handler) handleGetLegends(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	legends, err := h.rankings.GetLegends(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, legends)
}

func (h *Handler) handleGetLegendCategory(w http.ResponseWriter, r *http.Request) {
	if h.rankings == nil {
		writeError(w, http.StatusNotImplemented, errors.New("ranking service not configured"))
		return
	}
	catStr := r.PathValue("category")
	category := ranking.LegendCategory(catStr)
	if !ranking.IsValidLegendCategory(category) {
		writeError(w, http.StatusBadRequest, ranking.ErrInvalidLegendCategory)
		return
	}
	page, err := h.rankings.GetLegendCategory(r.Context(), category)
	if err != nil {
		if errors.Is(err, ranking.ErrInvalidLegendCategory) {
			writeError(w, http.StatusBadRequest, err)
			return
		}
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

	limit, offset := parsePaginationParams(r)
	page, err := h.rankings.GetRankingByType(r.Context(), rankingType, limit, offset)
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
