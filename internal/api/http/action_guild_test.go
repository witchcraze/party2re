package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// guildFixture is an in-memory guild: "g1" with the actor "hero" as master or
// member, a pending applicant and a gold medal decoration.
type guildFixture struct {
	*gatewayFixture
	g         guild.Guild
	role      guild.Role
	member    bool
	others    []guild.Guild
	readErr   error
	outcome   guild.AssignOutcome
	calls     []string
	presences []string
}

func (f *guildFixture) Get(_ context.Context, id string) (guild.Detail, error) {
	if f.readErr != nil {
		return guild.Detail{}, f.readErr
	}
	if id != f.g.ID {
		return guild.Detail{}, guild.ErrGuildNotFound
	}
	members := []guild.Member{{CharacterID: "boss", Role: guild.RoleLeader, Title: guild.DefaultTitleLeader}, {CharacterID: "applicant", Title: guild.DefaultTitlePending, IsPending: true}}
	if f.member {
		members = append(members, guild.Member{CharacterID: "hero", Role: f.role, Title: "隊長"})
	}
	return guild.Detail{Guild: f.g, Members: members, Decorations: []guild.Decoration{{Kind: guild.GoldMedal, Icon: "medal.svg"}}}, nil
}

func (f *guildFixture) GetByCharacter(_ context.Context, id string) (guild.Guild, guild.Member, error) {
	if f.readErr != nil {
		return guild.Guild{}, guild.Member{}, f.readErr
	}
	if id != "hero" || !f.member {
		return guild.Guild{}, guild.Member{}, guild.ErrCharacterNotInGuild
	}
	return f.g, guild.Member{GuildID: f.g.ID, CharacterID: id, Role: f.role, Title: "隊長"}, nil
}

func (f *guildFixture) List(context.Context, int, int) ([]guild.Guild, error) {
	return append([]guild.Guild{f.g}, f.others...), f.readErr
}

func (f *guildFixture) Listing(context.Context) (guild.Listing, error) {
	return guild.Listing{}, f.readErr
}

func (f *guildFixture) record(ctx context.Context, call string) error {
	f.calls = append(f.calls, call)
	_, err := f.execute(ctx, "hero", 0)
	return err
}

func (f *guildFixture) Create(ctx context.Context, actor, name string) (guild.Guild, guild.Member, corecharacter.Character, error) {
	if err := f.record(ctx, "create "+name); err != nil {
		return guild.Guild{}, guild.Member{}, corecharacter.Character{}, err
	}
	f.g, f.member, f.role = guild.Guild{ID: "g2", Name: name, LeaderCharacterID: actor}, true, guild.RoleLeader
	return f.g, guild.Member{}, f.char, nil
}

func (f *guildFixture) ApplyToJoin(ctx context.Context, id, actor string) error {
	return f.record(ctx, "apply "+id+" "+actor)
}

func (f *guildFixture) Assign(ctx context.Context, id, actor, target, title string) (guild.AssignOutcome, error) {
	return f.outcome, f.record(ctx, fmt.Sprintf("assign %s %s %s %s", id, actor, target, title))
}

func (f *guildFixture) BroadcastCallout(ctx context.Context, id, actor, message string) error {
	return f.record(ctx, "callout "+id+" "+message)
}

func (f *guildFixture) RecordRoomPresence(_ context.Context, id string) error {
	f.presences = append(f.presences, id)
	return nil
}

func (f *guildFixture) UpdateColor(ctx context.Context, id, actor, color string) error {
	return f.record(ctx, "color "+id+" "+color)
}

func (f *guildFixture) UpdateNotice(ctx context.Context, id, actor, notice string) error {
	return f.record(ctx, "notice "+id+" "+notice)
}

func (f *guildFixture) ChangeMark(ctx context.Context, id, actor, mark string) (corecharacter.Character, error) {
	return f.char, f.record(ctx, "mark "+id+" "+mark)
}

func (f *guildFixture) ChangeWallpaper(ctx context.Context, id, actor, wallpaper string) (corecharacter.Character, error) {
	return f.char, f.record(ctx, "wallpaper "+id+" "+wallpaper)
}

func (f *guildFixture) Disband(ctx context.Context, id, actor string) error {
	return f.record(ctx, "disband "+id)
}

func (f *guildFixture) Leave(ctx context.Context, id, actor string) error {
	return f.record(ctx, "leave "+id)
}

func (f *guildFixture) DecayGuildPoints(context.Context, float64) error { return nil }

func newGuildFixture(t *testing.T, destination string, member bool, role guild.Role) (*guildFixture, *gatewayNavigationStore, *fakePlaceMembers, http.Handler) {
	t.Helper()
	f := &guildFixture{gatewayFixture: newGatewayFixture(t), g: guild.Guild{ID: "g1", Name: "銀の盾", LeaderCharacterID: "boss", Color: "#33CCFF"}, member: member, role: role}
	if role == guild.RoleLeader {
		f.g.LeaderCharacterID = "hero"
	}
	f.char.Name = "勇者"
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	members := &fakePlaceMembers{}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true},
		playercontext.SceneDefinition{ID: "join_guild", Parent: "town", SubjectKind: "guild", Pageable: true, SubjectAvailable: func(ctx context.Context, _, target string) (bool, error) {
			_, err := f.Get(ctx, target)
			return err == nil, nil
		}},
		playercontext.SceneDefinition{ID: "guild", Parent: "town", CanEnter: func(ctx context.Context, c corecharacter.Character) (bool, error) {
			_, _, err := f.GetByCharacter(ctx, c.ID)
			if errors.Is(err, guild.ErrCharacterNotInGuild) {
				return false, nil
			}
			return err == nil, err
		}}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithGuild(f), WithPlayerContext(pc), WithPlaceLog(&fakePlaceLog{}), WithPlaceMembers(members), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return f, store, members, h.Router()
}

func assertOffers(t *testing.T, observation PlayerContextResponse, offers map[string]bool) {
	t.Helper()
	for action, want := range offers {
		if offered(observation, action) != want {
			t.Fatalf("%s offered=%v, want %v", action, !want, want)
		}
	}
}

func TestGuildAssociationOffersCommandsByAffiliation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		member bool
		role   guild.Role
		offers map[string]bool
	}{
		{"unaffiliated", false, "", map[string]bool{"guild_create": true, "scene_select": true, "npc_talk": true, "guild_mark": false, "guild_leave": false, "guild_disband": false}},
		{"member", true, guild.RoleMember, map[string]bool{"guild_create": false, "guild_leave": true, "guild_mark": false, "guild_wallpaper": false, "guild_disband": false}},
		{"master", true, guild.RoleLeader, map[string]bool{"guild_create": false, "guild_leave": true, "guild_mark": true, "guild_wallpaper": true, "guild_disband": true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f, store, _, router := newGuildFixture(t, "join_guild", tc.member, tc.role)
			status, observation := navigationGET(t, router)
			data := decodeShopScene[GuildAssociationSceneData](t, observation)
			if status != 200 || observation.Scene.Title != "ギルド" || len(data.Guilds) != 1 || len(data.Marks) != 66 || data.Wallpapers[0].Price != 0 ||
				(data.Affiliation != nil) != tc.member {
				t.Fatalf("scene: %d %+v", status, data)
			}
			assertOffers(t, observation, tc.offers)
			if store.writes != 0 || f.executions != 0 || len(f.presences) != 0 {
				t.Fatalf("GET mutated: %d %d %v", store.writes, f.executions, f.presences)
			}
		})
	}
}

func TestGuildAssociationSelectedGuildBindsApply(t *testing.T) {
	_, store, _, router := newGuildFixture(t, "join_guild", false, "")
	store.selection.Subject = playercontext.Subject{Kind: "guild", ID: "g1"}
	_, observation := navigationGET(t, router)
	data := decodeShopScene[GuildAssociationSceneData](t, observation)
	if data.Selected == nil || len(data.Selected.Members) != 2 || !data.Selected.Members[1].Pending || len(data.Selected.Decorations) != 1 {
		t.Fatalf("selected: %+v", data.Selected)
	}
	i := slices.IndexFunc(observation.AvailableActions, func(a ContextAction) bool { return a.Action == "guild_apply" })
	if i < 0 || observation.AvailableActions[i].ParamsTemplate["guild_id"] != "g1" {
		t.Fatalf("guild_apply not bound: %+v", observation.AvailableActions)
	}
	store.selection.Subject.ID = "gone"
	if _, observation := navigationGET(t, router); observation.Scene.Kind != "selection_unavailable" {
		t.Fatalf("missing guild: %+v", observation.Scene)
	}
}

func TestGuildRoomObservesRosterForMembersOnly(t *testing.T) {
	f, store, _, router := newGuildFixture(t, "guild", true, guild.RoleMember)
	status, observation := navigationGET(t, router)
	data := decodeShopScene[GuildRoomSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "銀の盾" || len(data.View.Members) != 3 || data.IsLeader || len(data.ColorSamples) != 0 {
		t.Fatalf("room: %d %+v", status, data)
	}
	if len(observation.Scene.Members) == 0 || observation.Scene.Members[0].Name != "金メダル" {
		t.Fatalf("decoration not listed: %+v", observation.Scene.Members)
	}
	assertOffers(t, observation, map[string]bool{"guild_callout": true, "npc_talk": true, "place_say": true, "guild_color": false, "guild_message": false, "guild_assign": false})
	if store.writes != 0 || f.executions != 0 || len(f.presences) != 0 {
		t.Fatal("GET mutated")
	}

	f.role, f.g.LeaderCharacterID = guild.RoleLeader, "hero"
	_, observation = navigationGET(t, router)
	if data := decodeShopScene[GuildRoomSceneData](t, observation); !data.IsLeader || len(data.ColorSamples) != 10 {
		t.Fatalf("leader room: %+v", data)
	}
	assertOffers(t, observation, map[string]bool{"guild_color": true, "guild_message": true, "guild_assign": true})

	// Expelled while inside: the room is unavailable and only navigation remains.
	f.member = false
	_, observation = navigationGET(t, router)
	if observation.Scene.Kind != "selection_unavailable" || offered(observation, "guild_callout") {
		t.Fatalf("expelled: %+v", observation.Scene)
	}
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("scene_back", `{}`), nil); status != 200 {
		t.Fatalf("expelled actor could not leave: %d %s", status, raw)
	}
}

func TestGuildRoomRejectsNonMembers(t *testing.T) {
	_, _, _, router := newGuildFixture(t, "town", false, "")
	_, observation := navigationGET(t, router)
	if data := decodeShopScene[TownSceneData](t, observation); slices.ContainsFunc(data.Destinations, func(d SceneDestination) bool { return d.ID == "guild" }) {
		t.Fatal("non-member offered the guild room")
	}
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("scene_enter", `{"destination":"guild"}`), nil); status/100 != 4 {
		t.Fatalf("non-member entered: %d", status)
	}
}

func TestGuildScenesFailOnRequiredRead(t *testing.T) {
	for _, destination := range []string{"join_guild", "guild"} {
		f, _, _, router := newGuildFixture(t, destination, true, guild.RoleMember)
		f.readErr = errors.New("private")
		if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
			t.Fatalf("%s partial observation: %d %+v", destination, status, observation)
		}
	}
}

func TestGuildRoomPresenceKeepsGuildActive(t *testing.T) {
	f, _, members, router := newGuildFixture(t, "guild", true, guild.RoleMember)
	if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("presence_refresh", `{}`), nil); status != 200 {
		t.Fatalf("%d %s", status, raw)
	}
	if !slices.Equal(f.presences, []string{"g1"}) || len(members.refreshed) != 1 || members.refreshed[0] != (placelog.Place{Kind: "guild", ID: "g1"}) {
		t.Fatalf("presence %v place %v", f.presences, members.refreshed)
	}
}

func TestGuildRoomTalkShowsMessageOrRoster(t *testing.T) {
	f, _, _, router := newGuildFixture(t, "guild", true, guild.RoleMember)
	f.g.Notice = "今夜集合"
	_, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
	if r := npcResult(t, raw); r.NPCName != "@銀の盾" || !r.Private || r.Dialogue != "今夜集合" {
		t.Fatalf("notice talk %+v", r)
	}
	f.g.Notice = ""
	_, raw = gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
	if r := npcResult(t, raw); r.Dialogue != "勇者＠ギルマス,勇者＠参加申請中,勇者＠隊長" {
		t.Fatalf("roster talk %+v", r)
	}
}

func TestGuildCommands(t *testing.T) {
	for _, tc := range []struct {
		destination, action, params string
		role                        guild.Role
		outcome                     guild.AssignOutcome
		call, message               string
	}{
		{"join_guild", "guild_mark", `{"mark":"12"}`, guild.RoleLeader, "", "mark g1 12", "銀の盾のギルドマークを 12 に変更しました"},
		{"join_guild", "guild_wallpaper", `{"wallpaper":"farm.gif"}`, guild.RoleLeader, "", "wallpaper g1 farm.gif", "銀の盾の壁紙を farm に変更しました"},
		{"join_guild", "guild_leave", `{}`, guild.RoleMember, "", "leave g1", "銀の盾から脱退しました"},
		{"join_guild", "guild_disband", `{}`, guild.RoleLeader, "", "disband g1", "銀の盾を解散させました"},
		{"guild", "guild_callout", `{"message":"集合"}`, guild.RoleMember, "", "callout g1 集合", "銀の盾のメンバー全員に手紙を送りました"},
		{"guild", "guild_color", `{"color":"#ff3333"}`, guild.RoleLeader, "", "color g1 #ff3333", "ギルドカラーを#FF3333に変更しました"},
		{"guild", "guild_message", `{"message":"ようこそ"}`, guild.RoleLeader, "", "notice g1 ようこそ", "ギルドメッセージを変更しました"},
		{"guild", "guild_assign", `{"member_id":"applicant","title":"見習い"}`, guild.RoleLeader, guild.AssignApproved, "assign g1 hero applicant 見習い", "勇者を銀の盾に参加することを許可しました"},
		{"guild", "guild_assign", `{"member_id":"applicant","title":"追放"}`, guild.RoleLeader, guild.AssignRejected, "assign g1 hero applicant 追放", "勇者の参加を拒否しました"},
		{"guild", "guild_assign", `{"member_id":"boss","title":"追放"}`, guild.RoleLeader, guild.AssignExpelled, "assign g1 hero boss 追放", "勇者を銀の盾から追放しました"},
		{"guild", "guild_assign", `{"member_id":"boss","title":"副長"}`, guild.RoleLeader, guild.AssignRetitled, "assign g1 hero boss 副長", ""},
	} {
		t.Run(tc.call, func(t *testing.T) {
			f, _, _, router := newGuildFixture(t, tc.destination, true, tc.role)
			f.outcome = tc.outcome
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			got := tavernResult[GuildResult](t, raw)
			if status != 200 || !slices.Equal(f.calls, []string{tc.call}) || got.Message != tc.message || got.Outcome != tc.outcome {
				t.Fatalf("%d %v %+v", status, f.calls, got)
			}
		})
	}
}

func TestGuildCreateMovesFounderIntoRoom(t *testing.T) {
	f, store, _, router := newGuildFixture(t, "join_guild", false, "")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("guild_create", `{"name":"新月"}`), nil)
	got := tavernResult[GuildResult](t, raw)
	if status != 200 || got.Message != "＠新ギルド 新月の設立を許可します！" || got.Selection == nil || got.Selection.Destination != "guild" || store.selection.Destination != "guild" || got.Character == nil {
		t.Fatalf("%d %+v %v", status, got, f.calls)
	}
}

func TestGuildApplyNamesMasterAndUsesSelectedGuild(t *testing.T) {
	f, store, _, router := newGuildFixture(t, "join_guild", false, "")
	store.selection.Subject = playercontext.Subject{Kind: "guild", ID: "g1"}
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("guild_apply", `{"guild_id":"g1"}`), nil)
	if got := tavernResult[GuildResult](t, raw); status != 200 || got.Message != "銀の盾のギルマス 勇者 に参加申請の手紙を送りました。ギルマスからの返事を待ちましょう" || !slices.Equal(f.calls, []string{"apply g1 hero"}) {
		t.Fatalf("%d %+v %v", status, got, f.calls)
	}
	// A guild other than the observed subject is rejected before execution.
	f.calls = nil
	if status, _ := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("guild_apply", `{"guild_id":"g9"}`), nil); status/100 != 4 || len(f.calls) != 0 {
		t.Fatalf("unbound guild accepted: %d %v", status, f.calls)
	}
}

func TestGuildCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		err    error
		code   string
		status int
	}{
		{guild.ErrUnauthorized, "GUILD_FORBIDDEN", 403},
		{guild.ErrCannotKickLeader, "GUILD_FORBIDDEN", 403},
		{guild.ErrTargetNotMember, "GUILD_MEMBER_NOT_FOUND", 409},
		{guild.ErrCharacterNotInGuild, "GUILD_NOT_MEMBER", 409},
		{guild.ErrReservedRoleTitle, "GUILD_INVALID_INPUT", 400},
		{guild.ErrInvalidNotice, "GUILD_INVALID_INPUT", 400},
		{errors.New("private storage failure"), "EXECUTION_FAILED", 500},
	} {
		t.Run(tc.code, func(t *testing.T) {
			f, _, _, router := newGuildFixture(t, "guild", true, guild.RoleLeader)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("guild_assign", `{"member_id":"boss","title":"副長"}`), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || (tc.status == 500 && strings.Contains(fmt.Sprint(got), "private")) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}
