package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/helperquest"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// HelperService is the helper quest surface used by its scene and helper_complete.
type HelperService interface {
	ListQuests(ctx context.Context, now time.Time) ([]helperquest.Quest, error)
	CompleteQuest(ctx context.Context, characterID, questID string, now time.Time) (helperquest.CompletionResult, error)
}

type helperCompleteRequest struct {
	QuestID string `json:"quest_id"` // empty only exchanges expired requests
}

// HelperQuest is one board request with its reward's display name.
type HelperQuest struct {
	helperquest.Quest
	RewardName string `json:"reward_name"`
}

// HelperSceneData is the shared request board (＠みる), expired requests included.
type HelperSceneData struct {
	Parent string        `json:"parent"`
	Quests []HelperQuest `json:"quests"`
}

// HelperCompleteResult is the resolved request (null for renewal only), the
// expired requests exchanged for new ones and the clerk's reply.
type HelperCompleteResult struct {
	CompletedQuest *HelperQuest        `json:"completed_quest"`
	Renewed        []helperquest.Quest `json:"renewed"`
	Message        string              `json:"message"`
}

// helperRefusal carries the request for helper.cgi's refusal $mes.
type helperRefusal struct {
	quest helperquest.Quest
	err   error
}

func (e helperRefusal) Error() string { return e.err.Error() }
func (e helperRefusal) Unwrap() error { return e.err }

// WithHelper configures the helper quest scene and helper_complete.
func WithHelper(service HelperService) Option {
	return func(h *Handler) {
		h.helpers = service
		if service == nil {
			withActionCommand[helperCompleteRequest]("helper_complete", nil, nil)(h)
			return
		}
		withActionCommand("helper_complete", func(ctx context.Context, actor string, p helperCompleteRequest) (any, error) {
			now := time.Now().UTC()
			res, err := service.CompleteQuest(ctx, actor, p.QuestID, now)
			if errors.Is(err, helperquest.ErrGuildRequired) || errors.Is(err, helperquest.ErrInsufficientItems) {
				if quest, ok := findHelperQuest(ctx, service, p.QuestID, now); ok {
					return nil, helperRefusal{quest, err}
				}
			}
			if err != nil {
				return nil, err
			}
			return helperCompleteResult(res), nil
		}, helperActionRejection)(h)
	}
}

func findHelperQuest(ctx context.Context, service HelperService, id string, now time.Time) (helperquest.Quest, bool) {
	quests, err := service.ListQuests(ctx, now)
	if err != nil {
		return helperquest.Quest{}, false
	}
	for _, q := range quests {
		if q.ID == id {
			return q, true
		}
	}
	return helperquest.Quest{}, false
}

// helperCompleteResult composes ＠かいけつ's $npc_com lines (helper.cgi:112–128).
func helperCompleteResult(res helperquest.CompletionResult) HelperCompleteResult {
	out := HelperCompleteResult{Renewed: append([]helperquest.Quest{}, res.Renewed...)}
	var lines []string
	if res.CompletedQuest != nil {
		q := helperQuestView(*res.CompletedQuest)
		out.CompletedQuest = &q
		name := ""
		if res.Character != nil {
			name = res.Character.Name
		}
		lines = append(lines, fmt.Sprintf("%s たしかに受け取りました。こちらが報酬の %s になります！%sさんの預り所に送っておきますね！", helperDetail(q.Quest), q.RewardName, name))
	}
	for _, old := range res.Renewed {
		lines = append(lines, fmt.Sprintf("『%s』は誰も解決できそうにないので、新しい依頼がきました！", old.Title))
	}
	out.Message = strings.Join(lines, "\n")
	return out
}

func helperQuestView(q helperquest.Quest) HelperQuest {
	return HelperQuest{Quest: q, RewardName: itemName(q.RewardItemID)}
}

// helperDetail is the legacy clear condition, e.g. "やくそう 3 個".
func helperDetail(q helperquest.Quest) string {
	unit := map[helperquest.QuestKind]string{helperquest.KindWeapon: "本", helperquest.KindArmor: "着", helperquest.KindItem: "個", helperquest.KindMonster: "匹"}[q.Kind]
	return fmt.Sprintf("%s %d %s", q.TargetName, q.RequiredCount, unit)
}

func (h *Handler) helperSceneData(ctx context.Context, _ playercontext.Result) (any, error) {
	if h.helpers == nil {
		return nil, errSceneNotConfigured
	}
	quests, err := h.helpers.ListQuests(ctx, time.Now().UTC())
	if err != nil {
		return nil, err
	}
	views := make([]HelperQuest, 0, len(quests))
	for _, q := range quests {
		views = append(views, helperQuestView(q))
	}
	return HelperSceneData{Parent: "town", Quests: views}, nil
}

func helperActionRejection(err error) (int, ErrorDetail) {
	var refusal helperRefusal
	hasQuest := errors.As(err, &refusal)
	switch {
	case errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, helperquest.ErrQuestNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "HELPER_QUEST_NOT_FOUND", Message: "Choose one of the observed requests."}
	case errors.Is(err, helperquest.ErrQuestAlreadyDone):
		return http.StatusConflict, ErrorDetail{Code: "HELPER_QUEST_COMPLETED", Message: "The request is already resolved."}
	case errors.Is(err, helperquest.ErrGuildRequired):
		message := "ギルド専用のクエストよ。ギルドに加入していないと依頼を受けることができないわ"
		if hasQuest {
			message = refusal.quest.Title + "は" + message
		}
		return http.StatusConflict, ErrorDetail{Code: "HELPER_GUILD_REQUIRED", Message: message}
	case errors.Is(err, helperquest.ErrInsufficientItems):
		message := "条件を満たしてないようです"
		if hasQuest {
			message = helperDetail(refusal.quest) + " の" + message
		}
		return http.StatusConflict, ErrorDetail{Code: "HELPER_CONDITION_UNMET", Message: message}
	default:
		return 0, ErrorDetail{}
	}
}

// 手助けクエスト keeps its legacy title; the clerk uses a clean-room name
// (docs/design/rescue-and-helper.md). helper.cgi @words.
var helperFacility = facility{
	title: "手助けクエスト", commands: []string{"helper_complete"}, read: (*Handler).helperSceneData, placeLog: true,
	npc: &npcProfile{name: "@ミモザ", words: []string{
		"ここは困っている人達を助ける何でも屋よ",
		"アイテムやモンスターを依頼主の代わりに探してきてほしいの",
		"報酬は錬金に必要となる素材など、他では手に入らないアイテムよ",
		"たまにレアクエストといって、条件を満たすのが難しい依頼がくるの。でも、その時の報酬は他では手に入れることができないものよ",
		"誰も解決することができない依頼はしばらくすると違う依頼に変わるわ",
	}},
}
