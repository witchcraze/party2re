package pvp

import (
	"context"
	"errors"
	"strconv"

	"github.com/witchcraze/party2re/internal/runhost"
)

// matchLobby is the run host's lobby port over the round-0 Colosseum room.
type matchLobby struct{ s *RunService }

// WithRoster is the first かいし (vs_player.cgi:75–93). Under the room lock it
// commits the handoff transition, which releases the lobby claims and moves
// the bet escrow into the run, in the run host's enrollment transaction.
// Transaction: WithWaitingLobby. Lock Order: runs(0) -> pvp_room_transitions(0) -> characters(2).
func (l matchLobby) WithRoster(ctx context.Context, actor, roomID string, start func(context.Context, runhost.Admission) error) error {
	lobby := l.s.cfg.Lobby
	return lobby.withRoomLock(ctx, roomID, func(ctx context.Context) error {
		last := lastTransitionFromContext(ctx)
		detail, err := lobby.checkInactiveLocked(ctx, roomID)
		if err != nil {
			return err
		}
		if detail.Room.Status != StatusRecruiting || detail.Room.Round > 0 {
			return ErrMatchAlreadyStarted
		}
		if err := checkFirstKaishi(detail, actor); err != nil {
			return err
		}
		ids := []string{actor}
		for _, m := range detail.Members {
			if m.CharacterID != actor {
				ids = append(ids, m.CharacterID)
			}
		}
		a := runhost.Admission{Generation: "party:" + roomID, PartyID: roomID, ActorID: actor, CharacterIDs: ids, StageID: strconv.Itoa(detail.Room.Stage), Speed: detail.Room.Speed, Name: detail.Room.Name}
		now := lobby.now()
		handed := detail
		handed.Room.Status, handed.Room.PrizePool, handed.Room.UpdatedAt = StatusDisbanded, 0, now
		t := Transition{Kind: "host", ActorID: actor, Detail: handed, RemovedIDs: ids, LastActivityAt: now}
		err = l.s.WithWaitingLobby(ctx, roomID, func(ctx context.Context) error {
			_, err := lobby.commitSQL(ctx, t, func(ctx context.Context) error { return start(ctx, a) })
			return err
		})
		if errors.Is(err, ErrRoomInactive) {
			if _, cleanErr := lobby.cleanupInactiveRoom(ctx, last, now); cleanErr != nil {
				return cleanErr
			}
			return ErrRoomNotFound
		}
		return err
	})
}

// checkFirstKaishi applies the first かいし's checks in legacy order: two
// colors, every member's color, then the leader.
func checkFirstKaishi(d RoomDetail, actor string) error {
	colors, unset := map[string]bool{}, false
	for _, m := range d.Members {
		colors[m.TeamColor] = true
		unset = unset || m.TeamColor == ""
	}
	switch {
	case len(colors) <= 1:
		return ErrNeedAtLeastTwoTeams
	case unset:
		return ErrTeamsNotConfigured
	case d.Room.LeaderCharacterID != actor:
		return ErrNotRoomLeader
	}
	return nil
}

// CleanupLobby publishes the committed handoff: the room and its reverse
// mappings go, then the claims are released.
func (l matchLobby) CleanupLobby(ctx context.Context, a runhost.Admission) error {
	return l.s.cfg.Lobby.withRoomLock(ctx, a.PartyID, func(context.Context) error { return nil })
}

// CheckSoloAdmission rejects solo runs: a match always comes from a lobby.
func (matchLobby) CheckSoloAdmission(context.Context, string) error { return runhost.ErrForbidden }
