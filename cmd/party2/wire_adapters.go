package main

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/boss"
	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/contest"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/tavern"
)

func buildAdventureVictoryHook(record func(context.Context, string, medal.MetricType, int) error) adventure.VictoryHook {
	return func(ctx context.Context, characterID string, monstersDefeated, goldEarned int) error {
		err := record(ctx, characterID, medal.MetricAdventureVictories, 1)
		if monstersDefeated > 0 {
			err = errors.Join(err, record(ctx, characterID, medal.MetricMonstersSlain, monstersDefeated))
		}
		if goldEarned > 0 {
			err = errors.Join(err, record(ctx, characterID, medal.MetricGoldEarned, goldEarned))
		}
		return err
	}
}

func buildPartyVictoryHook(record func(context.Context, string, medal.MetricType, int) error) party.VictoryHook {
	victory := buildAdventureVictoryHook(record)
	return func(ctx context.Context, characterIDs []string, monstersDefeated, goldEarned int) error {
		var err error
		for _, id := range characterIDs {
			err = errors.Join(err, victory(ctx, id, monstersDefeated, goldEarned))
		}
		return err
	}
}

type victoryPlaza interface {
	RecordVictoryCrowd(context.Context) error
}

func buildVictoryBanquetHook(plaza victoryPlaza) boss.VictoryBanquetHook {
	return func(ctx context.Context, _, _, _, _ string, _ int) error {
		// An inner RunInTx may have joined an outer transaction that can still roll back.
		if database.TxFromContext(ctx) != nil {
			return errors.New("victory banquet requires a committed settlement")
		}
		return plaza.RecordVictoryCrowd(ctx)
	}
}

type postAdventureTavern interface {
	ResetFullness(ctx context.Context, characterID string) error
	ClaimDelivery(ctx context.Context, characterID string) (tavern.OrderResult, error)
}

func buildPostAdventureHook(t postAdventureTavern) func(ctx context.Context, characterID string) (string, error) {
	return func(ctx context.Context, characterID string) (string, error) {
		if err := t.ResetFullness(ctx, characterID); err != nil {
			return "", err
		}
		result, err := t.ClaimDelivery(ctx, characterID)
		if errors.Is(err, tavern.ErrNoActiveDelivery) {
			return "", nil
		}
		if errors.Is(err, tavern.ErrInsufficientFunds) {
			return "酒場から出前が届きましたが、お金が足りませんでした", nil
		}
		if err != nil {
			return "", err
		}
		return result.Message, nil
	}
}

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

type homePetAdapter struct {
	monsterRepo    *database.MonsterRepository
	homeMemberRepo *database.HomeMemberRepository
}

func (a *homePetAdapter) ListHomePets(ctx context.Context, characterID string) ([]home.HomePet, error) {
	var res []home.HomePet

	if a.monsterRepo != nil {
		monsters, err := a.monsterRepo.ListByCharacterIDAndLocation(ctx, characterID, monster.LocationHome)
		if err != nil {
			return nil, err
		}
		for _, m := range monsters {
			res = append(res, home.HomePet{
				ID:         m.ID,
				CustomName: m.CustomName,
				MonsterID:  m.MonsterID,
			})
		}
	}

	if a.homeMemberRepo != nil {
		members, err := a.homeMemberRepo.FindByCharacterID(ctx, characterID)
		if err != nil {
			return nil, err
		}
		for _, m := range members {
			if !m.IsNPC {
				continue
			}
			res = append(res, home.HomePet{
				ID:         m.ID,
				CustomName: m.Name,
				MonsterID:  m.Icon,
			})
		}
	}

	return res, nil
}
