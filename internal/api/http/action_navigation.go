package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/witchcraze/party2re/internal/playercontext"
)

type sceneEnterParams struct {
	Destination string `json:"destination"`
}

// navigationCommand supplies the authenticated owner separately from typed params.
func navigationCommand[P any](h *Handler, id string, execute func(context.Context, string, string, P) (playercontext.Selection, error)) {
	def, ok := playercontext.GetAction(id)
	if !ok {
		panic("unregistered navigation command: " + id)
	}
	if h.actionCommands == nil {
		h.actionCommands = make(map[string]actionCommand)
	}
	if _, exists := h.actionCommands[id]; exists {
		panic("duplicate navigation command: " + id)
	}
	h.actionCommands[id] = actionCommand{
		reject: navigationRejection,
		prepare: func(raw json.RawMessage) (func(context.Context, string, string) (any, error), error) {
			var p P
			if err := decodeActionParams(raw, def.RequiredParams, &p); err != nil {
				return nil, err
			}
			var fields map[string]json.RawMessage
			if len(raw) > 0 {
				if err := json.Unmarshal(raw, &fields); err != nil {
					return nil, err
				}
				for name, value := range fields {
					if !slices.Contains(def.RequiredParams, name) && !(id == "scene_page" && slices.Contains([]string{"limit", "offset", "cursor"}, name)) {
						return nil, playercontext.ErrInvalidSelection
					}
					if bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
						return nil, playercontext.ErrInvalidSelection
					}
				}
				if id == "scene_page" {
					_, offset := fields["offset"]
					_, cursor := fields["cursor"]
					if offset == cursor {
						return nil, playercontext.ErrInvalidSelection
					}
				}
				if value, supplied := fields["limit"]; supplied {
					var limit int
					if err := json.Unmarshal(value, &limit); err != nil || limit < 1 || limit > 100 {
						return nil, playercontext.ErrInvalidSelection
					}
				}
			}
			return func(ctx context.Context, playerID, actorID string) (any, error) {
				return execute(ctx, playerID, actorID, p)
			}, nil
		},
	}
}

func (h *Handler) registerNavigation(service *playercontext.Service) {
	if service == nil || !service.NavigationConfigured() {
		return
	}
	navigationCommand(h, "scene_enter", func(ctx context.Context, owner, actor string, p sceneEnterParams) (playercontext.Selection, error) {
		return service.Enter(ctx, owner, actor, p.Destination)
	})
	navigationCommand(h, "scene_select", service.Select)
	navigationCommand(h, "scene_page", service.Page)
	navigationCommand(h, "scene_back", func(ctx context.Context, owner, actor string, _ struct{}) (playercontext.Selection, error) {
		return service.Back(ctx, owner, actor)
	})
}

func navigationRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, playercontext.ErrInvalidSelection):
		return http.StatusBadRequest, ErrorDetail{Code: "INVALID_SELECTION", Message: "Invalid scene, subject or page."}
	case errors.Is(err, playercontext.ErrSelectionNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SELECTION_NOT_FOUND", Message: "Selected subject is unavailable."}
	case errors.Is(err, playercontext.ErrNavigationForbidden):
		return http.StatusForbidden, ErrorDetail{Code: "NAVIGATION_FORBIDDEN", Message: "Character is not owned."}
	case errors.Is(err, playercontext.ErrNavigationUnavailable):
		return http.StatusConflict, ErrorDetail{Code: "ACTION_UNAVAILABLE", Message: "Navigation is unavailable during recovery or unfinished work."}
	case errors.Is(err, playercontext.ErrNavigationNotConfigured):
		return http.StatusNotImplemented, ErrorDetail{Code: "ACTION_NOT_IMPLEMENTED", Message: "Navigation is not configured."}
	default:
		return 0, ErrorDetail{}
	}
}
