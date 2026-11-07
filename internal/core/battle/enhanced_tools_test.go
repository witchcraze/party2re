package battle

import (
	"math"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/core/item"
)

// The cost is floor(base price×2.5) for odd prices too (_skill.cgi:10296–10300).
func TestEnhancedPurchaseCostFloorsFractions(t *testing.T) {
	catalog, err := item.DefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	odd := false
	for _, d := range catalog.Definitions() {
		if !strings.HasPrefix(d.ID, "item-") {
			continue
		}
		_, number, err := d.LegacyStorageKey()
		if err != nil {
			t.Fatal(err)
		}
		got, err := enhancedPurchaseCost(number)
		if err != nil || got != int(math.Floor(float64(d.Price)*2.5)) {
			t.Fatal(d.ID, d.Price, got, err)
		}
		odd = odd || d.Price%2 == 1
	}
	if !odd {
		t.Fatal("catalog has no odd price to exercise the fraction")
	}
}
