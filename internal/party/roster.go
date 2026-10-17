package party

import (
	"context"
	"errors"
)

var ErrMemberBusy = errors.New("party member is busy")

// SetMemberActivityCheck prevents the ordinary Party start from bypassing
// Dungeon/Challenge enrollment after the lobby handoff.
func (s *Service) SetMemberActivityCheck(check func(context.Context, string) error) {
	s.activityCheck = check
}

// WithReadyRoster validates consent from the live lobby and serializes its handoff.
// The callback receives the stored roster, with the leader first.
func (s *Service) WithReadyRoster(ctx context.Context, actor, partyID string, claimed []string, start func(context.Context, string, []string) error) error {
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
		if detail.Party.Status != StatusRecruiting {
			return ErrPartyNotRecruiting
		}
		seen := make(map[string]bool, len(detail.Members))
		ids := []string{actor}
		for _, m := range detail.Members {
			if m.CharacterID == "" || m.PartyID != partyID || seen[m.CharacterID] {
				return ErrCharacterNotInParty
			}
			seen[m.CharacterID] = true
			if !m.ReadyState {
				return ErrPartyNotReady
			}
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
		return start(ctx, partyID, ids)
	})
}

func (s *Service) withLobbyMutation(ctx context.Context, partyID string, fn func(context.Context) error) error {
	return s.repo.WithPartyAdventureLock(ctx, partyID, func(ctx context.Context) error {
		return s.runInTx(ctx, fn)
	})
}
