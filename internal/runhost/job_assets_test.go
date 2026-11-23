package runhost

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/economy"
)

type mockJobAssetRunner struct {
	tc *economy.TxContext
}

func (m *mockJobAssetRunner) ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error) {
	if fn != nil {
		return &economy.TransactionResult{}, fn(m.tc)
	}
	return &economy.TransactionResult{}, nil
}

func TestWithJobAssetsPreservesKnownAndUnknownHands(t *testing.T) {
	tests := []struct {
		name     string
		items    []string
		wantHeld *string
	}{
		{
			name:     "empty inventory proves empty hand",
			items:    nil,
			wantHeld: ptr(""),
		},
		{
			name:     "single tool proves held tool",
			items:    []string{"item-001"},
			wantHeld: ptr("item-001"),
		},
		{
			name:     "single accessory proves held accessory",
			items:    []string{"item-012"},
			wantHeld: ptr("item-012"),
		},
		{
			name:     "weapon and armor prove an empty hand",
			items:    []string{"weapon-01", "armor-01"},
			wantHeld: ptr(""),
		},
		{
			name:     "weapon and armor beside a tool prove that tool",
			items:    []string{"weapon-01", "armor-01", "item-001"},
			wantHeld: ptr("item-001"),
		},
		{
			name:     "two tool-hand rows leave hand unknown",
			items:    []string{"item-001", "item-002"},
			wantHeld: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			charID := "actor-1"
			playerID := "player-1"
			runID := "run-1"

			inv, err := coreinventory.New(charID)
			if err != nil {
				t.Fatal(err)
			}
			for i, defID := range tt.items {
				inst := coreitem.Instance{ID: string(rune('a' + i)), DefinitionID: defID, Quantity: 1}
				inv.Items = append(inv.Items, inst)
			}

			tc := &economy.TxContext{
				Character: corecharacter.Character{ID: charID, PlayerID: playerID, Money: 100},
				Inventory: inv,
			}
			runner := &mockJobAssetRunner{tc: tc}
			svc := &Service[testRun, *testRun]{cfg: Config[testRun]{JobAssetRunner: runner}}

			r := &State{ID: runID, Roster: []string{charID}, Recipients: map[string]Recipient{charID: {PlayerID: playerID}}}
			cmd := Command{
				RunID:    runID,
				PlayerID: playerID,
				ActorID:  charID,
			}

			ctx := commandContext(context.Background(), runID)
			var captured *corebattle.JobAssets
			err = svc.withJobAssets(ctx, r, cmd, func(assets *corebattle.JobAssets, _ *corecharacter.Character) error {
				captured = assets
				return nil
			})
			if err != nil {
				t.Fatalf("withJobAssets error = %v", err)
			}
			if captured == nil {
				t.Fatal("expected apply callback to be called")
			}
			if tt.wantHeld == nil {
				if captured.HeldToolID != nil {
					t.Fatalf("HeldToolID = %v, want nil", *captured.HeldToolID)
				}
			} else {
				if captured.HeldToolID == nil || *captured.HeldToolID != *tt.wantHeld {
					t.Fatalf("HeldToolID = %v, want %v", captured.HeldToolID, *tt.wantHeld)
				}
			}
		})
	}
}

func ptr(s string) *string {
	return &s
}

func TestWithJobAssetsRequiresTheRunCommand(t *testing.T) {
	r := &State{ID: "run-1", Roster: []string{"actor-1"}, Recipients: map[string]Recipient{"actor-1": {PlayerID: "player-1"}}}
	cmd := Command{RunID: "run-1", PlayerID: "player-1", ActorID: "actor-1"}
	svc := &Service[testRun, *testRun]{cfg: Config[testRun]{JobAssetRunner: &mockJobAssetRunner{}}}
	err := svc.withJobAssets(context.Background(), r, cmd, func(*corebattle.JobAssets, *corecharacter.Character) error {
		t.Fatal("mutation outside Command")
		return nil
	})
	if !errors.Is(err, ErrForbidden) {
		t.Fatal(err)
	}
}
