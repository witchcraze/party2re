package adventure

import (
	"context"
	"errors"
	"testing"
)

type publicRunStore struct {
	NormalRunStore
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
		s := &NormalRunService{cfg: NormalRunConfig{Store: publicRunStore{run: tc.run, err: tc.err}}}
		active, err := s.HasActiveRun(context.Background(), "target")
		if active != (tc.run != nil) || !errors.Is(err, tc.err) {
			t.Fatalf("active=%v err=%v", active, err)
		}
	}
}
