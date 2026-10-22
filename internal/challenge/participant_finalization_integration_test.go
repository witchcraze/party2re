package challenge_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// The failure trigger must never change another test's database behavior.
func participantSettlementDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated participant settlement tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	})
	admin, err := database.Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	ctx := context.Background()
	name := "party2_challenge_test_" + id.New()
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
	for _, table := range []string{"players", "characters", "inventory_items", "equipment_slots", "character_depots", "depot_items", "challenge_sessions", "character_challenge_records", "challenge_run_transitions", "challenge_participants", "challenge_character_claims", "challenge_command_results", "tavern_character_status", "challenge_hall_of_fame"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestChallengeParticipantFinalizationIntegration(t *testing.T) {
	db := participantSettlementDB(t)
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewChallengeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []string{"claimed", "defeated", "member update rollback", "equipment read rollback", "concurrent"} {
		t.Run(scenario, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			original := make([]corecharacter.Character, 2)
			for i := range original {
				c, err := database.CreateTestCharacter(ctx, db, "Member "+id.New()[:8])
				if err != nil {
					t.Fatal(err)
				}
				c.Level, c.Experience = 30, 5000
				c.Stats.HP, c.Stats.MaxHP, c.Stats.MP, c.Stats.MaxMP = 100, 100, 40, 40
				if err := chars.Update(ctx, c); err != nil {
					t.Fatal(err)
				}
				original[i] = c
			}
			slices.SortFunc(original, func(a, b corecharacter.Character) int { return strings.Compare(a.ID, b.ID) })
			run := sampleChallengeSession(id.New(), original[0].ID, "1")
			run.CurrentRound = 3
			for _, c := range original {
				mp := 5
				run.Members = append(run.Members, challenge.ChallengeMember{CharacterID: c.ID,
					CharacterCurrentHP: 30, CharacterCurrentMP: &mp, MaxHP: 100, MaxMP: 40,
					AccumulatedExp: 7, AccumulatedGold: 23,
					ConsumedItems: []corebattle.ConsumedItem{{ID: "item-193", Quantity: 1}}})
				instID := id.New()
				if _, err := db.ExecContext(ctx, "INSERT INTO inventory_items (id,character_id,definition_id,quantity,enhancement_level) VALUES (?,?,?,1,0)", instID, c.ID, "item-193"); err != nil {
					t.Fatal(err)
				}
				if _, err := db.ExecContext(ctx, "INSERT INTO equipment_slots (character_id,slot,instance_id) VALUES (?,'accessory',?)", c.ID, instID); err != nil {
					t.Fatal(err)
				}
			}
			if err := repo.SaveSession(ctx, run); err != nil {
				t.Fatal(err)
			}
			run.Status = challenge.StatusClaimed
			if scenario == "defeated" {
				run.Status = challenge.StatusDefeated
				for i := range run.Members {
					run.Members[i].CharacterCurrentHP = 0
				}
			}
			finalize := func() error { return repo.FinalizeSession(ctx, run, 99, 99, []string{"item-001"}, 2) }
			if scenario == "claimed" {
				for _, memberID := range []string{id.New(), run.Members[0].CharacterID} {
					forged := run
					forged.Members = slices.Clone(run.Members)
					forged.Members[1].CharacterID = memberID
					if err := repo.FinalizeSession(ctx, forged, 99, 99, nil, 2); !errors.Is(err, challenge.ErrForbidden) {
						t.Fatalf("forged/duplicate participant accepted: %v", err)
					}
				}
			}
			assertState := func(settled bool) {
				t.Helper()
				for i, c := range original {
					got, err := chars.FindByID(ctx, c.ID)
					if err != nil {
						t.Fatal(err)
					}
					hp, mp, gold, exp := 100, 40, c.Money, c.Experience
					if settled {
						hp, mp, gold, exp = 30, 5, gold+23, exp+7
						if scenario == "defeated" {
							hp = 1
						}
					}
					if got.Stats.HP != hp || got.Stats.MP != mp || got.Money != gold || got.Experience != exp {
						t.Fatalf("member %d state: %+v", i, got)
					}
					var amulets, slots, treasures int
					if err := db.QueryRowContext(ctx, `SELECT
					(SELECT COUNT(*) FROM inventory_items WHERE character_id=? AND definition_id='item-193'),
					(SELECT COUNT(*) FROM equipment_slots WHERE character_id=?),
					(SELECT COUNT(*) FROM inventory_items WHERE character_id=? AND definition_id='item-001')`, c.ID, c.ID, c.ID).Scan(&amulets, &slots, &treasures); err != nil {
						t.Fatal(err)
					}
					wantItems, wantTreasure := 1, 0
					if settled {
						wantItems = 0
						if i == 0 {
							wantTreasure = 1
						}
					}
					if amulets != wantItems || slots != wantItems || treasures != wantTreasure {
						t.Fatalf("member %d items=%d slots=%d treasure=%d", i, amulets, slots, treasures)
					}
					record, err := repo.FindRecord(ctx, c.ID, "1")
					if err != nil {
						t.Fatal(err)
					}
					if (!settled && record != nil) || (settled && (record == nil || record.TotalAttempts != 1 || record.HighestRound != 2)) {
						t.Fatalf("member %d record: %+v", i, record)
					}
				}
			}
			if scenario == "member update rollback" {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_member BEFORE UPDATE ON characters FOR EACH ROW BEGIN IF NEW.id = '`+original[1].ID+`' AND NEW.hp <> OLD.hp THEN SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT='member update failed'; END IF; END`); err != nil {
					t.Fatal(err)
				}
				if err := finalize(); err == nil || !strings.Contains(err.Error(), "member update failed") {
					t.Fatalf("expected member update error: %v", err)
				}
				assertState(false)
				stored, err := repo.FindSessionByID(ctx, run.ID)
				if err != nil || stored.Status != challenge.StatusActive {
					t.Fatalf("claim not rolled back: %+v %v", stored, err)
				}
				if _, err := db.ExecContext(ctx, "DROP TRIGGER fail_member"); err != nil {
					t.Fatal(err)
				}
			}
			if scenario == "equipment read rollback" {
				// Only this dedicated DB loses its equipment table during fault injection.
				if _, err := db.ExecContext(ctx, "RENAME TABLE equipment_slots TO unavailable_equipment_slots"); err != nil {
					t.Fatal(err)
				}
				err := finalize()
				if _, restoreErr := db.ExecContext(ctx, "RENAME TABLE unavailable_equipment_slots TO equipment_slots"); restoreErr != nil {
					t.Fatal(restoreErr)
				}
				var readErr *mysql.MySQLError
				if !errors.As(err, &readErr) || readErr.Number != 1146 {
					t.Fatalf("expected original missing equipment table read error: %v", err)
				}
				assertState(false)
				stored, err := repo.FindSessionByID(ctx, run.ID)
				if err != nil || stored.Status != challenge.StatusActive {
					t.Fatalf("claim not rolled back: %+v %v", stored, err)
				}
			}
			if scenario == "concurrent" || scenario == "equipment read rollback" {
				var claims atomic.Int64
				result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(_, _ int) error {
					err := finalize()
					if err == nil {
						claims.Add(1)
						return nil
					}
					if errors.Is(err, challenge.ErrSessionNotActive) {
						return nil
					}
					return err
				})
				if result.Failures != 0 || claims.Load() != 1 {
					t.Fatalf("claims=%d failures=%d", claims.Load(), result.Failures)
				}
			} else if err := finalize(); err != nil {
				t.Fatal(err)
			}
			if err := finalize(); !errors.Is(err, challenge.ErrSessionNotActive) {
				t.Fatalf("retry: %v", err)
			}
			assertState(true)
		})
	}
}
