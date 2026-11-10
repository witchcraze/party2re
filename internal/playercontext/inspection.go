package playercontext

import (
	"context"
	"fmt"
)

// PublicLocation contains only the coarse whereabouts visible to other players.
type PublicLocation struct {
	Kind                 string `json:"kind"`
	SceneID              string `json:"scene_id,omitempty"`
	HomeOwnerCharacterID string `json:"home_owner_character_id,omitempty"`
	HomeOwnerName        string `json:"home_owner_name,omitempty"`
}

// PublicActivityReader reveals admission only, without an owner identity or run details.
type PublicActivityReader func(context.Context, string) (bool, error)

func WithPublicActivities(read PublicActivityReader) func(*Service) {
	return func(s *Service) { s.publicActivities = read }
}

// InspectLocation never queries owned context, validates selected products, or writes.
func (s *Service) InspectLocation(ctx context.Context, characterID string) (PublicLocation, error) {
	if err := ctx.Err(); err != nil {
		return PublicLocation{}, err
	}
	if s.publicActivities == nil || s.navigation == nil {
		return PublicLocation{}, ErrNavigationNotConfigured
	}
	active, err := s.publicActivities(ctx, characterID)
	if err != nil {
		return PublicLocation{}, fmt.Errorf("read public activity: %w", err)
	}
	if active {
		return PublicLocation{Kind: "quest"}, nil
	}
	n, err := s.selection(ctx, characterID)
	if err != nil {
		return PublicLocation{}, err
	}
	definition, ok := s.scenes[n.Destination]
	if !ok || (n.Subject != (Subject{}) && n.Subject.Kind != definition.SubjectKind) {
		return PublicLocation{}, ErrInvalidSelection
	}
	if n.Destination != "home" && n.Destination != "home_inbox" && n.Destination != "home_outbox" {
		return PublicLocation{Kind: "scene", SceneID: n.Destination}, nil
	}
	ownerID := characterID
	if n.Destination == "home" && n.Subject.Kind == "home" {
		ownerID = n.Subject.ID
	}
	owner, err := s.characters.FindByID(ctx, ownerID)
	if err != nil {
		return PublicLocation{}, fmt.Errorf("read home owner: %w", err)
	}
	return PublicLocation{Kind: "home", HomeOwnerCharacterID: owner.ID, HomeOwnerName: owner.Name}, nil
}
