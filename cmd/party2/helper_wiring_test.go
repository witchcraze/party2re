package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/testutil"
)

var helperWiringRequiredTables = []string{
	"players",
	"characters",
	"inventory_items",
	"character_depots",
	"depot_items",
	"normal_adventure_runs",
	"normal_adventure_enrollments",
	"normal_adventure_recipients",
	"guilds",
	"guild_members",
	"character_memories",
	"helper_quests",
	"character_monsters",
}

// wiringIsolatedDB copies only table definitions for operations that scan shared state.
func wiringIsolatedDB(t *testing.T, tables []string) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})

	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated wiring tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})

	name := fmt.Sprintf("party2_wire_%06x_%s", time.Now().UnixNano()%0x1000000, id.New())
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error(err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})

	copyTableDefinitions(t, source, db, tables)
	return db
}

func copyTableDefinitions(t *testing.T, source, db *sql.DB, tables []string) {
	t.Helper()
	ctx := context.Background()
	conn, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
			t.Errorf("restore foreign key checks: %v", err)
			_ = conn.Raw(func(any) error { return driver.ErrBadConn }) //lint:ignore error-swallow marks connection bad so the pool discards it
		}
		if err := conn.Close(); err != nil {
			t.Error(err)
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, ddl); err != nil {
			t.Fatalf("copy %s: %v", table, err)
		}
	}
}

func TestHelperProductionGuildWiring(t *testing.T) {
	db := wiringIsolatedDB(t, helperWiringRequiredTables)
	ctx := context.Background()

	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Both completion replacement and expired-row exchange must use production catalog wiring.
	misc.helper.SetRandomSource(helperRareMonsterRandom{})

	runID := fmt.Sprintf("%06x", time.Now().UnixNano()%0x1000000)

	// Create test guild with leader character in dedicated DB
	testGuild, leader, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("HWG_%s", runID), 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	// Setup depots with required turn-in materials (item-001 x 4) in dedicated DB
	leaderMaterial, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, leader.ID, 5, []item.Instance{leaderMaterial}); err != nil {
		t.Fatalf("CreateTestDepot for leader failed: %v", err)
	}

	// Create a guild-only quest in dedicated DB
	questRepo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	guildQuest := helperquest.Quest{
		ID:            id.New(),
		Title:         "ギルド専用依頼",
		Kind:          helperquest.KindItem,
		TargetID:      "item-001",
		TargetName:    "薬草",
		RequiredCount: 2,
		RewardItemID:  "item-126", // 幸福袋
		IsRare:        false,
		IsGuild:       true,
		CreatedAt:     now,
		ExpiresAt:     now.Add(-time.Hour),
	}
	if err := questRepo.Save(ctx, guildQuest); err != nil {
		t.Fatalf("failed to save guildQuest: %v", err)
	}
	// 2. Affiliated leader completes guild-only quest -> succeeds
	res, err := misc.helper.CompleteQuest(ctx, leader.ID, guildQuest.ID, now)
	if err != nil {
		t.Fatalf("CompleteQuest for leader failed: %v", err)
	}

	if res.NewQuest == nil {
		t.Fatal("production catalog did not generate a replacement")
	}
	if res.NewQuest.TargetID != "monster-165" || res.NewQuest.TargetName == "" || !res.NewQuest.IsRare {
		t.Fatalf("replacement did not use native rare-monster fallback: %+v", res.NewQuest)
	}
	// Verify database persistence in dedicated DB:
	// a. Guild points incremented by 100 (GP100)
	gUpdated, _, err := soc.guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("failed to get updated guild: %v", err)
	}
	if gUpdated.Points != 100 {
		t.Errorf("expected guild points 100, got %d", gUpdated.Points)
	}

}

type helperRareMonsterRandom struct{}

func (helperRareMonsterRandom) Intn(max int) (int, error) {
	switch max {
	case 4:
		return 3, nil // Monster quest.
	case 14:
		return 0, nil // Rare.
	case 15:
		return 1, nil // Nonguild.
	case 80:
		return 79, nil // Missing monster-579.
	case 6:
		return 5, nil // Fallback monster-165 and last rare reward slot.
	default:
		return 0, nil
	}
}

type faultyGuildPointAdapter struct {
	inner helperquest.GuildRepository
}

func (f *faultyGuildPointAdapter) FindGuildIDByCharacterID(ctx context.Context, characterID string) (string, error) {
	return f.inner.FindGuildIDByCharacterID(ctx, characterID)
}

func (f *faultyGuildPointAdapter) AddGuildPoints(ctx context.Context, guildID string, points int) error {
	if err := f.inner.AddGuildPoints(ctx, guildID, points); err != nil {
		return err
	}
	return errors.New("simulated guild point database failure")
}

func TestHelperProductionGuildWiring_GPUpdateErrorRollsBack(t *testing.T) {
	db := wiringIsolatedDB(t, helperWiringRequiredTables)
	ctx := context.Background()

	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}

	runID := fmt.Sprintf("%06x", time.Now().UnixNano()%0x1000000)
	testGuild, leader, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("HWG_FAIL_%s", runID), 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	mat, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, leader.ID, 5, []item.Instance{mat}); err != nil {
		t.Fatalf("CreateTestDepot failed: %v", err)
	}

	questRepo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	guildQuest := helperquest.Quest{
		ID:            id.New(),
		Title:         "ギルド専用依頼(失敗検証)",
		Kind:          helperquest.KindItem,
		TargetID:      "item-001",
		TargetName:    "薬草",
		RequiredCount: 2,
		RewardItemID:  "item-126",
		IsGuild:       true,
		CreatedAt:     now,
		ExpiresAt:     now.Add(24 * time.Hour),
	}
	if err := questRepo.Save(ctx, guildQuest); err != nil {
		t.Fatalf("failed to save guildQuest: %v", err)
	}

	monsterRepo, err := database.NewMonsterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	prodGuildAdapter := &helperGuildAdapter{guild: soc.guild}
	faultyAdapter := &faultyGuildPointAdapter{inner: prodGuildAdapter}

	helperService := helperquest.NewService(
		questRepo,
		core.charRepo,
		faultyAdapter,
		core.txProvider,
		helperquest.WithMonsterLookup(testutil.MonsterNameLookup(t)), helperquest.WithDepotRepository(econ.depotRepo),
		helperquest.WithFarmRepository(&helperFarmAdapter{repo: monsterRepo}),
	)

	_, err = helperService.CompleteQuest(ctx, leader.ID, guildQuest.ID, now)
	if err == nil {
		t.Fatal("expected error when AddGuildPoints fails, got nil")
	}

	// Verify complete rollback in dedicated DB:
	// 1. Quest completed_at is still nil
	qInDB, err := questRepo.FindByIDForUpdate(ctx, guildQuest.ID)
	if err != nil {
		t.Fatalf("failed to load quest: %v", err)
	}
	if qInDB.CompletedAt != nil {
		t.Errorf("expected CompletedAt to be nil after rollback, got %v", qInDB.CompletedAt)
	}

	// 2. Character HelpCount remains 0
	leaderInDB, err := core.charRepo.FindByID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find leader in DB: %v", err)
	}
	if leaderInDB.HelpCount != 0 {
		t.Errorf("expected leader HelpCount 0 after rollback, got %d", leaderInDB.HelpCount)
	}

	// 3. Materials in depot remain intact (4)
	leaderDepot, err := econ.depotRepo.FindByCharacterID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find leader depot: %v", err)
	}
	if qty := leaderDepot.Quantity("item-001"); qty != 4 {
		t.Errorf("expected materials intact (4) after rollback, got %d", qty)
	}
	if qty := leaderDepot.Quantity("item-126"); qty != 0 {
		t.Errorf("expected no reward in depot after rollback, got %d", qty)
	}

	// 4. Guild points remain 0
	gCheck, _, err := soc.guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("failed to get guild: %v", err)
	}
	if gCheck.Points != 0 {
		t.Errorf("expected guild points 0 after rollback, got %d", gCheck.Points)
	}

}
