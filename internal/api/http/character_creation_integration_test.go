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

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
)

type httpFaultInjectingCharacterRepo struct {
	*database.CharacterRepository
	db        *sql.DB
	failDepot bool
}

func (r *httpFaultInjectingCharacterRepo) Save(ctx context.Context, value corecharacter.Character) error {
	if !r.failDepot {
		return r.CharacterRepository.Save(ctx, value)
	}

	color := value.Color
	if color == "" {
		color = corecharacter.DefaultColor
	}

	_, err := database.ExecutorFromContext(ctx, r.db).ExecContext(ctx, `
		INSERT INTO characters
			(id, player_id, name, job_id, gender, max_hp, max_mp, hp, mp, attack, defense, agility, money, level, experience, sp, job_level, old_job_id, old_sp, job_memory_job_id, job_memory_sp, job_memory_old_job_id, job_memory_old_sp, job_memory_kind, small_medals, help_count, hero_count, pvp_wins, casino_wins, monster_kills, mao_count, orb, tired, over_level, over_depot, over_monster, over_future, over_flea, over_store, color, deposit, crystal, wea_seal, wea_name, arm_name, pending_wake)
		VALUES (
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?, ?, ?,
			?, ?, ?, ?, ?, ?
		)
	`, value.ID, value.PlayerID, value.Name, value.JobID, value.Gender, value.Stats.MaxHP, value.Stats.MaxMP,
		value.Stats.HP, value.Stats.MP, value.Stats.Attack, value.Stats.Defense, value.Stats.Agility,
		value.Money, value.Level, value.Experience, value.SP, value.JobLevel, value.OldJobID, value.OldSP,
		"", 0, "", 0, string(corecharacter.JobMemoryKindPersistent), value.SmallMedals, value.HelpCount, value.HeroCount, value.PvPWins, value.CasinoWins,
		value.MonsterKills, value.MaoCount,
		value.Orb, value.Tired, value.OverLevel, value.OverDepot, value.OverMonster, value.OverFuture, value.OverFlea, value.OverStore, color,
		value.Deposit, value.Crystal, value.WeaponSeal, value.WeaponCustomName, value.ArmorCustomName, value.PendingWake)
	if err != nil {
		return err
	}

	return errors.New("simulated http depot insert failure")
}

func TestHTTPCreateCharacter_RollbackOnDepotFailure_Integration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID)
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID)
	})

	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	faultRepo := &httpFaultInjectingCharacterRepo{
		CharacterRepository: charRepo,
		db:                  db,
		failDepot:           true,
	}

	charService, err := character.NewService(
		faultRepo,
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
	defer server.Close()

	client := &http.Client{}
	charName := fmt.Sprintf("H_%05d", time.Now().UnixNano()%100000)

	body, _ := json.Marshal(map[string]string{
		"name":   charName,
		"job_id": "job-01",
		"gender": "m",
	})

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

	// 2. Verify DB: Character must NOT exist
	var count int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE name = ?", charName).Scan(&count); err != nil {
		t.Fatalf("failed to query characters: %v", err)
	}
	if count != 0 {
		t.Fatalf("character leaked into database after HTTP endpoint failure! count = %d, want 0", count)
	}

	// 3. Resolve failure injection and retry: must return 201 Created
	faultRepo.failDepot = false

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
