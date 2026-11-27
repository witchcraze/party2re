package http

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/gemstore"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// GemStoreSceneData replaces the retired gem-box, catalog, recipe and dialogue
// GETs. Every list is a fixed catalog or capacity-bounded, so it is not paged.
// GetGemBox already returns a non-nil item slice, including for a new box.
type GemStoreSceneData struct {
	Parent  string            `json:"parent"`
	GemBox  gemstore.GemBox   `json:"gem_box"`
	Catalog []gemstore.Gem    `json:"catalog"`
	Recipes []gemstore.Recipe `json:"recipes"`
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
		Catalog: append([]gemstore.Gem{}, h.gemstore.GetCatalog(char.JobLevel)...),
		Recipes: append([]gemstore.Recipe{}, h.gemstore.GetRecipes()...)}, nil
}

var gemStoreFacility = facility{
	title: "宝石店", commands: []string{"gemstore_buy", "gemstore_sell", "gemstore_send", "gemstore_synthesize", "gemstore_appraise", "gemstore_sort"},
	read: (*Handler).gemStoreSceneData, placeLog: true,
	npc: &npcProfile{name: "@ジェマ", wordsOf: gemStoreWords},
}

// gemStoreWords recommends a random gem on sale at the speaker's tier (gem_store.cgi @words).
func gemStoreWords(h *Handler, _ context.Context, char corecharacter.Character) ([]string, error) {
	words := []string{
		"ここは宝石店です。色んな宝石を売っています", "どんな宝石がお好きですか？",
		"アイテムや宝石を使って、新しい宝石に加工することもできますよ", "宝石ごとに不思議な力があるんです",
	}
	if h.gemstore == nil {
		return words, nil
	}
	if sales := h.gemstore.GetCatalog(char.JobLevel); len(sales) > 0 {
		words = append(words, "{name}さんには "+sales[random.Intn(len(sales))].Name+" が似合いそうですね")
	}
	return words, nil
}
