package http

import (
	"github.com/witchcraze/party2re/internal/adventure"
	"time"
)

type adventureResponse struct {
	MonsterTames          map[string][]monsterTameResponse `json:"monster_tames,omitempty"`
	PostAdventureMessages map[string]string                `json:"post_adventure_messages,omitempty"`
	ID                    string                           `json:"id"`
	CharacterID           string                           `json:"character_id"`
	StageID               string                           `json:"stage_id"`
	StartedAt             time.Time                        `json:"started_at"`
	FloorsCleared         int                              `json:"floors_cleared"`
	IsCleared             bool                             `json:"is_cleared"`
	PartySize             int                              `json:"party_size"`
	Resolved              bool                             `json:"resolved"`
	ExperienceReward      int                              `json:"experience_reward"`
}

func toAdventureResponse(adv adventure.Adventure) adventureResponse {
	return adventureResponse{
		PostAdventureMessages: adv.PostAdventureMessages,
		MonsterTames:          toMonsterTames(adv.MonsterTames),
		ID:                    adv.ID,
		CharacterID:           adv.CharacterID,
		StageID:               adv.StageID,
		StartedAt:             adv.StartedAt,
		FloorsCleared:         adv.FloorsCleared,
		IsCleared:             adv.IsCleared,
		PartySize:             adv.PartySize,
		Resolved:              adv.Resolved,
		ExperienceReward:      adv.ExperienceReward,
	}
}
