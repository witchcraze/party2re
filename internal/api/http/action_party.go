package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/runhost"
)

// PartyService is the Party lobby port used by the quest scene and its commands.
type PartyService interface {
	CreateParty(ctx context.Context, leaderCharID string, req party.CreatePartyRequest) (party.PartyDetail, error)
	ListParties(ctx context.Context, status string, limit, offset int) (pagination.Page[party.PartySummary], error)
	GetPartySummary(ctx context.Context, partyID string) (party.PartySummary, error)
	JoinParty(ctx context.Context, partyID, characterID, password string) (party.PartyDetail, error)
	LeaveParty(ctx context.Context, partyID, characterID string) error
	KickMember(ctx context.Context, partyID, leaderCharID, targetCharID string) error
}

// WithParty connects the quest scene's lobby list and the lobby commands.
func WithParty(p PartyService) Option {
	return func(h *Handler) {
		h.parties = p
		partyCreateCommand(p)(h)
		partyJoinCommand(p)(h)
		partyLeaveCommand(p)(h)
		partyKickCommand(p)(h)
	}
}

// ＠パーティー form fields (quest.cgi:347–358); 見学可 belongs to #2728.
type partyCreateParams struct {
	Name       string `json:"name"`
	StageID    string `json:"stage_id"`
	Speed      int    `json:"speed"`
	MaxMembers int    `json:"max_members"`
	NeedJoin   string `json:"need_join"`
	Password   string `json:"password"`
}

type partyJoinParams struct {
	PartyID  string `json:"party_id"`
	Password string `json:"password"`
}

type partyLeaveParams struct {
	PartyID string `json:"party_id"`
}

type partyKickParams struct {
	PartyID           string `json:"party_id"`
	TargetCharacterID string `json:"target_character_id"`
}

type partyMessageResponse struct {
	Message string `json:"message"`
}

func partyCreateCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyCreateParams]("party_create", nil, nil)
	}
	return withActionCommand("party_create", func(ctx context.Context, actor string, p partyCreateParams) (any, error) {
		return service.CreateParty(ctx, actor, party.CreatePartyRequest{
			Name: p.Name, StageID: p.StageID, Speed: p.Speed, MaxMembers: p.MaxMembers, NeedJoin: p.NeedJoin, Password: p.Password,
		})
	}, partyLobbyRejection)
}

func partyJoinCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyJoinParams]("party_join", nil, nil)
	}
	return withActionCommand("party_join", func(ctx context.Context, actor string, p partyJoinParams) (any, error) {
		return service.JoinParty(ctx, p.PartyID, actor, p.Password)
	}, partyLobbyRejection)
}

func partyLeaveCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyLeaveParams]("party_leave", nil, nil)
	}
	return withActionCommand("party_leave", func(ctx context.Context, actor string, p partyLeaveParams) (any, error) {
		if err := service.LeaveParty(ctx, p.PartyID, actor); err != nil {
			return nil, err
		}
		return partyMessageResponse{Message: "パーティーから離れました"}, nil
	}, partyLobbyRejection)
}

func partyKickCommand(service PartyService) Option {
	if service == nil {
		return withActionCommand[partyKickParams]("party_kick", nil, nil)
	}
	return withActionCommand("party_kick", func(ctx context.Context, actor string, p partyKickParams) (any, error) {
		if err := service.KickMember(ctx, p.PartyID, actor, p.TargetCharacterID); err != nil {
			return nil, err
		}
		return partyMessageResponse{Message: "メンバーをきっくしました"}, nil
	}, partyLobbyRejection)
}

func partyLobbyRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, party.ErrNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "PARTY_NOT_FOUND", Message: "参加しようとしたクエストは、解散してしまったようです"}
	case errors.Is(err, party.ErrCharacterNotInParty):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_NOT_MEMBER", Message: "Character is not in this party."}
	case errors.Is(err, party.ErrNotPartyLeader):
		return http.StatusForbidden, ErrorDetail{Code: "PARTY_NOT_LEADER", Message: "Only the party leader can do that."}
	case errors.Is(err, party.ErrCannotKickSelf):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_CANNOT_KICK_SELF", Message: "自分をきっくすることはできません"}
	case errors.Is(err, runhost.ErrBusy), errors.Is(err, party.ErrPartyNotRecruiting):
		return http.StatusConflict, ErrorDetail{Code: "PARTY_NOT_RECRUITING", Message: "クエスト途中から参加することはできません"}
	case errors.Is(err, party.ErrPartyFull):
		return http.StatusConflict, ErrorDetail{Code: "PARTY_FULL", Message: "定員がいっぱいで参加することができません"}
	case errors.Is(err, party.ErrInvalidPassword):
		return http.StatusForbidden, ErrorDetail{Code: "PARTY_PASSWORD_MISMATCH", Message: "参加するための合言葉が違います"}
	case errors.Is(err, party.ErrAlreadyInParty), errors.Is(err, party.ErrMemberBusy):
		return http.StatusConflict, ErrorDetail{Code: "PARTY_ACTOR_BUSY", Message: "Character is already in a party or another activity."}
	case errors.Is(err, party.ErrCharacterUnconscious):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_UNCONSCIOUS", Message: "HPを回復してください"}
	case errors.Is(err, party.ErrCharacterExhausted):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_EXHAUSTED", Message: "疲労がたまっています"}
	case errors.Is(err, party.ErrNeedJoinNotMet):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_JOIN_CONDITION", Message: "参加条件を満たしていません"}
	case errors.Is(err, party.ErrInvalidPartyName), errors.Is(err, party.ErrInvalidMaxMembers), errors.Is(err, party.ErrInvalidSpeed), errors.Is(err, party.ErrStageNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "INVALID_ACTION_PARAMS", Message: "Invalid action parameters."}
	case errors.Is(err, adventure.ErrStageNotAvailable), errors.Is(err, adventure.ErrLevelRequirementNotMet), errors.Is(err, adventure.ErrJobLevelRequirementNotMet):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_STAGE_UNAVAILABLE", Message: "冒険場所が異常です"}
	case errors.Is(err, party.ErrDailyOnceAlreadyUsed):
		return http.StatusUnprocessableEntity, ErrorDetail{Code: "PARTY_DAILY_ONCE_USED", Message: err.Error()}
	default:
		return 0, ErrorDetail{}
	}
}
