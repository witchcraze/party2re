package http

import (
	"context"
	"encoding/json"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// AuctionService defines the live P2P auction send exposed through the Gateway.
type AuctionService interface {
	Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error)
}

// PlayerInspection is the common しらべる result for another player or the actor
// (lib/system.cgi shiraberu). Guild, gems, message and presence come from their
// owning features; gem IDs and names follow the custom skill's non-empty slots.
type PlayerInspection struct {
	auction.InspectResult
	GuildID       string                    `json:"guild_id"`
	GuildName     string                    `json:"guild_name"`
	Gems          []string                  `json:"gems"`
	GemNames      []string                  `json:"gem_names"`
	Location      *PlayerInspectionLocation `json:"location"`
	LoginMessage  string                    `json:"login_message"`
	Online        bool                      `json:"online"`
	LastUpdatedAt *time.Time                `json:"last_updated_at"`
}

type PlayerInspectionLocation struct {
	playercontext.PublicLocation
	Title string `json:"title"`
}

// PlayerInspector composes PlayerInspection at the composition root. An omitted
// or missing target inspects the actor, as legacy does.
type PlayerInspector interface {
	InspectPlayer(ctx context.Context, actorID, targetID, targetName string) (PlayerInspection, error)
}

type playerInspectParams struct {
	TargetCharacterID   string `json:"target_character_id,omitempty"`
	TargetCharacterName string `json:"target_character_name,omitempty"`
}

func (p *playerInspectParams) UnmarshalJSON(raw []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	present := make([]string, 0, len(fields))
	for name := range fields {
		present = append(present, name)
	}
	type params playerInspectParams
	return decodeActionParams(raw, present, (*params)(p))
}

// WithPlayerInspect connects the common player_inspect command.
func WithPlayerInspect(inspector PlayerInspector) Option {
	if inspector == nil {
		return withActionCommand[playerInspectParams]("player_inspect", nil, nil)
	}
	return func(h *Handler) {
		withActionCommand("player_inspect", func(ctx context.Context, actorID string, p playerInspectParams) (any, error) {
			result, err := inspector.InspectPlayer(ctx, actorID, p.TargetCharacterID, p.TargetCharacterName)
			if err != nil {
				return nil, err
			}
			if location := result.Location; location != nil {
				switch location.Kind {
				case "quest":
					location.Title = "クエスト中"
				case "home":
					location.Title = location.HomeOwnerName + "の家"
				case "scene":
					adapter, ok := h.sceneAdapters[location.SceneID]
					if !ok || adapter.title == "" {
						return nil, errSceneNotConfigured
					}
					location.Title = adapter.title
				default:
					return nil, errSceneNotConfigured
				}
			}
			return result, nil
		}, nil)(h)
	}
}
