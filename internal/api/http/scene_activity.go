package http

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/playercontext"
)

// ActivitySceneData retains owned facts and authorized feature projections;
// private game facts remain masked by the owning service.
type ActivitySceneData struct {
	Activities []playercontext.Activity `json:"activities"`
	Casino     *CasinoRoomSceneData     `json:"casino,omitempty"`
}

func (h *Handler) composeActivityScene(ctx context.Context, result playercontext.Result, response *PlayerContextResponse) (bool, error) {
	facts := result.Snapshot.ActiveActivities()
	if len(facts) == 0 {
		return false, nil
	}
	data := ActivitySceneData{Activities: facts}
	for i := range facts {
		if facts[i].Kind == "casino" {
			var err error
			data.Casino, err = h.casinoActivityData(ctx, result, facts[i])
			if err != nil {
				return true, err
			}
		}
		facts[i].Actions = slices.DeleteFunc(facts[i].Actions, func(id string) bool { return !slices.Contains(result.AvailableActions, id) })
	}
	titles := map[string]string{"sleep": "休息中", "scheduled": "実行待ち", "party": "パーティー", "pvp": "対戦部屋", "gvg": "ギルド戦", "dungeon": "ダンジョン探索", "challenge": "試練", "casino": "カジノ", "activity_conflict": "活動の状態を確認してください"}
	kind := result.Snapshot.LocationID
	scene := SceneSnapshot{Kind: "activity", LocationID: kind, Title: titles[kind], Dialogue: "現在の活動を確認してください。", Data: data, Support: SceneSupport{Observation: "details", Actions: []SceneActionSupport{}}}
	if kind == "activity_conflict" {
		scene.Kind = "activity_conflict"
	}
	for _, def := range playercontext.DefaultCatalog {
		eligible := slices.Contains(result.AvailableActions, def.ID)
		if def.ActivityKind == "" && def.ID != "home_wake" && def.ID != "rescue_request" {
			continue
		}
		if !eligible {
			continue
		}
		connected := h.actionCommands[def.ID].prepare != nil
		mode := "continuation"
		if def.Recovery || def.ID == "home_wake" || def.ID == "rescue_request" {
			mode = "recovery"
		}
		scene.Support.Actions = append(scene.Support.Actions, SceneActionSupport{Action: def.ID, Connected: connected, Mode: mode, EntryEligible: false})
		if !connected {
			continue
		}
		template := make(map[string]any)
		for _, a := range facts {
			if a.Kind != def.ActivityKind {
				continue
			}
			for _, field := range []string{"room_id", "party_id", "session_id"} {
				if slices.Contains(def.RequiredParams, field) {
					template[field] = a.ID
				}
			}
		}
		action, err := h.contextAction(def.ID, template)
		if err != nil {
			return true, err
		}
		response.AvailableActions = append(response.AvailableActions, action)
	}
	response.Scene = scene
	return true, nil
}
