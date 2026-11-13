package http

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/witchcraze/party2re/internal/placelog"
)

// PlaceMembers is the per-place member list (docs/architecture/place-log.md#members).
type PlaceMembers interface {
	Refresh(ctx context.Context, place placelog.Place, member placelog.Member, now time.Time) error
	Leave(ctx context.Context, characterID string) error
	Read(ctx context.Context, place placelog.Place, now time.Time) ([]placelog.Member, error)
}

// PlaceMember is the public projection of a member: the facility NPC first,
// then characters. Character IDs stay server-side.
type PlaceMember struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Color      string `json:"color"`
	IconURL    string `json:"icon_url"`
	AuraEffect int    `json:"aura_effect"`
}

// WithPlaceMembers configures the member list refreshed by commands and
// observed by the scenes that keep a place log.
func WithPlaceMembers(members PlaceMembers) Option {
	return func(h *Handler) { h.placeMembers = members }
}

func (h *Handler) readPlaceMembers(ctx context.Context, destination string) ([]PlaceMember, error) {
	out := []PlaceMember{}
	if npc := h.sceneAdapters[destination].npc; npc != nil {
		out = append(out, PlaceMember{Kind: "npc", Name: npc.name, Color: npcColor})
	}
	if h.placeMembers == nil {
		return out, nil
	}
	rows, err := h.placeMembers.Read(ctx, placelog.Place{Kind: placelog.KindFacility, ID: destination}, time.Now())
	if err != nil {
		return nil, err
	}
	for _, m := range rows {
		out = append(out, PlaceMember{Kind: "character", Name: m.Name, Color: m.Color, IconURL: m.IconURL, AuraEffect: m.AuraEffect})
	}
	return out, nil
}

// refreshPlaceMember lists the actor in its current place before a command
// runs, as legacy read_member precedes the action (party.cgi:19-20).
func (h *Handler) refreshPlaceMember(ctx context.Context, characterID, location string) error {
	if h.placeMembers == nil || !h.sceneAdapters[location].placeLog {
		return nil
	}
	profile, err := h.characters.GetProfile(ctx, characterID)
	if err != nil {
		return err
	}
	return h.placeMembers.Refresh(ctx, placelog.Place{Kind: placelog.KindFacility, ID: location}, placelog.Member{
		CharacterID: characterID, Name: profile.Character.Name, Color: profile.Character.Color,
		IconURL: profile.Profile.AvatarURL, AuraEffect: profile.Profile.AuraEffect,
	}, time.Now())
}

// leavePlace removes the actor after it moved. The move has committed, so a
// failure is logged and the stale entry expires on its own.
func (h *Handler) leavePlace(ctx context.Context, characterID string) {
	if h.placeMembers == nil {
		return
	}
	if err := h.placeMembers.Leave(ctx, characterID); err != nil {
		slog.WarnContext(ctx, "place member not removed", slog.String("character_id", characterID), slog.String("error", err.Error()))
	}
}

// registerPresenceRefresh connects presence_refresh, the legacy page reload.
// The dispatcher's preflight member refresh is its whole effect.
func (h *Handler) registerPresenceRefresh() {
	if h.playerContext == nil || h.placeMembers == nil {
		return
	}
	if h.actionCommands == nil {
		h.actionCommands = make(map[string]actionCommand)
	}
	h.actionCommands["presence_refresh"] = actionCommand{prepare: func(raw json.RawMessage) (func(context.Context, string, string) (any, error), error) {
		var none struct{}
		if err := decodeActionParams(raw, nil, &none); err != nil {
			return nil, err
		}
		return func(context.Context, string, string) (any, error) { return struct{}{}, nil }, nil
	}}
}
