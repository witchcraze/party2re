package guild

import (
	"context"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

const GoldMedal = "gold_medal"

// Decoration is a display record, never a player membership or authority.
type Decoration struct {
	ID        string    `json:"id"`
	Kind      string    `json:"kind"`
	Icon      string    `json:"icon"`
	CreatedAt time.Time `json:"created_at"`
}

// MakeGorgeous grants the guild decoration from an admitted Heaven wish.
// Transaction: RunInTx (joins the wish transaction).
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) MakeGorgeous(ctx context.Context, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrCharacterNotFound
	}
	return s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, characterID); err != nil {
			return err
		}
		g, _, err := s.repo.GetGuildByCharacter(txCtx, characterID)
		if err != nil {
			return err
		}
		_, members, err := s.repo.GetGuildForUpdate(txCtx, g.ID)
		if err != nil {
			return err
		}
		for _, m := range members {
			if m.CharacterID == characterID && !m.IsPending {
				if err := s.repo.UpdateBgimg(txCtx, g.ID, "god.gif"); err != nil {
					return err
				}
				return s.repo.AddDecoration(txCtx, g.ID, Decoration{
					ID: id.New(), Kind: GoldMedal,
					Icon:      "assets/images/placeholders/guild/gold-medal.svg",
					CreatedAt: s.nowFunc().UTC(),
				})
			}
		}
		return ErrCharacterNotInGuild
	})
}
