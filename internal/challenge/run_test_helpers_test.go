package challenge_test

import (
	"context"
	"github.com/witchcraze/party2re/internal/challenge"
	"testing"
	"time"
)

// Test teardown removes only the run observed for this actor.
func clearRun(ctx context.Context, store challenge.ActiveSessionStore, actor string) error {
	run, err := store.GetActiveSession(ctx, actor)
	if err != nil || run == nil {
		return err
	}
	if run.Revision == 0 {
		return store.DeleteActiveSession(ctx, actor)
	}
	run.Revision++
	run.Status = challenge.StatusClaimed
	ids := make([]string, len(run.Members))
	for i, m := range run.Members {
		ids[i] = m.CharacterID
	}
	return store.PublishRun(ctx, challenge.Transition{RunID: run.ID, Revision: run.Revision, Publication: run, RemovedIDs: ids, Deadline: time.Now().Add(time.Hour)})
}

func saveRunFixture(t *testing.T, ctx context.Context, repo challenge.TransitionRepository, store challenge.ActiveSessionStore, run *challenge.ChallengeSession) {
	t.Helper()
	run.Revision++
	publication := challenge.Transition{RunID: run.ID, Revision: run.Revision, Pending: true, Publication: run, Deadline: time.Now().UTC().Add(store.SessionTTL())}
	if err := repo.WithRunLock(ctx, run.ID, func(ctx context.Context) error { return repo.SaveTransition(ctx, publication, nil) }); err != nil {
		t.Fatal(err)
	}
	if err := store.PublishRun(ctx, publication); err != nil {
		t.Fatal(err)
	}
	if err := repo.WithRunLock(ctx, run.ID, func(ctx context.Context) error { return repo.AcknowledgePublication(ctx, run.ID, run.Revision) }); err != nil {
		t.Fatal(err)
	}
}
