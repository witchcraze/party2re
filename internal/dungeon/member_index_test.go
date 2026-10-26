package dungeon_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
)

func TestMemberIndex(t *testing.T) {
	for _, live := range []bool{false, true} {
		name := "memory"
		if live {
			name = "valkey"
		}
		t.Run(name, func(t *testing.T) {
			cleanupReference := func(string) {}
			var store dungeon.ActiveExpeditionStore = dungeon.NewMemoryExpeditionRepository()
			if live {
				client := openTestValkey(t)
				t.Cleanup(client.Close)
				r, err := dungeon.NewValkeyExpeditionRepository(client)
				if err != nil {
					t.Fatal(err)
				}
				store = r
				cleanupReference = func(actor string) {
					key := "party2:dungeon:{char:" + actor + "}:member"
					if err := client.Do(context.Background(), client.B().Del().Key(key).Build()).Error(); err != nil {
						t.Error(err)
					}
				}
			}
			ctx := context.Background()
			leader, follower := id.New(), id.New()
			t.Cleanup(func() { cleanupReference(follower) })
			run := sampleExpedition(id.New(), leader, "0")
			run.PartyID = "consented-party"
			run.Members = []dungeon.ExpeditionMember{{CharacterID: leader}, {CharacterID: follower}}
			if err := store.SaveActiveExpedition(ctx, run); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, leader, run.ID); err != nil {
					t.Error(err)
				}
			})
			got, err := store.GetActiveExpedition(ctx, follower)
			if err != nil || got == nil || got.ID != run.ID {
				t.Fatalf("member missing: %+v %v", got, err)
			}
			if got, err := store.GetActiveExpedition(ctx, id.New()); err != nil || got != nil {
				t.Fatalf("unrelated actor: %+v %v", got, err)
			}
			outcome, err := store.Step(ctx, leader, dungeon.StepParams{ExpectedExpeditionID: run.ID, NewFloor: 1, NewX: 1, NewY: 0, TurnsDelta: -1, Now: time.Now()})
			updated := outcome.Expedition
			if err != nil || updated.PartyID != run.PartyID || len(updated.Members) != 2 {
				t.Fatalf("transition lost roster: %+v %v", updated, err)
			}
			got, err = store.GetActiveExpedition(ctx, follower)
			if err != nil || got == nil || !got.UpdatedAt.Equal(updated.UpdatedAt) {
				t.Fatalf("member saw stale run: %+v %v", got, err)
			}
			// Rebinding a member to a new run cannot be erased by old-run cleanup.
			replacement := run
			replacement.ID = id.New()
			replacement.CharacterID = id.New()
			replacement.Members[0].CharacterID = replacement.CharacterID
			if err := store.SaveActiveExpedition(ctx, replacement); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(ctx, replacement.CharacterID, replacement.ID); err != nil {
					t.Error(err)
				}
			})
			if err := store.DeleteActiveExpedition(ctx, leader, run.ID); err != nil {
				t.Fatal(err)
			}
			got, err = store.GetActiveExpedition(ctx, follower)
			if err != nil || got == nil || got.ID != replacement.ID {
				t.Fatalf("old cleanup erased new enrollment: %+v %v", got, err)
			}
			// A stale reference does not authorize an actor removed from the canonical roster.
			replacement.Members = nil
			if err := store.SaveActiveExpedition(ctx, replacement); err != nil {
				t.Fatal(err)
			}

			if err := store.DeleteActiveExpedition(ctx, leader, run.ID); err != nil {
				t.Fatal(err)
			}
			if got, err := store.GetActiveExpedition(ctx, follower); err != nil || got != nil {
				t.Fatalf("member not released: %+v %v", got, err)
			}
		})
	}
}
