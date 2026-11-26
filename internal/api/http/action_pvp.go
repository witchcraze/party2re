package http

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/pvp"
)

type pvpLeaveParams struct {
	RoomID string `json:"room_id"`
}

type pvpKickParams struct {
	RoomID            string `json:"room_id"`
	TargetCharacterID string `json:"target_character_id"`
}

func pvpLeaveCommand(service PvPService) Option {
	if service == nil {
		return withActionCommand[pvpLeaveParams]("pvp_leave", nil, nil)
	}
	return withActionCommand("pvp_leave", func(ctx context.Context, actor string, p pvpLeaveParams) (any, error) {
		res, err := service.LeaveRoom(ctx, actor, p.RoomID)
		if err != nil {
			return nil, err
		}
		resp := leaveResponse{Message: escapeMessage}
		if res.RefundedBet > 0 {
			// Pre-start refund notice from _battle.cgi:1230-1234.
			resp.Message += "\n賭け金の " + strconv.Itoa(res.RefundedBet) + " Gが返金されました"
		}
		if d := res.Delivery; d != nil {
			resp.DeliveryNotice = deliveryNoticeText(d.ItemName, d.Paid)
		}
		return resp, nil
	}, pvpLeaveRejection)
}

func pvpKickCommand(service PvPService) Option {
	if service == nil {
		return withActionCommand[pvpKickParams]("pvp_kick", nil, nil)
	}
	return func(h *Handler) {
		withActionCommand("pvp_kick", func(ctx context.Context, actor string, p pvpKickParams) (any, error) {
			res, err := service.KickMember(ctx, p.RoomID, actor, p.TargetCharacterID)
			if err != nil {
				return nil, err
			}
			h.appendKickPlaceLog(ctx, placelog.Place{Kind: placelog.KindFacility, ID: "quest"}, res.RoomName, res.LeaderName, p.TargetCharacterID)
			return pvpMessageResponse{Message: kickMessage(res.TargetName)}, nil
		}, pvpKickRejection)(h)
	}
}

func pvpLeaveRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, pvp.ErrRoomNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "PVP_ROOM_NOT_FOUND", Message: "PvP room not found."}
	case errors.Is(err, pvp.ErrCharacterNotInRoom):
		return http.StatusForbidden, ErrorDetail{Code: "PVP_NOT_IN_ROOM", Message: "Character is not in this PvP room."}
	case errors.Is(err, pvp.ErrTransitionConflict):
		return http.StatusConflict, ErrorDetail{Code: "PVP_TRANSITION_CONFLICT", Message: "PvP room changed; refresh before retrying."}
	default:
		return 0, ErrorDetail{}
	}
}

func pvpKickRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, pvp.ErrRoomNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "PVP_ROOM_NOT_FOUND", Message: "PvP room not found."}
	case errors.Is(err, pvp.ErrNotRoomLeader):
		return http.StatusForbidden, ErrorDetail{Code: "PVP_NOT_LEADER", Message: kickNotLeaderMessage}
	case errors.Is(err, pvp.ErrCannotKickSelf):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_CANNOT_KICK_SELF", Message: kickSelfMessage}
	case errors.Is(err, pvp.ErrCharacterNotInRoom):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_NOT_IN_ROOM", Message: "Character is not in this PvP room."}
	case errors.Is(err, pvp.ErrMatchAlreadyStarted):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_MATCH_ALREADY_STARTED", Message: kickStartedMessage}
	case errors.Is(err, pvp.ErrRoomInactive):
		return http.StatusConflict, ErrorDetail{Code: "PVP_ROOM_INACTIVE", Message: "PvP room expired due to inactivity."}
	case errors.Is(err, pvp.ErrTransitionConflict):
		return http.StatusConflict, ErrorDetail{Code: "PVP_TRANSITION_CONFLICT", Message: "PvP room changed; refresh before retrying."}
	default:
		return 0, ErrorDetail{}
	}
}
