package http

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
)

// Legacy NPC presentation (lib/system.cgi:240-246,322-328,413; facility $npc_name,
// @words and shiraberu_npc). "{name}" stands for the actor's name ($m).
const (
	npcColor          = "#FF69B4" // $npc_color, config.cgi
	npcInspectDefault = "しかし何も見つからなかった…"
	npcNoWords        = "返事がない、ただのしかばねのようだ…"
)

var (
	talkVerb     = legacyVerb("npc_talk")
	inspectVerb  = legacyVerb("npc_inspect")
	puffPuffVerb = legacyVerb("secretshop_puffpuff")
	divinateVerb = legacyVerb("park_divinate")
)

type npcProfile struct {
	name         string
	words        []string
	wordsOf      func(*Handler, context.Context, corecharacter.Character) ([]string, error) // replaces words built from live facts
	inspect      string                                                                     // empty: npcInspectDefault
	inspectWords []string                                                                   // inspection answered by ＠はなす: logged publicly
	silent       bool                                                                       // inspection prints nothing (an empty shiraberu_npc)
	// talk replaces ＠はなす where the speaker is chosen per call (a resident pet).
	talk func(*Handler, context.Context, playercontext.Result) (NPCResult, error)
}

// NPCResult is the NPC's reply. Private replies are shown only to the actor and
// are not logged (legacy $mes).
type NPCResult struct {
	NPCName  string `json:"npc_name"`
	Dialogue string `json:"dialogue"`
	Private  bool   `json:"private"`
}

// registerNPC connects npc_talk and npc_inspect; the facility comes from the
// owner's current scene, never from client input.
func (h *Handler) registerNPC(service *playercontext.Service) {
	if service == nil {
		return
	}
	if h.actionCommands == nil {
		h.actionCommands = make(map[string]actionCommand)
	}
	acts := map[string]func(context.Context, playercontext.Result) (NPCResult, error){
		"npc_talk": func(ctx context.Context, current playercontext.Result) (NPCResult, error) {
			return h.npcAct(ctx, current, true)
		},
		"npc_inspect": func(ctx context.Context, current playercontext.Result) (NPCResult, error) {
			return h.npcAct(ctx, current, false)
		},
		"secretshop_puffpuff": func(ctx context.Context, current playercontext.Result) (NPCResult, error) {
			return h.puffPuff(ctx, current)
		},
		"park_divinate": func(ctx context.Context, current playercontext.Result) (NPCResult, error) {
			return h.divinate(ctx, current)
		},
	}
	for id, act := range acts {
		def, _ := playercontext.GetAction(id)
		h.actionCommands[id] = actionCommand{
			prepare: func(raw json.RawMessage) (func(context.Context, string, string, playercontext.Result) (any, error), error) {
				var none struct{}
				if err := decodeActionParams(raw, def.RequiredParams, &none); err != nil {
					return nil, err
				}
				return func(ctx context.Context, _, _ string, current playercontext.Result) (any, error) {
					return act(ctx, current)
				}, nil
			},
		}
	}
}

// currentNPC uses the scene already admitted by the dispatcher.
func (h *Handler) currentNPC(current playercontext.Result) (*npcProfile, corecharacter.Character) {
	destination := current.Snapshot.LocationID
	return h.sceneAdapters[destination].npc, current.Snapshot.Character
}

// puffPuff is ＠ぱふぱふ (secret.cgi:79-84): dialogue only, logged as a private
// row pair addressed to the actor ($to_name = $m).
func (h *Handler) puffPuff(ctx context.Context, current playercontext.Result) (NPCResult, error) {
	npc, char := h.currentNPC(current)
	word := fmt.Sprintf(puffPuffDialogueFormat, char.Name)
	h.appendNPCLog(ctx, current, puffPuffVerb, npc.name, word, char.ID)
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

// divinate is ＠うらない (park.cgi uranai): a public $npc_com that starts the wait.
func (h *Handler) divinate(ctx context.Context, current playercontext.Result) (NPCResult, error) {
	npc, char := h.currentNPC(current)
	word := divination(random.Intn, char.Name)
	h.appendNPCLog(ctx, current, divinateVerb, npc.name, word, "")
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

func (h *Handler) npcAct(ctx context.Context, current playercontext.Result, talk bool) (NPCResult, error) {
	npc, char := h.currentNPC(current)
	if talk && npc.talk != nil {
		return npc.talk(h, ctx, current)
	}
	if !talk && npc.silent {
		return NPCResult{NPCName: npc.name, Private: true}, nil
	}
	command, words := talkVerb, npc.words
	if !talk && len(npc.inspectWords) > 0 {
		command, words = inspectVerb+">"+npc.name, npc.inspectWords
	} else if !talk {
		text := npcReplacer(h, char).Replace(npc.inspect)
		if text == "" {
			text = npcInspectDefault
		}
		return NPCResult{NPCName: npc.name, Dialogue: text, Private: true}, nil
	}
	if talk && npc.wordsOf != nil {
		var err error
		if words, err = npc.wordsOf(h, ctx, char); err != nil {
			return NPCResult{}, err
		}
	}
	if len(words) == 0 {
		return NPCResult{NPCName: npc.name, Dialogue: npcNoWords, Private: true}, nil
	}
	word := npcReplacer(h, char).Replace(words[random.Intn(len(words))])
	h.appendNPCLog(ctx, current, command, npc.name, word, "")
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

// npcReplacer fills the speaker's facts into NPC lines.
func npcReplacer(h *Handler, char corecharacter.Character) *strings.Replacer {
	return strings.NewReplacer("{name}", char.Name, "{sp}", strconv.Itoa(char.SP), "{medals}", strconv.Itoa(char.SmallMedals), "{job}", h.jobName(char.JobID),
		"{monster_kills}", strconv.Itoa(char.MonsterKills), "{pvp_wins}", strconv.Itoa(char.PvPWins), "{mao}", strconv.Itoa(char.MaoCount),
		"{hero}", strconv.Itoa(char.HeroCount), "{casino_wins}", strconv.Itoa(char.CasinoWins))
}

// appendNPCLog writes the actor's command row and the NPC reply; a nonempty
// recipient makes both rows private, like legacy $to_name.
func (h *Handler) appendNPCLog(ctx context.Context, current playercontext.Result, command, npcName, word, recipient string) {
	if h.placeLog == nil {
		return
	}
	now := time.Now().UTC()
	char := current.Snapshot.Character
	place, err := h.scenePlace(ctx, current)
	if err != nil {
		slog.WarnContext(ctx, "place log place unresolved", slog.String("error", err.Error()))
		return
	}
	rows := []placelog.Entry{{OccurredAt: now, SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: char.ID, Name: char.Name, Color: char.Color, Text: command, RecipientCharacterID: recipient}}
	if word != "" {
		rows = append(rows, placelog.Entry{OccurredAt: now, SpeakerKind: placelog.SpeakerNPC, Name: npcName, Color: npcColor, Text: word, RecipientCharacterID: recipient})
	}
	if err := h.placeLog.Append(ctx, place, rows...); err != nil {
		// The reply is already decided; a lost log row is display-only.
		slog.WarnContext(ctx, "place log append failed", slog.String("place", place.Kind+":"+place.ID), slog.String("error", err.Error()))
	}
}
