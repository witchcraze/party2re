package challenge_test

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestRunExitHandoffAndOwnedReplay(t *testing.T) {
	ctx := context.Background()
	repo := newMockChallengeRepo()
	chars := &mockCharRepo{chars: map[string]corecharacter.Character{"lead": createPartyTestChar("lead", 30, 200, 50, 30), "member": createPartyTestChar("member", 30, 200, 50, 30)}}
	svc, err := challenge.NewService(repo, chars, nil, challenge.WithAdmission(approvedRoster{}))
	if err != nil {
		t.Fatal(err)
	}
	run, err := svc.StartPartySession(ctx, "lead", []string{"lead", "member"}, "1", "Party", "")
	if err != nil {
		t.Fatal(err)
	}
	exited, err := svc.ExitSession(ctx, "lead", run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if exited.Session.CharacterID != "member" || len(exited.Session.Members) != 1 || exited.Session.Status != challenge.StatusActive {
		t.Fatalf("handoff: %+v", exited)
	}
	retry, err := svc.ExitSession(ctx, "lead", run.ID, run.Revision)
	if err != nil || retry.Session.Revision != exited.Session.Revision {
		t.Fatalf("retry: %+v %v", retry, err)
	}
	if _, _, err := svc.AdvanceRound(ctx, "lead", run.ID, run.Revision); !errors.Is(err, challenge.ErrRevisionConflict) {
		t.Fatalf("different action reused receipt: %v", err)
	}
	if _, err := svc.GetCommandResult(ctx, "outsider", run.ID, run.Revision); !errors.Is(err, challenge.ErrForbidden) {
		t.Fatalf("receipt ownership: %v", err)
	}
}
