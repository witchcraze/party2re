package replay_test

import (
	"context"
	"errors"
	"testing"

	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/replay"
)

type fakeArchiveRepo struct {
	reads, publishes int
	stored           replay.StoredArchive
}

func (f *fakeArchiveRepo) AppendScene(context.Context, string, string, string, string, int64) (bool, error) {
	return true, nil
}
func (f *fakeArchiveRepo) StageCopy(context.Context, string, string, string) (string, error) {
	return "copy", nil
}
func (f *fakeArchiveRepo) PublishPending(context.Context, string) error { f.publishes++; return nil }
func (f *fakeArchiveRepo) FindLatest(context.Context, string) (replay.StoredArchive, error) {
	f.reads++
	return f.stored, nil
}

// replayIdentity: every character belongs to player "owner".
func TestArchiveAuthorizationPrecedesPayloadReadsAndRecovery(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		session string
		want    error
	}{
		{"participant", replay.ErrReplayNotFound},
		{"invalid", coreplayer.ErrAuthentication},
		{"", coreplayer.ErrAuthentication},
	} {
		repo := &fakeArchiveRepo{}
		svc, err := replay.NewArchiveService(repo, replayIdentity{}, replayIdentity{})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := svc.GetLatest(ctx, tc.session, "recipient"); !errors.Is(err, tc.want) {
			t.Fatalf("%q GetLatest err = %v", tc.session, err)
		}
		if err := svc.Recover(ctx, tc.session, "recipient"); !errors.Is(err, tc.want) {
			t.Fatalf("%q Recover err = %v", tc.session, err)
		}
		if repo.reads != 0 || repo.publishes != 0 {
			t.Fatalf("%q reached storage: %+v", tc.session, repo)
		}
	}
}

func TestArchiveGetLatestDoesNotPublishAndRejectsUndecodableScenes(t *testing.T) {
	ctx := context.Background()
	repo := &fakeArchiveRepo{stored: replay.StoredArchive{CopyID: "c", Scenes: []string{`{"v":1,"title":"t","members":[],"comments":[]}`}, Pending: true}}
	svc, err := replay.NewArchiveService(repo, replayIdentity{}, replayIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	got, err := svc.GetLatest(ctx, "owner", "recipient")
	if err != nil || got.CopyID != "c" || len(got.Scenes) != 1 || got.Scenes[0].Title != "t" || !got.Pending || repo.publishes != 0 {
		t.Fatalf("got %+v, %v, publishes=%d", got, err, repo.publishes)
	}
	for _, raw := range []string{`{`, `{"v":2,"title":"t"}`} {
		repo.stored.Scenes = []string{raw}
		if _, err := svc.GetLatest(ctx, "owner", "recipient"); !errors.Is(err, replay.ErrArchiveSourceUnavailable) {
			t.Fatalf("%s err = %v", raw, err)
		}
	}
}

func TestArchiveStagingRejectsMissingIdentities(t *testing.T) {
	svc, err := replay.NewArchiveService(&fakeArchiveRepo{}, replayIdentity{}, replayIdentity{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	if _, err := svc.StageScene(ctx, "run", "", "actor", baseCapture()); !errors.Is(err, replay.ErrInvalidScene) {
		t.Fatalf("scene err = %v", err)
	}
	if _, err := svc.StageExitCopy(ctx, "run", "cmd", " "); !errors.Is(err, replay.ErrInvalidReplay) {
		t.Fatalf("copy err = %v", err)
	}
	if err := svc.Publish(ctx, ""); !errors.Is(err, replay.ErrInvalidReplay) {
		t.Fatalf("publish err = %v", err)
	}
}
