package wishingwell

import (
	"context"
	"errors"
	"fmt"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// NPCName is the well's goddess (sp_change.cgi $npc_name).
const NPCName = "@女神"

// NPCWords are sp_change.cgi @words; {name} and {sp} are the speaker's.
var NPCWords = []string{
	"スキルポイントはレベルが上がるごとに１ポイント増えていくのです",
	"スキルポイントを早く上げるコツは、何度も同じ職業に転職することです",
	"{name}のスキルポイントは現在 {sp} ポイントです",
	"スキル習得を目指している場合は、ささげずにとっておくのですよ",
	"スキルポイントをささげるのです",
	"スキルポイントのお礼に、{name}のステータスを上げてあげましょう",
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

// WishingWellStatus is the exchange eligibility observed by the scene.
type WishingWellStatus struct {
	JobMemoryActive bool `json:"job_memory_active"`
	OverLevel       bool `json:"over_level"`
	CanExchange     bool `json:"can_exchange"`
}

// StatusOf reports whether char may offer SP; SP and stats are the character's own.
func StatusOf(char corecharacter.Character) WishingWellStatus {
	jobMemory := char.JobMemory != nil && char.JobMemory.IsPersistent()
	return WishingWellStatus{JobMemoryActive: jobMemory, OverLevel: char.OverLevel, CanExchange: !jobMemory && !char.OverLevel}
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
