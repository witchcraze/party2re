package http

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// GuildMemberView is one roster row as ＠めんばー lists it: name＠title.
type GuildMemberView struct {
	CharacterID string     `json:"character_id"`
	Name        string     `json:"name"`
	Title       string     `json:"title"`
	Role        guild.Role `json:"role"`
	Pending     bool       `json:"pending"`
}

// GuildView is one guild with its roster, pending applicants included, and decorations.
type GuildView struct {
	Guild       guild.Guild        `json:"guild"`
	Members     []GuildMemberView  `json:"members"`
	Decorations []guild.Decoration `json:"decorations"`
}

// GuildAffiliation is the actor's active guild membership.
type GuildAffiliation struct {
	Guild guild.Guild `json:"guild"`
	Role  guild.Role  `json:"role"`
	Title string      `json:"title"`
}

// GuildWallpaper is one ＠かべがみ choice.
type GuildWallpaper struct {
	Wallpaper string `json:"wallpaper"`
	Price     int    `json:"price"`
}

// GuildAssociationSceneData is the guild association (join_guild.cgi): a page of
// guilds, the selected guild's roster, and the mark and wallpaper choices.
type GuildAssociationSceneData struct {
	Parent      string            `json:"parent"`
	Affiliation *GuildAffiliation `json:"affiliation"`
	Guilds      []guild.Guild     `json:"guilds"`
	Page        ScenePage         `json:"page"`
	Selected    *GuildView        `json:"selected,omitempty"`
	Marks       []string          `json:"marks"`
	Wallpapers  []GuildWallpaper  `json:"wallpapers"`
}

// GuildRoomSceneData is the actor's own guild room (guild.cgi).
type GuildRoomSceneData struct {
	Parent   string    `json:"parent"`
	View     GuildView `json:"view"`
	IsLeader bool      `json:"is_leader"`
	// ColorSamples are the ＠からー sample palette, offered to the guild master.
	ColorSamples []HomeColorSample `json:"color_samples"`
}

// guildColorSamples is the guild.cgi ＠からー palette; it differs from Home's.
var guildColorSamples = []HomeColorSample{
	{"レッド", "#FF3333"}, {"ピンク", "#FF33CC"}, {"オレンジ", "#FF9933"}, {"イエロー", "#FFFF33"}, {"グリーン", "#33FF33"},
	{"アクア", "#33CCFF"}, {"ブルー", "#3333FF"}, {"パープル", "#CC66FF"}, {"グレイ", "#CCCCCC"}, {"ホワイト", "#FFFFFF"},
}

var guildMarks = func() []string {
	marks := make([]string, guild.MarkCount)
	for i := range marks {
		marks[i] = strconv.Itoa(i)
	}
	return marks
}()

// guildWallpapers lists the ＠かべがみ catalog cheapest first, as legacy sorts it.
var guildWallpapers = func() []GuildWallpaper {
	out := make([]GuildWallpaper, 0, len(guild.WallpaperPrices))
	for name, price := range guild.WallpaperPrices {
		out = append(out, GuildWallpaper{Wallpaper: name, Price: price})
	}
	slices.SortFunc(out, func(a, b GuildWallpaper) int {
		return cmp.Or(cmp.Compare(a.Price, b.Price), strings.Compare(a.Wallpaper, b.Wallpaper))
	})
	return out
}()

// guildView reads a guild with its member names; every read is required.
func (h *Handler) guildView(ctx context.Context, guildID string) (GuildView, error) {
	detail, err := h.guild.Get(ctx, guildID)
	if err != nil {
		return GuildView{}, err
	}
	view := GuildView{Guild: detail.Guild, Members: make([]GuildMemberView, 0, len(detail.Members)), Decorations: detail.Decorations}
	if view.Decorations == nil {
		view.Decorations = []guild.Decoration{}
	}
	for _, m := range detail.Members {
		char, err := h.characters.Get(ctx, m.CharacterID)
		if err != nil {
			return GuildView{}, err
		}
		view.Members = append(view.Members, GuildMemberView{CharacterID: m.CharacterID, Name: char.Name, Title: m.Title, Role: m.Role, Pending: m.IsPending})
	}
	return view, nil
}

// affiliation is the actor's active guild, or nil when it has none.
func (h *Handler) affiliation(ctx context.Context, actorID string) (*GuildAffiliation, error) {
	g, m, err := h.guild.GetByCharacter(ctx, actorID)
	if errors.Is(err, guild.ErrCharacterNotInGuild) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &GuildAffiliation{Guild: g, Role: m.Role, Title: m.Title}, nil
}

func (h *Handler) guildAssociationSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.guild == nil {
		return nil, errSceneNotConfigured
	}
	n := r.Navigation.Selection
	affiliation, err := h.affiliation(ctx, r.Snapshot.Character.ID)
	if err != nil {
		return nil, err
	}
	limit := cmp.Or(n.Limit, 20)
	data := GuildAssociationSceneData{Parent: "town", Affiliation: affiliation, Guilds: []guild.Guild{},
		Page: ScenePage{Mode: "offset", Offset: n.Offset, Limit: limit}, Marks: guildMarks, Wallpapers: guildWallpapers}
	if n.Subject != (playercontext.Subject{}) {
		view, err := h.guildView(ctx, n.Subject.ID)
		if errors.Is(err, guild.ErrGuildNotFound) {
			return nil, playercontext.ErrSelectionNotFound
		}
		if err != nil {
			return nil, err
		}
		data.Selected = &view
		return data, nil
	}
	guilds, err := h.guild.List(ctx, n.Offset, limit)
	if err != nil {
		return nil, err
	}
	data.Guilds = append(data.Guilds, guilds...)
	// shortcut: a full page offers a next page that may be empty; upgrade if List reports totals.
	if len(guilds) == limit {
		data.Page.Next = &playercontext.PageParams{Destination: n.Destination, Offset: n.Offset + limit, Limit: limit}
	}
	return data, nil
}

func (h *Handler) guildRoomSceneData(ctx context.Context, r playercontext.Result) (any, error) {
	if h.guild == nil {
		return nil, errSceneNotConfigured
	}
	actor := r.Snapshot.Character.ID
	g, _, err := h.guild.GetByCharacter(ctx, actor)
	if errors.Is(err, guild.ErrCharacterNotInGuild) {
		return nil, playercontext.ErrSelectionNotFound
	}
	if err != nil {
		return nil, err
	}
	view, err := h.guildView(ctx, g.ID)
	if errors.Is(err, guild.ErrGuildNotFound) {
		return nil, playercontext.ErrSelectionNotFound
	}
	if err != nil {
		return nil, err
	}
	data := GuildRoomSceneData{Parent: "town", View: view, IsLeader: view.Guild.LeaderCharacterID == actor, ColorSamples: []HomeColorSample{}}
	if data.IsLeader {
		data.ColorSamples = guildColorSamples
	}
	return data, nil
}

// guildRoomTalk is ＠はなす in the guild room (guild.cgi:hanasu): the guild
// message privately, or the member list when there is none.
func guildRoomTalk(h *Handler, ctx context.Context, r playercontext.Result) (NPCResult, error) {
	data, err := h.guildRoomSceneData(ctx, r)
	if err != nil {
		return NPCResult{}, err
	}
	view := data.(GuildRoomSceneData).View
	text := view.Guild.Notice
	if text == "" {
		rows := make([]string, 0, len(view.Members))
		for _, m := range view.Members {
			rows = append(rows, m.Name+"＠"+m.Title)
		}
		text = strings.Join(rows, ",")
	}
	return NPCResult{NPCName: "@" + view.Guild.Name, Dialogue: text, Private: true}, nil
}

// decorationNames are the legacy member-list names of guild decorations (god.cgi).
var decorationNames = map[string]string{guild.GoldMedal: "金メダル"}

var (
	guildAssociationCommands = []string{"guild_create", "guild_mark", "guild_wallpaper", "guild_leave", "guild_disband"}
	// guildAssociationFacility is join_guild.cgi; ＠さんか applies to the selected guild.
	guildAssociationFacility = facility{
		title: "ギルド", commands: guildAssociationCommands, subjectCommands: append(slices.Clone(guildAssociationCommands), "guild_apply"),
		read: (*Handler).guildAssociationSceneData, placeLog: true,
		bind: func(n playercontext.Selection, _ string) map[string]map[string]any {
			if n.Subject == (playercontext.Subject{}) {
				return nil
			}
			return map[string]map[string]any{"guild_apply": {"guild_id": n.Subject.ID}}
		},
		present: func(_ *Handler, _ context.Context, r playercontext.Result, data any) scenePresentation {
			d := data.(GuildAssociationSceneData)
			p := scenePresentation{next: d.Page.Next}
			// Offer only what the actor's current affiliation and role permit.
			switch {
			case d.Affiliation == nil:
				p.hide = []string{"guild_mark", "guild_wallpaper", "guild_leave", "guild_disband"}
			case d.Affiliation.Role == guild.RoleLeader:
				p.hide = []string{"guild_create", "guild_apply"}
			default:
				p.hide = []string{"guild_create", "guild_apply", "guild_mark", "guild_wallpaper", "guild_disband"}
			}
			if d.Selected != nil && slices.ContainsFunc(d.Selected.Members, func(m GuildMemberView) bool { return m.CharacterID == r.Snapshot.Character.ID }) {
				p.hide = append(p.hide, "guild_apply")
			}
			return p
		},
		npc: &npcProfile{name: "@支配人", words: []string{
			"20 日以上「＠ぎるど」による出入りがない場合は、自動的に削除となります",
			"ギルドとは、気が合うメンバーの集まりです",
			"ギルド名は、途中で変えることができませんので、じっくり考えてください",
			"ギルマスとは、ギルドマスターの略称です。そのギルド内で一番の権限があります",
			"ギルマスは、メンバーに役職名をあたえることができるのです",
			"ギルドマークや壁紙は、お金がかかりますが何度でも変えることが可能です",
			"ギルド参加者は、ギルド戦ができるようになります",
			"ギルドを新しく作るには、5000 G必要です",
			"ギルドマークを変更するには、3000 G必要です",
			"ギルド戦で優勝すると勝利メダルがギルド内に飾られていきます",
		}},
	}
	// guildRoomFacility is guild.cgi: the actor's own guild, with a log shared by its members.
	guildRoomFacility = facility{
		title: "所属ギルド", commands: []string{"guild_callout", "guild_color", "guild_message", "guild_assign"},
		read: (*Handler).guildRoomSceneData, placeLog: true, npc: &npcProfile{talk: guildRoomTalk},
		place: func(h *Handler, ctx context.Context, r playercontext.Result) (placelog.Place, error) {
			g, _, err := h.guild.GetByCharacter(ctx, r.Snapshot.Character.ID)
			if err != nil {
				return placelog.Place{}, err
			}
			return placelog.Place{Kind: "guild", ID: g.ID}, nil
		},
		refreshed: func(h *Handler, ctx context.Context, p placelog.Place) error {
			return h.guild.RecordRoomPresence(ctx, p.ID)
		},
		present: func(_ *Handler, _ context.Context, _ playercontext.Result, data any) scenePresentation {
			d := data.(GuildRoomSceneData)
			title := d.View.Guild.Name
			p := scenePresentation{title: &title}
			if !d.IsLeader {
				p.hide = []string{"guild_color", "guild_message", "guild_assign"}
			}
			for _, decoration := range d.View.Decorations {
				p.members = append(p.members, PlaceMember{Kind: "npc", Name: cmp.Or(decorationNames[decoration.Kind], decoration.Kind), Color: npcColor, IconURL: decoration.Icon})
			}
			return p
		},
	}
)
