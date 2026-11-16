package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/medal"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// MedalService is the medal castle surface: Small Medal prizes, achievements
// and commemorative medals.
type MedalService interface {
	GetRewards() []medal.Reward
	Claim(ctx context.Context, characterID string, itemID string) (corecharacter.Character, depot.Depot, error)
	GetAchievements(ctx context.Context, characterID string) ([]medal.AchievementProgress, error)
	ClaimAchievement(ctx context.Context, characterID string, achievementID string) (medal.ClaimResult, error)
	GetCharacterMedals(ctx context.Context, characterID string) ([]medal.CharacterMedal, error)
}

type medalClaimRequest struct {
	ItemID string `json:"item_id"`
}

type achievementClaimRequest struct {
	AchievementID string `json:"achievement_id"`
}

// MedalReward is one prize tier of ＠こうかん.
type MedalReward struct {
	Cost     int    `json:"cost"`
	ItemID   string `json:"item_id"`
	ItemName string `json:"item_name"`
}

// MedalSceneData is the actor's Small Medal balance, the prize tiers, their
// achievement progress and the commemorative medals they hold.
type MedalSceneData struct {
	Parent       string                      `json:"parent"`
	SmallMedals  int                         `json:"small_medals"`
	Rewards      []MedalReward               `json:"rewards"`
	Achievements []medal.AchievementProgress `json:"achievements"`
	Medals       []medal.CharacterMedal      `json:"medals"`
}

// MedalClaimResult is the exchanged prize, the remaining balance and the reply.
type MedalClaimResult struct {
	MedalReward
	SmallMedals int    `json:"small_medals"`
	Message     string `json:"message"`
}

// AchievementClaimResult is the claimed achievement and the medal it awarded.
type AchievementClaimResult struct {
	AchievementID   string               `json:"achievement_id"`
	AchievementName string               `json:"achievement_name"`
	Medal           medal.CharacterMedal `json:"medal"`
}

// medalShort carries the tier for medal.cgi's not-enough-medals $mes.
type medalShort struct{ cost int }

func (e medalShort) Error() string { return medal.ErrInsufficientMedals.Error() }
func (e medalShort) Unwrap() error { return medal.ErrInsufficientMedals }

// WithMedal configures the medal castle scene and its commands.
func WithMedal(service MedalService) Option {
	return func(h *Handler) {
		h.medals = service
		if service == nil {
			withActionCommand[medalClaimRequest]("medal_claim", nil, nil)(h)
			withActionCommand[achievementClaimRequest]("achievement_claim", nil, nil)(h)
			return
		}
		withActionCommand("medal_claim", func(ctx context.Context, actor string, p medalClaimRequest) (any, error) {
			reward := medalRewardView(service.GetRewards(), p.ItemID)
			char, _, err := service.Claim(ctx, actor, p.ItemID)
			if errors.Is(err, medal.ErrInsufficientMedals) {
				return nil, medalShort{reward.Cost}
			}
			if err != nil {
				return nil, err
			}
			return MedalClaimResult{MedalReward: reward, SmallMedals: char.SmallMedals,
				Message: fmt.Sprintf("メダル%d枚の賞品と交換するのじゃな！%sは%sの預かり所に送っておいたぞ！", reward.Cost, reward.ItemName, char.Name)}, nil
		}, medalActionRejection)(h)
		withActionCommand("achievement_claim", func(ctx context.Context, actor string, p achievementClaimRequest) (any, error) {
			res, err := service.ClaimAchievement(ctx, actor, p.AchievementID)
			if err != nil {
				return nil, err
			}
			return AchievementClaimResult{AchievementID: res.AchievementID, AchievementName: res.AchievementName, Medal: res.Medal}, nil
		}, medalActionRejection)(h)
	}
}

func (h *Handler) medalSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.medals == nil {
		return nil, errSceneNotConfigured
	}
	actor := r.Snapshot.Character
	achievements, err := h.medals.GetAchievements(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	medals, err := h.medals.GetCharacterMedals(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	rewards := h.medals.GetRewards()
	views := make([]MedalReward, 0, len(rewards))
	for _, reward := range rewards {
		views = append(views, medalRewardView(rewards, reward.ItemID))
	}
	return MedalSceneData{Parent: "town", SmallMedals: actor.SmallMedals, Rewards: views,
		Achievements: append([]medal.AchievementProgress{}, achievements...), Medals: append([]medal.CharacterMedal{}, medals...)}, nil
}

// medalRewardView finds itemID's tier; an unknown item yields a zero tier,
// which Claim rejects.
func medalRewardView(rewards []medal.Reward, itemID string) MedalReward {
	for _, reward := range rewards {
		if reward.ItemID == itemID {
			return MedalReward{Cost: reward.Cost, ItemID: reward.ItemID, ItemName: itemName(reward.ItemID)}
		}
	}
	return MedalReward{ItemID: itemID}
}

// itemName is the catalog display name, or the ID when the catalog lacks it.
func itemName(id string) string {
	catalog, err := coreitem.DefaultCatalog()
	if err != nil {
		return id
	}
	def, err := catalog.FindByID(id)
	if err != nil {
		return id
	}
	return def.Name
}

func medalActionRejection(err error) (int, ErrorDetail) {
	var short medalShort
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.As(err, &short):
		return http.StatusConflict, ErrorDetail{Code: "MEDAL_INSUFFICIENT", Message: fmt.Sprintf("小さなメダル%d枚の賞品と交換するにはメダルが足りないぞ", short.cost)}
	case errors.Is(err, medal.ErrRewardNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "MEDAL_REWARD_NOT_FOUND", Message: "Choose one of the observed rewards."}
	case errors.Is(err, medal.ErrAchievementNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "ACHIEVEMENT_NOT_FOUND", Message: "Choose one of the observed achievements."}
	case errors.Is(err, medal.ErrAchievementNotCompleted):
		return http.StatusConflict, ErrorDetail{Code: "ACHIEVEMENT_NOT_COMPLETED", Message: "The achievement is not completed yet."}
	case errors.Is(err, medal.ErrAchievementAlreadyClaimed):
		return http.StatusConflict, ErrorDetail{Code: "ACHIEVEMENT_ALREADY_CLAIMED", Message: "The achievement medal is already claimed."}
	default:
		return 0, ErrorDetail{}
	}
}

// The medal castle and its keeper use clean-room names (docs/design/medal.md);
// medal.cgi @words.
var medalFacility = facility{
	title: "メダル伯爵の城", commands: []string{"medal_claim", "achievement_claim"}, read: (*Handler).medalSceneData, placeLog: true,
	npc: &npcProfile{name: "@メダル伯爵", words: []string{
		"わしはメダル伯爵じゃ、小さなメダルを集めておる",
		"小さなメダルを持ってきたら代わりに褒美をやろう",
		"世界中の小さなメダルはわしのもんじゃ！",
		"わしの夢は小さなメダルを山ほど集めてだな…ムニャムニャ…",
		"小さなメダルをよこさんかい！",
		"自分の家で小さなメダルを使うと、わしの所にメダルが届けられるのじゃ",
		"小さなメダルはモンスターの住処の奥深くにあるらしいのじゃ",
		"{name}からは小さなメダルを{medals}枚あずかっておるぞ",
	}},
}
