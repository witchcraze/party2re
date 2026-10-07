package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// actionCommand separates parameter decoding from execution so invalid requests
// never reach a service. Only explicit 4xx mappings are known rejections.
type actionCommand struct {
	prepare func(json.RawMessage) (func(context.Context, string, string) (any, error), error)
	reject  func(error) (int, ErrorDetail)
}

// withActionCommand is the package-local seam for subsequent service adapters.
// Register only catalog IDs, once during Handler construction. A nil execute
// function represents an unconfigured dependency. No domain adapters are enabled
// by this common boundary alone.
func withActionCommand[P any](id string, execute func(context.Context, string, P) (any, error), reject func(error) (int, ErrorDetail)) Option {
	def, known := actionDefinition(id)
	if !known {
		panic("action command is not in the catalog: " + id)
	}
	command := actionCommand{reject: reject}
	if execute != nil {
		command.prepare = func(raw json.RawMessage) (func(context.Context, string, string) (any, error), error) {
			var params P
			if err := decodeActionParams(raw, def.RequiredParams, &params); err != nil {
				return nil, err
			}
			return func(ctx context.Context, _ string, actorID string) (any, error) { return execute(ctx, actorID, params) }, nil
		}
	}
	return func(h *Handler) {
		if h.actionCommands == nil {
			h.actionCommands = make(map[string]actionCommand)
		}
		if _, exists := h.actionCommands[id]; exists {
			panic("action command already registered: " + id)
		}
		h.actionCommands[id] = command
	}
}

func actionDefinition(id string) (playercontext.ActionDefinition, bool) {
	for _, def := range playercontext.DefaultCatalog {
		if def.ID == id {
			return def, true
		}
	}
	return playercontext.ActionDefinition{}, false
}

type actionRequest struct {
	Action string          `json:"action"`
	Params json.RawMessage `json:"params"`
}

type actionFailureResponse struct {
	Success bool        `json:"success"`
	Error   ErrorDetail `json:"error"`
}

type actionSuccessResponse struct {
	Success      bool                   `json:"success"`
	Result       any                    `json:"result"`
	Context      *PlayerContextResponse `json:"context"`
	ContextError *ErrorDetail           `json:"context_error,omitempty"`
}

type actionRejectionResponse struct {
	actionFailureResponse
	Context      *PlayerContextResponse `json:"context"`
	ContextError *ErrorDetail           `json:"context_error,omitempty"`
}

func writeActionFailure(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, actionFailureResponse{Error: ErrorDetail{Code: code, Message: message}})
}

func (h *Handler) handleCharacterAction(w http.ResponseWriter, r *http.Request) {
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(player coreplayer.Player, char corecharacter.Character) {
		var req *actionRequest
		if !decodeJSON(w, r, &req) {
			return
		}
		if req == nil || strings.TrimSpace(req.Action) == "" {
			writeError(w, http.StatusBadRequest, errors.New("missing action"))
			return
		}
		if _, known := actionDefinition(req.Action); !known {
			writeActionFailure(w, http.StatusNotFound, "ACTION_NOT_FOUND", "Unknown action.")
			return
		}
		command := h.actionCommands[req.Action]
		if command.prepare == nil || h.playerContext == nil {
			writeActionFailure(w, http.StatusNotImplemented, "ACTION_NOT_IMPLEMENTED", "Action service not configured.")
			return
		}
		execute, err := command.prepare(req.Params)
		if err != nil {
			writeActionFailure(w, http.StatusBadRequest, "INVALID_ACTION_PARAMS", "Invalid action parameters.")
			return
		}
		current, err := h.playerContext.Query(r.Context(), char.ID, player.ID)
		if err != nil {
			if errors.Is(err, playercontext.ErrNavigationForbidden) {
				writeError(w, http.StatusForbidden, err)
				return
			}
			writeActionFailure(w, http.StatusInternalServerError, "ACTION_PREFLIGHT_FAILED", "Unable to verify current action eligibility.")
			return
		}
		// A premature Wake reaches the service for its recognized rejection.
		// GET still offers Wake only when recovery is ready; an awake actor
		// remains subject to entry eligibility.
		earlyWake := req.Action == "home_wake" && current.Snapshot.SleepRemaining > 0
		if !earlyWake && !slices.Contains(current.AvailableActions, req.Action) {
			h.writeActionRejection(w, r, player.ID, char.ID, http.StatusConflict, ErrorDetail{Code: "ACTION_UNAVAILABLE", Message: "Action is currently unavailable."})
			return
		}
		if req.Action != "home_wake" && req.Action != "rescue_request" {
			message, err := h.sleepingCharacterMessage(r.Context(), char.ID)
			if err != nil {
				writeActionFailure(w, http.StatusInternalServerError, "ACTION_PREFLIGHT_FAILED", "Unable to verify current sleep state.")
				return
			}
			if message != "" {
				h.writeActionRejection(w, r, player.ID, char.ID, http.StatusConflict, ErrorDetail{Code: "ACTION_UNAVAILABLE", Message: message})
				return
			}
		}
		result, err := execute(r.Context(), player.ID, char.ID)
		if err != nil {
			if command.reject != nil {
				status, detail := command.reject(err)
				if status >= 400 && status < 500 {
					h.writeActionRejection(w, r, player.ID, char.ID, status, detail)
					return
				}
			}
			writeActionFailure(w, http.StatusInternalServerError, "EXECUTION_FAILED", "Action outcome could not be confirmed. Do not automatically retry.")
			return
		}
		observation, refreshError := h.refreshActionContext(r.Context(), player.ID, char.ID)
		writeJSON(w, http.StatusOK, actionSuccessResponse{Success: true, Result: result, Context: observation, ContextError: refreshError})
	})
}

func (h *Handler) writeActionRejection(w http.ResponseWriter, r *http.Request, playerID, charID string, status int, detail ErrorDetail) {
	observation, refreshError := h.refreshActionContext(r.Context(), playerID, charID)
	writeJSON(w, status, actionRejectionResponse{actionFailureResponse: actionFailureResponse{Error: detail}, Context: observation, ContextError: refreshError})
}

func (h *Handler) refreshActionContext(ctx context.Context, playerID, charID string) (*PlayerContextResponse, *ErrorDetail) {
	failure := &ErrorDetail{Code: "CONTEXT_REFRESH_FAILED", Message: "状態を再取得してください。操作を再送しないでください。"}
	result, err := h.playerContext.Query(ctx, charID, playerID)
	if err != nil {
		return nil, failure
	}
	observation, err := h.playerContextResponse(ctx, result, time.Now().UTC())
	if err != nil {
		return nil, failure
	}
	return &observation, nil
}
