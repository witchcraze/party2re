package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/gvg"
)

type gvgLeaveParams struct {
	RoomID string `json:"room_id"`
}

func gvgLeaveCommand(service GvGService) Option {
	if service == nil {
		return withActionCommand[gvgLeaveParams]("gvg_leave", nil, nil)
	}
	return withActionCommand("gvg_leave", func(ctx context.Context, actor string, p gvgLeaveParams) (any, error) {
		res, err := service.LeaveRoom(ctx, actor, p.RoomID)
		if err != nil {
			return nil, err
		}
		resp := leaveResponse{Message: escapeMessage}
		if d := res.Delivery; d != nil {
			resp.DeliveryNotice = deliveryNoticeText(d.ItemName, d.Paid)
		}
		return resp, nil
	}, gvgLeaveRejection)
}

func gvgLeaveRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, gvg.ErrRoomNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GVG_ROOM_NOT_FOUND", Message: "GvG room not found."}
	case errors.Is(err, gvg.ErrCharacterNotInRoom):
		return http.StatusForbidden, ErrorDetail{Code: "GVG_NOT_IN_ROOM", Message: "Character is not in this GvG room."}
	default:
		return 0, ErrorDetail{}
	}
}
