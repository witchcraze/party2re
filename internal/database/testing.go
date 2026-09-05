package database

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

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

// sanitizeTestName ensures the name obeys domain length constraints (<= maxLen).
// If empty, a default name with a unique suffix is generated.
// If overly long, it is safely truncated to maxLen.
func sanitizeTestName(name string, maxLen int, defaultPrefix string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		suffix := id.New()[:8]
		if len(defaultPrefix)+1+len(suffix) > maxLen {
			prefixLen := maxLen - (1 + len(suffix))
			if prefixLen < 1 {
				return suffix[:maxLen]
			}
			defaultPrefix = defaultPrefix[:prefixLen]
		}
		return fmt.Sprintf("%s_%s", defaultPrefix, suffix)
	}
	if len(name) > maxLen {
		return name[:maxLen]
	}
	return name
}

// CreateTestPlayer creates and persists a player for testing purposes.
func CreateTestPlayer(ctx context.Context, db *sql.DB) (coreplayer.Player, error) {
	username := fmt.Sprintf("tp_%s", id.New()[:12])
	return CreateTestPlayerWithUsername(ctx, db, username)
}

// CreateTestPlayerWithUsername creates and persists a player with a designated username.
// Domain constraints (max length 32 chars, valid password hashing, UTC timestamp) are enforced.
func CreateTestPlayerWithUsername(ctx context.Context, db *sql.DB, username string) (coreplayer.Player, error) {
	safeUsername := sanitizeTestName(username, 32, "tp")
	p, err := coreplayer.New(safeUsername, "testpassword123", time.Now().UTC())
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
	return CreateTestCharacterWithFunds(ctx, db, name, corecharacter.InitialMoney)
}

// CreateTestCharacterWithFunds creates and persists a character with specified initial funds.
// Automatically ensures the name adheres to the <= 32 char constraint.
func CreateTestCharacterWithFunds(ctx context.Context, db *sql.DB, name string, money int) (corecharacter.Character, error) {
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		return corecharacter.Character{}, err
	}
	safeName := sanitizeTestName(name, 32, "Char")
	char, err := corecharacter.New(safeName)
	if err != nil {
		return corecharacter.Character{}, err
	}
	char.PlayerID = p.ID
	char.Money = money
	repo, err := NewCharacterRepository(db)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if err := repo.Save(ctx, char); err != nil {
		return corecharacter.Character{}, err
	}
	return char, nil
}

// CreateTestGuildWithLeader creates and persists a guild with an associated leader character.
func CreateTestGuildWithLeader(ctx context.Context, db *sql.DB, guildName string, leaderMoney int) (guild.Guild, corecharacter.Character, error) {
	safeGuildName := sanitizeTestName(guildName, guild.MaxNameLength, "Gld")
	leaderName := sanitizeTestName(safeGuildName+"_Ldr", 32, "Ldr")
	leaderChar, err := CreateTestCharacterWithFunds(ctx, db, leaderName, leaderMoney)
	if err != nil {
		return guild.Guild{}, corecharacter.Character{}, err
	}

	now := time.Now().UTC()
	testGuild := guild.Guild{
		ID:                id.New(),
		Name:              safeGuildName,
		LeaderCharacterID: leaderChar.ID,
		Level:             1,
		Exp:               0,
		Gold:              0,
		Notice:            "Test Guild Notice",
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	creatorMember := guild.Member{
		GuildID:          testGuild.ID,
		CharacterID:      leaderChar.ID,
		Role:             guild.RoleLeader,
		JoinedAt:         now,
		TotalDonatedGold: 0,
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

// CreateTestDepot creates and persists a depot with initial gold and items for a character.
func CreateTestDepot(ctx context.Context, db *sql.DB, charID string, gold int, items []coreitem.Instance) (depot.Depot, error) {
	dep, err := depot.NewDepot(charID)
	if err != nil {
		return depot.Depot{}, err
	}
	dep.Gold = gold
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
