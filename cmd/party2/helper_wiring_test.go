package main

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
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
	"guilds",
	"guild_members",
	"helper_quests",
	"character_monsters",
}

// helperWiringIsolatedDB creates a unique dedicated database holding only copied table
// definitions (no data). CompleteQuest scans and exchanges expired
// requests in helper_quests, so tests must not run against the shared test DB.
// Missing admin configuration fails the test rather than falling back to the shared DB.
func helperWiringIsolatedDB(t *testing.T) *sql.DB {
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
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated helper quest tests")
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

	name := fmt.Sprintf("party2_helper_wire_%06x_%s", time.Now().UnixNano()%0x1000000, id.New())
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

	copyTableDefinitions(t, source, db, helperWiringRequiredTables)
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
	db := helperWiringIsolatedDB(t)
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

	runID := fmt.Sprintf("%06x", time.Now().UnixNano()%0x1000000)

	// Create test guild with leader character in dedicated DB
	testGuild, leader, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("HWG_%s", runID), 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	// Create unaffiliated character in dedicated DB
	soloChar, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("HWS_%s", runID))
	if err != nil {
		t.Fatalf("CreateTestCharacter failed: %v", err)
	}

	// Setup depots with required turn-in materials (item-001 x 4) in dedicated DB
	leaderMaterial, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, leader.ID, 5, []item.Instance{leaderMaterial}); err != nil {
		t.Fatalf("CreateTestDepot for leader failed: %v", err)
	}

	soloMaterial, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, soloChar.ID, 5, []item.Instance{soloMaterial}); err != nil {
		t.Fatalf("CreateTestDepot for soloChar failed: %v", err)
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
	expired := guildQuest
	expired.ID = "expired-other"
	if err := questRepo.Save(ctx, expired); err != nil {
		t.Fatal(err)
	}
	boardBefore, err := questRepo.ListUncompleted(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Unaffiliated character attempts to complete guild-only quest -> rejected with ErrGuildRequired
	_, err = misc.helper.CompleteQuest(ctx, soloChar.ID, guildQuest.ID, now)
	if !errors.Is(err, helperquest.ErrGuildRequired) {
		t.Fatalf("expected ErrGuildRequired for unaffiliated character, got %v", err)
	}
	boardAfter, err := questRepo.ListUncompleted(ctx)
	if err != nil || !reflect.DeepEqual(boardBefore, boardAfter) {
		t.Fatalf("guild rejection changed board: %+v, %v", boardAfter, err)
	}

	// Verify solo character state unchanged in dedicated DB
	soloDepot, err := econ.depotRepo.FindByCharacterID(ctx, soloChar.ID)
	if err != nil {
		t.Fatalf("failed to find soloChar depot: %v", err)
	}
	if qty := soloDepot.Quantity("item-001"); qty != 4 {
		t.Errorf("expected soloChar materials intact (4), got %d", qty)
	}
	if soloDepot.Quantity("item-126") != 0 {
		t.Fatal("guild rejection granted a reward")
	}
	soloInDB, err := core.charRepo.FindByID(ctx, soloChar.ID)
	if err != nil {
		t.Fatalf("failed to find soloChar in DB: %v", err)
	}
	if soloInDB.HelpCount != 0 {
		t.Errorf("expected soloChar HelpCount 0, got %d", soloInDB.HelpCount)
	}

	// Verify guild points remain 0
	gCheck, _, err := soc.guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("failed to get guild: %v", err)
	}
	if gCheck.Points != 0 {
		t.Errorf("expected guild points 0, got %d", gCheck.Points)
	}

	// 2. Affiliated leader completes guild-only quest -> succeeds
	res, err := misc.helper.CompleteQuest(ctx, leader.ID, guildQuest.ID, now)
	if err != nil {
		t.Fatalf("CompleteQuest for leader failed: %v", err)
	}

	// Verify result struct
	if res.Character.HelpCount != 1 {
		t.Errorf("expected HelpCount 1 in result, got %d", res.Character.HelpCount)
	}
	if res.CompletedQuest.CompletedAt == nil {
		t.Error("expected CompletedAt to be set on completed quest")
	}
	if res.CompletedQuest.CompletedBy != leader.ID {
		t.Errorf("expected CompletedBy %s, got %s", leader.ID, res.CompletedQuest.CompletedBy)
	}
	if res.NewQuest == nil {
		t.Fatal("expected replacement quest in result, got nil")
	}
	if res.RenewedCount != 1 {
		t.Fatalf("renewed_count = %d, want 1", res.RenewedCount)
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

	// b. Leader HelpCount incremented in DB
	leaderInDB, err := core.charRepo.FindByID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find leader in DB: %v", err)
	}
	if leaderInDB.HelpCount != 1 {
		t.Errorf("expected leader HelpCount 1 in DB, got %d", leaderInDB.HelpCount)
	}

	// c. Leader Depot: turn-in items deducted (4 - 2 = 2), reward delivered (item-126 x 1)
	leaderDepot, err := econ.depotRepo.FindByCharacterID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find leader depot: %v", err)
	}
	if qty := leaderDepot.Quantity("item-001"); qty != 2 {
		t.Errorf("expected remaining materials 2, got %d", qty)
	}
	if qty := leaderDepot.Quantity("item-126"); qty != 1 {
		t.Errorf("expected reward item-126 quantity 1, got %d", qty)
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
	db := helperWiringIsolatedDB(t)
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
		helperquest.WithDepotRepository(econ.depotRepo),
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

func TestHelperProductionGuildWiring_ConcurrentClaimRace(t *testing.T) {
	db := helperWiringIsolatedDB(t)
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

	runID := fmt.Sprintf("%06x", time.Now().UnixNano()%0x1000000)

	// Create test guild with leader in dedicated DB
	testGuild, leader, err := database.CreateTestGuildWithLeader(ctx, db, fmt.Sprintf("HWG_RACE_%s", runID), 10000)
	if err != nil {
		t.Fatalf("CreateTestGuildWithLeader failed: %v", err)
	}

	// Add second guild member in dedicated DB
	member2, err := database.CreateTestCharacterWithFunds(ctx, db, fmt.Sprintf("HWM2_%s", runID), 10000)
	if err != nil {
		t.Fatalf("CreateTestCharacterWithFunds failed: %v", err)
	}
	m := guild.Member{
		GuildID:     testGuild.ID,
		CharacterID: member2.ID,
		Role:        guild.RoleMember,
		Title:       "Member",
		IsPending:   false,
		JoinedAt:    time.Now().UTC(),
	}
	if _, err := soc.guildRepo.AddMember(ctx, m); err != nil {
		t.Fatalf("AddMember failed: %v", err)
	}

	// Prepare depots: both have 4 items in dedicated DB
	matLeader, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, leader.ID, 5, []item.Instance{matLeader}); err != nil {
		t.Fatalf("CreateTestDepot for leader failed: %v", err)
	}
	matMember, err := item.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, db, member2.ID, 5, []item.Instance{matMember}); err != nil {
		t.Fatalf("CreateTestDepot for member2 failed: %v", err)
	}

	// Create a single shared guild quest in dedicated DB
	questRepo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Second)
	guildQuest := helperquest.Quest{
		ID:            id.New(),
		Title:         "ギルド専用依頼(Race)",
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

	var winnerRes helperquest.CompletionResult

	raceErrors := testutil.RunRace(
		func() error {
			r, err := misc.helper.CompleteQuest(ctx, leader.ID, guildQuest.ID, now)
			if err == nil {
				winnerRes = r
			}
			return err
		},
		func() error {
			r, err := misc.helper.CompleteQuest(ctx, member2.ID, guildQuest.ID, now)
			if err == nil {
				winnerRes = r
			}
			return err
		},
	)

	// Exactly one must succeed and one must fail with ErrQuestAlreadyDone
	var successCount, alreadyDoneCount int
	for _, rErr := range raceErrors {
		if rErr == nil {
			successCount++
		} else if errors.Is(rErr, helperquest.ErrQuestAlreadyDone) {
			alreadyDoneCount++
		} else {
			t.Fatalf("unexpected race error: %v", rErr)
		}
	}
	if successCount != 1 || alreadyDoneCount != 1 {
		t.Fatalf("expected 1 success and 1 ErrQuestAlreadyDone, got %d successes and %d alreadyDone", successCount, alreadyDoneCount)
	}

	if winnerRes.NewQuest == nil {
		t.Fatal("expected replacement quest for winner, got nil")
	}

	// Verify guild points: exactly 100 points awarded (not 200)
	gCheck, _, err := soc.guildRepo.GetGuild(ctx, testGuild.ID)
	if err != nil {
		t.Fatalf("failed to get guild: %v", err)
	}
	if gCheck.Points != 100 {
		t.Errorf("expected exactly 100 guild points after concurrent race, got %d", gCheck.Points)
	}

	// Verify asset conservation:
	// Winner had 2 items consumed (4 -> 2) and received 1 reward item.
	// Loser had 0 items consumed (still 4) and received 0 reward items.
	depotLeader, err := econ.depotRepo.FindByCharacterID(ctx, leader.ID)
	if err != nil {
		t.Fatalf("failed to find leader depot: %v", err)
	}
	depotMember, err := econ.depotRepo.FindByCharacterID(ctx, member2.ID)
	if err != nil {
		t.Fatalf("failed to find member2 depot: %v", err)
	}

	totalRemainingMaterials := depotLeader.Quantity("item-001") + depotMember.Quantity("item-001")
	if totalRemainingMaterials != 6 { // (4 - 2) + 4 = 6
		t.Errorf("expected 6 total remaining materials across both members, got %d", totalRemainingMaterials)
	}

	totalRewardItems := depotLeader.Quantity("item-126") + depotMember.Quantity("item-126")
	if totalRewardItems != 1 {
		t.Errorf("expected exactly 1 reward item across both members, got %d", totalRewardItems)
	}
}
