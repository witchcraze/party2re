package party

import (
	"context"
	"errors"
	"testing"
)

func TestReadyRosterConsent(t *testing.T) {
	for _, tc := range []struct {
		name, actor, partyID string
		claimed              []string
		ready                bool
		want                 error
	}{
		{"stored roster", "leader", "p", nil, true, nil},
		{"explicit stored roster", "leader", "p", []string{"member", "leader"}, true, nil},
		{"fake party", "leader", "fake", nil, true, ErrNotFound},
		{"nonleader", "member", "p", nil, true, ErrNotPartyLeader},
		{"unready", "leader", "p", nil, false, ErrPartyNotReady},
		{"unrelated", "leader", "p", []string{"leader", "outsider"}, true, ErrCharacterNotInParty},
		{"duplicate", "leader", "p", []string{"leader", "leader"}, true, ErrCharacterNotInParty},
		{"subset", "leader", "p", []string{"leader"}, true, ErrCharacterNotInParty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := setupTestService(t)
			repo.parties["p"] = Party{ID: "p", LeaderCharacterID: "leader", Status: StatusRecruiting}
			repo.members["p"] = map[string]Member{
				"leader": {PartyID: "p", CharacterID: "leader", ReadyState: true},
				"member": {PartyID: "p", CharacterID: "member", ReadyState: tc.ready},
			}
			called := false
			err := svc.WithReadyRoster(context.Background(), tc.actor, tc.partyID, tc.claimed, func(ctx context.Context, partyID string, ids []string) error {
				called = true
				if partyID != "p" || len(ids) != 2 || ids[0] != "leader" || ids[1] != "member" {
					t.Fatalf("unexpected roster %s %v", partyID, ids)
				}
				if !isPartyAdventureLocked(ctx, "p") {
					t.Fatal("handoff not locked")
				}
				if err := svc.KickMember(context.Background(), "p", "leader", "member"); !errors.Is(err, ErrPartyNotRecruiting) {
					t.Fatal(err)
				}
				return nil
			})
			if !errors.Is(err, tc.want) || called != (tc.want == nil) {
				t.Fatalf("called=%v err=%v want=%v", called, err, tc.want)
			}
		})
	}
}

type corruptRoster struct {
	Repository
	members []Member
}

func (r corruptRoster) GetMembers(context.Context, string) ([]Member, error) { return r.members, nil }
func TestReadyRosterRejectsCorruptMembership(t *testing.T) {
	for _, members := range [][]Member{
		{{PartyID: "p", CharacterID: "leader", ReadyState: true}, {PartyID: "p", CharacterID: "leader", ReadyState: true}},
		{{PartyID: "other", CharacterID: "leader", ReadyState: true}},
		{{PartyID: "p", CharacterID: "outsider", ReadyState: true}},
	} {
		svc, repo, _, _ := setupTestService(t)
		repo.parties["p"] = Party{ID: "p", LeaderCharacterID: "leader", Status: StatusRecruiting}
		svc.repo = corruptRoster{Repository: repo, members: members}
		err := svc.WithReadyRoster(context.Background(), "leader", "p", nil, func(context.Context, string, []string) error { t.Fatal("corrupt roster reached start"); return nil })
		if !errors.Is(err, ErrCharacterNotInParty) {
			t.Fatalf("corrupt roster accepted: %v", err)
		}
	}
}
