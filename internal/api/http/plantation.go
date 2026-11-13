package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/plantation"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// PlantationService is the seed garden surface used by its scene and commands.
type PlantationService interface {
	GetStatus(ctx context.Context, characterID string) (plantation.StatusResponse, error)
	Sow(ctx context.Context, characterID string, seedID string) (plantation.SowResult, error)
	Fertilize(ctx context.Context, characterID string, fertilizerID string) (plantation.FertilizeResult, error)
	Harvest(ctx context.Context, characterID string) (plantation.HarvestResult, error)
}

type plantationSowRequest struct {
	SeedID string `json:"seed_id"`
}

type plantationFertilizeRequest struct {
	FertilizerID string `json:"fertilizer_id"`
}

// PlantationSceneData is the actor's plot, the seeds on sale and the usable
// fertilizers (item fertilizers only while held, as legacy lists them).
type PlantationSceneData struct {
	Parent string `json:"parent"`
	plantation.StatusResponse
}

// PlantationResult is a seed garden command outcome with the gardener's reply.
type PlantationResult struct {
	Plot     *plantation.Plot          `json:"plot,omitempty"`
	Withered bool                      `json:"withered,omitempty"`
	Yields   []plantation.HarvestYield `json:"yields,omitempty"`
	Message  string                    `json:"message"`
}

// WithPlantation configures the seed garden scene observation and its commands.
func WithPlantation(service PlantationService) Option {
	return func(h *Handler) {
		h.plantation = service
		if service == nil {
			withActionCommand[plantationSowRequest]("plantation_sow", nil, nil)(h)
			withActionCommand[plantationFertilizeRequest]("plantation_fertilize", nil, nil)(h)
			withActionCommand[struct{}]("plantation_harvest", nil, nil)(h)
			return
		}
		withActionCommand("plantation_sow", func(ctx context.Context, actor string, p plantationSowRequest) (any, error) {
			res, err := service.Sow(ctx, actor, p.SeedID)
			if err != nil {
				return nil, err
			}
			return PlantationResult{Plot: &res.Plot, Message: res.SeedName + "をまいたよ！"}, nil
		}, plantationRejection)(h)
		withActionCommand("plantation_fertilize", func(ctx context.Context, actor string, p plantationFertilizeRequest) (any, error) {
			res, err := service.Fertilize(ctx, actor, p.FertilizerID)
			if err != nil {
				return nil, err
			}
			return PlantationResult{Plot: &res.Plot, Message: res.FertilizerName + "をまくよ！"}, nil
		}, plantationRejection)(h)
		withActionCommand("plantation_harvest", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			res, err := service.Harvest(ctx, actor)
			if err != nil {
				return nil, err
			}
			return PlantationResult{Withered: res.Withered, Yields: res.Yields, Message: harvestMessage(res)}, nil
		}, plantationRejection)(h)
	}
}

// harvestMessage follows plantation.cgi shukaku/getSeed.
func harvestMessage(res plantation.HarvestResult) string {
	if res.Withered {
		return res.SeedName + "は芽が出なかったよ…"
	}
	var b strings.Builder
	b.WriteString("収穫したよ！\n")
	for _, y := range res.Yields {
		fmt.Fprintf(&b, "%sを%d個\n", y.ItemName, y.Quantity)
	}
	b.WriteString("倉庫に送っておいたよ")
	return b.String()
}

func (h *Handler) plantationSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.plantation == nil {
		return nil, errSceneNotConfigured
	}
	status, err := h.plantation.GetStatus(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return PlantationSceneData{Parent: "town", StatusResponse: status}, nil
}

func plantationRejection(err error) (int, ErrorDetail) {
	var applied plantation.FertilizerAppliedError
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, plantation.ErrInvalidSeedID):
		return http.StatusBadRequest, ErrorDetail{Code: "PLANTATION_INVALID_SEED", Message: "Choose one of the observed seeds."}
	case errors.Is(err, plantation.ErrInvalidFertilizerID), errors.Is(err, plantation.ErrMissingFertilizerItem):
		return http.StatusBadRequest, ErrorDetail{Code: "PLANTATION_INVALID_FERTILIZER", Message: "Choose one of the observed fertilizers."}
	case errors.Is(err, plantation.ErrInsufficientGold):
		return http.StatusConflict, ErrorDetail{Code: "PLANTATION_INSUFFICIENT_GOLD", Message: "お金が足りないよ！"}
	case errors.Is(err, plantation.ErrPlotAlreadySown):
		return http.StatusConflict, ErrorDetail{Code: "PLANTATION_ALREADY_SOWN", Message: "収穫するまでちょっと待ってね"}
	case errors.As(err, &applied):
		return http.StatusConflict, ErrorDetail{Code: "PLANTATION_ALREADY_FERTILIZED", Message: "もう" + applied.Name + "をまいてあるよ"}
	case errors.Is(err, plantation.ErrNoActivePlot), errors.Is(err, plantation.ErrPlotNotFound):
		return http.StatusConflict, ErrorDetail{Code: "PLANTATION_NO_PLOT", Message: "まず種をまいてね"}
	case errors.Is(err, plantation.ErrCropNotMatured):
		return http.StatusConflict, ErrorDetail{Code: "PLANTATION_NOT_MATURED", Message: "芽が出るまで一日待ってね"}
	default:
		return 0, ErrorDetail{}
	}
}

var plantationFacility = facility{
	title: "種菜園", commands: []string{"plantation_sow", "plantation_fertilize", "plantation_harvest"},
	read: (*Handler).plantationSceneData, placeLog: true,
	npc: &npcProfile{name: "@ロータス", wordsOf: plantationWords},
}

// plantationWords are plantation.cgi @words; the tips name a random seed and
// one of the first seven fertilizers ($ferts[int(rand($#ferts-7))+1]).
func plantationWords(*Handler, context.Context, corecharacter.Character) ([]string, error) {
	seeds, ferts := plantation.AllSeeds(), plantation.AllFertilizers()
	return []string{
		"種をまいて何ができるかはお楽しみ〜♪",
		"一日経つと収穫できるんだ",
		"肥料をまくと良いものが収穫できたり、たくさん芽が出るよ！",
		"肥料によっては枯れやすくなるかも…",
		"肥料に使えるアイテムもあるみたいだよ",
		"どの種をまこうか？",
		"種によって採れるものが変わるよ！",
		"今日は" + seeds[random.Intn(len(seeds))].Name + "がおすすめだよ！",
		"肥料は" + ferts[random.Intn(7)].Name + "をまくのはどうかな？",
	}, nil
}
