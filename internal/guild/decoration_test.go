package guild_test

import (
	"context"

	"github.com/witchcraze/party2re/internal/guild"
)

func (m *mockGuildRepo) AddDecoration(context.Context, string, guild.Decoration) error {
	return nil
}

func (m *mockGuildRepo) ListDecorations(context.Context, string) ([]guild.Decoration, error) {
	return []guild.Decoration{}, nil
}
