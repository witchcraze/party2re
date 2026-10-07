package http

import (
	"context"
	"errors"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// WithPlayerContext configures the existing observation query service.
func WithPlayerContext(service *playercontext.Service) Option {
	return func(h *Handler) {
		h.playerContext = service
		h.registerNavigation(service)
	}
}

// PlayerContextResponse is the shared observation DTO for GET /context and the
// action gateway. Arrays are always present, including when empty.
type PlayerContextResponse struct {
	Character        CharacterSnapshot       `json:"character"`
	Scene            SceneSnapshot           `json:"scene"`
	OngoingActions   []OngoingActionSnapshot `json:"ongoing_actions"`
	AvailableActions []ContextAction         `json:"available_actions"`
}

type CharacterSnapshot struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	JobID      string `json:"job_id"`
	JobName    string `json:"job_name"`
	Level      int    `json:"level"`
	HP         int    `json:"hp"`
	MaxHP      int    `json:"max_hp"`
	MP         int    `json:"mp"`
	MaxMP      int    `json:"max_mp"`
	Gold       int    `json:"gold"`
	Tired      int    `json:"tired"`
	IsDead     bool   `json:"is_dead"`
	IsSleeping bool   `json:"is_sleeping"`
	Icon       string `json:"icon"`
	IconURL    string `json:"icon_url"`
}

type SceneSnapshot struct {
	Navigation *playercontext.NavigationObservation `json:"navigation,omitempty"`
	LocationID string                               `json:"location_id"`
	Title      string                               `json:"title"`
	Bgimg      string                               `json:"bgimg"`
	BgimgURL   string                               `json:"bgimg_url"`
	Dialogue   string                               `json:"dialogue"`
	Speaker    *SceneSpeaker                        `json:"speaker,omitempty"`
	Opponent   *SceneOpponent                       `json:"opponent,omitempty"`
}

type SceneSpeaker struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
}

// SceneOpponent reserves structured opponent facts for the combat migration.
type SceneOpponent struct {
	Name    string `json:"name"`
	IconURL string `json:"icon_url"`
	HP      int    `json:"hp"`
	MaxHP   int    `json:"max_hp"`
}

type OngoingActionSnapshot struct {
	ID               string    `json:"id"`
	ActionType       string    `json:"action_type"`
	Label            string    `json:"label"`
	ExecuteAt        time.Time `json:"execute_at"`
	RemainingSeconds int64     `json:"remaining_seconds"`
	IsReady          bool      `json:"is_ready"`
}

type ContextAction struct {
	Action         string   `json:"action"`
	Label          string   `json:"label"`
	Category       string   `json:"category"`
	Style          string   `json:"style"`
	RequiredParams []string `json:"required_params"`
}

func (h *Handler) handleGetPlayerContext(w http.ResponseWriter, r *http.Request) {
	h.withAuthenticatedCharacter(w, r, r.PathValue("id"), func(player coreplayer.Player, char corecharacter.Character) {
		if h.playerContext == nil {
			writeError(w, http.StatusNotImplemented, errors.New("player context service not configured"))
			return
		}
		result, err := h.playerContext.Query(r.Context(), char.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		if result.Snapshot.Character.PlayerID != player.ID {
			writeError(w, http.StatusForbidden, errors.New("forbidden: character belongs to another player"))
			return
		}
		response, err := h.playerContextResponse(r.Context(), result, time.Now().UTC())
		if err != nil {
			writeError(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, response)
	})
}

// playerContextResponse enriches read facts with HTTP-owned presentation. It
// performs no mutations and never treats an eligible entry as command approval.
// Profile read errors are propagated to callers so GET /context yields 500 and
// command gateway refresh yields CONTEXT_REFRESH_FAILED instead of defaulting.
func (h *Handler) playerContextResponse(ctx context.Context, result playercontext.Result, now time.Time) (PlayerContextResponse, error) {
	s := result.Snapshot
	c := s.Character
	profile, err := h.characters.GetProfile(ctx, c.ID)
	if err != nil {
		return PlayerContextResponse{}, err
	}
	if profile.Character.PlayerID != c.PlayerID {
		return PlayerContextResponse{}, errors.New("context ownership changed during profile read")
	}
	response := PlayerContextResponse{
		Character: CharacterSnapshot{ID: c.ID, Name: c.Name, JobID: c.JobID, Level: c.Level,
			HP: c.Stats.HP, MaxHP: c.Stats.MaxHP, MP: c.Stats.MP, MaxMP: c.Stats.MaxMP,
			Gold: c.Money, Tired: c.Tired, IsDead: c.Stats.HP <= 0, IsSleeping: s.Sleeping, IconURL: profile.Profile.AvatarURL},
		Scene: SceneSnapshot{Navigation: result.Navigation, LocationID: s.LocationID, Title: "始まりの街", Bgimg: "bg_town",
			BgimgURL: "data:image/svg+xml," + url.PathEscape(townBackgroundPlaceholder),
			Dialogue: "広場へようこそ。次の行動を選んでください。", Speaker: &SceneSpeaker{Name: "広場の案内人"}},
		OngoingActions:   make([]OngoingActionSnapshot, 0, len(s.OngoingActions)+1),
		AvailableActions: make([]ContextAction, 0, len(result.AvailableActions)),
	}
	if h.jobs != nil {
		for _, job := range h.jobs.ListDefinitions() {
			if job.ID == c.JobID {
				response.Character.JobName = job.Name
				break
			}
		}
	}
	for _, action := range s.OngoingActions {
		response.OngoingActions = append(response.OngoingActions, ongoingAction(action.ID, action.ActionType, "実行待ち", action.ExecuteAt, now))
	}
	if s.Sleeping {
		// Sleep locks are not ScheduledActions. Include pending wake recovery even
		// after the duration lock expires; waking remains an explicit command.
		response.OngoingActions = append(response.OngoingActions, ongoingAction("sleep", "home_sleep", "休息中", now.Add(max(s.SleepRemaining, 0)), now))
	}
	slices.SortFunc(response.OngoingActions, func(a, b OngoingActionSnapshot) int {
		if cmp := a.ExecuteAt.Compare(b.ExecuteAt); cmp != 0 {
			return cmp
		}
		return strings.Compare(a.ID, b.ID)
	})
	for _, id := range result.AvailableActions {
		for _, def := range playercontext.DefaultCatalog {
			if def.ID != id {
				continue
			}
			style := "secondary"
			if def.Category == "adventure" {
				style = "primary"
			}
			response.AvailableActions = append(response.AvailableActions, ContextAction{
				Action: id, Label: def.Label, Category: def.Category, Style: style,
				RequiredParams: append([]string{}, def.RequiredParams...),
			})
			break
		}
	}
	return response, nil
}

func ongoingAction(id, actionType, label string, deadline, now time.Time) OngoingActionSnapshot {
	return OngoingActionSnapshot{ID: id, ActionType: actionType, Label: label, ExecuteAt: deadline,
		RemainingSeconds: int64(math.Ceil(max(deadline.Sub(now).Seconds(), 0))), IsReady: !deadline.After(now)}
}

// Self-authored neutral placeholder; production art resolution belongs to
// #654/#729. No reference-game images are reused.
const townBackgroundPlaceholder = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 800 400"><path fill="#dbeafe" d="M0 0h800v400H0z"/><path fill="#bbf7d0" d="M0 260h800v140H0z"/><path fill="#e2e8f0" d="M300 400l60-180h80l60 180z"/><path fill="#94a3b8" d="M100 180h150v100H100zm450 0h150v100H550z"/><path fill="#64748b" d="M80 180l95-70 95 70zm450 0l95-70 95 70z"/></svg>`
