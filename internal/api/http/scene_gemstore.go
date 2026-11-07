package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// GemStoreSceneData replaces the retired gem-box, catalog, recipe and dialogue
// GETs. Every list is a fixed catalog or capacity-bounded, so it is not paged.
// GetGemBox already returns a non-nil item slice, including for a new box.
type GemStoreSceneData struct {
	Parent   string            `json:"parent"`
	GemBox   gemstore.GemBox   `json:"gem_box"`
	Catalog  []gemstore.Gem    `json:"catalog"`
	Recipes  []gemstore.Recipe `json:"recipes"`
	Dialogue []string          `json:"dialogue"`
}

func (h *Handler) gemStoreSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.gemstore == nil {
		return nil, errSceneNotConfigured
	}
	char := r.Snapshot.Character
	box, err := h.gemstore.GetGemBox(ctx, char.ID)
	if err != nil {
		return nil, err
	}
	return GemStoreSceneData{Parent: "town", GemBox: box,
		Catalog:  append([]gemstore.Gem{}, h.gemstore.GetCatalog(char.JobLevel)...),
		Recipes:  append([]gemstore.Recipe{}, h.gemstore.GetRecipes()...),
		Dialogue: append([]string{}, h.gemstore.GetDialogue()...)}, nil
}
