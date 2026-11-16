package http

import (
	"context"
	"errors"
	"net/http"

	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/customskill"
	"github.com/witchcraze/party2re/internal/gemstore"
	jobapp "github.com/witchcraze/party2re/internal/job"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// Home record views (home.cgi aitemuzukan/monster_book/job_master) show the
// observed home owner's records to owner and visitors alike; ＠かすたむすきる
// is offered only in one's own home. Every list is bounded by its catalog.
var (
	homeRecordScenes        = []string{"home_collection", "home_monster_book", "home_job_master"}
	homeCollectionFacility  = facility{title: "アイテム図鑑", read: (*Handler).homeCollectionSceneData}
	homeMonsterBookFacility = facility{title: "モンスターブック", read: (*Handler).homeMonsterBookSceneData}
	homeJobMasterFacility   = facility{title: "ジョブマスター", read: (*Handler).homeJobMasterSceneData}
	homeCustomSkillFacility = facility{title: "カスタムスキル", commands: []string{"custom_skill_set"}, read: (*Handler).homeCustomSkillSceneData}
)

// CollectionSection is one completion-tracked list of the item collection.
type CollectionSection struct {
	Entries  []collection.ItemCollectionEntry `json:"entries"`
	Progress collection.CompletionProgress    `json:"progress"`
}

type HomeCollectionSceneData struct {
	Parent  string            `json:"parent"`
	OwnerID string            `json:"owner_id"`
	Weapons CollectionSection `json:"weapons"`
	Armors  CollectionSection `json:"armors"`
	Items   CollectionSection `json:"items"`
}

type HomeMonsterBookSceneData struct {
	Parent   string                        `json:"parent"`
	OwnerID  string                        `json:"owner_id"`
	Entries  []collection.MonsterBookEntry `json:"entries"`
	Progress collection.CompletionProgress `json:"progress"`
}

type HomeJobMasterSceneData struct {
	Parent  string                     `json:"parent"`
	Mastery jobapp.CharacterJobMastery `json:"mastery"`
}

type HomeCustomSkillSceneData struct {
	Parent string `json:"parent"`
	// CustomSkill is null until the owner first sets one.
	CustomSkill *customskill.CustomSkill `json:"custom_skill"`
}

func (h *Handler) homeCollectionSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.collections == nil {
		return nil, errSceneNotConfigured
	}
	owner := homeOwner(r.Navigation, r.Snapshot.Character.ID)
	data := HomeCollectionSceneData{Parent: "home", OwnerID: owner}
	for _, section := range []struct {
		into *CollectionSection
		read func(context.Context, string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
	}{
		{&data.Weapons, h.collections.GetWeaponCollection},
		{&data.Armors, h.collections.GetArmorCollection},
		{&data.Items, func(ctx context.Context, id string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error) {
			return h.collections.GetItemCollection(ctx, id, "item")
		}},
	} {
		entries, progress, err := section.read(ctx, owner)
		if err != nil {
			return nil, err
		}
		*section.into = CollectionSection{Entries: append([]collection.ItemCollectionEntry{}, entries...), Progress: progress}
	}
	return data, nil
}

func (h *Handler) homeMonsterBookSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.collections == nil {
		return nil, errSceneNotConfigured
	}
	owner := homeOwner(r.Navigation, r.Snapshot.Character.ID)
	entries, progress, err := h.collections.GetMonsterBook(ctx, owner)
	if err != nil {
		return nil, err
	}
	return HomeMonsterBookSceneData{Parent: "home", OwnerID: owner, Entries: append([]collection.MonsterBookEntry{}, entries...), Progress: progress}, nil
}

func (h *Handler) homeJobMasterSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.jobs == nil {
		return nil, errSceneNotConfigured
	}
	mastery, err := h.jobs.GetJobMastery(ctx, homeOwner(r.Navigation, r.Snapshot.Character.ID))
	if err != nil {
		return nil, err
	}
	if mastery.Jobs == nil {
		mastery.Jobs = []jobapp.JobProgress{}
	}
	return HomeJobMasterSceneData{Parent: "home", Mastery: mastery}, nil
}

// homeCustomSkillSceneData always observes the actor, as custom_skill_set does.
func (h *Handler) homeCustomSkillSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.customSkills == nil {
		return nil, errSceneNotConfigured
	}
	skill, err := h.customSkills.GetCustomSkill(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	return HomeCustomSkillSceneData{Parent: "home", CustomSkill: skill}, nil
}

type CustomSkillService interface {
	GetCustomSkill(ctx context.Context, characterID string) (*customskill.CustomSkill, error)
	SetCustomSkill(ctx context.Context, characterID, name, comment string, gems [3]string) (*customskill.CustomSkill, error)
}

type customSkillSetRequest struct {
	Name    string    `json:"name"`
	Comment string    `json:"comment"`
	Gems    [3]string `json:"gems"`
}

// WithCustomSkill configures the custom skill view and its custom_skill_set command.
func WithCustomSkill(skills CustomSkillService) Option {
	return func(h *Handler) {
		h.customSkills = skills
		if skills == nil {
			withActionCommand[customSkillSetRequest]("custom_skill_set", nil, nil)(h)
			return
		}
		withActionCommand("custom_skill_set", func(ctx context.Context, actor string, p customSkillSetRequest) (any, error) {
			return skills.SetCustomSkill(ctx, actor, p.Name, p.Comment, p.Gems)
		}, customSkillRejection)(h)
	}
}

func customSkillRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, customskill.ErrInvalidSkillName):
		return http.StatusBadRequest, ErrorDetail{Code: "CUSTOM_SKILL_INVALID_NAME", Message: "Enter a valid skill name."}
	case errors.Is(err, customskill.ErrInvalidSkillComment):
		return http.StatusBadRequest, ErrorDetail{Code: "CUSTOM_SKILL_INVALID_COMMENT", Message: "Enter a valid skill line."}
	case errors.Is(err, customskill.ErrTooManyGemSlots):
		return http.StatusBadRequest, ErrorDetail{Code: "CUSTOM_SKILL_TOO_MANY_SLOTS", Message: "The gems need more than 3 slots."}
	case errors.Is(err, customskill.ErrGemNotOwned), errors.Is(err, customskill.ErrGemNotFound):
		return http.StatusBadRequest, ErrorDetail{Code: "CUSTOM_SKILL_GEM_UNAVAILABLE", Message: "Choose gems from your gem box."}
	case errors.Is(err, gemstore.ErrGemBoxFull):
		return http.StatusConflict, ErrorDetail{Code: "GEM_BOX_FULL", Message: "The gem box cannot take the replaced gems back."}
	default:
		return 0, ErrorDetail{}
	}
}

// CollectionService reads a character's monster book and item collections.
type CollectionService interface {
	GetMonsterBook(ctx context.Context, characterID string) ([]collection.MonsterBookEntry, collection.CompletionProgress, error)
	GetItemCollection(ctx context.Context, characterID, category string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
	GetWeaponCollection(ctx context.Context, characterID string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
	GetArmorCollection(ctx context.Context, characterID string) ([]collection.ItemCollectionEntry, collection.CompletionProgress, error)
}

// WithCollection configures the collection reads of the Home record views and trajectory.
func WithCollection(c CollectionService) Option {
	return func(h *Handler) { h.collections = c }
}
