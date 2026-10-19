package battle

import (
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
)

// This finite list includes all source category3 callbacks, including empty
// bodies. New catalog entries do not acquire reset certification implicitly.
func resetAccessorySupported(id string) bool {
	switch id {
	case "item-110", "item-111", "item-112", "item-115", "item-116", "item-117", "item-118", "item-119", "item-120", "item-121", "item-122", "item-123", "item-124",
		"item-143", "item-144", "item-145", "item-146", "item-147", "item-148", "item-149",
		"item-152", "item-153", "item-154", "item-155", "item-156", "item-157", "item-158", "item-159", "item-160", "item-161", "item-162", "item-163", "item-164", "item-165", "item-166", "item-167",
		"item-169", "item-170", "item-171", "item-172", "item-173", "item-174", "item-175", "item-176", "item-177", "item-178", "item-179", "item-180", "item-181", "item-182", "item-183",
		"item-185", "item-186", "item-187", "item-188", "item-189", "item-190", "item-191", "item-192", "item-193", "item-194", "item-195", "item-196", "item-197", "item-198",
		"item-200", "item-201", "item-202", "item-204", "item-205", "item-206", "item-208", "item-209", "item-210", "item-211", "item-212", "item-215", "item-217", "item-218", "item-219", "item-220", "item-221", "item-222", "item-223", "item-224", "item-225", "item-226", "item-227", "item-228", "item-229", "item-230",
		"item-238", "item-239", "item-240", "item-241", "item-242", "item-243", "item-244", "item-246", "item-247", "item-249", "item-250", "item-251", "item-252", "item-253", "item-254", "item-255", "item-256", "item-258", "item-260", "item-261", "item-263", "item-264", "item-265", "item-266", "item-267", "item-268":
		return true
	}
	return false
}

func (s *LifetimeState) applyResetAccessory(c *EquipmentResetContext, a *LifetimeActor, rng random.Generator) error {
	id := a.Gear.AccessoryID
	at, df, ag := item.CalculateAccessoryStats(id)
	a.Attack += at
	a.Defense += df
	a.Agility += ag
	switch id {
	case "item-111":
		if a.HP > 0 {
			a.MP = min(999, a.MP+5+int(rng.Float64()*9))
		}
	case "item-112":
		if a.HP > 0 {
			a.HP = min(999, a.HP+9+int(rng.Float64()*9))
		}
	case "item-115":
		if rng.Float64()*3 <= 1 {
			a.Tension, a.Temporary = 1.7, TemporaryDouble
		}
	case "item-116":
		a.Status = StatusDofuu
	case "item-143":
		a.Status = StatusPhysicalSeal
	case "item-145":
		a.Defense = int(float64(a.Defense) * .9)
		if float64(a.HP) <= float64(a.MaxHP)*.3 {
			a.Attack = int(float64(a.Attack) * 1.2)
		}
	case "item-147":
		if rng.Float64()*4 <= 1 {
			a.Attack += 10
		}
	case "item-149":
		a.Hit = min(99, a.Hit+3)
	case "item-159":
		c.Cursor = TargetCursor{Source: "item159", ActorID: a.ID}
		for _, v := range []*int{&a.Attack, &a.Defense, &a.Agility} {
			if rng.Float64()*3 <= 1 {
				*v = int(float64(*v) * 1.1)
			}
		}
	case "item-162":
		if a.HP > 0 {
			a.Attack = int(float64(a.Attack) * .8)
			a.Agility = int(float64(a.Agility) * .8)
		}
	case "item-163":
		if a.HP > 0 {
			a.Temporary = TemporaryReflectMagic
		}
	case "item-187":
		w, err := resetJobWeights(a.Jobs.JobID)
		if err != nil {
			return err
		}
		sum := w.Attack + w.Defense + w.Agility
		if sum > 0 {
			total := int(float64(a.Attack+a.Defense+a.Agility) * 1.3)
			a.Attack = int(float64(total) * (float64(w.Attack) / float64(sum)))
			a.Defense = int(float64(total) * (float64(w.Defense) / float64(sum)))
			a.Agility = int(float64(total) * (float64(w.Agility) / float64(sum)))
		}
	case "item-197":
		a.Hit = min(99, a.Hit+4)
	case "item-209":
		if a.HP > 0 {
			a.Defense = int(float64(a.Defense) * 1.2)
		}
	case "item-247":
		if a.HP > 0 {
			a.Extended, a.ExtendedTurns = ExtendedIncense, 2+int(rng.Float64()*3)
		}
	case "item-251", "item-252", "item-253", "item-254", "item-255":
		// Reset discards the appraisal result, but still consumes its draw.
		rng.Float64()
	}
	return nil
}
