package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type medalHelperFixture struct {
	*gatewayFixture
	questsErr                error
	quests                   []helperquest.Quest
	completion               helperquest.CompletionResult
	completedID, claimedItem string
}

func (f *medalHelperFixture) GetRewards() []medal.Reward {
	return []medal.Reward{{Cost: 3, ItemID: "armor-32"}, {Cost: 5, ItemID: "item-013"}}
}

func (f *medalHelperFixture) Claim(ctx context.Context, actor, itemID string) (corecharacter.Character, depot.Depot, error) {
	f.claimedItem = itemID
	_, err := f.execute(ctx, actor, 0)
	return corecharacter.Character{ID: actor, Name: "勇者", SmallMedals: 2}, depot.Depot{}, err
}

func (f *medalHelperFixture) ListQuests(context.Context, time.Time) ([]helperquest.Quest, error) {
	return f.quests, f.questsErr
}

func (f *medalHelperFixture) CompleteQuest(ctx context.Context, actor, questID string, _ time.Time) (helperquest.CompletionResult, error) {
	f.completedID = questID
	_, err := f.execute(ctx, actor, 0)
	return f.completion, err
}

func newMedalHelperFixture(t *testing.T) *medalHelperFixture {
	f := &medalHelperFixture{gatewayFixture: newGatewayFixture(t),
		quests: []helperquest.Quest{{ID: "q1", Title: "趣味でその7", Kind: helperquest.KindWeapon, TargetName: "こんぼう", RequiredCount: 2, RewardItemID: "item-128"}}}
	f.char.Name, f.char.SmallMedals = "勇者", 5
	return f
}

func (f *medalHelperFixture) router(t *testing.T, destination string) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "medal", Parent: "town"}, playercontext.SceneDefinition{ID: "helper", Parent: "town"}))
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{},
		WithMedal(f), WithHelper(f), WithPlayerContext(pc), WithPlaceLog(&fakePlaceLog{}), func(h *Handler) { h.homes = f.gatewayFixture })
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func TestMedalAndHelperScenesObserveTheirLists(t *testing.T) {
	f := newMedalHelperFixture(t)
	store, router := f.router(t, "medal")
	status, observation := navigationGET(t, router)
	data := decodeShopScene[MedalSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "メダル伯爵の城" || data.SmallMedals != 5 || len(data.Rewards) != 2 ||
		data.Rewards[0] != (MedalReward{Cost: 3, ItemID: "armor-32", ItemName: "戦士のパジャマ"}) {
		t.Fatalf("medal: %d %+v", status, data)
	}
	for _, action := range []string{"medal_claim", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("medal missing %s", action)
		}
	}
	_, router = f.router(t, "helper")
	status, observation = navigationGET(t, router)
	helper := decodeShopScene[HelperSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "手助けクエスト" || len(helper.Quests) != 1 || helper.Quests[0].RewardName != "応用錬金レシピ" {
		t.Fatalf("helper: %d %+v", status, helper)
	}
	for _, action := range []string{"helper_complete", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("helper missing %s", action)
		}
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET mutated: writes %d executions %d", store.writes, f.executions)
	}
}

func TestHelperSceneFailsOnRequiredRead(t *testing.T) {
	f := newMedalHelperFixture(t)
	f.questsErr = errors.New("private")
	_, router := f.router(t, "helper")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d %+v", status, observation)
	}
}

func TestMedalNPCSaysTheHeldMedals(t *testing.T) {
	f := newMedalHelperFixture(t)
	_, router := f.router(t, "medal")
	for range 40 {
		_, raw := gatewayRequest(t, router, "hero", "session", "application/json", `{"action":"npc_talk","params":{}}`, nil)
		r := npcResult(t, raw)
		if r.NPCName != "@メダル伯爵" || strings.ContainsAny(r.Dialogue, "{}") {
			t.Fatalf("npc_talk %+v", r)
		}
		if strings.Contains(r.Dialogue, "あずかって") && r.Dialogue != "勇者からは小さなメダルを5枚あずかっておるぞ" {
			t.Fatalf("medal count line %q", r.Dialogue)
		}
	}
}

func TestMedalCommands(t *testing.T) {
	f := newMedalHelperFixture(t)
	_, router := f.router(t, "medal")
	status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("medal_claim", `{"item_id":"armor-32"}`), nil)
	got := tavernResult[MedalClaimResult](t, raw)
	if status != 200 || f.claimedItem != "armor-32" || got.Cost != 3 || got.SmallMedals != 2 ||
		got.Message != "メダル3枚の賞品と交換するのじゃな！戦士のパジャマは勇者の預かり所に送っておいたぞ！" {
		t.Fatalf("medal_claim: %d %+v", status, got)
	}
	for _, body := range []string{saleGatewayBody("medal_claim", `{}`), saleGatewayBody("medal_claim", `{"item_id":"armor-32","x":1}`), saleGatewayBody("helper_complete", `{"quest_id":1}`)} {
		before := f.executions
		if status, raw := gatewayRequest(t, router, "hero", "session", "application/json", body, nil); status != 400 || f.executions != before {
			t.Fatalf("%s executed: %d %s", body, status, raw)
		}
	}
}

func TestHelperCompleteReplies(t *testing.T) {
	done := helperquest.Quest{ID: "q1", Title: "趣味でその7", Kind: helperquest.KindWeapon, TargetName: "こんぼう", RequiredCount: 2, RewardItemID: "item-128"}
	for _, tc := range []struct {
		name, params string
		completion   helperquest.CompletionResult
		want         string
	}{
		{"resolve", `{"quest_id":"q1"}`, helperquest.CompletionResult{Character: &corecharacter.Character{Name: "勇者"}, CompletedQuest: &done, Renewed: []helperquest.Quest{{Title: "かわいいのでその3"}}},
			"こんぼう 2 本 たしかに受け取りました。こちらが報酬の 応用錬金レシピ になります！勇者さんの預り所に送っておきますね！\n『かわいいのでその3』は誰も解決できそうにないので、新しい依頼がきました！"},
		{"renew only", `{}`, helperquest.CompletionResult{}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newMedalHelperFixture(t)
			f.completion = tc.completion
			_, router := f.router(t, "helper")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("helper_complete", tc.params), nil)
			got := tavernResult[HelperCompleteResult](t, raw)
			if status != 200 || got.Message != tc.want || got.Renewed == nil || (tc.completion.CompletedQuest == nil) != (got.CompletedQuest == nil) {
				t.Fatalf("%d %+v", status, got)
			}
		})
	}
}

func TestMedalAndHelperCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		scene, action, params string
		err                   error
		code, text            string
		status                int
	}{
		{"medal", "medal_claim", `{"item_id":"item-013"}`, medal.ErrInsufficientMedals, "MEDAL_INSUFFICIENT", "小さなメダル5枚の賞品と交換するにはメダルが足りないぞ", 409},
		{"medal", "medal_claim", `{"item_id":"x"}`, medal.ErrRewardNotFound, "MEDAL_REWARD_NOT_FOUND", "", 400},
		{"helper", "helper_complete", `{"quest_id":"q1"}`, helperquest.ErrInsufficientItems, "HELPER_CONDITION_UNMET", "こんぼう 2 本 の条件を満たしてないようです", 409},
		{"helper", "helper_complete", `{"quest_id":"q1"}`, helperquest.ErrGuildRequired, "HELPER_GUILD_REQUIRED", "趣味でその7はギルド専用のクエストよ。", 409},
		{"helper", "helper_complete", `{"quest_id":"gone"}`, helperquest.ErrQuestNotFound, "HELPER_QUEST_NOT_FOUND", "", 400},
		{"helper", "helper_complete", `{"quest_id":"q1"}`, helperquest.ErrQuestAlreadyDone, "HELPER_QUEST_COMPLETED", "", 409},
		{"helper", "helper_complete", `{"quest_id":"q1"}`, errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			f := newMedalHelperFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, tc.scene)
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(string(got["error"]), tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}
