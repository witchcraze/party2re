// Package stagedata embeds the legacy stage transcription (party2/stage/<n>.cgi)
// shared by every feature that draws stage enemies.
package stagedata

import (
	_ "embed"
	"encoding/json"
	"fmt"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

// Stage is one stage file's source-position tables.
type Stage struct {
	BossRosters [][]string `json:"boss_rosters"`
	MonsterIDs  []string   `json:"monster_ids"`
	Appears     []int      `json:"appears"`
	PartyCopy   bool       `json:"party_copy,omitempty"`
	// EXRoster is the floor-12 table; summons keep using MonsterIDs/Appears.
	EXRoster []string `json:"ex_roster,omitempty"`
	EXField  string   `json:"ex_field,omitempty"`
}

// Monster is one stage entry: base stats, rewards and the legacy battle payload.
type Monster struct {
	ID                string                    `json:"id"`
	Name              string                    `json:"name"`
	EncounterIdentity string                    `json:"encounter_identity,omitempty"`
	HP                int                       `json:"hp"`
	MP                int                       `json:"mp"`
	Attack            int                       `json:"attack"`
	Defense           int                       `json:"defense"`
	Agility           int                       `json:"agility"`
	CMP               int                       `json:"cmp,omitempty"`
	ExperienceReward  int                       `json:"exp_reward"`
	GoldReward        int                       `json:"gold_reward"`
	DropItemIDs       []string                  `json:"drop_item_ids,omitempty"`
	Combat            *corebattle.LifetimeActor `json:"combat,omitempty"`
}

//go:embed monsters.json
var monsterData []byte

//go:embed stages.json
var stageData []byte

// Monsters decodes a fresh copy of every stage entry.
func Monsters() ([]Monster, error) {
	var list []Monster
	if err := json.Unmarshal(monsterData, &list); err != nil {
		return nil, fmt.Errorf("decode stage monsters: %w", err)
	}
	return list, nil
}

// Stages decodes a fresh copy of every stage table, keyed like "stage-05" or
// "stage-26-summer".
func Stages() (map[string]Stage, error) {
	var stages map[string]Stage
	if err := json.Unmarshal(stageData, &stages); err != nil {
		return nil, fmt.Errorf("decode stage tables: %w", err)
	}
	return stages, nil
}
