package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// BlacksmithSceneData replaces the retired seal-catalog and storage GETs.
type BlacksmithSceneData struct {
	Parent string `json:"parent"`
	blacksmith.Observation
	Seals           []blacksmith.Seal `json:"seals"`
	StorageCapacity int               `json:"storage_capacity"`
}

func (h *Handler) blacksmithSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.blacksmith == nil {
		return nil, errSceneNotConfigured
	}
	observation, err := h.blacksmith.Observe(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return BlacksmithSceneData{Parent: "town", Observation: observation, Seals: blacksmith.Seals, StorageCapacity: blacksmith.MaxStorageSlots}, nil
}

var blacksmithFacility = facility{
	title: "鍛冶屋", commands: []string{"blacksmith_seal", "blacksmith_name", "blacksmith_deposit", "blacksmith_withdraw"},
	read: (*Handler).blacksmithSceneData, placeLog: true,
	npc: &npcProfile{name: "@ブッキー", words: []string{
		"ここは鍛冶屋だ！武器を強化できるぜ！", "武器に刻印するには刻印晶がいるぜ！", "武器を預けたりすると、刻印は消えちまうぜ",
	}},
}
