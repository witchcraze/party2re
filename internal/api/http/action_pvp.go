package http

import (
	"context"
	"errors"
	"net/http"

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
		if err := service.LeaveRoom(ctx, actor, p.RoomID); err != nil {
			return nil, err
		}
		return pvpMessageResponse{Message: "left room successfully"}, nil
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
			return pvpMessageResponse{Message: "kicked member successfully"}, nil
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
		return http.StatusForbidden, ErrorDetail{Code: "PVP_NOT_LEADER", Message: "Only the room leader can kick members."}
	case errors.Is(err, pvp.ErrCannotKickSelf):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_CANNOT_KICK_SELF", Message: "Leader cannot kick self."}
	case errors.Is(err, pvp.ErrCharacterNotInRoom):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_NOT_IN_ROOM", Message: "Character is not in this PvP room."}
	case errors.Is(err, pvp.ErrMatchAlreadyStarted):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PVP_MATCH_ALREADY_STARTED", Message: "Cannot kick members after the match starts."}
	case errors.Is(err, pvp.ErrRoomInactive):
		return http.StatusConflict, ErrorDetail{Code: "PVP_ROOM_INACTIVE", Message: "PvP room expired due to inactivity."}
	case errors.Is(err, pvp.ErrTransitionConflict):
		return http.StatusConflict, ErrorDetail{Code: "PVP_TRANSITION_CONFLICT", Message: "PvP room changed; refresh before retrying."}
	default:
		return 0, ErrorDetail{}
	}
}
