package battle

import (
	"fmt"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// JobAssets belongs to the requesting player, independently of the effect actor.
// The consumer loads and commits these facts inside its command transaction.
type JobAssets struct {
	RequesterID   string
	Wallet        int
	HeldToolID    *string                     // nil means the durable single-hand projection is unavailable.
	HelperToolIDs []int                       // nil means helper-quest facts are unavailable; empty is known.
	AddWallet     func(int, int) (int, error) `json:"-"` // Inject the shared safe arithmetic; cap only at durable commit.
}

type JobAssetContext struct {
	DamageContext
	Arguments     []string
	Assets        *JobAssets
	DefeatContext *DefeatContext
	Roster        *RosterContext // Enhanced item78's summon pool; other bodies ignore it.
}

func (c *JobAssetContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (c *JobAssetContext) power(a *LifetimeActor, id string) float64 {
	switch id {
	case "job-7-entry-0", "job-8-entry-0":
		return max(1, min(500, float64(c.Assets.Wallet)*.03))
	case "job-7-entry-2":
		return float64(a.Attack) * .8
	case "job-28-entry-3":
		if c.Assets.Wallet < 100 {
			return 50
		}
		return 180
	case "job-78-entry-5":
		return float64(a.Agility) * 1.4
	case "job-81-entry-2":
		return float64(a.Attack) * 1.2
	}
	return 0
}

// ApplyJobAssets executes intrinsic effects only; ordinary costs and random MP
// restoration belong to the enclosing invocation, SQL writes to Adventure.
func (s *LifetimeState) ApplyJobAssets(c *JobAssetContext, id string, rng random.Generator) error {
	if err := s.ValidateJobAssets(c, id, rng); err != nil {
		return err
	}
	c.ActorNPC = s.actor(c.ActorID).NPC
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		s.actor(c.ActorID).MP = max(0, s.actor(c.ActorID).MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	// Match admission: without a native closure the supplied Defeat stays bound.
	if c.DefeatContext != nil {
		previousReset, previousDefeat := c.Reset, c.Defeat
		damage := s.nativeDamageContext(c.DamageContext, c.DefeatContext, rng)
		c.Reset, c.Defeat = damage.Reset, damage.Defeat
		defer func() { c.Reset, c.Defeat = previousReset, previousDefeat }()
	}
	if id == "job-78-entry-0" {
		return s.applyEnhancedTool(c, rng)
	}
	power := c.power(s.actor(c.ActorID), id)
	if id == "job-28-entry-3" {
		c.Assets.Wallet = max(0, c.Assets.Wallet-100)
		_, err := s.ApplyAreaDamage(&c.DamageContext, power, CategoryPhysical, true, rng)
		return err
	}
	r, err := s.ApplyDamage(&c.DamageContext, dualAgilityEffect(c.shift(), power, CategoryPhysical, false), rng)
	if err != nil || !r.Returned || r.TargetID == "" {
		return err
	}
	switch id {
	case "job-7-entry-2", "job-81-entry-2":
		credit := int(float64(r.Amount)*.1) + 5
		if id == "job-81-entry-2" {
			if _, err := s.ApplyHealing(&c.EffectContext, controlEffect(c.ActorID, float64(r.Amount)*.75, CategoryPhysical), rng); err != nil {
				return err
			}
			credit = min(400, int(float64(r.Amount)*.3)+5)
		}
		wallet, err := c.Assets.AddWallet(c.Assets.Wallet, credit)
		if err != nil {
			return err
		}
		c.Assets.Wallet = wallet
	case "job-78-entry-5":
		if _, err := s.ApplyStatUp(&c.EffectContext, controlEffect(c.ActorID, .25, CategoryUntyped), StatAgility, rng); err != nil {
			return err
		}
		a := s.actor(c.ActorID)
		if *a.HeldToolID == "" && rng.Float64()*5 < 1 {
			pool := [...]int{1, 1, 1, 1, 1, 2, 2, 3, 4, 11, 11, 12, 15, 15, 41, 42, 80, 80, 81, 81, 84, 84}
			selected := pool[int(rng.Float64()*float64(len(pool)))]
			tool := fmt.Sprintf("item-%03d", selected)
			for _, helper := range c.Assets.HelperToolIDs {
				// Numeric regex matching in the source is substring matching, after the draw.
				if strings.Contains(fmt.Sprint(helper), fmt.Sprint(selected)) {
					tool = ""
					break
				}
			}
			*a.HeldToolID = tool
			if !a.NPC {
				*c.Assets.HeldToolID = tool
			}
		}
	}
	return nil
}
