package home

// sealRefundCrystals returns the number of crystals (刻印晶) refunded when a
// weapon seal of the given ID is removed via the raw-crystal consumable.
// The refund is 50% of the original crystal cost for each seal.
func sealRefundCrystals(sealID int) int {
	switch sealID {
	case 1, 4:
		return 25
	case 2, 5:
		return 250
	case 3, 6:
		return 2500
	case 7, 8, 9, 10, 11, 12:
		return 50
	default:
		return 0
	}
}
