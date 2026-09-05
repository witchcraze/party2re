package testutil

import (
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Concurrency harness re-exports
type ConcurrencyStressConfig = testutil.ConcurrencyStressConfig
type ConcurrencyStressResult = testutil.ConcurrencyStressResult

var (
	GetStressConfig         = testutil.GetStressConfig
	RunConcurrentStressTest = testutil.RunConcurrentStressTest
	RunRace                 = testutil.RunRace
	RunRace2                = testutil.RunRace2
	IsDeadlockError         = testutil.IsDeadlockError
)

// Database test fixtures
var (
	CreateTestPlayer             = database.CreateTestPlayer
	CreateTestPlayerWithUsername = database.CreateTestPlayerWithUsername
	CreateTestCharacter          = database.CreateTestCharacter
	CreateTestCharacterWithFunds = database.CreateTestCharacterWithFunds
	CreateTestGuildWithLeader    = database.CreateTestGuildWithLeader
	CreateTestInventoryWithItems = database.CreateTestInventoryWithItems
	CreateTestDepot              = database.CreateTestDepot
)
