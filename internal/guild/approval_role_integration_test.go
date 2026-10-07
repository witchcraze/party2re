package guild_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type approvalRoleFixture struct {
	db                      *sql.DB
	repo                    *database.GuildRepository
	service                 *guild.Service
	g                       guild.Guild
	leader, active, pending corecharacter.Character
}

type approvalRoleLetterAdapter struct{ service *home.Service }

func (a approvalRoleLetterAdapter) SendLetter(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
	_, err := a.service.SendLetter(ctx, senderID, recipientID, content, color)
	return err
}

func newApprovalRoleFixture(t *testing.T) *approvalRoleFixture {
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
	g, leader, err := database.CreateTestGuildWithLeader(context.Background(), db, "Ar_"+id.New()[:8], 20000)
	if err != nil {
		t.Fatal(err)
	}
	f := &approvalRoleFixture{db: db, repo: repo, g: g, leader: leader}
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
		if err := repo.DisbandGuild(ctx, g.ID); err != nil {
			t.Error(err)
		}
		for _, c := range []corecharacter.Character{f.pending, f.active, leader} {
			if c.ID == "" {
				continue
			}
			_, _ = f.db.Exec("DELETE FROM character_letters WHERE recipient_character_id = ? OR sender_character_id = ?", c.ID, c.ID)
			if err := chars.Delete(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := players.Delete(ctx, c.PlayerID); err != nil {
				t.Error(err)
			}
		}
	})
	f.active, err = database.CreateTestCharacter(context.Background(), db, "ArActive_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.pending, err = database.CreateTestCharacter(context.Background(), db, "ArPending_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.service = f.newService(t, repo)

	if _, err = f.service.Join(context.Background(), g.ID, f.active.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.service.ApplyToJoin(context.Background(), g.ID, f.pending.ID); err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *approvalRoleFixture) newService(t *testing.T, repo guild.Repository) *guild.Service {
	t.Helper()
	chars, err := database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := database.NewHomeRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	letters, err := home.NewService(homes, chars)
	if err != nil {
		t.Fatal(err)
	}
	s, err := guild.NewService(
		repo,
		guild.WithTransactionProvider(database.NewTransactionProvider(f.db)),
		guild.WithCharacterReader(chars),
		guild.WithLetterSender(approvalRoleLetterAdapter{letters}),
	)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *approvalRoleFixture) executeOp(ctx context.Context, s *guild.Service, mode string, title string) error {
	switch mode {
	case "assignActive":
		return s.AssignCustomRole(ctx, f.g.ID, f.leader.ID, f.active.ID, title)
	case "assignPending":
		return s.AssignCustomRole(ctx, f.g.ID, f.leader.ID, f.pending.ID, title)
	case "approvePending":
		return s.ApproveApplication(ctx, f.g.ID, f.leader.ID, f.pending.ID, title)
	default:
		return errors.New("unknown mode: " + mode)
	}
}

func (f *approvalRoleFixture) assertRoster(t *testing.T) guild.Detail {
	t.Helper()
	d, err := f.service.Get(context.Background(), f.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	leaders := 0
	for _, m := range d.Members {
		if m.Role == guild.RoleLeader {
			leaders++
			if m.CharacterID != d.Guild.LeaderCharacterID || m.IsPending {
				t.Fatalf("invalid leader: %+v / %+v", d.Guild, m)
			}
		}
	}
	if leaders != 1 {
		t.Fatalf("leader absent or duplicated: %+v", d)
	}
	return d
}

func (f *approvalRoleFixture) recipientLetters(t *testing.T, charID string) []string {
	t.Helper()
	rows, err := f.db.Query("SELECT content FROM character_letters WHERE recipient_character_id = ? ORDER BY created_at ASC", charID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var contents []string
	for rows.Next() {
		var content string
		if err := rows.Scan(&content); err != nil {
			t.Fatal(err)
		}
		contents = append(contents, content)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return contents
}

type approvalRoleGateRepo struct {
	*database.GuildRepository
	beforeLocked, afterRead func(context.Context)
}

func (r *approvalRoleGateRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuild(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func (r *approvalRoleGateRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func awaitApprovalRoleSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("interleaving gate timed out")
	}
}

func TestGuildApprovalRole_TransferFirst(t *testing.T) {
	for _, mode := range []string{"assignActive", "assignPending", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			locked, resume, opStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			committed := make(chan struct{})
			var started sync.Once

			var transferLockedOnce sync.Once
			transferRepo := &approvalRoleGateRepo{GuildRepository: f.repo, afterRead: func(context.Context) {
				transferLockedOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			transfer := f.newService(t, transferRepo)

			opRepo := &approvalRoleGateRepo{GuildRepository: f.repo,
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
			awaitApprovalRoleSignal(t, locked)

			go func() { opDone <- f.executeOp(ctx, opService, mode, "AuditChanged") }()
			awaitApprovalRoleSignal(t, opStarted)
			close(resume)

			if err := <-transferDone; err != nil {
				t.Fatal(err)
			}
			if err := <-opDone; !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("former leader operation %s = %v, want unauthorized", mode, err)
			}

			d := f.assertRoster(t)
			if d.Guild.LeaderCharacterID != f.active.ID {
				t.Fatalf("expected active member to be promoted leader: %+v", d.Guild)
			}

			// Verify target was not modified by the former leader
			for _, m := range d.Members {
				if m.CharacterID == f.active.ID {
					if m.Title == "AuditChanged" {
						t.Fatalf("active title was changed to %q by former leader", m.Title)
					}
				}
				if m.CharacterID == f.pending.ID {
					if !m.IsPending || m.Title == "AuditChanged" {
						t.Fatalf("pending applicant was approved or titled by former leader: %+v", m)
					}
				}
			}

			// Verify zero success letters persisted
			for _, targetID := range []string{f.active.ID, f.pending.ID} {
				if letters := f.recipientLetters(t, targetID); len(letters) != 0 {
					t.Fatalf("former leader operation sent letters to %s: %v", targetID, letters)
				}
			}
		})
	}
}

func TestGuildApprovalRole_MutationFirst(t *testing.T) {
	for _, mode := range []string{"assignActive", "assignPending", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			var lockOnce, transferStartedOnce sync.Once
			locked, resume, transferStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			opRepo := &approvalRoleGateRepo{GuildRepository: f.repo, afterRead: func(c context.Context) {
				lockOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			opService := f.newService(t, opRepo)

			transferRepo := &approvalRoleGateRepo{GuildRepository: f.repo, beforeLocked: func(context.Context) {
				transferStartedOnce.Do(func() { close(transferStarted) })
			}}
			transfer := f.newService(t, transferRepo)

			opDone, transferDone := make(chan error, 1), make(chan error, 1)
			go func() { opDone <- f.executeOp(ctx, opService, mode, "MutTitle") }()
			awaitApprovalRoleSignal(t, locked)

			go func() { transferDone <- transfer.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID) }()
			awaitApprovalRoleSignal(t, transferStarted)
			close(resume)

			if err := <-opDone; err != nil {
				t.Fatalf("op %s failed: %v", mode, err)
			}
			if err := <-transferDone; err != nil {
				t.Fatalf("transfer failed: %v", err)
			}

			d := f.assertRoster(t)
			if d.Guild.LeaderCharacterID != f.active.ID {
				t.Fatalf("leader = %s, want %s", d.Guild.LeaderCharacterID, f.active.ID)
			}

			switch mode {
			case "assignPending", "approvePending":
				for _, m := range d.Members {
					if m.CharacterID == f.pending.ID {
						if m.IsPending {
							t.Fatalf("pending applicant not approved: %+v", m)
						}
						if m.Title != "MutTitle" {
							t.Fatalf("applicant title = %q, want 'MutTitle'", m.Title)
						}
					}
				}
				letters := f.recipientLetters(t, f.pending.ID)
				if len(letters) != 1 || !strings.Contains(letters[0], "【＋参加許可証＋】") || !strings.Contains(letters[0], f.g.Name) {
					t.Fatalf("expected 1 acceptance letter, got: %v", letters)
				}
			case "assignActive":
				// Active member had title changed before transfer, but on transfer becomes leader with default title
				letters := f.recipientLetters(t, f.active.ID)
				if len(letters) != 0 {
					t.Fatalf("unexpected letters for active title change: %v", letters)
				}
			}
		})
	}

	t.Run("promoted leader rejects role assignment", func(t *testing.T) {
		f := newApprovalRoleFixture(t)
		ctx := context.Background()

		// Transfer leadership to active member first
		if err := f.service.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID); err != nil {
			t.Fatal(err)
		}

		// New leader active member cannot assign custom role to themselves
		if err := f.service.AssignCustomRole(ctx, f.g.ID, f.active.ID, f.active.ID, "大将軍"); !errors.Is(err, guild.ErrCannotAssignToLeader) {
			t.Fatalf("assign to leader = %v, want %v", err, guild.ErrCannotAssignToLeader)
		}
	})
}

func TestGuildApprovalRole_HealthyAndRollback(t *testing.T) {
	for _, mode := range []string{"assignActive", "assignPending", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx := context.Background()

			// 1. Rollback test
			rollbackErr := errors.New("outer transaction rollback")
			err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
				if err := f.executeOp(txCtx, f.service, mode, "RbTitle"); err != nil {
					return err
				}
				return rollbackErr
			})
			if !errors.Is(err, rollbackErr) {
				t.Fatalf("expected %v, got %v", rollbackErr, err)
			}

			d := f.assertRoster(t)
			for _, m := range d.Members {
				if m.CharacterID == f.active.ID && m.Title == "RbTitle" {
					t.Fatalf("active title mutation escaped rollback: %+v", m)
				}
				if m.CharacterID == f.pending.ID && (!m.IsPending || m.Title == "RbTitle") {
					t.Fatalf("pending approval escaped rollback: %+v", m)
				}
			}
			for _, charID := range []string{f.active.ID, f.pending.ID} {
				if letters := f.recipientLetters(t, charID); len(letters) != 0 {
					t.Fatalf("rollback retained letters for %s: %v", charID, letters)
				}
			}

			// 2. Healthy execution test
			if err := f.executeOp(ctx, f.service, mode, "GoodTitle"); err != nil {
				t.Fatalf("healthy op %s failed: %v", mode, err)
			}

			d = f.assertRoster(t)
			switch mode {
			case "assignActive":
				for _, m := range d.Members {
					if m.CharacterID == f.active.ID && m.Title != "GoodTitle" {
						t.Fatalf("active title = %q, want 'GoodTitle'", m.Title)
					}
				}
				if letters := f.recipientLetters(t, f.active.ID); len(letters) != 0 {
					t.Fatalf("unexpected letters for active assign: %v", letters)
				}
			case "assignPending", "approvePending":
				for _, m := range d.Members {
					if m.CharacterID == f.pending.ID {
						if m.IsPending {
							t.Fatalf("pending applicant not approved: %+v", m)
						}
						if m.Title != "GoodTitle" {
							t.Fatalf("applicant title = %q, want 'GoodTitle'", m.Title)
						}
					}
				}
				letters := f.recipientLetters(t, f.pending.ID)
				if len(letters) != 1 || !strings.Contains(letters[0], "【＋参加許可証＋】") || !strings.Contains(letters[0], f.g.Name) {
					t.Fatalf("expected 1 acceptance letter, got: %v", letters)
				}

				// Repeat approval of already approved member must fail with ErrMemberNotPending
				if err := f.service.ApproveApplication(ctx, f.g.ID, f.leader.ID, f.pending.ID, "RepeatTitle"); !errors.Is(err, guild.ErrMemberNotPending) {
					t.Fatalf("repeat approval = %v, want %v", err, guild.ErrMemberNotPending)
				}
				if letters := f.recipientLetters(t, f.pending.ID); len(letters) != 1 {
					t.Fatalf("repeat approval persisted unexpected letter: %v", letters)
				}
			}
		})
	}
}

func TestGuildApprovalRole_CurrentState(t *testing.T) {
	f := newApprovalRoleFixture(t)
	ctx := context.Background()

	t.Run("unauthorized caller cannot assign role or approve", func(t *testing.T) {
		if err := f.service.AssignCustomRole(ctx, f.g.ID, f.active.ID, f.pending.ID, "Title"); !errors.Is(err, guild.ErrUnauthorized) {
			t.Fatalf("non-leader assign = %v, want %v", err, guild.ErrUnauthorized)
		}
		if err := f.service.ApproveApplication(ctx, f.g.ID, f.active.ID, f.pending.ID, "Title"); !errors.Is(err, guild.ErrUnauthorized) {
			t.Fatalf("non-leader approve = %v, want %v", err, guild.ErrUnauthorized)
		}
	})

	t.Run("non-member target returns ErrTargetNotMember", func(t *testing.T) {
		strangerID := "stranger_" + id.New()[:8]
		if err := f.service.AssignCustomRole(ctx, f.g.ID, f.leader.ID, strangerID, "Title"); !errors.Is(err, guild.ErrTargetNotMember) {
			t.Fatalf("stranger assign = %v, want %v", err, guild.ErrTargetNotMember)
		}
		if err := f.service.ApproveApplication(ctx, f.g.ID, f.leader.ID, strangerID, "Title"); !errors.Is(err, guild.ErrTargetNotMember) {
			t.Fatalf("stranger approve = %v, want %v", err, guild.ErrTargetNotMember)
		}
	})

	t.Run("approve on active member returns ErrMemberNotPending", func(t *testing.T) {
		if err := f.service.ApproveApplication(ctx, f.g.ID, f.leader.ID, f.active.ID, "Title"); !errors.Is(err, guild.ErrMemberNotPending) {
			t.Fatalf("approve active member = %v, want %v", err, guild.ErrMemberNotPending)
		}
	})

	t.Run("assign to leader returns ErrCannotAssignToLeader", func(t *testing.T) {
		if err := f.service.AssignCustomRole(ctx, f.g.ID, f.leader.ID, f.leader.ID, "大将軍"); !errors.Is(err, guild.ErrCannotAssignToLeader) {
			t.Fatalf("assign to leader = %v, want %v", err, guild.ErrCannotAssignToLeader)
		}
	})
}

func TestGuildApprovalRole_Stress(t *testing.T) {
	for _, mode := range []string{"assignActive", "assignPending", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				err1, err2 := testutil.RunRace2(
					func() error { return f.executeOp(ctx, f.service, mode, "StressTitle") },
					func() error { return f.service.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.active.ID) },
				)
				for _, err := range []error{err1, err2} {
					if err != nil &&
						!errors.Is(err, guild.ErrUnauthorized) &&
						!errors.Is(err, guild.ErrCannotAssignToLeader) &&
						!errors.Is(err, guild.ErrMemberNotPending) &&
						!errors.Is(err, guild.ErrTargetNotMember) {
						return err
					}
				}
				return nil
			})
			if res.Failures != 0 {
				t.Fatalf("unexpected stress failures: %+v", res)
			}

			d := f.assertRoster(t)
			if d.Guild.LeaderCharacterID == f.active.ID {
				// Transfer succeeded
				t.Logf("%s: transfer leader is active member", mode)
			} else if d.Guild.LeaderCharacterID == f.leader.ID {
				t.Logf("%s: original leader retained", mode)
			} else {
				t.Fatalf("unexpected leader: %+v", d.Guild)
			}
			t.Logf("%s: %+v", mode, res)
		})
	}
}
