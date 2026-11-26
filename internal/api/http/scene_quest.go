package http

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/adventure"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
	"github.com/witchcraze/party2re/internal/runhost"
)

// QuestStageLister supplies the ＠つくる form's stage choices for the actor
// and the stage names of listed runs.
type QuestStageLister interface {
	AvailableStages(corecharacter.Character) []adventure.Stage
	FindByID(string) (adventure.Stage, error)
}

// WithQuestStages connects the quest scene's stage choices.
func WithQuestStages(stages QuestStageLister) Option {
	return func(h *Handler) { h.questStages = stages }
}

// QuestRunLister lists in-progress normal runs for the quest list.
type QuestRunLister interface {
	ListQuestRooms(context.Context) ([]runhost.QuestRoom, error)
}

// WithQuestRuns connects the quest scene's in-progress normal runs.
func WithQuestRuns(runs QuestRunLister) Option {
	return func(h *Handler) { h.questRuns = runs }
}

type QuestStage struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// QuestRun is an in-progress normal run's public row.
type QuestRun struct {
	runhost.QuestRoom
	StageName string `json:"stage_name"`
}

type QuestParty struct {
	party.PartySummary
	SelectParams playercontext.Subject `json:"select_params"`
}

// QuestSceneData is the quest place (quest.cgi): creatable stages, waiting
// Party lobbies, in-progress normal runs and PvP/GvG waiting rooms.
type QuestSceneData struct {
	Parent       string            `json:"parent"`
	Stages       []QuestStage      `json:"stages"`
	Parties      []QuestParty      `json:"parties"`
	Runs         []QuestRun        `json:"runs"`
	Arenas       []pvp.RoomSummary `json:"arenas"`
	GuildBattles []gvg.RoomSummary `json:"guild_battles"`
	Page         ScenePage         `json:"page"`
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
		p, err := h.parties.GetPartySummary(ctx, n.Subject.ID)
		if errors.Is(err, party.ErrNotFound) {
			return nil, playercontext.ErrSelectionNotFound
		}
		if err != nil {
			return nil, err
		}
		return QuestSelectedSceneData{Parent: "town", Party: p, JoinParams: QuestPartyParams{PartyID: p.ID}}, nil
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
	data := QuestSceneData{Parent: "town", Stages: stages, Parties: rows, Runs: []QuestRun{}, Arenas: []pvp.RoomSummary{}, GuildBattles: []gvg.RoomSummary{}, Page: scenePage}
	// Only Party lobbies page; the other rooms are few and listed on the first page.
	if n.Offset == 0 {
		if err := h.questOtherRooms(ctx, &data); err != nil {
			return nil, err
		}
	}
	return data, nil
}

// questOtherRooms adds the room kinds other than Party lobbies (quest.cgi:80–170).
// Their entry is not a Gateway command yet, so the scene offers none for them.
func (h *Handler) questOtherRooms(ctx context.Context, data *QuestSceneData) error {
	if h.questRuns != nil {
		rooms, err := h.questRuns.ListQuestRooms(ctx)
		if err != nil {
			return err
		}
		for _, room := range rooms {
			run := QuestRun{QuestRoom: room}
			if st, err := h.questStages.FindByID(room.StageID); err == nil {
				run.StageName = st.Name
			}
			data.Runs = append(data.Runs, run)
		}
	}
	if h.pvp != nil {
		rooms, err := h.pvp.ListRooms(ctx)
		if err != nil {
			return err
		}
		data.Arenas = append(data.Arenas, rooms...)
	}
	if h.gvg != nil {
		rooms, err := h.gvg.ListRooms(ctx)
		if err != nil {
			return err
		}
		data.GuildBattles = append(data.GuildBattles, rooms...)
	}
	return nil
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
