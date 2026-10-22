package challenge_test

import (
	"context"
	"reflect"
	"testing"
)

func TestRunProtocolMemberExitThenDefeatIntegration(t *testing.T) {
	f := newProtocolFixture(t, 2)
	ctx := context.Background()
	run := f.advance(t, f.start(t))
	leaderBefore := run.Members[0]
	result, err := f.svc.ExitSession(ctx, f.actors[1].ID, run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if result.Session.CharacterID != f.actors[0].ID || len(result.Session.Members) != 1 || !reflect.DeepEqual(result.Session.Members[0], leaderBefore) {
		t.Fatalf("member exit changed leader: %+v", result)
	}
	f.assertSettled(t, 1, 1, 1)
	unchanged, err := f.chars.FindByID(ctx, f.actors[0].ID)
	if err != nil || !reflect.DeepEqual(unchanged, f.actors[0]) {
		t.Fatalf("member exit persisted leader: %+v %v", unchanged, err)
	}
	run = f.advance(t, &result.Session)
	f.engine.defeat.Store(true)
	run = f.advance(t, run)
	f.assertSettled(t, 0, 2, 3)
	f.assertSettled(t, 1, 1, 1)
	if len(run.Members) != 1 || run.Members[0].CharacterID != f.actors[0].ID {
		t.Fatalf("departed member returned to defeat: %+v", run)
	}
}
