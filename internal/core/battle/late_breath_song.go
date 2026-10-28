package battle

import (
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// LateBreathSongContext freezes BurnStrike at table construction. HeldBooks is
// current operation input, keyed by actor: none, ordinary or forbidden. Missing
// book facts reject book-power; asset classification remains the asset owner.
type LateBreathSongContext struct {
	DamageContext
	Arguments  []string
	BurnStrike bool
	HeldBooks  map[string]string
	Death      *StatusContext
}

// LateBreathSongBurn uses the requester's gender, independently of the actor's
// original job. Save the result with frozen command input before fields change.
func (a LifetimeActor) LateBreathSongBurn(field, requesterGender string) bool {
	return field == "burn" && strings.Contains(a.Icon, nativeFormIcon("job-84-ex", requesterGender))
}

// LateBreathSongEntries preserves complete ordered tables. Field and book-asset
// bodies retain their separate owners and are not certified by registration.
func LateBreathSongEntries(job int) []CallbackEntry {
	switch job {
	case 84:
		return []CallbackEntry{
			{ID: "job-84-entry-0", Name: "breath-composure", RequiredSP: 5, MPCost: 6},
			{ID: "job-84-entry-1", Name: "fixed-breath", RequiredSP: 20, MPCost: 16},
			{ID: "job-84-entry-2", Name: "burning-strike", RequiredSP: 50, MPCost: 26},
			{ID: "job-84-entry-3", Name: "passion-dance", RequiredSP: 80, MPCost: 14},
			{ID: "job-84-entry-4", Name: "burn-field", RequiredSP: 140, MPCost: 35},
		}
	case 85:
		return []CallbackEntry{
			{ID: "job-85-entry-0", Name: "mana-ritual", RequiredSP: 25, MPCost: 1},
			{ID: "job-85-entry-1", Name: "ending-song", RequiredSP: 50, MPCost: 39},
			{ID: "job-85-entry-2", Name: "undo-song", RequiredSP: 75, MPCost: 47},
			{ID: "job-85-entry-3", Name: "binding-song", RequiredSP: 125, MPCost: 45},
			{ID: "job-85-entry-4", Name: "chain-burst", RequiredSP: 150, MPCost: 35},
		}
	case 86:
		return []CallbackEntry{
			{ID: "job-86-entry-0", Name: "book-search", RequiredSP: 5, MPCost: 3},
			{ID: "job-86-entry-1", Name: "library-field", RequiredSP: 5, MPCost: 10},
			{ID: "job-86-entry-2", Name: "book-mana", RequiredSP: 30, MPCost: 2},
			{ID: "job-86-entry-3", Name: "book-release", RequiredSP: 60, MPCost: 38},
			{ID: "job-86-entry-4", Name: "book-power", RequiredSP: 100, MPCost: 54},
			{ID: "job-86-entry-5", Name: "book-chain", RequiredSP: 150},
		}
	}
	return nil
}

func (c *LateBreathSongContext) shift() string {
	if len(c.Arguments) == 0 {
		return ""
	}
	id := c.Arguments[0]
	c.Arguments = c.Arguments[1:]
	return id
}

func (s *LifetimeState) lateSongDamage(c *LateBreathSongContext, id, target string) TargetEffect {
	a := s.actor(c.ActorID)
	switch id {
	case "job-84-entry-1":
		return dualAgilityEffect(target, 140, CategoryBreath, true)
	case "job-84-entry-2":
		coefficient := 2.0
		if c.BurnStrike {
			coefficient = 2.5
		}
		return dualAgilityEffect(target, float64(a.Attack)*coefficient, CategoryPhysical, false)
	case "job-85-entry-4":
		power := 100.0
		if t := s.actor(target); t != nil && t.Status == StatusSabaku {
			power = 700
		}
		return dualAgilityEffect(target, power, CategorySong, true)
	default:
		coefficient := .2
		switch c.HeldBooks[c.ActorID] {
		case "ordinary":
			coefficient = .4
		case "forbidden":
			coefficient = .75
		}
		return dualAgilityEffect(target, float64(a.MaxMP)*coefficient, CategoryMagic, true)
	}
}

// applyManaRitual is the shared intrinsic HP-to-MP body of source75.4/85.0.
func (s *LifetimeState) applyManaRitual(c *EffectContext, rng random.Generator) error {
	a := s.actor(c.ActorID)
	spent := min(int(float64(a.MaxHP)*.3*(.9+rng.Float64()*.3)), a.HP-1)
	a.HP -= spent
	_, err := s.ApplyMPHealing(c, controlEffect(a.ID, float64(spent)*.4+5, CategoryUntyped), rng)
	return err
}

// ProjectLateBreathSong carries complete deterministic inline bodies into
// later admission. It leaves all state and arguments unchanged for other IDs.
func (s *LifetimeState) ProjectLateBreathSong(c *LateBreathSongContext, id string) bool {
	a := s.actor(c.ActorID)
	switch id {
	case "job-84-entry-0":
		a.Hit, a.Temporary = 95, TemporaryReduceBreath
	case "job-85-entry-2":
		t := s.actor(c.shift())
		if t == nil {
			t = a
		}
		t.Status, t.Temporary = "", ""
	default:
		return false
	}
	return true
}

// ApplyLateBreathSong executes intrinsic bodies without ordinary MP costs.
func (s *LifetimeState) ApplyLateBreathSong(c *LateBreathSongContext, id string, rng random.Generator) error {
	if err := s.ValidateLateBreathSong(c, id, rng); err != nil {
		return err
	}
	if s.ProjectLateBreathSong(c, id) {
		return nil
	}
	previousDamage, previousEffect := c.targetResources, c.EffectContext.targetResources
	resources := func(r TargetResolution) {
		a := s.actor(c.ActorID)
		a.MP = max(0, a.MP-r.MPDebit)
		applyStatusResources(s, r)
	}
	c.targetResources, c.EffectContext.targetResources = resources, resources
	defer func() { c.targetResources, c.EffectContext.targetResources = previousDamage, previousEffect }()
	a := s.actor(c.ActorID)
	switch id {
	case "job-84-entry-1", "job-84-entry-2", "job-86-entry-4":
		e := s.lateSongDamage(c, id, c.shift())
		_, err := s.ApplyDamage(&c.DamageContext, e, rng)
		if err == nil && id == "job-86-entry-4" && c.HeldBooks[c.ActorID] == "forbidden" {
			_, err = s.ApplyRecoil(&c.EffectContext, &c.End, e.Payload.Number*.2, rng)
		}
		return err
	case "job-84-entry-3":
		allowed, err := a.CheckCategorySeal(CategoryDance, rng)
		if err != nil || !allowed {
			return err
		}
		r, err := s.ResolvePartyTarget(c.TargetContext, controlSymbol(c.shift(), "passion", CategoryUntyped), rng)
		if err != nil {
			return err
		}
		resources(r)
		if err := s.statTension(&c.EffectContext, a, false, rng); err != nil {
			return err
		}
		target := r.TargetID
		if !r.Resolved || target == "" {
			target = a.ID
		}
		if target != a.ID {
			return s.statTension(&c.EffectContext, s.actor(target), false, rng)
		}
	case "job-85-entry-0":
		return s.applyManaRitual(&c.EffectContext, rng)
	case "job-85-entry-1":
		child := *c.Death
		child.EffectContext = c.EffectContext
		_, err := s.ApplyAreaStatus(&child, controlSymbol("", "instant_death", CategorySong), 18, false, true, rng)
		c.EffectContext = child.EffectContext
		*c.Death = child
		return err
	case "job-85-entry-3", "job-85-entry-4":
		r, err := s.ResolveEnemyTarget(c.TargetContext, controlSymbol(c.shift(), StatusSabaku, CategorySong), rng)
		if err != nil {
			return err
		}
		resources(r)
		if id == "job-85-entry-3" {
			if r.Resolved && rng.Float64()*5 < 3 {
				s.actor(r.TargetID).Status, s.actor(r.TargetID).Tension = StatusSabaku, 1
			}
			return nil
		}
		target := r.TargetID
		if !r.Resolved {
			target = ""
		}
		_, err = s.ApplyDamage(&c.DamageContext, s.lateSongDamage(c, id, target), rng)
		return err
	}
	return nil
}
