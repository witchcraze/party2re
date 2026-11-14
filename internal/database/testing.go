package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Re-export concurrency stress testing harness from testutil for database package tests.
type ConcurrencyStressConfig = testutil.ConcurrencyStressConfig
type ConcurrencyStressResult = testutil.ConcurrencyStressResult

var (
	GetStressConfig         = testutil.GetStressConfig
	RunConcurrentStressTest = testutil.RunConcurrentStressTest
	RunRace                 = testutil.RunRace
	RunRace2                = testutil.RunRace2
	IsDeadlockError         = testutil.IsDeadlockError
	isDeadlockError         = testutil.IsDeadlockError
)

// getStressConfig provides compatibility for tests expecting (workers, opsPerWorker) tuple.
func getStressConfig() (int, int) {
	cfg := GetStressConfig()
	return cfg.Workers, cfg.OpsPerWorker
}

// sanitizeTestName ensures the name obeys domain length and character constraints (<= maxLen runes, no whitespace).
// If empty or whitespace-only, a default name with a unique suffix is generated.
// If overly long, it is safely truncated to maxLen Unicode code points (runes).
func sanitizeTestName(name string, maxLen int, defaultPrefix string) string {
	var b strings.Builder
	for _, r := range name {
		if !unicode.IsSpace(r) && r != '\u3000' {
			b.WriteRune(r)
		}
	}
	cleaned := b.String()
	if cleaned == "" {
		suffix := id.New()
		suffixRunes := []rune(suffix)
		if len(suffixRunes) > 8 {
			suffixRunes = suffixRunes[:8]
		}
		prefixRunes := []rune(defaultPrefix)
		if len(prefixRunes)+1+len(suffixRunes) > maxLen {
			prefixLen := maxLen - (1 + len(suffixRunes))
			if prefixLen < 1 {
				if len(suffixRunes) > maxLen {
					return string(suffixRunes[:maxLen])
				}
				return string(suffixRunes)
			}
			prefixRunes = prefixRunes[:prefixLen]
		}
		return fmt.Sprintf("%s_%s", string(prefixRunes), string(suffixRunes))
	}
	runes := []rune(cleaned)
	if len(runes) > maxLen {
		return string(runes[:maxLen])
	}
	return cleaned
}

// testPlayerPassword is the password of every factory-created player.
const testPlayerPassword = "testpassword123"

// testPlayerTemplate hashes testPlayerPassword once: bcrypt at production cost dominated fixture setup.
var testPlayerTemplate = sync.OnceValues(func() (coreplayer.Player, error) {
	return coreplayer.New("template", testPlayerPassword, time.Now())
})

// CreateTestPlayer creates and persists a player for testing purposes.
func CreateTestPlayer(ctx context.Context, db *sql.DB) (coreplayer.Player, error) {
	username := fmt.Sprintf("tp_%s", id.New()[:12])
	return CreateTestPlayerWithUsername(ctx, db, username)
}

// CreateTestPlayerWithUsername creates and persists a player with a designated username.
// Domain constraints (max length 32 chars, valid password hashing, UTC timestamp) are enforced.
func CreateTestPlayerWithUsername(ctx context.Context, db *sql.DB, username string) (coreplayer.Player, error) {
	p, err := testPlayerTemplate()
	if err != nil {
		return coreplayer.Player{}, err
	}
	now := time.Now().UTC()
	p.ID, p.Username, p.CreatedAt, p.UpdatedAt = id.New(), sanitizeTestName(username, 32, "tp"), now, now
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
	return CreateTestCharacterWithFunds(ctx, db, name, corecharacter.InitialMoney)
}

// CreateTestCharacterWithFunds creates and persists a character with specified initial funds.
// Automatically ensures the name adheres to the <= 8 char constraint.
// If the sanitized label is taken, retries with a fresh eight-character name.
func CreateTestCharacterWithFunds(ctx context.Context, db *sql.DB, name string, money int) (corecharacter.Character, error) {
	repo, err := NewCharacterRepository(db)
	if err != nil {
		return corecharacter.Character{}, err
	}
	var char corecharacter.Character
	err = RunInTx(ctx, db, func(txCtx context.Context) error {
		p, err := CreateTestPlayer(txCtx, db)
		if err != nil {
			return err
		}
		safeName := sanitizeTestName(name, corecharacter.MaxNameLength, "Char")
		char, err = corecharacter.New(safeName)
		if err != nil {
			return err
		}
		char.PlayerID = p.ID
		char.Money = money
		for attempt := 0; attempt < 3; attempt++ {
			err = repo.Save(txCtx, char)
			if err == nil {
				return nil
			}
			if !errors.Is(err, character.ErrNameAlreadyTaken) {
				return err
			}
			char.Name = id.New()[:corecharacter.MaxNameLength]
		}
		return err
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return char, nil
}

// CreateTestGuildWithLeader creates and persists a guild with an associated leader character.
func CreateTestGuildWithLeader(ctx context.Context, db *sql.DB, guildName string, leaderMoney int) (guild.Guild, corecharacter.Character, error) {
	safeGuildName := sanitizeTestName(guildName, guild.MaxNameLength, "Gld")
	leaderName := sanitizeTestName(safeGuildName+"_Ldr", corecharacter.MaxNameLength, "Ldr")
	leaderChar, err := CreateTestCharacterWithFunds(ctx, db, leaderName, leaderMoney)
	if err != nil {
		return guild.Guild{}, corecharacter.Character{}, err
	}

	now := time.Now().UTC()
	testGuild := guild.Guild{
		ID:                id.New(),
		Name:              safeGuildName,
		LeaderCharacterID: leaderChar.ID,
		Points:            0,
		Notice:            "Test Guild Notice",
		Color:             guild.DefaultColor,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	creatorMember := guild.Member{
		GuildID:     testGuild.ID,
		CharacterID: leaderChar.ID,
		Role:        guild.RoleLeader,
		Title:       guild.DefaultTitleLeader,
		JoinedAt:    now,
	}

	guildRepo, err := NewGuildRepository(db)
	if err != nil {
		return guild.Guild{}, corecharacter.Character{}, err
	}
	createdGuild, _, _, err := guildRepo.CreateGuild(ctx, testGuild, creatorMember, 0)
	if err != nil {
		return guild.Guild{}, corecharacter.Character{}, err
	}
	return createdGuild, leaderChar, nil
}

// CreateTestInventoryWithItems creates and persists an inventory populated with items for a character.
func CreateTestInventoryWithItems(ctx context.Context, db *sql.DB, charID string, items []coreitem.Instance) (coreinventory.Inventory, error) {
	inv, err := coreinventory.New(charID)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	for _, it := range items {
		if err := inv.Add(it); err != nil {
			return coreinventory.Inventory{}, err
		}
	}
	invRepo, err := NewInventoryRepository(db)
	if err != nil {
		return coreinventory.Inventory{}, err
	}
	if err := invRepo.Save(ctx, inv); err != nil {
		return coreinventory.Inventory{}, err
	}
	return inv, nil
}

// CreateTestDepot creates and persists a depot with initial exDepot and items for a character.
func CreateTestDepot(ctx context.Context, db *sql.DB, charID string, exDepot int, items []coreitem.Instance) (depot.Depot, error) {
	dep, err := depot.NewDepot(charID)
	if err != nil {
		return depot.Depot{}, err
	}
	dep.ExDepot = exDepot
	dep.Capacity = depot.CalculateCapacity(0, exDepot, 0)
	for _, it := range items {
		if err := dep.AddItem(it); err != nil {
			return depot.Depot{}, err
		}
	}
	depotRepo, err := NewDepotRepository(db)
	if err != nil {
		return depot.Depot{}, err
	}
	if err := depotRepo.Save(ctx, dep); err != nil {
		return depot.Depot{}, err
	}
	return dep, nil
}
