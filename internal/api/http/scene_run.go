package http

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/runhost"
)

// RunSceneData is the actor's owned run observation: public encounter facts,
// its own ledger and the operations it may choose. Only the actor's ledger is
// exposed; other members show battle facts every room member sees.
type RunSceneData struct {
	RunID      string               `json:"run_id"`
	Version    int                  `json:"version"`
	Round      int                  `json:"round"`
	LeaderID   string               `json:"leader_id"`
	Speed      int                  `json:"speed"`
	ReadyAt    time.Time            `json:"ready_at"`
	IdleAt     time.Time            `json:"idle_at"`
	ServerTime time.Time            `json:"server_time"`
	Actors     []RunActorView       `json:"actors"`
	Boxes      []RunBoxView         `json:"boxes"`
	Operations []RunOperationView   `json:"operations"`
	Recipient  RunRecipientResponse `json:"recipient"`
}

type RunActorView struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	TeamID string `json:"team_id"`
	HP     int    `json:"hp"`
	MaxHP  int    `json:"max_hp"`
	MP     int    `json:"mp"`
	MaxMP  int    `json:"max_mp"`
	NPC    bool   `json:"npc"`
	Enemy  bool   `json:"enemy"`
	Leader bool   `json:"leader"`
}

// RunBoxView names an unopened box's content only after someone opens it.
type RunBoxView struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	OpenedBy string `json:"opened_by,omitempty"`
	ItemName string `json:"item_name,omitempty"`
	Defeated bool   `json:"defeated"`
}

// RunOperationView is one discovered operation. Submittable ones are the
// run_operation/run_chest/run_exit inputs; the rest stay visible but belong
// to other owners or are gated in the current state.
type RunOperationView struct {
	Operation   string `json:"operation"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	MPCost      int    `json:"mp_cost"`
	Submittable bool   `json:"submittable"`
}

func (h *Handler) runActivityData(ctx context.Context, result playercontext.Result) (*RunSceneData, error) {
	if h.runs == nil {
		return nil, nil
	}
	c := result.Snapshot.Character
	obs, err := h.runs.ObserveRun(ctx, c.PlayerID, c.ID)
	if err != nil || obs == nil {
		return nil, err
	}
	data := &RunSceneData{RunID: obs.RunID, Version: obs.Version, Round: obs.Round, LeaderID: obs.LeaderID, Speed: obs.Speed,
		ReadyAt: obs.ReadyAt.UTC(), IdleAt: obs.IdleAt.UTC(), ServerTime: obs.Now.UTC(),
		Actors: []RunActorView{}, Boxes: []RunBoxView{}, Operations: []RunOperationView{},
		Recipient: runRecipient(obs.Recipient)}
	for _, a := range obs.Actors {
		data.Actors = append(data.Actors, RunActorView{ID: a.ID, Name: a.Name, TeamID: a.TeamID, HP: a.HP, MaxHP: a.MaxHP, MP: a.MP, MaxMP: a.MaxMP, NPC: a.NPC, Enemy: a.Enemy, Leader: a.Leader})
	}
	for _, b := range obs.Boxes {
		data.Boxes = append(data.Boxes, RunBoxView{ID: b.ID, Name: b.Name, OpenedBy: b.OpenedBy, ItemName: b.ItemName, Defeated: b.Defeated})
	}
	for _, o := range obs.Choices {
		data.Operations = append(data.Operations, RunOperationView{Operation: o.Action, Name: o.Name, Kind: string(o.Kind), MPCost: o.MPCost, Submittable: o.Submittable})
	}
	return data, nil
}

// runInputEnums narrows the offered run command inputs to the observed
// choices and encounter. Execution revalidates them against the locked run.
func (d *RunSceneData) runInputEnums(id string) map[string][]string {
	if d == nil {
		return nil
	}
	var operations, boxes, targets []string
	for _, o := range d.Operations {
		if o.Submittable && o.Operation != "chest" && o.Operation != "exit" && !slices.Contains(operations, o.Operation) {
			operations = append(operations, o.Operation)
		}
	}
	for _, a := range d.Actors {
		targets = append(targets, a.ID)
	}
	for _, b := range d.Boxes {
		if b.OpenedBy == "" && !b.Defeated {
			boxes = append(boxes, b.ID)
			targets = append(targets, b.ID)
		}
	}
	switch id {
	case "run_operation":
		return map[string][]string{"operation": operations, "second_operation": operations, "follow_up_operation": operations,
			"target_id": targets, "second_target_id": targets, "follow_up_target_id": targets}
	case "run_chest":
		return map[string][]string{"box_id": boxes}
	}
	return nil
}

// withEnums restricts string properties of an offered schema.
func withEnums(action ContextAction, enums map[string][]string) (ContextAction, error) {
	if len(enums) == 0 {
		return action, nil
	}
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(action.ParamsSchema, &schema); err != nil {
		return action, err
	}
	var properties map[string]map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &properties); err != nil {
		return action, err
	}
	for name, values := range enums {
		property, ok := properties[name]
		if !ok {
			continue
		}
		encoded, err := json.Marshal(append([]string{}, values...))
		if err != nil {
			return action, err
		}
		property["enum"] = encoded
	}
	var err error
	if schema["properties"], err = json.Marshal(properties); err != nil {
		return action, err
	}
	action.ParamsSchema, err = json.Marshal(schema)
	return action, err
}

func runRecipient(p runhost.Recipient) RunRecipientResponse {
	return RunRecipientResponse{Pending: p.Pending, Conditional: p.Conditional, Applied: p.Applied, Inspected: p.Inspected}
}
