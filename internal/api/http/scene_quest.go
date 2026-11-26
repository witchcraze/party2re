package http

import (
	"context"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// QuestStageLister supplies the ＠つくる form's stage choices for the actor.
type QuestStageLister interface {
	AvailableStages(corecharacter.Character) []adventure.Stage
}

// WithQuestStages connects the quest scene's stage choices.
func WithQuestStages(stages QuestStageLister) Option {
	return func(h *Handler) { h.questStages = stages }
}

type QuestStage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type QuestParty struct {
	party.PartySummary
	SelectParams playercontext.Subject `json:"select_params"`
}

// QuestSceneData is the quest place (quest.cgi): creatable stages and waiting lobbies.
type QuestSceneData struct {
	Parent  string       `json:"parent"`
	Stages  []QuestStage `json:"stages"`
	Parties []QuestParty `json:"parties"`
	Page    ScenePage    `json:"page"`
}

type QuestPartyParams struct {
	PartyID string `json:"party_id"`
}

// QuestSelectedSceneData shows one lobby's public row; joining is an explicit command.
type QuestSelectedSceneData struct {
	Parent     string             `json:"parent"`
	Party      party.PartySummary `json:"party"`
	JoinParams QuestPartyParams   `json:"join_params"`
}

func (h *Handler) questSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.parties == nil || h.questStages == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	if n.Subject != (playercontext.Subject{}) {
		// shortcut: scans the first 100 lobbies; read the lobby by ID if more can wait at once.
		page, err := h.parties.ListParties(ctx, party.StatusRecruiting, 100, 0)
		if err != nil {
			return nil, err
		}
		for _, p := range page.Items {
			if p.ID == n.Subject.ID {
				return QuestSelectedSceneData{Parent: "town", Party: p, JoinParams: QuestPartyParams{PartyID: p.ID}}, nil
			}
		}
		return nil, playercontext.ErrSelectionNotFound
	}
	limit := n.Limit
	if limit == 0 {
		limit = 20
	}
	page, err := h.parties.ListParties(ctx, party.StatusRecruiting, limit, n.Offset)
	if err != nil {
		return nil, err
	}
	scenePage := ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}
	if end := n.Offset + len(page.Items); end < page.Total {
		scenePage.Next = &playercontext.PageParams{Destination: "quest", Offset: end, Limit: limit}
	}
	rows := make([]QuestParty, 0, len(page.Items))
	for _, p := range page.Items {
		rows = append(rows, QuestParty{PartySummary: p, SelectParams: playercontext.Subject{Kind: "party", ID: p.ID}})
	}
	available := h.questStages.AvailableStages(r.Snapshot.Character)
	stages := make([]QuestStage, 0, len(available))
	for _, st := range available {
		stages = append(stages, QuestStage{ID: st.ID, Name: st.Name})
	}
	return QuestSceneData{Parent: "town", Stages: stages, Parties: rows, Page: scenePage}, nil
}

// questFacility is 冒険に出る (config.cgi @places, lib/quest.cgi): solo and
// Party quests are created here, and waiting lobbies are joined from its list.
var questFacility = facility{
	title: "冒険中のパーティー", commands: []string{"adventure_start", "party_create"},
	subjectCommands: []string{"party_join"},
	bind: func(n playercontext.Selection, _ string) map[string]map[string]any {
		return map[string]map[string]any{"party_join": {"party_id": n.Subject.ID}}
	},
	read: (*Handler).questSceneData, placeLog: true,
	present: func(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
		switch data := data.(type) {
		case QuestSceneData:
			return scenePresentation{next: data.Page.Next}
		case QuestSelectedSceneData:
			presentation := scenePresentation{title: &data.Party.Name}
			if data.Party.CurrentMembers >= data.Party.MaxMembers {
				presentation.hide = []string{"party_join"}
			}
			return presentation
		}
		return scenePresentation{}
	},
}
