package battle

// checkRevival retains consumption evidence for damage, backlash and status deaths.
func (ctx *battleContext) checkRevival(target Participant) RevivalResult {
	mp := ctx.mpMap[target.ID]
	target.Abilities = ctx.abilitiesMap[target.ID]
	res := CheckRevival(&target, &mp)
	ctx.mpMap[target.ID] = mp
	if res.ConsumedItem != nil {
		if ctx.consumedItems == nil {
			ctx.consumedItems = make(map[string][]ConsumedItem)
		}
		ctx.consumedItems[target.ID] = append(ctx.consumedItems[target.ID], *res.ConsumedItem)
	}
	return res
}
