package adventure

import (
	"context"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

type NormalRunConfig struct {
	Store       runhost.Store[NormalRun]
	Characters  runhost.Characters
	Inventories runhost.Inventories
	Builder     runhost.ActorBuilder
	Settler     PostBattleSettler
	History     Repository
	Stages      *StageCatalog
	Monsters    *MonsterCatalog
	Jobs        job.DefinitionProvider
	Admission   runhost.Lobby
	Blessings   BlessingProvider
	Penalty     runhost.Penalty
	Clock       Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error
	EntryLocks  TimerService

	JobAssetRunner    economy.TransactionRunner
	JobAssetHelpers   func(context.Context) ([]int, error)
	JobAssetDepots    runhost.JobAssetDepots
	JobAssetEquipment depot.EquipmentRepository
}

// NormalRunService runs Adventure stages on the shared run host: the host owns
// enrollment, commands and their combat executor, receipts, exit and cleanup;
// stageRoom supplies the stage rules and combat seams.
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
		Clock: cfg.Clock, RNG: cfg.RNG, CheckEntry: cfg.CheckEntry, ExitEffects: cfg.ExitEffects, Room: room, CombatRoom: room,
		Blessings: cfg.Blessings, JobAssetRunner: cfg.JobAssetRunner, JobAssetHelpers: cfg.JobAssetHelpers,
		JobAssetDepots: cfg.JobAssetDepots, JobAssetEquipment: cfg.JobAssetEquipment,
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

func (s *NormalRunService) SetBlessings(provider BlessingProvider) {
	s.cfg.Blessings = provider
	s.Service.SetBlessings(provider)
}
