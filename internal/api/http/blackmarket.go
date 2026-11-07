package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blackmarket"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/core/random"
)

// BlackMarketService defines the black market observation, commands and retained NPC reads.
type BlackMarketService interface {
	GetStatus(ctx context.Context, characterID string) (*blackmarket.Status, error)
	Talk(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	Inspect(ctx context.Context, characterID string) (*blackmarket.TalkResult, error)
	SacrificeItem(ctx context.Context, characterID string, itemInstanceID string) (*blackmarket.SacrificeResult, error)
	TradePrize(ctx context.Context, characterID string, prizeID string) (*blackmarket.TradeResult, error)
}

var blackMarketTalkDialogues = []string{
	"よく来たな…。ここは闇市場だ…",
	"表の世界では手に入れられない物を取引している…",
	"物の取引は金では買えないもの…。つまり、魂…ゴホッゴホッ…ではなく、レアアイテムだ…",
	"お前の魂…ではなく、お前が装備しているレアアイテムをささげろ…",
	"レアアイテムをささげることによって…お前のレアポイントが増える…",
	"レアポイントにより取引できるアイテムが違う…",
}

const blackMarketInspectDialogue = "…お前の魂で取引したいのか？"

type blackMarketTalkResponse struct {
	CharacterID string `json:"character_id"`
	NPCName     string `json:"npc_name"`
	Dialogue    string `json:"dialogue"`
}

func (h *Handler) handleBlackMarketTalk(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.blackmarket.Talk(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		dialogue := ""
		if len(blackMarketTalkDialogues) > 0 {
			dialogue = blackMarketTalkDialogues[random.IntN(len(blackMarketTalkDialogues))]
		}

		writeJSON(w, http.StatusOK, blackMarketTalkResponse{
			CharacterID: result.CharacterID,
			NPCName:     result.NPCName,
			Dialogue:    dialogue,
		})
	})
}

func (h *Handler) handleBlackMarketInspect(w http.ResponseWriter, r *http.Request) {
	if h.blackmarket == nil {
		writeError(w, http.StatusNotImplemented, errors.New("black market service not configured"))
		return
	}

	charID := r.PathValue("id")
	h.withAuthenticatedCharacter(w, r, charID, func(_ coreplayer.Player, char corecharacter.Character) {
		result, err := h.blackmarket.Inspect(r.Context(), char.ID)
		if err != nil {
			h.writeBlackMarketError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, blackMarketTalkResponse{
			CharacterID: result.CharacterID,
			NPCName:     result.NPCName,
			Dialogue:    blackMarketInspectDialogue,
		})
	})
}

func (h *Handler) writeBlackMarketError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, blackmarket.ErrAccessDenied):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, blackmarket.ErrCharacterNotFound):
		writeError(w, http.StatusNotFound, err)
	default:
		writeError(w, http.StatusInternalServerError, err)
	}
}
