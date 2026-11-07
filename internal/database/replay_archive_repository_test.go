package database_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/replay"
	"github.com/witchcraze/party2re/internal/testutil"
)

const archiveAlly = "#FFCC00"

// archiveIdentity: session tokens are player IDs and every character belongs to player "owner".
type archiveIdentity struct{}

func (archiveIdentity) Authenticate(_ context.Context, token string) (coreplayer.Player, error) {
	if token == "" {
		return coreplayer.Player{}, coreplayer.ErrAuthentication
	}
	return coreplayer.Player{ID: token}, nil
}

func (archiveIdentity) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	return corecharacter.Character{ID: id, PlayerID: "owner"}, nil
}

// failingPublish simulates post-commit publication failure, optionally after the commit (lost acknowledgement).
type failingPublish struct {
	*database.ReplayRepository
	afterCommit bool
}

func (f failingPublish) PublishPending(ctx context.Context, recipientID string) error {
	if f.afterCommit {
		if err := f.ReplayRepository.PublishPending(ctx, recipientID); err != nil {
			return err
		}
	}
	return errors.New("publication acknowledgement lost")
}

type archiveFixture struct {
	t    *testing.T
	db   *sql.DB
	repo *database.ReplayRepository
	svc  *replay.ArchiveService
	runs []string
	recs []string
}

func newArchiveFixture(t *testing.T) *archiveFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f := &archiveFixture{t: t, db: db, repo: repo}
	f.svc = f.service(repo)
	t.Cleanup(func() {
		ctx := context.Background()
		for _, run := range f.runs {
			for _, table := range []string{"replay_scenes", "replay_scene_sources", "replay_archive_copies"} {
				if _, err := db.ExecContext(ctx, "DELETE FROM "+table+" WHERE run_id = ?", run); err != nil {
					t.Error(err)
				}
			}
		}
		for _, rec := range f.recs {
			if _, err := db.ExecContext(ctx, "DELETE FROM replay_latest_slots WHERE recipient_id = ?", rec); err != nil {
				t.Error(err)
			}
		}
		db.Close()
	})
	return f
}

func (f *archiveFixture) service(repo replay.ArchiveRepository) *replay.ArchiveService {
	svc, err := replay.NewArchiveService(repo, archiveIdentity{}, archiveIdentity{})
	if err != nil {
		f.t.Fatal(err)
	}
	return svc
}

func (f *archiveFixture) run() string { r := "run-" + id.New(); f.runs = append(f.runs, r); return r }
func (f *archiveFixture) rec() string { r := "chr-" + id.New(); f.recs = append(f.recs, r); return r }

func sceneCapture(title string) replay.SceneCapture {
	return replay.SceneCapture{Title: title, ActorColor: archiveAlly,
		Members: []replay.MemberCapture{{Name: "a", Color: archiveAlly, HP: 5, MaxHP: 10}}}
}

func (f *archiveFixture) scene(run, cmd, title string) bool {
	f.t.Helper()
	ok, err := f.svc.StageScene(context.Background(), run, cmd, "actor", sceneCapture(title))
	if err != nil {
		f.t.Fatal(err)
	}
	return ok
}

func (f *archiveFixture) exit(run, cmd, rec string) string {
	f.t.Helper()
	copyID, err := f.svc.StageExitCopy(context.Background(), run, cmd, rec)
	if err != nil {
		f.t.Fatal(err)
	}
	return copyID
}

func (f *archiveFixture) latest(rec string) replay.LatestArchive {
	f.t.Helper()
	got, err := f.svc.GetLatest(context.Background(), "owner", rec)
	if err != nil {
		f.t.Fatal(err)
	}
	return got
}

func titles(a replay.LatestArchive) string {
	var out []string
	for _, s := range a.Scenes {
		out = append(out, s.Title)
	}
	return strings.Join(out, ",")
}

func TestReplayArchiveByteBudgetCountsPersistedUTF8(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	run := f.run()
	empty, err := replay.Project(sceneCapture(""))
	if err != nil {
		t.Fatal(err)
	}
	base, err := replay.EncodeScene(empty)
	if err != nil {
		t.Fatal(err)
	}
	need := int(replay.MaxSourceBytes) - len(base)
	title := strings.Repeat("あ", need/3) + strings.Repeat("a", need%3)
	if !f.scene(run, "big", title) {
		t.Fatal("scene below the limit was suppressed")
	}
	var total, persisted int64
	if err := f.db.QueryRowContext(ctx, `SELECT s.encoded_bytes, (SELECT SUM(LENGTH(scene_json)) FROM replay_scenes WHERE run_id = s.run_id)
		FROM replay_scene_sources s WHERE s.run_id = ?`, run).Scan(&total, &persisted); err != nil {
		t.Fatal(err)
	}
	if total != replay.MaxSourceBytes || persisted != total {
		t.Fatalf("total=%d persisted=%d", total, persisted)
	}
	if !f.scene(run, "equal", "crosses") {
		t.Fatal("scene at exactly the limit must append whole")
	}
	if f.scene(run, "above", "suppressed") {
		t.Fatal("scene above the limit must be suppressed")
	}
	rec := f.rec()
	f.exit(run, "exit", rec)
	if err := f.svc.Publish(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if got := f.latest(rec); len(got.Scenes) != 2 || got.Scenes[1].Title != "crosses" || got.Scenes[0].Title != title {
		t.Fatalf("archive has %d scenes", len(got.Scenes))
	}
}

func TestReplayArchiveExitPrefixesAndLatestReplacement(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	run, a, b := f.run(), f.rec(), f.rec()
	f.scene(run, "c1", "s1")
	f.scene(run, "c2", "s2")
	if f.scene(run, "c2", "dup") {
		t.Fatal("duplicate command identity appended twice")
	}
	copyA := f.exit(run, "c3", a)
	f.scene(run, "c3", "a-exit")
	if again := f.exit(run, "c3", a); again != copyA {
		t.Fatalf("duplicate exit staged a second copy: %s vs %s", again, copyA)
	}
	f.exit(run, "c4", b)
	for _, rec := range []string{a, b} {
		if err := f.svc.Publish(ctx, rec); err != nil {
			t.Fatal(err)
		}
	}
	if got := titles(f.latest(a)); got != "s1,s2" {
		t.Fatalf("A copy = %s", got)
	}
	if got := titles(f.latest(b)); got != "s1,s2,a-exit" {
		t.Fatalf("B copy = %s", got)
	}

	// Empty and absent sources leave A's slot untouched.
	empty := f.run()
	if _, err := f.db.ExecContext(ctx, `INSERT INTO replay_scene_sources (run_id) VALUES (?)`, empty); err != nil {
		t.Fatal(err)
	}
	if f.exit(empty, "x", a) != "" || f.exit("run-absent-"+id.New(), "x", a) != "" {
		t.Fatal("empty source staged a copy")
	}

	next := f.run()
	f.scene(next, "n1", "next")
	f.exit(next, "n2", a)
	if err := f.svc.Publish(ctx, a); err != nil {
		t.Fatal(err)
	}
	got := f.latest(a)
	if titles(got) != "next" || got.CopyID == copyA || got.Pending {
		t.Fatalf("replacement = %+v", got)
	}
	// Superseded copy IDs are not replay IDs and never reach detail or history reads.
	reads, err := replay.NewService(f.repo, archiveIdentity{}, archiveIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := reads.GetReplay(ctx, "owner", copyA); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatalf("superseded copy detail err = %v", err)
	}
	if h, err := reads.GetCharacterHistory(ctx, "owner", a, "", 10); err != nil || len(h) != 0 {
		t.Fatalf("history = %v, %v", h, err)
	}
	if _, err := f.svc.GetLatest(ctx, "participant", b); !errors.Is(err, replay.ErrReplayNotFound) {
		t.Fatalf("foreign read err = %v", err)
	}
}

func TestReplayArchiveRollbackFailureAndRecovery(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	rec := f.rec()

	rolled := f.run()
	errAbort := errors.New("command aborted")
	err := database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
		if _, err := f.svc.StageScene(txCtx, rolled, "c1", "actor", sceneCapture("never")); err != nil {
			return err
		}
		if _, err := f.svc.StageExitCopy(txCtx, rolled, "c2", rec); err != nil {
			return err
		}
		return errAbort
	})
	if !errors.Is(err, errAbort) {
		t.Fatal(err)
	}
	var rows int
	if err := f.db.QueryRowContext(ctx, `SELECT (SELECT COUNT(*) FROM replay_scene_sources WHERE run_id = ?) +
		(SELECT COUNT(*) FROM replay_scenes WHERE run_id = ?) + (SELECT COUNT(*) FROM replay_archive_copies WHERE run_id = ?)`,
		rolled, rolled, rolled).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("rollback left %d rows (%v)", rows, err)
	}

	first := f.run()
	f.scene(first, "c1", "old")
	f.exit(first, "c2", rec)
	if err := f.svc.Publish(ctx, rec); err != nil {
		t.Fatal(err)
	}

	// Failed publication: old slot stays readable and the frozen copy stays pending, also after restart.
	second := f.run()
	f.scene(second, "c1", "new")
	f.exit(second, "c2", rec)
	if err := f.service(failingPublish{ReplayRepository: f.repo}).Publish(ctx, rec); err == nil {
		t.Fatal("expected publication failure")
	}
	if got := f.latest(rec); titles(got) != "old" || !got.Pending {
		t.Fatalf("after failure = %+v", got)
	}
	restarted, err := database.NewReplayRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.service(restarted).Recover(ctx, "owner", rec); err != nil {
		t.Fatal(err)
	}
	if got := f.latest(rec); titles(got) != "new" || got.Pending {
		t.Fatalf("after recovery = %+v", got)
	}

	// Lost acknowledgement: recovery after a committed publication is a no-op without duplicates.
	third := f.run()
	f.scene(third, "c1", "third")
	f.exit(third, "c2", rec)
	if err := f.service(failingPublish{ReplayRepository: f.repo, afterCommit: true}).Publish(ctx, rec); err == nil {
		t.Fatal("expected lost acknowledgement")
	}
	if err := f.svc.Recover(ctx, "owner", rec); err != nil {
		t.Fatal(err)
	}
	var copies int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM replay_archive_copies WHERE recipient_id = ?`, rec).Scan(&copies); err != nil || copies != 3 {
		t.Fatalf("copies = %d (%v)", copies, err)
	}
	if got := f.latest(rec); titles(got) != "third" {
		t.Fatalf("after lost ack = %s", titles(got))
	}

	// Reversed completion across runs: older work finishing late never replaces the newer slot.
	older, newer := f.run(), f.run()
	f.scene(older, "c1", "older")
	olderCopy := f.exit(older, "c2", rec)
	f.scene(newer, "c1", "newer")
	f.exit(newer, "c2", rec)
	if err := f.svc.Publish(ctx, rec); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, `UPDATE replay_archive_copies SET status = 'pending' WHERE id = ?`, olderCopy); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Recover(ctx, "owner", rec); err != nil {
		t.Fatal(err)
	}
	if got := f.latest(rec); titles(got) != "newer" || got.Pending {
		t.Fatalf("late older work = %+v", got)
	}

	// Unreadable frozen source is an observable failure, not an empty copy.
	broken := f.run()
	f.scene(broken, "c1", "lost")
	f.exit(broken, "c2", rec)
	if _, err := f.db.ExecContext(ctx, `DELETE FROM replay_scenes WHERE run_id = ?`, broken); err != nil {
		t.Fatal(err)
	}
	if err := f.svc.Recover(ctx, "owner", rec); !errors.Is(err, replay.ErrArchiveSourceUnavailable) {
		t.Fatalf("broken source err = %v", err)
	}
	if got := f.latest(rec); titles(got) != "newer" || !got.Pending {
		t.Fatalf("after source failure = %+v", got)
	}
}

func TestReplayArchiveConcurrentPublicationKeepsNewestSlot(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	rec := f.rec()
	cfg := testutil.GetStressConfig()
	runs := make([]string, cfg.Workers)
	for w := range runs {
		runs[w] = f.run()
		f.scene(runs[w], "seed", fmt.Sprintf("w%d", w))
	}
	var staged int64
	testutil.RunConcurrentStressTest(t, cfg, func(w, op int) error {
		if op%2 == 0 {
			copyID, err := f.svc.StageExitCopy(ctx, runs[w], fmt.Sprintf("exit-%d", op), rec)
			if err == nil && copyID != "" {
				atomic.AddInt64(&staged, 1)
			}
			return err
		}
		return f.svc.Publish(ctx, rec)
	})
	if err := f.svc.Publish(ctx, rec); err != nil {
		t.Fatal(err)
	}
	var copies, pending int64
	var newest string
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*), SUM(status = 'pending'),
		(SELECT id FROM replay_archive_copies WHERE recipient_id = ? ORDER BY seq DESC LIMIT 1)
		FROM replay_archive_copies WHERE recipient_id = ?`, rec, rec).Scan(&copies, &pending, &newest); err != nil {
		t.Fatal(err)
	}
	got := f.latest(rec)
	if copies != staged || pending != 0 || got.CopyID != newest || len(got.Scenes) != 1 {
		t.Fatalf("copies=%d staged=%d pending=%d slot=%s newest=%s scenes=%d", copies, staged, pending, got.CopyID, newest, len(got.Scenes))
	}
}

func TestReplayArchivePublicationRejectsUndecodableScenes(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()
	for name, bad := range map[string]string{
		"corrupt json":        `{"v":1,"title":`,
		"unsupported version": `{"v":2,"title":"future","members":[],"comments":[]}`,
	} {
		t.Run(name, func(t *testing.T) {
			rec := f.rec()
			good := f.run()
			f.scene(good, "c1", "good")
			f.exit(good, "c2", rec)
			if err := f.svc.Publish(ctx, rec); err != nil {
				t.Fatal(err)
			}
			broken := f.run()
			f.scene(broken, "c1", "first")
			f.scene(broken, "c2", "second")
			f.exit(broken, "c3", rec)
			if _, err := f.db.ExecContext(ctx, `UPDATE replay_scenes SET scene_json = ? WHERE run_id = ? AND seq = 2`, bad, broken); err != nil {
				t.Fatal(err)
			}
			if err := f.svc.Publish(ctx, rec); !errors.Is(err, replay.ErrArchiveSourceUnavailable) {
				t.Fatalf("publish err = %v", err)
			}
			if got := f.latest(rec); titles(got) != "good" || !got.Pending {
				t.Fatalf("old slot not kept: %+v", got)
			}
			// Once the source is readable again, owner recovery publishes the same frozen copy.
			fixed, err := replay.EncodeScene(replay.Scene{Version: replay.SceneVersion, Title: "second", Members: []replay.SceneMember{}, Comments: []replay.SceneComment{}})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.db.ExecContext(ctx, `UPDATE replay_scenes SET scene_json = ? WHERE run_id = ? AND seq = 2`, fixed, broken); err != nil {
				t.Fatal(err)
			}
			if err := f.svc.Recover(ctx, "owner", rec); err != nil {
				t.Fatal(err)
			}
			if got := f.latest(rec); titles(got) != "first,second" || got.Pending {
				t.Fatalf("after recovery = %+v", got)
			}
		})
	}
}

func (f *archiveFixture) corrupt(run string) {
	f.t.Helper()
	if _, err := f.db.ExecContext(context.Background(), `UPDATE replay_scenes SET scene_json = '{' WHERE run_id = ?`, run); err != nil {
		f.t.Fatal(err)
	}
}

func (f *archiveFixture) status(copyID string) string {
	f.t.Helper()
	var s string
	if err := f.db.QueryRowContext(context.Background(), `SELECT status FROM replay_archive_copies WHERE id = ?`, copyID).Scan(&s); err != nil {
		f.t.Fatal(err)
	}
	return s
}

// Each exit copy stands alone, as the original per-exit file copy does (#1913).
func TestReplayArchiveUnreadableCopiesDoNotBlockOtherExits(t *testing.T) {
	f := newArchiveFixture(t)
	ctx := context.Background()

	t.Run("broken older, valid newer", func(t *testing.T) {
		rec, older, newer := f.rec(), f.run(), f.run()
		f.scene(older, "c1", "older")
		olderCopy := f.exit(older, "c2", rec)
		f.corrupt(older)
		f.scene(newer, "c1", "newer")
		newerCopy := f.exit(newer, "c2", rec)
		if err := f.svc.Publish(ctx, rec); err != nil {
			t.Fatal(err)
		}
		if got := f.latest(rec); titles(got) != "newer" || got.Pending || got.CopyID != newerCopy {
			t.Fatalf("latest = %+v", got)
		}
		if s := f.status(olderCopy); s != "superseded" {
			t.Fatalf("older status = %s", s)
		}
	})

	t.Run("valid older, broken newer", func(t *testing.T) {
		rec, older, newer := f.rec(), f.run(), f.run()
		f.scene(older, "c1", "older")
		olderCopy := f.exit(older, "c2", rec)
		f.scene(newer, "c1", "newer")
		newerCopy := f.exit(newer, "c2", rec)
		f.corrupt(newer)
		if err := f.svc.Publish(ctx, rec); !errors.Is(err, replay.ErrArchiveSourceUnavailable) {
			t.Fatalf("publish err = %v", err)
		}
		if got := f.latest(rec); titles(got) != "older" || !got.Pending || got.CopyID != olderCopy {
			t.Fatalf("latest = %+v", got)
		}
		if s := f.status(newerCopy); s != "pending" {
			t.Fatalf("newer status = %s", s)
		}
		// A later valid exit supersedes the still-unreadable copy.
		later := f.run()
		f.scene(later, "c1", "later")
		f.exit(later, "c2", rec)
		if err := f.svc.Recover(ctx, "owner", rec); err != nil {
			t.Fatal(err)
		}
		if got := f.latest(rec); titles(got) != "later" || got.Pending {
			t.Fatalf("after later exit = %+v", got)
		}
		if s := f.status(newerCopy); s != "superseded" {
			t.Fatalf("broken copy status = %s", s)
		}
	})
}
