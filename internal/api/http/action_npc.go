package http

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/witchcraze/party2re/internal/bank"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/placelog"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/secretshop"
	"github.com/witchcraze/party2re/internal/shop"
)

// Legacy NPC presentation (lib/system.cgi:240-246,322-328,413; facility $npc_name,
// @words and shiraberu_npc). "{name}" stands for the actor's name ($m).
const (
	npcColor          = "#FF69B4" // $npc_color, config.cgi
	npcInspectDefault = "しかし何も見つからなかった…"
	npcNoWords        = "返事がない、ただのしかばねのようだ…"
)

type npcProfile struct {
	name    string
	words   []string
	inspect string // empty: npcInspectDefault
}

func shopNPC(kind shop.ShopType, inspect string) npcProfile {
	_, name := shop.GetShopMeta(kind)
	if inspect != "" {
		inspect = name + "「" + inspect + "」"
	}
	return npcProfile{name: name, words: shop.GetShopWords(kind), inspect: inspect}
}

// npcProfiles is keyed by scene destination. Lines with item recommendations or
// capacities ($gems/$ites/$max_depot) are omitted until those facts are wired.
var npcProfiles = map[string]npcProfile{
	"bank":           {name: bank.NPCName, words: bank.NPCDialogues},
	"blackmarket":    {name: "@闇商人", words: blackMarketTalkDialogues, inspect: "…お前の魂で取引したいのか？"},
	"secretshop":     {name: secretshop.NPCName, words: secretshop.DefaultTalkDialogues, inspect: secretshop.InspectDialogue},
	"shop_weapon":    shopNPC(shop.ShopTypeWeapon, "おいおい、俺は武器じゃねぇぜ"),
	"shop_armor":     shopNPC(shop.ShopTypeArmor, "な、な、何を見ているッスか！？"),
	"shop_item":      shopNPC(shop.ShopTypeItem, "ほえ？なんでしょうかぁ？"),
	"shop_accessory": shopNPC(shop.ShopTypeAccessory, ""),
	"gemstore": {name: "@ジェマ", words: []string{
		"ここは宝石店です。色んな宝石を売っています", "どんな宝石がお好きですか？",
		"アイテムや宝石を使って、新しい宝石に加工することもできますよ", "宝石ごとに不思議な力があるんです",
	}},
	"blacksmith": {name: "@ブッキー", words: []string{
		"ここは鍛冶屋だ！武器を強化できるぜ！", "武器に刻印するには刻印晶がいるぜ！", "武器を預けたりすると、刻印は消えちまうぜ",
	}},
	"alchemy": {name: "@トロデ", words: []string{
		"２つのアイテムを錬金することで新たなアイテムを作ることができるぞい", "錬金レシピを使うことで錬金することが可能になるぞい",
		"錬金したアイテムの完成は、お主が寝て起きた次の日には完成しているじゃろう", "錬金で作ることでしか手に入らない武器や防具があるそうじゃ・・・",
		"錬金レシピは習得済み以外のものを習得することができるぞい",
	}, inspect: "@トロデ「いやん。どこをさわっとるんじゃっ！」"},
	"depot": {name: "@ニキータ", words: []string{
		"ここは預かり所だけど、何か用かい？", "転職回数が増えるごとに預けられる個数も増えていくぜ",
		"＠おくる時は、送るアイテムと相手の名前を教えてくれな", "＠せいとんすると、武器、防具、道具の順に整頓できるぜ",
		"預かり所がまんぱんだと、相手からのアイテムが受け取れないぜ", "預かり所がまんぱんだと、クエストでの宝物を手に入れることができないぜ",
		"ここで売るのも専門店で売るのも売値は変わらないぜ",
	}},
	"fleamarket": {name: "@FMGKDP", words: []string{
		"ここはフリーマーケットだ。物の売り買いができるぜ", "物の価値は人それぞれだ。思いのほか良い物があるかもな",
		"１人で出せる品は限りがあるから気をつけな", "俺の名前を知らないようじゃピヨッコだ",
	}},
	"chapel": {name: "@シスター", words: []string{
		"{name}に神のご加護があらんことを", "待つだけでは祈りは叶いません…", "{name}が力を尽くすとき、祈りは叶うことでしょう",
	}},
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
	for _, id := range []string{"npc_talk", "npc_inspect"} {
		def, _ := playercontext.GetAction(id)
		talk := id == "npc_talk"
		h.actionCommands[id] = actionCommand{
			reject: npcRejection,
			prepare: func(raw json.RawMessage) (func(context.Context, string, string) (any, error), error) {
				var none struct{}
				if err := decodeActionParams(raw, def.RequiredParams, &none); err != nil {
					return nil, err
				}
				return func(ctx context.Context, owner, actor string) (any, error) {
					return h.npcAct(ctx, service, owner, actor, talk)
				}, nil
			},
		}
	}
}

func (h *Handler) npcAct(ctx context.Context, service *playercontext.Service, owner, actor string, talk bool) (NPCResult, error) {
	current, err := service.Query(ctx, actor, owner)
	if err != nil {
		return NPCResult{}, err
	}
	n := current.Navigation
	if n == nil || n.Unavailable {
		return NPCResult{}, errNPCUnavailable
	}
	destination := n.Selection.Destination
	npc, ok := npcProfiles[destination]
	if !ok {
		return NPCResult{}, errNPCUnavailable
	}
	char := current.Snapshot.Character
	// A qualification lost after entry (e.g. job level) also hides the NPC.
	if def, ok := h.sceneAdapters[destination]; !ok || (def.definition.CanEnter != nil && !def.definition.CanEnter(char)) {
		return NPCResult{}, errNPCUnavailable
	}
	if !talk {
		text := npc.inspect
		if text == "" {
			text = npcInspectDefault
		}
		return NPCResult{NPCName: npc.name, Dialogue: text, Private: true}, nil
	}
	if len(npc.words) == 0 {
		return NPCResult{NPCName: npc.name, Dialogue: npcNoWords, Private: true}, nil
	}
	word := strings.ReplaceAll(npc.words[random.Intn(len(npc.words))], "{name}", char.Name)
	if h.placeLog != nil {
		now := time.Now().UTC()
		if err := h.placeLog.Append(ctx, placelog.Place{Kind: placelog.KindFacility, ID: destination},
			placelog.Entry{OccurredAt: now, SpeakerKind: placelog.SpeakerCharacter, SpeakerCharacterID: char.ID, Name: char.Name, Color: char.Color, Text: "＠はなす"},
			placelog.Entry{OccurredAt: now, SpeakerKind: placelog.SpeakerNPC, Name: npc.name, Color: npcColor, Text: word}); err != nil {
			// The reply is already decided; a lost log row is display-only.
			slog.WarnContext(ctx, "place log append failed", slog.String("place", destination), slog.String("error", err.Error()))
		}
	}
	return NPCResult{NPCName: npc.name, Dialogue: word}, nil
}
