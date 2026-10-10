package guild_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type mutationLockFixture struct {
	db             *sql.DB
	repo           *database.GuildRepository
	service        *guild.Service
	g              guild.Guild
	leader, active corecharacter.Character
	initialMoney   int
}

type mutationLockGateRepo struct {
	*database.GuildRepository
	beforeLocked, afterRead func(context.Context)
}

func (r *mutationLockGateRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuild(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func (r *mutationLockGateRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func newMutationLockFixture(t *testing.T) *mutationLockFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	repo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	initFunds := 50000
	g, leader, err := database.CreateTestGuildWithLeader(context.Background(), db, "MutL_"+id.New()[:8], initFunds)
	if err != nil {
		t.Fatal(err)
	}
	f := &mutationLockFixture{db: db, repo: repo, g: g, leader: leader, initialMoney: initFunds}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		_ = repo.DisbandGuild(ctx, g.ID)
		for _, c := range []corecharacter.Character{f.active, leader} {
			if c.ID == "" {
				continue
			}
			_, _ = f.db.Exec("DELETE FROM character_letters WHERE recipient_character_id = ? OR sender_character_id = ?", c.ID, c.ID)
			_ = chars.Delete(ctx, c.ID)
			_ = players.Delete(ctx, c.PlayerID)
		}
	})
	f.active, err = database.CreateTestCharacter(context.Background(), db, "MutAct_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.service = f.newService(t, repo)

	if _, err = f.service.Join(context.Background(), g.ID, f.active.ID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *mutationLockFixture) newService(t *testing.T, repo guild.Repository) *guild.Service {
	t.Helper()
	txProvider := database.NewTransactionProvider(f.db)
	svc, err := guild.NewService(repo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

func (f *mutationLockFixture) getLeaderMoney(t *testing.T) int {
	t.Helper()
	var money int
	err := f.db.QueryRow("SELECT money FROM characters WHERE id = ?", f.leader.ID).Scan(&money)
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func (f *mutationLockFixture) executeOp(ctx context.Context, svc *guild.Service, mode string) error {
	switch mode {
	case "disband":
		return svc.Disband(ctx, f.g.ID, f.leader.ID)
	case "color":
		return svc.UpdateColor(ctx, f.g.ID, f.leader.ID, "#112233")
	case "notice":
		return svc.UpdateNotice(ctx, f.g.ID, f.leader.ID, "AuditNoticeUpdate")
	case "mark":
		_, err := svc.ChangeMark(ctx, f.g.ID, f.leader.ID, "99")
		return err
	case "wallpaper":
		_, err := svc.ChangeWallpaper(ctx, f.g.ID, f.leader.ID, "stage0")
		return err
	default:
		return fmt.Errorf("unknown mode: %s", mode)
	}
}

func awaitMutationLockSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("interleaving gate timed out")
	}
}

func TestGuildMutationLock_TransferFirst(t *testing.T) {
	modes := []string{"disband", "color", "notice", "mark", "wallpaper"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			f := newMutationLockFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			locked, resume, opStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			committed := make(chan struct{})
			var started sync.Once

			var transferLockedOnce sync.Once
			transferRepo := &mutationLockGateRepo{GuildRepository: f.repo, afterRead: func(context.Context) {
				transferLockedOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			transfer := f.newService(t, transferRepo)

			opRepo := &mutationLockGateRepo{GuildRepository: f.repo,
				beforeLocked: func(context.Context) { started.Do(func() { close(opStarted) }) },
				afterRead: func(c context.Context) {
					if database.TxFromContext(c) == nil {
						started.Do(func() { close(opStarted) })
						select {
						case <-committed:
						case <-ctx.Done():
						}
					}
				},
			}
			opService := f.newService(t, opRepo)

			transferDone, opDone := make(chan error, 1), make(chan error, 1)
			go func() {
				transferDone <- transfer.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID)
				close(committed)
			}()
			awaitMutationLockSignal(t, locked)

			go func() { opDone <- f.executeOp(ctx, opService, mode) }()
			awaitMutationLockSignal(t, opStarted)
			close(resume)

			if err := <-transferDone; err != nil {
				t.Fatalf("transfer failed: %v", err)
			}
			if err := <-opDone; !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("former leader operation %s = %v, want ErrUnauthorized", mode, err)
			}

			// Verify former leader's money was not deducted
			currentMoney := f.getLeaderMoney(t)
			if currentMoney != f.initialMoney {
				t.Fatalf("money changed on rejected operation: got %d, want %d", currentMoney, f.initialMoney)
			}

			// Verify guild survived and active member is now leader
			gDetail, members, err := f.repo.GetGuild(context.Background(), f.g.ID)
			if err != nil {
				t.Fatalf("expected guild to survive transfer: %v", err)
			}
			if gDetail.LeaderCharacterID != f.active.ID {
				t.Fatalf("expected leader to be %s, got %s", f.active.ID, gDetail.LeaderCharacterID)
			}
			if mode == "color" && gDetail.Color == "#112233" {
				t.Fatal("color was modified by former leader")
			}
			if mode == "notice" && gDetail.Notice == "AuditNoticeUpdate" {
				t.Fatal("notice was modified by former leader")
			}
			if mode == "mark" && gDetail.Mark == "99" {
				t.Fatal("mark was modified by former leader")
			}
			if mode == "wallpaper" && gDetail.Bgimg == "stage0.gif" {
				t.Fatal("wallpaper was modified by former leader")
			}
			_ = members
		})
	}
}

func TestGuildMutationLock_MutationFirst(t *testing.T) {
	modes := []string{"disband", "color", "notice", "mark", "wallpaper"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			f := newMutationLockFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var lockOnce, transferStartedOnce sync.Once
			locked, resume, transferStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			opRepo := &mutationLockGateRepo{GuildRepository: f.repo, afterRead: func(c context.Context) {
				lockOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			opService := f.newService(t, opRepo)

			transferRepo := &mutationLockGateRepo{GuildRepository: f.repo, beforeLocked: func(context.Context) {
				transferStartedOnce.Do(func() { close(transferStarted) })
			}}
			transfer := f.newService(t, transferRepo)

			opDone, transferDone := make(chan error, 1), make(chan error, 1)
			go func() { opDone <- f.executeOp(ctx, opService, mode) }()
			awaitMutationLockSignal(t, locked)

			go func() { transferDone <- transfer.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID) }()
			awaitMutationLockSignal(t, transferStarted)
			close(resume)

			if err := <-opDone; err != nil {
				t.Fatalf("op %s failed: %v", mode, err)
			}

			if mode == "disband" {
				// Guild was disbanded first; TransferLeadership must fail with ErrGuildNotFound
				if err := <-transferDone; !errors.Is(err, guild.ErrGuildNotFound) {
					t.Fatalf("transfer after disband = %v, want ErrGuildNotFound", err)
				}
				// Guild must not exist
				_, _, err := f.repo.GetGuild(context.Background(), f.g.ID)
				if !errors.Is(err, guild.ErrGuildNotFound) {
					t.Fatalf("expected guild not found after disband, got %v", err)
				}
			} else {
				// Mutation succeeded, then transfer succeeded
				if err := <-transferDone; err != nil {
					t.Fatalf("transfer failed: %v", err)
				}
				gDetail, _, err := f.repo.GetGuild(context.Background(), f.g.ID)
				if err != nil {
					t.Fatalf("expected guild to exist: %v", err)
				}
				if gDetail.LeaderCharacterID != f.active.ID {
					t.Fatalf("expected active member to be promoted leader: got %s, want %s", gDetail.LeaderCharacterID, f.active.ID)
				}

				switch mode {
				case "color":
					if gDetail.Color != "#112233" {
						t.Fatalf("color = %s, want '#112233'", gDetail.Color)
					}
				case "notice":
					if gDetail.Notice != "AuditNoticeUpdate" {
						t.Fatalf("notice = %s, want 'AuditNoticeUpdate'", gDetail.Notice)
					}
				case "mark":
					if gDetail.Mark != "99" {
						t.Fatalf("mark = %s, want '99'", gDetail.Mark)
					}
					if money := f.getLeaderMoney(t); money != f.initialMoney-guild.MarkChangeFee {
						t.Fatalf("mark fee not deducted correctly: money = %d, want %d", money, f.initialMoney-guild.MarkChangeFee)
					}
				case "wallpaper":
					if gDetail.Bgimg != "stage0.gif" {
						t.Fatalf("bgimg = %s, want 'stage0.gif'", gDetail.Bgimg)
					}
					price := 6000
					if money := f.getLeaderMoney(t); money != f.initialMoney-price {
						t.Fatalf("wallpaper fee not deducted correctly: money = %d, want %d", money, f.initialMoney-price)
					}
				}
			}
		})
	}
}

func TestGuildMutationLock_Stress(t *testing.T) {
	modes := []string{"color", "notice", "mark", "wallpaper"}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			f := newMutationLockFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				err1, err2 := testutil.RunRace2(
					func() error { return f.executeOp(ctx, f.service, mode) },
					func() error { return f.service.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID) },
				)
				for _, err := range []error{err1, err2} {
					if err != nil &&
						!errors.Is(err, guild.ErrUnauthorized) &&
						!errors.Is(err, guild.ErrGuildNotFound) &&
						!errors.Is(err, guild.ErrTargetNotMember) {
						return err
					}
				}
				return nil
			})
			if res.Failures != 0 {
				t.Fatalf("unexpected stress failures: %+v", res)
			}

			gDetail, _, err := f.repo.GetGuild(context.Background(), f.g.ID)
			if err != nil {
				t.Fatalf("expected guild to exist: %v", err)
			}
			if gDetail.LeaderCharacterID != f.active.ID && gDetail.LeaderCharacterID != f.leader.ID {
				t.Fatalf("unexpected leader: %+v", gDetail)
			}
		})
	}
}
