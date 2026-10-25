package boss

import (
	"context"
	"fmt"
)

// recordBossDefeats saves the mandatory Monster Book / Hall of Fame record for a
// victorious character. Any failure is returned so the enclosing settlement rolls back;
// only news and celebration remain best-effort.
func (s *Service) recordBossDefeats(ctx context.Context, stage BossStage, characterID string, cloneRecords map[string]DefeatedMonsterRecord) error {
	if s.monsterRecorder == nil {
		return nil
	}
	if stage.ID == "king99" {
		rec, ok := cloneRecords[characterID]
		if !ok {
			return nil
		}
		if err := s.monsterRecorder.RecordMonsterDefeat(ctx, characterID, rec); err != nil {
			return fmt.Errorf("record monster defeat for %s: %w", characterID, err)
		}
		return nil
	}
	for i, b := range stage.Bosses {
		monID := parseBossMonsterID(b.Icon, fmt.Sprintf("%s-boss-%d", stage.ID, i))
		hp := b.MaxHP
		if hp <= 0 {
			hp = b.HP
		}
		mp := b.MaxMP
		if mp <= 0 {
			mp = b.MP
		}
		strong := int(float64(hp) + float64(mp) + float64(b.Attack) + float64(b.Defense)*0.5 + float64(b.Agility))
		rec := DefeatedMonsterRecord{
			MonsterID:        monID,
			MonsterName:      b.Name,
			Habitat:          "封印戦",
			Icon:             b.Icon,
			Strong:           strong,
			HP:               hp,
			MP:               mp,
			Attack:           b.Attack,
			Defense:          b.Defense,
			Agility:          b.Agility,
			ExperienceReward: b.GetExp,
			GoldReward:       b.GetMoney,
		}
		if err := s.monsterRecorder.RecordMonsterDefeat(ctx, characterID, rec); err != nil {
			return fmt.Errorf("record monster defeat for %s: %w", characterID, err)
		}
	}
	return nil
}
