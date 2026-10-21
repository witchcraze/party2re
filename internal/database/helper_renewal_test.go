package database_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// isolatedRepositoryDB copies table definitions into a dedicated, disposable database.
func isolatedRepositoryDB(t *testing.T, tables ...string) *sql.DB {
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
		t.Fatal("PARTY2_TEST_DB_ADMIN_DSN is required for isolated repository tests")
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
	name := "party2_repository_test_" + id.New()
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
	for _, table := range tables {
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

type renewalFixture struct {
	db      *sql.DB
	repo    *database.HelperRepository
	service *helperquest.Service
	now     time.Time
}

func newRenewalFixture(t *testing.T) renewalFixture {
	t.Helper()
	db := isolatedRepositoryDB(t, "helper_quests")
	repo, err := database.NewHelperRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	// Renewal-only operations touch helper_quests; character and depot ports are unused.
	svc := helperquest.NewService(repo, nil, nil, database.NewTransactionProvider(db), helperquest.WithMonsterLookup(testutil.MonsterNameLookup(t)))
	return renewalFixture{db: db, repo: repo, service: svc, now: time.Now().UTC().Truncate(time.Second)}
}

func (f renewalFixture) seed(t *testing.T, questID string, expiresAt time.Time, completedBy string) helperquest.Quest {
	t.Helper()
	q := helperquest.Quest{
		ID: questID, Title: questID, Kind: helperquest.KindItem, TargetID: "item-001", TargetName: "薬草",
		RequiredCount: 2, RewardItemID: "item-128", CreatedAt: f.now.Add(-7 * 24 * time.Hour), ExpiresAt: expiresAt,
	}
	if completedBy != "" {
		q.CompletedAt, q.CompletedBy = &f.now, completedBy
	}
	if err := f.repo.Save(context.Background(), q); err != nil {
		t.Fatal(err)
	}
	return q
}

func (f renewalFixture) count(t *testing.T) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM helper_quests").Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestHelperQuestExchangeReplacesExpiredRowsOnce(t *testing.T) {
	f := newRenewalFixture(t)
	ctx := context.Background()
	f.seed(t, "expired-1", f.now.Add(-time.Hour), "")
	f.seed(t, "expired-2", f.now.Add(-time.Second), "")
	f.seed(t, "expired-done", f.now.Add(-time.Hour), "someone")
	f.seed(t, "live", f.now.Add(time.Hour), "")

	if _, err := f.service.CompleteQuest(ctx, "nobody", "", f.now); err != nil {
		t.Fatal(err)
	}

	for _, id := range []string{"expired-1", "expired-2"} {
		if _, err := f.repo.FindByID(ctx, id); !errors.Is(err, helperquest.ErrQuestNotFound) {
			t.Errorf("%s still stored: %v", id, err)
		}
	}
	for _, id := range []string{"expired-done", "live"} {
		if _, err := f.repo.FindByID(ctx, id); err != nil {
			t.Errorf("%s must be retained: %v", id, err)
		}
	}
	active, err := f.repo.ListUncompleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 3 { // live + 2 exchanged
		t.Fatalf("active = %d, want 3", len(active))
	}
	if f.count(t) != 4 { // 4 originals - 2 exchanged + 2 replacements
		t.Fatalf("stored rows = %d, want 4", f.count(t))
	}
}

func TestHelperQuestExchangeConcurrentCallsReplaceEachRowOnce(t *testing.T) {
	f := newRenewalFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	const expired = 3
	for i := range expired {
		f.seed(t, "expired-"+string(rune('a'+i)), f.now.Add(-time.Hour), "")
	}
	operations := make([]func() error, 6)
	for i := range operations {
		operations[i] = func() error {
			_, err := f.service.CompleteQuest(ctx, "nobody", "", f.now)
			return err
		}
	}
	for _, err := range testutil.RunRace(operations...) {
		if err != nil {
			t.Fatalf("concurrent exchange: %v", err)
		}
	}
	active, err := f.repo.ListUncompleted(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != expired || f.count(t) != expired {
		t.Fatalf("active = %d, stored = %d, want %d each", len(active), f.count(t), expired)
	}
}

type failingReplacementRepository struct {
	*database.HelperRepository
	err error
}

func (r failingReplacementRepository) Save(ctx context.Context, q helperquest.Quest) error {
	if q.CompletedAt == nil && r.err != nil {
		return r.err
	}
	return r.HelperRepository.Save(ctx, q)
}

func TestHelperQuestExchangeRollsBackDeletionOnReplacementFailure(t *testing.T) {
	f := newRenewalFixture(t)
	failure := errors.New("replacement save failed")
	svc := helperquest.NewService(failingReplacementRepository{HelperRepository: f.repo, err: failure}, nil, nil, database.NewTransactionProvider(f.db), helperquest.WithMonsterLookup(testutil.MonsterNameLookup(t)))
	repo := f.repo
	ctx := context.Background()
	f.seed(t, "expired", f.now.Add(-time.Hour), "")

	if _, err := svc.CompleteQuest(ctx, "nobody", "", f.now); !errors.Is(err, failure) {
		t.Fatalf("error = %v, want %v", err, failure)
	}
	if _, err := repo.FindByID(ctx, "expired"); err != nil {
		t.Fatalf("expired request lost after rollback: %v", err)
	}
	if f.count(t) != 1 {
		t.Fatalf("stored rows = %d, want 1", f.count(t))
	}
}

func TestHelperQuestExchangeLeavesEmptyStoreEmpty(t *testing.T) {
	f := newRenewalFixture(t)
	ids, err := f.service.GetActiveHelperItemIDs(context.Background(), f.now)
	if err != nil || len(ids) != 0 {
		t.Fatalf("empty board exclusions = %v, %v", ids, err)
	}
	if _, err := f.service.CompleteQuest(context.Background(), "nobody", "", f.now); err != nil {
		t.Fatal(err)
	}
	if f.count(t) != 0 {
		t.Fatalf("empty store gained rows: %d", f.count(t))
	}
}
