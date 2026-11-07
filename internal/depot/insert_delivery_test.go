package depot

import (
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

func TestInsertDeliveryKeepsPositionAndClamps(t *testing.T) {
	d := Depot{Capacity: 1, Items: []item.Instance{{ID: "a"}, {ID: "c"}}}
	d.InsertDelivery(1, item.Instance{ID: "b"})
	d.InsertDelivery(9, item.Instance{ID: "d"})
	var got string
	for _, i := range d.Items {
		got += i.ID
	}
	if got != "abcd" {
		t.Fatal(got)
	}
}
