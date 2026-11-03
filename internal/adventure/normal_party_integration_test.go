package adventure_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"sync/atomic"
	"testing"
	"time"

	valkey "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/testutil"
)

type normalPartyAdmission struct {
	service *party.Service
	repo    *party.ValkeyRepository
	fail    atomic.Bool
}

func (a *normalPartyAdmission) CheckSoloAdmission(ctx context.Context, actor string) error {
	_, _, err := a.service.GetActiveParty(ctx, actor)
	if errors.Is(err, party.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return adventure.ErrRunBusy
}

func (a *normalPartyAdmission) WithNormalRoster(ctx context.Context, actor, partyID string, fn func(context.Context, adventure.NormalAdmission) error) error {
	return a.service.WithNormalRoster(ctx, actor, partyID, func(ctx context.Context, p party.Party, ids []string) error {
		return fn(ctx, adventure.NormalAdmission{Generation: "party:" + p.ID, PartyID: p.ID, ActorID: actor, CharacterIDs: ids, StageID: p.StageID, Season: p.Season, Mode: p.Mode, Speed: p.Speed})
	})
}
func (a *normalPartyAdmission) CleanupNormalLobby(ctx context.Context, snapshot adventure.NormalAdmission) error {
	if a.fail.Load() {
		return errors.New("Valkey cleanup unavailable")
	}
	lobby := party.LobbyState{Party: party.Party{ID: snapshot.PartyID, LeaderCharacterID: snapshot.ActorID}}
	for _, id := range snapshot.CharacterIDs {
		lobby.Members = append(lobby.Members, party.Member{CharacterID: id})
	}
	return a.repo.CleanupParty(ctx, lobby)
}

func (f *normalFixture) waitingParty(t *testing.T, mode string) (*normalPartyAdmission, string) {
	t.Helper()
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is required")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	repo := party.NewValkeyRepository(client)
	svc, err := party.NewService(repo, f.chars, f.invs, f.cfg.Stages, f.cfg.Monsters, nil)
	if err != nil {
		t.Fatal(err)
	}
	a := &normalPartyAdmission{service: svc, repo: repo}
	f.cfg.Admission = a
	f.restart(t)
	svc.SetLobbyBoundary(f.svc.WithWaitingLobby)
	p, err := svc.CreateParty(context.Background(), f.actors[0].ID, party.CreatePartyRequest{Name: "normal run", StageID: adventure.StarterAdventure, Mode: "normal", Speed: 3, MaxMembers: 4})
	if err != nil {
		t.Fatal(err)
	}
	// The retained batch lobby does not yet advertise Try. Exercise its frozen
	// admission contract without enabling a production start before #1330.
	if mode != "normal" {
		p.Party.Mode = mode
		if err := repo.UpdateParty(context.Background(), p.Party); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range f.actors[1:] {
		if _, err := svc.JoinParty(context.Background(), p.Party.ID, c.ID, ""); err != nil {
			t.Fatal(err)
		}
		if _, err := svc.SetReady(context.Background(), p.Party.ID, c.ID, true); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		state := party.LobbyState{Party: p.Party}
		for _, c := range f.actors {
			state.Members = append(state.Members, party.Member{CharacterID: c.ID})
		}
		if err := repo.CleanupParty(context.Background(), state); err != nil {
			t.Error(err)
		}
	})
	return a, p.Party.ID
}

func TestNormalRunPartyHandoffAndIndividualExit(t *testing.T) {
	f := newNormalFixture(t, 3)
	a, lobby := f.waitingParty(t, "normal")
	leader := f.actors[0]
	a.fail.Store(true)
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err == nil || entry.Version != 1 {
		t.Fatal(entry, err)
	}
	if _, err := a.service.SetReady(context.Background(), lobby, f.actors[1].ID, false); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal("handed-off roster mutated", err)
	}
	if err := a.service.LeaveParty(context.Background(), lobby, leader.ID); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal(err)
	}
	if err := a.service.KickMember(context.Background(), lobby, leader.ID, f.actors[1].ID); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal(err)
	}
	if err := a.service.DisbandParty(context.Background(), lobby, leader.ID); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal(err)
	}
	// Simulate an expired live lobby: SQL admission remains sufficient on restart.
	a.fail.Store(false)
	admission, err := f.store.FindAdmission(context.Background(), entry.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CleanupNormalLobby(context.Background(), *admission); err != nil {
		t.Fatal(err)
	}
	f.restart(t)
	again, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil || !reflect.DeepEqual(entry, again) {
		t.Fatal(again, err)
	}
	runID := entry.Command.RunID
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	member := f.actors[1]
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "first", Version: r.Version, Action: "advance"}); !errors.Is(err, adventure.ErrLeaderOnly) {
		t.Fatal(err)
	}
	f.send(t, runID, 0, "advance", "")
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Enemies) == 0 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), adventure.RunCommand{PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: runID, ID: "living-enemy", Version: r.Version, Action: "advance"}); !errors.Is(err, adventure.ErrInvalidFloorAction) {
		t.Fatal(err)
	}
	f.send(t, runID, 0, "earn", "")
	before := r.Battle
	f.send(t, runID, 0, "exit", "")
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || r.LeaderID != member.ID || len(r.Roster) != 2 || !reflect.DeepEqual(before, r.Battle) {
		t.Fatal("survivor continuation changed", r, err)
	}
	if r.Recipients[member.ID].Pending.Gold != 5 {
		t.Fatal("peer stock lost")
	}
	// Noncombat observation does not release another participant's pending stock.
	f.send(t, runID, 1, "screenshot", "")
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money {
		t.Fatal(c, err)
	}
	f.send(t, runID, 1, "exit", "")
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money+5 {
		t.Fatal(c, err)
	}
	if _, err := f.svc.GetRunReceipt(context.Background(), member.PlayerID, member.ID, runID, "enroll"); !errors.Is(err, adventure.ErrRunForbidden) {
		t.Fatal("another actor's receipt exposed", err)
	}
}

func TestNormalRunPartyEnrollmentWriteFailureRollsBackOuterTransaction(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	c := f.actors[0]
	f.store.failWrite.Store(true)
	if _, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby); err == nil {
		t.Fatal("uncommitted receipt hid enrollment failure")
	}
	for _, c := range f.actors {
		if run, err := f.svc.GetActorRun(context.Background(), c.PlayerID, c.ID); err != nil || run != nil {
			t.Fatal("failed admission retained ownership", run, err)
		}
	}
	f.store.failWrite.Store(false)
	f.store.loseCommit.Store(true)
	entry, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby)
	if err != nil || entry.Version != 1 {
		t.Fatal(entry, err)
	}
}

func TestNormalRunPairedAdmissionStress(t *testing.T) {
	f := newNormalFixture(t, 1)
	c := f.actors[0]
	stats := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(int, int) error {
		_, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "start", adventure.StarterAdventure, "normal", 3)
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	var enrollments, runs, receipts int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM normal_adventure_enrollments WHERE character_id=?`, c.ID).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM normal_adventure_recipients WHERE character_id=?`, c.ID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM normal_adventure_receipts WHERE actor_id=?`, c.ID).Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if enrollments != 1 || runs != 1 || receipts != 1 {
		t.Fatal(enrollments, runs, receipts)
	}
}

func TestNormalRunTimeoutPendingRecoveryAndRetention(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "try")
	c := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), c.PlayerID, c.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	f.send(t, runID, 0, "earn", "")
	f.send(t, runID, 0, "exit", "")
	member := f.actors[1]
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(1800 * time.Second)
	cmd := adventure.RunCommand{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "cleanup", Version: r.Version, Action: "cleanup"}
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, adventure.ErrRunConflict) {
		t.Fatal("exact idle boundary accepted", err)
	}
	f.clock.add(time.Nanosecond)
	f.penalty.fail.Store(true)
	result, err := f.svc.Command(context.Background(), cmd)
	if err == nil || result.Disposition != "timeout" {
		t.Fatal(result, err)
	}
	deadline := result.Recipient.PenaltyUntil
	if !deadline.Equal(f.clock.Now().Add(time.Hour)) || result.Recipient.Conditional.Gold != 5 {
		t.Fatal(result)
	}
	active, err := f.svc.GetActorRun(context.Background(), member.PlayerID, member.ID)
	if err != nil || active == nil || !active.Recipient.TimerPending {
		t.Fatal(active, err)
	}
	if _, err := f.svc.EnterSolo(context.Background(), member.PlayerID, member.ID, "new", adventure.StarterAdventure, "normal", 3); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal(err)
	}
	departed, err := f.chars.FindByID(context.Background(), c.ID)
	if err != nil || departed.PendingWake {
		t.Fatal("departed participant penalized", departed, err)
	}
	remaining, err := f.chars.FindByID(context.Background(), member.ID)
	if err != nil || !remaining.PendingWake || remaining.Money != member.Money || remaining.Tired != member.Tired {
		t.Fatal(remaining, err)
	}
	f.clock.add(10 * time.Minute)
	f.penalty.fail.Store(false)
	f.restart(t)
	if err := f.svc.RecoverTimeout(context.Background(), member.PlayerID, member.ID, runID); err != nil {
		t.Fatal(err)
	}
	if active, err := f.svc.GetActorRun(context.Background(), member.PlayerID, member.ID); err != nil || active != nil {
		t.Fatal(active, err)
	}
	facts, err := f.svc.GetActorPenalty(context.Background(), member.PlayerID, member.ID)
	if err != nil || !facts.Recipient.PenaltyUntil.Equal(deadline) || facts.Recipient.Conditional.Gold != 5 || facts.Recipient.TimerPending {
		t.Fatal(facts, err)
	}
	if _, err := f.svc.Command(context.Background(), cmd); err != nil {
		t.Fatal(err)
	}
	if time.Unix(0, f.penalty.deadline.Load()).UTC() != deadline {
		t.Fatal("retry extended deadline")
	}
}
