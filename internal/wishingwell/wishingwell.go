package wishingwell

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Legacy facility constants (sp_change.cgi).
const (
	LocationName    = "願いの泉"
	NPCName         = "@女神"
	BackgroundImage = "bgimg/sp_change.gif"
)

// Legacy dialogue lines (sp_change.cgi:20-29).
var DefaultDialogues = []string{
	"スキルポイントはレベルが上がるごとに１ポイント増えていくのです",
	"スキルポイントを早く上げるコツは、何度も同じ職業に転職することです",
	"%sのスキルポイントは現在 %d ポイントです",
	"スキル習得を目指している場合は、ささげずにとっておくのですよ",
	"スキルポイントをささげるのです",
	"スキルポイントのお礼に、%sのステータスを上げてあげましょう",
	"一度ささげたスキルポイントを戻すことはできません",
	"スキルポイントは、その職業のスキルを習得するのに必要です",
}

// Errors propagated from domain rules.
var (
	ErrNilRepository       = errors.New("character repository is nil")
	ErrInvalidSPAmount     = corecharacter.ErrInvalidSPAmount
	ErrInsufficientSP      = corecharacter.ErrInsufficientSP
	ErrJobMemoryActive     = corecharacter.ErrJobMemoryActive
	ErrOverLevelRestricted = corecharacter.ErrOverLevelRestricted
	ErrInvalidTargetStat   = corecharacter.ErrInvalidTargetStat
)

// CharacterRepository defines persistence operations needed by WishingWell.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, value corecharacter.Character) error
}

// TransactionProvider abstracts transactional execution.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// WishingWellStatus contains the current state and dialogues for a character.
type WishingWellStatus struct {
	LocationName    string   `json:"location_name"`
	NPCName         string   `json:"npc_name"`
	BackgroundImage string   `json:"background_image"`
	CharacterID     string   `json:"character_id"`
	CharacterName   string   `json:"character_name"`
	SP              int      `json:"sp"`
	JobMemoryActive bool     `json:"job_memory_active"`
	OverLevel       bool     `json:"over_level"`
	CanExchange     bool     `json:"can_exchange"`
	MaxHP           int      `json:"max_hp"`
	MaxMP           int      `json:"max_mp"`
	Attack          int      `json:"attack"`
	Defense         int      `json:"defense"`
	Agility         int      `json:"agility"`
	Dialogues       []string `json:"dialogues"`
}

// ExchangeRequest specifies which stat to raise and how much SP to spend.
type ExchangeRequest struct {
	CharacterID string `json:"character_id"`
	Stat        string `json:"stat"`
	SP          int    `json:"sp"`
}

// ExchangeResult reports the outcome of the SP offering.
type ExchangeResult struct {
	CharacterID  string                       `json:"character_id"`
	Stat         corecharacter.SPExchangeStat `json:"stat"`
	StatName     string                       `json:"stat_name"`
	SPConsumed   int                          `json:"sp_consumed"`
	StatIncrease int                          `json:"stat_increase"`
	RemainingSP  int                          `json:"remaining_sp"`
	Message      string                       `json:"message"`
	Character    corecharacter.Character      `json:"character"`
}

// FormatExchangeMessage formats the legacy NPC dialogue upon exchange ($npc_com).
func FormatExchangeMessage(sp int, statName string, statIncrease int) string {
	return fmt.Sprintf("SP %d のかわりに %s を %d あたえましょう", sp, statName, statIncrease)
}
