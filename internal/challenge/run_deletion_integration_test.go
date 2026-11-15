package challenge_test

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Real deletion traverses tables outside the settlement fixture. Copy their
// definitions, including foreign keys, into that fixture's dedicated database.
// This extends an already populated fixture; CreateTestDatabase creates a new DB.
func (f *protocolFixture) deletionService(t testing.TB, account bool) func(context.Context) error {
	t.Helper()
	// Schema source for a dedicated DB; close it when the schema copy finishes.
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	ctx := context.Background()
	rows, err := source.QueryContext(ctx, "SHOW TABLES")
	if err != nil {
		t.Fatal(err)
	}
	var tables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			t.Fatal(err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if err := rows.Close(); err != nil {
		t.Fatal(err)
	}
	conn, err := f.db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		restoreCtx, restoreCancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer restoreCancel()
		if _, err := conn.ExecContext(restoreCtx, "SET FOREIGN_KEY_CHECKS=1"); err != nil {
			t.Errorf("restore foreign key checks: %v", err)
			_ = conn.Raw(func(any) error { return driver.ErrBadConn }) //lint:ignore error-swallow marks connection bad so the pool discards it
		}
		if err := conn.Close(); err != nil && !errors.Is(err, sql.ErrConnDone) {
			t.Error(err)
		}
	}()
	if _, err := conn.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS=0"); err != nil {
		t.Fatal(err)
	}
	for _, table := range tables {
		var name, ddl string
		if err := source.QueryRowContext(ctx, "SHOW CREATE TABLE `"+strings.ReplaceAll(table, "`", "``")+"`").Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := conn.ExecContext(ctx, strings.Replace(ddl, "CREATE TABLE", "CREATE TABLE IF NOT EXISTS", 1)); err != nil {
			t.Fatal(err)
		}
	}
	tx := database.NewTransactionProvider(f.db)
	chars, err := character.NewService(f.chars, character.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	a := f.actors[0]
	if !account {
		return func(ctx context.Context) error { return chars.Delete(ctx, a.PlayerID, a.ID) }
	}
	players, err := database.NewPlayerRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := player.NewService(players, player.NewValkeySessionRepository(f.client), player.WithCharacterService(chars), player.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	return func(ctx context.Context) error { return svc.DeleteAccount(ctx, a.PlayerID, "") }
}

func (f *protocolFixture) departedLeader(t *testing.T) *challenge.ChallengeSession {
	t.Helper()
	ctx := context.Background()
	run := f.advance(t, f.start(t))
	// Pin a designated treasure without making the regression depend on RNG.
	run.TreasureAwards = []challenge.TreasureAward{{ItemID: "item-001", RecipientID: f.actors[1].ID}}
	run.AccumulatedItems = []string{"item-001"}
	saveRunFixture(t, ctx, f.repo, f.store, run)
	out, err := f.svc.ExitSession(ctx, f.actors[0].ID, run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if out.Session.CharacterID != f.actors[1].ID || len(out.Session.Members) != 1 {
		t.Fatalf("leader handoff: %+v", out)
	}
	f.assertSettled(t, 0, 1, 1)
	// Passing contrast: the survivor can advance before deletion.
	return f.advance(t, &out.Session)
}

func (f *protocolFixture) assertDeletionState(t *testing.T, run *challenge.ChallengeSession, settled bool) {
	t.Helper()
	ctx := context.Background()
	if _, err := f.chars.FindByID(ctx, f.actors[0].ID); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("original leader still exists: %v", err)
	}
	claim, err := f.repo.FindClaim(ctx, f.actors[1].ID)
	if err != nil || (!settled && claim != run.ID) || (settled && claim != "") {
		t.Fatalf("survivor claim=%q settled=%v err=%v", claim, settled, err)
	}
	if !settled {
		admission, err := f.repo.FindSessionByID(ctx, run.ID)
		if err != nil || admission.CharacterID != f.actors[0].ID || len(admission.Members) != 2 {
			t.Fatalf("immutable admission lost: %+v %v", admission, err)
		}
		live, err := f.store.ReadRun(ctx, run.ID)
		if err != nil || !reflect.DeepEqual(run, live) {
			t.Fatalf("deletion changed live run: %+v %v", live, err)
		}
		return
	}
	rounds := run.CurrentRound - 1
	f.assertSettled(t, 1, rounds, rounds)
	var treasures, receipts, claims int
	if err := f.db.QueryRowContext(ctx, `SELECT
		(SELECT COALESCE(SUM(quantity),0) FROM inventory_items WHERE character_id=? AND definition_id='item-001') +
		(SELECT COALESCE(SUM(quantity),0) FROM depot_items WHERE character_id=? AND definition_id='item-001'),
		(SELECT COUNT(*) FROM challenge_participants WHERE run_id=? AND character_id=? AND settled=TRUE),
		(SELECT COUNT(*) FROM challenge_character_claims WHERE run_id=?)`, f.actors[1].ID, f.actors[1].ID, run.ID, f.actors[1].ID, run.ID).Scan(&treasures, &receipts, &claims); err != nil {
		t.Fatal(err)
	}
	if treasures != 1 || receipts != 1 || claims != 0 {
		t.Fatalf("treasures=%d settlement receipts=%d remaining claims=%d", treasures, receipts, claims)
	}
	transition, err := f.repo.FindTransition(ctx, run.ID)
	if err != nil || transition.Pending {
		t.Fatalf("publication not acknowledged: %+v %v", transition, err)
	}
}

func TestRunProtocolDepartedLeaderDeletionIntegration(t *testing.T) {
	for _, account := range []bool{false, true} {
		name := "character"
		if account {
			name = "account"
		}
		t.Run(name, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			remove := f.deletionService(t, account)
			ctx := context.Background()
			run := f.departedLeader(t)
			if f.actors[0].PlayerID == f.actors[1].PlayerID {
				t.Fatal("survivor must belong to another account")
			}
			before, err := f.repo.FindCommandResult(ctx, run.ID, f.actors[0].ID, run.Revision-2)
			if err != nil || before == nil {
				t.Fatalf("departed leader receipt: %+v %v", before, err)
			}
			admission, err := f.repo.FindSessionByID(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			if err := remove(ctx); err != nil {
				t.Fatal(err)
			}
			if account {
				var count int
				if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM players WHERE id=?", f.actors[0].PlayerID).Scan(&count); err != nil || count != 0 {
					t.Fatalf("deleted account count=%d err=%v", count, err)
				}
			}
			f.assertDeletionState(t, run, false)
			retained, err := f.repo.FindSessionByID(ctx, run.ID)
			if err != nil || !reflect.DeepEqual(admission, retained) {
				t.Fatalf("deletion changed admission: %+v %v", retained, err)
			}
			after, err := f.repo.FindCommandResult(ctx, run.ID, f.actors[0].ID, before.ExpectedRevision)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("deletion changed command receipt: %+v %v", after, err)
			}
			run = f.advance(t, run)
			for i := 0; i < 2; i++ {
				if _, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision); err != nil {
					t.Fatal(err)
				}
				f.assertDeletionState(t, run, true)
			}
		})
	}
}

func TestRunProtocolDeletionDuringTerminalPublicationIntegration(t *testing.T) {
	f := newProtocolFixture(t, 2)
	remove := f.deletionService(t, false)
	ctx := context.Background()
	run := f.departedLeader(t)
	fault := &publicationFailure{ActiveSessionStore: f.store, fail: true}
	f.svc = f.service(t, fault, f.repo)
	out, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision)
	if out == nil || !errors.Is(err, challenge.ErrRecoveryRequired) {
		t.Fatalf("expected terminal publication pending: %+v %v", out, err)
	}
	if err := remove(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.FindSessionByID(ctx, run.ID); err != nil {
		t.Fatalf("pending terminal admission lost: %v", err)
	}
	fault.fail = false
	retry, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision)
	if err != nil || challenge.EncodeJSON(out) != challenge.EncodeJSON(retry) {
		t.Fatalf("terminal recovery: %+v %v", retry, err)
	}
	f.assertDeletionState(t, run, true)
}

func TestRunProtocolCompletedLeaderDeletionIntegration(t *testing.T) {
	f := newProtocolFixture(t, 1)
	remove := f.deletionService(t, false)
	ctx := context.Background()
	run := f.advance(t, f.start(t))
	if _, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision); err != nil {
		t.Fatal(err)
	}
	if err := remove(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.repo.FindSessionByID(ctx, run.ID); !errors.Is(err, challenge.ErrSessionNotFound) {
		t.Fatalf("completed admission not pruned: %v", err)
	}
	if _, err := f.chars.FindByID(ctx, f.actors[0].ID); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("completed character not deleted: %v", err)
	}
}

func TestRunProtocolLeaderDeletionRaceIntegration(t *testing.T) {
	for _, action := range []string{"advance", "exit"} {
		t.Run(action, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			remove := f.deletionService(t, false)
			run := f.departedLeader(t)
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			defer cancel()
			result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, _ int) error {
				if worker%2 == 0 {
					err := remove(ctx)
					if errors.Is(err, corecharacter.ErrNotFound) {
						return nil
					}
					return err
				}
				if action == "advance" {
					_, _, err := f.svc.AdvanceRound(ctx, run.CharacterID, run.ID, run.Revision)
					return err
				}
				_, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision)
				// Acknowledged completed admission can be pruned by a later deletion.
				if errors.Is(err, challenge.ErrSessionNotFound) {
					return nil
				}
				return err
			})
			if result.Failures != 0 {
				t.Fatalf("race failures=%d", result.Failures)
			}
			if action == "advance" {
				run = f.advance(t, run) // replay the accepted revision
				f.assertDeletionState(t, run, false)
				if _, err := f.svc.ExitSession(ctx, run.CharacterID, run.ID, run.Revision); err != nil {
					t.Fatal(err)
				}
			}
			f.assertDeletionState(t, run, true)
		})
	}
}

func TestRunProtocolDeletionServiceForeignKeyChecksIntegration(t *testing.T) {
	f := newProtocolFixture(t, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var dbName string
	if err := f.db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&dbName); err != nil {
		t.Fatal(err)
	}
	cfg, err := mysql.ParseDSN(os.Getenv("PARTY2_TEST_DB_ADMIN_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.DBName = dbName
	cfg.ParseTime = true
	baseConnector, err := mysql.NewConnector(cfg)
	if err != nil {
		t.Fatal(err)
	}
	fc := &faultConnector{Connector: baseConnector}
	f.db = sql.OpenDB(fc)
	f.db.SetMaxOpenConns(1)
	f.db.SetMaxIdleConns(1)
	t.Cleanup(func() {
		if err := f.db.Close(); err != nil {
			t.Error(err)
		}
	})
	f.chars, err = database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}

	var beforeID int64
	var fkChecks int
	if err := f.db.QueryRowContext(ctx, "SELECT CONNECTION_ID(), @@FOREIGN_KEY_CHECKS").Scan(&beforeID, &fkChecks); err != nil || fkChecks != 1 {
		t.Fatalf("initial connection=%d fkChecks=%d err=%v", beforeID, fkChecks, err)
	}

	runSetup := func() *deletionSpyTB {
		spy := &deletionSpyTB{TB: t}
		done := make(chan struct{})
		go func() {
			defer close(done)
			f.deletionService(spy, false)
		}()
		<-done
		return spy
	}

	copyErr := errors.New("injected schema copy error")
	fc.fail = func(query string) error {
		if strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS") {
			return copyErr
		}
		return nil
	}
	spy := runSetup()
	if len(spy.fatals) != 1 || !strings.Contains(spy.fatals[0], copyErr.Error()) {
		t.Fatalf("schema-copy failure fatals=%v errs=%v", spy.fatals, spy.errs)
	}
	if len(spy.errs) != 0 {
		t.Fatalf("unexpected non-fatal errors on copy failure: %v", spy.errs)
	}
	var afterID int64
	if err := f.db.QueryRowContext(ctx, "SELECT CONNECTION_ID(), @@FOREIGN_KEY_CHECKS").Scan(&afterID, &fkChecks); err != nil {
		t.Fatal(err)
	}
	if afterID != beforeID || fkChecks != 1 {
		t.Fatalf("session after copy failure: id=%d (want %d) fkChecks=%d (want 1)", afterID, beforeID, fkChecks)
	}

	restoreErr := errors.New("injected restore error")
	fc.fail = func(query string) error {
		if strings.HasPrefix(query, "CREATE TABLE IF NOT EXISTS") {
			return copyErr
		}
		if query == "SET FOREIGN_KEY_CHECKS=1" {
			return restoreErr
		}
		return nil
	}
	spy = runSetup()
	if len(spy.fatals) != 1 || !strings.Contains(spy.fatals[0], copyErr.Error()) {
		t.Fatalf("restoration failure lost original copy error: fatals=%v", spy.fatals)
	}
	if len(spy.errs) != 1 || !strings.Contains(spy.errs[0], "restore foreign key checks") || !strings.Contains(spy.errs[0], restoreErr.Error()) {
		t.Fatalf("expected restore error, got errs=%v", spy.errs)
	}
	if err := f.db.QueryRowContext(ctx, "SELECT CONNECTION_ID(), @@FOREIGN_KEY_CHECKS").Scan(&afterID, &fkChecks); err != nil {
		t.Fatal(err)
	}
	if afterID == beforeID || fkChecks != 1 {
		t.Fatalf("session after restore failure: id=%d (want != %d) fkChecks=%d (want 1)", afterID, beforeID, fkChecks)
	}

	fc.fail = nil
	remove := f.deletionService(t, false)
	if err := f.db.QueryRowContext(ctx, "SELECT @@FOREIGN_KEY_CHECKS").Scan(&fkChecks); err != nil || fkChecks != 1 {
		t.Fatalf("foreign key checks after success: value=%d err=%v", fkChecks, err)
	}
	var tableName, ddl string
	if err := f.db.QueryRowContext(ctx, "SHOW CREATE TABLE character_companion_phrases").Scan(&tableName, &ddl); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(ddl, "FOREIGN KEY") {
		t.Fatalf("copied table missing foreign key: %s", ddl)
	}
	got, err := f.chars.FindByID(ctx, f.actors[0].ID)
	if err != nil || !reflect.DeepEqual(got, f.actors[0]) {
		t.Fatalf("existing fixture character changed: %+v %v", got, err)
	}
	if err := remove(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := f.chars.FindByID(ctx, f.actors[0].ID); !errors.Is(err, corecharacter.ErrNotFound) {
		t.Fatalf("expected character deleted, got %v", err)
	}
}

type deletionSpyTB struct {
	testing.TB
	errs   []string
	fatals []string
}

func (s *deletionSpyTB) Helper() {}

func (s *deletionSpyTB) Error(args ...any) {
	s.errs = append(s.errs, fmt.Sprint(args...))
}

func (s *deletionSpyTB) Errorf(format string, args ...any) {
	s.errs = append(s.errs, fmt.Sprintf(format, args...))
}

func (s *deletionSpyTB) Fatal(args ...any) {
	s.fatals = append(s.fatals, fmt.Sprint(args...))
	runtime.Goexit()
}

type faultConnector struct {
	driver.Connector
	fail func(query string) error
}

func (c *faultConnector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.Connector.Connect(ctx)
	if err != nil {
		return nil, err
	}
	return &faultConn{Conn: conn, connector: c}, nil
}

type faultConn struct {
	driver.Conn
	connector *faultConnector
}

func (c *faultConn) BeginTx(ctx context.Context, opts driver.TxOptions) (driver.Tx, error) {
	return c.Conn.(driver.ConnBeginTx).BeginTx(ctx, opts)
}

func (c *faultConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if c.connector.fail != nil {
		if err := c.connector.fail(query); err != nil {
			return nil, err
		}
	}
	return c.Conn.(driver.ExecerContext).ExecContext(ctx, query, args)
}

func (c *faultConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	return c.Conn.(driver.QueryerContext).QueryContext(ctx, query, args)
}
