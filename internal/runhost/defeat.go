package runhost

import (
	"strings"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/economy"
)

// defeatRewards binds the shared defeat routine (_battle.cgi:28–277) to a
// single command's result. The caller publishes effects only after the
// complete operation succeeds.
func defeatRewards(r *State, room RoomCombat, requester corecharacter.Character, blessing string, rng random.Generator, effects *Effects) *corebattle.DefeatRewards {
	validate := func(s *corebattle.LifetimeState, c corebattle.TargetContext, id string) error {
		actor, target := findActor(s.Actors, c.ActorID), findActor(s.Actors, id)
		if actor == nil || target == nil {
			return ErrConflict
		}
		if actor.NPC || id == c.ActorID {
			return nil
		}
		if requester.ID != c.ActorID || r == nil || rng == nil || effects == nil {
			return ErrUnavailable
		}
		if target.ExperienceReward < 0 || target.GoldReward < 0 || target.CrystalReward < 0 {
			return ErrConflict
		}
		return nil
	}
	stats := requester.Stats
	requesterStrength := battle.Strong(stats.MaxHP, stats.MaxMP, stats.Attack, stats.Defense, stats.Agility)
	wish := strings.ToLower(strings.TrimSpace(blessing))
	return &corebattle.DefeatRewards{
		Validate: validate,
		Project: func(s *corebattle.LifetimeState, _ corebattle.TargetContext, _ string) ([]corebattle.LifetimeState, error) {
			// Reward facts do not alter reachability of subsequent combat effects.
			return []corebattle.LifetimeState{*s}, nil
		},
		Apply: func(s *corebattle.LifetimeState, c corebattle.TargetContext, id string) error {
			if err := validate(s, c, id); err != nil {
				return err
			}
			actor, target := findActor(s.Actors, c.ActorID), findActor(s.Actors, id)
			if actor.NPC || id == c.ActorID || r.HasBox(id) {
				return nil
			}
			strength := battle.Strong(target.MaxHP, target.MaxMP, target.Attack, target.Defense, target.Agility)
			strong := battle.IsStrongEnemy(strength, requesterStrength)
			reward := Earnings{EXP: target.ExperienceReward, Gold: target.GoldReward}
			rolled := target.CrystalDropRolled || target.CrystalDropped
			if !rolled {
				reward.Crystals = target.CrystalReward
				if reward.Crystals == 0 {
					chance := 1
					if strong {
						chance++
					}
					if rng.Intn(50) < chance {
						reward.Crystals = 1
					}
				}
			}
			dropped := reward.Crystals > 0
			var err error
			reward, err = modifyDefeatEarnings(reward, actor.Gear, room.Scale, wish, len(r.Roster), rng)
			if err != nil {
				return err
			}
			if effects.Earnings == nil {
				effects.Earnings = make(map[string]Earnings)
			}
			for _, allyID := range r.Roster {
				ally := findActor(s.Actors, allyID)
				if ally == nil || ally.HP <= 0 || ally.TeamID != actor.TeamID || ally.RosterDetached || ally.Banished {
					continue
				}
				effects.Earnings[allyID], err = AddEarnings(effects.Earnings[allyID], reward)
				if err != nil {
					return err
				}
			}
			target.CrystalDropRolled = true
			target.CrystalDropped = target.CrystalDropped || dropped
			identity := target.EncounterIdentity
			if identity == "" {
				identity = target.ID
			}
			habitat := ""
			if room.Habitat != nil {
				habitat = room.Habitat()
			}
			effects.Defeats = append(effects.Defeats, defeatFact(actor, target, c.DefeatedStatus, identity, habitat, strength, strong, wish, rng))
			if room.Credited != nil {
				room.Credited(identity, rng)
			}
			return nil
		},
	}
}

func modifyDefeatEarnings(e Earnings, gear *corebattle.BattleGearStats, scale func(Earnings) Earnings, wish string, partySize int, rng random.Generator) (Earnings, error) {
	if gear != nil && gear.AccessoryID == "item-167" {
		var err error
		e.EXP, err = economy.SafeAdd(e.EXP, e.EXP/5)
		if err != nil {
			return Earnings{}, err
		}
	}
	if scale != nil {
		e = scale(e)
	}
	bonus := ChapelDefeatBonus(e, wish, rng.Intn)
	if gear != nil && gear.WeaponID == "weapon-01" && gear.ArmorID == "armor-01" && rng.Intn(4) == 0 {
		bonus.EXP += 5
	}
	e, err := AddEarnings(e, bonus)
	if err != nil {
		return Earnings{}, err
	}
	return ScaleCooperativeEarnings(e, min(3, max(0, partySize-1))*10)
}

// ChapelDefeatBonus is a gold or EXP blessing's extra half of a defeat reward.
func ChapelDefeatBonus(e Earnings, blessing string, rollIntn func(int) int) Earnings {
	wish := strings.ToLower(strings.TrimSpace(blessing))
	switch {
	case battle.IsGoldBlessing(wish):
		if rollIntn(4) == 0 {
			return Earnings{Gold: e.Gold / 2}
		}
	case battle.IsExpBlessing(wish):
		if rollIntn(4) == 0 {
			return Earnings{EXP: e.EXP / 2}
		}
	}
	return Earnings{}
}

// ScaleCooperativeEarnings adds the party's cooperative EXP and gold bonus.
func ScaleCooperativeEarnings(e Earnings, bonusPercent int) (Earnings, error) {
	if bonusPercent == 0 {
		return e, nil
	}
	exp, err := economy.SafeMultiply(e.EXP, 100+bonusPercent)
	if err != nil {
		return Earnings{}, err
	}
	gold, err := economy.SafeMultiply(e.Gold, 100+bonusPercent)
	if err != nil {
		return Earnings{}, err
	}
	e.EXP, e.Gold = exp/100, gold/100
	return e, nil
}

func defeatFact(actor, target *corebattle.LifetimeActor, status, identity, habitat string, strength int, strong bool, wish string, rng random.Generator) battle.ExplicitDefeat {
	fact := battle.ExplicitDefeat{Monster: battle.DefeatedMonsterRecord{
		MonsterID: battle.ParseDefeatedMonsterID(strings.TrimRight(identity, "ABCDEFGH")), MonsterName: battle.CleanMonsterName(target.Name),
		Habitat: habitat, Icon: target.Icon, Strong: strength, HP: target.MaxHP, MP: target.MaxMP,
		Attack: target.Attack, Defense: target.Defense, Agility: target.Agility, ExperienceReward: target.ExperienceReward, GoldReward: target.GoldReward,
	}, StrongKill: strong && actor.HP > 0}
	if actor.HP > 0 && target.NPC {
		foodID := ""
		if actor.Gear != nil {
			foodID = actor.Gear.AccessoryID
		}
		if actor.HeldToolID != nil {
			foodID = *actor.HeldToolID
		}
		food := foodID == "item-077" || foodID == "item-77"
		dragonSet := actor.Gear != nil && actor.Gear.WeaponID == "weapon-36" && actor.Gear.ArmorID == "armor-39"
		chance := battle.CalculateTamePar(strong, actor.Jobs.JobID, food, battle.IsMonsterBlessing(wish), dragonSet, status == "capture")
		fact.Capture = rng.Float64()*200 < chance && !strings.HasPrefix(target.Icon, "upload/")
	}
	return fact
}
