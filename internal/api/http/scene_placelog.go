package http

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// PlaceLog reads a place's viewer-filtered log, newest first, and appends rows.
type PlaceLog interface {
	Read(ctx context.Context, place placelog.Place, viewerID string) ([]placelog.Entry, error)
	Append(ctx context.Context, place placelog.Place, entries ...placelog.Entry) error
}

// PlaceLogEntry is the public projection of a log row. Character IDs stay
// server-side; Private marks a whisper the viewer sent or received.
type PlaceLogEntry struct {
	OccurredAt  time.Time `json:"occurred_at"`
	SpeakerKind string    `json:"speaker_kind"`
	Name        string    `json:"name"`
	Color       string    `json:"color"`
	Text        string    `json:"text"`
	Private     bool      `json:"private"`
}

// WithPlaceLog configures the shared place log observed by facility scenes.
func WithPlaceLog(reader PlaceLog) Option {
	return func(h *Handler) { h.placeLog = reader }
}

// placeKindCasinoRoom is a casino room's own log (_casino.cgi:5), keyed by room ID.
const placeKindCasinoRoom = "casino_room"

func (h *Handler) scenePlace(ctx context.Context, current playercontext.Result) (placelog.Place, error) {
	destination := current.Snapshot.LocationID
	// The room activity shares the lobby's location ID but is its own place.
	if room, ok := activityFact(current, "casino"); ok {
		return placelog.Place{Kind: placeKindCasinoRoom, ID: room.ID}, nil
	}
	if resolve := h.sceneAdapters[destination].place; resolve != nil {
		return resolve(h, ctx, current)
	}
	return placelog.Place{Kind: placelog.KindFacility, ID: destination}, nil
}

func (h *Handler) readPlaceLog(ctx context.Context, current playercontext.Result) ([]PlaceLogEntry, error) {
	out := []PlaceLogEntry{}
	if h.placeLog == nil {
		return out, nil
	}
	place, err := h.scenePlace(ctx, current)
	if err != nil {
		return nil, err
	}
	rows, err := h.placeLog.Read(ctx, place, current.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, PlaceLogEntry{OccurredAt: r.OccurredAt.UTC(), SpeakerKind: string(r.SpeakerKind),
			Name: r.Name, Color: r.Color, Text: r.Text, Private: r.RecipientCharacterID != ""})
	}
	return out, nil
}

// appendKickPlaceLog posts the private legacy kick notice to the destination
// lobby place log (_skill.cgi:827-828, _casino.cgi:178); a lost row is display-only.
func (h *Handler) appendKickPlaceLog(ctx context.Context, place placelog.Place, roomName, leaderName, targetCharID string) {
	if h.placeLog == nil || roomName == "" || leaderName == "" {
		return
	}
	row := placelog.Entry{
		OccurredAt:           time.Now().UTC(),
		SpeakerKind:          placelog.SpeakerNPC,
		Name:                 "＠",
		Color:                npcColor,
		Text:                 fmt.Sprintf("%sのリーダー%sからきっくされました", roomName, leaderName),
		RecipientCharacterID: targetCharID,
	}
	if err := h.placeLog.Append(ctx, place, row); err != nil {
		slog.WarnContext(ctx, "place log append failed", slog.String("place", place.Kind+":"+place.ID), slog.String("error", err.Error()))
	}
}

// activityFact returns the current activity when it is of the given kind.
func activityFact(current playercontext.Result, kind string) (playercontext.Activity, bool) {
	if current.Snapshot.LocationID != kind {
		return playercontext.Activity{}, false
	}
	for _, a := range current.Snapshot.Activities {
		if a.Kind == kind {
			return a, true
		}
	}
	return playercontext.Activity{}, false
}
