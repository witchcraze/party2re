package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/blackmarket"
)

// BlackMarketService defines the black market observation, commands and retained NPC reads.
type BlackMarketService interface {
	GetStatus(ctx context.Context, characterID string) (*blackmarket.Status, error)
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
