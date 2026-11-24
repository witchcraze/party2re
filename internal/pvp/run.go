package pvp

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/runhost"
)

// runKind namespaces PvP run identities on the shared run host.
const runKind = "pvp"

var (
	ErrMatchFinished = errors.New("the prize has been paid; leave the room to disband")
	ErrRoundLimit    = errors.New("the match ended undecided at the round limit; leave the room to disband")
	ErrNotDecided    = errors.New("more than one team is still standing")
)

// Run is the PvP room library's snapshot: legacy $win, bet.cgi and win.cgi.
type Run struct {
	runhost.State
	TargetWins int `json:"target_wins"`
	// Pool is the bet escrow handed off from the lobby; Paid is give_bet's
	// emptied bet.cgi.
	Pool int  `json:"pool"`
	Paid bool `json:"paid"`
	// Wins counts round wins per team color.
	Wins map[string]int `json:"wins"`
}

func (r *Run) Host() *runhost.State { return &r.State }

type RunConfig struct {
	Store       runhost.Store[Run]
	Characters  runhost.Characters
	Inventories runhost.Inventories
	Builder     runhost.ActorBuilder
	Settler     runhost.Settler
	Jobs        job.DefinitionProvider
	// Lobby is the round-0 room whose first かいし hands the match to the host.
	Lobby       *Service
	Prizes      PrizeRemitter
	Blessings   runhost.Blessings
	Penalty     runhost.Penalty
	Clock       runhost.Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error

	JobAssetRunner    economy.TransactionRunner
	JobAssetHelpers   func(context.Context) ([]int, error)
	JobAssetDepots    runhost.JobAssetDepots
	JobAssetEquipment depot.EquipmentRepository
}

// RunService runs PvP rounds on the shared run host: the host owns commands,
// their combat executor, receipts, exit and cleanup; matchRoom supplies the
// vs_player rules. EnterParty with the lobby room ID is the first かいし.
type RunService struct {
	*runhost.Service[Run, *Run]
	cfg RunConfig
}

func NewRunService(cfg RunConfig) (*RunService, error) {
	if cfg.Characters == nil || cfg.Lobby == nil || cfg.Prizes == nil {
		return nil, runhost.ErrUnavailable
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	s := &RunService{cfg: cfg}
	room := matchRoom{s: s}
	host, err := runhost.NewService[Run](runhost.Config[Run]{
		Kind: runKind, MaxMembers: MaxMembers, Store: cfg.Store, Characters: cfg.Characters, Inventories: cfg.Inventories,
		Builder: cfg.Builder, Settler: cfg.Settler, Jobs: cfg.Jobs, Lobby: matchLobby{s: s}, Penalty: cfg.Penalty,
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
