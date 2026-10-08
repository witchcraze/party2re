package challenge_test

import (
	"context"
	"errors"
	"testing"

	valkey "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/testutil/valkeytest"
)

func TestActiveSessionPropagatesRewardReadFailure(t *testing.T) {
	want := errors.New("reward storage failed")
	client := valkeytest.NewMockClient(valkeytest.WithDoMultiHandler(func(_ context.Context, cmds ...valkey.Completed) []valkey.ValkeyResult {
		if len(cmds) != 2 || cmds[0].Commands()[0] != "HGETALL" || cmds[1].Commands()[0] != "HGETALL" {
			t.Fatal("observation mutated storage")
		}
		return []valkey.ValkeyResult{valkeytest.MakeStringMapResult(map[string]string{"id": "run", "character_id": "hero"}), valkeytest.MakeErrorResult(want)}
	}))
	repo, err := challenge.NewValkeySessionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := repo.GetActiveSession(context.Background(), "hero"); got != nil || !errors.Is(err, want) {
		t.Fatalf("partial observation: %+v %v", got, err)
	}
}
