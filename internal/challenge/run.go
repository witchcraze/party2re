package challenge

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// runKind namespaces Challenge run identities on the shared run host.
const runKind = "challenge"

var (
	ErrLeaderOnly    = errors.New("only the leader can make the first advance")
	ErrEnemiesRemain = errors.New("defeat every enemy before advancing")
	ErrActorFallen   = errors.New("a fallen character can only leave or be revived")
)

// Run is the Challenge room library's snapshot: the host state plus legacy
// $win, the stage's world record when the room opened or last beat it.
type Run struct {
	runhost.State
	Record int `json:"record"`
}

func (r *Run) Host() *runhost.State { return &r.State }

// HallOfFame reads and raises a stage's world record.
type HallOfFame interface {
	GetHallOfFame(ctx context.Context, tierID string) (*HallOfFameEntry, error)
	RaiseHallOfFame(ctx context.Context, entry HallOfFameEntry) (bool, error)
}

type RunConfig struct {
	Store       runhost.Store[Run]
	Characters  runhost.Characters
	Inventories runhost.Inventories
	Builder     runhost.ActorBuilder
	Settler     runhost.Settler
	HallOfFame  HallOfFame
	Jobs        job.DefinitionProvider
	Admission   runhost.Lobby
	Blessings   runhost.Blessings
	Penalty     runhost.Penalty
	Clock       runhost.Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error
	// Tiers overrides the shipped catalog.
	Tiers map[string]ChallengeTier

	JobAssetRunner    economy.TransactionRunner
	JobAssetHelpers   func(context.Context) ([]int, error)
	JobAssetDepots    runhost.JobAssetDepots
	JobAssetEquipment depot.EquipmentRepository
}

// RunService runs Challenge waves on the shared run host: the host owns
// enrollment, commands and their combat executor, receipts, exit and cleanup;
// stageRoom supplies the vs_challenge rules.
type RunService struct {
	*runhost.Service[Run, *Run]
	cfg RunConfig
}

func NewRunService(cfg RunConfig) (*RunService, error) {
	if cfg.Characters == nil || cfg.HallOfFame == nil {
		return nil, runhost.ErrUnavailable
	}
	if cfg.Tiers == nil {
		var err error
		if cfg.Tiers, err = loadTiers(); err != nil {
			return nil, err
		}
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	s := &RunService{cfg: cfg}
	room := stageRoom{s: s}
	host, err := runhost.NewService[Run](runhost.Config[Run]{
		Kind: runKind, Store: cfg.Store, Characters: cfg.Characters, Inventories: cfg.Inventories,
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
