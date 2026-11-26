package party

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
)

func TestRosterConsent(t *testing.T) {
	for _, tc := range []struct {
		name, actor, partyID string
		claimed              []string
		want                 error
	}{
		{"stored roster", "leader", "p", nil, nil},
		{"explicit stored roster", "leader", "p", []string{"member", "leader"}, nil},
		{"fake party", "leader", "fake", nil, ErrNotFound},
		{"nonleader", "member", "p", nil, ErrNotPartyLeader},
		{"unrelated", "leader", "p", []string{"leader", "outsider"}, ErrCharacterNotInParty},
		{"duplicate", "leader", "p", []string{"leader", "leader"}, ErrCharacterNotInParty},
		{"subset", "leader", "p", []string{"leader"}, ErrCharacterNotInParty},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, repo, _, _ := setupTestService(t)
			repo.parties["p"] = Party{ID: "p", LeaderCharacterID: "leader", Status: StatusRecruiting}
			repo.members["p"] = map[string]Member{
				"leader": {PartyID: "p", CharacterID: "leader"},
				"member": {PartyID: "p", CharacterID: "member"},
			}
			called := false
			err := svc.WithRoster(context.Background(), tc.actor, tc.partyID, tc.claimed, func(ctx context.Context, partyID string, ids []string) error {
				called = true
				if partyID != "p" || len(ids) != 2 || ids[0] != "leader" || ids[1] != "member" {
					t.Fatalf("unexpected roster %s %v", partyID, ids)
				}
				if !isPartyAdventureLocked(ctx, "p") {
					t.Fatal("handoff not locked")
				}
				if _, err := svc.KickMember(context.Background(), "p", "leader", "member"); !errors.Is(err, ErrPartyNotRecruiting) {
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
func TestRosterRejectsCorruptMembership(t *testing.T) {
	for _, members := range [][]Member{
		{{PartyID: "p", CharacterID: "leader"}, {PartyID: "p", CharacterID: "leader"}},
		{{PartyID: "other", CharacterID: "leader"}},
		{{PartyID: "p", CharacterID: "outsider"}},
	} {
		svc, repo, _, _ := setupTestService(t)
		repo.parties["p"] = Party{ID: "p", LeaderCharacterID: "leader", Status: StatusRecruiting}
		svc.repo = corruptRoster{Repository: repo, members: members}
		err := svc.WithRoster(context.Background(), "leader", "p", nil, func(context.Context, string, []string) error { t.Fatal("corrupt roster reached start"); return nil })
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
			PartyID: "p", CharacterID: "member",
			BattleActor: &corebattle.LifetimeActor{Participant: corebattle.Participant{ID: "member", MaxHP: 150, MaxMP: 120}},
		},
		"leader": {
			PartyID: "p", CharacterID: "leader", IsLeader: true,
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

type failingMemberRead struct {
	Repository
	err error
}

func (r failingMemberRead) GetMember(context.Context, string, string) (Member, error) {
	return Member{}, r.err
}

func TestKickMemberPropagatesLobbyReadFailure(t *testing.T) {
	svc, repo, _, _ := setupTestService(t)
	repo.parties["p"] = Party{ID: "p", LeaderCharacterID: "leader", Status: StatusRecruiting}
	down := errors.New("valkey down")
	svc.repo = failingMemberRead{Repository: repo, err: down}
	if _, err := svc.KickMember(context.Background(), "p", "leader", "member"); !errors.Is(err, down) {
		t.Fatalf("read failure masked: %v", err)
	}
	svc.repo = repo
	if _, err := svc.KickMember(context.Background(), "p", "leader", "absent"); !errors.Is(err, ErrCharacterNotInParty) {
		t.Fatalf("absent target: %v", err)
	}
}

type recordingKickWait struct {
	waits map[string]time.Time
	err   error
}

func (r *recordingKickWait) ImposeActionWaitUntil(_ context.Context, characterID string, until time.Time) error {
	if r.err != nil {
		return r.err
	}
	if r.waits == nil {
		r.waits = make(map[string]time.Time)
	}
	r.waits[characterID] = until
	return nil
}

type failingRemoveMember struct {
	Repository
	err error
}

func (r failingRemoveMember) RemoveMember(context.Context, string, string) error {
	return r.err
}

func TestKickMemberActionWaitAndConsistency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	t.Run("success imposes 60s wait and returns notice names", func(t *testing.T) {
		waits := &recordingKickWait{}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "冒険の旅", LeaderCharacterID: "leader", Status: StatusRecruiting}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		repo.memberParty["leader"] = "p"
		repo.memberParty["member"] = "p"

		res, err := svc.KickMember(ctx, "p", "leader", "member")
		if err != nil {
			t.Fatalf("KickMember: %v", err)
		}
		if res.PartyName != "冒険の旅" || res.LeaderName != "Leader" {
			t.Fatalf("KickResult = %+v", res)
		}
		if !waits.waits["member"].Equal(now.Add(KickActionWait)) {
			t.Fatalf("member wait = %v, want %v", waits.waits["member"], now.Add(KickActionWait))
		}
		if _, err := repo.GetMember(ctx, "p", "member"); !errors.Is(err, ErrCharacterNotInParty) {
			t.Fatalf("kicked member still in party: %v", err)
		}
	})

	t.Run("non-recruiting rejection skips wait", func(t *testing.T) {
		waits := &recordingKickWait{}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "Active", LeaderCharacterID: "leader", Status: StatusInProgress}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		if _, err := svc.KickMember(ctx, "p", "leader", "member"); !errors.Is(err, ErrPartyNotRecruiting) {
			t.Fatalf("got %v, want ErrPartyNotRecruiting", err)
		}
		if len(waits.waits) != 0 {
			t.Fatalf("rejected kick imposed wait: %+v", waits.waits)
		}
	})

	t.Run("wait write failure restores lobby membership", func(t *testing.T) {
		waitErr := errors.New("timer down")
		waits := &recordingKickWait{err: waitErr}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", Status: StatusRecruiting}
		joinedAt := now.Add(-time.Minute)
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true, JoinedAt: joinedAt},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member", JoinedAt: now},
		}
		repo.memberParty["leader"] = "p"
		repo.memberParty["member"] = "p"

		if _, err := svc.KickMember(ctx, "p", "leader", "member"); !errors.Is(err, waitErr) {
			t.Fatalf("got %v, want %v", err, waitErr)
		}
		if m, err := repo.GetMember(ctx, "p", "member"); err != nil || !m.JoinedAt.Equal(joinedAt.Add(time.Minute)) {
			t.Fatalf("member not restored: %+v err=%v", m, err)
		}
		if _, _, err := repo.GetActivePartyByCharacter(ctx, "member"); err != nil {
			t.Fatalf("character mapping not restored: %v", err)
		}
	})

	t.Run("remove failure skips wait", func(t *testing.T) {
		removeErr := errors.New("lobby write down")
		waits := &recordingKickWait{}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", Status: StatusRecruiting}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		svc.repo = failingRemoveMember{Repository: repo, err: removeErr}

		if _, err := svc.KickMember(ctx, "p", "leader", "member"); !errors.Is(err, removeErr) {
			t.Fatalf("got %v, want %v", err, removeErr)
		}
		if len(waits.waits) != 0 {
			t.Fatalf("failed remove imposed wait: %+v", waits.waits)
		}
	})
}
