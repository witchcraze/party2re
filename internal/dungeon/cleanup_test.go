package dungeon_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	valkey "github.com/valkey-io/valkey-go"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

type cleanupReadClient struct {
	valkey.Client
	afterRead func()
}

func (c *cleanupReadClient) DoMulti(ctx context.Context, cmds ...valkey.Completed) []valkey.ValkeyResult {
	read := len(cmds) > 0 && cmds[0].Commands()[0] == "HGETALL"
	results := c.Client.DoMulti(ctx, cmds...)
	if c.afterRead != nil && read {
		after := c.afterRead
		c.afterRead = nil
		after()
	}
	return results
}

func TestExpeditionCleanup_RunIdentity(t *testing.T) {
	for _, adapter := range []string{"memory", "valkey", "valkey-replaced-after-read"} {
		t.Run(adapter, func(t *testing.T) {
			ctx := context.Background()
			var store dungeon.ActiveExpeditionStore = dungeon.NewMemoryExpeditionRepository()
			var hooked *cleanupReadClient
			if adapter != "memory" {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				hooked = &cleanupReadClient{Client: client}
				r, err := dungeon.NewValkeyExpeditionRepository(hooked)
				if err != nil {
					t.Fatal(err)
				}
				store = r
			}
			owner, follower := id.New(), id.New()
			old := sampleExpedition(id.New(), owner, "dungeon-01")
			old.Members = []dungeon.ExpeditionMember{{CharacterID: owner}, {CharacterID: follower}}
			next := old
			next.ID = id.New()
			next.AccumulatedItems = []string{"potion"}
			next.ConsumedEvents = []string{"T:1:2:0"}
			next.PosX, next.TurnsRemaining, next.AccumulatedGold = 2, 17, 77
			if err := store.SaveActiveExpedition(ctx, old); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, owner, next.ID); err != nil {
					t.Error(err)
				}
			})
			replace := func() {
				if err := store.SaveActiveExpedition(ctx, next); err != nil {
					t.Fatal(err)
				}
			}
			if adapter == "valkey-replaced-after-read" {
				// Return A's real HGETALL results, then publish B before any cleanup writes.
				hooked.afterRead = replace
			} else {
				replace()
			}
			for i := 0; i < 2; i++ {
				if err := store.DeleteActiveExpedition(ctx, owner, old.ID); err != nil {
					t.Fatal(err)
				}
				for _, actor := range []string{owner, follower} {
					got, err := store.GetActiveExpedition(ctx, actor)
					if err != nil || !reflect.DeepEqual(got, &next) {
						t.Fatalf("stale cleanup changed replacement for %s: %+v, err=%v", actor, got, err)
					}
				}
			}
			if err := store.DeleteActiveExpedition(ctx, follower, next.ID); !errors.Is(err, dungeon.ErrForbidden) {
				t.Fatalf("follower cannot delete owner's run: %v", err)
			}
			for i := 0; i < 2; i++ {
				if err := store.DeleteActiveExpedition(ctx, owner, next.ID); err != nil {
					t.Fatal(err)
				}
			}
			for _, actor := range []string{owner, follower} {
				if got, err := store.GetActiveExpedition(ctx, actor); err != nil || got != nil {
					t.Fatalf("matching cleanup retained run: %+v, err=%v", got, err)
				}
			}
			if hooked != nil {
				stateKey := "party2:dungeon:{char:" + owner + "}:state"
				rewardsKey := "party2:dungeon:{char:" + owner + "}:rewards"
				if n, err := hooked.Do(ctx, hooked.B().Exists().Key(stateKey, rewardsKey).Build()).AsInt64(); err != nil || n != 0 {
					t.Fatalf("matching cleanup retained state/rewards: %d %v", n, err)
				}
				key := "party2:dungeon:{char:" + follower + "}:member"
				if n, err := hooked.Do(ctx, hooked.B().Exists().Key(key).Build()).AsInt64(); err != nil || n != 0 {
					t.Fatalf("matching cleanup retained member reference: %d %v", n, err)
				}
			}
		})
	}
}

type replacingFinalizeRepository struct {
	*mockDungeonRepo
	replace func()
}

func (r *replacingFinalizeRepository) FinalizeExpedition(ctx context.Context, h dungeon.DungeonExpeditionHistory, rec dungeon.CharacterDungeonRecord, c *corecharacter.Character, items []coreitem.Instance, members ...dungeon.ExpeditionMember) error {
	err := r.mockDungeonRepo.FinalizeExpedition(ctx, h, rec, c, items, members...)
	r.replace()
	return err
}

func TestSettlementCleanup_PreservesReplacement(t *testing.T) {
	for _, outcome := range []string{"clear", "escape", "wipeout"} {
		for _, duplicate := range []bool{false, true} {
			name := outcome
			if duplicate {
				name += "/duplicate"
			}
			t.Run(name, func(t *testing.T) {
				ctx := context.Background()
				store := dungeon.NewMemoryExpeditionRepository()
				repo := &replacingFinalizeRepository{mockDungeonRepo: newMockDungeonRepo()}
				chars := &mockCharRepo{chars: map[string]corecharacter.Character{"hero": createTestChar("hero", 30, 100, 50, 30)}}
				svc, err := dungeon.NewService(repo, chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(customSettlementDungeons()))
				if err != nil {
					t.Fatal(err)
				}
				dungeonID := "test-dg-01"
				if outcome == "clear" {
					dungeonID = "test-dg-bossless"
					repo.records["hero"] = dungeon.CharacterDungeonRecord{CharacterID: "hero", HighestDungeonCleared: 1}
				}
				old, err := svc.StartExpedition(ctx, "hero", dungeonID)
				if err != nil {
					t.Fatal(err)
				}
				if outcome == "wipeout" {
					old.TurnsRemaining = 1
					if err := store.SaveActiveExpedition(ctx, *old); err != nil {
						t.Fatal(err)
					}
				}
				if duplicate {
					repo.histories["hero"] = []dungeon.DungeonExpeditionHistory{{ID: old.ID}}
				}
				next := *old
				next.ID, next.AccumulatedGold = id.New(), 77
				next.AccumulatedItems = nil
				next.ConsumedEvents = nil
				repo.replace = func() {
					if err := store.SaveActiveExpedition(ctx, next); err != nil {
						t.Fatal(err)
					}
					if err := repo.SaveActiveExpedition(ctx, next); err != nil {
						t.Fatal(err)
					}
				}
				var result dungeon.ExpeditionStepResult
				if outcome == "escape" {
					result, err = svc.Escape(ctx, "hero")
				} else {
					result, err = svc.Move(ctx, "hero", dungeon.DirectionEast)
				}
				if err != nil || !result.IsFinished {
					t.Fatalf("expected terminal settlement: %+v %v", result, err)
				}
				if got, err := store.GetActiveExpedition(ctx, "hero"); err != nil || !reflect.DeepEqual(got, &next) {
					t.Fatalf("terminal cleanup changed replacement: %+v %v", got, err)
				}
				if got := repo.active["hero"]; !reflect.DeepEqual(got, &next) {
					t.Fatalf("fallback cleanup changed replacement: %+v", got)
				}
			})
		}
	}
}
