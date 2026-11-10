package guild_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
)

type inactiveInterceptingRepo struct {
	*database.GuildRepository
	listCompleted chan<- struct{}
	resumeCleanup <-chan struct{}
	intercepted   atomic.Bool
}

func (r *inactiveInterceptingRepo) ListInactiveGuilds(ctx context.Context, cutoff time.Time, limit int) ([]guild.Guild, error) {
	list, err := r.GuildRepository.ListInactiveGuilds(ctx, cutoff, limit)
	if err == nil {
		err = r.waitAfterList(ctx)
	}
	return list, err
}

func (r *inactiveInterceptingRepo) waitAfterList(ctx context.Context) error {
	if !r.intercepted.CompareAndSwap(false, true) {
		return nil
	}
	select {
	case r.listCompleted <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case <-r.resumeCleanup:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func TestInactiveInterceptingRepo_Cancellation(t *testing.T) {
	for _, stage := range []string{"before_selection", "waiting_for_resume"} {
		t.Run(stage, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			listCompleted := make(chan struct{})
			repo := &inactiveInterceptingRepo{
				listCompleted: listCompleted,
				resumeCleanup: make(chan struct{}),
			}
			result := make(chan error, 1)
			stopped := make(chan struct{})
			go func() {
				defer close(stopped)
				result <- repo.waitAfterList(ctx)
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-stopped:
				case <-time.After(5 * time.Second):
					t.Error("inactive-list worker did not stop after cancellation")
					<-stopped
				}
			})
			if stage == "waiting_for_resume" {
				select {
				case <-listCompleted:
				case <-time.After(5 * time.Second):
					t.Fatal("worker did not reach resume barrier")
				}
			}
			cancel()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not stop after cancellation")
			}
			if err := <-result; !errors.Is(err, context.Canceled) {
				t.Fatalf("expected cancellation, got %v", err)
			}
		})
	}
}

func TestGuildService_DisbandInactiveGuilds_ReactivatedGuildPreserved_RealDB(t *testing.T) {
	db := guildIsolatedDB(t)

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	service, err := guild.NewService(guildRepo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := database.CreateTestCharacter(ctx, db, "InactiveLdr1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("InactG_%s", leaderChar.ID[:8])
	g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	now := time.Now().UTC()
	// Set last_active_at to 25 days ago (older than 20-day cutoff)
	oldTime := now.Add(-25 * 24 * time.Hour)
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", oldTime, g.ID); err != nil {
		t.Fatalf("failed to age guild: %v", err)
	}

	// Verify guild appears in ListInactiveGuilds
	cutoff := now.Add(-guild.InactivityDisbandDuration)
	inactive, err := guildRepo.ListInactiveGuilds(ctx, cutoff, 100)
	if err != nil {
		t.Fatalf("ListInactiveGuilds failed: %v", err)
	}
	var found bool
	for _, in := range inactive {
		if in.ID == g.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected guild %s to be listed as inactive", g.ID)
	}

	// Reactivate guild before cleanup runs
	if err := guildRepo.TouchActive(ctx, g.ID); err != nil {
		t.Fatalf("TouchActive failed: %v", err)
	}

	// Run DisbandInactiveGuilds
	_, disbanded, err := service.DisbandInactiveGuilds(ctx, now, 100)
	if err != nil {
		t.Fatalf("DisbandInactiveGuilds failed: %v", err)
	}
	for _, id := range disbanded {
		if id == g.ID {
			t.Fatalf("reactivated guild %s should not have been disbanded", g.ID)
		}
	}

	// Verify guild and members are completely preserved in DB
	gDetail, err := service.Get(ctx, g.ID)
	if err != nil {
		t.Fatalf("Get guild failed: %v (guild should still exist!)", err)
	}
	if len(gDetail.Members) != 1 || gDetail.Members[0].CharacterID != leaderChar.ID {
		t.Errorf("unexpected members after preserved check: %+v", gDetail.Members)
	}
	if gDetail.Guild.LastActiveAt.Before(cutoff) {
		t.Errorf("expected guild last_active_at >= cutoff, got %v", gDetail.Guild.LastActiveAt)
	}
}

func TestGuildService_DisbandInactiveGuilds_DeterministicInterleaving_RealDB(t *testing.T) {
	db := guildIsolatedDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	realRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	listCompleted := make(chan struct{})
	resumeCleanup := make(chan struct{})
	repo := &inactiveInterceptingRepo{
		GuildRepository: realRepo,
		listCompleted:   listCompleted,
		resumeCleanup:   resumeCleanup,
	}

	txProvider := database.NewTransactionProvider(db)
	service, err := guild.NewService(repo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	leaderChar, err := database.CreateTestCharacter(ctx, db, "DetInactiveLdr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("DetIn_%s", leaderChar.ID[:8])
	g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	now := time.Now().UTC()
	// Set last_active_at to 25 days ago
	oldTime := now.Add(-25 * 24 * time.Hour)
	if _, err := db.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", oldTime, g.ID); err != nil {
		t.Fatalf("failed to age guild: %v", err)
	}

	// 1. Start DisbandInactiveGuilds in background.
	// It extracts candidate guilds in ListInactiveGuilds and pauses before deletion.
	type cleanupResult struct {
		disbanded []string
		err       error
	}
	cleanupDone := make(chan cleanupResult, 1)
	workerStopped := make(chan struct{})
	go func() {
		defer close(workerStopped)
		_, d, err := service.DisbandInactiveGuilds(ctx, now, 100)
		cleanupDone <- cleanupResult{disbanded: d, err: err}
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case <-workerStopped:
		case <-time.After(5 * time.Second):
			// Do not tear down a database still used by a stuck worker.
			t.Error("guild cleanup worker did not stop after cancellation")
			<-workerStopped
		}
	})

	// Wait until candidates are selected
	select {
	case <-listCompleted:
	case <-ctx.Done():
		t.Fatalf("waiting for candidate selection: %v", ctx.Err())
	}

	// 2. Concurrently reactivate the guild while cleanup is paused between candidate selection and deletion!
	if err := realRepo.TouchActive(ctx, g.ID); err != nil {
		t.Fatalf("concurrent TouchActive failed: %v", err)
	}

	// 3. Resume cleanup. It must recheck last_active_at under Rank-7 lock and preserve the reactivated guild!
	close(resumeCleanup)

	var res cleanupResult
	select {
	case res = <-cleanupDone:
		<-workerStopped
		if res.err != nil {
			t.Fatalf("DisbandInactiveGuilds returned error: %v", res.err)
		}
	case <-ctx.Done():
		t.Fatalf("waiting for cleanup to complete: %v", ctx.Err())
	}

	for _, id := range res.disbanded {
		if id == g.ID {
			t.Fatalf("reactivated guild %s was disbanded by cleanup despite recent activity!", g.ID)
		}
	}

	// Invariant verification: guild and member records remain intact in DB
	gDetail, err := service.Get(ctx, g.ID)
	if err != nil {
		t.Fatalf("expected surviving guild, got error: %v", err)
	}
	if len(gDetail.Members) != 1 || gDetail.Members[0].CharacterID != leaderChar.ID {
		t.Fatalf("surviving guild member corrupted: %+v", gDetail.Members)
	}
}

func TestGuildService_DisbandInactiveGuilds_ConcurrentRace_RealDB(t *testing.T) {
	db := guildIsolatedDB(t)

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	service, err := guild.NewService(guildRepo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run multiple paired races between cleanup and concurrent activity
	const iterations = 5
	for iter := 0; iter < iterations; iter++ {
		leaderChar, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaceInactLdr_%d", iter))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
			t.Fatal(err)
		}

		guildName := fmt.Sprintf("RcIn_%d_%s", iter, leaderChar.ID[:6])
		g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		now := time.Now().UTC()
		oldTime := now.Add(-25 * 24 * time.Hour)
		if _, err := db.ExecContext(ctx, "UPDATE guilds SET last_active_at = ? WHERE id = ?", oldTime, g.ID); err != nil {
			t.Fatalf("failed to age guild: %v", err)
		}

		// Concurrently race DisbandInactiveGuilds and TouchActive
		err1, err2 := database.RunRace2(
			func() error {
				_, _, err := service.DisbandInactiveGuilds(ctx, now, 100)
				return err
			},
			func() error {
				return guildRepo.TouchActive(ctx, g.ID)
			},
		)

		if database.IsDeadlockError(err1) || database.IsDeadlockError(err2) {
			t.Fatalf("iter %d: DEADLOCK detected! err1=%v, err2=%v", iter, err1, err2)
		}
		if err1 != nil {
			t.Fatalf("iter %d: DisbandInactiveGuilds error: %v", iter, err1)
		}
		if err2 != nil {
			t.Fatalf("iter %d: TouchActive error: %v", iter, err2)
		}

		// Invariant verification:
		// Either the guild survived with intact members, or it was cleanly disbanded (0 guild rows, 0 member rows).
		gDetail, err := service.Get(ctx, g.ID)
		if err == nil {
			// Guild survived
			if len(gDetail.Members) == 0 {
				t.Fatalf("iter %d: INVARIANT VIOLATION: surviving guild %s has 0 members", iter, g.ID)
			}
		} else if errors.Is(err, guild.ErrGuildNotFound) {
			// Disbanded: verify 0 rows in guilds and guild_members
			var memberCount int
			if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_members WHERE guild_id = ?", g.ID).Scan(&memberCount); err != nil {
				t.Fatal(err)
			}
			if memberCount != 0 {
				t.Fatalf("iter %d: INVARIANT VIOLATION: disbanded guild %s still has %d member rows", iter, g.ID, memberCount)
			}
		} else {
			t.Fatalf("iter %d: unexpected Get error: %v", iter, err)
		}
	}
}

func TestGuildRepository_DisbandGuild_RowsAffectedAndRollback_RealDB(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// 1. Calling DisbandGuild on non-existent guild returns ErrGuildNotFound
	nonExistentID := fmt.Sprintf("g_nonexist_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	err = guildRepo.DisbandGuild(ctx, nonExistentID)
	if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Fatalf("expected ErrGuildNotFound for non-existent guild, got %v", err)
	}

	// 2. Disband an existing guild once (succeeds), then disband again (returns ErrGuildNotFound)
	leaderChar, err := database.CreateTestCharacter(ctx, db, "DisbandOnceLdr")
	if err != nil {
		t.Fatal(err)
	}
	guildName := fmt.Sprintf("DisbandOnce_%08x", time.Now().UnixNano()%0xFFFFFFFF)
	gCreated, _, err := database.CreateTestGuildWithLeader(ctx, db, guildName, 10000)
	if err != nil {
		t.Fatal(err)
	}
	_ = leaderChar

	// First disband: succeeds
	if err := guildRepo.DisbandGuild(ctx, gCreated.ID); err != nil {
		t.Fatalf("first DisbandGuild failed: %v", err)
	}

	// Second disband on same ID: returns ErrGuildNotFound (not nil)
	err = guildRepo.DisbandGuild(ctx, gCreated.ID)
	if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Fatalf("expected ErrGuildNotFound on second DisbandGuild, got %v", err)
	}
}
