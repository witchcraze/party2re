package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// PlayerCallbackContexts supplies the existing operation owners. Rebuild these
// ports after reload; missing ports still reject through whole-action admission.
type PlayerCallbackContexts struct {
	BaseGuard           *corebattle.BaseGuardContext
	Martial             *corebattle.MartialContext
	BasicMagic          *corebattle.BasicMagicContext
	Incidental          *corebattle.IncidentalJobContext
	BeastClone          *corebattle.BeastCloneContext
	Ranged              *corebattle.RangedContext
	SongDanceDrain      *corebattle.SongDanceDrainContext
	AdvancedMagic       *corebattle.AdvancedMagicContext
	GuardianHealing     *corebattle.GuardianHealingContext
	RecoilDarkState     *corebattle.RecoilDarkStateContext
	WeaponWeatherStance *corebattle.WeaponWeatherStanceContext
	TimeMixedMagic      *corebattle.TimeMixedMagicContext
	Arithmetic          *corebattle.ArithmeticContext
	ProtectiveMagic     *corebattle.ProtectiveMagicContext
	CounterReset        *corebattle.CounterResetContext
	HybridBreath        *corebattle.HybridBreathContext
	TreatmentChance     *corebattle.TreatmentChanceContext
	HighPower           *corebattle.HighPowerContext
	TreatmentMischief   *corebattle.TreatmentMischiefContext
	ControlClimax       *corebattle.ControlClimaxContext
	Roster              *corebattle.RosterContext
	DuelCrown           *corebattle.DuelCrownContext
	RitualDance         *corebattle.RitualDanceContext
	DualAgility         *corebattle.DualAgilityContext
	Transformations     *corebattle.TransformationContext
	MachineFamiliar     *corebattle.MachineFamiliarContext
	LateBreathSong      *corebattle.LateBreathSongContext
	Resources           *corebattle.DamageContext
	Revival             *corebattle.RevivalContext
	Status              *corebattle.StatusContext
	Reset               *corebattle.EquipmentResetContext
	Fields              *corebattle.NamedFieldContext
	Tools               *corebattle.OrdinaryToolContext
}

// RegisterPlayerCallbacks assembles all 608 source positions, retaining missing
// asset descendants as gates, plus the two ordinary job73 gear bindings.
// partySize is a frozen table-construction fact for76.5. This does not wire a
// service or replace the representative scalar SkillProvider.
func (d *BattleDefinitions) RegisterPlayerCallbacks(c PlayerCallbackContexts, partySize int) {
	d.RegisterBaseGuard(c.BaseGuard)
	d.RegisterMartial(c.Martial)
	d.RegisterBasicMagic(c.BasicMagic)
	for _, entry := range RepresentativeDefinitions() {
		d.Invocations.Entries[entry.ID] = basicMagicInvocation(c.BasicMagic, entry.ID)
	}
	d.RegisterIncidentalJobs(c.Incidental)
	d.RegisterBeastClone(c.BeastClone)
	d.RegisterRanged(c.Ranged)
	d.RegisterSongDanceDrain(c.SongDanceDrain)
	d.RegisterAdvancedMagic(c.AdvancedMagic)
	d.RegisterGuardianHealing(c.GuardianHealing)
	d.RegisterRecoilDarkState(c.RecoilDarkState)
	d.RegisterWeaponWeatherStance(c.WeaponWeatherStance)
	d.RegisterTimeMixedMagic(c.TimeMixedMagic)
	d.RegisterArithmetic(c.Arithmetic)
	d.RegisterProtectiveMagic(c.ProtectiveMagic)
	d.RegisterCounterReset(c.CounterReset)
	d.RegisterHybridBreath(c.HybridBreath)
	d.RegisterTreatmentChance(c.TreatmentChance)
	d.RegisterHighPower(c.HighPower)
	d.RegisterTreatmentMischief(c.TreatmentMischief)
	d.RegisterControlClimax(c.ControlClimax)
	d.RegisterCallingJobs(c.Roster)
	d.RegisterDuelCrown(c.DuelCrown)
	d.RegisterRitualDance(c.RitualDance, partySize)
	d.RegisterDualAgility(c.DualAgility)
	d.RegisterMachineFamiliar(c.MachineFamiliar)
	d.RegisterTransformations(c.Transformations)
	d.RegisterLateBreathSong(c.LateBreathSong)
	d.RegisterResources(c.Resources)
	d.RegisterRevivals(c.Revival)
	d.RegisterStatuses(c.Status, c.Reset)
	d.RegisterNamedFields(c.Fields)
	for _, entry := range corebattle.InspectionEntries() {
		d.Invocations.Entries[entry.ID] = ordinaryToolInvocation(c.Tools, entry.ID, false)
	}
	for _, id := range []string{"job-55-entry-6", "job-56-entry-5"} {
		d.Invocations.Entries[id] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "random"}}
	}
	d.Invocations.Entries["job-82-entry-0"] = corebattle.CallbackDefinition{Invocation: corebattle.CallbackInvocation{Kind: "job-82-generated"}}
	for _, previous := range []bool{false, true} {
		id := "job-73-gear-current"
		if previous {
			id = "job-73-gear-previous"
		}
		d.Invocations.Entries[id] = corebattle.ShiftGearCallback(previous)
	}
}
