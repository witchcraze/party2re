package http

import (
	"context"
	"encoding/json"
	"time"

	"github.com/witchcraze/party2re/internal/auction"
)

// AuctionService defines the live P2P auction send exposed through the Gateway.
type AuctionService interface {
	Send(ctx context.Context, req auction.SendRequest) (auction.SendResult, error)
}

// PlayerInspection is the common しらべる result for another player or the actor
// (lib/system.cgi shiraberu). Guild, gems, message and presence come from their
// owning features; gem IDs are the custom skill's non-empty slots in order.
type PlayerInspection struct {
	auction.InspectResult
	GuildID       string     `json:"guild_id"`
	GuildName     string     `json:"guild_name"`
	Gems          []string   `json:"gems"`
	LoginMessage  string     `json:"login_message"`
	Online        bool       `json:"online"`
	LastUpdatedAt *time.Time `json:"last_updated_at"`
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
	return withActionCommand("player_inspect", func(ctx context.Context, actorID string, p playerInspectParams) (any, error) {
		return inspector.InspectPlayer(ctx, actorID, p.TargetCharacterID, p.TargetCharacterName)
	}, nil)
}
