package party

import (
	"context"
	"errors"

	"github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

var ErrMemberBusy = errors.New("party member is busy")

// SetMemberActivityCheck prevents the ordinary Party start from bypassing
// Dungeon/Challenge enrollment after the lobby handoff.
func (s *Service) SetMemberActivityCheck(check func(context.Context, string) error) {
	s.activityCheck = check
}

// WithRoster validates the live lobby roster and serializes its handoff.
// The callback receives the stored roster, with the leader first.
func (s *Service) WithRoster(ctx context.Context, actor, partyID string, claimed []string, start func(context.Context, string, []string) error) error {
	return s.withRosterDetail(ctx, actor, partyID, claimed, func(ctx context.Context, detail PartyDetail, ids []string) error {
		return start(ctx, detail.Party.ID, ids)
	})
}

func (s *Service) withRosterDetail(ctx context.Context, actor, partyID string, claimed []string, start func(context.Context, PartyDetail, []string) error) error {
	if partyID == "" {
		p, _, err := s.GetActiveParty(ctx, actor)
		if err != nil {
			return err
		}
		partyID = p.ID
	}
	return s.repo.WithPartyAdventureLock(ctx, partyID, func(ctx context.Context) error {
		detail, err := s.GetParty(ctx, partyID)
		if err != nil {
			return err
		}
		if detail.Party.ID != partyID {
			return ErrNotFound
		}
		if detail.Party.LeaderCharacterID != actor {
			return ErrNotPartyLeader
		}
		if detail.Party.Status != StatusRecruiting || detail.Party.sleepUntil != 0 {
			return ErrPartyNotRecruiting
		}
		seen := make(map[string]bool, len(detail.Members))
		ids := []string{actor}
		for _, m := range detail.Members {
			if m.CharacterID == "" || m.PartyID != partyID || seen[m.CharacterID] {
				return ErrCharacterNotInParty
			}
			seen[m.CharacterID] = true
			if m.CharacterID != actor {
				ids = append(ids, m.CharacterID)
			}
		}
		if !seen[actor] || len(ids) > MaxPartyMembers {
			return ErrCharacterNotInParty
		}
		if len(claimed) > 0 {
			if len(claimed) != len(ids) {
				return ErrCharacterNotInParty
			}
			for _, id := range claimed {
				if !seen[id] {
					return ErrCharacterNotInParty
				}
				delete(seen, id)
			}
		}
		return start(ctx, detail, ids)
	})
}

func (s *Service) withLobbyMutation(ctx context.Context, partyID string, fn func(context.Context) error) error {
	return s.repo.WithPartyAdventureLock(ctx, partyID, func(ctx context.Context) error {
		return s.withWaitingLobbyTx(ctx, partyID, fn)
	})
}

func (s *Service) withWaitingLobbyTx(ctx context.Context, partyID string, fn func(context.Context) error) error {
	return s.withWaitingLobby(ctx, partyID, func(ctx context.Context) error { return s.runInTx(ctx, fn) })
}

func (s *Service) SetLobbyBoundary(boundary func(context.Context, string, func(context.Context) error) error) {
	s.lobbyBoundary = boundary
}

func (s *Service) withWaitingLobby(ctx context.Context, partyID string, fn func(context.Context) error) error {
	if s.lobbyBoundary != nil {
		return s.lobbyBoundary(ctx, partyID, fn)
	}
	return fn(ctx)
}

// WithNormalRoster validates the live roster inside the durable handoff lock.
// The callback receives the party and members ordered with the leader first.
func (s *Service) WithNormalRoster(ctx context.Context, actor, partyID string, start func(context.Context, Party, []Member) error) error {
	return s.repo.WithPartyAdventureLock(ctx, partyID, func(ctx context.Context) error {
		return s.withWaitingLobby(ctx, partyID, func(ctx context.Context) error {
			return s.withRosterDetail(ctx, actor, partyID, nil, func(ctx context.Context, detail PartyDetail, ids []string) error {
				byID := make(map[string]Member, len(detail.Members))
				for _, m := range detail.Members {
					byID[m.CharacterID] = m
				}
				ordered := make([]Member, 0, len(ids))
				for _, id := range ids {
					ordered = append(ordered, byID[id])
				}
				return start(ctx, detail.Party, ordered)
			})
		})
	})
}

// NormalAdmissionFromRoster builds a NormalRun admission from a locked lobby roster.
func NormalAdmissionFromRoster(actor string, p Party, members []Member) runhost.Admission {
	ids := make([]string, len(members))
	hasCaptured := false
	for i, m := range members {
		ids[i] = m.CharacterID
		if m.BattleActor != nil {
			hasCaptured = true
		}
	}
	var actors []battle.LifetimeActor
	if hasCaptured {
		actors = make([]battle.LifetimeActor, len(members))
		for i, m := range members {
			if m.BattleActor != nil {
				actors[i] = m.BattleActor.Clone()
			}
		}
	}
	return runhost.Admission{
		Generation:   "party:" + p.ID,
		PartyID:      p.ID,
		ActorID:      actor,
		CharacterIDs: ids,
		Actors:       actors,
		StageID:      p.StageID,
		Season:       p.Season,
		Mode:         p.Mode,
		Speed:        p.Speed,
		Name:         p.Name,
		MaxMembers:   p.MaxMembers,
		NeedJoin:     p.NeedJoin,
	}
}
