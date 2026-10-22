package challenge_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/id"
)

func TestRunPublicationVersionAndReplacementParity(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "memory"
		if live {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			var store challenge.ActiveSessionStore = challenge.NewMemorySessionRepository()
			if live {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				var err error
				store, err = challenge.NewValkeySessionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
			}
			actor := id.New()
			run := sampleChallengeSession(id.New(), actor, "test")
			run.Revision = 1
			mp := 10
			run.Members = []challenge.ChallengeMember{{CharacterID: actor, CharacterCurrentHP: 200, CharacterCurrentMP: &mp}}
			t.Cleanup(func() {
				if err := clearRun(ctx, store, actor); err != nil {
					t.Error(err)
				}
			})
			publication := challenge.Transition{RunID: run.ID, Revision: 1, Publication: &run, Deadline: time.Now().Add(time.Hour)}
			if err := store.PublishRun(ctx, publication); err != nil {
				t.Fatal(err)
			}
			if err := store.PublishRun(ctx, publication); err != nil {
				t.Fatalf("identical publication: %v", err)
			}
			changed := run
			changed.CurrentRound++
			bad := publication
			bad.Publication = &changed
			if err := store.PublishRun(ctx, bad); !errors.Is(err, challenge.ErrRevisionConflict) {
				t.Fatalf("different body at same revision: %v", err)
			}
			terminal := run
			terminal.Revision = 2
			terminal.Status = challenge.StatusClaimed
			cleanup := challenge.Transition{RunID: run.ID, Revision: 2, Publication: &terminal, RemovedIDs: []string{actor}, Deadline: publication.Deadline}
			if err := store.PublishRun(ctx, cleanup); err != nil {
				t.Fatal(err)
			}
			replacement := run
			replacement.ID = id.New()
			if err := store.PublishRun(ctx, challenge.Transition{RunID: replacement.ID, Revision: 1, Publication: &replacement, Deadline: publication.Deadline}); err != nil {
				t.Fatal(err)
			}
			if err := store.PublishRun(ctx, cleanup); err != nil {
				t.Fatalf("delayed cleanup: %v", err)
			}
			got, err := store.GetActiveSession(ctx, actor)
			if err != nil || got == nil || got.ID != replacement.ID {
				t.Fatalf("replacement removed: %+v %v", got, err)
			}
			if err := store.PublishRun(ctx, publication); !errors.Is(err, challenge.ErrRevisionConflict) {
				t.Fatalf("old publication resurrected actor: %v", err)
			}
		})
	}
}
