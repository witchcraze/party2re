package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/challenge"
)

type challengeExitService interface {
	ExitSession(context.Context, string, string, int) (*challenge.CommandResult, error)
	GetCommandResult(context.Context, string, string, int) (*challenge.CommandResult, error)
}

type challengeRetireResult struct {
	*challenge.CommandResult
	RecoveryPending bool `json:"recovery_pending"`
}

func challengeRetireCommand(exits challengeExitService) Option {
	if exits == nil {
		return withActionCommand[advanceChallengeRequest]("challenge_retire", nil, nil)
	}
	return func(h *Handler) {
		withActionCommand("challenge_retire", func(ctx context.Context, actor string, p advanceChallengeRequest) (any, error) {
			if p.SessionID == "" || p.ExpectedRevision < 1 {
				return nil, challenge.ErrRevisionConflict
			}
			result, err := exits.ExitSession(ctx, actor, p.SessionID, p.ExpectedRevision)
			pending := errors.Is(err, challenge.ErrRecoveryRequired)
			if err != nil && !(pending && result != nil) {
				return nil, err
			}
			return challengeRetireResult{CommandResult: result, RecoveryPending: pending}, nil
		}, challengeActionRejection)(h)
		command := h.actionCommands["challenge_retire"]
		command.replay = func(ctx context.Context, actor string, raw json.RawMessage) (bool, error) {
			var p advanceChallengeRequest
			if err := json.Unmarshal(raw, &p); err != nil {
				return false, err
			}
			result, err := exits.GetCommandResult(ctx, actor, p.SessionID, p.ExpectedRevision)
			if errors.Is(err, challenge.ErrForbidden) {
				return false, nil
			}
			return result != nil, err
		}
		h.actionCommands["challenge_retire"] = command
	}
}

func challengeActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, challenge.ErrRecoveryRequired), errors.Is(err, challenge.ErrOutcomeUnknown):
		return 0, ErrorDetail{}
	case errors.Is(err, challenge.ErrRevisionConflict):
		return http.StatusConflict, ErrorDetail{Code: "CHALLENGE_REVISION_CONFLICT", Message: "Challenge revision or command conflicts."}
	case errors.Is(err, challenge.ErrForbidden):
		return http.StatusForbidden, ErrorDetail{Code: "CHALLENGE_FORBIDDEN", Message: "Challenge participant required."}
	case errors.Is(err, challenge.ErrSessionNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHALLENGE_NOT_FOUND", Message: "Challenge run not found."}
	case errors.Is(err, challenge.ErrSessionNotActive):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "CHALLENGE_NOT_ACTIVE", Message: "Challenge run is not active."}
	default:
		return 0, ErrorDetail{}
	}
}
