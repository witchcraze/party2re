package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// GuildReader observes guilds and the actor's affiliation.
type GuildReader interface {
	Get(ctx context.Context, guildID string) (guild.Detail, error)
	GetByCharacter(ctx context.Context, characterID string) (guild.Guild, guild.Member, error)
	List(ctx context.Context, offset, limit int) ([]guild.Guild, error)
}

// GuildManager covers membership and the guild room.
type GuildManager interface {
	Create(ctx context.Context, creatorCharID string, name string) (guild.Guild, guild.Member, corecharacter.Character, error)
	ApplyToJoin(ctx context.Context, guildID string, applicantID string) error
	Assign(ctx context.Context, guildID string, requesterCharID string, targetCharID string, title string) (guild.AssignOutcome, error)
	BroadcastCallout(ctx context.Context, guildID string, senderID string, message string) error
	RecordRoomPresence(ctx context.Context, guildID string) error
}

// GuildCustomizer covers the guild master's styling commands.
type GuildCustomizer interface {
	UpdateColor(ctx context.Context, guildID string, requesterCharID string, color string) error
	UpdateNotice(ctx context.Context, guildID string, requesterCharID string, notice string) error
	ChangeMark(ctx context.Context, guildID string, leaderID string, mark string) (corecharacter.Character, error)
	ChangeWallpaper(ctx context.Context, guildID string, leaderID string, wallpaper string) (corecharacter.Character, error)
}

// GuildLifecycle covers leaving and disbanding.
type GuildLifecycle interface {
	Disband(ctx context.Context, guildID string, leaderCharID string) error
	Leave(ctx context.Context, guildID string, characterID string) error
}

// GuildPointManager is the admin point decay.
type GuildPointManager interface {
	DecayGuildPoints(ctx context.Context, factor float64) error
}

// GuildService is the guild association, the guild room and admin decay.
type GuildService interface {
	GuildReader
	GuildManager
	GuildCustomizer
	GuildLifecycle
	GuildPointManager
}

// GuildResult is a guild command's reply: the replying NPC and its line, plus
// the paid character's state for fee-charging commands.
type GuildResult struct {
	NPCName   string                   `json:"npc_name"`
	Message   string                   `json:"message"`
	Outcome   guild.AssignOutcome      `json:"outcome,omitempty"`
	Character *characterResponse       `json:"character,omitempty"`
	Guild     *guild.Guild             `json:"guild,omitempty"`
	Selection *playercontext.Selection `json:"selection,omitempty"`
}

type guildCreateRequest struct {
	Name string `json:"name"`
}

type guildApplyRequest struct {
	GuildID string `json:"guild_id"`
}

type guildMarkRequest struct {
	Mark string `json:"mark"`
}

type guildWallpaperRequest struct {
	Wallpaper string `json:"wallpaper"`
}

type guildMessageRequest struct {
	Message string `json:"message"`
}

type guildColorRequest struct {
	Color string `json:"color"`
}

type guildAssignRequest struct {
	MemberID string `json:"member_id"`
	Title    string `json:"title"`
}

const guildAssociationNPC = "@支配人"

var guildCommandIDs = []string{"guild_create", "guild_apply", "guild_mark", "guild_wallpaper", "guild_leave", "guild_disband",
	"guild_callout", "guild_color", "guild_message", "guild_assign"}

// WithGuild configures the guild association and room scenes, their commands and admin decay.
func WithGuild(g GuildService) Option {
	return func(h *Handler) {
		h.guild = g
		if h.actionCommands == nil {
			h.actionCommands = make(map[string]actionCommand)
		}
		if g == nil {
			for _, id := range guildCommandIDs {
				h.actionCommands[id] = actionCommand{}
			}
			return
		}
		// Commands outside create/apply act on the actor's current active guild;
		// the service re-authorizes under its lock, so this read grants nothing.
		own := func(ctx context.Context, actor string, run func(guild.Guild) (GuildResult, error)) (GuildResult, error) {
			current, _, err := g.GetByCharacter(ctx, actor)
			if err != nil {
				return GuildResult{}, err
			}
			return run(current)
		}
		room := func(gd guild.Guild, message string) GuildResult {
			return GuildResult{NPCName: "@" + gd.Name, Message: message}
		}
		guildCommand(h, "guild_create", func(ctx context.Context, owner, actor string, p guildCreateRequest) (GuildResult, error) {
			created, _, char, err := g.Create(ctx, actor, p.Name)
			if err != nil {
				return GuildResult{}, err
			}
			result := GuildResult{NPCName: guildAssociationNPC, Message: "＠新ギルド " + created.Name + "の設立を許可します！", Guild: &created}
			result.Character = new(toCharacterResponse(char))
			// The founder moves into the new room (join_guild.cgi tsukuru sets $m{lib}).
			// The guild already exists, so a failed move is logged and left to ＠いどう.
			if h.playerContext != nil && h.playerContext.NavigationConfigured() {
				selection, err := h.playerContext.Enter(ctx, owner, actor, "guild")
				if err != nil {
					slog.WarnContext(ctx, "guild founder not moved to room", slog.String("character_id", actor), slog.String("error", err.Error()))
				} else {
					h.leavePlace(ctx, actor)
					result.Selection = &selection
				}
			}
			return result, nil
		})
		guildCommand(h, "guild_apply", func(ctx context.Context, _, actor string, p guildApplyRequest) (GuildResult, error) {
			// Read the master before applying so a failed read rejects before the commit.
			detail, err := g.Get(ctx, p.GuildID)
			if err != nil {
				return GuildResult{}, err
			}
			master, err := h.characters.Get(ctx, detail.Guild.LeaderCharacterID)
			if err != nil {
				return GuildResult{}, err
			}
			if err := g.ApplyToJoin(ctx, p.GuildID, actor); err != nil {
				return GuildResult{}, err
			}
			return GuildResult{NPCName: guildAssociationNPC, Message: detail.Guild.Name + "のギルマス " + master.Name + " に参加申請の手紙を送りました。ギルマスからの返事を待ちましょう"}, nil
		})
		guildCommand(h, "guild_mark", func(ctx context.Context, _, actor string, p guildMarkRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				char, err := g.ChangeMark(ctx, gd.ID, actor, p.Mark)
				if err != nil {
					return GuildResult{}, err
				}
				return GuildResult{NPCName: guildAssociationNPC, Message: gd.Name + "のギルドマークを " + strings.TrimSpace(p.Mark) + " に変更しました", Character: new(toCharacterResponse(char))}, nil
			})
		})
		guildCommand(h, "guild_wallpaper", func(ctx context.Context, _, actor string, p guildWallpaperRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				char, err := g.ChangeWallpaper(ctx, gd.ID, actor, p.Wallpaper)
				if err != nil {
					return GuildResult{}, err
				}
				base := strings.TrimSuffix(strings.TrimSpace(p.Wallpaper), ".gif")
				return GuildResult{NPCName: guildAssociationNPC, Message: gd.Name + "の壁紙を " + base + " に変更しました", Character: new(toCharacterResponse(char))}, nil
			})
		})
		guildCommand(h, "guild_leave", func(ctx context.Context, _, actor string, _ struct{}) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				if err := g.Leave(ctx, gd.ID, actor); err != nil {
					return GuildResult{}, err
				}
				return GuildResult{NPCName: guildAssociationNPC, Message: gd.Name + "から脱退しました"}, nil
			})
		})
		guildCommand(h, "guild_disband", func(ctx context.Context, _, actor string, _ struct{}) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				if err := g.Disband(ctx, gd.ID, actor); err != nil {
					return GuildResult{}, err
				}
				return GuildResult{NPCName: guildAssociationNPC, Message: gd.Name + "を解散させました"}, nil
			})
		})
		guildCommand(h, "guild_callout", func(ctx context.Context, _, actor string, p guildMessageRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				if err := g.BroadcastCallout(ctx, gd.ID, actor, p.Message); err != nil {
					return GuildResult{}, err
				}
				return room(gd, gd.Name+"のメンバー全員に手紙を送りました"), nil
			})
		})
		guildCommand(h, "guild_color", func(ctx context.Context, _, actor string, p guildColorRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				if err := g.UpdateColor(ctx, gd.ID, actor, p.Color); err != nil {
					return GuildResult{}, err
				}
				return room(gd, "ギルドカラーを"+strings.ToUpper(strings.TrimSpace(p.Color))+"に変更しました"), nil
			})
		})
		guildCommand(h, "guild_message", func(ctx context.Context, _, actor string, p guildMessageRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				if err := g.UpdateNotice(ctx, gd.ID, actor, p.Message); err != nil {
					return GuildResult{}, err
				}
				return room(gd, "ギルドメッセージを変更しました"), nil
			})
		})
		guildCommand(h, "guild_assign", func(ctx context.Context, _, actor string, p guildAssignRequest) (GuildResult, error) {
			return own(ctx, actor, func(gd guild.Guild) (GuildResult, error) {
				target, err := h.characters.Get(ctx, p.MemberID)
				if errors.Is(err, corecharacter.ErrNotFound) {
					return GuildResult{}, guild.ErrTargetNotMember
				}
				if err != nil {
					return GuildResult{}, err
				}
				outcome, err := g.Assign(ctx, gd.ID, actor, p.MemberID, p.Title)
				if err != nil {
					return GuildResult{}, err
				}
				// guild.cgi:ataeru $npc_com; a plain retitle has none.
				result := room(gd, map[guild.AssignOutcome]string{
					guild.AssignApproved: fmt.Sprintf("%sを%sに参加することを許可しました", target.Name, gd.Name),
					guild.AssignRejected: target.Name + "の参加を拒否しました",
					guild.AssignExpelled: fmt.Sprintf("%sを%sから追放しました", target.Name, gd.Name),
				}[outcome])
				result.Outcome = outcome
				return result, nil
			})
		})
	}
}

// guildCommand registers one guild command with the owning player available.
func guildCommand[P any](h *Handler, id string, run func(ctx context.Context, owner, actor string, p P) (GuildResult, error)) {
	def, known := actionDefinition(id)
	if !known {
		panic("guild command is not in the catalog: " + id)
	}
	h.actionCommands[id] = actionCommand{reject: guildRejection, prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
		var p P
		if err := decodeActionParams(raw, def.RequiredParams, &p); err != nil {
			return nil, err
		}
		return func(ctx context.Context, owner, actor string, _ playercontext.Result) (any, error) {
			return run(ctx, owner, actor, p)
		}, nil
	}}
}

func guildRejection(err error) (int, ErrorDetail) {
	switch {
	case errors.Is(err, guild.ErrCharacterNotInGuild):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_NOT_MEMBER", Message: "The character is not an active guild member."}
	case errors.Is(err, guild.ErrGuildNotFound):
		return http.StatusNotFound, ErrorDetail{Code: "GUILD_NOT_FOUND", Message: "Guild not found."}
	case errors.Is(err, guild.ErrUnauthorized), errors.Is(err, guild.ErrCannotKickLeader), errors.Is(err, guild.ErrCannotAssignToLeader):
		return http.StatusForbidden, ErrorDetail{Code: "GUILD_FORBIDDEN", Message: "Only the guild master may do this, and never to the master."}
	case errors.Is(err, guild.ErrTargetNotMember), errors.Is(err, guild.ErrMemberNotPending):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_MEMBER_NOT_FOUND", Message: "Choose one of the observed guild members."}
	case errors.Is(err, guild.ErrCharacterAlreadyInGuild):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_ALREADY_MEMBER", Message: "The character already belongs to a guild."}
	case errors.Is(err, guild.ErrApplicationAlreadyPending):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_APPLICATION_PENDING", Message: "An application to this guild is already pending."}
	case errors.Is(err, guild.ErrInsufficientFunds):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_INSUFFICIENT_GOLD", Message: "Not enough gold."}
	case errors.Is(err, guild.ErrGuildNameTaken):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_NAME_TAKEN", Message: "A guild with that name already exists."}
	case errors.Is(err, guild.ErrColorTaken):
		return http.StatusConflict, ErrorDetail{Code: "GUILD_COLOR_TAKEN", Message: "Another guild already uses that color."}
	case errors.Is(err, home.ErrLetterThrottled):
		return http.StatusTooManyRequests, ErrorDetail{Code: "GUILD_LETTER_THROTTLED", Message: "Letters are being sent too quickly."}
	case errors.Is(err, guild.ErrInvalidGuildName), errors.Is(err, guild.ErrInvalidGuildID), errors.Is(err, guild.ErrInvalidColorFormat),
		errors.Is(err, guild.ErrInvalidMark), errors.Is(err, guild.ErrInvalidWallpaper), errors.Is(err, guild.ErrInvalidRoleTitle),
		errors.Is(err, guild.ErrRoleTitleTooLong), errors.Is(err, guild.ErrReservedRoleTitle), errors.Is(err, guild.ErrEmptyCalloutMessage),
		errors.Is(err, guild.ErrCalloutMessageTooLong), errors.Is(err, guild.ErrNoticeTooLong), errors.Is(err, guild.ErrInvalidNotice),
		errors.Is(err, home.ErrContentTooLong), isTextValidationError(err):
		return http.StatusBadRequest, ErrorDetail{Code: "GUILD_INVALID_INPUT", Message: err.Error()}
	default:
		return 0, ErrorDetail{}
	}
}
