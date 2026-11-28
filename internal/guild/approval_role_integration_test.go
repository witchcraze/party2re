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
	ctx                     context.Context
	cancel                  context.CancelFunc
	db                      *sql.DB
	repo                    *database.GuildRepository
	service                 *guild.Service
	g                       guild.Guild
	leader, active, pending corecharacter.Character
	stopTimeout             time.Duration
}

type approvalRoleLetterAdapter struct{ service *home.Service }

func (a approvalRoleLetterAdapter) SendLetter(ctx context.Context, senderID, recipientID, content string) error {
	_, err := a.service.SendLetter(ctx, senderID, recipientID, content)
	return err
}

func newApprovalRoleFixture(t *testing.T) *approvalRoleFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	// Fixture creation and SQL share the scenario budget; worker stop detection is separate.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	start := time.Now()
	defer func() { t.Logf("fixture: %s", time.Since(start)) }()
	db := database.OpenTestDB(t)
	repo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	g, leader, err := database.CreateTestGuildWithLeader(ctx, db, "Ar_"+id.New()[:8], 20000)
	if err != nil {
		t.Fatal(err)
	}
	f := &approvalRoleFixture{ctx: ctx, cancel: cancel, db: db, repo: repo, g: g, leader: leader}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		start := time.Now()
		defer func() { t.Logf("cleanup: %s; pool: %+v", time.Since(start), db.Stats()) }()
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		if err := repo.DisbandGuild(ctx, g.ID); err != nil {
			t.Error(err)
		}
		for _, c := range []corecharacter.Character{f.pending, f.active, leader} {
			if c.ID == "" {
				continue
			}
			if _, err := f.db.ExecContext(ctx, "DELETE FROM character_letters WHERE recipient_character_id = ? OR sender_character_id = ?", c.ID, c.ID); err != nil {
				t.Error(err)
			}
			if err := chars.Delete(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := players.Delete(ctx, c.PlayerID); err != nil {
				t.Error(err)
			}
		}
	})
	f.active, err = database.CreateTestCharacter(ctx, db, "ArActive_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.pending, err = database.CreateTestCharacter(ctx, db, "ArPending_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.service = f.newService(t, repo)

	addActiveMember(t, repo, g.ID, f.active.ID)
	if err = f.service.ApplyToJoin(ctx, g.ID, f.pending.ID); err != nil {
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
		return assigned(s.Assign(ctx, f.g.ID, f.leader.ID, f.active.ID, title))
	case "approvePending":
		return assigned(s.Assign(ctx, f.g.ID, f.leader.ID, f.pending.ID, title))
	default:
		return errors.New("unknown mode: " + mode)
	}
}

func (f *approvalRoleFixture) assertRoster(t *testing.T) guild.Detail {
	t.Helper()
	d, err := f.service.Get(f.ctx, f.g.ID)
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
	rows, err := f.db.QueryContext(f.ctx, "SELECT content FROM character_letters WHERE recipient_character_id = ? ORDER BY created_at ASC", charID)
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
	t                       *testing.T
	beforeLocked, afterRead func(context.Context)
	// afterLocked skips Leave's unlocked roster discovery and fires once the guild row is locked.
	afterLocked func(context.Context)
}

func (r *approvalRoleGateRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	start := time.Now()
	g, members, err := r.GuildRepository.GetGuild(ctx, guildID)
	r.t.Logf("GetGuild SQL: %s, err=%v", time.Since(start), err)
	if err == nil && r.afterRead != nil {
		start = time.Now()
		r.afterRead(ctx)
		r.t.Logf("GetGuild barrier: %s", time.Since(start))
	}
	return g, members, err
}

func (r *approvalRoleGateRepo) LockCharacterForUpdate(ctx context.Context, characterID string) error {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	start := time.Now()
	err := r.GuildRepository.LockCharacterForUpdate(ctx, characterID)
	r.t.Logf("LockCharacterForUpdate SQL: %s, err=%v", time.Since(start), err)
	return err
}

func (r *approvalRoleGateRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	start := time.Now()
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	r.t.Logf("GetGuildForUpdate SQL: %s, err=%v", time.Since(start), err)
	if err == nil && r.afterRead != nil {
		start = time.Now()
		r.afterRead(ctx)
		r.t.Logf("GetGuildForUpdate barrier: %s", time.Since(start))
	}
	if err == nil && r.afterLocked != nil {
		r.afterLocked(ctx)
	}
	return g, members, err
}

func awaitApprovalRoleSignal(ctx context.Context, ch <-chan struct{}) error {
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (f *approvalRoleFixture) startWorker(t *testing.T, name string, fn func() error) func() error {
	t.Helper()
	result, stopped := make(chan error, 1), make(chan struct{})
	go func() {
		defer close(stopped)
		start := time.Now()
		err := fn()
		t.Logf("%s worker (including gates): %s, err=%v; pool: %+v", name, time.Since(start), err, f.db.Stats())
		result <- err
	}()
	stopTimeout := f.stopTimeout
	if stopTimeout <= 0 {
		stopTimeout = 5 * time.Second
	}
	join := func() {
		t.Helper()
		select {
		case <-stopped:
			return
		case <-f.ctx.Done():
		}
		select {
		case <-stopped:
		case <-time.After(stopTimeout):
			t.Error(name + " worker did not stop after cancellation")
			// Keep fixture cleanup behind worker termination, even after the diagnostic.
			<-stopped
		}
	}
	t.Cleanup(func() {
		f.cancel() // Releases every context-aware barrier before fixture cleanup.
		join()
	})
	return func() error {
		join()
		return <-result
	}
}

func TestGuildApprovalRole_LeaderLeaveFirst(t *testing.T) {
	for _, mode := range []string{"assignActive", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx := f.ctx

			locked, resume, opStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			committed := make(chan struct{})
			var started sync.Once

			var leaveLockedOnce sync.Once
			leaveRepo := &approvalRoleGateRepo{GuildRepository: f.repo, t: t, afterLocked: func(context.Context) {
				leaveLockedOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			leave := f.newService(t, leaveRepo)

			opRepo := &approvalRoleGateRepo{GuildRepository: f.repo, t: t,
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

			leaveDone := f.startWorker(t, "leave", func() error {
				err := leave.Leave(ctx, f.g.ID, f.leader.ID)
				close(committed)
				return err
			})
			if err := awaitApprovalRoleSignal(ctx, locked); err != nil {
				t.Fatalf("leave lock gate: %v", err)
			}

			opDone := f.startWorker(t, "mutation", func() error { return f.executeOp(ctx, opService, mode, "AuditChanged") })
			if err := awaitApprovalRoleSignal(ctx, opStarted); err != nil {
				t.Fatalf("mutation start gate: %v", err)
			}
			close(resume)

			leaveErr, opErr := leaveDone(), opDone()
			if leaveErr != nil {
				t.Fatal(leaveErr)
			}
			if !errors.Is(opErr, guild.ErrUnauthorized) {
				t.Fatalf("former leader operation %s = %v, want unauthorized", mode, opErr)
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
	for _, mode := range []string{"assignActive", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx := f.ctx

			var lockOnce, leaveStartedOnce sync.Once
			locked, resume, leaveStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			opRepo := &approvalRoleGateRepo{GuildRepository: f.repo, t: t, afterRead: func(c context.Context) {
				lockOnce.Do(func() {
					close(locked)
					select {
					case <-resume:
					case <-ctx.Done():
					}
				})
			}}
			opService := f.newService(t, opRepo)

			leaveRepo := &approvalRoleGateRepo{GuildRepository: f.repo, t: t, beforeLocked: func(context.Context) {
				leaveStartedOnce.Do(func() { close(leaveStarted) })
			}}
			leave := f.newService(t, leaveRepo)

			opDone := f.startWorker(t, "mutation", func() error { return f.executeOp(ctx, opService, mode, "MutTitle") })
			if err := awaitApprovalRoleSignal(ctx, locked); err != nil {
				t.Fatalf("mutation lock gate: %v", err)
			}

			leaveDone := f.startWorker(t, "leave", func() error { return leave.Leave(ctx, f.g.ID, f.leader.ID) })
			if err := awaitApprovalRoleSignal(ctx, leaveStarted); err != nil {
				t.Fatalf("leave start gate: %v", err)
			}
			close(resume)

			opErr, leaveErr := opDone(), leaveDone()
			if opErr != nil {
				t.Fatalf("op %s failed: %v", mode, opErr)
			}
			if leaveErr != nil {
				t.Fatalf("leave failed: %v", leaveErr)
			}

			d := f.assertRoster(t)
			if d.Guild.LeaderCharacterID != f.active.ID {
				t.Fatalf("leader = %s, want %s", d.Guild.LeaderCharacterID, f.active.ID)
			}

			switch mode {
			case "approvePending":
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
				// The retitled active member then succeeds the departing leader.
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

		// The leader departs, so the active member succeeds first
		if err := f.service.Leave(ctx, f.g.ID, f.leader.ID); err != nil {
			t.Fatal(err)
		}

		// New leader active member cannot assign custom role to themselves
		if err := assigned(f.service.Assign(ctx, f.g.ID, f.active.ID, f.active.ID, "大将軍")); !errors.Is(err, guild.ErrCannotAssignToLeader) {
			t.Fatalf("assign to leader = %v, want %v", err, guild.ErrCannotAssignToLeader)
		}
	})
}

func TestGuildApprovalRole_HealthyAndRollback(t *testing.T) {
	for _, mode := range []string{"assignActive", "approvePending"} {
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
			case "approvePending":
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

				// Assigning the approved member again only retitles it (guild.cgi:247-249).
				if outcome, err := f.service.Assign(ctx, f.g.ID, f.leader.ID, f.pending.ID, "RepeatTitle"); err != nil || outcome != guild.AssignRetitled {
					t.Fatalf("repeat assign = %q, %v; want retitled", outcome, err)
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

	t.Run("unauthorized caller cannot assign", func(t *testing.T) {
		for _, title := range []string{"Title", guild.RemovalTitle} {
			if err := assigned(f.service.Assign(ctx, f.g.ID, f.active.ID, f.pending.ID, title)); !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("non-leader assign %s = %v, want %v", title, err, guild.ErrUnauthorized)
			}
		}
	})

	t.Run("non-member target returns ErrTargetNotMember", func(t *testing.T) {
		strangerID := "stranger_" + id.New()[:8]
		if err := assigned(f.service.Assign(ctx, f.g.ID, f.leader.ID, strangerID, "Title")); !errors.Is(err, guild.ErrTargetNotMember) {
			t.Fatalf("stranger assign = %v, want %v", err, guild.ErrTargetNotMember)
		}
	})

	t.Run("an active member is retitled", func(t *testing.T) {
		if outcome, err := f.service.Assign(ctx, f.g.ID, f.leader.ID, f.active.ID, "Title"); err != nil || outcome != guild.AssignRetitled {
			t.Fatalf("assign active member = %q, %v; want retitled", outcome, err)
		}
	})

	t.Run("assign to leader returns ErrCannotAssignToLeader", func(t *testing.T) {
		if err := assigned(f.service.Assign(ctx, f.g.ID, f.leader.ID, f.leader.ID, "大将軍")); !errors.Is(err, guild.ErrCannotAssignToLeader) {
			t.Fatalf("assign to leader = %v, want %v", err, guild.ErrCannotAssignToLeader)
		}
	})
}

func TestGuildApprovalRole_Stress(t *testing.T) {
	for _, mode := range []string{"assignActive", "approvePending"} {
		t.Run(mode, func(t *testing.T) {
			f := newApprovalRoleFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()

			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				err1, err2 := testutil.RunRace2(
					func() error { return f.executeOp(ctx, f.service, mode, "StressTitle") },
					func() error { return f.service.Leave(ctx, f.g.ID, f.leader.ID) },
				)
				for _, err := range []error{err1, err2} {
					if err != nil &&
						!errors.Is(err, guild.ErrUnauthorized) &&
						!errors.Is(err, guild.ErrCharacterNotInGuild) &&
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

			// The first departure always commits, handing over to the earliest-joined active member.
			if d := f.assertRoster(t); d.Guild.LeaderCharacterID != f.active.ID {
				t.Fatalf("unexpected leader: %+v", d.Guild)
			}
			t.Logf("%s: %+v", mode, res)
		})
	}
}
