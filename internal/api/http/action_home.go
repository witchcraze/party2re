package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type sleepRequest struct {
	TargetHomeID string `json:"target_home_id"`
}

type wakeResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Character characterResponse `json:"character"`
}

type homeColorRequest struct {
	Color string `json:"color"`
}

type homeItemUseRequest struct {
	InstanceID string `json:"instance_id"`
	Source     string `json:"source"`
}

type homePhraseTeachRequest struct {
	Phrase string `json:"phrase"`
}

type homePhraseForgetRequest struct {
	PhraseID string `json:"phrase_id"`
}

type homeNoticeAckRequest struct {
	NoticeIDs []string `json:"notice_ids"`
}

// HomeColorResult is the colour ＠からー set.
type HomeColorResult struct {
	Color string `json:"color"`
}

// HomeNoticeAckResult lists the notices the owner acknowledged.
type HomeNoticeAckResult struct {
	NoticeIDs []string `json:"notice_ids"`
}

// HomeForgetResult names the forgotten phrase, as legacy ＠ことばをわすれさせる.
type HomeForgetResult struct {
	PhraseID string `json:"phrase_id"`
}

// WithHome configures the Home scene and its Gateway commands. Owner-only
// commands always act on the actor's own home; the scene admits them only there.
func WithHome(service HomeService) Option {
	return func(h *Handler) {
		h.homes = service
		if service == nil {
			withActionCommand[sleepRequest]("home_sleep", nil, nil)(h)
			withActionCommand[struct{}]("home_wake", nil, nil)(h)
			withActionCommand[homeColorRequest]("home_color", nil, nil)(h)
			withActionCommand[homeItemUseRequest]("home_item_use", nil, nil)(h)
			withActionCommand[homePhraseTeachRequest]("home_phrase_teach", nil, nil)(h)
			withActionCommand[homePhraseForgetRequest]("home_phrase_forget", nil, nil)(h)
			withActionCommand[homeNoticeAckRequest]("home_notice_ack", nil, nil)(h)
			return
		}
		withActionCommand("home_sleep", func(ctx context.Context, actor string, p sleepRequest) (any, error) {
			return service.Sleep(ctx, actor, p.TargetHomeID)
		}, homeActionRejection)(h)
		withActionCommand("home_wake", func(ctx context.Context, actor string, _ struct{}) (any, error) {
			value, err := service.Wake(ctx, actor)
			if err != nil {
				return nil, err
			}
			return wakeResponse{Success: value.Success, Message: value.Message, Character: toCharacterResponse(value.Character)}, nil
		}, homeActionRejection)(h)
		withActionCommand("home_color", func(ctx context.Context, actor string, p homeColorRequest) (any, error) {
			if err := service.SetCharacterColor(ctx, actor, p.Color); err != nil {
				return nil, err
			}
			return HomeColorResult{Color: p.Color}, nil
		}, homeActionRejection)(h)
		withActionCommand("home_item_use", func(ctx context.Context, actor string, p homeItemUseRequest) (any, error) {
			return service.UseHomeItem(ctx, actor, p.InstanceID, p.Source)
		}, homeActionRejection)(h)
		withActionCommand("home_phrase_teach", func(ctx context.Context, actor string, p homePhraseTeachRequest) (any, error) {
			return service.TeachCompanionPhrase(ctx, actor, p.Phrase)
		}, homeActionRejection)(h)
		withActionCommand("home_phrase_forget", func(ctx context.Context, actor string, p homePhraseForgetRequest) (any, error) {
			if err := service.ForgetCompanionPhrase(ctx, p.PhraseID, actor); err != nil {
				return nil, err
			}
			return HomeForgetResult{PhraseID: p.PhraseID}, nil
		}, homeActionRejection)(h)
		withActionCommand("home_notice_ack", func(ctx context.Context, actor string, p homeNoticeAckRequest) (any, error) {
			if err := service.ClearDeliveryNotices(ctx, actor, p.NoticeIDs); err != nil {
				return nil, err
			}
			return HomeNoticeAckResult{NoticeIDs: append([]string{}, p.NoticeIDs...)}, nil
		}, homeActionRejection)(h)
	}
}

// Legacy ＠はなす in a home (home.cgi hanasu) has a resident speak one of the
// visited home's taught phrases.
const homeNobodyHere = "しかし、誰もいなかった…"

// homeTalk speaks for a random resident of the observed home. Without phrases
// the resident stays silent, as legacy logs no empty $npc_com.
// shortcut: replies are not logged until Home has its owner-scoped place log (#2254).
func homeTalk(h *Handler, ctx context.Context, current playercontext.Result) (NPCResult, error) {
	talk, err := h.homes.TalkToCompanion(ctx, homeOwner(current.Navigation, current.Snapshot.Character.ID))
	if errors.Is(err, home.ErrNoPetsAtHome) {
		return NPCResult{Dialogue: homeNobodyHere, Private: true}, nil
	}
	if err != nil {
		return NPCResult{}, err
	}
	if talk.Phrase == "" {
		return NPCResult{}, nil
	}
	return NPCResult{NPCName: talk.PetName, Dialogue: talk.Phrase}, nil
}

func homeActionRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, home.ErrAlreadySleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_ALREADY_SLEEPING", Message: "Character is already sleeping."}
	case errors.Is(err, home.ErrStillSleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_STILL_SLEEPING", Message: "Sleep duration has not elapsed; recovery is not ready."}
	case errors.Is(err, home.ErrNotSleeping):
		return http.StatusConflict, ErrorDetail{Code: "HOME_NOT_SLEEPING", Message: "Character is not sleeping."}
	case errors.Is(err, home.ErrWakeInProgress):
		return http.StatusConflict, ErrorDetail{Code: "HOME_WAKE_IN_PROGRESS", Message: "Wake recovery is already in progress."}
	case errors.Is(err, home.ErrCharacterNotFound), errors.Is(err, corecharacter.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "CHARACTER_NOT_FOUND", Message: "Character not found."}
	case errors.Is(err, home.ErrHouseNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_HOUSE_NOT_FOUND", Message: "Target house not found."}
	case errors.Is(err, home.ErrHouseExpired):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_HOUSE_EXPIRED", Message: "Target house has expired."}
	case errors.Is(err, corecharacter.ErrInvalidColor):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INVALID_COLOR", Message: "#から始まる(16進数の)カラーコードを記入してください"}
	case errors.Is(err, home.ErrItemNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_ITEM_NOT_FOUND", Message: "Choose one of the observed home items."}
	case errors.Is(err, home.ErrCannotUseHere):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_ITEM_CANNOT_USE", Message: "ここでは使えません"}
	case errors.Is(err, home.ErrNoPetsAtHome):
		return http.StatusConflict, ErrorDetail{Code: "HOME_NO_RESIDENTS", Message: "教える相手がいません"}
	case errors.Is(err, home.ErrPhraseTooLong):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_PHRASE_TOO_LONG", Message: "言葉が長すぎます(120文字まで)"}
	case errors.Is(err, home.ErrEmptyPhrase), isTextValidationError(err):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INVALID_PHRASE", Message: "Phrase must be one nonempty line of plain text."}
	case errors.Is(err, home.ErrPhraseNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_PHRASE_NOT_FOUND", Message: "Choose one of the observed phrases."}
	default:
		return 0, ErrorDetail{}
	}
}
