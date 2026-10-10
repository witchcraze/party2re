package main

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/guild"
)

type contestGuildAdapter struct {
	guild *guild.Service
}

func (a contestGuildAdapter) GuildOf(ctx context.Context, characterID string) (contest.GuildRef, bool, error) {
	if a.guild == nil {
		return contest.GuildRef{}, false, nil
	}
	g, _, err := a.guild.GetByCharacter(ctx, characterID)
	if err != nil {
		if errors.Is(err, guild.ErrCharacterNotInGuild) {
			return contest.GuildRef{}, false, nil
		}
		return contest.GuildRef{}, false, err
	}
	return contest.GuildRef{
		ID:   g.ID,
		Name: g.Name,
	}, true, nil
}

func (a contestGuildAdapter) AddPoints(ctx context.Context, guildID string, points int64) error {
	if a.guild == nil {
		return nil
	}
	return a.guild.AddPoints(ctx, guildID, points)
}

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
