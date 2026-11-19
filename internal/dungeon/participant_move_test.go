package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/testutil"
)

// startMap1Party starts lead+ally on map1 variant 1 beside its locked door (4,0).
func startMap1Party(t *testing.T, allyJob string) *nativeFixture {
	t.Helper()
	f := newNativeFixture(t, map1Hero("lead", "job-01"))
	f.chars.chars["ally"] = map1Hero("ally", allyJob)
	f.startNativeParty(t, dungeon.NativeMap1ID, 1, 4, 0)
	return f
}

// Legacy registers the directions for every member (vs_dungeon.cgi:37–44) and
// the map event acts through the mover: the ally's job 9 opens the door.
func TestMove_LivingMemberMovesAsTheActor(t *testing.T) {
	f := startMap1Party(t, "job-09")
	if c := f.reload(t, "ally").Controls("ally"); !c.CanMove || c.CanEscape || c.CanFight {
		t.Fatalf("member controls %+v", c)
	}
	if _, err := f.svc.Move(context.Background(), "ally", dungeon.DirectionWest); err != nil {
		t.Fatal(err)
	}
	if exp := f.reload(t, "lead"); exp.PosX != 3 || exp.Round != 2 || !slices.Equal(exp.ConsumedEvents, []string{"D"}) {
		t.Fatalf("member move %+v", exp)
	}
}

// A fallen character can only Escape (_battle.cgi:654–663); living members continue.
func TestMove_FallenActorIsRejected(t *testing.T) {
	ctx := context.Background()
	f := startMap1Party(t, "job-01")
	exp := f.reload(t, "lead")
	exp.Members[0].CurrentHP, exp.CurrentHP = 0, 0
	if err := f.store.SaveActiveExpedition(ctx, exp); err != nil {
		t.Fatal(err)
	}
	before := f.reload(t, "lead")
	if c := before.Controls("lead"); c.CanMove || !c.CanEscape {
		t.Fatalf("fallen leader controls %+v", c)
	}
	if _, err := f.svc.Move(ctx, "lead", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrActorFallen) {
		t.Fatalf("fallen leader err=%v", err)
	}
	if after := f.reload(t, "lead"); after.Revision != before.Revision || f.rng.calls() != 0 {
		t.Fatalf("rejected move changed %+v rng=%d", after, f.rng.calls())
	}
	if _, err := f.svc.Move(ctx, "ally", dungeon.DirectionWest); err != nil {
		t.Fatalf("living member err=%v", err)
	}
	if _, err := f.svc.Move(ctx, "stranger", dungeon.DirectionWest); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
		t.Fatalf("non-member err=%v", err)
	}
}

// Concurrent moves by two members never apply one revision twice.
func TestMove_ConcurrentMembersApplyOneStepPerRevision(t *testing.T) {
	t.Run("memory", func(t *testing.T) {
		f := startMap1Party(t, "job-01")
		raceDoorMoves(t, f.svc, f.store, "lead", "ally")
	})
	t.Run("valkey", func(t *testing.T) {
		client := testutil.OpenValkey(t)
		suffix := time.Now().Format("150405.000000")
		lead, ally := "pm-lead-"+suffix, "pm-ally-"+suffix
		t.Cleanup(func() {
			cleanupDungeonValkey(t, client, lead)
			client.Do(context.Background(), client.B().Del().Key(fmt.Sprintf("party2:dungeon:{char:%s}:member", ally)).Build())
		})
		store, err := dungeon.NewValkeyExpeditionRepository(client)
		if err != nil {
			t.Fatal(err)
		}
		chars := &mockCharRepo{chars: map[string]corecharacter.Character{lead: map1Hero(lead, "job-01"), ally: map1Hero(ally, "job-01")}}
		svc, err := dungeon.NewService(newMockDungeonRepo(), chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store),
			dungeon.WithRNG(&scriptedRNG{ints: []int{0}}), dungeon.WithInventoryProvider(&mockInventoryRepo{}), dungeon.WithAdmission(approvedRoster{}))
		if err != nil {
			t.Fatal(err)
		}
		ctx := context.Background()
		if _, err := svc.StartPartyExpedition(ctx, lead, []string{lead, ally}, dungeon.NativeMap1ID, "p-"+suffix); err != nil {
			t.Fatal(err)
		}
		exp, err := store.GetActiveExpedition(ctx, lead)
		if err != nil || exp == nil {
			t.Fatalf("reload %v %v", exp, err)
		}
		exp.PosX, exp.PosY = 4, 0
		if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatal(err)
		}
		raceDoorMoves(t, svc, store, lead, ally)
	})
}

// raceDoorMoves races both members into variant 1's locked door, which spends a
// round without drawing RNG; the round must equal the accepted moves.
func raceDoorMoves(t *testing.T, svc *dungeon.Service, store dungeon.ActiveExpeditionStore, lead, ally string) {
	t.Helper()
	ctx := context.Background()
	before, err := store.GetActiveExpedition(ctx, lead)
	if err != nil || before == nil {
		t.Fatalf("before %v %v", before, err)
	}
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied := 0
	for range 20 {
		for _, id := range []string{lead, ally} {
			wg.Add(1)
			go func() {
				defer wg.Done()
				_, err := svc.Move(ctx, id, dungeon.DirectionWest)
				if err != nil && !errors.Is(err, dungeon.ErrExpeditionStateChanged) {
					t.Errorf("%s: %v", id, err)
					return
				}
				if err == nil {
					mu.Lock()
					applied++
					mu.Unlock()
				}
			}()
		}
	}
	wg.Wait()
	after, err := store.GetActiveExpedition(ctx, lead)
	if err != nil || after == nil || applied == 0 || after.Round != before.Round+applied || after.Revision != before.Revision+applied {
		t.Fatalf("after %+v err=%v, want %d accepted moves from round %d", after, err, applied, before.Round)
	}
}
