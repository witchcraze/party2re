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
	"github.com/witchcraze/party2re/internal/plantation"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type plantationFixture struct {
	*gatewayFixture
	status    plantation.StatusResponse
	statusErr error
	harvest   plantation.HarvestResult
	inputs    []string
}

func (f *plantationFixture) GetStatus(context.Context, string) (plantation.StatusResponse, error) {
	return f.status, f.statusErr
}

func (f *plantationFixture) Sow(ctx context.Context, actor, seedID string) (plantation.SowResult, error) {
	f.inputs = append(f.inputs, seedID)
	_, err := f.execute(ctx, actor, 0)
	return plantation.SowResult{Plot: plantation.Plot{SeedID: seedID}, SeedName: "赤の種"}, err
}

func (f *plantationFixture) Fertilize(ctx context.Context, actor, fertilizerID string) (plantation.FertilizeResult, error) {
	f.inputs = append(f.inputs, fertilizerID)
	_, err := f.execute(ctx, actor, 0)
	return plantation.FertilizeResult{FertilizerName: "化学肥料"}, err
}

func (f *plantationFixture) Harvest(ctx context.Context, actor string) (plantation.HarvestResult, error) {
	_, err := f.execute(ctx, actor, 0)
	return f.harvest, err
}

func (f *plantationFixture) router(t *testing.T, destination string, options ...Option) (*gatewayNavigationStore, http.Handler) {
	t.Helper()
	store := &gatewayNavigationStore{selection: playercontext.Selection{Destination: destination}}
	pc := playercontext.NewService(f.gatewayFixture, f.gatewayFixture, timer.NewService(nil), playercontext.WithNavigation(store,
		playercontext.SceneDefinition{ID: "town", Pageable: true}, playercontext.SceneDefinition{ID: "bank", Parent: "town"},
		playercontext.SceneDefinition{ID: "plantation", Parent: "town"}))
	opts := append([]Option{WithPlantation(f), WithPlayerContext(pc), func(h *Handler) { h.homes = f.gatewayFixture }}, options...)
	h, err := NewHandler(f.gatewayFixture, f.gatewayFixture, &struct{ AdventureService }{}, &struct{ ShopService }{}, opts...)
	if err != nil {
		t.Fatal(err)
	}
	return store, h.Router()
}

func newPlantationFixture(t *testing.T) *plantationFixture {
	return &plantationFixture{gatewayFixture: newGatewayFixture(t), status: plantation.StatusResponse{Status: plantation.StatusNone,
		Seeds: plantation.AllSeeds(), Fertilizers: []plantation.Fertilizer{}}}
}

func TestPlantationSceneObservesThePlot(t *testing.T) {
	f := newPlantationFixture(t)
	store, router := f.router(t, "plantation", WithPlaceLog(&fakePlaceLog{}))
	status, observation := navigationGET(t, router)
	data := decodeShopScene[PlantationSceneData](t, observation)
	if status != 200 || observation.Scene.Title != "種菜園" || observation.Scene.Support.Observation != "details" ||
		data.Parent != "town" || data.Status != plantation.StatusNone || len(data.Seeds) != 6 {
		t.Fatalf("plantation: %d %+v", status, data)
	}
	if len(observation.Scene.Members) == 0 || observation.Scene.Members[0].Name != "@ロータス" {
		t.Fatalf("members %+v", observation.Scene.Members)
	}
	for _, action := range []string{"plantation_sow", "plantation_fertilize", "plantation_harvest", "npc_talk", "npc_inspect", "place_say", "scene_back"} {
		if !offered(observation, action) {
			t.Fatalf("missing choice %s", action)
		}
	}
	f.statusErr = errors.New("private plot read")
	if status, observation := navigationGET(t, router); status != 500 || observation.Scene.Data != nil {
		t.Fatalf("partial observation: %d", status)
	}
	if store.writes != 0 || f.executions != 0 {
		t.Fatalf("GET wrote: %d %d", store.writes, f.executions)
	}
}

func TestPlantationCommandReplies(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		harvest        plantation.HarvestResult
		want           string
	}{
		{"plantation_sow", `{"seed_id":"red"}`, plantation.HarvestResult{}, "赤の種をまいたよ！"},
		{"plantation_fertilize", `{"fertilizer_id":"chemical"}`, plantation.HarvestResult{}, "化学肥料をまくよ！"},
		{"plantation_harvest", `{}`, plantation.HarvestResult{Withered: true, SeedName: "金の種"}, "金の種は芽が出なかったよ…"},
		{"plantation_harvest", `{}`, plantation.HarvestResult{Yields: []plantation.HarvestYield{{ItemName: "力の種", Quantity: 2}, {ItemName: "薬草", Quantity: 1}}},
			"収穫したよ！\n力の種を2個\n薬草を1個\n倉庫に送っておいたよ"},
	} {
		t.Run(tc.action+tc.want, func(t *testing.T) {
			f := newPlantationFixture(t)
			f.harvest = tc.harvest
			_, router := f.router(t, "plantation")
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			if got := tavernResult[PlantationResult](t, raw); status != 200 || got.Message != tc.want {
				t.Fatalf("%s: %d %q", tc.action, status, got.Message)
			}
		})
	}
}

func TestPlantationCommandErrors(t *testing.T) {
	for _, tc := range []struct {
		action, params string
		err            error
		code, text     string
		status         int
	}{
		{"plantation_sow", `{"seed_id":"red"}`, plantation.ErrPlotAlreadySown, "PLANTATION_ALREADY_SOWN", "収穫するまでちょっと待ってね", 409},
		{"plantation_sow", `{"seed_id":"red"}`, plantation.ErrInsufficientGold, "PLANTATION_INSUFFICIENT_GOLD", "お金が足りないよ！", 409},
		{"plantation_sow", `{"seed_id":"x"}`, plantation.ErrInvalidSeedID, "PLANTATION_INVALID_SEED", "", 400},
		{"plantation_fertilize", `{"fertilizer_id":"lime"}`, plantation.FertilizerAppliedError{Name: "骨粉"}, "PLANTATION_ALREADY_FERTILIZED", "もう骨粉をまいてあるよ", 409},
		{"plantation_fertilize", `{"fertilizer_id":"lime"}`, plantation.ErrNoActivePlot, "PLANTATION_NO_PLOT", "まず種をまいてね", 409},
		{"plantation_fertilize", `{"fertilizer_id":"kupo_nut"}`, plantation.ErrMissingFertilizerItem, "PLANTATION_INVALID_FERTILIZER", "", 400},
		{"plantation_harvest", `{}`, plantation.ErrNoActivePlot, "PLANTATION_NO_PLOT", "まず種をまいてね", 409},
		{"plantation_harvest", `{}`, plantation.ErrCropNotMatured, "PLANTATION_NOT_MATURED", "芽が出るまで一日待ってね", 409},
		{"plantation_harvest", `{}`, errors.New("private storage failure"), "EXECUTION_FAILED", "", 500},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			f := newPlantationFixture(t)
			f.executionErr = fmt.Errorf("private wrapped: %w", tc.err)
			_, router := f.router(t, "plantation")
			status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody(tc.action, tc.params), nil)
			assertGatewayError(t, got, tc.code)
			if status != tc.status || strings.Contains(fmt.Sprint(got), "private") || !strings.Contains(string(got["error"]), tc.text) {
				t.Fatalf("error mapping: %d %s", status, got)
			}
		})
	}
}

func TestPlantationCommandsRequireTheGarden(t *testing.T) {
	f := newPlantationFixture(t)
	_, router := f.router(t, "bank")
	if status, got := gatewayRequest(t, router, "hero", "session", "application/json", saleGatewayBody("plantation_harvest", `{}`), nil); status != 409 || f.executions != 0 {
		t.Fatalf("harvest outside the garden: %d %s", status, got)
	}
}

func TestPlantationWordsNameCatalogEntries(t *testing.T) {
	words, err := plantationWords(nil, context.Background(), corecharacter.Character{})
	if err != nil || !strings.HasPrefix(words[7], "今日は") || !strings.HasPrefix(words[8], "肥料は") {
		t.Fatalf("words %v %v", words, err)
	}
	firstSeven := []string{}
	for _, f := range plantation.AllFertilizers()[:7] {
		firstSeven = append(firstSeven, "肥料は"+f.Name+"をまくのはどうかな？")
	}
	if !strings.Contains(strings.Join(firstSeven, "|"), words[8]) {
		t.Fatalf("fertilizer tip %q is outside the first seven", words[8])
	}
}
