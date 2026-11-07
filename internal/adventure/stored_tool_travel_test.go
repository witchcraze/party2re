package adventure

import (
	"context"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// Job50 reaches travel only through this port; the stored row is consumed by
// the outcome, never by the travel transition.
func TestStoredToolTravelThroughJob50(t *testing.T) {
	for _, tc := range []struct {
		name, mode, stage string
		result            corebattle.StoredToolResult
	}{
		{"applied", "try", "stage-15", corebattle.StoredToolConsumed},
		{"wrong mode", "challenge", "stage-00", corebattle.StoredToolRetained},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, r, _, _ := travelFixture(t)
			r.Mode = tc.mode
			for i := range r.Battle.Actors {
				r.Battle.Actors[i].TeamID = "party"
			}
			ctx := context.WithValue(context.Background(), normalMutationContext{}, r.ID)
			travel, transition := s.PrepareStoredToolTravel(ctx, r, "a", nil)
			frame := corebattle.TargetContext{ActorID: "a", PartyIDs: []string{"a", "b"}}
			held := ""
			f := corebattle.CallbackContext{TargetContext: frame, Assets: &corebattle.JobAssets{
				RequesterID: "a", HeldToolID: &held, StoredItem: "item-066",
				StoredTools: []corebattle.StoredTool{{InstanceID: "wing", DefinitionID: "item-066", Category: item.UsageCategoryCombatOnly}},
			}}
			var d skill.BattleDefinitions
			d.RegisterPlayerCallbacks(skill.PlayerCallbackContexts{JobAssets: &corebattle.JobAssetContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: frame}}, Travel: travel}}, 2)
			ops, err := d.InvocationAction(&r.Battle, &f, []string{"job-50-entry-7"}, s.cfg.RNG)
			if err != nil {
				t.Fatal(err)
			}
			if err := r.Battle.ApplyInvocations(&f, d.Invocations, ops, s.cfg.RNG); err != nil {
				t.Fatal(err)
			}
			if r.Stage.ID != tc.stage || f.Assets.StoredOutcomes[0].Result != tc.result || transition.Consume || !r.Battle.StoredToolHistory["party"]["item-066"] {
				t.Fatal(r.Stage.ID, f.Assets.StoredOutcomes, transition, r.Battle.StoredToolHistory)
			}
		})
	}
	s, r, _, _ := travelFixture(t)
	travel, _ := s.PrepareStoredToolTravel(context.Background(), r, "a", nil)
	if err := travel.Validate("item-168", ""); err != corebattle.ErrUnsupportedEffect {
		t.Fatal("non-travel tool admitted", err)
	}
}
