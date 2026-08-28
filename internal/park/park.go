package park

import (
	"errors"
	"fmt"
	"html"
	"math/rand"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidCharacterID = errors.New("character ID cannot be empty")
	ErrEmptyContent       = errors.New("post content cannot be empty")
	ErrContentTooLong     = errors.New("post content exceeds maximum length of 200 characters")
	ErrInvalidColor       = errors.New("invalid post color format")
	ErrRateLimited        = errors.New("posting rate limit exceeded, please wait a few moments")
	ErrCharacterNotFound  = errors.New("character not found")
)

const (
	MaxContentLength = 200
	MaxColorLength   = 16
	DefaultColor     = "#000000"
)

type Post struct {
	ID            string    `json:"id"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	Content       string    `json:"content"`
	Color         string    `json:"color"`
	RecipientName string    `json:"recipient_name"`
	CreatedAt     time.Time `json:"created_at"`
}

type DivinationResult struct {
	Fortune    string `json:"fortune"`
	LuckyColor string `json:"lucky_color"`
	Message    string `json:"message"`
}

// SanitizeContent trims spaces and escapes HTML tags.
func SanitizeContent(content string) string {
	trimmed := strings.TrimSpace(content)
	return html.EscapeString(trimmed)
}

// ValidatePost validates the input fields for creating a post.
func ValidatePost(characterID, content, color, recipient string) error {
	if strings.TrimSpace(characterID) == "" {
		return ErrInvalidCharacterID
	}
	cleanContent := strings.TrimSpace(content)
	if cleanContent == "" {
		return ErrEmptyContent
	}
	if utf8.RuneCountInString(cleanContent) > MaxContentLength {
		return ErrContentTooLong
	}
	if len(color) > MaxColorLength {
		return ErrInvalidColor
	}
	return nil
}

// TownGirlNPC represents the @町娘 NPC in the park.
type TownGirlNPC struct {
	mu  sync.Mutex
	rng *rand.Rand
}

func NewTownGirlNPC(rng *rand.Rand) *TownGirlNPC {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &TownGirlNPC{rng: rng}
}

func (n *TownGirlNPC) randomInt(max int) int {
	if max <= 0 {
		return 0
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	return n.rng.Intn(max)
}

var divinationFortunes = []string{
	"大吉", "吉", "中吉", "末吉", "小吉", "凶", "大凶",
	"大吉", "吉", "中吉", "末吉", "小吉", "凶", "大凶",
	"ハッピー", "アンハッピー", "オッパピー", "残念", "頑張って",
	"愛があります", "開き直ってください", "何か起きます",
}

var divinationColors = []string{
	"黒", "白", "青", "赤", "空", "ピンク", "紫", "緑", "灰",
	"ブルー", "水", "肌", "オレンジ", "黄", "茶", "ワインレッド",
	"猫", "海", "土", "森", "藍", "杏子", "イチゴ", "オリーブ",
	"金", "銀", "パール",
}

var inspectDialogues = []string{
	"何かお探しですか？",
	"メガネメガネ…",
	"はい！メガネ！",
	"お探し物はこれですか？-Ｏ-Ｏ-",
	"キャッ！何しているんですか！",
}

// Talk generates a conversational dialogue from the town girl.
func (n *TownGirlNPC) Talk(jobName, characterName string) string {
	dialogues := []string{
		fmt.Sprintf("%sさんこんにちわ", characterName),
		"今日はいい天気ですね〜",
		"夕方からはお天気が悪くなるみたいですよ",
		fmt.Sprintf("今日の夕飯は何を作ろうかしら。%sさんはどんな食べ物が好きですか？", characterName),
		fmt.Sprintf("あら、%sさん♪今日も元気そうですね", characterName),
		"これからどこに行くんですか？",
		fmt.Sprintf("%sさんを占ってあげましょう", characterName),
		"私の占いって結構当たるらしいですよ",
		"趣味は占いです",
	}
	if jobName != "" {
		dialogues = append(dialogues, fmt.Sprintf("%sさんの職業は%sですね？どうですか？当たりですか？", characterName, jobName))
	}

	idx := n.randomInt(len(dialogues))
	return dialogues[idx]
}

// Divinate generates a fortune and lucky color.
func (n *TownGirlNPC) Divinate(characterName string) DivinationResult {
	fortuneIdx := n.randomInt(len(divinationFortunes))
	colorIdx := n.randomInt(len(divinationColors))

	fortune := divinationFortunes[fortuneIdx]
	luckyColor := divinationColors[colorIdx]
	message := fmt.Sprintf("今日の%sさんの運勢はずばり…%sです♪ラッキーカラーは%s色ですよ〜。", characterName, fortune, luckyColor)

	return DivinationResult{
		Fortune:    fortune,
		LuckyColor: luckyColor,
		Message:    message,
	}
}

// Inspect generates an inspection dialogue response.
func (n *TownGirlNPC) Inspect() string {
	idx := n.randomInt(len(inspectDialogues))
	return inspectDialogues[idx]
}
