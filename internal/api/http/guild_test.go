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
	"github.com/witchcraze/party2re/internal/guild"
)

type stubGuildService struct {
	guilds        []guild.Guild
	detail        guild.Detail
	member        guild.Member
	char          corecharacter.Character
	err           error
	lastCallout   string
	lastCustomCol string
	lastCustomMrk string
	lastCustomWlp string
	lastNotice    string
	lastTitle     string
	approvedApp   bool
	rejectedApp   bool
	disbanded     bool
	left          bool
	kicked        bool
}

func (s *stubGuildService) Get(_ context.Context, guildID string) (guild.Detail, error) {
	if s.err != nil {
		return guild.Detail{}, s.err
	}
	return s.detail, nil
}

func (s *stubGuildService) GetByCharacter(_ context.Context, characterID string) (guild.Guild, guild.Member, error) {
	if s.err != nil {
		return guild.Guild{}, guild.Member{}, s.err
	}
	return s.detail.Guild, s.member, nil
}

func (s *stubGuildService) List(_ context.Context, offset, limit int) ([]guild.Guild, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.guilds, nil
}

func (s *stubGuildService) Create(_ context.Context, creatorCharID, name string) (guild.Guild, guild.Member, corecharacter.Character, error) {
	if s.err != nil {
		return guild.Guild{}, guild.Member{}, corecharacter.Character{}, s.err
	}
	g := guild.Guild{ID: "guild-1", Name: name, LeaderCharacterID: creatorCharID}
	m := guild.Member{GuildID: "guild-1", CharacterID: creatorCharID, Role: guild.RoleLeader, Title: guild.DefaultTitleLeader}
	return g, m, s.char, nil
}

func (s *stubGuildService) ApplyToJoin(_ context.Context, guildID, applicantID string) error {
	return s.err
}

func (s *stubGuildService) ApproveApplication(_ context.Context, guildID, leaderID, applicantID, title string) error {
	if s.err != nil {
		return s.err
	}
	s.approvedApp = true
	s.lastTitle = title
	return nil
}

func (s *stubGuildService) RejectApplication(_ context.Context, guildID, leaderID, applicantID string) error {
	if s.err != nil {
		return s.err
	}
	s.rejectedApp = true
	return nil
}

func (s *stubGuildService) BroadcastCallout(_ context.Context, guildID, senderID, message string) error {
	if s.err != nil {
		return s.err
	}
	s.lastCallout = message
	return nil
}

func (s *stubGuildService) AssignCustomRole(_ context.Context, guildID, requesterCharID, targetCharID, title string) error {
	if s.err != nil {
		return s.err
	}
	s.lastTitle = title
	return nil
}

func (s *stubGuildService) Kick(_ context.Context, guildID, requesterCharID, targetCharID string) error {
	if s.err != nil {
		return s.err
	}
	s.kicked = true
	return nil
}

func (s *stubGuildService) UpdateColor(_ context.Context, guildID, requesterCharID, color string) error {
	if s.err != nil {
		return s.err
	}
	s.lastCustomCol = color
	return nil
}

func (s *stubGuildService) UpdateNotice(_ context.Context, guildID, requesterCharID, notice string) error {
	if s.err != nil {
		return s.err
	}
	s.lastNotice = notice
	return nil
}

func (s *stubGuildService) ChangeMark(_ context.Context, guildID, leaderID, mark string) (corecharacter.Character, error) {
	if s.err != nil {
		return corecharacter.Character{}, s.err
	}
	s.lastCustomMrk = mark
	return s.char, nil
}

func (s *stubGuildService) ChangeWallpaper(_ context.Context, guildID, leaderID, wallpaper string) (corecharacter.Character, error) {
	if s.err != nil {
		return corecharacter.Character{}, s.err
	}
	s.lastCustomWlp = wallpaper
	return s.char, nil
}

func (s *stubGuildService) Disband(_ context.Context, guildID, leaderCharID string) error {
	if s.err != nil {
		return s.err
	}
	s.disbanded = true
	return nil
}

func (s *stubGuildService) Leave(_ context.Context, guildID, characterID string) error {
	if s.err != nil {
		return s.err
	}
	s.left = true
	return nil
}

func newGuildJSONRequest(method, url string, body any, sessionID string) *http.Request {
	var r *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	} else {
		r = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, url, r)
	req.Header.Set("Content-Type", "application/json")
	if sessionID != "" {
		req.Header.Set("Authorization", "Bearer "+sessionID)
	}
	return req
}

func setupGuildTest(guildSvc *stubGuildService) (http.Handler, string, string) {
	const (
		sessionID = "sess-token"
		charID    = "char-1"
		playerID  = "player-1"
	)

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sid string) (coreplayer.Player, error) {
			if sid == sessionID {
				return coreplayer.Player{ID: playerID, Username: "HeroPlayer"}, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}

	charSvc := &stubCharacterServiceExtended{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return corecharacter.Character{
				ID:       id,
				PlayerID: playerID,
				Name:     "LeaderHero",
				Money:    10000,
			}, nil
		},
	}

	opts := []apihttp.Option{}
	if guildSvc != nil {
		opts = append(opts, apihttp.WithGuild(guildSvc))
	}

	handler, _ := apihttp.NewHandler(
		playerSvc,
		charSvc,
		&stubAdventureService{},
		&stubShopService{},
		opts...,
	)

	return handler.Router(), sessionID, charID
}

func TestGuild_ListAndGet(t *testing.T) {
	now := time.Now().UTC()
	guildSvc := &stubGuildService{
		guilds: []guild.Guild{
			{ID: "g1", Name: "Alpha", LeaderCharacterID: "c1", Points: 100, CreatedAt: now},
			{ID: "g2", Name: "Beta", LeaderCharacterID: "c2", Points: 50, CreatedAt: now},
		},
		detail: guild.Detail{
			Guild: guild.Guild{ID: "g1", Name: "Alpha", LeaderCharacterID: "c1", Points: 100, CreatedAt: now},
			Members: []guild.Member{
				{GuildID: "g1", CharacterID: "c1", Role: guild.RoleLeader, Title: guild.DefaultTitleLeader},
			},
		},
		member: guild.Member{GuildID: "g1", CharacterID: "c1", Role: guild.RoleLeader, Title: guild.DefaultTitleLeader},
	}

	router, _, charID := setupGuildTest(guildSvc)

	// 1. GET /guilds
	req := httptest.NewRequest(http.MethodGet, "/guilds", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /guilds returned %d, want 200", w.Code)
	}
	var list []guild.Guild
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("failed to decode GET /guilds response: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("expected 2 guilds, got %d", len(list))
	}

	// 2. GET /guilds/g1
	req = httptest.NewRequest(http.MethodGet, "/guilds/g1", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /guilds/g1 returned %d, want 200", w.Code)
	}
	var detail guild.Detail
	if err := json.NewDecoder(w.Body).Decode(&detail); err != nil {
		t.Fatalf("failed to decode GET /guilds/g1 response: %v", err)
	}
	if detail.Guild.ID != "g1" || len(detail.Members) != 1 {
		t.Fatalf("unexpected detail: %+v", detail)
	}

	// 3. GET /characters/{id}/guild
	req = httptest.NewRequest(http.MethodGet, "/characters/"+charID+"/guild", nil)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("GET /characters/%s/guild returned %d, want 200", charID, w.Code)
	}
}

func TestGuild_Create(t *testing.T) {
	guildSvc := &stubGuildService{}
	router, sessionID, charID := setupGuildTest(guildSvc)

	req := newGuildJSONRequest(http.MethodPost, "/guilds", map[string]string{
		"character_id": charID,
		"name":         "Knights",
	}, sessionID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("POST /guilds returned %d, want 201: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	gData, ok := resp["guild"].(map[string]any)
	if !ok || gData["name"] != "Knights" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestGuild_MemberAndApplicationActions(t *testing.T) {
	guildSvc := &stubGuildService{}
	router, sessionID, charID := setupGuildTest(guildSvc)

	// 1. POST /guilds/g1/apply
	req := newGuildJSONRequest(http.MethodPost, "/guilds/g1/apply", map[string]string{
		"character_id": charID,
	}, sessionID)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST /guilds/g1/apply returned %d, want 200: %s", w.Code, w.Body.String())
	}

	// 2. POST /guilds/g1/applications/c2/approve
	req = newGuildJSONRequest(http.MethodPost, "/guilds/g1/applications/c2/approve", map[string]string{
		"character_id": charID,
		"title":        "先鋒",
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST approve returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if !guildSvc.approvedApp || guildSvc.lastTitle != "先鋒" {
		t.Fatalf("approve not called properly")
	}

	// 3. POST /guilds/g1/applications/c3/reject
	req = newGuildJSONRequest(http.MethodPost, "/guilds/g1/applications/c3/reject", map[string]string{
		"character_id": charID,
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST reject returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if !guildSvc.rejectedApp {
		t.Fatalf("reject not called properly")
	}

	// 4. POST /guilds/g1/callout
	req = newGuildJSONRequest(http.MethodPost, "/guilds/g1/callout", map[string]string{
		"character_id": charID,
		"message":      "全員集合！",
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST callout returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if guildSvc.lastCallout != "全員集合！" {
		t.Fatalf("callout message mismatch: %s", guildSvc.lastCallout)
	}

	// 5. PUT /guilds/g1/members/c2/title
	req = newGuildJSONRequest(http.MethodPut, "/guilds/g1/members/c2/title", map[string]string{
		"character_id": charID,
		"title":        "参謀",
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT title returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if guildSvc.lastTitle != "参謀" {
		t.Fatalf("role title mismatch: %s", guildSvc.lastTitle)
	}

	// 6. PUT /guilds/g1/customization
	req = newGuildJSONRequest(http.MethodPut, "/guilds/g1/customization", map[string]string{
		"character_id": charID,
		"color":        "#112233",
		"mark":         "1",
		"wallpaper":    "kabe.gif",
		"notice":       "よろしく！",
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("PUT customization returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if guildSvc.lastCustomCol != "#112233" || guildSvc.lastCustomMrk != "1" || guildSvc.lastCustomWlp != "kabe.gif" || guildSvc.lastNotice != "よろしく！" {
		t.Fatalf("customization mismatch: col=%s mrk=%s wlp=%s not=%s", guildSvc.lastCustomCol, guildSvc.lastCustomMrk, guildSvc.lastCustomWlp, guildSvc.lastNotice)
	}

	// 7. POST /guilds/g1/leave
	req = newGuildJSONRequest(http.MethodPost, "/guilds/g1/leave", map[string]string{
		"character_id": charID,
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("POST leave returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if !guildSvc.left {
		t.Fatalf("leave not called")
	}

	// 8. DELETE /guilds/g1/members/c2
	req = newGuildJSONRequest(http.MethodDelete, "/guilds/g1/members/c2", map[string]string{
		"character_id": charID,
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE member returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if !guildSvc.kicked {
		t.Fatalf("kick not called")
	}

	// 9. DELETE /guilds/g1
	req = newGuildJSONRequest(http.MethodDelete, "/guilds/g1", map[string]string{
		"character_id": charID,
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("DELETE guild returned %d, want 200: %s", w.Code, w.Body.String())
	}
	if !guildSvc.disbanded {
		t.Fatalf("disband not called")
	}
}

func TestGuild_ErrorMappings(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		wantStatus int
	}{
		{"not found", guild.ErrGuildNotFound, http.StatusNotFound},
		{"unauthorized", guild.ErrUnauthorized, http.StatusForbidden},
		{"insufficient funds", guild.ErrInsufficientFunds, http.StatusBadRequest},
		{"invalid name", guild.ErrInvalidGuildName, http.StatusBadRequest},
		{"name taken", guild.ErrGuildNameTaken, http.StatusBadRequest},
		{"invalid color", guild.ErrInvalidColorFormat, http.StatusBadRequest},
		{"color taken", guild.ErrColorTaken, http.StatusBadRequest},
		{"invalid mark", guild.ErrInvalidMark, http.StatusBadRequest},
		{"invalid wallpaper", guild.ErrInvalidWallpaper, http.StatusBadRequest},
		{"title too long", guild.ErrRoleTitleTooLong, http.StatusBadRequest},
		{"reserved title", guild.ErrReservedRoleTitle, http.StatusBadRequest},
		{"empty callout", guild.ErrEmptyCalloutMessage, http.StatusBadRequest},
		{"notice too long", guild.ErrNoticeTooLong, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			guildSvc := &stubGuildService{err: tt.err}
			router, sessionID, charID := setupGuildTest(guildSvc)

			req := newGuildJSONRequest(http.MethodPost, "/guilds", map[string]string{
				"character_id": charID,
				"name":         "FailGuild",
			}, sessionID)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("error %v: got status %d, want %d", tt.err, w.Code, tt.wantStatus)
			}
		})
	}
}

func TestGuild_NotConfigured(t *testing.T) {
	router, sessionID, charID := setupGuildTest(nil)

	// GET /guilds
	req := httptest.NewRequest(http.MethodGet, "/guilds", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("GET /guilds without service returned %d, want 501", w.Code)
	}

	// POST /guilds
	req = newGuildJSONRequest(http.MethodPost, "/guilds", map[string]string{
		"character_id": charID,
		"name":         "Knights",
	}, sessionID)
	w = httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotImplemented {
		t.Fatalf("POST /guilds without service returned %d, want 501", w.Code)
	}
}

func TestGuild_Unauthorized(t *testing.T) {
	guildSvc := &stubGuildService{}
	router, _, charID := setupGuildTest(guildSvc)

	// Without auth header
	req := newGuildJSONRequest(http.MethodPost, "/guilds", map[string]string{
		"character_id": charID,
		"name":         "Knights",
	}, "")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("POST /guilds without token returned %d, want 401", w.Code)
	}
}
