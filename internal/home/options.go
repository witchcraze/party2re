package home

import (
	"context"
	mrand "math/rand"
	"time"

	"github.com/witchcraze/party2re/internal/economy"
)

// TransactionRunner defines the interface for running atomic cross-domain transactions.
type TransactionRunner interface {
	ExecuteTransaction(ctx context.Context, req economy.TransactionRequest, fn economy.TransactionCallback) (*economy.TransactionResult, error)
}

type ServiceOption func(*Service)

func WithVisitorLimiter(_ Limiter, _ time.Duration) ServiceOption {
	return func(s *Service) {}
}

func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func WithRNG(rng *mrand.Rand) ServiceOption {
	return func(s *Service) {
		s.rng = rng
	}
}

func WithTimer(t TimerService) ServiceOption {
	return func(s *Service) {
		s.timer = t
	}
}

func WithCharacterUpdater(u CharacterUpdater) ServiceOption {
	return func(s *Service) {
		s.charUpdater = u
	}
}

func WithGuildPoints(g GuildPointsRegistrar) ServiceOption {
	return func(s *Service) {
		s.guildPoints = g
	}
}

func WithInventoryManager(i InventoryManager) ServiceOption {
	return func(s *Service) {
		s.invMgr = i
	}
}

func WithDepotManager(d DepotManager) ServiceOption {
	return func(s *Service) {
		s.depotMgr = d
	}
}

func WithItemCatalog(c ItemCatalog) ServiceOption {
	return func(s *Service) {
		s.catalog = c
	}
}

func WithFullnessResetter(f FullnessResetter) ServiceOption {
	return func(s *Service) {
		s.fullness = f
	}
}

func WithBlessingCleaner(b BlessingCleaner) ServiceOption {
	return func(s *Service) {
		s.chapel = b
	}
}

func WithOnlineCounter(c OnlineCounter) ServiceOption {
	return func(s *Service) {
		s.onlineCounter = c
	}
}

func WithBaseSleepDuration(d time.Duration) ServiceOption {
	return func(s *Service) {
		s.baseSleepDuration = d
	}
}

// WithTransactionRunner sets the transaction runner for atomic operations.
func WithTransactionRunner(runner TransactionRunner) ServiceOption {
	return func(s *Service) {
		s.runner = runner
	}
}

// WithEconomy sets the economy service as the transaction runner.
func WithEconomy(eco *economy.Service) ServiceOption {
	return func(s *Service) {
		s.runner = eco
	}
}
