package http

import "fmt"

// The town hub is the legacy default place and 交流広場 (party.cgi:18, lib/park.cgi).
var townFacility = facility{
	title: "交流広場", commands: []string{"adventure_start", "park_divinate"}, read: (*Handler).townSceneData, placeLog: true,
	npc: &npcProfile{name: parkNPCName, words: parkWords, inspectWords: parkInspectWords},
}

// parkNPCName is the park's $npc_name.
const parkNPCName = "@町娘"

// parkWords are the park's @words; {name} is the actor and {job} the actor's job name.
var parkWords = []string{
	"{name}さんこんにちわ",
	"今日はいい天気ですね〜",
	"夕方からはお天気が悪くなるみたいですよ",
	"今日の夕飯は何を作ろうかしら。{name}さんはどんな食べ物が好きですか？",
	"あら、{name}さん♪今日も元気そうですね",
	"これからどこに行くんですか？",
	"{name}さんを占ってあげましょう",
	"私の占いって結構当たるらしいですよ",
	"趣味は占いです",
	"{name}さんの職業は{job}ですね？どうですか？当たりですか？",
}

// parkInspectWords replace @words for ＠しらべる>@町娘, which legacy answers through
// ＠はなす, so the line is logged publicly unlike other facilities' inspection.
var parkInspectWords = []string{
	"何かお探しですか？",
	"メガネメガネ…",
	"はい！メガネ！",
	"お探し物はこれですか？-Ｏ-Ｏ-",
	"キャッ！何しているんですか！",
}

// Fortunes keep legacy's repeated 大吉..大凶 run, which doubles their weight.
var fortunes = []string{
	"大吉", "吉", "中吉", "末吉", "小吉", "凶", "大凶",
	"大吉", "吉", "中吉", "末吉", "小吉", "凶", "大凶",
	"ハッピー", "アンハッピー", "オッパピー", "残念", "頑張って",
	"愛があります", "開き直ってください", "何か起きます",
}

var luckyColors = []string{
	"黒", "白", "青", "赤", "空", "ピンク", "紫", "緑", "灰",
	"ブルー", "水", "肌", "オレンジ", "黄", "茶", "ワインレッド",
	"猫", "海", "土", "森", "藍", "杏子", "イチゴ", "オリーブ",
	"金", "銀", "パール",
}

// divination is ＠うらない: a fortune and lucky colour for name, drawn by intn.
func divination(intn func(int) int, name string) string {
	return fmt.Sprintf("今日の%sさんの運勢はずばり…%sです♪ラッキーカラーは%s色ですよ〜。",
		name, fortunes[intn(len(fortunes))], luckyColors[intn(len(luckyColors))])
}
