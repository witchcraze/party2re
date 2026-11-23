package adventure

import (
	"strings"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// NewRunDefeatRewards binds the defeat port to a single command's result. The
// caller publishes effects only after the complete operation succeeds.
func NewRunDefeatRewards(r *NormalRun, requester corecharacter.Character, blessing string, pvp bool, rng random.Generator, effects *RunActionEffects) *corebattle.DefeatRewards {
	find := func(s *corebattle.LifetimeState, id string) *corebattle.LifetimeActor {
		for i := range s.Actors {
			if s.Actors[i].ID == id {
				return &s.Actors[i]
			}
		}
		return nil
	}
	validate := func(s *corebattle.LifetimeState, c corebattle.TargetContext, id string) error {
		actor, target := find(s, c.ActorID), find(s, id)
		if actor == nil || target == nil {
			return runhost.ErrConflict
		}
		if actor.NPC || id == c.ActorID {
			return nil
		}
		if requester.ID != c.ActorID || r == nil || rng == nil || effects == nil {
			return runhost.ErrUnavailable
		}
		if target.ExperienceReward < 0 || target.GoldReward < 0 || target.CrystalReward < 0 {
			return runhost.ErrConflict
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
			actor, target := find(s, c.ActorID), find(s, id)
			if actor.NPC || id == c.ActorID || r.HasBox(id) {
				return nil
			}
			strength := battle.Strong(target.MaxHP, target.MaxMP, target.Attack, target.Defense, target.Agility)
			strong := battle.IsStrongEnemy(strength, requesterStrength)
			reward := runhost.Earnings{EXP: target.ExperienceReward, Gold: target.GoldReward}
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
			reward, err = modifyRunDefeatEarnings(reward, actor.Gear, r.Mode, wish, len(r.Roster), rng)
			if err != nil {
				return err
			}
			if effects.Earnings == nil {
				effects.Earnings = make(map[string]runhost.Earnings)
			}
			for _, allyID := range r.Roster {
				ally := find(s, allyID)
				if ally == nil || ally.HP <= 0 || ally.TeamID != actor.TeamID || ally.RosterDetached || ally.Banished {
					continue
				}
				effects.Earnings[allyID], err = runhost.AddEarnings(effects.Earnings[allyID], reward)
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
			effects.CreditedDefeats = append(effects.CreditedDefeats, identity)
			effects.Defeats = append(effects.Defeats, runDefeatFact(r, actor, target, c.DefeatedStatus, identity, strength, strong, pvp, wish, rng))
			effects.EXUnlocked = rollRoyalBeeEX(r.Stage, identity, rng) || effects.EXUnlocked
			return nil
		},
	}
}

func modifyRunDefeatEarnings(e runhost.Earnings, gear *corebattle.BattleGearStats, mode, wish string, partySize int, rng random.Generator) (runhost.Earnings, error) {
	if gear != nil && gear.AccessoryID == "item-167" {
		var err error
		e.EXP, err = economy.SafeAdd(e.EXP, e.EXP/5)
		if err != nil {
			return runhost.Earnings{}, err
		}
	}
	if mode == CrawlModeCalm {
		e.EXP, e.Gold, e.Crystals = e.EXP/2, e.Gold/2, e.Crystals/2
	}
	bonus := chapelDefeatBonus(e, wish, rng.Intn)
	if gear != nil && gear.WeaponID == "weapon-01" && gear.ArmorID == "armor-01" && rng.Intn(4) == 0 {
		bonus.EXP += 5
	}
	e, err := runhost.AddEarnings(e, bonus)
	if err != nil {
		return runhost.Earnings{}, err
	}
	return scaleCooperativeEarnings(e, min(3, max(0, partySize-1))*10)
}

func runDefeatFact(r *NormalRun, actor, target *corebattle.LifetimeActor, status, identity string, strength int, strong, pvp bool, wish string, rng random.Generator) battle.ExplicitDefeat {
	fact := battle.ExplicitDefeat{Monster: battle.DefeatedMonsterRecord{
		MonsterID: battle.ParseDefeatedMonsterID(strings.TrimRight(identity, "ABCDEFGH")), MonsterName: battle.CleanMonsterName(target.Name),
		Habitat: r.Stage.Name, Icon: target.Icon, Strong: strength, HP: target.MaxHP, MP: target.MaxMP,
		Attack: target.Attack, Defense: target.Defense, Agility: target.Agility, ExperienceReward: target.ExperienceReward, GoldReward: target.GoldReward,
	}, StrongKill: strong && actor.HP > 0, PvP: pvp}
	if actor.HP > 0 && !pvp && target.NPC {
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

// LevelUpEligible is the player step's branch input. releasedStockEXP is any
// already pending (including retained) stock the operation releases.
func (e RunActionEffects) LevelUpEligible(s *corebattle.LifetimeState, requester corecharacter.Character, mode string, releasedStockEXP int) (bool, error) {
	return runhost.LevelUpDue(s, requester, mode == CrawlModeTry, releasedStockEXP, e.Earnings[requester.ID].EXP)
}
