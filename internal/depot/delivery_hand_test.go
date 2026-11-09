package depot_test

import (
	"context"
	"fmt"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
)

// Hand eligibility is a caller contract (#1995): treasure (_npc_action.cgi:66–79) only
// fills an empty tool hand, while NPC delivery fills any empty matching hand.
func TestDeliverRewardItem_HandEligibilityByPolicy(t *testing.T) {
	for _, tc := range []struct {
		name      string
		held      []string
		depotUsed int
		found     string
		policy    depot.DeliveryPolicy
		want      depot.DeliveredTo
	}{
		{"treasure weapon, full depot, tool held", []string{"item-001"}, 5, "weapon-01", depot.PolicyTreatOverflowAsLost, depot.DeliveredToLost},
		{"treasure armor, full depot, tool held", []string{"item-001"}, 5, "armor-01", depot.PolicyTreatOverflowAsLost, depot.DeliveredToLost},
		{"treasure weapon, depot space, tool held", []string{"item-001"}, 4, "weapon-01", depot.PolicyTreatOverflowAsLost, depot.DeliveredToDepot},
		{"treasure weapon, depot space, empty hands", nil, 0, "weapon-01", depot.PolicyTreatOverflowAsLost, depot.DeliveredToDepot},
		{"treasure armor, full depot, empty hands", nil, 5, "armor-01", depot.PolicyTreatOverflowAsLost, depot.DeliveredToLost},
		{"treasure tool, full depot, empty tool hand", []string{"weapon-02"}, 5, "item-001", depot.PolicyTreatOverflowAsLost, depot.DeliveredToInventory},
		{"treasure tool, full depot, occupied tool hand", []string{"item-002"}, 5, "item-001", depot.PolicyTreatOverflowAsLost, depot.DeliveredToLost},
		{"treasure tool, full depot, matching held stack", []string{"item-001"}, 5, "item-001", depot.PolicyTreatOverflowAsLost, depot.DeliveredToLost},
		{"treasure tool, depot space, occupied tool hand", []string{"item-002"}, 4, "item-001", depot.PolicyTreatOverflowAsLost, depot.DeliveredToDepot},
		{"NPC weapon, empty weapon hand", []string{"item-001"}, 5, "weapon-01", depot.PolicyDeliverOverCapacity, depot.DeliveredToInventory},
		{"NPC weapon, occupied weapon hand", []string{"weapon-02"}, 5, "weapon-01", depot.PolicyDeliverOverCapacity, depot.DeliveredToDepot},
	} {
		t.Run(tc.name, func(t *testing.T) {
			char := corecharacter.Character{ID: "char-1", JobLevel: 0} // capacity 5
			inv, _ := coreinventory.New(char.ID)
			for _, id := range tc.held {
				inst, _ := item.NewInstance(id, 1)
				if err := inv.Add(inst); err != nil {
					t.Fatal(err)
				}
			}
			invRepo := &mockInvRepo{inv: inv}
			dep, _ := depot.NewDepotWithCapacity(char.ID, 0, 0, 0)
			for i := 0; i < tc.depotUsed; i++ {
				dep.AppendDelivery(item.Instance{ID: fmt.Sprintf("row-%d", i), DefinitionID: "weapon-02", Quantity: 1})
			}
			depotRepo := &mockDepotRepo{depot: dep}

			res, err := depot.DeliverRewardItem(context.Background(), invRepo, depotRepo, char, tc.found, 1, tc.policy)
			if err != nil {
				t.Fatal(err)
			}
			if res.DeliveredTo != tc.want {
				t.Fatalf("DeliveredTo = %s, want %s", res.DeliveredTo, tc.want)
			}
			wantInv, wantDepot := len(tc.held), tc.depotUsed
			switch tc.want {
			case depot.DeliveredToInventory:
				wantInv++
			case depot.DeliveredToDepot:
				wantDepot++
			}
			if len(invRepo.inv.Items) != wantInv || len(depotRepo.depot.Items) != wantDepot {
				t.Fatalf("inventory=%d depot=%d, want %d / %d", len(invRepo.inv.Items), len(depotRepo.depot.Items), wantInv, wantDepot)
			}
		})
	}
}
