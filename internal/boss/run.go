// Package boss is the sealing room library (vs_king.cgi) on the shared run
// host: unseal opens one running room per king stage, characters join it
// mid-battle, fight with explicit commands, seal it and inspect its chests.
package boss

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// runKind namespaces sealing run identities on the shared run host.
const runKind = "sealing"

var (
	ErrEnemiesRemain  = errors.New("defeat every enemy before sealing")
	ErrSealed         = errors.New("sealing room is already sealed")
	ErrNeedJoinNotMet = errors.New("character does not meet the room's join condition")
)

// Run is the sealing room's snapshot: the host state plus the leader boss,
// whose defeat lets the last exit delete the room.
type Run struct {
	runhost.State
	Leader string `json:"leader"`
}

func (r *Run) Host() *runhost.State { return &r.State }

type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// UnsealClockReader reads the persisted demon king unseal timestamp.
type UnsealClockReader interface {
	GetUnsealedAt(ctx context.Context) (*time.Time, error)
}

// VictoryBanquetHook adds the celebration crowd after a sealing commits (_win_vs_king.cgi).
type VictoryBanquetHook func(ctx context.Context) error

type RunConfig struct {
	Store       runhost.Store[Run]
	Characters  runhost.Characters
	Inventories runhost.Inventories
	Builder     runhost.ActorBuilder
	Settler     runhost.Settler
	Jobs        job.DefinitionProvider
	Blessings   runhost.Blessings
	Penalty     runhost.Penalty
	Clock       runhost.Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error
	UnsealClock UnsealClockReader
	News        NewsPublisher
	// Stages overrides the shipped catalog.
	Stages []BossStage

	JobAssetRunner    economy.TransactionRunner
	JobAssetHelpers   func(context.Context) ([]int, error)
	JobAssetDepots    runhost.JobAssetDepots
	JobAssetEquipment depot.EquipmentRepository
}

// RunService runs sealing rooms on the shared run host: the host owns
// joining, commands and their combat executor, receipts, exit and disposal;
// sealingRoom supplies the vs_king rules.
type RunService struct {
	*runhost.Service[Run, *Run]
	cfg     RunConfig
	banquet VictoryBanquetHook
	stages  []BossStage
	byID    map[string]BossStage
}

func NewRunService(cfg RunConfig) (*RunService, error) {
	if cfg.Characters == nil || cfg.Clock == nil || cfg.UnsealClock == nil {
		return nil, runhost.ErrUnavailable
	}
	if cfg.Stages == nil {
		var err error
		if cfg.Stages, err = DefaultBossCatalog(); err != nil {
			return nil, err
		}
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	s := &RunService{cfg: cfg, stages: cfg.Stages, byID: make(map[string]BossStage, len(cfg.Stages))}
	maxMembers := 0
	for _, stage := range cfg.Stages {
		s.byID[stage.ID] = stage
		maxMembers = max(maxMembers, stage.MaxMembers)
	}
	room := sealingRoom{s: s}
	host, err := runhost.NewService[Run](runhost.Config[Run]{
		Kind: runKind, MaxMembers: maxMembers, Store: cfg.Store, Characters: cfg.Characters, Inventories: cfg.Inventories,
		Builder: cfg.Builder, Settler: cfg.Settler, Jobs: cfg.Jobs, Penalty: cfg.Penalty, Clock: cfg.Clock, RNG: cfg.RNG,
		CheckEntry: cfg.CheckEntry, ExitEffects: cfg.ExitEffects, Room: room, CombatRoom: room, SystemRoom: room,
		Blessings: cfg.Blessings, JobAssetRunner: cfg.JobAssetRunner, JobAssetHelpers: cfg.JobAssetHelpers,
		JobAssetDepots: cfg.JobAssetDepots, JobAssetEquipment: cfg.JobAssetEquipment,
	})
	if err != nil {
		return nil, err
	}
	s.Service = host
	return s, nil
}

// SetBanquet sets the celebration hook run after a sealing commits.
func (s *RunService) SetBanquet(hook VictoryBanquetHook) { s.banquet = hook }

// SetBlessings sets the Chapel blessing reader for both the host's combat
// and ＠ふういん's treasure count.
func (s *RunService) SetBlessings(provider runhost.Blessings) {
	s.cfg.Blessings = provider
	s.Service.SetBlessings(provider)
}
