package skill

import (
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
)

// RegisterNPCCallbacks adds the ordinary NPC-only tables to the shared job and
// gem definitions. Supply fresh shared contexts for this NPC response.
func (d *BattleDefinitions) RegisterNPCCallbacks(shared PlayerCallbackContexts, frame *corebattle.CallbackContext) {
	damage, death := *frame.Damage, frame.Defeat
	var reset *corebattle.EquipmentResetContext
	if death != nil {
		reset = &death.EquipmentResetContext
	}
	if shared.Roster != nil {
		roster := *shared.Roster
		roster.DamageContext, roster.Death = damage, death
		shared.Roster = &roster
	}
	d.RegisterPlayerCallbacks(shared, len(frame.PartyIDs))
	d.RegisterNPCJobs(shared.Roster)
	d.RegisterNPCStatus(&corebattle.NPCStatusContext{DamageContext: damage, Death: death})
	d.RegisterNPCJob97(&corebattle.NPCJob97Context{DamageContext: damage, EquipmentReset: reset})
	d.RegisterNPCJob98(&corebattle.NPCJob98Context{DamageContext: damage, EquipmentReset: reset})
	d.RegisterNPCJob402(&corebattle.NPCJob402Context{DamageContext: damage, Death: death})
	d.RegisterNPCOrderedReset(&corebattle.NPCOrderedResetContext{DamageContext: damage, EquipmentReset: reset})
	d.RegisterDamageGems(frame.Damage)
	d.RegisterRecoveryStatGems(&frame.Damage.EffectContext)
	d.RegisterDefenseGems()
	d.RegisterSummonAppearanceGems(shared.Roster, shared.Transformations)
	d.RegisterNonJobInvocations()
}

// ApplyNPCOperation binds fresh definitions for the selected NPC. Use the mode
// seed for a new request and the returned seed for another response in it.
// register builds ports from the working frame without mutating caller inputs.
func ApplyNPCOperation(s *corebattle.LifetimeState, op corebattle.NPCOperation, seed []corebattle.CallbackBinding, register func(*BattleDefinitions, *corebattle.CallbackContext), rng random.Generator) (corebattle.NPCOperationResult, error) {
	if register == nil {
		return corebattle.NPCOperationResult{}, corebattle.ErrInvalidRequest
	}
	return s.ApplyNPCOperation(op, seed, func(s *corebattle.LifetimeState, frame *corebattle.CallbackContext) (corebattle.NPCActionPlan, error) {
		var d BattleDefinitions
		register(&d, frame)
		return d.npcChoicePool(s, frame, seed)
	}, rng)
}
