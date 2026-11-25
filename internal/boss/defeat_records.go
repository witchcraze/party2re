package boss

import (
	"context"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// recordBossDefeats saves the mandatory Monster Book / Hall of Fame record of
// every roster enemy for a victorious character. Any failure is returned so the
// enclosing settlement rolls back; only news and celebration remain best-effort.
func (s *Service) recordBossDefeats(ctx context.Context, stageID, characterID string, roster []corebattle.LifetimeActor) error {
	if s.monsterRecorder == nil {
		return nil
	}
	for i, e := range roster {
		rec := DefeatedMonsterRecord{
			MonsterID: parseBossMonsterID(e.Icon, fmt.Sprintf("%s-boss-%d", stageID, i)), MonsterName: e.Name, Habitat: "封印戦", Icon: e.Icon,
			Strong: int(float64(e.MaxHP) + float64(e.MaxMP) + float64(e.Attack) + float64(e.Defense)*0.5 + float64(e.Agility)),
			HP:     e.MaxHP, MP: e.MaxMP, Attack: e.Attack, Defense: e.Defense, Agility: e.Agility,
			ExperienceReward: e.ExperienceReward, GoldReward: e.GoldReward,
		}
		if stageID == cloneStageID {
			rec.MonsterID, rec.MonsterName, rec.Icon = "king99-clone", "影", "king99-clone"
		}
		if err := s.monsterRecorder.RecordMonsterDefeat(ctx, characterID, rec); err != nil {
			return fmt.Errorf("record monster defeat for %s: %w", characterID, err)
		}
	}
	return nil
}
