package playercontext

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/core/character"
)

func TestQueryPendingOwnerWorkBlocksWake(t *testing.T) {
	ctx := context.Background()
	r := &queryReaders{ctx: ctx, character: character.Character{ID: "actor", PlayerID: "player", PendingWake: true}}
	svc := NewService(r, r, r, WithActivities(func(context.Context, string, string) ([]Activity, error) {
		return []Activity{{Kind: "adventure", ID: "run", Phase: "recovery_pending", Role: "participant", BlocksWake: true}}, nil
	}))
	result, err := svc.Query(ctx, "actor", "player")
	if err != nil || result.Snapshot.CanWake || !result.Snapshot.Sleeping {
		t.Fatal(result, err)
	}
	if location := activityLocation([]Activity{{Kind: "party", ID: "lobby"}, {Kind: "adventure", ID: "run", PartyID: "lobby"}}); location != "adventure" {
		t.Fatal(location)
	}
}
