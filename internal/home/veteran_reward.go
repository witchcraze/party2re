package home

import (
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
)

const (
	veteranRewardName = "歴戦の褒章"
	// veteranRewardEXP is the level-99 EXP that item 237 restores (legacy _data.cgi:1994).
	veteranRewardEXP = 98010
	noEffectMessage  = "しかし、何も起こらなかった…"
)

// homeItemKept reports whether a dispatched callback keeps its item after
// committing its mutations; legacy sets mes after mutating for item 237 at d<=0.
func homeItemKept(def item.Definition, char corecharacter.Character) bool {
	return def.Name == veteranRewardName && char.Level >= progression.MaxLevel && char.Experience <= veteranRewardEXP
}

// applyVeteranReward converts EXP above veteranRewardEXP into gold (legacy
// _data.cgi:1994). A non-positive difference still converts but keeps the item.
func (s *Service) applyVeteranReward(char *corecharacter.Character) (string, error) {
	if char.Level < progression.MaxLevel {
		return noEffectMessage, errNoEffect
	}
	d := char.Experience - veteranRewardEXP
	gain := char.ConvertExperienceToMoney(veteranRewardEXP, 1+s.randomFloat()/2)
	if d <= 0 {
		return noEffectMessage, nil
	}
	return fmt.Sprintf("これまでの戦いの報酬として %d Gもらった!", gain), nil
}
