package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/ranking"
)

type mockHTTPRankingService struct {
	pvpVictoryPage    ranking.RankingPage[ranking.CharacterRankingEntry]
	monsterKillsPage  ranking.RankingPage[ranking.CharacterRankingEntry]
	maoCountPage      ranking.RankingPage[ranking.CharacterRankingEntry]
	heroCountPage     ranking.RankingPage[ranking.CharacterRankingEntry]
	jobPopularityPage ranking.RankingPage[ranking.JobPopularityEntry]
	helperPage        ranking.RankingPage[ranking.CharacterRankingEntry]
	casinoWinsPage    ranking.RankingPage[ranking.CharacterRankingEntry]
	alchemyPage       ranking.RankingPage[ranking.CharacterRankingEntry]
	weeklyJobPage     ranking.RankingPage[ranking.CharacterRankingEntry]
	legends           []ranking.LegendCategoryInfo
	legendPages       map[ranking.LegendCategory]ranking.LegendCategoryPage
	refreshedTypes    []ranking.RankingType
	refreshedAll      bool
	lastLimit         int
	lastOffset        int
}

func (m *mockHTTPRankingService) GetMonsterKillsRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.monsterKillsPage, nil
}
func (m *mockHTTPRankingService) GetMaoCountRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.maoCountPage, nil
}
func (m *mockHTTPRankingService) GetHeroCountRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.heroCountPage, nil
}
func (m *mockHTTPRankingService) GetPvPVictoryRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.pvpVictoryPage, nil
}
func (m *mockHTTPRankingService) GetJobPopularityRanking(ctx context.Context) (ranking.RankingPage[ranking.JobPopularityEntry], error) {
	return m.jobPopularityPage, nil
}
func (m *mockHTTPRankingService) GetHelperRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	m.lastLimit = limit
	m.lastOffset = offset
	page := m.helperPage
	page.Limit = limit
	page.Offset = offset
	return page, nil
}
func (m *mockHTTPRankingService) GetCasinoWinsRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.casinoWinsPage, nil
}
func (m *mockHTTPRankingService) GetAlchemyRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.alchemyPage, nil
}
func (m *mockHTTPRankingService) GetWeeklyJobChangeRanking(ctx context.Context, limit, offset int) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.weeklyJobPage, nil
}
func (m *mockHTTPRankingService) GetLegends(ctx context.Context) ([]ranking.LegendCategoryInfo, error) {
	return m.legends, nil
}
func (m *mockHTTPRankingService) GetLegendCategory(ctx context.Context, category ranking.LegendCategory) (ranking.LegendCategoryPage, error) {
	if !ranking.IsValidLegendCategory(category) {
		return ranking.LegendCategoryPage{}, ranking.ErrInvalidLegendCategory
	}
	if m.legendPages != nil {
		if page, ok := m.legendPages[category]; ok {
			return page, nil
		}
	}
	return ranking.LegendCategoryPage{Category: category}, nil
}
func (m *mockHTTPRankingService) GetRankingByType(ctx context.Context, rankingType ranking.RankingType, limit, offset int) (any, error) {
	m.lastLimit = limit
	m.lastOffset = offset
	switch ranking.NormalizeRankingType(rankingType) {
	case ranking.RankingTypePvPVictory:
		return m.pvpVictoryPage, nil
	case ranking.RankingTypeJobPopularity:
		return m.jobPopularityPage, nil
	case ranking.RankingTypeHelper:
		return m.helperPage, nil
	case ranking.RankingTypeCasinoWins:
		return m.casinoWinsPage, nil
	case ranking.RankingTypeAlchemy:
		return m.alchemyPage, nil
	case ranking.RankingTypeWeeklyJobChange:
		return m.weeklyJobPage, nil
	case ranking.RankingTypeMonsterKills:
		page := m.monsterKillsPage
		page.Limit = limit
		page.Offset = offset
		return page, nil
	case ranking.RankingTypeMaoCount:
		return m.maoCountPage, nil
	case ranking.RankingTypeHeroCount:
		return m.heroCountPage, nil
	default:
		return nil, ranking.ErrInvalidRankingType
	}
}
func (m *mockHTTPRankingService) RefreshSnapshot(ctx context.Context, rankingType ranking.RankingType) error {
	m.refreshedTypes = append(m.refreshedTypes, rankingType)
	return nil
}
func (m *mockHTTPRankingService) RefreshAllSnapshots(ctx context.Context) error {
	m.refreshedAll = true
	return nil
}

func setupRankingHandler(rankings apihttp.RankingService) *apihttp.Handler {
	players := &stubPlayerService{}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{ID: id, PlayerID: "p1"}, nil
		},
	}
	adventures := &stubAdventureService{}
	shops := &stubShopService{}

	opts := []apihttp.Option{
		apihttp.WithAdminAPIKey("test-admin-key"),
	}
	if rankings != nil {
		opts = append(opts, apihttp.WithRanking(rankings))
	}

	h, _ := apihttp.NewHandler(players, chars, adventures, shops, opts...)
	return h
}

func TestRankingEndpoints(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	mock := &mockHTTPRankingService{
		legendPages: map[ranking.LegendCategory]ranking.LegendCategoryPage{
			ranking.LegendCategoryJobMastery: {
				Category: ranking.LegendCategoryJobMastery,
				Total:    1,
				Entries:  []ranking.LegendEntry{{CharacterID: "c1", CharacterName: "Hero", Color: "#123456"}},
			},
		},
		pvpVictoryPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypePvPVictory,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", CharacterName: "Hero", Score: 12}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
			IsSnapshot:   true,
		},
		monsterKillsPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeMonsterKills,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", CharacterName: "Hero", Score: 42}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
			IsSnapshot:   true,
		},
		maoCountPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeMaoCount,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 3}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		heroCountPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeHeroCount,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 7}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		jobPopularityPage: ranking.RankingPage[ranking.JobPopularityEntry]{
			RankingType:  ranking.RankingTypeJobPopularity,
			Entries:      []ranking.JobPopularityEntry{{Rank: 1, JobID: "warrior", TotalCount: 10, Percentage: 50.0}},
			Total:        1,
			Limit:        1,
			Offset:       0,
			CalculatedAt: now,
		},
		helperPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeHelper,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 5}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
	}

	h := setupRankingHandler(mock)
	router := h.Router()

	tests := []struct {
		name       string
		method     string
		url        string
		body       string
		headers    map[string]string
		wantStatus int
		checkBody  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "GET /rankings/monster-kills (snapshot=false ignored)",
			method:     http.MethodGet,
			url:        "/rankings/monster-kills?limit=10&offset=0&snapshot=false",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var res ranking.RankingPage[ranking.CharacterRankingEntry]
				if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if !res.IsSnapshot || res.Total != 1 || len(res.Entries) != 1 || res.Entries[0].CharacterName != "Hero" || res.Entries[0].Score != 42 {
					t.Fatalf("unexpected body: %+v", res)
				}
			},
		},
		{
			name:       "GET /rankings/mao-count",
			method:     http.MethodGet,
			url:        "/rankings/mao-count",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/hero-count",
			method:     http.MethodGet,
			url:        "/rankings/hero-count",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/job-popularity",
			method:     http.MethodGet,
			url:        "/rankings/job-popularity",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/helpers (limit and offset)",
			method:     http.MethodGet,
			url:        "/rankings/helpers?limit=5&offset=2",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var res ranking.RankingPage[ranking.CharacterRankingEntry]
				if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if res.Limit != 5 || res.Offset != 2 || mock.lastLimit != 5 || mock.lastOffset != 2 {
					t.Fatalf("expected limit=5 offset=2, got page=(%d,%d) mock=(%d,%d)", res.Limit, res.Offset, mock.lastLimit, mock.lastOffset)
				}
			},
		},
		{
			name:       "GET /rankings/pvp_victory (dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/pvp_victory",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/kill_p (legacy alias dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/kill_p",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/job_popularity (dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/job_popularity",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/monster_kills (dynamic route with limit and offset)",
			method:     http.MethodGet,
			url:        "/rankings/monster_kills?limit=3&offset=4",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var res ranking.RankingPage[ranking.CharacterRankingEntry]
				if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if res.Limit != 3 || res.Offset != 4 || mock.lastLimit != 3 || mock.lastOffset != 4 {
					t.Fatalf("expected limit=3 offset=4, got page=(%d,%d) mock=(%d,%d)", res.Limit, res.Offset, mock.lastLimit, mock.lastOffset)
				}
			},
		},
		{
			name:       "GET /rankings/kill_m (legacy alias dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/kill_m",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/mao_count (dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/mao_count",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/mao_c (legacy alias dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/mao_c",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/hero_count (dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/hero_count",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/hero_c (legacy alias dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/hero_c",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/levels (removed -> 400 via dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/levels",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/level (removed -> 400)",
			method:     http.MethodGet,
			url:        "/rankings/level",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/wealth (removed -> 400)",
			method:     http.MethodGet,
			url:        "/rankings/wealth",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/characters-wealth (removed -> 400)",
			method:     http.MethodGet,
			url:        "/rankings/characters-wealth",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/job-mastery (removed -> 400)",
			method:     http.MethodGet,
			url:        "/rankings/job-mastery",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/medals (removed -> 400)",
			method:     http.MethodGet,
			url:        "/rankings/medals",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/unknown_type (400 Bad Request)",
			method:     http.MethodGet,
			url:        "/rankings/unknown_type",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "GET /rankings/casino-wins",
			method:     http.MethodGet,
			url:        "/rankings/casino-wins",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/alchemy",
			method:     http.MethodGet,
			url:        "/rankings/alchemy",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/weekly-job-change",
			method:     http.MethodGet,
			url:        "/rankings/weekly-job-change",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/cas_c (legacy alias)",
			method:     http.MethodGet,
			url:        "/rankings/cas_c",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/alc_c (legacy alias)",
			method:     http.MethodGet,
			url:        "/rankings/alc_c",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /legends",
			method:     http.MethodGet,
			url:        "/legends",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /legends/comp_job",
			method:     http.MethodGet,
			url:        "/legends/comp_job",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var page ranking.LegendCategoryPage
				if err := json.NewDecoder(rr.Body).Decode(&page); err != nil {
					t.Fatal(err)
				}
				want := mock.legendPages[ranking.LegendCategoryJobMastery]
				if page.Category != want.Category || page.Total != want.Total || len(page.Entries) != 1 || page.Entries[0] != want.Entries[0] {
					t.Fatalf("legend response = %+v, want %+v", page, want)
				}
			},
		},
		{
			name:       "GET /legends/invalid_cat (400 Bad Request)",
			method:     http.MethodGet,
			url:        "/legends/invalid_cat",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "POST /rankings/refresh (missing admin credentials -> 401)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "POST /rankings/refresh (invalid admin key -> 403)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			headers:    map[string]string{"X-Admin-Key": "wrong-key"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "POST /rankings/refresh (all with X-Admin-Key)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			headers:    map[string]string{"X-Admin-Key": "test-admin-key"},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !mock.refreshedAll {
					t.Fatalf("expected refreshedAll=true")
				}
			},
		},
		{
			name:       "POST /rankings/refresh (all with Bearer token)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			headers:    map[string]string{"Authorization": "Bearer test-admin-key"},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !mock.refreshedAll {
					t.Fatalf("expected refreshedAll=true")
				}
			},
		},
		{
			name:       "POST /rankings/refresh (single type with X-Admin-Key)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			body:       `{"ranking_type":"monster_kills"}`,
			headers:    map[string]string{"X-Admin-Key": "test-admin-key"},
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if len(mock.refreshedTypes) == 0 || mock.refreshedTypes[0] != ranking.RankingTypeMonsterKills {
					t.Fatalf("expected monster_kills in refreshedTypes: %+v", mock.refreshedTypes)
				}
			},
		},
		{
			name:       "POST /rankings/refresh (removed type with X-Admin-Key)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			body:       `{"ranking_type":"level"}`,
			headers:    map[string]string{"X-Admin-Key": "test-admin-key"},
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "POST /rankings/refresh (invalid type with X-Admin-Key)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			body:       `{"ranking_type":"invalid_category"}`,
			headers:    map[string]string{"X-Admin-Key": "test-admin-key"},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var bodyReader *bytes.Reader
			if tt.body != "" {
				bodyReader = bytes.NewReader([]byte(tt.body))
			} else {
				bodyReader = bytes.NewReader([]byte{})
			}
			req := httptest.NewRequest(tt.method, tt.url, bodyReader)
			req.Header.Set("Content-Type", "application/json")
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Fatalf("expected status %d, got %d. Body: %s", tt.wantStatus, rec.Code, rec.Body.String())
			}
			if tt.checkBody != nil {
				tt.checkBody(t, rec)
			}
		})
	}
}

func TestRankingEndpoints_NotConfigured(t *testing.T) {
	h := setupRankingHandler(nil)
	router := h.Router()

	req := httptest.NewRequest(http.MethodGet, "/rankings/monster-kills", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 Not Implemented, got %d", rec.Code)
	}
}

type snapshotReadTestRepo struct {
	ranking.Repository
	getSnapshot func(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error)
}

func (r *snapshotReadTestRepo) GetSnapshot(ctx context.Context, rankingType ranking.RankingType) (ranking.RankingSnapshot, error) {
	return r.getSnapshot(ctx, rankingType)
}

func (r *snapshotReadTestRepo) GetAllSnapshots(ctx context.Context) (map[ranking.RankingType]ranking.RankingSnapshot, error) {
	out := make(map[ranking.RankingType]ranking.RankingSnapshot)
	for _, rt := range []ranking.RankingType{
		ranking.RankingTypePvPVictory,
		ranking.RankingTypeMonsterKills,
		ranking.RankingTypeMaoCount,
		ranking.RankingTypeHeroCount,
		ranking.RankingTypeJobPopularity,
		ranking.RankingTypeHelper,
		ranking.RankingTypeCasinoWins,
		ranking.RankingTypeAlchemy,
		ranking.RankingTypeWeeklyJobChange,
	} {
		snap, err := r.getSnapshot(ctx, rt)
		if errors.Is(err, ranking.ErrSnapshotNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		out[rt] = snap
	}
	return out, nil
}

func TestRankingEndpoints_RealServiceSnapshotErrorsAndAbsence(t *testing.T) {
	endpoints := []string{
		"/rankings/monster-kills",
		"/rankings/job-popularity",
		"/rankings/weekly-job-change",
		"/rankings/pvp_victory",
		"/rankings/kill_m",
	}

	cases := []struct {
		name       string
		get        func(context.Context, ranking.RankingType) (ranking.RankingSnapshot, error)
		wantStatus int
		wantBody   string
	}{
		{
			name: "unpublished snapshot returns 200 empty page",
			get: func(context.Context, ranking.RankingType) (ranking.RankingSnapshot, error) {
				return ranking.RankingSnapshot{}, ranking.ErrSnapshotNotFound
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "sql read failure returns 500 internal server error",
			get: func(context.Context, ranking.RankingType) (ranking.RankingSnapshot, error) {
				return ranking.RankingSnapshot{}, errors.New("db read failed")
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
		{
			name: "invalid published snapshot json returns 500 internal server error",
			get: func(ctx context.Context, rt ranking.RankingType) (ranking.RankingSnapshot, error) {
				return ranking.RankingSnapshot{
					RankingType:  rt,
					SnapshotData: "{invalid-json",
					TotalCount:   1,
				}, nil
			},
			wantStatus: http.StatusInternalServerError,
			wantBody:   "internal server error",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, err := ranking.NewService(&snapshotReadTestRepo{getSnapshot: tc.get}, ranking.WithCacheTTL(0))
			if err != nil {
				t.Fatalf("ranking.NewService: %v", err)
			}
			router := setupRankingHandler(svc).Router()

			for _, url := range endpoints {
				req := httptest.NewRequest(http.MethodGet, url, nil)
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, req)

				if rec.Code != tc.wantStatus {
					t.Fatalf("%s: expected status %d, got %d (body: %s)", url, tc.wantStatus, rec.Code, rec.Body.String())
				}
				if tc.wantStatus == http.StatusOK {
					var page struct {
						Entries    []json.RawMessage `json:"entries"`
						Total      int               `json:"total"`
						IsSnapshot bool              `json:"is_snapshot"`
					}
					if err := json.NewDecoder(rec.Body).Decode(&page); err != nil {
						t.Fatalf("%s: decode 200 body: %v", url, err)
					}
					if page.IsSnapshot || page.Total != 0 || page.Entries == nil || len(page.Entries) != 0 {
						t.Fatalf("%s: expected empty unpublished page, got %+v", url, page)
					}
				} else {
					var errBody map[string]string
					if err := json.NewDecoder(rec.Body).Decode(&errBody); err != nil {
						t.Fatalf("%s: decode error body: %v", url, err)
					}
					if errBody["error"] != tc.wantBody {
						t.Fatalf("%s: expected error %q, got %q", url, tc.wantBody, errBody["error"])
					}
				}
			}
		})
	}
}

// Unused imports check suppression
var (
	_ = coreplayer.Player{}
)
