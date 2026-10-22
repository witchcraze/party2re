package http_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
)

func httpIsolatedDB(t *testing.T) *sql.DB {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	adminDSN := os.Getenv("PARTY2_TEST_DB_ADMIN_DSN")
	if adminDSN == "" {
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated HTTP character creation tests")
	}

	ctx := context.Background()
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := source.Close(); err != nil {
			t.Errorf("failed to close source DB: %v", err)
		}
	})

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
			t.Errorf("failed to close admin DB: %v", err)
		}
	})

	name := "party2_http_char_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP DATABASE `"+name+"`"); err != nil {
			t.Errorf("failed to drop isolated database %s: %v", name, err)
		}
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Errorf("failed to close isolated DB: %v", err)
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

func TestHTTPCreateCharacter_RollbackOnDepotFailure_Integration(t *testing.T) {
	db := httpIsolatedDB(t)
	ctx := context.Background()

	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID); err != nil {
			t.Errorf("failed to cleanup character_depots: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup characters: %v", err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID); err != nil {
			t.Errorf("failed to cleanup players: %v", err)
		}
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	charService, err := character.NewService(
		charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "test-session" {
				return player, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}

	handler, err := apihttp.NewHandler(playerSvc, charService, &stubAdventureService{}, &stubShopService{})
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(handler.Router())
	t.Cleanup(func() {
		server.Close()
	})

	client := &http.Client{}
	charName := fmt.Sprintf("H_%05d", time.Now().UnixNano()%100000)

	body, _ := json.Marshal(map[string]string{
		"name":   charName,
		"job_id": "job-01",
		"gender": "m",
	})

	// Inject real database-level failure on character_depots INSERT statement.
	// This ensures the real CharacterRepository.Save executes characters INSERT successfully,
	// but the second INSERT (character_depots) fails at the real MariaDB SQL level.
	_, err = db.ExecContext(ctx, "CREATE TRIGGER trg_http_fail_depot BEFORE INSERT ON character_depots FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'simulated http depot insert failure'")
	if err != nil {
		t.Fatal(err)
	}

	// 1. POST /characters with depot failure injected: must return 422 error
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer test-session")
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("POST /characters request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 Unprocessable Entity, got %d", resp.StatusCode)
	}

	// 2. Verify DB: Neither character nor depot must exist
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE name = ?", charName).Scan(&count); err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if count != 0 {
		t.Fatalf("character leaked into database after HTTP endpoint failure! count = %d, want 0", count)
	}

	var totalDepots int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_depots").Scan(&totalDepots); err != nil {
		t.Fatalf("failed to query total depots: %v", err)
	}
	if totalDepots != 0 {
		t.Fatalf("depot leaked into database on depot failure! count = %d, want 0", totalDepots)
	}

	// 3. Resolve failure injection by dropping the trigger, then retry: must return 201 Created
	if _, err := db.ExecContext(ctx, "DROP TRIGGER trg_http_fail_depot"); err != nil {
		t.Fatal(err)
	}

	reqRetry, _ := http.NewRequest(http.MethodPost, server.URL+"/characters", bytes.NewReader(body))
	reqRetry.Header.Set("Authorization", "Bearer test-session")
	reqRetry.Header.Set("Content-Type", "application/json")
	respRetry, err := client.Do(reqRetry)
	if err != nil {
		t.Fatalf("POST /characters retry request failed: %v", err)
	}
	defer respRetry.Body.Close()

	if respRetry.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created on retry, got %d", respRetry.StatusCode)
	}

	var createdResp map[string]any
	if err := json.NewDecoder(respRetry.Body).Decode(&createdResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if createdResp["name"] != charName {
		t.Errorf("expected name = %s, got %v", charName, createdResp["name"])
	}

	// 4. Verify DB: Exactly 1 character and 1 depot exist
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE name = ?", charName).Scan(&count); err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected 1 character in DB, got %d", count)
	}

	createdID, _ := createdResp["id"].(string)
	var depotCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_depots WHERE character_id = ?", createdID).Scan(&depotCount); err != nil {
		t.Fatalf("failed to query depots: %v", err)
	}
	if depotCount != 1 {
		t.Fatalf("expected 1 depot in DB, got %d", depotCount)
	}
}
