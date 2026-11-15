package http

import (
	"context"
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

func (h *Handler) scenePlace(current playercontext.Result) placelog.Place {
	destination := current.Snapshot.LocationID
	if resolve := h.sceneAdapters[destination].place; resolve != nil {
		return resolve(current)
	}
	return placelog.Place{Kind: placelog.KindFacility, ID: destination}
}

func (h *Handler) readPlaceLog(ctx context.Context, current playercontext.Result) ([]PlaceLogEntry, error) {
	out := []PlaceLogEntry{}
	if h.placeLog == nil {
		return out, nil
	}
	rows, err := h.placeLog.Read(ctx, h.scenePlace(current), current.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	for _, r := range rows {
		out = append(out, PlaceLogEntry{OccurredAt: r.OccurredAt.UTC(), SpeakerKind: string(r.SpeakerKind),
			Name: r.Name, Color: r.Color, Text: r.Text, Private: r.RecipientCharacterID != ""})
	}
	return out, nil
}
