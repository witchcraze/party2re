package helper

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/witchcraze/party2re/internal/id"
)

type QuestKind int

const (
	KindWeapon  QuestKind = 1
	KindArmor   QuestKind = 2
	KindItem    QuestKind = 3
	KindMonster QuestKind = 4
)

var (
	ErrQuestNotFound     = errors.New("helper quest not found")
	ErrQuestExpired      = errors.New("helper quest has expired")
	ErrQuestAlreadyDone  = errors.New("helper quest has already been completed")
	ErrGuildRequired     = errors.New("guild membership required for guild quest")
	ErrInsufficientItems = errors.New("insufficient items to complete helper quest")
	ErrInvalidParameters = errors.New("invalid helper quest parameters")
)

type Quest struct {
	ID            string     `json:"id"`
	Title         string     `json:"title"`
	Kind          QuestKind  `json:"kind"`
	TargetID      string     `json:"target_id"`
	TargetName    string     `json:"target_name"`
	RequiredCount int        `json:"required_count"`
	RewardItemID  string     `json:"reward_item_id"`
	IsRare        bool       `json:"is_rare"`
	IsGuild       bool       `json:"is_guild"`
	ExpiresAt     time.Time  `json:"expires_at"`
	CompletedAt   *time.Time `json:"completed_at,omitempty"`
	CompletedBy   string     `json:"completed_by,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
}

type RandomSource interface {
	Intn(max int) (int, error)
}

type cryptoRandomSource struct{}

func (cryptoRandomSource) Intn(max int) (int, error) {
	if max <= 0 {
		return 0, nil
	}
	val, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		return 0, err
	}
	return int(val.Int64()), nil
}

func DefaultRandomSource() RandomSource {
	return cryptoRandomSource{}
}

type TargetSpec struct {
	ID   string
	Name string
}

var (
	normalWeapons = []TargetSpec{
		{"weapon-01", "ヒノキの棒"}, {"weapon-02", "竹の槍"}, {"weapon-03", "こんぼう"},
		{"weapon-04", "かしの杖"}, {"weapon-05", "いばらのむち"}, {"weapon-06", "ブロンズナイフ"},
		{"weapon-07", "おおきづち"}, {"weapon-08", "どうのつるぎ"}, {"weapon-09", "石のオノ"},
		{"weapon-10", "くさりがま"}, {"weapon-11", "ブーメラン"}, {"weapon-12", "モーニングスター"},
		{"weapon-13", "まどうしの杖"}, {"weapon-14", "どくばり"}, {"weapon-15", "鉄の槍"},
		{"weapon-16", "鉄のつるぎ"}, {"weapon-17", "バトルアックス"}, {"weapon-18", "はがねのつるぎ"},
		{"weapon-19", "はがねのオノ"}, {"weapon-20", "ホーリーランス"}, {"weapon-21", "さばきの杖"},
		{"weapon-22", "ウォーハンマー"}, {"weapon-23", "まどろみの剣"}, {"weapon-24", "プラチナソード"},
		{"weapon-25", "炎のつるぎ"}, {"weapon-26", "ゾンビキラー"}, {"weapon-27", "ドラゴンキラー"},
	}
	rareWeapons = []TargetSpec{
		{"weapon-28", "はやぶさの剣"}, {"weapon-29", "きせきのつるぎ"}, {"weapon-30", "光の杖"},
		{"weapon-31", "グレートアックス"}, {"weapon-32", "ふぶきのつるぎ"}, {"weapon-33", "らいめいの剣"},
		{"weapon-34", "オーガシールド"}, {"weapon-35", "バスタードソード"}, {"weapon-36", "風神の盾"},
		{"weapon-37", "皆殺しの剣"}, {"weapon-38", "覇王のオノ"}, {"weapon-39", "メタルキングの剣"},
		{"weapon-40", "ロトのつるぎ"},
	}

	normalArmors = []TargetSpec{
		{"armor-01", "布の服"}, {"armor-02", "旅人の服"}, {"armor-03", "皮の鎧"},
		{"armor-04", "皮のドレス"}, {"armor-05", "くさりかたびら"}, {"armor-06", "青銅の鎧"},
		{"armor-07", "鉄の鎧"}, {"armor-08", "鉄の胸当て"}, {"armor-09", "みかわしの服"},
		{"armor-10", "はがねの鎧"}, {"armor-11", "まほうの法衣"}, {"armor-12", "まほうの鎧"},
		{"armor-13", "シルバーメイル"}, {"armor-14", "ダンシングメイル"}, {"armor-15", "ドラゴンメイル"},
		{"armor-16", "プラチナメイル"}, {"armor-17", "みずのはごろも"}, {"armor-18", "風のローブ"},
		{"armor-19", "スパイクアーマー"}, {"armor-20", "やいばのよろい"}, {"armor-21", "神秘のビキニ"},
		{"armor-22", "聖なる鎧"}, {"armor-23", "ふしぎなボレロ"}, {"armor-24", "炎の鎧"},
		{"armor-25", "ミラーアーマー"}, {"armor-26", "ギガントアーマー"}, {"armor-27", "ドラゴンローブ"},
	}
	rareArmors = []TargetSpec{
		{"armor-28", "天使のローブ"}, {"armor-29", "メタルキング鎧"}, {"armor-30", "光のドレス"},
		{"armor-31", "王者のローブ"}, {"armor-32", "セラフィムローブ"}, {"armor-33", "覇者の鎧"},
		{"armor-34", "神託のローブ"}, {"armor-35", "創世の鎧"},
	}

	normalItems = []TargetSpec{
		{"item-001", "薬草"}, {"item-002", "上薬草"}, {"item-003", "特薬草"},
		{"item-007", "毒消し草"}, {"item-008", "満月草"}, {"item-009", "目覚まし草"},
		{"item-010", "天使のすず"}, {"item-011", "聖水"}, {"item-012", "キメラの翼"},
	}
	rareItems = []TargetSpec{
		{"item-004", "賢者の石"}, {"item-005", "霊樹のしずく"}, {"item-006", "霊樹の葉"},
		{"item-028", "エルフの飲み薬"}, {"item-029", "世界樹の雫"},
	}

	normalMonsters = []TargetSpec{
		{"monster-001", "ドットスライム"}, {"monster-002", "スライム"}, {"monster-003", "レッドスライム"},
		{"monster-004", "おおがらす"}, {"monster-005", "いっかくうさぎ"}, {"monster-006", "おおみみず"},
		{"monster-007", "ゴースト"}, {"monster-008", "ドラキー"}, {"monster-009", "まほうつかい"},
	}
	rareMonsters = []TargetSpec{
		{"monster-280", "エルダーグリフォン"}, {"monster-281", "ブラッドロード"}, {"monster-282", "アビスドラゴン"},
		{"monster-283", "ヘルバトラー"}, {"monster-284", "デスジャガー"}, {"monster-285", "アークデーモン"},
	}

	normalRewards = []string{"item-128", "item-130", "item-131", "item-132", "item-133", "item-134", "item-184"}
	rareRewards   = []string{"item-129", "item-135", "item-136", "item-137"}
	guildReward   = "item-126" // 幸福袋
)

var weaponTitles = []string{"店を始めたいので", "強くなりたくて", "戦い用に", "ライバルに勝ちたくて", "見てみたい", "趣味で", "家宝にしたい", "探しています"}
var armorTitles = []string{"コンプリートのために", "カッコ良くなりたい", "オシャレになりたくて", "あこがれの服", "プレゼント用に", "着てみたい", "集めたい", "流行なので"}
var itemTitles = []string{"コレクション用", "病気を治すために", "非常用に", "必要なんです", "大好物なので", "気になるので", "自分用に欲しい", "用途は秘密です"}
var monsterTitles = []string{"かわいいので", "ペットほしい", "仲良くなりたい", "プニプニしたい", "いやされたい", "触ってみたい", "背中に乗ってみたい", "幸せになるために"}

func GenerateQuest(random RandomSource, now time.Time) (Quest, error) {
	if random == nil {
		random = DefaultRandomSource()
	}

	// Kind: 1..4
	kVal, err := random.Intn(4)
	if err != nil {
		return Quest{}, err
	}
	kind := QuestKind(kVal + 1)

	// IsRare: 1/14
	rVal, err := random.Intn(14)
	if err != nil {
		return Quest{}, err
	}
	isRare := (rVal == 0)

	// IsGuild: 1/15
	gVal, err := random.Intn(15)
	if err != nil {
		return Quest{}, err
	}
	isGuild := (gVal == 0)

	var targetList []TargetSpec
	var titles []string
	var needCount int

	switch kind {
	case KindWeapon:
		titles = weaponTitles
		if isRare {
			targetList = rareWeapons
		} else {
			targetList = normalWeapons
		}
		c, _ := random.Intn(3)
		needCount = c + 2
	case KindArmor:
		titles = armorTitles
		if isRare {
			targetList = rareArmors
		} else {
			targetList = normalArmors
		}
		c, _ := random.Intn(3)
		needCount = c + 2
	case KindItem:
		titles = itemTitles
		if isRare {
			targetList = rareItems
		} else {
			targetList = normalItems
		}
		c, _ := random.Intn(3)
		needCount = c + 2
	case KindMonster:
		titles = monsterTitles
		if isRare {
			targetList = rareMonsters
		} else {
			targetList = normalMonsters
		}
		c, _ := random.Intn(2)
		needCount = c + 1
	}

	if isGuild {
		needCount *= 2
	}

	tIdx, err := random.Intn(len(targetList))
	if err != nil {
		return Quest{}, err
	}
	target := targetList[tIdx]

	titleIdx, _ := random.Intn(len(titles))
	suffixNum, _ := random.Intn(999)
	title := fmt.Sprintf("%sその%d", titles[titleIdx], suffixNum+1)

	var rewardID string
	if isGuild {
		rewardID = guildReward
	} else if isRare {
		rwIdx, _ := random.Intn(len(rareRewards))
		rewardID = rareRewards[rwIdx]
	} else {
		rwIdx, _ := random.Intn(len(normalRewards))
		rewardID = normalRewards[rwIdx]
	}

	qid := id.New()

	return Quest{
		ID:            qid,
		Title:         title,
		Kind:          kind,
		TargetID:      target.ID,
		TargetName:    target.Name,
		RequiredCount: needCount,
		RewardItemID:  rewardID,
		IsRare:        isRare,
		IsGuild:       isGuild,
		ExpiresAt:     now.Add(6 * 24 * time.Hour),
		CreatedAt:     now,
	}, nil
}
