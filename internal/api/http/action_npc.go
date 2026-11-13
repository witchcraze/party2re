package http

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
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

type npcProfile struct {
	name         string
	words        []string
	wordsOf      func(*Handler, context.Context, corecharacter.Character) ([]string, error) // replaces words built from live facts
	inspect      string                                                                     // empty: npcInspectDefault
	inspectWords []string                                                                   // inspection answered by ＠はなす: logged publicly
	silent       bool                                                                       // inspection prints nothing (an empty shiraberu_npc)
}

// NPCResult is the NPC's reply. Private replies are shown only to the actor and
// are not logged (legacy $mes).
type NPCResult struct {
	NPCName  string `json:"npc_name"`
	Dialogue string `json:"dialogue"`
	Private  bool   `json:"private"`
}

var errNPCUnavailable = errors.New("no NPC in the current scene")

func npcRejection(err error) (int, ErrorDetail) {
	if errors.Is(err, errNPCUnavailable) {
		return http.StatusConflict, ErrorDetail{Code: "NPC_UNAVAILABLE", Message: "この場所には話せる相手がいません。"}
	}
	return 0, ErrorDetail{}
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
	acts := map[string]func(context.Context, string, string) (NPCResult, error){
		"npc_talk": func(ctx context.Context, owner, actor string) (NPCResult, error) {
			return h.npcAct(ctx, service, owner, actor, true)
		},
		"npc_inspect": func(ctx context.Context, owner, actor string) (NPCResult, error) {
			return h.npcAct(ctx, service, owner, actor, false)
		},
		"secretshop_puffpuff": func(ctx context.Context, owner, actor string) (NPCResult, error) {
			return h.puffPuff(ctx, service, owner, actor)
		},
		"park_divinate": func(ctx context.Context, owner, actor string) (NPCResult, error) {
			return h.divinate(ctx, service, owner, actor)
		},
	}
	for id, act := range acts {
		def, _ := playercontext.GetAction(id)
		h.actionCommands[id] = actionCommand{
			reject: npcRejection,
			prepare: func(raw json.RawMessage) (func(context.Context, string, string) (any, error), error) {
				var none struct{}
				if err := decodeActionParams(raw, def.RequiredParams, &none); err != nil {
					return nil, err
				}
				return func(ctx context.Context, owner, actor string) (any, error) { return act(ctx, owner, actor) }, nil
			},
		}
	}
}

// currentScene resolves the owner's current, still qualified scene; ok is
// false outside a registered scene.
func (h *Handler) currentScene(ctx context.Context, service *playercontext.Service, owner, actor string) (string, corecharacter.Character, bool, error) {
	current, err := service.Query(ctx, actor, owner)
	if err != nil {
		return "", corecharacter.Character{}, false, err
	}
	n := current.Navigation
	if n == nil || n.Unavailable {
		return "", corecharacter.Character{}, false, nil
	}
	destination, char := n.Selection.Destination, current.Snapshot.Character
	// A qualification lost after entry (e.g. job level) also leaves the scene.
	if def, ok := h.sceneAdapters[destination]; !ok || (def.definition.CanEnter != nil && !def.definition.CanEnter(char)) {
		return "", corecharacter.Character{}, false, nil
	}
	return destination, char, true, nil
}

// currentNPC resolves the NPC of the owner's current, still qualified facility.
func (h *Handler) currentNPC(ctx context.Context, service *playercontext.Service, owner, actor string) (string, *npcProfile, corecharacter.Character, error) {
	destination, char, ok, err := h.currentScene(ctx, service, owner, actor)
	if err != nil {
		return "", nil, corecharacter.Character{}, err
	}
	npc := h.sceneAdapters[destination].npc
	if !ok || npc == nil {
		return "", nil, corecharacter.Character{}, errNPCUnavailable
	}
	return destination, npc, char, nil
}

// puffPuff is ＠ぱふぱふ (secret.cgi:79-84): dialogue only, logged as a private
// row pair addressed to the actor ($to_name = $m).
func (h *Handler) puffPuff(ctx context.Context, service *playercontext.Service, owner, actor string) (NPCResult, error) {
	destination, npc, char, err := h.currentNPC(ctx, service, owner, actor)
	if err != nil {
		return NPCResult{}, err
	}
	if destination != "secretshop" {
		return NPCResult{}, errNPCUnavailable
	}
	word := fmt.Sprintf(puffPuffDialogueFormat, char.Name)
	h.appendNPCLog(ctx, destination, char, "＠ぱふぱふ", npc.name, word, char.ID)
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

// divinate is ＠うらない (park.cgi uranai): a public $npc_com that starts the wait.
func (h *Handler) divinate(ctx context.Context, service *playercontext.Service, owner, actor string) (NPCResult, error) {
	destination, npc, char, err := h.currentNPC(ctx, service, owner, actor)
	if err != nil {
		return NPCResult{}, err
	}
	if destination != "town" {
		return NPCResult{}, errNPCUnavailable
	}
	word := divination(random.Intn, char.Name)
	h.appendNPCLog(ctx, destination, char, "＠うらない", npc.name, word, "")
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

func (h *Handler) npcAct(ctx context.Context, service *playercontext.Service, owner, actor string, talk bool) (NPCResult, error) {
	destination, npc, char, err := h.currentNPC(ctx, service, owner, actor)
	if err != nil {
		return NPCResult{}, err
	}
	if !talk && npc.silent {
		return NPCResult{NPCName: npc.name, Private: true}, nil
	}
	command, words := "＠はなす", npc.words
	if !talk && len(npc.inspectWords) > 0 {
		command, words = "＠しらべる>"+npc.name, npc.inspectWords
	} else if !talk {
		text := strings.ReplaceAll(npc.inspect, "{name}", char.Name)
		if text == "" {
			text = npcInspectDefault
		}
		return NPCResult{NPCName: npc.name, Dialogue: text, Private: true}, nil
	}
	if talk && npc.wordsOf != nil {
		if words, err = npc.wordsOf(h, ctx, char); err != nil {
			return NPCResult{}, err
		}
	}
	if len(words) == 0 {
		return NPCResult{NPCName: npc.name, Dialogue: npcNoWords, Private: true}, nil
	}
	word := strings.NewReplacer("{name}", char.Name, "{sp}", strconv.Itoa(char.SP), "{job}", h.jobName(char.JobID)).Replace(words[random.Intn(len(words))])
	h.appendNPCLog(ctx, destination, char, command, npc.name, word, "")
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}

// appendNPCLog writes the actor's command row and the NPC reply; a nonempty
// recipient makes both rows private, like legacy $to_name.
func (h *Handler) appendNPCLog(ctx context.Context, destination string, char corecharacter.Character, command, npcName, word, recipient string) {
	if h.placeLog == nil {
		return
	}
	now := time.Now().UTC()
	if err := h.placeLog.Append(ctx, placelog.Place{Kind: placelog.KindFacility, ID: destination},
		placelog.Entry{OccurredAt: now, SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: char.ID, Name: char.Name, Color: char.Color, Text: command, RecipientCharacterID: recipient},
		placelog.Entry{OccurredAt: now, SpeakerKind: placelog.SpeakerNPC, Name: npcName, Color: npcColor, Text: word, RecipientCharacterID: recipient}); err != nil {
		// The reply is already decided; a lost log row is display-only.
		slog.WarnContext(ctx, "place log append failed", slog.String("place", destination), slog.String("error", err.Error()))
	}
}
