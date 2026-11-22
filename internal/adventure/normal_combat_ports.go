package adventure

import (
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/core/skill"
)

// runPorts are rebuilt for every operation from the saved run and requester.
// A nil port keeps its entries as whole-action gates: arithmetic discovery,
// control MP drain, transformation area damage, job assets and job encounters
// have no production owner in this composition yet.
type runPorts struct {
	damage corebattle.DamageContext
	defeat *corebattle.DefeatContext
	status *corebattle.StatusContext
}

func newRunPorts(frame corebattle.TargetContext, level *int, seals *corebattle.WeaponSealTable, field string, rewards *corebattle.DefeatRewards, npc bool) runPorts {
	defeat := &corebattle.DefeatContext{
		EquipmentResetContext: corebattle.EquipmentResetContext{TargetContext: frame, RequesterLevel: level, Seals: seals},
		Rewards:               rewards,
		FieldAtLoad:           field,
	}
	effect := corebattle.EffectContext{TargetContext: frame, RequesterLevel: level, ActorNPC: npc}
	return runPorts{
		damage: corebattle.DamageContext{EffectContext: effect},
		defeat: defeat,
		status: &corebattle.StatusContext{EffectContext: effect, Defeat: defeat},
	}
}

func (p runPorts) callbacks(r *NormalRun, state *corebattle.LifetimeState, requester *corecharacter.Character, inspection map[string]corebattle.InspectionContent) skill.PlayerCallbackContexts {
	d := p.damage
	actor := findActor(state.Actors, d.ActorID)
	held, icon := "", ""
	if actor != nil {
		icon = actor.Icon
		if actor.HeldToolID != nil {
			held = *actor.HeldToolID
		}
	}
	gender := ""
	if requester != nil {
		gender = requester.Gender
	}
	enemyTeam := ""
	for _, id := range slices.Concat(d.EnemyIDs, d.MemberOnlyIDs) {
		if a := findActor(state.Actors, id); a != nil && (actor == nil || a.TeamID != actor.TeamID) {
			enemyTeam = a.TeamID
			break
		}
	}
	prior := make(map[string]corebattle.TemporaryEffect, len(state.Actors))
	for _, a := range state.Actors {
		prior[a.ID] = a.Temporary
	}
	burn := actor != nil && requester != nil && actor.LateBreathSongBurn(state.Fields.Current(), requester.Gender)
	reset := &p.defeat.EquipmentResetContext
	roster := &corebattle.RosterContext{
		DamageContext: d, RequesterItemID: held, EnemyTeamID: enemyTeam,
		EnemyPool: r.EncounterPool, AppearancePositions: r.AppearancePositions,
		Hard: r.Mode == CrawlModeHard, Death: p.defeat,
	}
	resources := d
	return skill.PlayerCallbackContexts{
		BaseGuard:           &corebattle.BaseGuardContext{DamageContext: d, Status: p.status},
		Martial:             &corebattle.MartialContext{DamageContext: d, Status: p.status},
		BasicMagic:          &corebattle.BasicMagicContext{DamageContext: d},
		Incidental:          &corebattle.IncidentalJobContext{DamageContext: d, Status: *p.status},
		BeastClone:          &corebattle.BeastCloneContext{DamageContext: d, RequesterItemID: held, StatusDefeat: p.defeat},
		Ranged:              &corebattle.RangedContext{DamageContext: d, Status: *p.status},
		SongDanceDrain:      &corebattle.SongDanceDrainContext{DamageContext: d, Status: p.status},
		AdvancedMagic:       &corebattle.AdvancedMagicContext{DamageContext: d, Status: *p.status},
		GuardianHealing:     &corebattle.GuardianHealingContext{DamageContext: d},
		RecoilDarkState:     &corebattle.RecoilDarkStateContext{DamageContext: d, RawDefeat: p.defeat},
		WeaponWeatherStance: &corebattle.WeaponWeatherStanceContext{DamageContext: d, StatusDefeat: p.defeat},
		TimeMixedMagic:      &corebattle.TimeMixedMagicContext{DamageContext: d},
		ProtectiveMagic:     &corebattle.ProtectiveMagicContext{DamageContext: d},
		CounterReset:        &corebattle.CounterResetContext{DamageContext: d, StatusDefeat: p.defeat},
		HybridBreath:        &corebattle.HybridBreathContext{DamageContext: d},
		TreatmentChance:     &corebattle.TreatmentChanceContext{DamageContext: d, Status: *p.status},
		HighPower:           &corebattle.HighPowerContext{DamageContext: d, EquipmentReset: reset},
		TreatmentMischief:   &corebattle.TreatmentMischiefContext{DamageContext: d, Status: *p.status},
		ControlClimax:       &corebattle.ControlClimaxContext{DamageContext: d, PriorTemporary: prior},
		Roster:              roster,
		DuelCrown:           &corebattle.DuelCrownContext{DamageContext: d},
		RitualDance:         &corebattle.RitualDanceContext{DamageContext: d},
		DualAgility:         &corebattle.DualAgilityContext{DamageContext: d, Status: p.status},
		Transformations:     &corebattle.TransformationContext{EffectContext: d.EffectContext, RequesterIcon: icon, RequesterGender: gender},
		MachineFamiliar:     &corebattle.MachineFamiliarContext{DamageContext: d, RequesterGender: gender},
		LateBreathSong:      &corebattle.LateBreathSongContext{DamageContext: d, BurnStrike: burn, Death: p.status},
		Resources:           &resources,
		Revival:             &corebattle.RevivalContext{TargetContext: d.TargetContext, NativeDefeat: p.defeat},
		Status:              p.status,
		Reset:               reset,
		Fields:              &corebattle.NamedFieldContext{DamageContext: d, Death: p.defeat, Roster: roster, RequesterGender: gender, RequesterIcon: icon},
		Tools:               &corebattle.OrdinaryToolContext{DamageContext: d, OrdinaryToolInput: corebattle.OrdinaryToolInput{Inspection: inspection}, EquipmentReset: reset, Death: p.defeat},
	}
}

// bindRunAction supplies Validate/Execute for parsed entries that parsing
// admitted but left unbound: the normal attack, the custom command and
// ordinary registered invocations.
func bindRunAction(action *ParsedRunAction, frame corebattle.CallbackContext, defs skill.BattleDefinitions) {
	if action.Validate != nil {
		return
	}
	frame.Arguments = slices.Clone(action.Arguments)
	frame.Command, frame.Custom = action.EntryID, action.Custom
	switch {
	case action.Custom:
		action.Validate = func(s *corebattle.LifetimeState) error {
			work := frame
			_, _, err := defs.PlayerCustomAction(s, &work, nil, random.Default())
			return err
		}
		action.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
			return runFacts(frame, func(work *corebattle.CallbackContext) error {
				return defs.ApplyPlayerCustomAction(s, work, nil, rng)
			})
		}
	case action.Action == "attack":
		target := action.TargetID
		action.Validate = func(s *corebattle.LifetimeState) error {
			damage := *frame.Damage
			damage.TargetContext = frame.TargetContext
			return s.ValidateNormalAttack(&damage, target, random.Default())
		}
		action.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
			return runFacts(frame, func(work *corebattle.CallbackContext) error {
				work.Damage.TargetContext = work.TargetContext
				_, err := s.ApplyNormalAttack(work.Damage, target, rng)
				work.TargetContext = work.Damage.TargetContext
				return err
			})
		}
	default:
		bindOrdinaryPlayerAction(action, frame, defs.Invocations)
	}
}

// reportFieldSuppression returns the field port's `$npc_turn_skip` (cosmic
// first use) as the submission's fact; the port itself is not the tail input.
func reportFieldSuppression(action *corebattle.PlayerAction, field *corebattle.NamedFieldContext) {
	execute := action.Execute
	if execute == nil {
		return
	}
	action.Execute = func(s *corebattle.LifetimeState, rng random.Generator) (corebattle.PlayerActionFacts, error) {
		facts, err := execute(s, rng)
		facts.Suppressed = facts.Suppressed || field.ResponseSuppressed
		return facts, err
	}
}

// runFacts executes on a fresh damage copy and reports its counters and any
// side arrays a roster callback rebuilt.
func runFacts(frame corebattle.CallbackContext, apply func(*corebattle.CallbackContext) error) (corebattle.PlayerActionFacts, error) {
	work := frame
	var base corebattle.DamageContext
	if frame.Damage != nil {
		base = *frame.Damage
	}
	damage := base
	work.Damage = &damage
	if err := apply(&work); err != nil {
		return corebattle.PlayerActionFacts{}, err
	}
	facts := corebattle.PlayerActionFacts{
		DamageCalls: damage.DamageCalls - base.DamageCalls,
		HealCalls:   damage.HealCalls - base.HealCalls,
		End:         damage.End,
	}
	if !slices.Equal(work.EnemyIDs, frame.EnemyIDs) || !slices.Equal(work.PartyIDs, frame.PartyIDs) || !slices.Equal(work.MemberOnlyIDs, frame.MemberOnlyIDs) {
		facts.Target = work.TargetContext
	}
	return facts, nil
}
