package http

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
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
	if errors.Is(err, casino.ErrRoomNotFound) || errors.Is(err, casino.ErrMemberNotFound) {
		return casinoActionRejection(err)
	}
	if errors.Is(err, pvp.ErrCharacterNotInRoom) {
		return pvpLeaveRejection(err)
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
	whisperVerb := legacyVerb("place_whisper")
	h.actionCommands["place_whisper"] = actionCommand{reject: speechRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var p whisperParams
		if err := decodeActionParams(raw, whisperDef.RequiredParams, &p); err != nil {
			return nil, err
		}
		name, row, err := whisperRow(whisperVerb, p.RecipientName, p.Text)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, _, _ string, current playercontext.Result) (any, error) {
			return h.speak(ctx, current, row, name)
		}, nil
	}}
	h.registerInvite("pvp_invite", placelog.Place{Kind: placelog.KindFacility, ID: "quest"}, pvp.ErrCharacterNotInRoom)
	h.registerInvite("casino_room_invite", placelog.Place{Kind: placelog.KindFacility, ID: "casino"}, casino.ErrMemberNotFound)
}

// whisperRow validates a whisper and renders its row as the legacy command
// line. Legacy logs a whisper without a body as the bare command; a
// whitespace-only body still gets the character and length checks.
func whisperRow(verb, recipientName, body string) (name, row string, err error) {
	if name, err = validation.ValidateCharacterName(recipientName); err != nil {
		return "", "", err
	}
	row = verb + ">" + name
	text, err := validation.ValidateLegacyComment(body, maxSpeechLength)
	switch {
	case errors.Is(err, validation.ErrEmpty):
		if err := validation.ValidateVerbatimLine(validation.StripLegacyText(body), maxSpeechLength); err != nil {
			return "", "", err
		}
	case err != nil:
		return "", "", err
	default:
		row += " " + text
	}
	return name, row, nil
}

type inviteParams struct {
	RoomID        string `json:"room_id"`
	RecipientName string `json:"recipient_name"`
	Text          string `json:"text"`
}

// InviteCandidates answers ＠さそう without a recipient: the characters
// currently in the lobby, as legacy lists them instead of writing.
type InviteCandidates struct {
	Message    string        `json:"message"`
	Candidates []PlaceMember `json:"candidates"`
}

// registerInvite connects a room's ＠さそう (_skill.cgi:769-791,
// _casino.cgi:122-145): a whisper written to the lobby's log instead of the
// room's, so the room is neither renewed nor touched. Eligibility (members
// before round 1) is the room activity's; notInRoom rejects a stale room ID.
func (h *Handler) registerInvite(action string, lobby placelog.Place, notInRoom error) {
	def, _ := playercontext.GetAction(action)
	verb := legacyVerb(action)
	h.actionCommands[action] = actionCommand{reject: speechRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var p inviteParams
		if err := decodeActionParams(raw, def.RequiredParams, &p); err != nil {
			return nil, err
		}
		name, row := "", ""
		if p.RecipientName != "" {
			var err error
			if name, row, err = whisperRow(verb, p.RecipientName, p.Text); err != nil {
				return nil, err
			}
		}
		return func(ctx context.Context, _, _ string, current playercontext.Result) (any, error) {
			if room, ok := activityFact(current, def.ActivityKind); !ok || room.ID != p.RoomID {
				return nil, notInRoom
			}
			if name != "" {
				return h.writeSpeech(ctx, current, lobby, row, name)
			}
			out := InviteCandidates{Message: "誰をさそいますか？", Candidates: []PlaceMember{}}
			if h.placeMembers == nil {
				return out, nil
			}
			rows, err := h.placeMembers.Read(ctx, lobby, time.Now())
			if err != nil {
				return nil, err
			}
			for _, m := range rows {
				out.Candidates = append(out.Candidates, PlaceMember{Kind: "character", Name: m.Name, Color: m.Color, IconURL: m.IconURL, AuraEffect: m.AuraEffect})
			}
			return out, nil
		}, nil
	}}
}

// speak appends one character row to the current place. A nonempty recipient
// name makes it a whisper, visible only to the sender and the recipient. The
// row is the command's only effect, so an append failure is not swallowed.
func (h *Handler) speak(ctx context.Context, current playercontext.Result, text, recipientName string) (SpeechResult, error) {
	place, err := h.scenePlace(ctx, current)
	if err != nil {
		return SpeechResult{}, err
	}
	return h.writeSpeech(ctx, current, place, text, recipientName)
}

// writeSpeech appends the actor's row to place, resolving the recipient first.
func (h *Handler) writeSpeech(ctx context.Context, current playercontext.Result, place placelog.Place, text, recipientName string) (SpeechResult, error) {
	char := current.Snapshot.Character
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
	// Writing a room's log keeps the room alive (casino.cgi:124-129); the
	// renewal also rejects a speaker whose room ended or who left it.
	if place.Kind == placeKindCasinoRoom {
		if err := h.casino.RenewRoom(ctx, place.ID, char.ID); err != nil {
			return SpeechResult{}, err
		}
	}
	if err := h.placeLog.Append(ctx, place, row); err != nil {
		return SpeechResult{}, err
	}
	return SpeechResult{Text: row.Text, Private: recipientID != ""}, nil
}
