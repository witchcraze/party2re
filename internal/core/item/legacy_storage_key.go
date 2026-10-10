package item

import (
	"errors"
	"strconv"
	"strings"
)

// ErrNoLegacyStorageKey is returned when a definition ID is not a canonical
// weapon-NN, armor-NN or item-NNN catalog identity.
var ErrNoLegacyStorageKey = errors.New("item definition has no legacy storage key")

// LegacyStorageKey returns the kind and number the original Party2 stored for
// this item (depot.cgi azukeru/send_item): 1 = weapon, 2 = armor and
// 3 = any entry of the item catalog, including shields and accessories.
// This is distinct from Slot.Kind(), which groups equipment by slot.
func (d Definition) LegacyStorageKey() (kind, no int, err error) {
	for prefix, k := range map[string]int{"weapon-": 1, "armor-": 2, "item-": 3} {
		suffix, ok := strings.CutPrefix(d.ID, prefix)
		if !ok {
			continue
		}
		n, convErr := strconv.Atoi(suffix)
		if convErr != nil || suffix[0] < '0' || suffix[0] > '9' {
			break
		}
		return k, n, nil
	}
	return 0, 0, ErrNoLegacyStorageKey
}
