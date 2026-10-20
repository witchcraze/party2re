// Package helperquest implements the legacy Party2 Helper Quests (手助けクエスト / 便利屋クエスト, lib/helper.cgi).
// It manages community fetch quests for weapons, armors, items, and monsters, offering alchemy rewards and guild points.
// This is a dedicated game feature domain package, NOT a generic utility package.
package helperquest

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/depot"
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
	ErrQuestAlreadyDone  = errors.New("helper quest has already been completed")
	ErrGuildRequired     = errors.New("guild membership required for guild quest")
	ErrInsufficientItems = errors.New("insufficient items to complete helper quest")
	ErrInvalidParameters = errors.New("invalid helper quest parameters")
	ErrDepotFull         = depot.ErrDepotFull
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
	normalRewards = []string{"item-128", "item-128", "item-128", "item-130", "item-131", "item-132", "item-133", "item-134", "item-134", "item-184"}
	rareRewards   = []string{"item-129", "item-129", "item-129", "item-135", "item-136", "item-137"}
	guildReward   = "item-126"
)

// questTargets validates item slots against the native catalog and uses its names.
// Monster slots remain intact so a missing definition triggers a separate fallback draw.
func questTargets(kind QuestKind, rare bool) ([]TargetSpec, error) {
	prefix, width := "item", 3
	var ranges [][2]int
	switch kind {
	case KindWeapon, KindArmor:
		if kind == KindWeapon {
			prefix = "weapon"
		} else {
			prefix = "armor"
		}
		width = 2
		ranges = [][2]int{{1, 27}}
		if rare {
			ranges = [][2]int{{28, 40}}
		}
	case KindItem:
		ranges = [][2]int{{1, 27}, {30, 43}, {60, 65}, {72, 103}, {108, 108}}
		if rare {
			ranges = [][2]int{{28, 29}, {40, 40}, {58, 59}, {66, 71}, {104, 107}, {109, 109}}
		}
	case KindMonster:
		prefix = "monster"
		ranges = [][2]int{{4, 120}, {198, 260}}
		if rare {
			ranges = [][2]int{{500, 579}}
		}
	}
	catalog, err := item.DefaultCatalog()
	if err != nil {
		return nil, err
	}
	var targets []TargetSpec
	for _, bounds := range ranges {
		for n := bounds[0]; n <= bounds[1]; n++ {
			target := TargetSpec{ID: fmt.Sprintf("%s-%0*d", prefix, width, n)}
			if kind != KindMonster {
				def, err := catalog.FindByID(target.ID)
				if err != nil {
					return nil, fmt.Errorf("helper target %s: %w", target.ID, err)
				}
				target.Name = def.Name
			}
			targets = append(targets, target)
		}
	}
	if len(targets) == 0 {
		return nil, ErrInvalidParameters
	}
	return targets, nil
}

var weaponTitles = []string{"店を始めたいので", "強くなりたくて", "戦い用に", "ライバルに勝ちたくて", "見てみたい", "趣味で", "家宝にしたい", "探しています"}
var armorTitles = []string{"コンプリートのために", "カッコ良くなりたい", "オシャレになりたくて", "あこがれの服", "プレゼント用に", "着てみたい", "集めたい", "流行なので"}
var itemTitles = []string{"コレクション用", "病気を治すために", "非常用に", "必要なんです", "大好物なので", "気になるので", "自分用に欲しい", "用途は秘密です"}
var monsterTitles = []string{"かわいいので", "ペットほしい", "仲良くなりたい", "プニプニしたい", "いやされたい", "触ってみたい", "背中に乗ってみたい", "幸せになるために"}

func GenerateQuest(random RandomSource, now time.Time, lookupMonster func(string) (string, bool)) (Quest, error) {
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

	var titles []string
	countSlots, countBase := 3, 2
	switch kind {
	case KindWeapon:
		titles = weaponTitles
	case KindArmor:
		titles = armorTitles
	case KindItem:
		titles = itemTitles
	case KindMonster:
		titles = monsterTitles
		countSlots, countBase = 2, 1
	}
	c, err := random.Intn(countSlots)
	if err != nil {
		return Quest{}, err
	}
	needCount := c + countBase
	targetList, err := questTargets(kind, isRare)
	if err != nil {
		return Quest{}, err
	}

	if isGuild {
		needCount *= 2
	}

	tIdx, err := random.Intn(len(targetList))
	if err != nil {
		return Quest{}, err
	}
	target := targetList[tIdx]
	if kind == KindMonster {
		if lookupMonster == nil {
			return Quest{}, fmt.Errorf("monster catalog lookup is required: %w", ErrInvalidParameters)
		}
		var ok bool
		target.Name, ok = lookupMonster(target.ID)
		if !ok {
			first, slots := 1, 3
			if isRare {
				first, slots = 160, 6
			}
			fallback, err := random.Intn(slots)
			if err != nil {
				return Quest{}, err
			}
			target.ID = fmt.Sprintf("monster-%03d", first+fallback)
			target.Name, ok = lookupMonster(target.ID)
			if !ok {
				return Quest{}, fmt.Errorf("monster fallback is unavailable: %w", ErrInvalidParameters)
			}
		}
	}

	titleIdx, err := random.Intn(len(titles))
	if err != nil {
		return Quest{}, err
	}
	suffixNum, err := random.Intn(999)
	if err != nil {
		return Quest{}, err
	}
	title := fmt.Sprintf("%sその%d", titles[titleIdx], suffixNum+1)

	var rewardID string
	if isGuild {
		rewardID = guildReward
	} else if isRare {
		rwIdx, err := random.Intn(len(rareRewards))
		if err != nil {
			return Quest{}, err
		}
		rewardID = rareRewards[rwIdx]
	} else {
		rwIdx, err := random.Intn(len(normalRewards))
		if err != nil {
			return Quest{}, err
		}
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
