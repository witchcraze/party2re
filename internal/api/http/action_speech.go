package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/validation"
)

// maxSpeechLength is the legacy $max_comment, counted after HTML escaping like
// letters; the whisper recipient is taken separately.
const maxSpeechLength = 500

// CharacterNameLookup resolves a whisper recipient by exact name.
type CharacterNameLookup interface {
	FindByName(ctx context.Context, name string) (corecharacter.Character, error)
}

// WithCharacterNames configures the recipient lookup used by place_whisper.
func WithCharacterNames(lookup CharacterNameLookup) Option {
	return func(h *Handler) { h.characterNames = lookup }
}

// SpeechResult echoes the stored row text.
type SpeechResult struct {
	Text    string `json:"text"`
	Private bool   `json:"private"`
}

type recipientNotFoundError struct{ name string }

func (e recipientNotFoundError) Error() string { return "whisper recipient not found: " + e.name }

func speechRejection(err error) (int, ErrorDetail) {
	var missing recipientNotFoundError
	if errors.As(err, &missing) {
		return http.StatusNotFound, ErrorDetail{Code: "RECIPIENT_NOT_FOUND", Message: missing.name + "というプレイヤーが存在しません"}
	}
	return 0, ErrorDetail{}
}

type sayParams struct {
	Text string `json:"text"`
}

type whisperParams struct {
	RecipientName string `json:"recipient_name"`
	Text          string `json:"text"`
}

// registerSpeech connects place_say and place_whisper (party.cgi:20-26,
// lib/system.cgi:251-263) once the options have configured their
// dependencies. Text is validated before execution, so invalid input never
// writes a row.
func (h *Handler) registerSpeech() {
	if h.playerContext == nil || h.placeLog == nil {
		return
	}
	if h.actionCommands == nil {
		h.actionCommands = make(map[string]actionCommand)
	}
	sayDef, _ := playercontext.GetAction("place_say")
	h.actionCommands["place_say"] = actionCommand{reject: speechRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var p sayParams
		if err := decodeActionParams(raw, sayDef.RequiredParams, &p); err != nil {
			return nil, err
		}
		text, err := validation.ValidateLegacyComment(p.Text, maxSpeechLength)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, _, _ string, current playercontext.Result) (any, error) {
			return h.speak(ctx, current, text, "")
		}, nil
	}}
	if h.characterNames == nil {
		return
	}
	whisperDef, _ := playercontext.GetAction("place_whisper")
	h.actionCommands["place_whisper"] = actionCommand{reject: speechRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var p whisperParams
		if err := decodeActionParams(raw, whisperDef.RequiredParams, &p); err != nil {
			return nil, err
		}
		name, err := validation.ValidateCharacterName(p.RecipientName)
		if err != nil {
			return nil, err
		}
		row := "＠ささやき>" + name
		// Legacy logs a whisper without a body as the bare command; a
		// whitespace-only body still gets the character and length checks.
		text, err := validation.ValidateLegacyComment(p.Text, maxSpeechLength)
		switch {
		case errors.Is(err, validation.ErrEmpty):
			if err := validation.ValidateVerbatimLine(strings.NewReplacer("\r", "", "\n", "").Replace(p.Text), maxSpeechLength); err != nil {
				return nil, err
			}
		case err != nil:
			return nil, err
		default:
			row += " " + text
		}
		return func(ctx context.Context, _, _ string, current playercontext.Result) (any, error) {
			return h.speak(ctx, current, row, name)
		}, nil
	}}
}

// speak appends one character row to the current place. A nonempty recipient
// name makes it a whisper, visible only to the sender and the recipient. The
// row is the command's only effect, so an append failure is not swallowed.
func (h *Handler) speak(ctx context.Context, current playercontext.Result, text, recipientName string) (SpeechResult, error) {
	destination, char := current.Snapshot.LocationID, current.Snapshot.Character
	recipientID := ""
	if recipientName != "" {
		recipient, err := h.characterNames.FindByName(ctx, recipientName)
		if errors.Is(err, corecharacter.ErrNotFound) {
			return SpeechResult{}, recipientNotFoundError{name: recipientName}
		}
		if err != nil {
			return SpeechResult{}, err
		}
		recipientID = recipient.ID
	}
	row := placelog.Entry{OccurredAt: time.Now().UTC(), SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: char.ID,
		Name: char.Name, Color: char.Color, Text: text, RecipientCharacterID: recipientID}
	if err := h.placeLog.Append(ctx, placelog.Place{Kind: placelog.KindFacility, ID: destination}, row); err != nil {
		return SpeechResult{}, err
	}
	return SpeechResult{Text: row.Text, Private: recipientID != ""}, nil
}
