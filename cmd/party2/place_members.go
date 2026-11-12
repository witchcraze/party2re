package main

import (
	"context"
	"log/slog"

	"github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// placeMemberLogout removes the account's characters from their place member
// lists on logout, as legacy ろぐあうと calls leave_member (lib/system.cgi:944-975).
// The member list is display context, so a failed removal only logs: the
// entry expires on its own and logout still proceeds.
type placeMemberLogout struct {
	http.PlayerService
	characters interface {
		FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error)
	}
	members interface {
		Leave(ctx context.Context, characterID string) error
	}
}

func (s placeMemberLogout) Logout(ctx context.Context, sessionID string) error {
	if p, err := s.Authenticate(ctx, sessionID); err == nil {
		chars, err := s.characters.FindByPlayerID(ctx, p.ID)
		if err != nil {
			slog.WarnContext(ctx, "place members not removed on logout", slog.String("error", err.Error()))
		}
		for _, c := range chars {
			if err := s.members.Leave(ctx, c.ID); err != nil {
				slog.WarnContext(ctx, "place member not removed on logout", slog.String("character_id", c.ID), slog.String("error", err.Error()))
			}
		}
	}
	return s.PlayerService.Logout(ctx, sessionID)
}
