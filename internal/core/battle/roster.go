package battle

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/core/random"
)

// RosterContext belongs to one operation. Requester facts stay unchanged when
// a commanded helper becomes actor. EnemyPool is the current stage's complete
// payload, with weighted appearance positions supplied by the stage owner.
type RosterContext struct {
	DamageContext
	RequesterItemID     string
	EnemyTeamID         string
	EnemyPool           []LifetimeActor
	AppearancePositions []int
	Hard                bool
	Status              *RosterStatusCallbacks
}

// RosterStatusCallbacks connects the status owner's fully certified targeted
// helper. Validate is pure and includes all reachable redirects/reset effects;
// a missing half rejects the enclosing roster producer before any damage.
type RosterStatusCallbacks struct {
	Validate func(*LifetimeState, TargetContext, TargetEffect, float64) error
	Execute  func(*LifetimeState, TargetContext, TargetEffect, float64, random.Generator) error
}

type RosterResult struct {
	Applied        bool
	HelperID       string
	TargetConsumed bool
	Effects        []EffectApplication
	Hits           []NormalHit
}

type HelperPayload struct {
	Identity, Name string
	Form           int
	Stats          [5]float64 // HP, MP, AT, DF, AG before independent summon rolls.
	Jobs           BattleJobs
}

func (s *LifetimeState) validateHelper(c *RosterContext, p HelperPayload, rng random.Generator) error {
	if c == nil {
		return ErrInvalidRequest
	}
	if err := s.validateTargetContext(c.TargetContext, rng); err != nil {
		return err
	}
	if p.Identity == "" || p.Name == "" {
		return ErrInvalidParticipant
	}
	if s.rosterIdentity(p.Identity) != nil {
		return nil
	}
	if p.Jobs.SP < 0 || p.Jobs.OldSP < 0 {
		return ErrInvalidParticipant
	}
	if c.RequesterLevel == nil || *c.RequesterLevel < 0 {
		return ErrUnsupportedEffect
	}
	for _, v := range p.Stats {
		if v < 0 || v > 1e12 || v != v {
			return ErrInvalidParticipant
		}
	}
	if s.RosterSequence >= 1e15 {
		return ErrInvalidParticipant
	}
	return nil
}

// AddHelper is shared by calling, tools and field recreation. Duplicate
// presence, including a fallen record, is checked before draws or prerequisites.
func (s *LifetimeState) AddHelper(c *RosterContext, p HelperPayload, rng random.Generator) (RosterResult, error) {
	r := RosterResult{}
	if err := s.validateHelper(c, p, rng); err != nil {
		return r, err
	}
	if s.rosterIdentity(p.Identity) != nil {
		return r, nil
	}
	a := s.actor(c.ActorID)
	values := summonStats(p.Stats, c.RequesterItemID == "item-165", rng)
	id := s.nextRosterID()
	helper := LifetimeActor{Participant: Participant{
		ID: id, Name: p.Name, TeamID: a.TeamID,
		HP: values[0], MaxHP: values[0], MP: values[1], MaxMP: values[1],
		Attack: values[2], Defense: values[3], Agility: values[4],
		BaseStats: BattleBaseStats{values[2], values[3], values[4]}, Gear: &BattleGearStats{},
		Icon:             "assets/images/placeholders/battle/helpers.svg#" + p.Identity + "-" + strconv.Itoa(p.Form),
		ExperienceReward: *c.RequesterLevel / 2, GoldReward: *c.RequesterLevel / 4,
	}, Jobs: p.Jobs, OriginalJobs: p.Jobs, NPC: a.NPC, RosterIdentity: p.Identity, SummonForm: p.Form, Hit: 95, Tension: 1}
	s.Actors = append(s.Actors, helper)
	c.PartyIDs = append(c.PartyIDs, id)
	return RosterResult{Applied: true, HelperID: id}, nil
}

func summonStats(base [5]float64, bonus bool, rng random.Generator) (values [5]int) {
	for i, v := range base {
		values[i] = int(v * (.9 + rng.Float64()*.3))
		if bonus {
			values[i] = int(float64(values[i]) * 1.2)
		}
	}
	return
}

func (s *LifetimeState) nextRosterID() string {
	for {
		s.RosterSequence++
		id := fmt.Sprintf("summoned-%d", s.RosterSequence)
		if s.actor(id) == nil {
			return id
		}
	}
}

func (s *LifetimeState) rosterIdentity(identity string) *LifetimeActor {
	for i := range s.Actors {
		if s.Actors[i].RosterIdentity == identity {
			return &s.Actors[i]
		}
	}
	return nil
}

// PruneRoster is the explicit save boundary, never a per-hit deletion. Fallen
// real players retain their position and identity. Dismissed helpers leave all
// three ordered rosters here; recalling creates a new incarnation.
func (s *LifetimeState) PruneRoster(c *TargetContext) {
	actor := s.actor(c.ActorID)
	actorTeam := ""
	if actor != nil {
		actorTeam = actor.TeamID
	}
	s.Actors = slices.DeleteFunc(s.Actors, func(a LifetimeActor) bool { return a.HP <= 0 && (a.NPC || a.RosterIdentity != "") })
	c.PartyIDs, c.EnemyIDs = nil, nil
	for i := range s.Actors {
		a := &s.Actors[i]
		a.RosterDetached = false
		if actor != nil && a.TeamID == actorTeam {
			c.PartyIDs = append(c.PartyIDs, a.ID)
		} else {
			c.EnemyIDs = append(c.EnemyIDs, a.ID)
		}
	}
}

func callingIdentity(job int) string { return "calling-" + strconv.Itoa(job) }

// FindCallingHelper uses presence before liveness; a fallen named helper never
// falls back. Appearance matching is ordered, prefix-based, excludes self and
// mixed forms, and deliberately ignores current and previous job IDs.
func (s *LifetimeState) FindCallingHelper(c TargetContext, job int) string {
	if a := s.rosterIdentity(callingIdentity(job)); a != nil {
		return a.ID
	}
	prefix := job
	switch job {
	case 59:
		prefix = 39
	case 60:
		prefix = 41
	case 62:
		prefix = 56
	case 65:
		prefix = 45
	case 67:
		prefix = 40
	case 71:
		prefix = 44
	case 80:
		prefix = 55
	}
	for _, id := range c.PartyIDs {
		a := s.actor(id)
		if a == nil || id == c.ActorID || strings.Contains(a.Icon, "mix") {
			continue
		}
		if strings.HasPrefix(a.Icon, strings.TrimSuffix(nativeFormIcon("job-"+strconv.Itoa(prefix), ""), "-")) {
			return id
		}
	}
	return ""
}
