package http

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/playercontext"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

type casinoSceneAccounts struct {
	casino.Repository
	err error
}

type casinoActivityProjection struct {
	CasinoService
	view *casino.RoomView
}

func (r *casinoActivityProjection) GetRoomView(context.Context, string, string, string) (*casino.RoomView, error) {
	return r.view, nil
}
func (r *casinoActivityProjection) GetAccount(context.Context, string) (casino.Account, error) {
	return casino.Account{}, nil
}
func (r *casinoSceneAccounts) GetAccount(context.Context, string) (casino.Account, error) {
	return casino.Account{CharacterID: "hero", Coins: 1000}, r.err
}

type casinoSceneCharacters struct {
	casino.CharacterRepository
	f   *gatewayFixture
	err error
}

func (r *casinoSceneCharacters) FindByID(ctx context.Context, id string) (corecharacter.Character, error) {
	r.f.checkContext(ctx)
	if r.err != nil {
		return corecharacter.Character{}, r.err
	}
	if id == "hero" {
		return r.f.char, nil
	}
	return corecharacter.Character{ID: id, PlayerID: "peer-owner", Name: id}, nil
}

func (r *casinoSceneCharacters) FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(ctx, id)
}
func (r *casinoSceneCharacters) Update(_ context.Context, c corecharacter.Character) error {
	r.f.char = c
	return nil
}

type casinoSceneReads struct {
	CasinoService
	f       *gatewayFixture
	err     error
	viewErr error
}

func (r *casinoSceneReads) ListRooms(ctx context.Context) ([]casino.RoomSummary, error) {
	r.f.checkContext(ctx)
	if r.err != nil {
		return nil, r.err
	}
	return r.CasinoService.ListRooms(ctx)
}
func (r *casinoSceneReads) GetRoomView(ctx context.Context, room, owner, actor string) (*casino.RoomView, error) {
	r.f.checkContext(ctx)
	if owner != "owner" || actor != "hero" {
		r.f.t.Fatal("lost authenticated viewer")
	}
	if r.viewErr != nil {
		return nil, r.viewErr
	}
	return r.CasinoService.GetRoomView(ctx, room, owner, actor)
}

type casinoSceneFixture struct {
	*gatewayFixture
	rooms      *casino.MemoryRoomRepository
	accounts   *casinoSceneAccounts
	characters *casinoSceneCharacters
	service    *casino.Service
	reads      *casinoSceneReads
	store      *gatewayNavigationStore
	facts      []playercontext.Activity
}

func newCasinoSceneFixture(t *testing.T) *casinoSceneFixture {
	f := &casinoSceneFixture{gatewayFixture: newGatewayFixture(t), rooms: casino.NewMemoryRoomRepository(), accounts: &casinoSceneAccounts{}, store: &gatewayNavigationStore{selection: playercontext.Selection{Destination: "casino"}}}
	f.characters = &casinoSceneCharacters{f: f.gatewayFixture}
	var err error
	f.service, err = casino.NewService(f.accounts, casino.WithRoomRepository(f.rooms), casino.WithCharacterRepository(f.characters))
	if err != nil {
		t.Fatal(err)
	}
	f.reads = &casinoSceneReads{CasinoService: f.service, f: f.gatewayFixture}
	return f
}

func (f *casinoSceneFixture) router(options ...Option) http.Handler {
	scene := playercontext.SceneDefinition{ID: "casino", Parent: "town", Pageable: true, SubjectKind: "room", SubjectAvailable: func(ctx context.Context, _, id string) (bool, error) {
		rows, err := f.service.ListRooms(ctx)
		return slices.ContainsFunc(rows, func(r casino.RoomSummary) bool { return r.ID == id }), err
	}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(f.store, playercontext.SceneDefinition{ID: "town"}, scene), playercontext.WithActivities(f.readActivities))
	opts := []Option{WithPlayerContext(pc), WithCasino(f.reads)}
	opts = append(opts, options...)
	return f.gatewayFixture.router("rescue_request", timer.NewService(nil), opts...)
}

func (f *casinoSceneFixture) readActivities(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
	if f.facts != nil {
		return f.facts, nil
	}
	v, err := f.service.GetCharacterRoomView(ctx, owner, actor)
	if err != nil || v == nil {
		return []playercontext.Activity{}, err
	}
	c, err := v.Controls(actor)
	if err != nil {
		return nil, err
	}
	a := playercontext.Activity{Kind: "casino", ID: v.Room.ID, Role: c.Role, Phase: string(v.Room.Status), Round: v.Room.Round, Actions: []string{"casino_room_leave"}}
	if c.CanStart {
		a.Actions = append(a.Actions, "casino_room_start", "casino_room_kick")
	}
	if len(c.Actions) > 0 {
		a.Actions = append(a.Actions, "casino_room_action")
	}
	return []playercontext.Activity{a}, nil
}

func (f *casinoSceneFixture) room(id string, game casino.GameType, actor bool) {
	f.t.Helper()
	ctx := context.Background()
	leader := "peer-" + id
	if actor {
		leader = "hero"
	}
	hash := sha256.Sum256([]byte("secret"))
	r := casino.Room{ID: id, Name: id, GameType: game, Status: casino.RoomStatusWaiting, LeaderCharacterID: leader, Rate: 10, MaxPlayers: 8, Speed: casino.SpeedNormal, HasPassword: true, PasswordHash: hex.EncodeToString(hash[:]), AllowSpectators: true, UpdatedAt: time.Now()}
	if err := f.rooms.CreateRoom(ctx, r, casino.RoomMember{RoomID: id, CharacterID: leader, Card: 2}); err != nil {
		f.t.Fatal(err)
	}
	if actor {
		if err := f.rooms.AddMember(ctx, casino.RoomMember{RoomID: id, CharacterID: "peer", Card: 7, Action: "high"}); err != nil {
			f.t.Fatal(err)
		}
	}
}

func TestCasinoLobbySelectionPagingAndMenus(t *testing.T) {
	f := newCasinoSceneFixture(t)
	for i := range 23 {
		f.room(fmt.Sprintf("room-%02d", i), casino.GameTypeHighLow, false)
	}
	router := f.router()
	status, response := navigationGET(t, router)
	data := decodeShopScene[CasinoLobbySceneData](t, response)
	if status != 200 || response.Scene.Support.Observation != "details" || len(data.Rooms) != 20 || data.Page.Next == nil || data.WindowLimit != 100 || data.Menu.Coins != 1000 || data.Menu.GoldPerCoin != 20 || len(data.Menu.Prizes) != 18 || !reflect.DeepEqual(data.Menu.SlotBetRates, []int64{1, 10, 50, 100}) {
		t.Fatalf("lobby: %d %+v", status, data)
	}
	for i, row := range data.Rooms {
		if row.ID != fmt.Sprintf("room-%02d", i) || row.SelectParams != (playercontext.Subject{Kind: "room", ID: row.ID}) {
			t.Fatalf("row: %+v", row)
		}
	}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_page","params":{"destination":"casino","offset":20,"limit":20}}`, nil)
	if status != 200 {
		t.Fatalf("page: %d %s", status, raw)
	}
	_, response = navigationGET(t, router)
	data = decodeShopScene[CasinoLobbySceneData](t, response)
	if len(data.Rooms) != 3 || data.Page.Next != nil {
		t.Fatalf("last page: %+v", data)
	}
	status, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"room","target_id":"room-00"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" {
		t.Fatalf("select: %d %s", status, raw)
	}
	var refreshed PlayerContextResponse
	if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
		t.Fatal(err)
	}
	_, response = navigationGET(t, router)
	selected := decodeShopScene[CasinoSelectedSceneData](t, response)
	if !reflect.DeepEqual(refreshed.Scene, response.Scene) || response.Scene.Kind != "subject" || selected.Room.ID != "room-00" || selected.JoinParams.RoomID != "room-00" || selected.SpectateParams == nil || f.store.writes != 2 {
		t.Fatalf("selection: %+v", response)
	}
	serialized, err := json.Marshal(response.Scene.Data)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"members", "card", "pot", "leader_character_id", "peer-room-00"} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("selected summary leaked: %s", serialized)
		}
	}
	if _, err := f.service.GetRoomView(context.Background(), "room-00", "owner", "hero"); !errors.Is(err, casino.ErrRoomViewForbidden) {
		t.Fatalf("selection granted admission: %v", err)
	}
	// Another client overwrites only ordinary selection; reads do not renew it.
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_back","params":{}}`, nil); status != 200 {
		t.Fatal(status)
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_select","params":{"target_kind":"room","target_id":"room-01"}}`, nil); status != 200 {
		t.Fatal(status)
	}
	_, response = navigationGET(t, router)
	if response.Scene.Subject.ID != "room-01" || f.store.writes != 4 {
		t.Fatal("clients did not share last selection")
	}
	f.char.JobID = "job-46"
	f.store.selection = playercontext.Selection{Destination: "casino", Offset: 1000000, Limit: 100}
	_, response = navigationGET(t, router)
	data = decodeShopScene[CasinoLobbySceneData](t, response)
	if data.Rooms == nil || len(data.Rooms) != 0 || len(data.Menu.SlotBetRates) != 5 {
		t.Fatalf("empty/menu: %+v", data)
	}
}

func TestCasinoActualRoomMaskingRolesAndConflicts(t *testing.T) {
	for _, game := range []casino.GameType{casino.GameTypeIndian, casino.GameTypeHighLow, casino.GameTypeDoppel} {
		for _, role := range []string{"leader", "member", "spectator"} {
			t.Run(string(game)+"/"+role, func(t *testing.T) {
				f := newCasinoSceneFixture(t)
				f.room("active", game, true)
				ctx := context.Background()
				r, err := f.rooms.GetRoom(ctx, "active")
				if err != nil {
					t.Fatal(err)
				}
				r.Round, r.Status = 1, casino.RoomStatusInProgress
				if role == "member" {
					r.LeaderCharacterID = "peer"
				}
				if err := f.rooms.UpdateRoom(ctx, *r); err != nil {
					t.Fatal(err)
				}
				if role == "spectator" {
					m, err := f.rooms.GetMember(ctx, "active", "hero")
					if err != nil {
						t.Fatal(err)
					}
					m.IsSpectator = true
					if err := f.rooms.UpdateMember(ctx, *m); err != nil {
						t.Fatal(err)
					}
				}
				actions := []string{"casino_room_leave"}
				if role != "spectator" {
					actions = append(actions, "casino_room_action")
				}
				f.facts = []playercontext.Activity{{Kind: "casino", ID: "active", Role: role, Phase: string(r.Status), Round: 1, Actions: actions}}
				f.store.loadErr = errors.New("selection must not be read")
				router := f.router()
				status, response := navigationGET(t, router)
				data := decodeShopScene[ActivitySceneData](t, response)
				view, err := f.service.GetRoomView(ctx, "active", "owner", "hero")
				if err != nil {
					t.Fatal(err)
				}
				if status != 200 || data.Casino == nil || !reflect.DeepEqual(data.Casino.View, *view) || data.Casino.Controls.Role != role || f.store.writes != 0 {
					t.Fatalf("active: %d %+v", status, data)
				}
				if role == "spectator" && len(data.Casino.Controls.Actions) != 0 {
					t.Fatal("spectator can play")
				}
				status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"rescue_request","params":{"reason":"observe"}}`, nil)
				var refreshed PlayerContextResponse
				if err := json.Unmarshal(raw["context"], &refreshed); err != nil {
					t.Fatal(err)
				}
				if status != 200 || !reflect.DeepEqual(refreshed.Scene, response.Scene) {
					t.Fatalf("active GET/refresh differs: %d %s", status, raw)
				}
				for _, a := range response.Scene.Support.Actions {
					if a.Connected && strings.HasPrefix(a.Action, "casino_") {
						t.Fatalf("observation connected mutation: %+v", a)
					}
				}
				f.facts = append(f.facts, playercontext.Activity{Kind: "pvp", ID: "conflict", Role: "member", Phase: "recruiting", Actions: []string{}})
				status, response = navigationGET(t, router)
				data = decodeShopScene[ActivitySceneData](t, response)
				if status != 200 || response.Scene.Kind != "activity_conflict" || len(data.Activities) != 2 || data.Casino == nil || len(data.Casino.Controls.Actions) != 0 {
					t.Fatalf("conflict: %+v", response)
				}
			})
		}
	}
}

func TestCasinoObservationFailuresAndMissingSelection(t *testing.T) {
	f := newCasinoSceneFixture(t)
	f.room("room", casino.GameTypeIndian, false)
	router := f.router()
	for _, set := range []func(error){func(e error) { f.accounts.err = e }, func(e error) { f.reads.err = e }} {
		set(errors.New("required source failed"))
		if status, _ := navigationGET(t, router); status != 500 {
			t.Fatalf("partial observation: %d", status)
		}
		set(nil)
	}
	f.store.selection.Subject = playercontext.Subject{Kind: "room", ID: "missing"}
	if status, response := navigationGET(t, router); status != 200 || response.Scene.Kind != "selection_unavailable" || f.store.writes != 0 {
		t.Fatalf("missing: %d %+v", status, response)
	}
	for _, tc := range []struct {
		actor, token string
		status       int
	}{{"hero", "", 401}, {"other", "session", 403}, {"missing", "session", 404}} {
		r := httptest.NewRequest(http.MethodGet, "/api/v1/characters/"+tc.actor+"/context", nil)
		r.Header.Set("Authorization", "Bearer "+tc.token)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatalf("ownership: %d %s", w.Code, w.Body.String())
		}
	}
	f.store.selection = playercontext.Selection{Destination: "town"}
	f.store.afterSave = func() { f.accounts.err = errors.New("refresh failed") }
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"casino"}}`, nil)
	if status != 200 || string(raw["success"]) != "true" || string(raw["context"]) != "null" || f.store.writes != 1 {
		t.Fatalf("known selection lost: %d %s", status, raw)
	}
	f.accounts.err = nil
	if status, _ := navigationGET(t, router); status != 200 || f.store.writes != 1 {
		t.Fatal("GET recovery replayed selection")
	}
}

func TestCasinoGatewayPreservesAdmissionAndEntryGatesAcrossRenewal(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := vk.NewClientWithConfig(vk.ConfigFromEnvironment())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	ctx := context.Background()
	f := newCasinoSceneFixture(t)
	roomRepo, err := casino.NewValkeyRoomRepository(client, casino.WithValkeyCharacterRepository(f.characters))
	if err != nil {
		t.Fatal(err)
	}
	f.service, err = casino.NewService(f.accounts, casino.WithRoomRepository(roomRepo), casino.WithCharacterRepository(f.characters))
	if err != nil {
		t.Fatal(err)
	}
	f.reads = &casinoSceneReads{CasinoService: f.service, f: f.gatewayFixture}
	router := f.router()

	prefix := "http1091-" + id.New()[:8]
	roomID := prefix + "-room"
	peerID := prefix + "-peer"
	for _, actor := range []string{"hero", peerID} {
		_ = client.Do(ctx, client.B().Del().Key(casino.DefaultCharacterKeyPrefix+actor).Build())
	}
	t.Cleanup(func() {
		bg := context.Background()
		_ = client.Do(bg, client.B().Del().Key(casino.DefaultRoomKeyPrefix+roomID).Build())
		_ = client.Do(bg, client.B().Zrem().Key(casino.DefaultRoomsActiveIndexKey).Member(roomID).Build())
		for _, actor := range []string{"hero", peerID} {
			_ = client.Do(bg, client.B().Del().Key(casino.DefaultCharacterKeyPrefix+actor).Build())
		}
	})

	now := time.Now().UTC()
	r := casino.Room{
		ID: roomID, Name: "Gateway-" + prefix, GameType: casino.GameTypeIndian, Status: casino.RoomStatusWaiting,
		LeaderCharacterID: peerID, Rate: 10, MaxPlayers: 4, Speed: casino.SpeedFast, AllowSpectators: true, CreatedAt: now, UpdatedAt: now,
	}
	if err := roomRepo.CreateRoom(ctx, r, casino.RoomMember{RoomID: roomID, CharacterID: peerID, Action: "待機中", Card: 2, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := roomRepo.AddMember(ctx, casino.RoomMember{RoomID: roomID, CharacterID: "hero", Action: "待機中", Card: 5, JoinedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}

	// Shorten hero's mapping to 60s, then let peer's room activity renew the room.
	heroKey := casino.DefaultCharacterKeyPrefix + "hero"
	if err := client.Do(ctx, client.B().Expire().Key(heroKey).Seconds(60).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := roomRepo.UpdateMember(ctx, casino.RoomMember{RoomID: roomID, CharacterID: peerID, Action: "待機中", Card: 2, JoinedAt: now, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	ttl, err := client.Do(ctx, client.B().Ttl().Key(heroKey).Build()).AsInt64()
	if err != nil || ttl <= 60 {
		t.Fatalf("expected hero mapping TTL > 60 after peer activity, got %d err=%v", ttl, err)
	}
	// Advance past the original 60s admission deadline.
	if err := client.Do(ctx, client.B().Expire().Key(heroKey).Seconds(ttl-61).Build()).Error(); err != nil {
		t.Fatal(err)
	}

	status, response := navigationGET(t, router)
	data := decodeShopScene[ActivitySceneData](t, response)
	if status != 200 || response.Scene.LocationID != "casino" || response.Scene.Kind != "activity" || data.Casino == nil || data.Casino.View.Room.ID != roomID || len(data.Activities) != 1 {
		t.Fatalf("expected active casino scene across original admission deadline, got status=%d scene=%+v data=%+v", status, response.Scene, data)
	}
	for _, gated := range []string{"scene_enter", "casino_room_create", "casino_room_join", "casino_room_spectate"} {
		if slices.ContainsFunc(response.AvailableActions, func(a ContextAction) bool { return a.Action == gated }) {
			t.Fatalf("expected %s to be gated while admitted in live room: %+v", gated, response.AvailableActions)
		}
	}
	if gateStatus, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, nil); gateStatus != http.StatusConflict {
		t.Fatalf("expected ordinary scene_enter gate 409 across original admission deadline, got %d %s", gateStatus, raw)
	}

	// GET must not renew TTL beyond the remaining deadline.
	if err := client.Do(ctx, client.B().Expire().Key(heroKey).Seconds(45).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if status, _ := navigationGET(t, router); status != 200 {
		t.Fatalf("GET status=%d", status)
	}
	if afterTTL, err := client.Do(ctx, client.B().Ttl().Key(heroKey).Build()).AsInt64(); err != nil || afterTTL > 45 {
		t.Fatalf("GET renewed hero mapping TTL=%d err=%v", afterTTL, err)
	}

	// Leave/removal clears admission; saved room selection never grants or repairs membership.
	f.store.selection = playercontext.Selection{Destination: "casino", Subject: playercontext.Subject{Kind: "room", ID: roomID}}
	if err := roomRepo.RemoveMember(ctx, roomID, "hero"); err != nil {
		t.Fatal(err)
	}
	status, response = navigationGET(t, router)
	if status != 200 || response.Scene.Kind != "subject" || response.Scene.LocationID != "casino" {
		t.Fatalf("expected ordinary selected room summary after removal, got status=%d scene=%+v", status, response.Scene)
	}
	if !slices.ContainsFunc(response.Scene.Support.Actions, func(a SceneActionSupport) bool { return a.Action == "casino_room_join" }) {
		t.Fatalf("expected casino_room_join support candidate restored after removal: %+v", response.Scene.Support.Actions)
	}
	if gateStatus, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"scene_enter","params":{"destination":"town"}}`, nil); gateStatus != http.StatusOK {
		t.Fatalf("expected ordinary scene_enter gate lifted after removal, got %d %s", gateStatus, raw)
	}

	// Unreadable blank mapping must return 500, not false idle.
	if err := client.Do(ctx, client.B().Set().Key(heroKey).Value("").Ex(casino.DefaultLobbyTTL).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if status, _ := navigationGET(t, router); status != 500 {
		t.Fatalf("expected 500 on unreadable mapping, got %d", status)
	}
}
