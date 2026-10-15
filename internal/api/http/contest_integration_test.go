package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/contest"
	core_scheduling "github.com/witchcraze/party2re/internal/core/scheduling"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/scheduling"
)

type statefulSchedRepo struct {
	mu          sync.Mutex
	actions     map[string]core_scheduling.ScheduledAction
	failSched   bool
	scheduleErr error
}

func newStatefulSchedRepo() *statefulSchedRepo {
	return &statefulSchedRepo{
		actions: make(map[string]core_scheduling.ScheduledAction),
	}
}

func (r *statefulSchedRepo) Schedule(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failSched && r.scheduleErr != nil {
		return r.scheduleErr
	}
	r.actions[action.ID] = action
	return nil
}

func (r *statefulSchedRepo) FetchDue(_ context.Context, upTo time.Time, limit int) ([]core_scheduling.ScheduledAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var due []core_scheduling.ScheduledAction
	for _, a := range r.actions {
		if a.State == core_scheduling.StatePending && !a.ExecuteAt.After(upTo) {
			due = append(due, a)
			if len(due) >= limit {
				break
			}
		}
	}
	return due, nil
}

func (r *statefulSchedRepo) FindPendingByActorID(_ context.Context, actorID string) ([]core_scheduling.ScheduledAction, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []core_scheduling.ScheduledAction
	for _, a := range r.actions {
		if a.ActorID == actorID && a.State == core_scheduling.StatePending {
			res = append(res, a)
		}
	}
	return res, nil
}

func (r *statefulSchedRepo) AcquireLock(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (r *statefulSchedRepo) Save(_ context.Context, action core_scheduling.ScheduledAction) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.actions[action.ID] = action
	return nil
}

func (r *statefulSchedRepo) CancelByActorID(_ context.Context, actorID string) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for id, a := range r.actions {
		if a.ActorID == actorID {
			delete(r.actions, id)
			count++
		}
	}
	return count, nil
}

func contestHTTPIntegrationDB(t *testing.T) (*database.ContestRepository, *database.CharacterRepository, database.TransactionProvider) {
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
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated contest settlement tests")
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

	name := "party2_contest_http_test_" + id.New()
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

	for _, table := range []string{"players", "characters", "character_depots", "character_photos", "contest_rounds", "contest_entries", "contest_votes", "contest_legends"} {
		var tableName, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+table+"`").Scan(&tableName, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, ddl); err != nil {
			t.Fatal(err)
		}
	}

	contestRepo, err := database.NewContestRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)

	return contestRepo, charRepo, txProvider
}

func TestContestVenueAndUnreadySettlement_RealService_HTTP_Integration(t *testing.T) {
	ctx := context.Background()
	contestRepo, charRepo, txProvider := contestHTTPIntegrationDB(t)

	now := time.Now().UTC().Truncate(time.Second)
	schedRepo := newStatefulSchedRepo()
	schedService := scheduling.NewService(schedRepo)

	targetEndTime := now.Add(time.Hour)
	r1 := contest.ContestRound{
		Round:     1,
		Status:    contest.StatusActive,
		StartTime: now,
		EndTime:   targetEndTime,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := contestRepo.SaveRound(ctx, r1); err != nil {
		t.Fatalf("SaveRound failed: %v", err)
	}

	svc, err := contest.NewService(
		charRepo,
		contestRepo,
		contest.WithTransactionProvider(txProvider),
		contest.WithServiceScheduler(schedService),
		contest.WithNowFunc(func() time.Time { return now }),
	)
	if err != nil {
		t.Fatalf("NewService failed: %v", err)
	}

	adminKey := "test-admin-key-1391"
	handler, err := apihttp.NewHandler(
		&stubPlayerService{},
		&stubCharacterService{},
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithAdminAPIKey(adminKey),
		apihttp.WithContest(svc),
	)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}

	server := httptest.NewServer(handler.Router())
	defer server.Close()

	client := &http.Client{}

	// =========================================================================
	// Phase 1: Scheduler failure injected
	// =========================================================================
	schedRepo.failSched = true
	schedRepo.scheduleErr = errors.New("queue unavailable")

	// 1-A. GET /contest/venue must fail with 500 Internal Server Error
	resp, err := client.Get(server.URL + "/contest/venue")
	if err != nil {
		t.Fatalf("GET /contest/venue request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error on GET /contest/venue when scheduler fails, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Verify SQL round is intact in MariaDB
	roundAfterFail, err := contestRepo.GetRoundByNumber(ctx, 1)
	if err != nil {
		t.Fatalf("GetRoundByNumber failed: %v", err)
	}
	if !roundAfterFail.EndTime.Equal(targetEndTime) || roundAfterFail.Status != contest.StatusActive {
		t.Fatalf("expected round 1 to remain untouched in MariaDB, got: %+v", roundAfterFail)
	}

	// Verify 0 actions in queue
	if len(schedRepo.actions) != 0 {
		t.Fatalf("expected 0 actions scheduled during failure, got %d", len(schedRepo.actions))
	}

	// 1-B. POST /contest/settle with unready round must fail with 500 (reconciliation error propagated)
	settleBody, _ := json.Marshal(map[string]bool{"force": false})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/contest/settle", bytes.NewReader(settleBody))
	req.Header.Set("X-Admin-Key", adminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /contest/settle request failed: %v", err)
	}
	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("expected 500 Internal Server Error on POST /contest/settle when unready reconciliation fails, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// =========================================================================
	// Phase 2: Clear scheduler fault (recovery)
	// =========================================================================
	schedRepo.failSched = false
	schedRepo.scheduleErr = nil

	// 2-A. GET /contest/venue must succeed with 200 OK and self-heal 1 reservation
	resp, err = client.Get(server.URL + "/contest/venue")
	if err != nil {
		t.Fatalf("GET /contest/venue recovery request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on GET /contest/venue after recovery, got %d", resp.StatusCode)
	}
	var venue struct {
		ActiveRound *contest.ContestRound `json:"active_round"`
		Dialogue    struct {
			NPCName string `json:"npc_name"`
		} `json:"dialogue"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&venue); err != nil {
		t.Fatalf("decode venue response failed: %v", err)
	}
	_ = resp.Body.Close()

	if venue.ActiveRound == nil || venue.ActiveRound.Round != 1 {
		t.Fatalf("expected active round 1 in venue response, got %+v", venue.ActiveRound)
	}
	if venue.Dialogue.NPCName != "@ワコール" {
		t.Errorf("expected NPCName @ワコール, got %q", venue.Dialogue.NPCName)
	}

	// Verify exactly 1 action was registered in the queue
	if len(schedRepo.actions) != 1 {
		t.Fatalf("expected 1 scheduled action after venue recovery, got %d", len(schedRepo.actions))
	}
	expectedActionPrefix := contest.SettlementActionID(1, targetEndTime)
	var createdAction *core_scheduling.ScheduledAction
	for _, a := range schedRepo.actions {
		if strings.HasPrefix(a.ID, expectedActionPrefix) {
			createdAction = &a
			break
		}
	}
	if createdAction == nil {
		t.Fatalf("expected scheduled action with prefix %q in queue", expectedActionPrefix)
	}
	if createdAction.State != core_scheduling.StatePending {
		t.Errorf("expected action State %s, got %s", core_scheduling.StatePending, createdAction.State)
	}

	// 2-B. POST /contest/settle with unready round must return 400 Bad Request (reconciliation succeeds, preserves reservation)
	settleBody, _ = json.Marshal(map[string]bool{"force": false})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/contest/settle", bytes.NewReader(settleBody))
	req.Header.Set("X-Admin-Key", adminKey)
	req.Header.Set("Content-Type", "application/json")
	resp, err = client.Do(req)
	if err != nil {
		t.Fatalf("POST /contest/settle recovered request failed: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 Bad Request on unready round after recovery, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// Verify reservation was NOT duplicated
	if len(schedRepo.actions) != 1 {
		t.Fatalf("expected still 1 scheduled action (no duplicate on unready settle), got %d", len(schedRepo.actions))
	}

	// 2-C. Subsequent GET /contest/venue must succeed with 200 OK without duplicate reservation
	resp, err = client.Get(server.URL + "/contest/venue")
	if err != nil {
		t.Fatalf("GET /contest/venue second request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK on repeated GET /contest/venue, got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	if len(schedRepo.actions) != 1 {
		t.Fatalf("expected still 1 scheduled action (no duplicate on repeated venue GET), got %d", len(schedRepo.actions))
	}
}
