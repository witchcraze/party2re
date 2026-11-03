package depot

import (
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestAppendDeliveryKeepsSeparateInstancesAboveCapacity(t *testing.T) {
	dep := Depot{CharacterID: "referrer", Capacity: 1, Items: []item.Instance{{ID: "existing", DefinitionID: "item-023", Quantity: 2}}}
	for _, id := range []string{"delivery-1", "delivery-2"} {
		dep.AppendDelivery(item.Instance{ID: id, DefinitionID: "item-023", Quantity: 1})
	}
	if dep.Capacity != 1 || len(dep.Items) != 3 || dep.Items[0].Quantity != 2 || dep.Quantity("item-023") != 4 {
		t.Fatalf("delivery changed capacity or stacked instances: %+v", dep)
	}
	if err := dep.AddItem(item.Instance{ID: "ordinary", DefinitionID: "weapon-01", Quantity: 1}); !errors.Is(err, ErrDepotFull) {
		t.Fatalf("ordinary addition above capacity: %v", err)
	}
}
