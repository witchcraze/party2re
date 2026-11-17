package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/character"
	"github.com/witchcraze/party2re/internal/playercontext"
)

type AdventureRecordService interface {
	OpenOwnRecord(context.Context, string, string) (adventure.RecordEntryResult, error)
}

func WithAdventureRecord(service AdventureRecordService) Option {
	return func(h *Handler) { h.recordEntry = service }
}

// ＠ぷろふぃーる and ＠ぼうけんのきろく are offered in every home and show the
// observed owner (home.cgi:26-39,164-178); only the owner edits the profile
// (profile.cgi:40-47), so visitors are offered no command.
var (
	homeProfileFacility = facility{title: "プロフィール", commands: []string{"home_profile_update"}, subjectCommands: []string{},
		ownedSubjectCommands: []string{"home_profile_update"}, read: (*Handler).homeProfileSceneData}
	homeAdventureRecordFacility = facility{title: "冒険の記録", read: (*Handler).homeAdventureRecordSceneData}
)

// homeProfileLabels are the legacy profile form captions (profile.cgi:6-30).
var homeProfileLabels = map[string]string{
	"name": "名前", "sex": "性別", "blood": "血液型", "birthday": "誕生日", "age": "年齢", "job": "職業",
	"address": "住んでいる所", "hobby": "趣味", "boom": "マイブーム", "site": "オススメサイト", "dream": "夢/目標",
	character.PersonalityBioKey: "性格・特徴", "like": "好きなもの", "dislike": "嫌いなもの", "like_food": "好きな食べ物",
	"dislike_food": "嫌いな食べ物", "goal": "このゲームの目標", "login": "ログイン時間", "boast": "自慢",
	"reference": "このサイトを知ったきっかけ", "message": "何か一言",
}

type HomeProfileField struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

type HomeProfileSceneData struct {
	Parent  string `json:"parent"`
	OwnerID string `json:"owner_id"`
	// Fields lists every legacy profile field in form order, empty ones included.
	Fields []HomeProfileField `json:"fields"`
}

type HomeAdventureRecordStage struct {
	StageID       string `json:"stage_id"`
	StageName     string `json:"stage_name"`
	SeasonalSubID int    `json:"seasonal_sub_id,omitempty"`
	Season        string `json:"season,omitempty"`
	ClearCount    int    `json:"clear_count"`
}

type HomeAdventureRecordSceneData struct {
	Parent  string `json:"parent"`
	OwnerID string `json:"owner_id"`
	// Stages are the cleared stages, as legacy records only clears.
	Stages      []HomeAdventureRecordStage `json:"stages"`
	TotalClears int                        `json:"total_clears"`
}

func (h *Handler) homeProfileSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	owner := homeOwner(r.Navigation, r.Snapshot.Character.ID)
	view, err := h.characters.GetProfile(ctx, owner)
	if err != nil {
		return nil, err
	}
	data := HomeProfileSceneData{Parent: "home", OwnerID: owner, Fields: make([]HomeProfileField, 0, len(character.ProfileFields))}
	for _, key := range character.ProfileFields {
		data.Fields = append(data.Fields, HomeProfileField{Key: key, Label: homeProfileLabels[key], Value: view.Profile.BioData[key]})
	}
	return data, nil
}

func (h *Handler) homeAdventureRecordSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	owner := homeOwner(r.Navigation, r.Snapshot.Character.ID)
	chronicle, err := h.adventures.GetChronicle(ctx, owner)
	if err != nil {
		return nil, err
	}
	data := HomeAdventureRecordSceneData{Parent: "home", OwnerID: owner, Stages: []HomeAdventureRecordStage{}}
	for _, s := range chronicle.Stages {
		if s.ClearCount > 0 {
			data.Stages = append(data.Stages, HomeAdventureRecordStage{StageID: s.StageID, StageName: s.StageName, SeasonalSubID: s.SeasonalSubID, Season: s.Season, ClearCount: s.ClearCount})
			data.TotalClears += s.ClearCount
		}
	}
	return data, nil
}

type homeProfileUpdateRequest struct {
	BioData map[string]string `json:"bio_data"`
}

func homeProfileCommand(characters CharacterService) Option {
	return withActionCommand("home_profile_update", func(ctx context.Context, actor string, p homeProfileUpdateRequest) (any, error) {
		return characters.UpdateProfile(ctx, actor, p.BioData)
	}, homeProfileRejection)
}

func homeProfileRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, character.ErrUnknownBioField):
		return http.StatusBadRequest, ErrorDetail{Code: "PROFILE_UNKNOWN_FIELD", Message: "Use only the profile form fields."}
	case errors.Is(err, character.ErrBioValueTooLong):
		return http.StatusBadRequest, ErrorDetail{Code: "PROFILE_FIELD_TOO_LONG", Message: "Each profile field takes up to 160 characters."}
	case errors.Is(err, character.ErrBioValueInvalid), isTextValidationError(err):
		return http.StatusBadRequest, ErrorDetail{Code: "PROFILE_FIELD_INVALID", Message: "A profile field contains characters that cannot be used."}
	default:
		return 0, ErrorDetail{}
	}
}
