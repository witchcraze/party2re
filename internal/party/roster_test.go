package party

import (
	"context"
	"errors"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
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
	repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", StageID: "stage-01", Mode: "normal", Speed: 3, MaxMembers: 3, NeedJoin: "hp_100_o", Status: StatusRecruiting}
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
		if adm.Name != "Quest" || adm.MaxMembers != p.MaxMembers || adm.NeedJoin != p.NeedJoin {
			t.Fatalf("admission name/capacity/condition = %q/%d/%q, want the lobby's", adm.Name, adm.MaxMembers, adm.NeedJoin)
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

type kickOrderRepo struct {
	Repository
	onRemove func() error
	onAdd    func()
}

func (r kickOrderRepo) RemoveMember(ctx context.Context, partyID, characterID string) error {
	if r.onRemove != nil {
		if err := r.onRemove(); err != nil {
			return err
		}
	}
	return r.Repository.RemoveMember(ctx, partyID, characterID)
}

func (r kickOrderRepo) AddMember(ctx context.Context, m Member) error {
	if r.onAdd != nil {
		r.onAdd()
	}
	return r.Repository.AddMember(ctx, m)
}

func TestKickMemberActionWaitAndConsistency(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 26, 12, 0, 0, 0, time.UTC)

	t.Run("success imposes 60s wait before removing member and returns notice names", func(t *testing.T) {
		waits := &recordingKickWait{}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "冒険の旅", LeaderCharacterID: "leader", Status: StatusRecruiting}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		repo.memberParty["leader"] = "p"
		repo.memberParty["member"] = "p"
		svc.repo = kickOrderRepo{
			Repository: repo,
			onRemove: func() error {
				if !waits.waits["member"].Equal(now.Add(KickActionWait)) {
					t.Fatalf("RemoveMember ran before 60s wait was set: %+v", waits.waits)
				}
				return nil
			},
			onAdd: func() { t.Fatal("AddMember must not be called during KickMember") },
		}

		res, err := svc.KickMember(ctx, "p", "leader", "member")
		if err != nil {
			t.Fatalf("KickMember: %v", err)
		}
		if res.PartyName != "冒険の旅" || res.LeaderName != "Leader" {
			t.Fatalf("KickResult = %+v", res)
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

	t.Run("wait write failure leaves lobby intact without remove or compensatory add", func(t *testing.T) {
		waitErr := errors.New("timer down")
		waits := &recordingKickWait{err: waitErr}
		svc, repo, charRepo, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		charRepo.chars["member"] = corecharacter.Character{ID: "member", Name: "Member", Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
		repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", Status: StatusRecruiting, MaxMembers: 4}
		repo.parties["p2"] = Party{ID: "p2", Name: "Other", LeaderCharacterID: "other", Status: StatusRecruiting, MaxMembers: 4}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		repo.members["p2"] = map[string]Member{
			"other": {PartyID: "p2", CharacterID: "other", CharacterName: "Other", IsLeader: true},
		}
		repo.memberParty["leader"] = "p"
		repo.memberParty["member"] = "p"
		repo.memberParty["other"] = "p2"
		svc.repo = kickOrderRepo{
			Repository: repo,
			onRemove:   func() error { t.Fatal("RemoveMember must not run when wait write fails"); return nil },
			onAdd:      func() { t.Fatal("AddMember compensation must not run when wait write fails") },
		}

		if _, err := svc.KickMember(ctx, "p", "leader", "member"); !errors.Is(err, waitErr) {
			t.Fatalf("got %v, want %v", err, waitErr)
		}
		if _, err := repo.GetMember(ctx, "p", "member"); err != nil {
			t.Fatalf("member removed despite wait failure: %v", err)
		}
		if p, _, err := repo.GetActivePartyByCharacter(ctx, "member"); err != nil || p.ID != "p" {
			t.Fatalf("character mapping changed: %+v err=%v", p, err)
		}
		if _, err := svc.JoinParty(ctx, "p2", "member", ""); !errors.Is(err, ErrAlreadyInParty) {
			t.Fatalf("member joined second party after failed kick: %v", err)
		}
	})

	t.Run("remove failure keeps target in lobby without double membership", func(t *testing.T) {
		removeErr := errors.New("lobby write down")
		waits := &recordingKickWait{}
		svc, repo, _, _ := setupTestService(t, WithActionWait(waits), WithClock(func() time.Time { return now }))
		repo.parties["p"] = Party{ID: "p", Name: "Quest", LeaderCharacterID: "leader", Status: StatusRecruiting}
		repo.members["p"] = map[string]Member{
			"leader": {PartyID: "p", CharacterID: "leader", CharacterName: "Leader", IsLeader: true},
			"member": {PartyID: "p", CharacterID: "member", CharacterName: "Member"},
		}
		repo.memberParty["leader"] = "p"
		repo.memberParty["member"] = "p"
		svc.repo = kickOrderRepo{
			Repository: repo,
			onRemove:   func() error { return removeErr },
		}

		if _, err := svc.KickMember(ctx, "p", "leader", "member"); !errors.Is(err, removeErr) {
			t.Fatalf("got %v, want %v", err, removeErr)
		}
		if p, _, err := repo.GetActivePartyByCharacter(ctx, "member"); err != nil || p.ID != "p" {
			t.Fatalf("character mapping lost on remove failure: %+v err=%v", p, err)
		}
	})
}

type joinFaultRepo struct {
	Repository
	addErr           error
	failMembersAfter bool
	membersReadErr   error
	addSucceeded     bool
}

func (r *joinFaultRepo) AddMember(ctx context.Context, m Member) error {
	if r.addErr != nil {
		return r.addErr
	}
	if err := r.Repository.AddMember(ctx, m); err != nil {
		return err
	}
	r.addSucceeded = true
	return nil
}

func (r *joinFaultRepo) GetMembers(ctx context.Context, partyID string) ([]Member, error) {
	if r.failMembersAfter && r.addSucceeded {
		return nil, r.membersReadErr
	}
	return r.Repository.GetMembers(ctx, partyID)
}

func TestJoinPartyPreservesAppliedMembershipWhenPostWriteRosterReadFails(t *testing.T) {
	ctx := context.Background()
	now := time.Date(2026, 11, 27, 12, 0, 0, 0, time.UTC)

	t.Run("applied join succeeds and keeps membership when post-write roster read fails", func(t *testing.T) {
		svc, baseRepo, charRepo, _ := setupTestService(t, WithClock(func() time.Time { return now }))
		charRepo.chars["leader"] = corecharacter.Character{ID: "leader", Name: "Leader", JobID: "job-01", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
		charRepo.chars["joiner"] = corecharacter.Character{ID: "joiner", Name: "Joiner", JobID: "job-02", Level: 4, Stats: corecharacter.Stats{HP: 40, MaxHP: 40}}

		created, err := svc.CreateParty(ctx, "leader", CreatePartyRequest{
			Name: "Quest", StageID: "forest", Speed: 3, MaxMembers: 4, Password: "secret",
		})
		if err != nil {
			t.Fatalf("CreateParty failed: %v", err)
		}

		readErr := errors.New("valkey roster read timeout")
		svc.repo = &joinFaultRepo{
			Repository:       baseRepo,
			failMembersAfter: true,
			membersReadErr:   readErr,
		}

		joined, err := svc.JoinParty(ctx, created.Party.ID, "joiner", "secret")
		if err != nil {
			t.Fatalf("JoinParty failed despite acknowledged AddMember: %v", err)
		}
		if joined.Party.ID != created.Party.ID || len(joined.Members) != 2 {
			t.Fatalf("unexpected joined detail: %+v", joined)
		}
		if joined.Members[0].CharacterID != "leader" || joined.Members[1].CharacterID != "joiner" {
			t.Fatalf("unexpected member order: %+v", joined.Members)
		}

		members, err := baseRepo.GetMembers(ctx, created.Party.ID)
		if err != nil || len(members) != 2 || members[1].CharacterID != "joiner" {
			t.Fatalf("healthy roster = %+v, err=%v", members, err)
		}
		activeParty, activeMember, err := baseRepo.GetActivePartyByCharacter(ctx, "joiner")
		if err != nil || activeParty.ID != created.Party.ID || activeMember.CharacterID != "joiner" {
			t.Fatalf("healthy reverse lookup = %+v / %+v, err=%v", activeParty, activeMember, err)
		}
	})

	t.Run("failed AddMember propagates error without adding membership", func(t *testing.T) {
		svc, baseRepo, charRepo, _ := setupTestService(t, WithClock(func() time.Time { return now }))
		charRepo.chars["leader"] = corecharacter.Character{ID: "leader", Name: "Leader", JobID: "job-01", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
		charRepo.chars["joiner"] = corecharacter.Character{ID: "joiner", Name: "Joiner", JobID: "job-02", Level: 4, Stats: corecharacter.Stats{HP: 40, MaxHP: 40}}

		created, err := svc.CreateParty(ctx, "leader", CreatePartyRequest{
			Name: "Quest", StageID: "forest", Speed: 3, MaxMembers: 4,
		})
		if err != nil {
			t.Fatalf("CreateParty failed: %v", err)
		}

		writeErr := errors.New("valkey write failed")
		svc.repo = &joinFaultRepo{Repository: baseRepo, addErr: writeErr}

		if _, err := svc.JoinParty(ctx, created.Party.ID, "joiner", ""); !errors.Is(err, writeErr) {
			t.Fatalf("expected writeErr, got %v", err)
		}
		members, err := baseRepo.GetMembers(ctx, created.Party.ID)
		if err != nil || len(members) != 1 || members[0].CharacterID != "leader" {
			t.Fatalf("failed AddMember mutated roster: %+v, err=%v", members, err)
		}
		if _, _, err := baseRepo.GetActivePartyByCharacter(ctx, "joiner"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("failed AddMember left reverse lookup: %v", err)
		}
	})
}

type activeLookupFaultRepo struct {
	Repository
	activeErr error
	onSave    func()
	onAdd     func()
}

func (r *activeLookupFaultRepo) GetActivePartyByCharacter(context.Context, string) (Party, Member, error) {
	return Party{}, Member{}, r.activeErr
}

func (r *activeLookupFaultRepo) SaveParty(ctx context.Context, p Party, members ...Member) error {
	if r.onSave != nil {
		r.onSave()
	}
	return r.Repository.SaveParty(ctx, p, members...)
}

func (r *activeLookupFaultRepo) AddMember(ctx context.Context, m Member) error {
	if r.onAdd != nil {
		r.onAdd()
	}
	return r.Repository.AddMember(ctx, m)
}

func TestLobbyEntryPropagatesActivePartyLookupErrors(t *testing.T) {
	ctx := context.Background()
	lookupErr := errors.New("valkey active lookup failed")

	for _, testErr := range []error{lookupErr, ErrCharacterNotInParty} {
		t.Run(testErr.Error(), func(t *testing.T) {
			svc, baseRepo, charRepo, _ := setupTestService(t)
			charRepo.chars["leader"] = corecharacter.Character{ID: "leader", Name: "Leader", JobID: "job-01", Level: 5, Stats: corecharacter.Stats{HP: 50, MaxHP: 50}}
			charRepo.chars["joiner"] = corecharacter.Character{ID: "joiner", Name: "Joiner", JobID: "job-02", Level: 4, Stats: corecharacter.Stats{HP: 40, MaxHP: 40}}

			created, err := svc.CreateParty(ctx, "leader", CreatePartyRequest{
				Name: "Quest", StageID: "forest", Speed: 3, MaxMembers: 4,
			})
			if err != nil {
				t.Fatalf("CreateParty setup failed: %v", err)
			}

			svc.repo = &activeLookupFaultRepo{
				Repository: baseRepo,
				activeErr:  testErr,
				onSave:     func() { t.Fatal("SaveParty must not run on active-party lookup error") },
				onAdd:      func() { t.Fatal("AddMember must not run on active-party lookup error") },
			}

			if _, err := svc.CreateParty(ctx, "joiner", CreatePartyRequest{
				Name: "Second", StageID: "forest", Speed: 3, MaxMembers: 4,
			}); !errors.Is(err, testErr) {
				t.Fatalf("CreateParty error = %v, want %v", err, testErr)
			}

			if _, err := svc.JoinParty(ctx, created.Party.ID, "joiner", ""); !errors.Is(err, testErr) {
				t.Fatalf("JoinParty error = %v, want %v", err, testErr)
			}
		})
	}
}
