package main

import (
	"context"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/collection"
)

type battleMonsterDefeatAdapter struct {
	svc *collection.Service
}

func (a battleMonsterDefeatAdapter) RecordMonsterDefeat(ctx context.Context, characterID string, record battle.DefeatedMonsterRecord) error {
	if a.svc == nil {
		return nil
	}
	return a.svc.RecordMonsterDefeat(ctx, characterID, collection.DefeatedMonsterRecord{
		MonsterID:        record.MonsterID,
		MonsterName:      record.MonsterName,
		Habitat:          record.Habitat,
		Icon:             record.Icon,
		Strong:           record.Strong,
		HP:               record.HP,
		MP:               record.MP,
		Attack:           record.Attack,
		Defense:          record.Defense,
		Agility:          record.Agility,
		ExperienceReward: record.ExperienceReward,
		GoldReward:       record.GoldReward,
	})
}

type bossMonsterDefeatAdapter struct {
	svc *collection.Service
}

func (a bossMonsterDefeatAdapter) RecordMonsterDefeat(ctx context.Context, characterID string, record boss.DefeatedMonsterRecord) error {
	if a.svc == nil {
		return nil
	}
	return a.svc.RecordMonsterDefeat(ctx, characterID, collection.DefeatedMonsterRecord{
		MonsterID:        record.MonsterID,
		MonsterName:      record.MonsterName,
		Habitat:          record.Habitat,
		Icon:             record.Icon,
		Strong:           record.Strong,
		HP:               record.HP,
		MP:               record.MP,
		Attack:           record.Attack,
		Defense:          record.Defense,
		Agility:          record.Agility,
		ExperienceReward: record.ExperienceReward,
		GoldReward:       record.GoldReward,
	})
}
