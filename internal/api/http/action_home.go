package http

import (
	"context"
	"errors"
	"net/http"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/validation"
)

type sleepRequest struct {
	TargetHomeID string `json:"target_home_id"`
}

type wakeResponse struct {
	Success   bool              `json:"success"`
	Message   string            `json:"message"`
	Character characterResponse `json:"character"`
}

type houseBuildRequest struct {
	TownID     string `json:"town_id"`
	HouseStyle string `json:"house_style"`
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

type homeLetterSendRequest struct {
	RecipientName string `json:"recipient_name"`
	Content       string `json:"content"`
}

type homeLetterRequest struct {
	LetterID string `json:"letter_id"`
}

// HomeLetterResult names the letter a read or delete command applied to.
type HomeLetterResult struct {
	LetterID string `json:"letter_id"`
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
			withActionCommand[houseBuildRequest]("house_build", nil, nil)(h)
			withActionCommand[homeColorRequest]("home_color", nil, nil)(h)
			withActionCommand[homeItemUseRequest]("home_item_use", nil, nil)(h)
			withActionCommand[homePhraseTeachRequest]("home_phrase_teach", nil, nil)(h)
			withActionCommand[homePhraseForgetRequest]("home_phrase_forget", nil, nil)(h)
			withActionCommand[homeNoticeAckRequest]("home_notice_ack", nil, nil)(h)
			withActionCommand[homeLetterSendRequest]("home_letter_send", nil, nil)(h)
			withActionCommand[homeLetterRequest]("home_letter_read", nil, nil)(h)
			withActionCommand[homeLetterRequest]("home_letter_delete", nil, nil)(h)
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
		withActionCommand("house_build", func(ctx context.Context, actor string, p houseBuildRequest) (any, error) {
			return service.BuildHouse(ctx, actor, p.TownID, p.HouseStyle)
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
		// Legacy addresses a letter by the recipient's name (send_letter).
		withActionCommand("home_letter_send", func(ctx context.Context, actor string, p homeLetterSendRequest) (any, error) {
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
			letter, err := service.SendLetter(ctx, actor, recipient.ID, p.Content)
			if isTextValidationError(err) {
				return nil, invalidLetterError{err}
			}
			return letter, err
		}, homeActionRejection)(h)
		withActionCommand("home_letter_read", func(ctx context.Context, actor string, p homeLetterRequest) (any, error) {
			return HomeLetterResult{LetterID: p.LetterID}, service.ReadLetter(ctx, p.LetterID, actor)
		}, homeActionRejection)(h)
		withActionCommand("home_letter_delete", func(ctx context.Context, actor string, p homeLetterRequest) (any, error) {
			return HomeLetterResult{LetterID: p.LetterID}, service.DeleteLetter(ctx, p.LetterID, actor)
		}, homeActionRejection)(h)
	}
}

// Legacy ＠はなす in a home (home.cgi hanasu) has a resident speak one of the
// visited home's taught phrases.
const homeNobodyHere = "しかし、誰もいなかった…"

// homeTalk speaks for a random resident of the observed home. Without phrases
// the resident stays silent, as legacy logs no empty $npc_com.
func homeTalk(h *Handler, ctx context.Context, current playercontext.Result) (NPCResult, error) {
	talk, err := h.homes.TalkToCompanion(ctx, homeOwner(current.Navigation, current.Snapshot.Character.ID))
	if errors.Is(err, home.ErrNoPetsAtHome) {
		return NPCResult{Dialogue: homeNobodyHere, Private: true}, nil
	}
	if err != nil {
		return NPCResult{}, err
	}
	h.appendNPCLog(ctx, current, "＠はなす", talk.PetName, talk.Phrase, "")
	if talk.Phrase == "" {
		return NPCResult{}, nil
	}
	return NPCResult{NPCName: talk.PetName, Dialogue: talk.Phrase}, nil
}

// invalidLetterError keeps letter text failures apart from phrase text
// failures; it deliberately does not unwrap.
type invalidLetterError struct{ error }

func homeActionRejection(err error) (int, ErrorDetail) {
	if status, detail := speechRejection(err); status != 0 {
		return status, detail
	}
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
	case errors.Is(err, home.ErrInvalidTownID):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INVALID_TOWN", Message: "Invalid town."}
	case errors.Is(err, home.ErrInvalidHouseStyle):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INVALID_HOUSE_STYLE", Message: "This house style cannot be built in this town."}
	case errors.Is(err, home.ErrInsufficientFunds):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INSUFFICIENT_GOLD", Message: "Insufficient gold."}
	case errors.Is(err, home.ErrTownMaxHousesReached):
		return http.StatusConflict, ErrorDetail{Code: "HOME_TOWN_FULL", Message: "This town has reached its house limit."}
	case errors.Is(err, home.ErrAlreadyOwnsHouse):
		return http.StatusConflict, ErrorDetail{Code: "HOME_ALREADY_OWNED", Message: "You already own a house."}
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
	// Another character's letter is indistinguishable from a missing one.
	case errors.Is(err, home.ErrLetterNotFound), errors.Is(err, home.ErrForbidden):
		return http.StatusNotFound, ErrorDetail{Code: "HOME_LETTER_NOT_FOUND", Message: "Choose one of the observed letters."}
	case errors.Is(err, home.ErrLetterThrottled):
		return http.StatusTooManyRequests, ErrorDetail{Code: "HOME_LETTER_THROTTLED", Message: "連続で手紙を送ることはできません。しばらくしてから送ってください"}
	case errors.Is(err, home.ErrEmptyContent), errors.Is(err, home.ErrContentTooLong), errors.As(err, new(invalidLetterError)):
		return http.StatusBadRequest, ErrorDetail{Code: "HOME_INVALID_LETTER", Message: "Letter must be nonempty plain text within the length limit."}
	default:
		return 0, ErrorDetail{}
	}
}
