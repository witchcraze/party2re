package http

import (
	"context"
	"errors"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/pagination"
)

// GuildReader defines read operations for guilds.
type GuildReader interface {
	Get(ctx context.Context, guildID string) (guild.Detail, error)
	GetByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	List(ctx context.Context, offset, limit int) ([]guild.Guild, error)
}

// GuildManager defines member and application operations for guilds.
type GuildManager interface {
	Create(ctx context.Context, creatorCharID string, name string) (guild.Guild, guild.Member, corecharacter.Character, error)
	ApplyToJoin(ctx context.Context, guildID string, applicantID string) error
	ApproveApplication(ctx context.Context, guildID string, leaderID string, applicantID string, title string) error
	RejectApplication(ctx context.Context, guildID string, leaderID string, applicantID string) error
	BroadcastCallout(ctx context.Context, guildID string, senderID string, message string) error
	AssignCustomRole(ctx context.Context, guildID string, requesterCharID string, targetCharID string, title string) error
	Kick(ctx context.Context, guildID string, requesterCharID string, targetCharID string) error
}

// GuildCustomizer defines styling and customization operations for guilds.
type GuildCustomizer interface {
	UpdateColor(ctx context.Context, guildID string, requesterCharID string, color string) error
	UpdateNotice(ctx context.Context, guildID string, requesterCharID string, notice string) error
	ChangeMark(ctx context.Context, guildID string, leaderID string, mark string) (corecharacter.Character, error)
	ChangeWallpaper(ctx context.Context, guildID string, leaderID string, wallpaper string) (corecharacter.Character, error)
}

// GuildLifecycle defines lifecycle operations for guilds.
type GuildLifecycle interface {
	Disband(ctx context.Context, guildID string, leaderCharID string) error
	Leave(ctx context.Context, guildID string, characterID string) error
}

// GuildService composes all guild operations exposed over HTTP.
type GuildService interface {
	GuildReader
	GuildManager
	GuildCustomizer
	GuildLifecycle
}

// WithGuild configures the guild service for the Handler.
func WithGuild(g GuildService) Option {
	return func(h *Handler) {
		h.guild = g
	}
}

// -------------------------------------------------------------------
// Request & Response Types
// -------------------------------------------------------------------

type createGuildRequest struct {
	CharacterID string `json:"character_id"`
	Name        string `json:"name"`
}

type createGuildResponse struct {
	Guild  guild.Guild  `json:"guild"`
	Member guild.Member `json:"member"`
}

type guildCharacterResponse struct {
	Guild  guild.Guild  `json:"guild"`
	Member guild.Member `json:"member"`
}

type guildActionCharacterRequest struct {
	CharacterID string `json:"character_id"`
}

type approveGuildApplicationRequest struct {
	CharacterID string `json:"character_id"`
	Title       string `json:"title,omitempty"`
}

type assignGuildRoleTitleRequest struct {
	CharacterID string `json:"character_id"`
	Title       string `json:"title"`
}

type broadcastGuildCalloutRequest struct {
	CharacterID string `json:"character_id"`
	Message     string `json:"message"`
}

type customizeGuildRequest struct {
	CharacterID string `json:"character_id"`
	Color       string `json:"color,omitempty"`
	Mark        string `json:"mark,omitempty"`
	Wallpaper   string `json:"wallpaper,omitempty"`
	Notice      string `json:"notice,omitempty"`
}

type guildMessageResponse struct {
	Message string `json:"message"`
}

// -------------------------------------------------------------------
// HTTP Handlers
// -------------------------------------------------------------------

// handleListGuilds handles GET /guilds
func (h *Handler) handleListGuilds(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	params := pagination.ParseRequestWithDefaults(r, 20, 100)
	guilds, err := h.guild.List(r.Context(), params.Offset, params.Limit)
	if err != nil {
		h.writeGuildError(w, err)
		return
	}
	if guilds == nil {
		guilds = []guild.Guild{}
	}

	writeJSON(w, http.StatusOK, guilds)
}

// handleGetGuild handles GET /guilds/{id}
func (h *Handler) handleGetGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	detail, err := h.guild.Get(r.Context(), guildID)
	if err != nil {
		h.writeGuildError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, detail)
}

// handleGetCharacterGuild handles GET /characters/{id}/guild
func (h *Handler) handleGetCharacterGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	charID := r.PathValue("id")
	g, m, err := h.guild.GetByCharacter(r.Context(), charID)
	if err != nil {
		h.writeGuildError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, guildCharacterResponse{
		Guild:  g,
		Member: m,
	})
}

// handleCreateGuild handles POST /guilds
func (h *Handler) handleCreateGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	withAuthenticatedCharacterAndJSON(h, w, r, func(req *createGuildRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req createGuildRequest) {
		g, m, _, err := h.guild.Create(r.Context(), char.ID, req.Name)
		if err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusCreated, createGuildResponse{
			Guild:  g,
			Member: m,
		})
	})
}

// handleApplyGuild handles POST /guilds/{id}/apply
func (h *Handler) handleApplyGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *guildActionCharacterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, _ guildActionCharacterRequest) {
		if err := h.guild.ApplyToJoin(r.Context(), guildID, char.ID); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "application submitted"})
	})
}

// handleApproveGuildApplication handles POST /guilds/{id}/applications/{applicant_id}/approve
func (h *Handler) handleApproveGuildApplication(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	applicantID := r.PathValue("applicant_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *approveGuildApplicationRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req approveGuildApplicationRequest) {
		title := req.Title
		if title == "" {
			title = "メンバー"
		}
		if err := h.guild.ApproveApplication(r.Context(), guildID, char.ID, applicantID, title); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "applicant approved"})
	})
}

// handleRejectGuildApplication handles POST /guilds/{id}/applications/{applicant_id}/reject
func (h *Handler) handleRejectGuildApplication(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	applicantID := r.PathValue("applicant_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *guildActionCharacterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, _ guildActionCharacterRequest) {
		if err := h.guild.RejectApplication(r.Context(), guildID, char.ID, applicantID); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "applicant rejected"})
	})
}

// handleBroadcastGuildCallout handles POST /guilds/{id}/callout
func (h *Handler) handleBroadcastGuildCallout(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *broadcastGuildCalloutRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req broadcastGuildCalloutRequest) {
		if err := h.guild.BroadcastCallout(r.Context(), guildID, char.ID, req.Message); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "callout broadcasted"})
	})
}

// handleAssignGuildRoleTitle handles PUT /guilds/{id}/members/{char_id}/title
func (h *Handler) handleAssignGuildRoleTitle(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	targetCharID := r.PathValue("char_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *assignGuildRoleTitleRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req assignGuildRoleTitleRequest) {
		if err := h.guild.AssignCustomRole(r.Context(), guildID, char.ID, targetCharID, req.Title); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "role title assigned"})
	})
}

// handleCustomizeGuild handles PUT /guilds/{id}/customization
func (h *Handler) handleCustomizeGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *customizeGuildRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, req customizeGuildRequest) {
		if req.Color != "" {
			if err := h.guild.UpdateColor(r.Context(), guildID, char.ID, req.Color); err != nil {
				h.writeGuildError(w, err)
				return
			}
		}
		if req.Mark != "" {
			if _, err := h.guild.ChangeMark(r.Context(), guildID, char.ID, req.Mark); err != nil {
				h.writeGuildError(w, err)
				return
			}
		}
		if req.Wallpaper != "" {
			if _, err := h.guild.ChangeWallpaper(r.Context(), guildID, char.ID, req.Wallpaper); err != nil {
				h.writeGuildError(w, err)
				return
			}
		}
		if req.Notice != "" {
			if err := h.guild.UpdateNotice(r.Context(), guildID, char.ID, req.Notice); err != nil {
				h.writeGuildError(w, err)
				return
			}
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "customization updated"})
	})
}

// handleLeaveGuild handles POST /guilds/{id}/leave
func (h *Handler) handleLeaveGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *guildActionCharacterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, _ guildActionCharacterRequest) {
		if err := h.guild.Leave(r.Context(), guildID, char.ID); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "left guild"})
	})
}

// handleKickGuildMember handles DELETE /guilds/{id}/members/{char_id}
func (h *Handler) handleKickGuildMember(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	targetCharID := r.PathValue("char_id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *guildActionCharacterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, _ guildActionCharacterRequest) {
		if err := h.guild.Kick(r.Context(), guildID, char.ID, targetCharID); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "member kicked"})
	})
}

// handleDisbandGuild handles DELETE /guilds/{id}
func (h *Handler) handleDisbandGuild(w http.ResponseWriter, r *http.Request) {
	if h.guild == nil {
		writeError(w, http.StatusNotImplemented, errors.New("guild service not configured"))
		return
	}

	guildID := r.PathValue("id")
	withAuthenticatedCharacterAndJSON(h, w, r, func(req *guildActionCharacterRequest) string {
		return req.CharacterID
	}, func(_ coreplayer.Player, char corecharacter.Character, _ guildActionCharacterRequest) {
		if err := h.guild.Disband(r.Context(), guildID, char.ID); err != nil {
			h.writeGuildError(w, err)
			return
		}

		writeJSON(w, http.StatusOK, guildMessageResponse{Message: "guild disbanded"})
	})
}

func (h *Handler) writeGuildError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, guild.ErrGuildNotFound),
		errors.Is(err, guild.ErrCharacterNotFound),
		errors.Is(err, guild.ErrApplicationNotFound),
		errors.Is(err, guild.ErrCharacterNotInGuild),
		errors.Is(err, guild.ErrTargetNotMember):
		writeError(w, http.StatusNotFound, err)
	case errors.Is(err, guild.ErrUnauthorized),
		errors.Is(err, guild.ErrCannotKickLeader),
		errors.Is(err, guild.ErrCannotAssignToLeader):
		writeError(w, http.StatusForbidden, err)
	case errors.Is(err, guild.ErrInsufficientFunds),
		errors.Is(err, guild.ErrInvalidGuildName),
		errors.Is(err, guild.ErrGuildNameTaken),
		errors.Is(err, guild.ErrInvalidGuildID),
		errors.Is(err, guild.ErrInvalidColorFormat),
		errors.Is(err, guild.ErrColorTaken),
		errors.Is(err, guild.ErrInvalidMark),
		errors.Is(err, guild.ErrInvalidWallpaper),
		errors.Is(err, guild.ErrInvalidRoleTitle),
		errors.Is(err, guild.ErrRoleTitleTooLong),
		errors.Is(err, guild.ErrReservedRoleTitle),
		errors.Is(err, guild.ErrEmptyCalloutMessage),
		errors.Is(err, guild.ErrCalloutMessageTooLong),
		errors.Is(err, guild.ErrCharacterAlreadyInGuild),
		errors.Is(err, guild.ErrApplicationAlreadyPending),
		errors.Is(err, guild.ErrMemberNotPending),
		errors.Is(err, guild.ErrMemberIsPending),
		errors.Is(err, guild.ErrLeaderCannotLeaveWithMembers),
		errors.Is(err, guild.ErrNoticeTooLong):
		writeError(w, http.StatusBadRequest, err)
	default:
		if strings.Contains(err.Error(), "not found") {
			writeError(w, http.StatusNotFound, err)
		} else {
			writeError(w, http.StatusInternalServerError, err)
		}
	}
}
