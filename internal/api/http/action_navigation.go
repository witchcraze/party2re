package http

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type sceneEnterParams struct {
	Destination string `json:"destination"`
}

// navigationCommand supplies the authenticated owner separately from typed params.
func navigationCommand[P, R any](h *Handler, id string, execute func(context.Context, string, string, P, playercontext.Result) (R, error)) {
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
		prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
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
			return func(ctx context.Context, playerID, actorID string, current playercontext.Result) (any, error) {
				return execute(ctx, playerID, actorID, p, current)
			}, nil
		},
	}
}

func (h *Handler) registerNavigation(service *playercontext.Service) {
	if service == nil || !service.NavigationConfigured() {
		return
	}
	navigationCommand(h, "scene_enter", func(ctx context.Context, owner, actor string, p sceneEnterParams, _ playercontext.Result) (sceneEnterResult, error) {
		selection, err := service.Enter(ctx, owner, actor, p.Destination)
		if err != nil {
			return sceneEnterResult{}, err
		}
		h.leavePlace(ctx, actor)
		return h.deliverOnEntry(ctx, owner, actor, selection)
	})
	navigationCommand(h, "scene_select", func(ctx context.Context, owner, actor string, p playercontext.Subject, current playercontext.Result) (playercontext.Selection, error) {
		selection, err := service.Select(ctx, owner, actor, p)
		if err == nil {
			next := current
			next.Navigation = &playercontext.NavigationObservation{Selection: selection}
			// An unresolved place counts as a move; the entry would expire anyway.
			from, fromErr := h.scenePlace(ctx, current)
			to, toErr := h.scenePlace(ctx, next)
			if fromErr != nil || toErr != nil || from != to {
				h.leavePlace(ctx, actor)
			}
		}
		return selection, err
	})
	navigationCommand(h, "scene_page", func(ctx context.Context, owner, actor string, p playercontext.PageParams, _ playercontext.Result) (playercontext.Selection, error) {
		return service.Page(ctx, owner, actor, p)
	})
	navigationCommand(h, "scene_back", func(ctx context.Context, owner, actor string, _ struct{}, _ playercontext.Result) (playercontext.Selection, error) {
		selection, err := service.Back(ctx, owner, actor)
		if err == nil {
			h.leavePlace(ctx, actor)
		}
		return selection, err
	})
}

func navigationRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, playercontext.ErrInvalidSelection):
		return http.StatusBadRequest, ErrorDetail{Code: "INVALID_SELECTION", Message: "Invalid scene, subject or page."}
	case errors.Is(err, playercontext.ErrSelectionNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "SELECTION_NOT_FOUND", Message: "Selected subject is unavailable."}
	case errors.Is(err, playercontext.ErrSceneAccessDenied):
		return http.StatusForbidden, ErrorDetail{Code: "SCENE_ACCESS_DENIED", Message: "Character does not qualify for the selected scene."}
	case errors.Is(err, playercontext.ErrNavigationForbidden), errors.Is(err, adventure.ErrRecordForbidden):
		return http.StatusForbidden, ErrorDetail{Code: "NAVIGATION_FORBIDDEN", Message: "Character is not owned."}
	case errors.Is(err, playercontext.ErrNavigationUnavailable):
		return http.StatusConflict, ErrorDetail{Code: "ACTION_UNAVAILABLE", Message: "Navigation is unavailable during recovery or unfinished work."}
	case errors.Is(err, playercontext.ErrNavigationNotConfigured):
		return http.StatusNotImplemented, ErrorDetail{Code: "ACTION_NOT_IMPLEMENTED", Message: "Navigation is not configured."}
	default:
		return 0, ErrorDetail{}
	}
}
