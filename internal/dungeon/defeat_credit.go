package dungeon

import (
	"context"
	"fmt"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/economy"
)

// DefeatCredit is one character's accepted defeat earnings awaiting SQL. Its
// ID is unique within the run, so settlement applies it at most once.
type DefeatCredit struct {
	ID          string                  `json:"id"`
	CharacterID string                  `json:"character_id"`
	Reward      corebattle.Reward       `json:"reward"`
	Defeats     []battle.ExplicitDefeat `json:"defeats,omitempty"`
}

// BlessingProvider reads a character's active chapel wish.
type BlessingProvider interface {
	GetActiveBlessing(ctx context.Context, characterID string) (string, error)
}

// SetBlessingProvider configures the chapel wish reader for defeat modifiers.
func (s *Service) SetBlessingProvider(provider BlessingProvider) { s.blessings = provider }

// defeatShares is one combat step's result of the defeat routine.
type defeatShares struct {
	credits []DefeatCredit               // killer and requester shares, committed right after the step
	stock   map[string]corebattle.Reward // other living members' shares (stock.cgi)
	leader  corebattle.Reward            // the leader's whole share, recorded on the run
}

type killerFacts struct {
	strength int
	jobID    string
	wish     string
	gear     *corebattle.BattleGearStats
	food     bool
	resolved bool
}

// defeatShares runs the legacy defeat routine for every defeat the battle
// reached (_battle.cgi:140–277). Each enemy's earnings, after the killer's
// modifiers, go to every member alive at that defeat. The killer is credited
// at once, and so is the requester, whose own stock legacy applies at the end
// of the action (_battle.cgi:912); other members accrue stock.
func (s *Service) shareDefeats(ctx context.Context, exp *ActiveExpedition, requester string, out encounterOutcome, enemies []corebattle.Participant, monsters []DungeonMonster) (defeatShares, error) {
	res := defeatShares{stock: make(map[string]corebattle.Reward)}
	if len(out.defeats) == 0 {
		return res, nil
	}
	habitat := ""
	if d, ok := s.dungeonMap[exp.DungeonID]; ok {
		habitat = d.Name
	}
	killers := make(map[string]*killerFacts)
	crystals := make(map[string]int)
	creditID := func(id string) string { return fmt.Sprintf("r%d:%s", exp.Revision, id) }
	for _, d := range out.defeats {
		i := enemyIndex(enemies, d.TargetID)
		if i < 0 || i >= len(monsters) || !exp.ContainsCharacter(d.ActorID) {
			continue
		}
		k, err := s.killer(ctx, killers, out.allies, d.ActorID)
		if err != nil {
			return defeatShares{}, err
		}
		target, m := enemies[i], monsters[i]
		strength := battle.Strong(target.MaxHP, target.MaxMP, target.Attack, target.Defense, target.Agility)
		strong := battle.IsStrongEnemy(strength, k.strength)

		got, rolled := crystals[d.TargetID]
		if !rolled {
			if m.combat != nil {
				got = m.combat.CrystalReward
			}
			if got == 0 {
				chance := 1
				if strong {
					chance++
				}
				if s.rng.Intn(50) < chance {
					got = 1
				}
			}
			crystals[d.TargetID] = got
		}
		xp, gold := m.ExpReward, m.GoldReward
		if battle.IsGoldBlessing(k.wish) && s.rng.Intn(4) < 1 {
			gold += gold / 2
		}
		if battle.IsExpBlessing(k.wish) && s.rng.Intn(4) < 1 {
			xp += xp / 2
		}
		if k.gear != nil && k.gear.WeaponID == "weapon-01" && k.gear.ArmorID == "armor-01" && s.rng.Intn(4) < 1 {
			xp += 5
		}
		reward := corebattle.Reward{Experience: xp, Currency: gold, Crystals: got}

		for _, id := range d.RecipientIDs {
			if !exp.ContainsCharacter(id) {
				continue
			}
			if id == exp.CharacterID {
				if res.leader, err = addReward(res.leader, reward); err != nil {
					return defeatShares{}, err
				}
			}
			if id != d.ActorID && id != requester {
				if res.stock[id], err = addReward(res.stock[id], reward); err != nil {
					return defeatShares{}, err
				}
				continue
			}
			credit := DefeatCredit{ID: creditID(id), CharacterID: id, Reward: reward}
			if id == d.ActorID {
				credit.Defeats = []battle.ExplicitDefeat{s.defeatFact(k, target, m, habitat, strength, strong)}
			}
			if res.credits, err = mergeCredit(res.credits, credit); err != nil {
				return defeatShares{}, err
			}
		}
	}
	return res, nil
}

// defeatFact records the killer's strong-kill counter and recruitment roll.
// Dungeon never sets legacy $win, so a strong kill counts as a monster kill.
func (s *Service) defeatFact(k *killerFacts, target corebattle.Participant, m DungeonMonster, habitat string, strength int, strong bool) battle.ExplicitDefeat {
	dragonSet := k.gear != nil && k.gear.WeaponID == "weapon-36" && k.gear.ArmorID == "armor-39"
	// The capture-status bonus needs explicit combat (#1865); the auto-resolved battle never captures.
	chance := battle.CalculateTamePar(strong, k.jobID, k.food, battle.IsMonsterBlessing(k.wish), dragonSet, false)
	return battle.ExplicitDefeat{
		Monster: battle.DefeatedMonsterRecord{
			MonsterID: m.ID, MonsterName: battle.CleanMonsterName(m.Name), Habitat: habitat, Icon: target.Icon,
			Strong: strength, HP: target.MaxHP, MP: target.MaxMP, Attack: target.Attack, Defense: target.Defense,
			Agility: target.Agility, ExperienceReward: m.ExpReward, GoldReward: m.GoldReward,
		},
		StrongKill: strong,
		Capture:    s.rng.Float64()*200 < chance,
	}
}

func (s *Service) killer(ctx context.Context, cache map[string]*killerFacts, allies []corebattle.Participant, id string) (*killerFacts, error) {
	if k := cache[id]; k != nil {
		return k, nil
	}
	char, err := s.findCharacter(ctx, id)
	if err != nil {
		return nil, err
	}
	st := char.Stats
	k := &killerFacts{strength: battle.Strong(st.MaxHP, st.MaxMP, st.Attack, st.Defense, st.Agility), jobID: char.JobID}
	for _, a := range allies {
		if a.ID == id {
			k.gear = a.Gear
		}
	}
	if s.blessings != nil {
		if k.wish, err = s.blessings.GetActiveBlessing(ctx, id); err != nil {
			return nil, err
		}
	}
	if s.invRepo != nil {
		inv, err := s.invRepo.FindByCharacterID(ctx, id)
		if err != nil {
			return nil, err
		}
		held, ok := inv.HeldTool()
		k.food = ok && held.DefinitionID == "item-077"
	}
	cache[id] = k
	return k, nil
}

func enemyIndex(enemies []corebattle.Participant, id string) int {
	for i, e := range enemies {
		if e.ID == id {
			return i
		}
	}
	return -1
}

func addReward(a, b corebattle.Reward) (corebattle.Reward, error) {
	exp, err := economy.SafeAdd(a.Experience, b.Experience)
	if err != nil {
		return a, err
	}
	gold, err := economy.SafeAdd(a.Currency, b.Currency)
	if err != nil {
		return a, err
	}
	crystals, err := economy.SafeAdd(a.Crystals, b.Crystals)
	return corebattle.Reward{Experience: exp, Currency: gold, Crystals: crystals}, err
}

// mergeCredit keeps one credit per character and step.
func mergeCredit(credits []DefeatCredit, c DefeatCredit) ([]DefeatCredit, error) {
	for i := range credits {
		if credits[i].CharacterID == c.CharacterID {
			r, err := addReward(credits[i].Reward, c.Reward)
			if err != nil {
				return nil, err
			}
			credits[i].Reward = r
			credits[i].Defeats = append(credits[i].Defeats, c.Defeats...)
			return credits, nil
		}
	}
	return append(credits, c), nil
}
