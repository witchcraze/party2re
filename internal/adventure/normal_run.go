package adventure

import (
	"context"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// normalRunKind namespaces stage-run identities on the shared run host.
const normalRunKind = "normal-adventure"

// NormalRun is the stage room library's snapshot: the host state plus the
// stage, floor-progress and encounter fields only Adventure stages use.
type NormalRun struct {
	runhost.State
	Stage               Stage                      `json:"stage"`
	RecordSeason        string                     `json:"record_season"`
	Mode                string                     `json:"mode"`
	FloorsCleared       int                        `json:"floors_cleared"`
	StageCleared        bool                       `json:"stage_cleared"`
	ClearPending        bool                       `json:"clear_pending"`
	EXUnlocked          bool                       `json:"ex_unlocked"`
	EncounterFloor      int                        `json:"encounter_floor"`
	EncounterStageID    string                     `json:"encounter_stage_id"`
	EncounterPool       []corebattle.LifetimeActor `json:"encounter_pool,omitempty"`
	AppearancePositions []int                      `json:"appearance_positions,omitempty"`
}

func (r *NormalRun) Host() *runhost.State { return &r.State }

// RunCombat is Adventure's explicit command executor (NewRunCombat). It must
// admit the action before effects, use the ambient transaction, and return
// only new earnings/consumption. It must return an execution-time rejection or
// failure rather than absorb it: core restores only the battle state, so the
// host then saves neither this run copy (travel/room changes included) nor a
// receipt. No batch resolver is a fallback for this port.
type RunCombat interface {
	PrepareAdvance(context.Context, *NormalRun, string) error
	PrepareBossField(context.Context, *NormalRun, string) error
	ExecuteRunAction(context.Context, *NormalRun, runhost.Command) (RunActionEffects, error)
}

type RunActionEffects struct {
	Earnings          map[string]runhost.Earnings
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
	Defeats         []battle.ExplicitDefeat
	EXUnlocked      bool
}

type RunInventories interface {
	InventoryRepository
	FindByCharacterIDForUpdate(context.Context, string) (coreinventory.Inventory, error)
}

type NormalRunConfig struct {
	Store       runhost.Store[NormalRun]
	Characters  runhost.Characters
	Inventories RunInventories
	Builder     runhost.ActorBuilder
	Settler     PostBattleSettler
	History     Repository
	Stages      *StageCatalog
	Monsters    *MonsterCatalog
	Jobs        job.DefinitionProvider
	Admission   runhost.Lobby
	Combat      RunCombat
	Blessings   BlessingProvider
	Penalty     runhost.Penalty
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

// NormalRunService runs Adventure stages on the shared run host: the host owns
// enrollment, commands, receipts, exit and cleanup; stageRoom supplies the rules.
type NormalRunService struct {
	*runhost.Service[NormalRun, *NormalRun]
	cfg NormalRunConfig
}

func NewNormalRunService(cfg NormalRunConfig) (*NormalRunService, error) {
	if cfg.Inventories == nil || cfg.History == nil || cfg.Stages == nil || cfg.Monsters == nil {
		return nil, runhost.ErrUnavailable
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	s := &NormalRunService{cfg: cfg}
	room := stageRoom{s: s}
	host, err := runhost.NewService[NormalRun](runhost.Config[NormalRun]{
		Kind: normalRunKind, Store: cfg.Store, Characters: cfg.Characters, Inventories: cfg.Inventories,
		Builder: cfg.Builder, Settler: cfg.Settler, Jobs: cfg.Jobs, Lobby: cfg.Admission, Penalty: cfg.Penalty,
		Clock: cfg.Clock, RNG: cfg.RNG, CheckEntry: cfg.CheckEntry, ExitEffects: cfg.ExitEffects, Room: room, Combat: room,
	})
	if err != nil {
		return nil, err
	}
	s.Service = host
	return s, nil
}

// EnterSolo enrolls actor alone in stageID without creating an encounter. The
// caller retains key on retries.
func (s *NormalRunService) EnterSolo(ctx context.Context, player, actor, key, stageID, mode string, speed int) (runhost.Receipt, error) {
	return s.Enroll(ctx, player, key, runhost.Admission{ActorID: actor, StageID: stageID, Mode: mode, Speed: speed})
}

func (s *NormalRunService) SetBlessings(provider BlessingProvider) { s.cfg.Blessings = provider }
