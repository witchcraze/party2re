package adventure

import (
	"errors"

	"github.com/witchcraze/party2re/internal/runhost"
)

const (
	BossFloor         = 10
	TreasureRoomFloor = 11
	EXBossFloor       = 12
	EXTreasureFloor   = 13
)

var (
	ErrNoParticipants       = runhost.ErrNoParticipants
	ErrInvalidFloorAction   = runhost.ErrInvalidAction
	ErrLeaderOnly           = errors.New("only the party leader can advance the floor")
	ErrTreasureLimitReached = runhost.ErrTreasureLimitReached
	ErrUnconsciousCharacter = runhost.ErrActorUnconscious
	ErrNotParticipant       = runhost.ErrNotParticipant
	ErrNoUnopenedTreasure   = runhost.ErrNoUnopenedTreasure
)
