package battle

// LifetimeState is the shared, serializable effect portion of an explicit battle.
// Encounter admission, receipts, timestamps and rewards belong to the consumer.
// Batch resolvers deliberately retain their existing round-based contract.
type LifetimeState struct {
	Actors           []LifetimeActor            `json:"actors"`
	Fields           LifetimeFields             `json:"fields"`
	SkillBans        map[string]int             `json:"skill_bans"`
	SkillHistory     map[string]map[string]bool `json:"skill_history"`
	DroppedCrystals  int                        `json:"dropped_crystals"`
	SpecialBossField bool                       `json:"special_boss_field"`
	RosterSequence   uint64                     `json:"roster_sequence"`
}

// LifetimeActor keeps current resources and effect facts together. Participant
// must come from the battle adapter, including equipment and instance identities.
type LifetimeActor struct {
	Participant
	Jobs           BattleJobs      `json:"jobs"`
	OriginalJobs   BattleJobs      `json:"original_jobs"`
	WeaponSeal     int             `json:"weapon_seal"`
	JobHistory     map[string]*int `json:"job_history,omitempty"` // nil SP means a recorded job with irrecoverable historical SP.
	NPC            bool            `json:"npc"`
	RosterIdentity string          `json:"roster_identity,omitempty"`
	RosterDetached bool            `json:"-"` // Dismissal changes color before the request's ordered arrays are rebuilt.
	SummonForm     int             `json:"summon_form,omitempty"`
	CrystalReward  int             `json:"crystal_reward,omitempty"`
	Tension        float64         `json:"tension"`
	Hit            int             `json:"hit"`
	Temporary      TemporaryEffect `json:"temporary,omitempty"`
	Extended       ExtendedEffect  `json:"extended,omitempty"`
	ExtendedTurns  int             `json:"extended_turns,omitempty"`
	CMPCharge      int             `json:"cmp_charge"`
	CMPNeedCharge  int             `json:"cmp_need_charge"`
	Banished       bool            `json:"banished"`
	ConsumedItems  []ConsumedItem  `json:"consumed_items"`
	CrystalDropped bool            `json:"crystal_dropped"`
}

type ExtendedEffect string

const (
	ExtendedFrenzy       ExtendedEffect = "frenzy"
	ExtendedAwakening    ExtendedEffect = "awakening"
	ExtendedMagicSword   ExtendedEffect = "magic_sword"
	ExtendedCrown        ExtendedEffect = "crown"
	ExtendedWarGod       ExtendedEffect = "war_god"
	ExtendedFlame        ExtendedEffect = "flame"
	ExtendedBow          ExtendedEffect = "bow"
	ExtendedAdditional   ExtendedEffect = "additional_submission"
	ExtendedIncense      ExtendedEffect = "incense"
	ExtendedPurification ExtendedEffect = "purification"
	ExtendedInterception ExtendedEffect = "interception"
)

// LifetimeTail contains operation facts, never elapsed time or positive damage.
// DamageCalls is recorded at the helper's invocation point, including misses
// and one-hit defense, after its preceding validation gates.
type LifetimeTail struct {
	NPC             bool
	DamageCalls     int
	BowAction       bool
	SecondSubmitted bool // nonempty second input in the seven-entry whitelist
}

// FinishExtended applies a reached tail's lifetime predicates. The caller must
// not call this for rejected commands or a skipped player tail (suppression,
// no enemies, NPC comment, or the preceding level-up branch).
// Evidence: _battle.cgi:938–1172; _npc_action.cgi:466–468.
// Modifier formulas and expiry side effects are separate certified effects.
func (a *LifetimeActor) FinishExtended(f LifetimeTail) bool {
	tick := false
	if f.NPC {
		tick = a.Extended == ExtendedPurification
	} else {
		switch a.Extended {
		case ExtendedFrenzy, ExtendedAwakening:
			tick = true
		case ExtendedMagicSword, ExtendedCrown, ExtendedWarGod, ExtendedFlame:
			tick = f.DamageCalls > 0
		case ExtendedBow:
			tick = f.BowAction
		case ExtendedAdditional:
			tick = f.SecondSubmitted
		}
	}
	if tick {
		a.ExtendedTurns--
	}
	// War-god checks expiry even when no damage helper ran.
	if (tick || (!f.NPC && a.Extended == ExtendedWarGod)) && a.ExtendedTurns <= 0 {
		if a.Extended == ExtendedFrenzy {
			a.Attack = min(999, a.Attack)
		}
		a.Extended = ""
		return true
	}
	return false
}

// TickIncense ticks every holder on a reached player tail, including fallen
// holders, and returns the first holder who supplies the one party heal.
// The heal's target/formula helper remains responsible for its normal gates.
func (s *LifetimeState) TickIncense(partyIDs []string) string {
	first := ""
	for _, id := range partyIDs {
		for i := range s.Actors {
			a := &s.Actors[i]
			if a.ID != id || a.Extended != ExtendedIncense {
				continue
			}
			if first == "" {
				first = id
			}
			a.ExtendedTurns--
			if a.ExtendedTurns <= 0 {
				a.Extended = ""
			}
		}
	}
	return first
}

// CanExecuteSecond uses the raw cost after the first action's charge. Whitelist
// membership must come from the certified catalog, never arbitrary skill kinds.
// _battle.cgi:780–785; counter consumption independently uses SecondSubmitted.
// Capture activeAtSubmission before the first callback can reset/replace effects.
func CanExecuteSecond(activeAtSubmission, firstWhitelisted, secondWhitelisted bool, remainingMP, rawCost int) bool {
	return activeAtSubmission && firstWhitelisted && secondWhitelisted && rawCost >= 0 && remainingMP > rawCost
}

// AdvanceCMP checks the threshold BEFORE incrementing. Supply already resolved
// recharge amount and next threshold; do not persist an RNG in battle state.
// _battle.cgi:1039–1053. A reached player tail can recharge a fallen actor.
func (a *LifetimeActor) AdvanceCMP(amount, nextThreshold int) bool {
	if a.CMPCharge < a.CMPNeedCharge {
		a.CMPCharge++
		return false
	}
	a.CMP = min(a.MaxCMP, a.CMP+amount)
	a.CMPCharge = 0
	a.CMPNeedCharge = nextThreshold
	return true
}

// EnterSealField applies the operation-entry field rule, before effect execution.
func (a *LifetimeActor) EnterSealField() {
	a.CMPNeedCharge = 10
	a.CMPCharge = 0
}

// TickSkillBans is shared room state; only the reached player tail ticks it.
func (s *LifetimeState) TickSkillBans() {
	for skill, turns := range s.SkillBans {
		if turns == 999 {
			continue
		}
		turns--
		if turns <= 0 {
			delete(s.SkillBans, skill)
		} else {
			s.SkillBans[skill] = turns
		}
	}
}

// RecordFirstUse records successful qualifying use per side/color. A failed
// field creation/retrieval must not call it. True means this is the first use;
// its caller applies that callback's suppression, not a universal tail skip.
func (s *LifetimeState) RecordFirstUse(side, skill string) bool {
	if s.SkillHistory[side][skill] {
		return false
	}
	if s.SkillHistory == nil {
		s.SkillHistory = make(map[string]map[string]bool)
	}
	if s.SkillHistory[side] == nil {
		s.SkillHistory[side] = make(map[string]bool)
	}
	s.SkillHistory[side][skill] = true
	return true
}
