package battle

import (
	"context"
	"errors"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// ExplicitDefeat carries the reached reward facts into the command settler.
type ExplicitDefeat struct {
	Monster    DefeatedMonsterRecord
	StrongKill bool
	PvP        bool
	Capture    bool
}

func applyExplicitKillCounts(char *corecharacter.Character, req ApplyPostBattleRequest) {
	if char.ID != req.RecipientCharacterID {
		return
	}
	for _, defeat := range req.ExplicitDefeats {
		if !defeat.StrongKill {
			continue
		}
		if defeat.PvP {
			char.PvPWins++
		} else {
			char.MonsterKills++
		}
	}
}

func (s *Service) deliverExplicitCaptures(ctx context.Context, char corecharacter.Character, req ApplyPostBattleRequest, resp *ApplyPostBattleResponse) error {
	if char.ID != req.RecipientCharacterID {
		return nil
	}
	for _, defeat := range req.ExplicitDefeats {
		if !defeat.Capture || defeat.PvP {
			continue
		}
		if s.monsterTamer == nil {
			return ErrMonsterTamerNotConfigured
		}
		m := defeat.Monster
		err := s.monsterTamer.TameMonster(ctx, char.ID, m.MonsterID, m.MonsterName)
		full := errors.Is(err, ErrMonsterBoxFull)
		if err != nil && !full {
			return err
		}
		if !full && s.monsterRecorder != nil {
			if err := s.monsterRecorder.RecordMonsterDefeat(ctx, char.ID, m); err != nil {
				return err
			}
		}
		resp.MonsterTames[char.ID] = append(resp.MonsterTames[char.ID], MonsterTameResult{
			MonsterID: m.MonsterID, MonsterName: m.MonsterName, Success: !full, BoxFull: full, TamerName: char.Name,
		})
	}
	return nil
}
