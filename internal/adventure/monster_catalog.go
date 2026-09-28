package adventure

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalidMonster  = errors.New("monster definition is invalid")
	ErrMonsterNotFound = errors.New("monster definition not found")
)

type Monster struct {
	ID               string   `json:"id"`
	Name             string   `json:"name"`
	HP               int      `json:"hp"`
	MP               int      `json:"mp"`
	Attack           int      `json:"attack"`
	Defense          int      `json:"defense"`
	Agility          int      `json:"agility"`
	ExperienceReward int      `json:"exp_reward"`
	GoldReward       int      `json:"gold_reward"`
	DropItemIDs      []string `json:"drop_item_ids,omitempty"`
}

func NewMonster(id, name string, hp, mp, attack, defense, agility, expReward, goldReward int, dropItemIDs []string) (Monster, error) {
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	if id == "" || name == "" || hp <= 0 || mp < 0 || attack < 0 || defense < 0 || agility < 0 || expReward < 0 || goldReward < 0 {
		return Monster{}, ErrInvalidMonster
	}
	return Monster{
		ID:               id,
		Name:             name,
		HP:               hp,
		MP:               mp,
		Attack:           attack,
		Defense:          defense,
		Agility:          agility,
		ExperienceReward: expReward,
		GoldReward:       goldReward,
		DropItemIDs:      dropItemIDs,
	}, nil
}

type MonsterCatalog struct {
	monsters map[string]Monster
}

func NewMonsterCatalog(monsters []Monster) (*MonsterCatalog, error) {
	catalog := &MonsterCatalog{monsters: make(map[string]Monster, len(monsters))}
	for _, m := range monsters {
		m.ID = strings.TrimSpace(m.ID)
		m.Name = strings.TrimSpace(m.Name)
		if m.ID == "" || m.Name == "" || m.HP <= 0 || m.MP < 0 || m.Attack < 0 || m.Defense < 0 || m.Agility < 0 || m.ExperienceReward < 0 || m.GoldReward < 0 {
			return nil, ErrInvalidMonster
		}
		if _, exists := catalog.monsters[m.ID]; exists {
			return nil, ErrInvalidMonster
		}
		catalog.monsters[m.ID] = m
	}
	return catalog, nil
}

func (c *MonsterCatalog) FindByID(id string) (Monster, error) {
	if c == nil {
		return Monster{}, ErrMonsterNotFound
	}
	m, ok := c.monsters[id]
	if !ok {
		return Monster{}, ErrMonsterNotFound
	}
	return m, nil
}

func (c *MonsterCatalog) Monsters() []Monster {
	if c == nil {
		return nil
	}
	values := make([]Monster, 0, len(c.monsters))
	for _, m := range c.monsters {
		values = append(values, m)
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].ID < values[j].ID
	})
	return values
}

//go:embed data/monsters.json
var monstersCatalogData []byte

func InitialMonsterCatalog() (*MonsterCatalog, error) {
	var list []Monster
	if err := json.Unmarshal(monstersCatalogData, &list); err != nil {
		return nil, fmt.Errorf("decode monster catalog: %w", err)
	}
	return NewMonsterCatalog(list)
}
