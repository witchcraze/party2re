package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type farmFixture struct {
	*gatewayFixture
	summary    monster.MonsterBoxSummary
	summaryErr error
	calls      []string
}

func (f *farmFixture) GetSummary(context.Context, string) (monster.MonsterBoxSummary, error) {
	return f.summary, f.summaryErr
}

func (f *farmFixture) mutate(ctx context.Context, actor, call string, inst monster.MonsterInstance) (monster.MonsterInstance, error) {
	f.calls = append(f.calls, call)
	_, err := f.execute(ctx, actor, 0)
	return inst, err
}

func (f *farmFixture) BringToHome(ctx context.Context, actor, id string) (monster.MonsterInstance, error) {
	return f.mutate(ctx, actor, "home:"+id, monster.MonsterInstance{ID: id, CustomName: "スラりん", Location: monster.LocationHome})
}

func (f *farmFixture) DepositToBox(ctx context.Context, actor, id string) (monster.MonsterInstance, error) {
	return f.mutate(ctx, actor, "box:"+id, monster.MonsterInstance{ID: id, CustomName: "スラりん", Location: monster.LocationBox})
}

func (f *farmFixture) Rename(ctx context.Context, actor, id, name string) (monster.MonsterInstance, string, error) {
	inst, err := f.mutate(ctx, actor, "rename:"+id+":"+name, monster.MonsterInstance{ID: id, CustomName: name})
	return inst, "スラりん", err
}

func (f *farmFixture) SendMonster(ctx context.Context, actor, recipient, id string) (monster.MonsterInstance, error) {
	return f.mutate(ctx, actor, "send:"+id+":"+recipient, monster.MonsterInstance{ID: id, CharacterID: recipient, CustomName: "スラりん"})
}

func (f *farmFixture) ReleaseMonster(ctx context.Context, actor, id string) (monster.MonsterInstance, error) {
	return f.mutate(ctx, actor, "release:"+id, monster.MonsterInstance{ID: id, CustomName: "スラりん"})
}

func (f *farmFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "farm", Parent: "town"}))
	opts := append([]Option{WithMonster(f), WithPlayerContext(pc), WithCharacterNames(fakeCharacterNames{"Bob": "bob"}),
		func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newFarmFixture(t *testing.T) *farmFixture {
	f := &farmFixture{gatewayFixture: newGatewayFixture(t), summary: monster.MonsterBoxSummary{BoxCount: 1, BoxCapacity: 50, HomeCount: 1, HomeCapacity: 50,
		Monsters: []monster.MonsterInstance{{ID: "m1", CustomName: "スラりん", Location: monster.LocationBox}, {ID: "m2", CustomName: "ドラきち", Location: monster.LocationHome}}}}
	f.char.Name = "勇者"
	return f
}

func TestFarmSceneObservesBoxAndHome(t *testing.T) {
	f := newFarmFixture(t)
	store, router := f.router(t, "farm", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeSceneData[FarmSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "モンスターじいさん" || data.Parent != "town" || data.BoxCapacity != 50 || len(data.Monsters) != 2 {
		t.Fatalf("farm: %d %+v", status, data)
	}
	if len(observation.Scene.Members) == 0 || observation.Scene.Members[0].Name != "@モンジィ" {
		t.Fatalf("members %+v", observation.Scene.Members)
	}
	for _, action := range []string{"farm_bring_home", "farm_rename", "farm_deposit", "farm_send", "farm_release", "npc_talk", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s", action)
		}
	}
	f.summaryErr = errors.New("private box read")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d", status)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestFarmCommandReplies(t *testing.T) {
	for _, tc := range []struct{ action, params, call, want string }{
		{"farm_bring_home", `{"instance_id":"m1"}`, "home:m1", "スラりんを勇者の家に送っておいたぞ"},
		{"farm_rename", `{"instance_id":"m1","custom_name":"スラきち"}`, "rename:m1:スラきち", "スラりんをスラきちと名づけたぞ"},
		{"farm_deposit", `{"instance_id":"m2"}`, "box:m2", "スラりんを預かっておくぞ"},
		{"farm_send", `{"instance_id":"m1","recipient_name":"Bob"}`, "send:m1:bob", "スラりんをBobのモンスター預かり所に送っておいたぞ"},
		{"farm_release", `{"instance_id":"m1"}`, "release:m1", "スラりんを野生に帰しといたぞ"},
	} {
		t.Run(tc.action, func(t *testing.T) {
			f := newFarmFixture(t)
			_, router := f.router(t, "farm")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			if got := tavernResult[FarmResult](t, raw); status != 200 || got.Message != tc.want || len(f.calls) != 1 || f.calls[0] != tc.call {
				t.Fatalf("%s: %d %q %v", tc.action, status, got.Message, f.calls)
			}
		})
	}
}

func TestFarmCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		err            error
		code, text     string
		status         int
	}{
		{"farm_bring_home", `{"instance_id":"m1"}`, monster.ErrHomeFull, "FARM_HOME_FULL", "家に連れて行くことはできません", 409},
		{"farm_bring_home", `{"instance_id":"m1"}`, monster.ErrDuplicatePetNameAtHome, "FARM_HOME_NAME_TAKEN", "＠なづける", 409},
		{"farm_rename", `{"instance_id":"m1","custom_name":"とても長いなまえです"}`, monster.ErrNameTooLong, "FARM_NAME_TOO_LONG", "８文字まで", 400},
		{"farm_rename", `{"instance_id":"m2","custom_name":"x"}`, monster.ErrInvalidLocation, "FARM_WRONG_LOCATION", "", 409},
		{"farm_deposit", `{"instance_id":"m1"}`, monster.ErrInvalidLocation, "FARM_WRONG_LOCATION", "", 409},
		{"farm_deposit", `{"instance_id":"m2"}`, monster.ErrBoxFull, "FARM_BOX_FULL", "預かることはできんぞ", 409},
		{"farm_send", `{"instance_id":"m1","recipient_name":"Bob"}`, monster.ErrRecipientBoxFull, "FARM_RECIPIENT_FULL", "Bobのモンスター預かり所がいっぱいです", 409},
		{"farm_release", `{"instance_id":"x"}`, monster.ErrForbidden, "FARM_MONSTER_NOT_FOUND", "", 404},
		{"farm_release", `{"instance_id":"m1"}`, errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			f := newFarmFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, "farm")
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(string(got["error"]), tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestFarmSendRejectsUnknownRecipientBeforeMutation(t *testing.T) {
	f := newFarmFixture(t)
	_, router := f.router(t, "farm")
	status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("farm_send", `{"instance_id":"m1","recipient_name":"Nobody"}`), nil)
	assertGatewayError(t, got, "RECIPIENT_NOT_FOUND")
	if status != 404 || len(f.calls) != 0 || !strings.Contains(string(got["error"]), "Nobodyというプレイヤーは存在しません") {
		t.Fatalf("unknown recipient: %d %s %v", status, got, f.calls)
	}
}

func TestFarmCommandsRequireTheFarm(t *testing.T) {
	f := newFarmFixture(t)
	_, router := f.router(t, "bank")
	if status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("farm_release", `{"instance_id":"m1"}`), nil); status != 409 || len(f.calls) != 0 {
		t.Fatalf("release outside the farm: %d %s", status, got)
	}
}

func TestFarmWordsReadTheBoxCapacity(t *testing.T) {
	words, err := farmWords(nil, context.Background(), corecharacter.Character{JobLevel: 100, OverMonster: 1})
	if err != nil || words[4] != "自分の家には150匹までペットを連れて行くことができるぞい" || words[3] != "{name}の純粋な強さにモンスターはひきつけられるのじゃ" {
		t.Fatalf("words %v %v", words, err)
	}
}
