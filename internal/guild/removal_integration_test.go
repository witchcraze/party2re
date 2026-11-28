package guild_test

import (
	"context"
	"database/sql"
	"errors"
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

func (a removalLetterAdapter) SendLetter(ctx context.Context, senderID, recipientID, content string) error {
	_, err := a.service.SendLetter(ctx, senderID, recipientID, content)
	return err
}

func newRemovalFixture(t *testing.T, pending bool) *removalFixture {
	t.Helper()
	db := database.OpenTestDB(t)
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
		if err := repo.DisbandGuild(ctx, g.ID); err != nil && !errors.Is(err, guild.ErrGuildNotFound) {
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
	if !pending {
		addActiveMember(t, repo, g.ID, f.target.ID)
	} else if err = f.service.ApplyToJoin(context.Background(), g.ID, f.target.ID); err != nil {
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

// remove is ＠あたえる>追放: rejection for a pending target, expulsion otherwise.
func (f *removalFixture) remove(ctx context.Context, s *guild.Service) error {
	return assigned(s.Assign(ctx, f.g.ID, f.leader.ID, f.target.ID, guild.RemovalTitle))
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
	// afterLocked skips Leave's unlocked roster discovery and fires once the guild row is locked.
	afterLocked func(context.Context)
}

func (r *removalGateRepo) LockCharacterForUpdate(ctx context.Context, characterID string) error {
	if r.beforeLocked != nil {
		r.beforeLocked(ctx)
	}
	return r.GuildRepository.LockCharacterForUpdate(ctx, characterID)
}

func (r *removalGateRepo) GetGuild(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuild(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	return g, members, err
}

func (r *removalGateRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err == nil && r.afterRead != nil {
		r.afterRead(ctx)
	}
	if err == nil && r.afterLocked != nil {
		r.afterLocked(ctx)
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

func TestGuildRemoval_LeaderLeaveFirst(t *testing.T) {
	for _, mode := range []struct {
		name    string
		pending bool
	}{{"expelActive", false}, {"rejectPending", true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			locked, resume, removalStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			committed := make(chan struct{})
			var started sync.Once
			var lockedOnce sync.Once
			leaveRepo := &removalGateRepo{GuildRepository: f.repo, afterLocked: func(context.Context) {
				lockedOnce.Do(func() { close(locked) })
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}}
			leave := f.newService(t, leaveRepo)
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
			leaveDone, removalDone := make(chan error, 1), make(chan error, 1)
			go func() {
				leaveDone <- leave.Leave(ctx, f.g.ID, f.leader.ID)
				close(committed)
			}()
			awaitRemovalSignal(t, locked)
			go func() { removalDone <- f.remove(ctx, removal) }()
			awaitRemovalSignal(t, removalStarted)
			close(resume)
			if err := <-leaveDone; err != nil {
				t.Fatal(err)
			}
			if err := <-removalDone; !errors.Is(err, guild.ErrUnauthorized) {
				t.Fatalf("former leader removal = %v, want unauthorized", err)
			}
			d := f.assertRoster(t)
			if len(d.Members) != 1 || d.Guild.LeaderCharacterID != f.target.ID {
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
		name    string
		pending bool
		marker  string
	}{{"expelActive", false, "【＋追放＋】"}, {"rejectPending", true, "【＋不合格＋】"}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx := context.Background()
			rollback := errors.New("outer transaction rollback")
			err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
				if err := f.remove(txCtx, f.service); err != nil {
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
			if err := f.remove(ctx, f.service); err != nil {
				t.Fatal(err)
			}
			if d := f.assertRoster(t); len(d.Members) != 1 {
				t.Fatalf("target not removed: %+v", d)
			}
			letters := f.removalLetters(t)
			if len(letters) != 1 || !strings.Contains(letters[0], mode.marker) || !strings.Contains(letters[0], f.g.Name) {
				t.Fatalf("notification = %v", letters)
			}
			if err := f.remove(ctx, f.service); !errors.Is(err, guild.ErrTargetNotMember) {
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
		name    string
		pending bool
	}{{"expelActive", false}, {"rejectPending", true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			locked, resume, leaveStarted := make(chan struct{}), make(chan struct{}), make(chan struct{})
			removalRepo := &removalGateRepo{GuildRepository: f.repo, afterRead: func(context.Context) {
				close(locked)
				select {
				case <-resume:
				case <-ctx.Done():
				}
			}}
			removal := f.newService(t, removalRepo)
			var startedOnce sync.Once
			leaveRepo := &removalGateRepo{GuildRepository: f.repo, beforeLocked: func(context.Context) { startedOnce.Do(func() { close(leaveStarted) }) }}
			leave := f.newService(t, leaveRepo)
			removalDone, leaveDone := make(chan error, 1), make(chan error, 1)
			go func() { removalDone <- f.remove(ctx, removal) }()
			awaitRemovalSignal(t, locked)
			go func() { leaveDone <- leave.Leave(ctx, f.g.ID, f.leader.ID) }()
			awaitRemovalSignal(t, leaveStarted)
			close(resume)
			if err := <-removalDone; err != nil {
				t.Fatal(err)
			}
			// The removed target was discovered before the locks but is no longer a successor.
			if err := <-leaveDone; err != nil {
				t.Fatalf("sole leader leave = %v", err)
			}
			if _, err := f.service.Get(context.Background(), f.g.ID); !errors.Is(err, guild.ErrGuildNotFound) {
				t.Fatalf("guild after sole leader leave = %v, want disbanded", err)
			}
			if letters := f.removalLetters(t); len(letters) != 1 {
				t.Fatalf("successful removal letters = %v", letters)
			}
		})
	}
}

func TestGuildRemoval_CurrentState(t *testing.T) {
	t.Run("removing an approved applicant expels it", func(t *testing.T) {
		f := newRemovalFixture(t, true)
		ctx := context.Background()
		if err := assigned(f.service.Assign(ctx, f.g.ID, f.leader.ID, f.target.ID, "member")); err != nil {
			t.Fatal(err)
		}
		if outcome, err := f.service.Assign(ctx, f.g.ID, f.leader.ID, f.target.ID, guild.RemovalTitle); err != nil || outcome != guild.AssignExpelled {
			t.Fatalf("remove approved applicant = %q, %v", outcome, err)
		}
		if d := f.assertRoster(t); len(d.Members) != 1 {
			t.Fatalf("approved member kept: %+v", d)
		}
	})
	t.Run("member cannot remove leader", func(t *testing.T) {
		f := newRemovalFixture(t, false)
		ctx := context.Background()
		if err := assigned(f.service.Assign(ctx, f.g.ID, f.target.ID, f.leader.ID, guild.RemovalTitle)); !errors.Is(err, guild.ErrUnauthorized) {
			t.Fatalf("member removal = %v", err)
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
		name    string
		pending bool
	}{{"expelActive", false}, {"rejectPending", true}} {
		t.Run(mode.name, func(t *testing.T) {
			f := newRemovalFixture(t, mode.pending)
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			defer cancel()
			res := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
				err1, err2 := testutil.RunRace2(
					func() error { return f.remove(ctx, f.service) },
					func() error { return f.service.Leave(ctx, f.g.ID, f.leader.ID) },
				)
				for _, err := range []error{err1, err2} {
					if err != nil && !errors.Is(err, guild.ErrUnauthorized) && !errors.Is(err, guild.ErrTargetNotMember) &&
						!errors.Is(err, guild.ErrCharacterNotInGuild) && !errors.Is(err, guild.ErrGuildNotFound) {
						return err
					}
				}
				return nil
			})
			if res.Failures != 0 {
				t.Fatalf("unexpected stress failures: %+v", res)
			}
			letters := f.removalLetters(t)
			if _, err := f.service.Get(context.Background(), f.g.ID); errors.Is(err, guild.ErrGuildNotFound) {
				if len(letters) != 1 {
					t.Fatalf("removal winner letters = %v", letters)
				}
			} else if d := f.assertRoster(t); len(d.Members) != 1 || d.Guild.LeaderCharacterID != f.target.ID || len(letters) != 0 {
				t.Fatalf("leave winner: roster=%+v letters=%v", d, letters)
			}
			t.Logf("%s: %+v", mode.name, res)
		})
	}
}
