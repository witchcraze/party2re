package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/runhost"
)

type partyLobbyStub struct {
	calls    []string
	err      error
	leaveRes party.LeaveResult
	parties  []party.PartySummary
}

func (s *partyLobbyStub) CreateParty(_ context.Context, leader string, req party.CreatePartyRequest) (party.PartyDetail, error) {
	s.calls = append(s.calls, fmt.Sprintf("create/%s/%s/%s/%d/%d/%s/%s", leader, req.Name, req.StageID, req.Speed, req.MaxMembers, req.NeedJoin, req.Password))
	return party.PartyDetail{Party: party.Party{ID: "new"}}, s.err
}

func (s *partyLobbyStub) JoinParty(_ context.Context, partyID, actor, password string) (party.PartyDetail, error) {
	s.calls = append(s.calls, "join/"+partyID+"/"+actor+"/"+password)
	return party.PartyDetail{Party: party.Party{ID: partyID}}, s.err
}

func (s *partyLobbyStub) GetPartySummary(_ context.Context, id string) (party.PartySummary, error) {
	for _, p := range s.parties {
		if p.ID == id {
			return p, nil
		}
	}
	return party.PartySummary{}, party.ErrNotFound
}

func (s *partyLobbyStub) ListParties(_ context.Context, status string, limit, offset int) (pagination.Page[party.PartySummary], error) {
	if status != party.StatusRecruiting {
		return pagination.Page[party.PartySummary]{}, errors.New("quest scene must list recruiting lobbies")
	}
	end := min(offset+limit, len(s.parties))
	return pagination.NewPage(s.parties[min(offset, end):end], len(s.parties), limit, offset), nil
}

func (s *partyLobbyStub) LeaveParty(_ context.Context, partyID, actor string) (party.LeaveResult, error) {
	s.calls = append(s.calls, "leave/"+partyID+"/"+actor)
	return s.leaveRes, s.err
}

func (s *partyLobbyStub) KickMember(_ context.Context, partyID, leader, target string) (party.KickResult, error) {
	s.calls = append(s.calls, "kick/"+partyID+"/"+leader+"/"+target)
	return party.KickResult{PartyName: "PartyOne", LeaderName: "Hero"}, s.err
}

func TestPartyLobbyGateway(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		facts            []string
		err              error
		status           int
	}{
		{"leave", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, nil, 200},
		{"kick", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_leave", "party_kick"}, nil, 200},
		{"kick not offered", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "", []string{"party_leave"}, nil, 409},
		{"missing party", `{"action":"party_leave","params":{}}`, "", []string{"party_leave"}, nil, 400},
		{"unknown param", `{"action":"party_leave","params":{"party_id":"p1","ready":true}}`, "", []string{"party_leave"}, nil, 400},
		{"lobby gone", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, party.ErrNotFound, 404},
		{"not member", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, party.ErrCharacterNotInParty, 422},
		{"not leader", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_kick"}, party.ErrNotPartyLeader, 403},
		{"kick self", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"hero"}}`, "kick/p1/hero/hero", []string{"party_kick"}, party.ErrCannotKickSelf, 422},
		{"started", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_kick"}, runhost.ErrBusy, 409},
		{"store down", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, errors.New("down"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			stub := &partyLobbyStub{err: tc.err}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{{Kind: "party", ID: "p1", Phase: party.StatusRecruiting, Role: "leader", Actions: tc.facts}}, nil
			}))
			router := f.router("rescue_request", timer.NewService(nil), WithParty(stub), WithPlayerContext(pc))
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
			if status != tc.status {
				t.Fatalf("status %d %s", status, raw)
			}
			if got := len(stub.calls); (tc.want == "" && got != 0) || (tc.want != "" && (got != 1 || stub.calls[0] != tc.want)) {
				t.Fatalf("calls %v, want %q", stub.calls, tc.want)
			}
		})
	}
}

type multiPartyGatewayFixture struct {
	CharacterService
	PlayerService
	HomeService
	chars map[string]corecharacter.Character
}

func (f *multiPartyGatewayFixture) Authenticate(_ context.Context, token string) (player.Player, error) {
	if token == "" {
		return player.Player{}, errors.New("unauthorized")
	}
	return player.Player{ID: token}, nil
}

func (f *multiPartyGatewayFixture) Get(_ context.Context, id string) (corecharacter.Character, error) {
	c, ok := f.chars[id]
	if !ok {
		return corecharacter.Character{}, corecharacter.ErrNotFound
	}
	return c, nil
}

func (f *multiPartyGatewayFixture) GetProfile(ctx context.Context, id string) (character.ProfileView, error) {
	c, err := f.Get(ctx, id)
	if err != nil {
		return character.ProfileView{}, err
	}
	return character.ProfileView{Character: c}, nil
}

func (f *multiPartyGatewayFixture) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	return f.Get(ctx, id)
}

func (f *multiPartyGatewayFixture) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return f.Get(ctx, id)
}

func (f *multiPartyGatewayFixture) Update(_ context.Context, c corecharacter.Character) error {
	f.chars[c.ID] = c
	return nil
}

func (f *multiPartyGatewayFixture) FindPendingByActorID(context.Context, string) ([]scheduling.ScheduledAction, error) {
	return nil, nil
}

func (f *multiPartyGatewayFixture) GetSleepStatus(context.Context, string) (home.SleepStatus, error) {
	return home.SleepStatus{}, nil
}

type partyTestStageCatalog struct{}

func (partyTestStageCatalog) FindByID(id string) (adventure.Stage, error) {
	return adventure.Stage{ID: id, Name: "はじまりの森"}, nil
}
func (partyTestStageCatalog) CanAccessStage(corecharacter.Character, string) error { return nil }
func (partyTestStageCatalog) AvailableStages(corecharacter.Character) []adventure.Stage {
	return []adventure.Stage{{ID: "stage-00", Name: "はじまりの森"}}
}

type partyTestMonsterCatalog struct{}

func (partyTestMonsterCatalog) FindByID(string) (adventure.Monster, error) {
	return adventure.Monster{}, adventure.ErrMonsterNotFound
}

func TestPartyKickSetsActionWaitAndQuestPlaceLog(t *testing.T) {
	ctx := context.Background()
	f := &multiPartyGatewayFixture{
		chars: map[string]corecharacter.Character{
			"hero":  {ID: "hero", PlayerID: "p-hero", Name: "Hero", Money: 100, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
			"ally":  {ID: "ally", PlayerID: "p-ally", Name: "Ally", Money: 100, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
			"other": {ID: "other", PlayerID: "p-other", Name: "Other", Money: 100, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}},
		},
	}
	repo := party.NewMemoryRepository()
	partySvc, err := party.NewService(repo, f, nil, partyTestStageCatalog{}, partyTestMonsterCatalog{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	detail, err := partySvc.CreateParty(ctx, "hero", party.CreatePartyRequest{Name: "冒険の旅", StageID: "stage-00", Speed: 3, MaxMembers: 4})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := partySvc.JoinParty(ctx, detail.Party.ID, "ally", ""); err != nil {
		t.Fatal(err)
	}

	waits := timer.NewService(nil)
	nav := &gatewayNavigationStore{selection: playercontext.Selection{Destination: "quest"}}
	pc := playercontext.NewService(f, f, waits,
		playercontext.WithActionWait(waits, playercontext.DefaultActionWait),
		playercontext.WithNavigation(nav,
			playercontext.SceneDefinition{ID: "town", Pageable: true},
			playercontext.SceneDefinition{ID: "quest", Parent: "town", SubjectKind: "party", Pageable: true,
				SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
					_, err := partySvc.GetPartySummary(ctx, target)
					if errors.Is(err, party.ErrNotFound) {
						return false, nil
					}
					return err == nil, err
				}},
		),
		playercontext.WithActivities(func(ctx context.Context, _, actor string) ([]playercontext.Activity, error) {
			p, m, err := partySvc.GetActiveParty(ctx, actor)
			if errors.Is(err, party.ErrNotFound) {
				return nil, nil
			}
			if err != nil {
				return nil, err
			}
			actions := []string{"party_leave"}
			if m.IsLeader && p.LeaderCharacterID == actor && p.Status == party.StatusRecruiting {
				actions = append(actions, "party_start", "party_kick")
			}
			return []playercontext.Activity{{Kind: "party", ID: p.ID, Phase: p.Status, Role: "leader", Actions: actions}}, nil
		}),
	)
	partySvc.SetActionWait(pc)
	placeLog := &fakePlaceLog{byPlace: map[placelog.Place][]placelog.Entry{}}
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, navigationShopService{},
		WithParty(partySvc), WithQuestStages(partyTestStageCatalog{}), WithPlayerContext(pc), WithPlaceLog(placeLog))
	if err != nil {
		t.Fatal(err)
	}
	router := h.Router()

	// Rejected kick imposes no wait and writes no log row.
	if status, _ := gatewayRequest(t, router, "hero", "p-hero", "application/json",
		`{"action":"party_kick","params":{"party_id":"`+detail.Party.ID+`","target_character_id":"hero"}}`, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 self-kick, got %d", status)
	}
	if rem, _ := waits.GetRemainingLock(ctx, timer.CategoryActionWait, "hero"); rem != 0 {
		t.Fatalf("rejected kick set wait %v", rem)
	}
	if len(placeLog.byPlace) != 0 {
		t.Fatalf("rejected kick wrote log %+v", placeLog.byPlace)
	}

	// Leader kicks ally: ally gets 60s wait, is removed from lobby, and receives the private quest place-log notice.
	status, raw := gatewayRequest(t, router, "hero", "p-hero", "application/json",
		`{"action":"party_kick","params":{"party_id":"`+detail.Party.ID+`","target_character_id":"ally"}}`, nil)
	if status != http.StatusOK {
		t.Fatalf("kick failed: %d %s", status, raw)
	}
	if _, _, err := partySvc.GetActiveParty(ctx, "ally"); !errors.Is(err, party.ErrNotFound) {
		t.Fatalf("kicked ally still active: %v", err)
	}
	if rem, _ := waits.GetRemainingLock(ctx, timer.CategoryActionWait, "ally"); rem <= 55*time.Second || rem > 60*time.Second {
		t.Fatalf("kicked ally wait = %v, want ~60s", rem)
	}

	// Kicked actor's context shows action_wait_ms and subsequent Gateway commands fail with 409 ACTION_WAIT.
	getCtx := func(actor, token string) PlayerContextResponse {
		t.Helper()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+actor+"/context", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("GET context(%s): %d %s", actor, rec.Code, rec.Body.String())
		}
		var out PlayerContextResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	allyCtx := getCtx("ally", "p-ally")
	if allyCtx.Character.ActionWaitMS <= 50000 || allyCtx.Scene.Kind == "activity" {
		t.Fatalf("kicked ally context = wait:%d scene:%+v", allyCtx.Character.ActionWaitMS, allyCtx.Scene)
	}
	if len(allyCtx.Scene.PlaceLog) != 1 || !allyCtx.Scene.PlaceLog[0].Private ||
		allyCtx.Scene.PlaceLog[0].SpeakerKind != string(placelog.SpeakerNPC) ||
		allyCtx.Scene.PlaceLog[0].Name != "＠" ||
		allyCtx.Scene.PlaceLog[0].Color != npcColor ||
		allyCtx.Scene.PlaceLog[0].Text != "冒険の旅のリーダーHeroからきっくされました" {
		t.Fatalf("kicked ally quest place_log = %+v", allyCtx.Scene.PlaceLog)
	}
	if otherCtx := getCtx("other", "p-other"); len(otherCtx.Scene.PlaceLog) != 0 {
		t.Fatalf("kick notice leaked to other viewer: %+v", otherCtx.Scene.PlaceLog)
	}
	status, raw = gatewayRequest(t, router, "ally", "p-ally", "application/json",
		`{"action":"scene_enter","params":{"destination":"town"}}`, nil)
	if status != http.StatusConflict {
		t.Fatalf("expected 409 ACTION_WAIT for kicked ally, got %d %s", status, raw)
	}
	if actionErrorCode(t, raw) != "ACTION_WAIT" {
		t.Fatalf("expected ACTION_WAIT, got %s", raw["error"])
	}

	// Best-effort place log append failure does not fail the kick.
	if _, err := partySvc.JoinParty(ctx, detail.Party.ID, "other", ""); err != nil {
		t.Fatal(err)
	}
	placeLog.err = errors.New("valkey log write failed")
	status, raw = gatewayRequest(t, router, "hero", "p-hero", "application/json",
		`{"action":"party_kick","params":{"party_id":"`+detail.Party.ID+`","target_character_id":"other"}}`, nil)
	if status != http.StatusOK {
		t.Fatalf("kick should succeed even if place log append fails: %d %s", status, raw)
	}
}

func TestPartyLeaveDeliveryNotice(t *testing.T) {
	for _, tc := range []struct {
		name       string
		delivery   *party.DeliveryOutcome
		wantNotice string
	}{
		{"no delivery", nil, ""},
		{"paid delivery", &party.DeliveryOutcome{ItemName: "癒しの湧き水", Paid: true}, "酒場から癒しの湧き水が届きました！"},
		{"unpaid delivery", &party.DeliveryOutcome{ItemName: "完熟トマトジュース", Paid: false}, "酒場から完熟トマトジュースが届きましたが、お金が足りませんでした"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			stub := &partyLobbyStub{leaveRes: party.LeaveResult{Delivery: tc.delivery}}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{{Kind: "party", ID: "p1", Phase: party.StatusRecruiting, Role: "leader", Actions: []string{"party_leave"}}}, nil
			}))
			router := f.router("rescue_request", timer.NewService(nil), WithParty(stub), WithPlayerContext(pc))
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"party_leave","params":{"party_id":"p1"}}`, nil)
			if status != http.StatusOK {
				t.Fatalf("status %d %s", status, raw)
			}
			var result struct {
				Message        string `json:"message"`
				DeliveryNotice string `json:"delivery_notice"`
			}
			if err := json.Unmarshal(raw["result"], &result); err != nil {
				t.Fatal(err)
			}
			if result.Message != "パーティーから離れました" {
				t.Fatalf("message = %q", result.Message)
			}
			if result.DeliveryNotice != tc.wantNotice {
				t.Fatalf("delivery_notice = %q, want %q", result.DeliveryNotice, tc.wantNotice)
			}
			var resultObj map[string]any
			if err := json.Unmarshal(raw["result"], &resultObj); err != nil {
				t.Fatal(err)
			}
			if _, hasNotice := resultObj["delivery_notice"]; (tc.wantNotice != "") != hasNotice {
				t.Fatalf("delivery_notice presence mismatch: %v", resultObj)
			}
		})
	}
}
