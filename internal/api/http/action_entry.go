package http

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/alchemy"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

const alchemyClaimCommand = "＠れんきんでかんせいしたものをうけとる"

// sceneEnterResult keeps the selection fields and adds what entry itself delivered.
type sceneEnterResult struct {
	playercontext.Selection
	AlchemyClaim    *AlchemyClaimResult          `json:"alchemy_claim,omitempty"`
	AdventureRecord *adventure.RecordEntryResult `json:"adventure_record,omitempty"`
}

// deliverOnEntry runs after the selection is saved, so a rejected entry never
// claims and a repeated entry cannot deliver twice. GET never reaches it.
func (h *Handler) deliverOnEntry(ctx context.Context, playerID, actor string, selection playercontext.Selection, char corecharacter.Character) (sceneEnterResult, error) {
	result := sceneEnterResult{Selection: selection}
	if selection.Destination == "home_adventure_record" && (selection.Subject.ID == "" || selection.Subject.ID == actor) {
		if h.recordEntry == nil {
			return sceneEnterResult{}, errors.New("adventure record service is not configured")
		}
		record, err := h.recordEntry.OpenOwnRecord(ctx, playerID, actor)
		if err != nil {
			return sceneEnterResult{}, err
		}
		result.AdventureRecord = &record
	}
	if selection.Destination != "alchemy" || h.alchemy == nil {
		return result, nil
	}
	claim, err := h.alchemy.Claim(ctx, actor)
	switch {
	case err == nil:
		reply := alchemyClaimReply(claim)
		result.AlchemyClaim = &reply
		// finish replaces the input with this unregistered ＠ line, so legacy logs
		// both rows without a wait (alchemy.cgi:17,185; party.cgi:20-25).
		h.appendPlaceLog(ctx, placelog.Place{Kind: placelog.KindFacility, ID: "alchemy"}, char, alchemyClaimCommand, alchemyFacility.npc.name, reply.Message, "")
	case errors.Is(err, alchemy.ErrNoActiveSynthesis), errors.Is(err, alchemy.ErrSynthesisNotReady):
		// Nothing to deliver: an ordinary entry.
	default:
		return sceneEnterResult{}, err
	}
	return result, nil
}
