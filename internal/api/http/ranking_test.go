package http_test

import (
	"bytes"
	"context"
	"encoding/json"
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
	levelPage         ranking.RankingPage[ranking.CharacterRankingEntry]
	wealthPage        ranking.RankingPage[ranking.PlayerWealthRankingEntry]
	charWealthPage    ranking.RankingPage[ranking.CharacterRankingEntry]
	battlePage        ranking.RankingPage[ranking.CharacterRankingEntry]
	jobMasteryPage    ranking.RankingPage[ranking.CharacterRankingEntry]
	jobPopularityPage ranking.RankingPage[ranking.JobPopularityEntry]
	helperPage        ranking.RankingPage[ranking.CharacterRankingEntry]
	rebirthPage       ranking.RankingPage[ranking.CharacterRankingEntry]
	medalPage         ranking.RankingPage[ranking.CharacterRankingEntry]
	refreshedTypes    []ranking.RankingType
	refreshedAll      bool
}

func (m *mockHTTPRankingService) GetLevelRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.levelPage, nil
}
func (m *mockHTTPRankingService) GetPlayerWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.PlayerWealthRankingEntry], error) {
	return m.wealthPage, nil
}
func (m *mockHTTPRankingService) GetCharacterWealthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.charWealthPage, nil
}
func (m *mockHTTPRankingService) GetBattleVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.battlePage, nil
}
func (m *mockHTTPRankingService) GetPvPVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.battlePage, nil
}
func (m *mockHTTPRankingService) GetBossDefeatRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.battlePage, nil
}
func (m *mockHTTPRankingService) GetAdventureVictoryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.battlePage, nil
}
func (m *mockHTTPRankingService) GetJobMasteryRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.jobMasteryPage, nil
}
func (m *mockHTTPRankingService) GetJobPopularityRanking(ctx context.Context, useSnapshot bool) (ranking.RankingPage[ranking.JobPopularityEntry], error) {
	return m.jobPopularityPage, nil
}
func (m *mockHTTPRankingService) GetHelperRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.helperPage, nil
}
func (m *mockHTTPRankingService) GetRebirthRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.rebirthPage, nil
}
func (m *mockHTTPRankingService) GetSmallMedalRanking(ctx context.Context, limit, offset int, useSnapshot bool) (ranking.RankingPage[ranking.CharacterRankingEntry], error) {
	return m.medalPage, nil
}
func (m *mockHTTPRankingService) GetRankingByType(ctx context.Context, rankingType ranking.RankingType, limit, offset int, useSnapshot bool) (any, error) {
	switch rankingType {
	case ranking.RankingTypeLevel:
		return m.levelPage, nil
	case ranking.RankingTypePlayerWealth:
		return m.wealthPage, nil
	case ranking.RankingTypeJobPopularity:
		return m.jobPopularityPage, nil
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

func setupRankingHandler(mock *mockHTTPRankingService) *apihttp.Handler {
	players := &stubPlayerService{}
	chars := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{ID: id, PlayerID: "p1"}, nil
		},
	}
	adventures := &stubAdventureService{}
	shops := &stubShopService{}

	opts := []apihttp.Option{}
	if mock != nil {
		opts = append(opts, apihttp.WithRanking(mock))
	}

	h, _ := apihttp.NewHandler(players, chars, adventures, shops, opts...)
	return h
}

func TestRankingEndpoints(t *testing.T) {
	now := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	mock := &mockHTTPRankingService{
		levelPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeLevel,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", CharacterName: "Hero", Level: 50}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
			IsSnapshot:   true,
		},
		wealthPage: ranking.RankingPage[ranking.PlayerWealthRankingEntry]{
			RankingType:  ranking.RankingTypePlayerWealth,
			Entries:      []ranking.PlayerWealthRankingEntry{{Rank: 1, PlayerID: "p1", Username: "rich", TotalWealth: 1000000}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		charWealthPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeCharacterWealth,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 50000}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		battlePage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeBattleVictory,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 25}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		jobMasteryPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeJobMastery,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 4}},
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
		rebirthPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeRebirth,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 2}},
			Total:        1,
			Limit:        20,
			Offset:       0,
			CalculatedAt: now,
		},
		medalPage: ranking.RankingPage[ranking.CharacterRankingEntry]{
			RankingType:  ranking.RankingTypeSmallMedals,
			Entries:      []ranking.CharacterRankingEntry{{Rank: 1, CharacterID: "c1", Score: 100}},
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
		wantStatus int
		checkBody  func(*testing.T, *httptest.ResponseRecorder)
	}{
		{
			name:       "GET /rankings/levels",
			method:     http.MethodGet,
			url:        "/rankings/levels?limit=10&offset=0&snapshot=true",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var res ranking.RankingPage[ranking.CharacterRankingEntry]
				if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if res.Total != 1 || len(res.Entries) != 1 || res.Entries[0].CharacterName != "Hero" {
					t.Fatalf("unexpected body: %+v", res)
				}
			},
		},
		{
			name:       "GET /rankings/wealth",
			method:     http.MethodGet,
			url:        "/rankings/wealth",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				var res ranking.RankingPage[ranking.PlayerWealthRankingEntry]
				if err := json.NewDecoder(rr.Body).Decode(&res); err != nil {
					t.Fatalf("decode failed: %v", err)
				}
				if res.Entries[0].TotalWealth != 1000000 {
					t.Fatalf("unexpected wealth: %+v", res)
				}
			},
		},
		{
			name:       "GET /rankings/characters-wealth",
			method:     http.MethodGet,
			url:        "/rankings/characters-wealth",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/battles",
			method:     http.MethodGet,
			url:        "/rankings/battles",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/job-mastery",
			method:     http.MethodGet,
			url:        "/rankings/job-mastery",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/job-popularity",
			method:     http.MethodGet,
			url:        "/rankings/job-popularity",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/helpers",
			method:     http.MethodGet,
			url:        "/rankings/helpers",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/rebirths",
			method:     http.MethodGet,
			url:        "/rankings/rebirths",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/medals",
			method:     http.MethodGet,
			url:        "/rankings/medals",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/level (dynamic route)",
			method:     http.MethodGet,
			url:        "/rankings/level",
			wantStatus: http.StatusOK,
		},
		{
			name:       "GET /rankings/unknown_type (400 Bad Request)",
			method:     http.MethodGet,
			url:        "/rankings/unknown_type",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "POST /rankings/refresh (all)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if !mock.refreshedAll {
					t.Fatalf("expected refreshedAll=true")
				}
			},
		},
		{
			name:       "POST /rankings/refresh (single type)",
			method:     http.MethodPost,
			url:        "/rankings/refresh",
			body:       `{"ranking_type":"level"}`,
			wantStatus: http.StatusOK,
			checkBody: func(t *testing.T, rr *httptest.ResponseRecorder) {
				if len(mock.refreshedTypes) == 0 || mock.refreshedTypes[0] != ranking.RankingTypeLevel {
					t.Fatalf("expected level in refreshedTypes: %+v", mock.refreshedTypes)
				}
			},
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

	req := httptest.NewRequest(http.MethodGet, "/rankings/levels", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("expected 501 Not Implemented, got %d", rec.Code)
	}
}

// Unused imports check suppression
var (
	_ = coreplayer.Player{}
)
