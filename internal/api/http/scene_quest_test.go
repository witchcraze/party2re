package http

import (
	"context"
	"errors"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/runhost"
)

type questStageStub []adventure.Stage

func (s questStageStub) AvailableStages(corecharacter.Character) []adventure.Stage { return s }
func (s questStageStub) FindByID(id string) (adventure.Stage, error) {
	for _, st := range s {
		if st.ID == id {
			return st, nil
		}
	}
	return adventure.Stage{}, adventure.ErrStageNotFound
}

type questRunStub struct {
	rooms []runhost.QuestRoom
	err   error
}

func (s questRunStub) ListQuestRooms(context.Context) ([]runhost.QuestRoom, error) {
	return s.rooms, s.err
}

type questPvPStub struct {
	PvPService
	rooms []pvp.RoomSummary
}

func (s questPvPStub) ListRooms(context.Context) ([]pvp.RoomSummary, error) { return s.rooms, nil }

type questGvGStub struct {
	GvGService
	rooms []gvg.RoomSummary
}

func (s questGvGStub) ListRooms(context.Context) ([]gvg.RoomSummary, error) { return s.rooms, nil }

func questRouter(t *testing.T, f *gatewayFixture, store *gatewayNavigationStore, lobbies *partyLobbyStub, opts ...Option) (*Handler, playercontext.Selection) {
	t.Helper()
	service := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "quest", Parent: "town", SubjectKind: "party", Pageable: true,
			SubjectAvailable: func(_ context.Context, _, target string) (bool, error) {
				return slices.ContainsFunc(lobbies.parties, func(p party.PartySummary) bool { return p.ID == target }), nil
			}},
	))
	opts = append([]Option{WithNormalRuns(&runStub{t: t}), WithParty(lobbies), WithQuestStages(questStageStub{{ID: "stage-00", Name: "はじまりの森"}}), WithPlayerContext(service)}, opts...)
	h, err := NewHandler(f, f, &struct{ AdventureService }{}, navigationShopService{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return h, playercontext.Selection{Destination: "quest"}
}

func TestQuestSceneListsLobbiesAndCreatesOrJoins(t *testing.T) {
	f, store := newGatewayFixture(t), &gatewayNavigationStore{}
	lobbies := &partyLobbyStub{parties: []party.PartySummary{
		{ID: "open", Name: "Open", LeaderName: "Lead", MemberNames: []string{"Lead", "Ally"}, CurrentMembers: 2, MaxMembers: 4, StageID: "stage-00", StageName: "はじまりの森", Speed: 3, Status: party.StatusRecruiting},
		{ID: "full", Name: "Full", LeaderName: "Solo", MemberNames: []string{"Solo"}, CurrentMembers: 1, MaxMembers: 1, StageID: "stage-00", Speed: 18, Status: party.StatusRecruiting},
	}}
	h, quest := questRouter(t, f, store, lobbies)
	router := h.Router()

	// The town no longer offers quest creation; 冒険に出る does.
	_, town := navigationGET(t, router)
	if slices.Contains(actionIDs(town.AvailableActions), "adventure_start") || !slices.ContainsFunc(decodeSceneData[TownSceneData](t, town).Destinations, func(d SceneDestination) bool { return d.ID == "quest" }) {
		t.Fatalf("town: %v", actionIDs(town.AvailableActions))
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":3,"max_members":4}}`, nil); status != 409 || len(lobbies.calls) != 0 {
		t.Fatalf("town create: %d %s", status, raw)
	}

	store.selection = quest
	status, obs := navigationGET(t, router)
	data := decodeSceneData[QuestSceneData](t, obs)
	if status != 200 || obs.Scene.LocationID != "quest" || len(data.Stages) != 1 || data.Stages[0].ID != "stage-00" || len(data.Parties) != 2 ||
		!slices.Equal(data.Parties[0].MemberNames, []string{"Lead", "Ally"}) || data.Parties[0].SelectParams != (playercontext.Subject{Kind: "party", ID: "open"}) {
		t.Fatalf("quest scene: %d %+v", status, data)
	}
	if got := actionIDs(obs.AvailableActions); !slices.Contains(got, "adventure_start") || !slices.Contains(got, "party_create") || slices.Contains(got, "party_join") {
		t.Fatalf("quest choices: %v", got)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":3,"max_members":2,"need_join":"hp_100_o","password":"pw"}}`, nil); status != 200 || !slices.Equal(lobbies.calls, []string{"create/hero/Q/stage-00/3/2/hp_100_o/pw"}) {
		t.Fatalf("create: %d %s %v", status, raw, lobbies.calls)
	}

	// Selecting a lobby is not joining; party_join is bound to the selection.
	lobbies.calls = nil
	store.selection = playercontext.Selection{Destination: "quest", Subject: playercontext.Subject{Kind: "party", ID: "open"}}
	status, obs = navigationGET(t, router)
	selected := decodeSceneData[QuestSelectedSceneData](t, obs)
	join := slices.IndexFunc(obs.AvailableActions, func(a ContextAction) bool { return a.Action == "party_join" })
	if status != 200 || obs.Scene.Title != "Open" || selected.JoinParams.PartyID != "open" || join < 0 || obs.AvailableActions[join].ParamsTemplate["party_id"] != "open" || len(lobbies.calls) != 0 {
		t.Fatalf("selected: %d %+v", status, obs)
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"party_join","params":{"party_id":"full"}}`, nil); status == 200 || len(lobbies.calls) != 0 {
		t.Fatalf("unbound join accepted: %d", status)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"party_join","params":{"party_id":"open","password":"pw"}}`, nil); status != 200 || !slices.Equal(lobbies.calls, []string{"join/open/hero/pw"}) {
		t.Fatalf("join: %d %s %v", status, raw, lobbies.calls)
	}

	// A full lobby is listed but not joinable (quest.cgi:159–163).
	store.selection = playercontext.Selection{Destination: "quest", Subject: playercontext.Subject{Kind: "party", ID: "full"}}
	if _, obs = navigationGET(t, router); slices.Contains(actionIDs(obs.AvailableActions), "party_join") {
		t.Fatalf("full lobby offered join: %v", actionIDs(obs.AvailableActions))
	}
}

// Legacy lists every room under $questdir (quest.cgi:80–170). Entry to runs
// and PvP/GvG rooms is not a Gateway command, so the scene offers none.
func TestQuestSceneListsRunsAndPvPGvGRooms(t *testing.T) {
	f, store := newGatewayFixture(t), &gatewayNavigationStore{}
	lobbies := &partyLobbyStub{parties: []party.PartySummary{{ID: "open", Name: "Open", CurrentMembers: 1, MaxMembers: 4, Status: party.StatusRecruiting}}}
	run := runhost.QuestRoom{Name: "Run", Speed: 3, StageID: "stage-00", Round: 2, LeaderName: "Lead", MemberNames: []string{"Lead", "Ally"}, MaxMembers: 4}
	arena := pvp.RoomSummary{ID: "arena", Name: "Arena", LeaderName: "Duel", MemberNames: []string{"Duel"}, CurrentMembers: 1, MaxMembers: 8, Bet: 10, NeedJoin: "joblv_3_u", Status: pvp.StatusRecruiting}
	guildBattle := gvg.RoomSummary{ID: "gvg", Name: "Siege", LeaderName: "Guild", MemberNames: []string{"Guild"}, CurrentMembers: 1, MaxMembers: 8, Status: gvg.StatusRecruiting}
	h, quest := questRouter(t, f, store, lobbies, WithQuestRuns(questRunStub{rooms: []runhost.QuestRoom{run, {Name: "Gone", StageID: "removed"}}}),
		WithPvP(questPvPStub{rooms: []pvp.RoomSummary{arena}}), WithGvG(questGvGStub{rooms: []gvg.RoomSummary{guildBattle}}))
	store.selection = quest
	status, obs := navigationGET(t, h.Router())
	data := decodeSceneData[QuestSceneData](t, obs)
	wantRuns := []QuestRun{{QuestRoom: run, StageName: "はじまりの森"}, {QuestRoom: runhost.QuestRoom{Name: "Gone", StageID: "removed"}}}
	if status != 200 || !reflect.DeepEqual(data.Runs, wantRuns) || !reflect.DeepEqual(data.Arenas, []pvp.RoomSummary{arena}) || !reflect.DeepEqual(data.GuildBattles, []gvg.RoomSummary{guildBattle}) {
		t.Fatalf("rooms: %d %+v", status, data)
	}
	if got := actionIDs(obs.AvailableActions); slices.ContainsFunc(got, func(id string) bool {
		return strings.HasPrefix(id, "pvp_") || strings.HasPrefix(id, "gvg_") || id == "run_operation"
	}) {
		t.Fatalf("room entry offered: %v", got)
	}

	// Only Party lobbies page; later pages repeat no other room.
	store.selection = playercontext.Selection{Destination: "quest", Offset: 1, Limit: 20}
	if status, obs = navigationGET(t, h.Router()); status != 200 || len(decodeSceneData[QuestSceneData](t, obs).Runs) != 0 {
		t.Fatalf("second page repeated runs: %d %+v", status, obs)
	}

	// A failed room source fails the scene instead of omitting its rows.
	failing, quest := questRouter(t, f, store, lobbies, WithQuestRuns(questRunStub{err: errors.New("runs unavailable")}))
	store.selection = quest
	if status, _ := navigationGET(t, failing.Router()); status == 200 {
		t.Fatal("scene hid a failed run listing")
	}
}

func TestQuestCreateJoinRejections(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		err        error
		status     int
	}{
		{"missing max members", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":3}}`, nil, 400},
		{"min/max level gone", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":3,"max_members":4,"min_level":5}}`, nil, 400},
		{"bad speed", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":7,"max_members":4}}`, party.ErrInvalidSpeed, 400},
		{"stage gate", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-09","speed":3,"max_members":4}}`, adventure.ErrJobLevelRequirementNotMet, 422},
		{"already in party", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-00","speed":3,"max_members":4}}`, party.ErrAlreadyInParty, 409},
		{"daily once", `{"action":"party_create","params":{"name":"Q","stage_id":"stage-26","speed":3,"max_members":4}}`, party.ErrDailyOnceAlreadyUsed, 422},
		{"full", `{"action":"party_join","params":{"party_id":"open"}}`, party.ErrPartyFull, 409},
		{"password", `{"action":"party_join","params":{"party_id":"open","password":"x"}}`, party.ErrInvalidPassword, 403},
		{"condition", `{"action":"party_join","params":{"party_id":"open"}}`, party.ErrNeedJoinNotMet, 422},
		{"started", `{"action":"party_join","params":{"party_id":"open"}}`, party.ErrPartyNotRecruiting, 409},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, store := newGatewayFixture(t), &gatewayNavigationStore{}
			lobbies := &partyLobbyStub{err: tc.err, parties: []party.PartySummary{{ID: "open", Name: "Open", CurrentMembers: 1, MaxMembers: 4, Status: party.StatusRecruiting}}}
			h, quest := questRouter(t, f, store, lobbies)
			store.selection = quest
			if strings.Contains(tc.body, `"party_join"`) {
				store.selection.Subject = playercontext.Subject{Kind: "party", ID: "open"}
			}
			status, raw := gatewayRequest(t, h.Router(), "hero", "session", "application/json", tc.body, nil)
			if status != tc.status || (tc.err != nil) != (len(lobbies.calls) == 1) {
				t.Fatalf("status %d calls %v %s", status, lobbies.calls, raw)
			}
		})
	}
}
