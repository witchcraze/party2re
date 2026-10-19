package http_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
)

type helperSleepIntegrationEnv struct {
	db        *sql.DB
	quests    *database.HelperRepository
	chars     *database.CharacterRepository
	depots    *database.DepotRepository
	service   *helperquest.Service
	player    coreplayer.Player
	char      corecharacter.Character
	otherChar corecharacter.Character
	router    http.Handler
	mockHome  *mockHomeService
}

func setupHelperSleepIntegrationEnv(t *testing.T) *helperSleepIntegrationEnv {
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
		_ = source.Close()
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
		_ = admin.Close()
	})

	name := "party2_helper_sleep_test_" + id.New()
	if _, err := admin.ExecContext(ctx, "CREATE DATABASE `"+name+"`"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(ctx, "DROP DATABASE `"+name+"`")
	})

	cfg.DBName = name
	db, err := database.Open(cfg.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	tables := []string{"players", "characters", "character_depots", "depot_items", "helper_quests"}
	for _, table := range tables {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}

	questRepo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	depotRepo, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	helperSvc := helperquest.NewService(
		questRepo,
		charRepo,
		nil,
		txProvider,
		helperquest.WithDepotRepository(depotRepo),
	)

	// Create primary player and character
	p, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := corecharacter.New("HelperHero")
	if err != nil {
		t.Fatal(err)
	}
	char.PlayerID = p.ID
	char.Money = 500
	if err := charRepo.Save(ctx, char); err != nil {
		t.Fatal(err)
	}

	// Create another player and character to verify ownership
	otherP, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	otherChar, err := corecharacter.New("OtherHero")
	if err != nil {
		t.Fatal(err)
	}
	otherChar.PlayerID = otherP.ID
	if err := charRepo.Save(ctx, otherChar); err != nil {
		t.Fatal(err)
	}

	playerSvc := &stubPlayerService{
		authenticateFn: func(_ context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return p, nil
			}
			return coreplayer.Player{}, errors.New("unauthorized")
		},
	}
	charSvc := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			return charRepo.FindByID(ctx, id)
		},
	}
	mockHome := &mockHomeService{}

	handler, err := apihttp.NewHandler(
		playerSvc,
		charSvc,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithHome(mockHome),
		apihttp.WithHelper(helperSvc),
	)
	if err != nil {
		t.Fatal(err)
	}

	return &helperSleepIntegrationEnv{
		db:        db,
		quests:    questRepo,
		chars:     charRepo,
		depots:    depotRepo,
		service:   helperSvc,
		player:    p,
		char:      char,
		otherChar: otherChar,
		router:    handler.Router(),
		mockHome:  mockHome,
	}
}

func TestHelperQuestCompletion_SleepGuard_RealService_Integration(t *testing.T) {
	ctx := context.Background()
	env := setupHelperSleepIntegrationEnv(t)

	now := time.Now().UTC().Truncate(time.Second)
	// Create an uncompleted valid quest: requires 2x item-001, rewards 1x item-128
	quest := helperquest.Quest{
		ID:            id.New(),
		Title:         "Gather Herbs",
		Kind:          helperquest.KindItem,
		TargetID:      "item-001",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		CreatedAt:     now,
		ExpiresAt:     now.Add(2 * time.Hour),
	}
	if err := env.quests.Save(ctx, quest); err != nil {
		t.Fatal(err)
	}

	// Create character depot with 4 units of item-001
	materials, err := coreitem.NewInstance("item-001", 4)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestDepot(ctx, env.db, env.char.ID, 5, []coreitem.Instance{materials}); err != nil {
		t.Fatal(err)
	}

	sendReq := func(token, charID, questID string) *httptest.ResponseRecorder {
		bodyMap := map[string]string{"character_id": charID}
		if questID != "__OMIT__" {
			bodyMap["quest_id"] = questID
		}
		bodyBytes, _ := json.Marshal(bodyMap)
		req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		return rec
	}

	assertDBUnchanged := func(t *testing.T) {
		t.Helper()
		// Quest should remain uncompleted
		storedQuest, err := env.quests.FindByID(ctx, quest.ID)
		if err != nil {
			t.Fatalf("failed to find quest: %v", err)
		}
		if storedQuest.CompletedAt != nil {
			t.Fatalf("expected quest to remain uncompleted, got completed_at=%v", storedQuest.CompletedAt)
		}

		// Character help count and money should be untouched
		c, err := env.chars.FindByID(ctx, env.char.ID)
		if err != nil {
			t.Fatalf("failed to find char: %v", err)
		}
		if c.HelpCount != 0 {
			t.Fatalf("expected HelpCount 0, got %d", c.HelpCount)
		}
		if c.Money != env.char.Money {
			t.Fatalf("expected money %d, got %d", env.char.Money, c.Money)
		}

		// Depot materials should remain 4x item-001 and no item-128
		dep, err := env.depots.FindByCharacterID(ctx, env.char.ID)
		if err != nil {
			t.Fatalf("failed to find depot: %v", err)
		}
		foundMaterials := 0
		for _, it := range dep.Items {
			if it.DefinitionID == "item-001" {
				foundMaterials += it.Quantity
			}
			if it.DefinitionID == "item-128" {
				t.Fatalf("expected no reward item in depot, found item-128")
			}
		}
		if foundMaterials != 4 {
			t.Fatalf("expected 4 materials in depot, got %d", foundMaterials)
		}
	}

	t.Run("auth and ownership checks are preserved", func(t *testing.T) {
		// Missing / invalid session token -> 401
		rec := sendReq("", env.char.ID, quest.ID)
		if rec.Code != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d: %s", rec.Code, rec.Body.String())
		}

		// Character belonging to another player -> 403
		rec = sendReq("valid-session", env.otherChar.ID, quest.ID)
		if rec.Code != http.StatusForbidden {
			t.Errorf("expected 403, got %d: %s", rec.Code, rec.Body.String())
		}
		assertDBUnchanged(t)
	})

	t.Run("rejected when sleeping with 409 Conflict and DB state unchanged", func(t *testing.T) {
		env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
			return home.SleepStatus{
				Sleeping:         true,
				RemainingSeconds: 300,
				Message:          "お休み中「Zzz...」",
			}, nil
		}

		rec := sendReq("valid-session", env.char.ID, quest.ID)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
		assertDBUnchanged(t)
	})

	t.Run("rejected when can_wake with 409 Conflict and DB state unchanged", func(t *testing.T) {
		env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
			return home.SleepStatus{
				Sleeping: false,
				CanWake:  true,
				Message:  "お休み中「Zzz...」 目を覚ましてください",
			}, nil
		}

		rec := sendReq("valid-session", env.char.ID, quest.ID)
		if rec.Code != http.StatusConflict {
			t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
		}
		assertDBUnchanged(t)
	})

	t.Run("rejected when sleep status lookup fails with 500 and DB state unchanged", func(t *testing.T) {
		env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
			return home.SleepStatus{}, errors.New("sleep status lookup failure")
		}

		rec := sendReq("valid-session", env.char.ID, quest.ID)
		if rec.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500 Internal Server Error, got %d: %s", rec.Code, rec.Body.String())
		}
		assertDBUnchanged(t)
	})

	t.Run("awake character completes quest and updates assets in DB", func(t *testing.T) {
		env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
			return home.SleepStatus{
				Sleeping: false,
				CanWake:  false,
				Message:  "起きています",
			}, nil
		}

		rec := sendReq("valid-session", env.char.ID, quest.ID)
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var res helperquest.CompletionResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.CompletedQuest == nil || res.CompletedQuest.ID != quest.ID {
			t.Fatalf("expected completed quest %s, got %+v", quest.ID, res.CompletedQuest)
		}

		// Verify database state: quest completed
		storedQuest, err := env.quests.FindByID(ctx, quest.ID)
		if err != nil {
			t.Fatalf("failed to find quest: %v", err)
		}
		if storedQuest.CompletedAt == nil {
			t.Fatalf("expected quest to be marked completed")
		}

		// Verify database state: HelpCount incremented
		c, err := env.chars.FindByID(ctx, env.char.ID)
		if err != nil {
			t.Fatalf("failed to find char: %v", err)
		}
		if c.HelpCount != 1 {
			t.Fatalf("expected HelpCount 1, got %d", c.HelpCount)
		}

		// Verify database state: materials consumed (4 -> 2) and reward item added (0 -> 1)
		dep, err := env.depots.FindByCharacterID(ctx, env.char.ID)
		if err != nil {
			t.Fatalf("failed to find depot: %v", err)
		}
		remainingMaterials := 0
		rewardItems := 0
		for _, it := range dep.Items {
			if it.DefinitionID == "item-001" {
				remainingMaterials += it.Quantity
			}
			if it.DefinitionID == "item-128" {
				rewardItems += it.Quantity
			}
		}
		if remainingMaterials != 2 {
			t.Fatalf("expected 2 remaining materials, got %d", remainingMaterials)
		}
		if rewardItems != 1 {
			t.Fatalf("expected 1 reward item, got %d", rewardItems)
		}
	})
}

func TestHelperQuestRenewalOnly_SleepGuard_RealService_Integration(t *testing.T) {
	ctx := context.Background()
	env := setupHelperSleepIntegrationEnv(t)

	now := time.Now().UTC().Truncate(time.Second)
	// Create two expired quests on the board
	expired1 := helperquest.Quest{
		ID:            id.New(),
		Title:         "Expired Quest 1",
		Kind:          helperquest.KindItem,
		TargetID:      "item-001",
		RequiredCount: 2,
		RewardItemID:  "item-128",
		CreatedAt:     now.Add(-2 * time.Hour),
		ExpiresAt:     now.Add(-1 * time.Hour),
	}
	expired2 := helperquest.Quest{
		ID:            id.New(),
		Title:         "Expired Quest 2",
		Kind:          helperquest.KindItem,
		TargetID:      "item-002",
		RequiredCount: 1,
		RewardItemID:  "item-129",
		CreatedAt:     now.Add(-2 * time.Hour),
		ExpiresAt:     now.Add(-1 * time.Hour),
	}
	if err := env.quests.Save(ctx, expired1); err != nil {
		t.Fatal(err)
	}
	if err := env.quests.Save(ctx, expired2); err != nil {
		t.Fatal(err)
	}

	sendRenewalReq := func(token, charID, questID string) *httptest.ResponseRecorder {
		bodyMap := map[string]string{"character_id": charID}
		if questID != "__OMIT__" {
			bodyMap["quest_id"] = questID
		}
		bodyBytes, _ := json.Marshal(bodyMap)
		req := httptest.NewRequest(http.MethodPost, "/helpers/complete", bytes.NewReader(bodyBytes))
		req.Header.Set("Content-Type", "application/json")
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		rec := httptest.NewRecorder()
		env.router.ServeHTTP(rec, req)
		return rec
	}

	assertBoardUnchanged := func(t *testing.T) {
		t.Helper()
		uncompleted, err := env.quests.ListUncompleted(ctx)
		if err != nil {
			t.Fatalf("failed to list uncompleted: %v", err)
		}
		if len(uncompleted) != 2 {
			t.Fatalf("expected 2 uncompleted quests, got %d", len(uncompleted))
		}
		found1, found2 := false, false
		for _, q := range uncompleted {
			if q.ID == expired1.ID {
				found1 = true
			}
			if q.ID == expired2.ID {
				found2 = true
			}
		}
		if !found1 || !found2 {
			t.Fatalf("expected original expired quests on board, got %+v", uncompleted)
		}
	}

	for _, inputCase := range []struct {
		name    string
		questID string
	}{
		{"omitted quest_id", "__OMIT__"},
		{"empty quest_id", ""},
	} {
		t.Run(inputCase.name, func(t *testing.T) {
			t.Run("rejected when sleeping with 409 Conflict", func(t *testing.T) {
				env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
					return home.SleepStatus{
						Sleeping:         true,
						RemainingSeconds: 300,
						Message:          "お休み中「Zzz...」",
					}, nil
				}
				rec := sendRenewalReq("valid-session", env.char.ID, inputCase.questID)
				if rec.Code != http.StatusConflict {
					t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
				}
				assertBoardUnchanged(t)
			})

			t.Run("rejected when can_wake with 409 Conflict", func(t *testing.T) {
				env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
					return home.SleepStatus{
						Sleeping: false,
						CanWake:  true,
						Message:  "お休み中「Zzz...」 目を覚ましてください",
					}, nil
				}
				rec := sendRenewalReq("valid-session", env.char.ID, inputCase.questID)
				if rec.Code != http.StatusConflict {
					t.Fatalf("expected 409 Conflict, got %d: %s", rec.Code, rec.Body.String())
				}
				assertBoardUnchanged(t)
			})

			t.Run("rejected when sleep status lookup fails with 500", func(t *testing.T) {
				env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
					return home.SleepStatus{}, errors.New("lookup error")
				}
				rec := sendRenewalReq("valid-session", env.char.ID, inputCase.questID)
				if rec.Code != http.StatusInternalServerError {
					t.Fatalf("expected 500, got %d: %s", rec.Code, rec.Body.String())
				}
				assertBoardUnchanged(t)
			})
		})
	}

	t.Run("awake character performs renewal and exchanges expired requests", func(t *testing.T) {
		env.mockHome.getSleepStatusFn = func(_ context.Context, _ string) (home.SleepStatus, error) {
			return home.SleepStatus{
				Sleeping: false,
				CanWake:  false,
				Message:  "起きています",
			}, nil
		}

		rec := sendRenewalReq("valid-session", env.char.ID, "")
		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200 OK, got %d: %s", rec.Code, rec.Body.String())
		}

		var res helperquest.CompletionResult
		if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if res.RenewedCount != 2 {
			t.Fatalf("expected renewed_count 2, got %d", res.RenewedCount)
		}
		if res.CompletedQuest != nil {
			t.Fatalf("expected completed quest to be nil on renewal-only, got %+v", res.CompletedQuest)
		}

		// Verify expired quests were exchanged
		uncompleted, err := env.quests.ListUncompleted(ctx)
		if err != nil {
			t.Fatalf("failed to list uncompleted: %v", err)
		}
		for _, q := range uncompleted {
			if q.ID == expired1.ID || q.ID == expired2.ID {
				t.Fatalf("expired quest %s should have been exchanged, but still present", q.ID)
			}
		}
	})
}
