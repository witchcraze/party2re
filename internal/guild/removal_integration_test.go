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

type removalFixture struct {
	db             *sql.DB
	repo           *database.GuildRepository
	service        *guild.Service
	g              guild.Guild
	leader, target corecharacter.Character
}

// Use the same Home service letter path as production, including ambient SQL propagation.
type removalLetterAdapter struct{ service *home.Service }

func (a removalLetterAdapter) SendLetter(ctx context.Context, senderID, senderName, recipientID, recipientName, content, color string) error {
	_, err := a.service.SendLetter(ctx, senderID, recipientID, content, color)
	return err
}

func newRemovalFixture(t *testing.T, pending bool) *removalFixture {
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
	g, leader, err := database.CreateTestGuildWithLeader(context.Background(), db, "Rm_"+id.New()[:8], 20000)
	if err != nil {
		t.Fatal(err)
	}
	f := &removalFixture{db: db, repo: repo, g: g, leader: leader}
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
		for _, c := range []corecharacter.Character{f.target, leader} {
			if c.ID == "" {
				continue
			}
			if err := chars.Delete(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := players.Delete(ctx, c.PlayerID); err != nil {
				t.Error(err)
			}
		}
	})
	f.target, err = database.CreateTestCharacter(context.Background(), db, "RmTarget_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	f.service = f.newService(t, repo)
	if pending {
		err = f.service.ApplyToJoin(context.Background(), g.ID, f.target.ID)
	} else {
		_, err = f.service.Join(context.Background(), g.ID, f.target.ID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *removalFixture) newService(t *testing.T, repo guild.Repository) *guild.Service {
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
	s, err := guild.NewService(repo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)), guild.WithCharacterReader(chars), guild.WithLetterSender(removalLetterAdapter{letters}))
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (f *removalFixture) remove(ctx context.Context, s *guild.Service, reject bool) error {
	if reject {
		return s.RejectApplication(ctx, f.g.ID, f.leader.ID, f.target.ID)
	}
	return s.Kick(ctx, f.g.ID, f.leader.ID, f.target.ID)
}

func (f *removalFixture) assertRoster(t *testing.T) guild.Detail {
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

func (f *removalFixture) removalLetters(t *testing.T) []string {
	t.Helper()
	rows, err := f.db.Query("SELECT content FROM character_letters WHERE recipient_character_id = ?", f.target.ID)
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

type removalGateRepo struct {
	*database.GuildRepository
	beforeLocked, afterRead func(context.Context)
}

func (r *removalGateRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuild(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func (r *removalGateRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func awaitRemovalSignal(t *testing.T, ch <-chan struct{}) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(5 * time.Second):
		t.Fatal("interleaving gate timed out")
	}
}

func TestGuildRemoval_TransferFirst(t *testing.T) {
	for _, mode := range []struct {
		name            string
		pending, reject bool
	}{{"kickActive", false, false}, {"kickPending", true, false}, {"reject", true, true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			locked, resume, removalStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			committed := make(chan struct{})
			var started sync.Once
			transferRepo := &removalGateRepo{GuildRepository: f.repo, afterRead: func(context.Context) {
				close(locked)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}}
			transfer := f.newService(t, transferRepo)
			removalRepo := &removalGateRepo{GuildRepository: f.repo,
				beforeLocked: func(context.Context) { started.Do(func() { close(removalStarted) }) },
				// On the old unlocked implementation, wait until its snapshot exists.
				afterRead: func(c context.Context) {
					if database.TxFromContext(c) == nil {
						started.Do(func() { close(removalStarted) })
						select {
						case <-committed:
						case <-ctx.Done():
						}
					}
				},
			}
			removal := f.newService(t, removalRepo)
			transferDone, removalDone := make(chan error, 1), make(chan error, 1)
			go func() {
				transferDone <- transfer.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.target.ID)
				close(committed)
			}()
			awaitRemovalSignal(t, locked)
			go func() { removalDone <- f.remove(ctx, removal, mode.reject) }()
			awaitRemovalSignal(t, removalStarted)
			close(resume)
			if err := <-transferDone; err != nil {
				t.Fatal(err)
			}
			if err := <-removalDone; !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("former leader removal = %v, want unauthorized", err)
			}
			d := f.assertRoster(t)
			if len(d.Members) != 2 || d.Guild.LeaderCharacterID != f.target.ID {
				t.Fatalf("unexpected roster: %+v", d)
			}
			if letters := f.removalLetters(t); len(letters) != 0 {
				t.Fatalf("failed removal sent letters: %v", letters)
			}
		})
	}
}

func TestGuildRemoval_HealthyAndRollback(t *testing.T) {
	for _, mode := range []struct {
		name            string
		pending, reject bool
		marker          string
	}{{"kickActive", false, false, "【＋追放＋】"}, {"kickPending", true, false, "【＋不合格＋】"}, {"reject", true, true, "【＋不合格＋】"}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx := context.Background()
			rollback := errors.New("outer transaction rollback")
			err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
				if err := f.remove(txCtx, f.service, mode.reject); err != nil {
					return err
				}
				return rollback
			})
			if !errors.Is(err, rollback) {
				t.Fatal(err)
			}
			if d := f.assertRoster(t); len(d.Members) != 2 {
				t.Fatalf("removal escaped rollback: %+v", d)
			}
			if letters := f.removalLetters(t); len(letters) != 0 {
				t.Fatalf("rollback retained letters: %v", letters)
			}
			if err := f.remove(ctx, f.service, mode.reject); err != nil {
				t.Fatal(err)
			}
			if d := f.assertRoster(t); len(d.Members) != 1 {
				t.Fatalf("target not removed: %+v", d)
			}
			letters := f.removalLetters(t)
			if len(letters) != 1 || !strings.Contains(letters[0], mode.marker) || !strings.Contains(letters[0], f.g.Name) {
				t.Fatalf("notification = %v", letters)
			}
			if err := f.remove(ctx, f.service, mode.reject); !errors.Is(err, guild.ErrTargetNotMember) {
				t.Fatalf("repeat removal = %v", err)
			}
			if letters := f.removalLetters(t); len(letters) != 1 {
				t.Fatalf("repeat removal sent letter: %v", letters)
			}
		})
	}
}

func TestGuildRemoval_RemovalFirst(t *testing.T) {
	for _, mode := range []struct {
		name            string
		pending, reject bool
	}{{"kickActive", false, false}, {"kickPending", true, false}, {"reject", true, true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			locked, resume, transferStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			removalRepo := &removalGateRepo{GuildRepository: f.repo, afterRead: func(context.Context) {
				close(locked)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}}
			removal := f.newService(t, removalRepo)
			transferRepo := &removalGateRepo{GuildRepository: f.repo, beforeLocked: func(context.Context) { close(transferStarted) }}
			transfer := f.newService(t, transferRepo)
			removalDone, transferDone := make(chan error, 1), make(chan error, 1)
			go func() { removalDone <- f.remove(ctx, removal, mode.reject) }()
			awaitRemovalSignal(t, locked)
			go func() { transferDone <- transfer.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.target.ID) }()
			awaitRemovalSignal(t, transferStarted)
			close(resume)
			if err := <-removalDone; err != nil {
				t.Fatal(err)
			}
			if err := <-transferDone; !errors.Is(err, guild.ErrTargetNotMember) {
				t.Fatalf("removed successor transfer = %v", err)
			}
			if d := f.assertRoster(t); len(d.Members) != 1 || d.Guild.LeaderCharacterID != f.leader.ID {
				t.Fatalf("unexpected roster: %+v", d)
			}
			if letters := f.removalLetters(t); len(letters) != 1 {
				t.Fatalf("successful removal letters = %v", letters)
			}
		})
	}
}

func TestGuildRemoval_CurrentState(t *testing.T) {
	t.Run("rejection requires pending target", func(t *testing.T) {
		f := newRemovalFixture(t, true)
		ctx := context.Background()
		if err := f.service.ApproveApplication(ctx, f.g.ID, f.leader.ID, f.target.ID, "member"); err != nil {
			t.Fatal(err)
		}
		if err := f.remove(ctx, f.service, true); !errors.Is(err, guild.ErrMemberNotPending) {
			t.Fatalf("approved applicant rejection = %v", err)
		}
		if d := f.assertRoster(t); len(d.Members) != 2 {
			t.Fatalf("approved member removed: %+v", d)
		}
		letters := f.removalLetters(t)
		if len(letters) != 1 || !strings.Contains(letters[0], "【＋参加許可証＋】") {
			t.Fatalf("failed rejection notification: %v", letters)
		}
	})
	t.Run("member cannot remove leader", func(t *testing.T) {
		f := newRemovalFixture(t, false)
		ctx := context.Background()
		for _, err := range []error{f.service.Kick(ctx, f.g.ID, f.target.ID, f.leader.ID), f.service.RejectApplication(ctx, f.g.ID, f.target.ID, f.leader.ID)} {
			if !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("member removal = %v", err)
			}
		}
		if d := f.assertRoster(t); len(d.Members) != 2 {
			t.Fatalf("unauthorized removal: %+v", d)
		}
		if letters := f.removalLetters(t); len(letters) != 0 {
			t.Fatalf("unauthorized letters = %v", letters)
		}
	})
}

func TestGuildRemoval_Stress(t *testing.T) {
	for _, mode := range []struct {
		name            string
		pending, reject bool
	}{{"kickActive", false, false}, {"kickPending", true, false}, {"reject", true, true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				err1, err2 := testutil.RunRace2(
					func() error { return f.remove(ctx, f.service, mode.reject) },
					func() error { return f.service.TransferLeadership(ctx, f.g.ID, f.leader.ID, f.target.ID) },
				)
				for _, err := range []error{err1, err2} {
					if err != nil && !errors.Is(err, guild.ErrUnauthorized) && !errors.Is(err, guild.ErrTargetNotMember) {
						return err
					}
				}
				return nil
			})
			if res.Failures != 0 {
				t.Fatalf("unexpected stress failures: %+v", res)
			}
			d := f.assertRoster(t)
			letters := f.removalLetters(t)
			if d.Guild.LeaderCharacterID == f.target.ID {
				if len(d.Members) != 2 || len(letters) != 0 {
					t.Fatalf("transfer winner: roster=%+v letters=%v", d, letters)
				}
			} else if len(d.Members) != 1 || len(letters) != 1 {
				t.Fatalf("removal winner: roster=%+v letters=%v", d, letters)
			}
			t.Logf("%s: %+v", mode.name, res)
		})
	}
}
