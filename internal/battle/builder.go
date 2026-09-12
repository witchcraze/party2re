package battle

import (
	"context"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/core/skill"
	"github.com/witchcraze/party2re/internal/custom_skill"
)

// Legacy item IDs with passive or active combat mechanics.
const (
	ItemToukiShield    = "item-161" // 闘気の盾 (UsageCategory 3, revive)
	ItemDokuroAmulet   = "item-193" // ドクロのお守り (UsageCategory 3, revive)
	ItemCursedTalisman = "item-260" // 転生の呪魂符 (UsageCategory 3, cursed revive)
	ItemPrayerRing     = "item-012" // 祈りの指輪 (UsageCategory 1, accessory + in-battle MP heal)
	ItemSkillOrb       = "item-157" // スキルの宝珠 (UsageCategory 3, 25% SP bonus on level-up)
)

// BuildParticipant loads a character and their inventory/equipment to construct a battle Participant.
func (s *Service) BuildParticipant(ctx context.Context, characterID string) (corebattle.Participant, error) {
	charID := strings.TrimSpace(characterID)
	if charID == "" {
		return corebattle.Participant{}, ErrCharacterNotFound
	}
	if s.charRepo == nil {
		return corebattle.Participant{}, ErrCharacterNotFound
	}

	char, err := s.charRepo.FindByID(ctx, charID)
	if err != nil {
		return corebattle.Participant{}, ErrCharacterNotFound
	}

	var inv coreinventory.Inventory
	if s.invRepo != nil {
		if loadedInv, err := s.invRepo.FindByCharacterID(ctx, charID); err == nil {
			inv = loadedInv
		}
	}

	var equip coreequipment.Equipment
	if s.equipRepo != nil {
		if loadedEquip, err := s.equipRepo.FindByCharacterID(ctx, charID); err == nil {
			equip = loadedEquip
		}
	}

	var skills []skill.Definition
	if s.skillProvider != nil && char.JobID != "" {
		skills = s.skillProvider.SkillsForJob(char.JobID)
	}

	var cs *custom_skill.CustomSkill
	if s.customSkills != nil {
		if custom, err := s.customSkills.FindCustomSkill(ctx, charID); err == nil {
			cs = custom
		}
	}

	return BuildParticipantFromData(char, inv, equip, skills, cs)
}

// BuildParticipantFromData constructs a Participant directly from domain structs.
func BuildParticipantFromData(
	char corecharacter.Character,
	inv coreinventory.Inventory,
	equip coreequipment.Equipment,
	skills []skill.Definition,
	cs *custom_skill.CustomSkill,
) (corebattle.Participant, error) {
	builder := corebattle.NewParticipantBuilder(char.ID).
		WithName(char.Name).
		FromCharacter(char)

	// Collect item definition IDs and find passive abilities & active combat items
	itemDefIDs := make([]string, 0, len(inv.Items)+len(equip.Slots))
	hasToukiShield := false
	hasDokuroAmulet := false
	hasCursedTalisman := false

	// Scan equipment slots
	for _, instID := range equip.Slots {
		if inst, found := inv.Find(instID); found {
			itemDefIDs = append(itemDefIDs, inst.DefinitionID)
			switch inst.DefinitionID {
			case ItemToukiShield:
				hasToukiShield = true
			case ItemDokuroAmulet:
				hasDokuroAmulet = true
			case ItemCursedTalisman:
				hasCursedTalisman = true
			case ItemPrayerRing:
				// Equipped combat tool (Category 1) available for use
				builder.WithActionItems(corebattle.ActionItem{
					ID:            ItemPrayerRing,
					InstanceID:    inst.ID,
					Name:          "祈りの指輪",
					UsageCategory: item.UsageCategoryCombatOnly,
					Kind:          corebattle.ActionKindHeal,
					Power:         100,
					TargetScope:   corebattle.TargetScopeSingleAlly,
				})
			}
		}
	}

	// Scan inventory items
	for _, inst := range inv.Items {
		itemDefIDs = append(itemDefIDs, inst.DefinitionID)
		switch inst.DefinitionID {
		case ItemToukiShield:
			hasToukiShield = true
		case ItemDokuroAmulet:
			hasDokuroAmulet = true
		case ItemCursedTalisman:
			hasCursedTalisman = true
		}

		// Actionable combat items (@どうぐ)
		if actItem, ok := actionItemFromDefinition(inst.DefinitionID, inst.ID); ok {
			// Do not re-add prayer ring if already bound from equipment
			if inst.DefinitionID != ItemPrayerRing || !isEquipped(equip, inst.ID) {
				builder.WithActionItems(actItem)
			}
		}
	}

	// Bind passive abilities
	var abilities []string
	if hasToukiShield {
		abilities = append(abilities, "touki_shield")
	}
	if hasDokuroAmulet {
		abilities = append(abilities, "dokuro_amulet")
	}
	if hasCursedTalisman {
		abilities = append(abilities, "cursed_revive")
	}
	if strings.Contains(strings.ToLower(char.JobID), "pharaoh") {
		abilities = append(abilities, "pharaoh")
	}
	if len(abilities) > 0 {
		builder.WithAbilities(abilities...)
	}

	// Bind job skills unlocked by SP threshold
	for _, sk := range skills {
		if char.SP >= sk.RequiredSP {
			builder.WithSkills(corebattle.ActionSkill{
				ID:          sk.ID,
				Name:        sk.Name,
				MPCost:      sk.MPCost,
				Power:       sk.Effect.Power,
				Kind:        actionKindFromEffect(sk.Effect.Kind),
				TargetScope: corebattle.TargetScopeSingleEnemy,
			})
		}
	}

	// Bind custom skill if available
	if cs != nil && cs.Name != "" {
		builder.WithCustomSkills(corebattle.ActionCustomSkill{
			ID:          "custom-skill-" + char.ID,
			Name:        cs.Name,
			Incantation: cs.Comment,
			CMPCost:     cs.CMP,
		})
	}

	builder.WithItems(itemDefIDs...)
	return builder.Build()
}

// BuildPartyBattleRequest constructs a PartyBattleRequest with provided combatants and rewards.
func BuildPartyBattleRequest(
	allies []corebattle.Participant,
	enemies []corebattle.Participant,
	victoryReward corebattle.Reward,
	defeatReward corebattle.Reward,
	drawReward corebattle.Reward,
	field *corebattle.FieldState,
) corebattle.PartyBattleRequest {
	return corebattle.PartyBattleRequest{
		Allies:        allies,
		Enemies:       enemies,
		InitialField:  field,
		VictoryReward: victoryReward,
		DefeatReward:  defeatReward,
		DrawReward:    drawReward,
	}
}

// ExtractStatOrbOptions inspects the character's inventory and builds ApplyExperienceOptions
// binding Stat Orb items (item-152 to item-156) and Skill Orb (item-157).
func ExtractStatOrbOptions(inv coreinventory.Inventory) progression.ApplyExperienceOptions {
	opts := progression.ApplyExperienceOptions{
		StatOrbItems: make(map[string]bool),
	}
	for _, inst := range inv.Items {
		switch inst.DefinitionID {
		case progression.ItemLifeStatOrb:
			opts.StatOrbItems[progression.ItemLifeStatOrb] = true
		case progression.ItemMagicStatOrb:
			opts.StatOrbItems[progression.ItemMagicStatOrb] = true
		case progression.ItemPowerStatOrb:
			opts.StatOrbItems[progression.ItemPowerStatOrb] = true
		case progression.ItemDefenseStatOrb:
			opts.StatOrbItems[progression.ItemDefenseStatOrb] = true
		case progression.ItemAgilityStatOrb:
			opts.StatOrbItems[progression.ItemAgilityStatOrb] = true
		case ItemSkillOrb:
			opts.HasSkillOrb = true
		}
	}
	return opts
}

func isEquipped(equip coreequipment.Equipment, instanceID string) bool {
	for _, id := range equip.Slots {
		if id == instanceID {
			return true
		}
	}
	return false
}

func actionKindFromEffect(kind string) string {
	switch kind {
	case "heal":
		return corebattle.ActionKindHeal
	case "buff":
		return corebattle.ActionKindBuff
	case "status":
		return corebattle.ActionKindStatus
	case "defend":
		return corebattle.ActionKindDefend
	default:
		return corebattle.ActionKindAttack
	}
}

// actionItemFromDefinition maps known combat consumables to ActionItem.
func actionItemFromDefinition(defID, instanceID string) (corebattle.ActionItem, bool) {
	switch defID {
	case "item-001": // 薬草
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "薬草",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 40, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-002": // 上薬草
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "上薬草",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 100, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-003": // 特薬草
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "特薬草",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 250, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-004": // 賢者の石
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "賢者の石",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 200, TargetScope: corebattle.TargetScopeAllAllies,
		}, true
	case "item-005": // 世界樹のしずく
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "世界樹のしずく",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 999, TargetScope: corebattle.TargetScopeAllAllies,
		}, true
	case "item-011": // 魔法の聖水
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "魔法の聖水",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 40, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-012": // 祈りの指輪
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "祈りの指輪",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 100, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-013": // エルフの飲み薬
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "エルフの飲み薬",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindHeal, Power: 999, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	case "item-014": // 守りの石
		return corebattle.ActionItem{
			ID: defID, InstanceID: instanceID, Name: "守りの石",
			UsageCategory: item.UsageCategoryCombatOnly, Kind: corebattle.ActionKindBuff, BuffStat: "defense", Power: 30, TargetScope: corebattle.TargetScopeSingleAlly,
		}, true
	default:
		return corebattle.ActionItem{}, false
	}
}
