package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/runhost"
)

type partyLeaveParams struct {
	PartyID string `json:"party_id"`
}

type partyKickParams struct {
	PartyID           string `json:"party_id"`
	TargetCharacterID string `json:"target_character_id"`
}

type partyMessageResponse struct {
	Message string `json:"message"`
}

func partyLeaveCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyLeaveParams]("party_leave", nil, nil)
	}
	return withActionCommand("party_leave", func(ctx context.Context, actor string, p partyLeaveParams) (any, error) {
		if err := service.LeaveParty(ctx, p.PartyID, actor); err != nil {
			return nil, err
		}
		return partyMessageResponse{Message: "パーティーから離れました"}, nil
	}, partyLobbyRejection)
}

func partyKickCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyKickParams]("party_kick", nil, nil)
	}
	return withActionCommand("party_kick", func(ctx context.Context, actor string, p partyKickParams) (any, error) {
		if err := service.KickMember(ctx, p.PartyID, actor, p.TargetCharacterID); err != nil {
			return nil, err
		}
		return partyMessageResponse{Message: "メンバーをきっくしました"}, nil
	}, partyLobbyRejection)
}

func partyLobbyRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, party.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "PARTY_NOT_FOUND", Message: "Party not found."}
	case errors.Is(err, party.ErrCharacterNotInParty):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_NOT_MEMBER", Message: "Character is not in this party."}
	case errors.Is(err, party.ErrNotPartyLeader):
		return http.StatusForbidden, ErrorDetail{Code: "PARTY_NOT_LEADER", Message: "Only the party leader can do that."}
	case errors.Is(err, party.ErrCannotKickSelf):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_CANNOT_KICK_SELF", Message: "自分をきっくすることはできません"}
	case errors.Is(err, runhost.ErrBusy), errors.Is(err, party.ErrPartyNotRecruiting):
		return http.StatusConflict, ErrorDetail{Code: "PARTY_NOT_RECRUITING", Message: "The party has started or is changing; refresh before retrying."}
	default:
		return 0, ErrorDetail{}
	}
}
