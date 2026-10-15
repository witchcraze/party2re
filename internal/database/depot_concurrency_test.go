package database

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Sparse tables can select different locking plans, so copy only DDL into a fresh DB.
func depotIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	ctx := context.Background()
	source, err := OpenFromEnvironment()
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
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated depot tests")
	}
	cfg, err := mysql.ParseDSN(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	admin, err := Open(adminDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := admin.Close(); err != nil {
			t.Error(err)
		}
	})
	name := "party2_depot_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Error(err)
		}
	})
	cfg.DBName = name
	db, err := Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	for _, table := range []string{"players", "characters", "character_depots", "depot_items"} {
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

func TestDepotRepositoryConcurrentAdjacentSaveNoDeadlock(t *testing.T) {
	db := depotIsolatedDB(t)
	ctx := context.Background()
	repo, err := NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	p, err := CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	// These are the only two characters in this DB and are adjacent in the index.
	characterIDs := []string{"00000000000000000000000000000001", "00000000000000000000000000000002"}
	expected := make([][]item.Instance, 2)
	for worker, characterID := range characterIDs {
		char, err := corecharacter.New(fmt.Sprintf("AdjChar%d", worker))
		if err != nil {
			t.Fatal(err)
		}
		char.ID, char.PlayerID = characterID, p.ID
		if err := chars.Save(ctx, char); err != nil {
			t.Fatal(err)
		}
		for _, definitionID := range []string{"weapon-31", "armor-34"} {
			inst, err := item.NewInstance(definitionID, 1)
			if err != nil {
				t.Fatal(err)
			}
			expected[worker] = append(expected[worker], inst)
		}
		if _, err := CreateTestDepot(ctx, db, characterID, 5, expected[worker]); err != nil {
			t.Fatal(err)
		}
	}

	for round := 0; round < 25; round++ {
		roundCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		var ready sync.WaitGroup
		ready.Add(2)
		operations := make([]func() error, 2)
		for worker, characterID := range characterIDs {
			newItem, err := item.NewInstance([]string{"weapon-31", "armor-34"}[worker], 1)
			if err != nil {
				cancel()
				t.Fatal(err)
			}
			before := slices.Clone(expected[worker])
			after := append(slices.Clone(before), newItem)
			if round == 12 {
				after = nil // Exercise the empty-set deletion as well as id NOT IN.
			} else if len(after) > 8 {
				after = after[:3]
			}
			operations[worker] = func() error {
				signalReady := sync.OnceFunc(ready.Done)
				defer signalReady() // Release the peer even if locking fails.
				err := repo.RunInTx(roundCtx, func(txCtx context.Context) error {
					dep, err := repo.FindByCharacterIDForUpdate(txCtx, characterID)
					if err != nil {
						return err
					}
					signalReady()
					ready.Wait()
					if !slices.Equal(dep.Items, before) {
						return fmt.Errorf("items before save: got %+v, want %+v", dep.Items, before)
					}
					if err := dep.AddItem(newItem); err != nil {
						return err
					}
					if round == 12 {
						dep.Items = nil
					} else if len(dep.Items) > 8 {
						dep.Items = dep.Items[:3]
					}
					return repo.Save(txCtx, dep)
				})
				if err == nil {
					expected[worker] = after // Only committed operations advance expected state.
				} else {
					cancel()
				}
				return err
			}
		}
		// RunRace joins both workers before reporting errors or closing DB connections.
		errs := testutil.RunRace(operations...)
		cancel()
		for worker, err := range errs {
			if err != nil {
				t.Fatalf("character %s round %d: %v", characterIDs[worker], round, err)
			}
		}
	}
	for worker, characterID := range characterIDs {
		got, err := repo.FindByCharacterID(ctx, characterID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got.Items, expected[worker]) {
			t.Errorf("character %s final items: got %+v, want %+v", characterID, got.Items, expected[worker])
		}
	}
}
