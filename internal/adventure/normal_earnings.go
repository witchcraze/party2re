package adventure

import (
	"slices"

	"github.com/witchcraze/party2re/internal/economy"
)

func addRunEarnings(a, b RunEarnings) (RunEarnings, error) {
	if b.EXP < 0 || b.Gold < 0 || b.Crystals < 0 {
		return RunEarnings{}, ErrRunConflict
	}
	var err error
	a.EXP, err = economy.SafeAdd(a.EXP, b.EXP)
	if err != nil {
		return RunEarnings{}, err
	}
	a.Gold, err = economy.SafeAdd(a.Gold, b.Gold)
	if err != nil {
		return RunEarnings{}, err
	}
	a.Crystals, err = economy.SafeAdd(a.Crystals, b.Crystals)
	return a, err
}

func scaleTryEarnings(e RunEarnings) (RunEarnings, error) {
	// int(total * 1.5), without floating-point rounding or intermediate overflow.
	return addRunEarnings(e, RunEarnings{EXP: e.EXP / 2, Gold: e.Gold / 2, Crystals: e.Crystals / 2})
}

func (r *NormalRun) recordEarnings(earned map[string]RunEarnings) error {
	for id, e := range earned {
		if !slices.Contains(r.Roster, id) {
			return ErrRunForbidden
		}
		p := r.Recipients[id]
		var err error
		if r.Mode == CrawlModeTry {
			p.Conditional, err = addRunEarnings(p.Conditional, e)
		} else {
			p.Pending, err = addRunEarnings(p.Pending, e)
		}
		if err != nil {
			return err
		}
		r.Recipients[id] = p
	}
	return nil
}
