package skill

import corebattle "github.com/witchcraze/party2re/internal/core/battle"

// BattleDefinitions contains explicitly certified native entries, independently
// of learning, commerce and composition catalogs. Each production entry needs
// source evidence and tests of its entire effect; a matching kind is not proof.
// The zero value admits no entries. Random/delegated effects remain unsupported
// until their executor and full original descendant pool can be certified.
type BattleDefinitions struct {
	JobSkills map[string]corebattle.ActionSkill
	Gems      map[string]corebattle.GemEffect
}

func (d BattleDefinitions) JobAction(def Definition) (corebattle.ActionSkill, error) {
	action, ok := d.JobSkills[def.ID]
	if !ok {
		return corebattle.ActionSkill{}, corebattle.ErrUnsupportedEffect
	}
	action.ID, action.Name, action.MPCost = def.ID, def.Name, def.MPCost
	if err := corebattle.ValidateSkillAction(action); err != nil {
		return corebattle.ActionSkill{}, err
	}
	return action, nil
}

// CustomAction hydrates occupied slots in order, including repeated gems.
func (d BattleDefinitions) CustomAction(id, name, incantation string, cost int, slots [3]string) (corebattle.ActionCustomSkill, error) {
	action := corebattle.ActionCustomSkill{ID: id, Name: name, Incantation: incantation, CMPCost: cost}
	for _, id := range slots {
		if id == "" {
			continue
		}
		gem, ok := d.Gems[id]
		if !ok {
			return corebattle.ActionCustomSkill{}, corebattle.ErrUnsupportedEffect
		}
		action.Gems = append(action.Gems, gem)
	}
	if err := corebattle.ValidateCustomSkillAction(action); err != nil {
		return corebattle.ActionCustomSkill{}, err
	}
	return action, nil
}
