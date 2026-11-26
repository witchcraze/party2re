package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/validation"
)

// MonsterService is the Monster Grandpa (farm.cgi) surface used by its scene and commands.
type MonsterService interface {
	GetSummary(ctx context.Context, characterID string) (monster.MonsterBoxSummary, error)
	BringToHome(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	DepositToBox(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	Rename(ctx context.Context, characterID, instanceID, newName string) (monster.MonsterInstance, string, error)
	SendMonster(ctx context.Context, senderCharID, recipientCharID, instanceID string) (monster.MonsterInstance, error)
	ReleaseMonster(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
}

type farmMonsterRequest struct {
	InstanceID string `json:"instance_id"`
}

type farmRenameRequest struct {
	InstanceID string `json:"instance_id"`
	CustomName string `json:"custom_name"`
}

type farmSendRequest struct {
	InstanceID    string `json:"instance_id"`
	RecipientName string `json:"recipient_name"`
}

// FarmSceneData is the actor's boxed and home monsters with their counts.
type FarmSceneData struct {
	Parent string `json:"parent"`
	monster.MonsterBoxSummary
}

// FarmResult is a Monster Grandpa command outcome with @モンジィ's reply.
type FarmResult struct {
	Monster monster.MonsterInstance `json:"monster"`
	Message string                  `json:"message"`
}

// farmRecipientFullError names the recipient, as legacy's reply does.
type farmRecipientFullError struct{ name string }

func (e farmRecipientFullError) Error() string { return "recipient monster box is full: " + e.name }

// WithMonster configures the Monster Grandpa scene observation and its commands.
func WithMonster(service MonsterService) Option {
	return func(h *Handler) {
		h.monster = service
		if service == nil {
			withActionCommand[farmMonsterRequest]("farm_bring_home", nil, nil)(h)
			withActionCommand[farmRenameRequest]("farm_rename", nil, nil)(h)
			withActionCommand[farmMonsterRequest]("farm_deposit", nil, nil)(h)
			withActionCommand[farmSendRequest]("farm_send", nil, nil)(h)
			withActionCommand[farmMonsterRequest]("farm_release", nil, nil)(h)
			return
		}
		// Replies are farm.cgi's $npc_com lines.
		withActionCommand("farm_bring_home", func(ctx context.Context, actor string, p farmMonsterRequest) (any, error) {
			char, err := h.characters.Get(ctx, actor)
			if err != nil {
				return nil, err
			}
			inst, err := service.BringToHome(ctx, actor, p.InstanceID)
			if err != nil {
				return nil, err
			}
			return FarmResult{Monster: inst, Message: inst.CustomName + "を" + char.Name + "の家に送っておいたぞ"}, nil
		}, farmRejection)(h)
		withActionCommand("farm_rename", func(ctx context.Context, actor string, p farmRenameRequest) (any, error) {
			inst, former, err := service.Rename(ctx, actor, p.InstanceID, p.CustomName)
			if err != nil {
				return nil, err
			}
			return FarmResult{Monster: inst, Message: former + "を" + inst.CustomName + "と名づけたぞ"}, nil
		}, farmRejection)(h)
		withActionCommand("farm_deposit", func(ctx context.Context, actor string, p farmMonsterRequest) (any, error) {
			inst, err := service.DepositToBox(ctx, actor, p.InstanceID)
			if err != nil {
				return nil, err
			}
			return FarmResult{Monster: inst, Message: inst.CustomName + "を預かっておくぞ"}, nil
		}, farmRejection)(h)
		// Legacy addresses a gift by the recipient's name (okuru).
		withActionCommand("farm_send", func(ctx context.Context, actor string, p farmSendRequest) (any, error) {
			if h.characterNames == nil {
				return nil, errSceneNotConfigured
			}
			name, err := validation.ValidateCharacterName(p.RecipientName)
			if err != nil {
				return nil, recipientNotFoundError{name: p.RecipientName}
			}
			recipient, err := h.characterNames.FindByName(ctx, name)
			if errors.Is(err, corecharacter.ErrNotFound) {
				return nil, recipientNotFoundError{name: name}
			}
			if err != nil {
				return nil, err
			}
			inst, err := service.SendMonster(ctx, actor, recipient.ID, p.InstanceID)
			if errors.Is(err, monster.ErrRecipientBoxFull) {
				return nil, farmRecipientFullError{name: name}
			}
			if err != nil {
				return nil, err
			}
			return FarmResult{Monster: inst, Message: inst.CustomName + "を" + name + "のモンスター預かり所に送っておいたぞ"}, nil
		}, farmRejection)(h)
		withActionCommand("farm_release", func(ctx context.Context, actor string, p farmMonsterRequest) (any, error) {
			inst, err := service.ReleaseMonster(ctx, actor, p.InstanceID)
			if err != nil {
				return nil, err
			}
			return FarmResult{Monster: inst, Message: inst.CustomName + "を野生に帰しといたぞ"}, nil
		}, farmRejection)(h)
	}
}

func (h *Handler) farmSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.monster == nil {
		return nil, errSceneNotConfigured
	}
	summary, err := h.monster.GetSummary(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return FarmSceneData{Parent: "town", MonsterBoxSummary: summary}, nil
}

func farmRejection(err error) (int, ErrorDetail) {
	var missing recipientNotFoundError
	var full farmRecipientFullError
	switch {
	case errors.As(err, &missing):
		return http.StatusNotFound, ErrorDetail{Code: "RECIPIENT_NOT_FOUND", Message: missing.name + "というプレイヤーは存在しません"}
	case errors.As(err, &full):
		return http.StatusConflict, ErrorDetail{Code: "FARM_RECIPIENT_FULL", Message: full.name + "のモンスター預かり所がいっぱいです"}
	case errors.Is(err, monster.ErrHomeFull):
		return http.StatusConflict, ErrorDetail{Code: "FARM_HOME_FULL", Message: "これ以上、モンスターを家に連れて行くことはできません"}
	case errors.Is(err, monster.ErrDuplicatePetNameAtHome):
		return http.StatusConflict, ErrorDetail{Code: "FARM_HOME_NAME_TAKEN", Message: "家に同じ名前のモンスターがいます。「＠なづける」で名前を変えてください"}
	case errors.Is(err, monster.ErrBoxFull):
		return http.StatusConflict, ErrorDetail{Code: "FARM_BOX_FULL", Message: "これ以上、モンスターを預かることはできんぞ"}
	case errors.Is(err, monster.ErrNameTooLong):
		return http.StatusBadRequest, ErrorDetail{Code: "FARM_NAME_TOO_LONG", Message: "モンスター名は８文字までです"}
	case errors.Is(err, monster.ErrInvalidName):
		return http.StatusBadRequest, ErrorDetail{Code: "FARM_INVALID_NAME", Message: "Monster name contains whitespace or a forbidden character."}
	case errors.Is(err, monster.ErrCannotSendToSelf):
		return http.StatusBadRequest, ErrorDetail{Code: "FARM_SELF_SEND", Message: "Cannot send a monster to yourself."}
	case errors.Is(err, monster.ErrInvalidLocation):
		return http.StatusConflict, ErrorDetail{Code: "FARM_WRONG_LOCATION", Message: "That monster is not where this command takes it from."}
	// Another character's monster is reported as missing, never confirmed.
	case errors.Is(err, monster.ErrMonsterNotFound), errors.Is(err, monster.ErrForbidden):
		return http.StatusNotFound, ErrorDetail{Code: "FARM_MONSTER_NOT_FOUND", Message: "Choose one of the observed monsters."}
	case errors.Is(err, monster.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	default:
		return 0, ErrorDetail{}
	}
}

var farmFacility = facility{
	title: "モンスターじいさん", commands: []string{"farm_bring_home", "farm_rename", "farm_deposit", "farm_send", "farm_release"},
	read: (*Handler).farmSceneData, placeLog: true,
	npc: &npcProfile{name: "@モンジィ", wordsOf: farmWords},
}

// farmWords are farm.cgi @words. The home limit reads the box capacity
// (docs/design/monster.md#capacity); the 30-monster line is legacy's literal text.
func farmWords(_ *Handler, _ context.Context, char corecharacter.Character) ([]string, error) {
	return []string{
		"わしが有名な@モンジィじゃ。モンスターのことなら何でも聞いてくれい",
		"何度かモンスターを倒していると、なついてくるモンスターがいるのじゃ",
		"人間を好むモンスターもいるということじゃ",
		"{name}の純粋な強さにモンスターはひきつけられるのじゃ",
		fmt.Sprintf("自分の家には%d匹までペットを連れて行くことができるぞい", monster.BoxCapacityForCharacter(char)),
		"モンスターは最大30匹まで預かっておけるぞい。それ以上は、残念じゃが＠わかれるしかないのぉ…",
		"モンスター預かり所がまんぱんの状態だと、モンスターは仲間にならんから注意じゃ",
		"自分が相手より強い方が仲間になりやすいぞい",
		"ふがふがふがふがふがふがふが",
	}, nil
}
