package http

import (
	"context"
	"errors"
	"net/http"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/exile"
)

// ExileService is the exile vote board exposed through the Gateway.
type ExileService interface {
	GetBoard(ctx context.Context, actorID string) (exile.Board, error)
	Petition(ctx context.Context, actorID, targetName, reason string, now time.Time) (exile.Petition, error)
	Vote(ctx context.Context, actorID, targetName string, approve bool, now time.Time) (exile.VoteResult, error)
}

type exilePetitionParams struct {
	TargetName string `json:"target_name"`
	Reason     string `json:"reason"`
}

type exileVoteParams struct {
	TargetName string `json:"target_name"`
}

// WithExile configures the exile board scene and its Gateway commands.
func WithExile(service ExileService) Option {
	return func(h *Handler) {
		h.exile = service
		if service == nil {
			withActionCommand[exilePetitionParams]("exile_petition", nil, nil)(h)
			withActionCommand[exileVoteParams]("exile_approve", nil, nil)(h)
			withActionCommand[exileVoteParams]("exile_oppose", nil, nil)(h)
			return
		}
		withActionCommand("exile_petition", func(ctx context.Context, actorID string, p exilePetitionParams) (any, error) {
			return service.Petition(ctx, actorID, p.TargetName, p.Reason, time.Now().UTC())
		}, exileActionRejection)(h)
		for id, approve := range map[string]bool{"exile_approve": true, "exile_oppose": false} {
			withActionCommand(id, func(ctx context.Context, actorID string, p exileVoteParams) (any, error) {
				return service.Vote(ctx, actorID, p.TargetName, approve, time.Now().UTC())
			}, exileActionRejection)(h)
		}
	}
}

func exileActionRejection(err error) (int, ErrorDetail) {
	conflict := func(code, message string) (int, ErrorDetail) {
		return http.StatusConflict, ErrorDetail{Code: code, Message: message}
	}
	switch {
	case errors.Is(err, exile.ErrInvalidTarget):
		return http.StatusBadRequest, ErrorDetail{Code: "EXILE_INVALID_INPUT", Message: "Target name and reason are required."}
	case errors.Is(err, exile.ErrInvalidReason):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "EXILE_INVALID_REASON", Message: "Exile reason is invalid or too long."}
	case errors.Is(err, exile.ErrInvalidCharacterID):
		return http.StatusBadRequest, ErrorDetail{Code: "EXILE_INVALID_CHARACTER_ID", Message: "Invalid character ID."}
	case errors.Is(err, exile.ErrTargetNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "EXILE_TARGET_NOT_FOUND", Message: "Target character does not exist."}
	case errors.Is(err, exile.ErrPetitionNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "EXILE_PETITION_NOT_FOUND", Message: "No open exile petition matched."}
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, exile.ErrJobLevelTooLow):
		return conflict("EXILE_JOB_LEVEL_TOO_LOW", "At least 50 job changes are required.")
	case errors.Is(err, exile.ErrSelfPetition):
		return conflict("EXILE_SELF_PETITION", "You cannot petition to exile yourself.")
	case errors.Is(err, exile.ErrAlreadyPetitioned):
		return conflict("EXILE_ALREADY_PETITIONED", "The target already has an open petition.")
	case errors.Is(err, exile.ErrPetitionLimit):
		return conflict("EXILE_PETITION_LIMIT", "Wait for open petitions to be resolved.")
	case errors.Is(err, exile.ErrCancelLocked):
		return conflict("EXILE_WITHDRAWAL_LOCKED", "Petitions cannot be withdrawn shortly after a vote.")
	case errors.Is(err, exile.ErrAlreadyVoted):
		return conflict("EXILE_ALREADY_VOTED", "You already voted on this petition.")
	case errors.Is(err, exile.ErrTargetCannotVote):
		return conflict("EXILE_TARGET_CANNOT_VOTE", "The petitioned character cannot vote.")
	default:
		return 0, ErrorDetail{}
	}
}
