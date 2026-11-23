package runhost

import (
	"context"
	"errors"
	"testing"
)

type testRun struct{ State }

func (r *testRun) Host() *State { return &r.State }

type publicRunStore struct {
	Store[testRun]
	run *ActorRun
	err error
}

func (s publicRunStore) FindActorRun(_ context.Context, actor string) (*ActorRun, error) {
	if actor != "target" {
		return nil, errors.New("wrong actor")
	}
	return s.run, s.err
}

func TestHasActiveRun(t *testing.T) {
	wantErr := errors.New("read failed")
	for _, tc := range []struct {
		run *ActorRun
		err error
	}{
		{}, {run: &ActorRun{RunID: "private-run"}}, {err: wantErr},
	} {
		s := &Service[testRun, *testRun]{cfg: Config[testRun]{Store: publicRunStore{run: tc.run, err: tc.err}}}
		active, err := s.HasActiveRun(context.Background(), "target")
		if active != (tc.run != nil) || !errors.Is(err, tc.err) {
			t.Fatalf("active=%v err=%v", active, err)
		}
	}
}
