package adventure

import (
	"context"
	"errors"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
)

var (
	ErrRunConflict    = errors.New("normal adventure command conflicts with committed state")
	ErrRunForbidden   = errors.New("normal adventure actor is not authorized")
	ErrRunBusy        = errors.New("character has unfinished normal adventure work")
	ErrRunUnavailable = errors.New("normal adventure command implementation unavailable")
	ErrRunCooldown    = errors.New("normal adventure actor interval has not elapsed")
	ErrRunRecovery    = errors.New("normal adventure requires recovery")
)

// NormalRun is the Adventure-owned durable state, independent of waiting lobbies.
// Character records and executable callbacks never belong in this snapshot.
type NormalRun struct {
	ID                  string                     `json:"id"`
	Version             int                        `json:"version"`
	Encoding            int                        `json:"encoding"`
	Admission           NormalAdmission            `json:"admission"`
	Stage               Stage                      `json:"stage"`
	RecordSeason        string                     `json:"record_season"`
	Mode                string                     `json:"mode"`
	LeaderID            string                     `json:"leader_id"`
	Roster              []string                   `json:"roster"`
	Floor               int                        `json:"floor"`
	FloorsCleared       int                        `json:"floors_cleared"`
	StageCleared        bool                       `json:"stage_cleared"`
	ClearPending        bool                       `json:"clear_pending"`
	EXUnlocked          bool                       `json:"ex_unlocked"`
	EncounterFloor      int                        `json:"encounter_floor"`
	EncounterStageID    string                     `json:"encounter_stage_id"`
	Enemies             []string                   `json:"enemies"`
	EncounterPool       []corebattle.LifetimeActor `json:"encounter_pool,omitempty"`
	AppearancePositions []int                      `json:"appearance_positions,omitempty"`
	Battle              corebattle.LifetimeState   `json:"battle"`
	LastActions         map[string]time.Time       `json:"last_actions"`
	Recipients          map[string]RunRecipient    `json:"recipients"`
	Boxes               []TreasureBox              `json:"boxes"`
	StartedAt           time.Time                  `json:"started_at"`
	LastActivity        time.Time                  `json:"last_activity"`
	TimedOut            bool                       `json:"timed_out"`
}

// NormalAdmission is frozen under Party's roster lock. Generation is the lobby
// identity for Party admission and the owned request identity for solo admission.
type NormalAdmission struct {
	Generation   string   `json:"generation"`
	PartyID      string   `json:"party_id"`
	ActorID      string   `json:"actor_id"`
	CharacterIDs []string `json:"character_ids"`
	StageID      string   `json:"stage_id"`
	Season       string   `json:"season"`
	Mode         string   `json:"mode"`
	Speed        int      `json:"speed"`
}

type RunEarnings struct {
	EXP      int `json:"experience"`
	Gold     int `json:"gold"`
	Crystals int `json:"crystals"`
}

type RunRecipient struct {
	PlayerID     string      `json:"player_id"`
	Pending      RunEarnings `json:"pending"`
	Conditional  RunEarnings `json:"conditional"`
	Applied      RunEarnings `json:"applied"`
	Inspected    int         `json:"inspected"`
	Disposition  string      `json:"disposition"`
	PenaltyUntil time.Time   `json:"penalty_until"`
	TimerPending bool        `json:"timer_pending"`
}

type RunCommand struct {
	PlayerID string `json:"player_id"`
	ActorID  string `json:"actor_id"`
	RunID    string `json:"run_id"`
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Action   string `json:"action"`
	TargetID string `json:"target_id"`
	Input    string `json:"input"`
}

// RunReceipt contains only the executing actor's result, including after exit.
type RunReceipt struct {
	Command     RunCommand   `json:"command"`
	Version     int          `json:"version"`
	Floor       int          `json:"floor"`
	StageID     string       `json:"stage_id"`
	Disposition string       `json:"disposition"`
	Box         *TreasureBox `json:"box,omitempty"`
	Recipient   RunRecipient `json:"recipient"`
}

type ActorRun struct {
	RunID        string
	PartyID      string
	Version      int
	Floor        int
	LeaderID     string
	Role         string
	IdleDue      bool
	LastActivity time.Time
	Recipient    RunRecipient
}

// RetainedEarnings is an unpaid unconditional balance that a timed-out run left
// in its recipient ledger after the active room was disposed.
type RetainedEarnings struct {
	RunID     string
	Recipient RunRecipient
}

// RetainedEarningsStore owns the claim path for RetainedEarnings. The read
// precedes settlement; the write runs after it (Rank 8), locks the row and fails
// with ErrRunConflict unless its unpaid balance equals the credited one.
type RetainedEarningsStore interface {
	FindRetainedEarnings(context.Context, string) ([]RetainedEarnings, error)
	SaveRetainedRecipient(context.Context, string, string, RunEarnings, RunRecipient) error
}

// NormalRunStore keeps receipts and recipient dispositions beyond active state
// disposal. WithRunForUpdate must start before any ambient character transaction.
type NormalRunStore interface {
	RetainedEarningsStore
	WithRunForUpdate(context.Context, string, bool, func(context.Context) error) error
	FindRun(context.Context, string) (*NormalRun, error)
	SaveRun(context.Context, NormalRun, int) error
	FindRunReceipt(context.Context, string, string) (*RunReceipt, error)
	SaveRunReceipt(context.Context, RunReceipt) error
	EnrollRun(context.Context, NormalRun) error
	SaveRunRecipient(context.Context, string, string, RunRecipient) error
	FindActorRun(context.Context, string) (*ActorRun, error)
	FindAdmission(context.Context, string) (*NormalAdmission, error)
	FindPenalty(context.Context, string) (*ActorRun, error)
}

type RunCharacters interface {
	FindByID(context.Context, string) (corecharacter.Character, error)
	FindByIDForUpdate(context.Context, string) (corecharacter.Character, error)
	Update(context.Context, corecharacter.Character) error
}

type RunActorBuilder interface {
	BuildLifetimeActor(context.Context, string, string) (corebattle.LifetimeActor, error)
}

// RunAdmission serializes live waiting-roster validation, but never owns SQL
// asset locks. Roster validation runs under the matching SQL handoff lock.
type RunAdmission interface {
	WithNormalRoster(context.Context, string, string, func(context.Context, NormalAdmission) error) error
	CleanupNormalLobby(context.Context, NormalAdmission) error
	CheckSoloAdmission(context.Context, string) error
}

// RunCombat is the explicit command seam owned by #1329. It must validate the
// whole action before effects, use the ambient transaction, and return only new
// earnings/consumption. No batch resolver is a fallback for this port.
type RunCombat interface {
	PrepareAdvance(context.Context, *NormalRun, string) error
	PrepareBossField(context.Context, *NormalRun, string) error
	ExecuteRunAction(context.Context, *NormalRun, RunCommand) (RunActionEffects, error)
}

type RunActionEffects struct {
	Earnings          map[string]RunEarnings
	Consumed          map[string][]corebattle.ConsumedItem
	RoomActivity      bool
	ActorActivity     bool
	ReleaseStock      bool
	EncounterDefeated bool
	// Only an owning encounter-creation transition supplies this event.
	UnsealDemonKing bool
	// Excluded from this operation's enemy array, restored last for next request.
	MemberOnlyIDs []string
	// Encounter identities of defeats that reached the player reward path, in
	// defeat order: no NPC killer, self-defeat or revival (_battle.cgi:28–51,142).
	CreditedDefeats []string
}

// RunPenalty projects a committed absolute deadline. SQL ownership remains held
// until this succeeds; Home must independently enforce the durable deadline.
type RunPenalty interface {
	ProjectRunPenalty(context.Context, string, time.Time) error
}

type NormalRunConfig struct {
	Store       NormalRunStore
	Characters  RunCharacters
	Inventories InventoryRepository
	Builder     RunActorBuilder
	Settler     PostBattleSettler
	History     Repository
	Stages      *StageCatalog
	Monsters    *MonsterCatalog
	Admission   RunAdmission
	Combat      RunCombat
	Blessings   BlessingProvider
	Penalty     RunPenalty
	Clock       Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error
	EntryLocks  TimerService

	JobAssetRunner  economy.TransactionRunner
	JobAssetHelpers func(context.Context) ([]int, error)
	JobAssetDepots  JobAssetDepots
}

// JobAssetDepots locks and saves the requesting player's depot inside the
// ambient Command transaction, after the economy runner's inventory lock.
type JobAssetDepots interface {
	FindByCharacterIDForUpdate(context.Context, string) (depot.Depot, error)
	Save(context.Context, depot.Depot) error
}

type NormalRunService struct{ cfg NormalRunConfig }

type normalMutationContext struct{}
type normalAdmissionContext struct{}

// CheckMutation protects conflicting service mutations without taking a run
// lock after character locks. Native commands carry the already-held run identity.
func (s *NormalRunService) CheckMutation(ctx context.Context, actor string) error {
	run, err := s.cfg.Store.FindActorRun(ctx, actor)
	if err != nil {
		return err
	}
	if run != nil && ctx.Value(normalMutationContext{}) != run.RunID {
		return ErrRunBusy
	}
	return nil
}

func NewNormalRunService(cfg NormalRunConfig) (*NormalRunService, error) {
	if cfg.Store == nil || cfg.Characters == nil || cfg.Inventories == nil || cfg.Builder == nil || cfg.Settler == nil || cfg.History == nil || cfg.Stages == nil || cfg.Monsters == nil || cfg.Clock == nil || cfg.Penalty == nil {
		return nil, ErrRunUnavailable
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	return &NormalRunService{cfg: cfg}, nil
}

func (s *NormalRunService) SetExitEffects(apply func(context.Context, string) error) {
	s.cfg.ExitEffects = apply
}

func (s *NormalRunService) SetBlessings(provider BlessingProvider) { s.cfg.Blessings = provider }

func (r *NormalRun) actor(id string) *corebattle.LifetimeActor {
	for i := range r.Battle.Actors {
		if r.Battle.Actors[i].ID == id {
			return &r.Battle.Actors[i]
		}
	}
	return nil
}
