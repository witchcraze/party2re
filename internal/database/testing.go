package database

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/id"
)

// CreateTestPlayer creates and persists a player for testing purposes.
func CreateTestPlayer(ctx context.Context, db *sql.DB) (coreplayer.Player, error) {
	username := fmt.Sprintf("testuser_%s", id.New()[:12])
	p, err := coreplayer.New(username, "testpassword123", time.Now().UTC())
	if err != nil {
		return coreplayer.Player{}, err
	}
	repo, err := NewPlayerRepository(db)
	if err != nil {
		return coreplayer.Player{}, err
	}
	if err := repo.Save(ctx, p); err != nil {
		return coreplayer.Player{}, err
	}
	return p, nil
}

// CreateTestCharacter creates and persists a character owned by a newly created test player.
func CreateTestCharacter(ctx context.Context, db *sql.DB, name string) (corecharacter.Character, error) {
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		return corecharacter.Character{}, err
	}
	char, err := corecharacter.New(name)
	if err != nil {
		return corecharacter.Character{}, err
	}
	char.PlayerID = p.ID
	repo, err := NewCharacterRepository(db)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if err := repo.Save(ctx, char); err != nil {
		return corecharacter.Character{}, err
	}
	return char, nil
}
