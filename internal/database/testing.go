package database

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

// CreateTestPlayer creates and persists a player for testing purposes.
func CreateTestPlayer(ctx context.Context, db *sql.DB) (coreplayer.Player, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return coreplayer.Player{}, err
	}
	username := fmt.Sprintf("testuser_%s", hex.EncodeToString(buf))
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
