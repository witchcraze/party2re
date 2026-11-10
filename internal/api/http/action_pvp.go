package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/pvp"
)

type pvpLeaveParams struct {
	RoomID string `json:"room_id"`
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

func pvpLeaveRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, pvp.ErrRoomNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "PVP_ROOM_NOT_FOUND", Message: "PvP room not found."}
	case errors.Is(err, pvp.ErrCharacterNotInRoom):
		return http.StatusForbidden, ErrorDetail{Code: "PVP_NOT_IN_ROOM", Message: "Character is not in this PvP room."}
	case errors.Is(err, pvp.ErrMatchAlreadyStarted):
		return http.StatusConflict, ErrorDetail{Code: "PVP_MATCH_STARTED", Message: "PvP match already started."}
	case errors.Is(err, pvp.ErrTransitionConflict):
		return http.StatusConflict, ErrorDetail{Code: "PVP_TRANSITION_CONFLICT", Message: "PvP room changed; refresh before retrying."}
	default:
		return 0, ErrorDetail{}
	}
}
