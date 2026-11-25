package party

import (
	"context"
	"errors"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
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

func TestWithNormalRosterAndAdmissionFromRoster(t *testing.T) {
	svc, repo, _, _ := setupTestService(t)
	repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", StageID: "stage-01", Mode: "normal", Speed: 3, Status: StatusRecruiting}
	repo.members["p"] = map[string]Member{
		"member": {
			PartyID: "p", CharacterID: "member", ReadyState: true,
			BattleActor: &corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "member", MaxHP: 150, MaxMP: 120}},
		},
		"leader": {
			PartyID: "p", CharacterID: "leader", IsLeader: true, ReadyState: true,
			BattleActor: &corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "leader", MaxHP: 220, MaxMP: 30}},
		},
	}

	var called bool
	if err := svc.WithNormalRoster(context.Background(), "leader", "p", func(_ context.Context, p Party, members []Member) error {
		called = true
		if len(members) != 2 || members[0].CharacterID != "leader" || members[1].CharacterID != "member" {
			t.Fatalf("unexpected member order: %+v", members)
		}
		adm := NormalAdmissionFromRoster("leader", p, members)
		if len(adm.Actors) != 2 || adm.Actors[0].MaxHP != 220 || adm.Actors[1].MaxMP != 120 {
			t.Fatalf("unexpected NormalAdmission actors: %+v", adm.Actors)
		}
		if adm.Name != "Quest" {
			t.Fatalf("admission name = %q, want the lobby name", adm.Name)
		}
		return nil
	}); err != nil || !called {
		t.Fatalf("WithNormalRoster failed: called=%v err=%v", called, err)
	}

	// Legacy lobby without BattleActor keeps Actors nil.
	legacy := NormalAdmissionFromRoster("leader", repo.parties["p"], []Member{{CharacterID: "leader"}, {CharacterID: "member"}})
	if legacy.Actors != nil {
		t.Fatalf("expected nil Actors for legacy lobby, got %+v", legacy.Actors)
	}
}
