package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/runhost"
)

// RunService is a run host's owned command, receipt and observation port.
type RunService interface {
	Command(context.Context, runhost.Command) (runhost.Receipt, error)
	GetRunReceipt(ctx context.Context, player, actor, runID, key string) (*runhost.Receipt, error)
	ObserveRun(ctx context.Context, player, actor string) (*runhost.Observation, error)
	RecoverTimeout(ctx context.Context, player, actor, runID string) error
}

// NormalRunService adds normal-adventure enrollment to the run port.
type NormalRunService interface {
	RunService
	EnterSolo(ctx context.Context, player, actor, key, name, stageID, mode string, speed int) (runhost.Receipt, error)
	EnterParty(ctx context.Context, player, actor, partyID string) (runhost.Receipt, error)
}

type runParams struct {
	RunID     string `json:"run_id"`
	Version   int    `json:"version"`
	CommandID string `json:"command_id"`
}

type runOperationParams struct {
	runParams
	Operation         string `json:"operation"`
	TargetID          string `json:"target_id"`
	Input             string `json:"input"`
	SecondOperation   string `json:"second_operation"`
	SecondTargetID    string `json:"second_target_id"`
	FollowUpOperation string `json:"follow_up_operation"`
	FollowUpTargetID  string `json:"follow_up_target_id"`
}

type runChestParams struct {
	runParams
	BoxID string `json:"box_id"`
}

type adventureStartParams struct {
	Name      string `json:"name"`
	StageID   string `json:"stage_id"`
	Speed     int    `json:"speed"`
	CommandID string `json:"command_id"`
}

type partyStartParams struct {
	PartyID string `json:"party_id"`
}

// RunReceiptResponse is the executing actor's committed result, including
// after its exit; it never carries other members' ledgers.
type RunReceiptResponse struct {
	RunID       string                `json:"run_id"`
	CommandID   string                `json:"command_id"`
	Version     int                   `json:"version"`
	Round       int                   `json:"round"`
	Disposition string                `json:"disposition"`
	Box         *runhost.TreasureBox  `json:"box,omitempty"`
	Recipient   RunRecipientResponse  `json:"recipient"`
	Inspections []runhost.Inspection  `json:"inspections"`
	Tames       []monsterTameResponse `json:"monster_tames"`
}

// RunRecipientResponse is the actor's own earnings ledger.
type RunRecipientResponse struct {
	Pending      runhost.Earnings `json:"pending"`
	Conditional  runhost.Earnings `json:"conditional"`
	Applied      runhost.Earnings `json:"applied"`
	Inspected    int              `json:"inspected"`
	PenaltyUntil *time.Time       `json:"penalty_until,omitempty"`
}

var errInvalidRunParams = errors.New("invalid run command parameters")

// WithNormalRuns connects normal-adventure enrollment, the run commands and
// the active run scene.
func WithNormalRuns(runs NormalRunService) Option {
	return func(h *Handler) {
		h.runs = runs
		registerRunCommand(h, "adventure_start", func(ctx context.Context, player, actor string, p adventureStartParams) (any, error) {
			if p.CommandID == "" {
				return nil, errInvalidRunParams
			}
			receipt, err := runs.EnterSolo(ctx, player, actor, p.CommandID, p.Name, p.StageID, adventure.CrawlModeNormal, p.Speed)
			return toRunReceipt(receipt), err
		}, nil)
		registerRunCommand(h, "party_start", func(ctx context.Context, player, actor string, p partyStartParams) (any, error) {
			receipt, err := runs.EnterParty(ctx, player, actor, p.PartyID)
			return toRunReceipt(receipt), err
		}, nil)
		replay := runReceiptReplay(runs)
		registerRunCommand(h, "run_operation", func(ctx context.Context, player, actor string, p runOperationParams) (any, error) {
			// chest, exit and cleanup have their own strict commands.
			if p.Operation == "" || slices.Contains([]string{"chest", "exit", "cleanup", "enroll", "join"}, p.Operation) {
				return nil, errInvalidRunParams
			}
			cmd := p.command(player, actor, p.Operation, p.TargetID)
			cmd.Input, cmd.SecondAction, cmd.SecondTargetID, cmd.FollowUpAction, cmd.FollowUpTargetID = p.Input, p.SecondOperation, p.SecondTargetID, p.FollowUpOperation, p.FollowUpTargetID
			return runCommand(ctx, runs, cmd)
		}, replay)
		registerRunCommand(h, "run_chest", func(ctx context.Context, player, actor string, p runChestParams) (any, error) {
			return runCommand(ctx, runs, p.command(player, actor, "chest", p.BoxID))
		}, replay)
		registerRunCommand(h, "run_exit", func(ctx context.Context, player, actor string, p runParams) (any, error) {
			return runCommand(ctx, runs, p.command(player, actor, "exit", ""))
		}, replay)
		registerRunCommand(h, "run_recover", func(ctx context.Context, player, actor string, p runParams) (any, error) {
			obs, err := runs.ObserveRun(ctx, player, actor)
			if err != nil {
				return nil, err
			}
			// A committed cleanup whose penalty projection failed resumes it.
			if obs != nil && obs.RunID == p.RunID && obs.Recipient.TimerPending {
				return nil, runs.RecoverTimeout(ctx, player, actor, p.RunID)
			}
			return runCommand(ctx, runs, p.command(player, actor, "cleanup", ""))
		}, replay)
	}
}

func (p runParams) command(player, actor, action, target string) runhost.Command {
	return runhost.Command{PlayerID: player, ActorID: actor, RunID: p.RunID, ID: p.CommandID, Version: p.Version, Action: action, TargetID: target}
}

func runCommand(ctx context.Context, runs RunService, cmd runhost.Command) (any, error) {
	if cmd.RunID == "" || cmd.ID == "" || cmd.Version < 1 {
		return nil, errInvalidRunParams
	}
	receipt, err := runs.Command(ctx, cmd)
	if err != nil {
		return nil, err
	}
	return toRunReceipt(receipt), nil
}

// runReceiptReplay admits an explicit retry of a committed command after its
// continuation disappeared, such as after personal exit. Command itself
// returns the stored result only for identical input.
func runReceiptReplay(runs RunService) func(context.Context, string, string, json.RawMessage) (bool, error) {
	return func(ctx context.Context, player, actor string, raw json.RawMessage) (bool, error) {
		var p runParams
		if json.Unmarshal(raw, &p) != nil || p.RunID == "" || p.CommandID == "" {
			return false, nil
		}
		receipt, err := runs.GetRunReceipt(ctx, player, actor, p.RunID, p.CommandID)
		if errors.Is(err, runhost.ErrForbidden) {
			return false, nil
		}
		return receipt != nil, err
	}
}

func registerRunCommand[P any](h *Handler, id string, execute func(context.Context, string, string, P) (any, error), replay func(context.Context, string, string, json.RawMessage) (bool, error)) {
	def, known := actionDefinition(id)
	if !known {
		panic("action command is not in the catalog: " + id)
	}
	if h.actionCommands == nil {
		h.actionCommands = make(map[string]actionCommand)
	}
	h.actionCommands[id] = actionCommand{reject: runRejection, replay: replay, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var params P
		if err := decodeActionParams(raw, def.RequiredParams, &params); err != nil {
			return nil, err
		}
		return func(ctx context.Context, player, actor string, _ playercontext.Result) (any, error) {
			return execute(ctx, player, actor, params)
		}, nil
	}}
}

func toRunReceipt(r runhost.Receipt) RunReceiptResponse {
	out := RunReceiptResponse{RunID: r.Command.RunID, CommandID: r.Command.ID, Version: r.Version, Round: r.Round, Disposition: r.Disposition, Box: r.Box,
		Recipient:   runRecipient(r.Recipient),
		Inspections: append([]runhost.Inspection{}, r.Inspections...), Tames: toMonsterTames(map[string][]battle.MonsterTameResult{r.Command.ActorID: r.MonsterTames})[r.Command.ActorID]}
	if !r.Recipient.PenaltyUntil.IsZero() {
		until := r.Recipient.PenaltyUntil.UTC()
		out.Recipient.PenaltyUntil = &until
	}
	return out
}

func runRejection(err error) (int, ErrorDetail) {
	if status, detail := adventureActionRejection(err); status != 0 {
		return status, detail
	}
	conflict := func(code, message string) (int, ErrorDetail) {
		return http.StatusConflict, ErrorDetail{Code: code, Message: message}
	}
	unprocessable := func(code, message string) (int, ErrorDetail) {
		return http.StatusUnprocessableEntity, ErrorDetail{Code: code, Message: message}
	}
	switch {
	case errors.Is(err, runhost.ErrRecovery):
		// Unknown commits join ErrRecovery; never present them as rejections.
		return 0, ErrorDetail{}
	case errors.Is(err, errInvalidRunParams):
		return http.StatusBadRequest, ErrorDetail{Code: "INVALID_ACTION_PARAMS", Message: "Invalid action parameters."}
	case errors.Is(err, runhost.ErrConflict):
		return conflict("RUN_CONFLICT", "The run changed or the command identity was reused with other input. Observe again.")
	case errors.Is(err, runhost.ErrCooldown):
		return conflict("RUN_COOLDOWN", "まだ行動することはできません")
	case errors.Is(err, runhost.ErrBusy), errors.Is(err, party.ErrMemberBusy), errors.Is(err, party.ErrAlreadyInParty):
		return conflict("RUN_BUSY", "A participant has unfinished work.")
	case errors.Is(err, runhost.ErrForbidden), errors.Is(err, runhost.ErrNotParticipant), errors.Is(err, party.ErrNotPartyLeader), errors.Is(err, party.ErrCharacterNotInParty), errors.Is(err, adventure.ErrLeaderOnly):
		return http.StatusForbidden, ErrorDetail{Code: "RUN_FORBIDDEN", Message: "This character cannot perform that run command."}
	case errors.Is(err, runhost.ErrNotFound), errors.Is(err, party.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "RUN_NOT_FOUND", Message: "Run not found."}
	case errors.Is(err, party.ErrPartyNotReady), errors.Is(err, party.ErrPartyNotRecruiting):
		return conflict("PARTY_NOT_READY", "Every member must be ready before starting.")
	case errors.Is(err, runhost.ErrCharacterUnconscious), errors.Is(err, runhost.ErrActorUnconscious), errors.Is(err, adventure.ErrUnconsciousCharacter), errors.Is(err, party.ErrCharacterUnconscious):
		return unprocessable("RUN_UNCONSCIOUS", "A fallen character cannot do that.")
	case errors.Is(err, runhost.ErrCharacterExhausted), errors.Is(err, party.ErrCharacterExhausted):
		return unprocessable("RUN_EXHAUSTED", "Character must rest before adventuring.")
	case errors.Is(err, runhost.ErrNoUnopenedTreasure):
		return unprocessable("RUN_NO_TREASURE", "No unopened treasure box remains.")
	case errors.Is(err, runhost.ErrTreasureLimitReached):
		return unprocessable("RUN_TREASURE_LIMIT", "This character cannot open another box.")
	case errors.Is(err, runhost.ErrNoParticipants), errors.Is(err, runhost.ErrRoomFull):
		return unprocessable("RUN_ROSTER_INVALID", "The run roster is not admissible.")
	case errors.Is(err, runhost.ErrUnavailable), errors.Is(err, runhost.ErrInvalidAction), errors.Is(err, adventure.ErrInvalidFloorAction), errors.Is(err, corebattle.ErrUnsupportedEffect), errors.Is(err, adventure.ErrInvalidCrawlMode), errors.Is(err, party.ErrStageNotFound):
		return unprocessable("RUN_OPERATION_UNAVAILABLE", "That operation is not available now.")
	default:
		return 0, ErrorDetail{}
	}
}
