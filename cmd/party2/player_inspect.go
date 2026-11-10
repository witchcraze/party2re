package main

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/auction"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/guild"
)

// playerInspectAdapter composes the common しらべる from its owning features.
// Every source is a required read: a failure fails the whole inspection.
type playerInspectAdapter struct {
	status interface {
		Inspect(ctx context.Context, inspectorID, targetID, targetName string) (auction.InspectResult, error)
	}
	guild interface {
		GetByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	}
	skills interface {
		GetCustomSkill(ctx context.Context, characterID string) (*customskill.CustomSkill, error)
	}
	presence interface {
		State(ctx context.Context, characterID string) (character.PresenceState, error)
	}
	now func() time.Time
}

func (a playerInspectAdapter) InspectPlayer(ctx context.Context, actorID, targetID, targetName string) (http.PlayerInspection, error) {
	status, err := a.status.Inspect(ctx, actorID, targetID, targetName)
	if err != nil {
		return http.PlayerInspection{}, err
	}
	out := http.PlayerInspection{InspectResult: status, Gems: []string{}}
	g, _, err := a.guild.GetByCharacter(ctx, status.CharacterID)
	switch {
	case err == nil:
		out.GuildID, out.GuildName = g.ID, g.Name
	case !errors.Is(err, guild.ErrCharacterNotInGuild):
		return http.PlayerInspection{}, err
	}
	skill, err := a.skills.GetCustomSkill(ctx, status.CharacterID)
	if err != nil {
		return http.PlayerInspection{}, err
	}
	if skill != nil {
		for _, gem := range skill.Gems {
			if gem != "" {
				out.Gems = append(out.Gems, gem)
			}
		}
	}
	state, err := a.presence.State(ctx, status.CharacterID)
	if err != nil {
		return http.PlayerInspection{}, err
	}
	out.LoginMessage = state.LoginMessage
	if !state.SampledAt.IsZero() {
		at := state.SampledAt.UTC()
		out.LastUpdatedAt = &at
		out.Online = a.now().Sub(at) <= character.PresenceWindowSeconds*time.Second
	}
	return out, nil
}
