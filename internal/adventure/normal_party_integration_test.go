package adventure_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	valkey "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/runhost"
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
	return runhost.ErrBusy
}

func (a *normalPartyAdmission) WithRoster(ctx context.Context, actor, partyID string, fn func(context.Context, runhost.Admission) error) error {
	return a.service.WithNormalRoster(ctx, actor, partyID, func(ctx context.Context, p party.Party, members []party.Member) error {
		return fn(ctx, party.NormalAdmissionFromRoster(actor, p, members))
	})
}
func (a *normalPartyAdmission) CleanupLobby(ctx context.Context, snapshot runhost.Admission) error {
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
	var opts []party.Option
	if builder, ok := f.cfg.Builder.(*battle.Service); ok {
		opts = append(opts, party.WithLifetimeActorBuilder(builder))
	}
	svc, err := party.NewService(repo, f.chars, f.cfg.Stages, opts...)
	if err != nil {
		t.Fatal(err)
	}
	a := &normalPartyAdmission{service: svc, repo: repo}
	f.cfg.Admission = a
	f.restart(t)
	svc.SetLobbyBoundary(f.svc.WithWaitingLobby)
	p, err := svc.CreateParty(context.Background(), f.actors[0].ID, party.CreatePartyRequest{Name: "normal-run", StageID: adventure.StarterAdventure, Mode: "normal", Speed: 3, MaxMembers: 4})
	if err != nil {
		t.Fatal(err)
	}
	// The lobby does not yet advertise Try. Exercise its frozen
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
	if _, err := a.service.LeaveParty(context.Background(), lobby, leader.ID); !errors.Is(err, runhost.ErrBusy) {
		t.Fatal(err)
	}
	if _, err := a.service.KickMember(context.Background(), lobby, leader.ID, f.actors[1].ID); !errors.Is(err, runhost.ErrBusy) {
		t.Fatal(err)
	}
	// Simulate an expired live lobby: SQL admission remains sufficient on restart.
	a.fail.Store(false)
	admission, err := f.store.FindAdmission(context.Background(), entry.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.CleanupLobby(context.Background(), *admission); err != nil {
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
	if _, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "first", Version: r.Version, Action: "advance"}); !errors.Is(err, adventure.ErrLeaderOnly) {
		t.Fatal(err)
	}
	f.send(t, runID, 0, "advance", "")
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Enemies) == 0 {
		t.Fatal(r, err)
	}
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: leader.PlayerID, ActorID: leader.ID, RunID: runID, ID: "living-enemy", Version: r.Version, Action: "advance"}); !errors.Is(err, adventure.ErrInvalidFloorAction) {
		t.Fatal(err)
	}
	f.earn(t, runID, 0)
	before := readStockRoom(t, f, runID).Battle
	f.send(t, runID, 0, "exit", "")
	r, err = f.store.FindRun(context.Background(), runID)
	wantBattle := before
	wantBattle.Actors = before.Actors[1:]
	if err != nil || r.LeaderID != member.ID || len(r.Roster) != 2 || slices.Contains(r.Roster, leader.ID) || !reflect.DeepEqual(wantBattle, r.Battle) {
		t.Fatal("survivor continuation changed", r, err)
	}
	// 5 gold with the three-member cooperative bonus (+20%).
	const peerGold = 6
	if r.Recipients[member.ID].Pending.Gold != peerGold {
		t.Fatal("peer stock lost", r.Recipients[member.ID])
	}
	// Noncombat observation does not release another participant's pending stock.
	f.clock.add(3 * time.Second)
	if _, err := f.svc.Command(context.Background(), runhost.Command{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "screenshot", Version: r.Version, Action: "screenshot"}); !errors.Is(err, runhost.ErrUnavailable) {
		t.Fatal(err)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money {
		t.Fatal(c, err)
	}
	f.send(t, runID, 1, "exit", "")
	r, err = f.store.FindRun(context.Background(), runID)
	wantBattle.Actors = before.Actors[2:]
	if err != nil || r.LeaderID != f.actors[2].ID || len(r.Roster) != 1 || slices.Contains(r.Roster, member.ID) || !reflect.DeepEqual(wantBattle, r.Battle) {
		t.Fatal("second survivor continuation changed", r, err)
	}
	if c, err := f.chars.FindByID(context.Background(), member.ID); err != nil || c.Money != member.Money+peerGold {
		t.Fatal(c, err)
	}
	if _, err := f.svc.GetRunReceipt(context.Background(), member.PlayerID, member.ID, runID, "enroll"); !errors.Is(err, runhost.ErrForbidden) {
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
		_, err := f.svc.EnterSolo(context.Background(), c.PlayerID, c.ID, "start", "solo-quest", adventure.StarterAdventure, "normal", 3)
		return err
	})
	if stats.Failures != 0 {
		t.Fatal(stats)
	}
	var enrollments, runs, receipts int
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM run_enrollments WHERE character_id=?`, c.ID).Scan(&enrollments); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM run_recipients WHERE character_id=?`, c.ID).Scan(&runs); err != nil {
		t.Fatal(err)
	}
	if err := f.db.QueryRow(`SELECT COUNT(*) FROM run_receipts WHERE actor_id=?`, c.ID).Scan(&receipts); err != nil {
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
	f.earn(t, runID, 0)
	f.send(t, runID, 0, "exit", "")
	member := f.actors[1]
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(1800 * time.Second)
	cmd := runhost.Command{PlayerID: member.PlayerID, ActorID: member.ID, RunID: runID, ID: "cleanup", Version: r.Version, Action: "cleanup"}
	if _, err := f.svc.Command(context.Background(), cmd); !errors.Is(err, runhost.ErrConflict) {
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
	if _, err := f.svc.EnterSolo(context.Background(), member.PlayerID, member.ID, "new", "solo-quest", adventure.StarterAdventure, "normal", 3); !errors.Is(err, runhost.ErrBusy) {
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

func TestNormalRunExitExcludesDepartedActorFromEncounterAndEXMarker(t *testing.T) {
	f := newNormalFixture(t, 2)
	installTravel(t, f)
	addTravelTool(t, f, 1, "item-184")

	// 1. Enroll A and B in a normal Party at floor 0.
	_, lobby := f.waitingParty(t, "normal")
	entry, err := f.svc.EnterParty(context.Background(), f.actors[0].PlayerID, f.actors[0].ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID

	// A explicitly exits; B becomes the only Roster member.
	f.send(t, runID, 0, "exit", "")
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Roster) != 1 || r.Roster[0] != f.actors[1].ID {
		t.Fatalf("unexpected roster after exit: %+v %v", r, err)
	}
	if len(r.Battle.Actors) != 1 || r.Battle.Actors[0].ID != f.actors[1].ID {
		t.Fatalf("departed actor remained in battle actors: %+v", r.Battle.Actors)
	}

	// 2. B uses owned item184 with {"destination_stage_id":"stage-23"}, then item107 with {}, then explicitly advances from floor10.
	cmd184 := travelCommand(t, f, runID, 1, `{"destination_stage_id":"stage-23"}`)
	if _, err := f.svc.Command(context.Background(), cmd184); err != nil {
		t.Fatal(err)
	}
	addTravelTool(t, f, 1, "item-107")
	cmd107 := travelCommand(t, f, runID, 1, `{}`)
	if _, err := f.svc.Command(context.Background(), cmd107); err != nil {
		t.Fatal(err)
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || r.Stage.ID != "stage-23" || r.Round != 10 {
		t.Fatalf("expected stage-23 floor 10, got stage %s floor %d: %v", r.Stage.ID, r.Round, err)
	}

	// Advance from floor 10 to floor 11.
	f.send(t, runID, 1, "advance", "")
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}

	// 3. Reload persisted run: len(Roster)=1, len(Battle.Actors)=1, EXUnlocked=false.
	if len(r.Roster) != 1 || len(r.Battle.Actors) != 1 || r.EXUnlocked {
		t.Fatalf("expected solo survivor without EX unlock, got roster=%d actors=%d ex_unlocked=%v", len(r.Roster), len(r.Battle.Actors), r.EXUnlocked)
	}

	// 4. Healthy contrast: in a run where two active participants reach stage-23 floor 10, EX unlocks.
	f2 := newNormalFixture(t, 2)
	installTravel(t, f2)
	addTravelTool(t, f2, 0, "item-184")
	_, lobby2 := f2.waitingParty(t, "normal")
	entry2, err := f2.svc.EnterParty(context.Background(), f2.actors[0].PlayerID, f2.actors[0].ID, lobby2)
	if err != nil {
		t.Fatal(err)
	}
	runID2 := entry2.Command.RunID
	cmd184_2 := travelCommand(t, f2, runID2, 0, `{"destination_stage_id":"stage-23"}`)
	if _, err := f2.svc.Command(context.Background(), cmd184_2); err != nil {
		t.Fatal(err)
	}
	addTravelTool(t, f2, 0, "item-107")
	cmd107_2 := travelCommand(t, f2, runID2, 0, `{}`)
	if _, err := f2.svc.Command(context.Background(), cmd107_2); err != nil {
		t.Fatal(err)
	}
	f2.send(t, runID2, 0, "advance", "")
	r2, err := f2.store.FindRun(context.Background(), runID2)
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Roster) != 2 || len(r2.Battle.Actors) != 2 || !r2.EXUnlocked {
		t.Fatalf("expected 2-player run to unlock EX on stage-23, got roster=%d actors=%d ex_unlocked=%v", len(r2.Roster), len(r2.Battle.Actors), r2.EXUnlocked)
	}
}

func TestNormalRunExitRollbackPreservesBattleMembership(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobby := f.waitingParty(t, "normal")
	leader := f.actors[0]
	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobby)
	if err != nil {
		t.Fatal(err)
	}
	runID := entry.Command.RunID
	r, err := f.store.FindRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	f.clock.add(3 * time.Second)
	cmd := runhost.Command{
		PlayerID: leader.PlayerID,
		ActorID:  leader.ID,
		RunID:    runID,
		ID:       "exit-fail",
		Version:  r.Version,
		Action:   "exit",
	}

	// Trigger write failure to ensure rollback covers active combat membership as well as roster/enrollment.
	f.store.failWrite.Store(true)
	if _, err := f.svc.Command(context.Background(), cmd); err == nil {
		t.Fatal("expected failure on failWrite")
	}
	f.store.failWrite.Store(false)

	// Persisted state after rollback must retain full membership.
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Roster) != 2 || len(r.Battle.Actors) != 2 || r.LeaderID != leader.ID {
		t.Fatalf("rollback did not preserve membership: roster=%d actors=%d leader=%s err=%v", len(r.Roster), len(r.Battle.Actors), r.LeaderID, err)
	}

	// Successful exit after rollback.
	receipt, err := f.svc.Command(context.Background(), cmd)
	if err != nil || receipt.Disposition != "exit" {
		t.Fatalf("expected exit success after rollback: %+v %v", receipt, err)
	}
	r, err = f.store.FindRun(context.Background(), runID)
	if err != nil || len(r.Roster) != 1 || len(r.Battle.Actors) != 1 || r.LeaderID != f.actors[1].ID {
		t.Fatalf("expected 1 member after exit: roster=%d actors=%d leader=%s err=%v", len(r.Roster), len(r.Battle.Actors), r.LeaderID, err)
	}

	// Receipt replay does not apply removal or side effects twice.
	receipt2, err := f.svc.Command(context.Background(), cmd)
	if err != nil || !reflect.DeepEqual(receipt, receipt2) {
		t.Fatalf("receipt replay changed receipt: %+v vs %+v", receipt, receipt2)
	}
	r2, err := f.store.FindRun(context.Background(), runID)
	if err != nil || !reflect.DeepEqual(r, r2) {
		t.Fatalf("receipt replay modified run state: %+v vs %+v", r, r2)
	}
}

func setupCapturedPartyMembers(t *testing.T, f *normalFixture) (*normalPartyAdmission, string) {
	t.Helper()
	ctx := context.Background()
	addr := os.Getenv("PARTY2_VALKEY_ADDR")
	if addr == "" {
		t.Skip("PARTY2_VALKEY_ADDR is required")
	}
	client, err := valkey.NewClient(valkey.ClientOption{InitAddress: []string{addr}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)

	leader := f.actors[0]
	leader.Stats.HP, leader.Stats.MaxHP = 120, 120
	leader.Stats.MP, leader.Stats.MaxMP = 25, 30
	leader.Stats.Attack, leader.Stats.Defense, leader.Stats.Agility = 40, 20, 15
	if err := f.chars.Update(ctx, leader); err != nil {
		t.Fatal(err)
	}
	f.actors[0] = leader

	member := f.actors[1]
	member.Stats.HP, member.Stats.MaxHP = 90, 90
	member.Stats.MP, member.Stats.MaxMP = 45, 50
	member.Stats.Attack, member.Stats.Defense, member.Stats.Agility = 25, 30, 20
	if err := f.chars.Update(ctx, member); err != nil {
		t.Fatal(err)
	}
	f.actors[1] = member

	// Equip leader with weapon-01, armor-01 + held item-113 (+100 MaxHP).
	leaderInv, err := coreinventory.New(leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	w1, err := coreitem.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	a1, err := coreitem.NewInstance("armor-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	bracelet, err := coreitem.NewInstance("item-113", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range []coreitem.Instance{w1, a1, bracelet} {
		if err := leaderInv.Add(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.invs.Save(ctx, leaderInv); err != nil {
		t.Fatal(err)
	}
	leaderEquip, err := coreequipment.New(leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	leaderEquip.Slots[coreitem.SlotMainHand] = w1.ID
	leaderEquip.Slots[coreitem.SlotBody] = a1.ID
	if err := f.equips.Save(ctx, leaderEquip); err != nil {
		t.Fatal(err)
	}

	// Equip member with weapon-01 + held item-114 (+100 MaxMP).
	memberInv, err := coreinventory.New(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	mw1, err := coreitem.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	ring, err := coreitem.NewInstance("item-114", 1)
	if err != nil {
		t.Fatal(err)
	}
	for _, it := range []coreitem.Instance{mw1, ring} {
		if err := memberInv.Add(it); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.invs.Save(ctx, memberInv); err != nil {
		t.Fatal(err)
	}
	memberEquip, err := coreequipment.New(member.ID)
	if err != nil {
		t.Fatal(err)
	}
	memberEquip.Slots[coreitem.SlotMainHand] = mw1.ID
	if err := f.equips.Save(ctx, memberEquip); err != nil {
		t.Fatal(err)
	}

	repo := party.NewValkeyRepository(client)
	svc, err := party.NewService(repo, f.chars, f.cfg.Stages, party.WithLifetimeActorBuilder(f.cfg.Builder.(*battle.Service)))
	if err != nil {
		t.Fatal(err)
	}
	adm := &normalPartyAdmission{service: svc, repo: repo}
	f.cfg.Admission = adm
	f.restart(t)
	svc.SetLobbyBoundary(f.svc.WithWaitingLobby)

	created, err := svc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Name: "captured-party", StageID: adventure.StarterAdventure, Mode: "normal", Speed: 3, MaxMembers: 4})
	if err != nil {
		t.Fatalf("CreateParty failed: %v", err)
	}
	t.Cleanup(func() {
		state := party.LobbyState{Party: created.Party, Members: []party.Member{{CharacterID: leader.ID}, {CharacterID: member.ID}}}
		if err := repo.CleanupParty(context.Background(), state); err != nil {
			t.Error(err)
		}
	})
	if created.Members[0].MaxHP != 220 {
		t.Fatalf("leader lobby MaxHP = %d, want 220 (120 + 113 bonus)", created.Members[0].MaxHP)
	}

	// Mutate leader in store AFTER CreateParty but BEFORE JoinParty/EnterParty.
	leader.Stats.MaxHP = 555
	leader.Stats.Attack = 999
	if err := f.chars.Update(ctx, leader); err != nil {
		t.Fatal(err)
	}
	if err := f.equips.Save(ctx, coreequipment.Equipment{CharacterID: leader.ID}); err != nil {
		t.Fatal(err)
	}
	emptyLeaderInv, _ := coreinventory.New(leader.ID)
	if err := f.invs.Save(ctx, emptyLeaderInv); err != nil {
		t.Fatal(err)
	}

	joined, err := svc.JoinParty(ctx, created.Party.ID, member.ID, "")
	if err != nil {
		t.Fatalf("JoinParty failed: %v", err)
	}
	publicRaw, err := json.Marshal(joined)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(publicRaw), "battle_actor") || strings.Contains(string(publicRaw), "weapon-01") {
		t.Fatalf("public lobby response exposed battle row or gear: %s", string(publicRaw))
	}

	// Mutate member in store AFTER JoinParty but BEFORE EnterParty.
	member.Stats.MaxMP = 777
	member.Stats.Defense = 999
	if err := f.chars.Update(ctx, member); err != nil {
		t.Fatal(err)
	}
	if err := f.equips.Save(ctx, coreequipment.Equipment{CharacterID: member.ID}); err != nil {
		t.Fatal(err)
	}
	emptyMemberInv, _ := coreinventory.New(member.ID)
	if err := f.invs.Save(ctx, emptyMemberInv); err != nil {
		t.Fatal(err)
	}

	return adm, created.Party.ID
}

func assertCapturedPartyActors(t *testing.T, r *adventure.NormalRun, leaderID, memberID string) {
	t.Helper()
	if len(r.Battle.Actors) != 2 {
		t.Fatalf("expected 2 actors, got %d", len(r.Battle.Actors))
	}
	gotLeader, gotMember := r.Battle.Actors[0], r.Battle.Actors[1]
	if gotLeader.ID != leaderID || gotLeader.HP != 120 || gotLeader.MaxHP != 220 || gotLeader.MP != 25 || gotLeader.MaxMP != 30 {
		t.Fatalf("leader initial HP/MP mismatch: %+v", gotLeader)
	}
	if gotLeader.Gear == nil || gotLeader.Gear.WeaponID != "weapon-01" || gotLeader.Gear.ArmorID != "armor-01" {
		t.Fatalf("leader gear mismatch: %+v", gotLeader.Gear)
	}
	if gotLeader.Attack >= 999 {
		t.Fatalf("leader attack used post-join store value: %d", gotLeader.Attack)
	}
	if gotMember.ID != memberID || gotMember.HP != 90 || gotMember.MaxHP != 90 || gotMember.MP != 45 || gotMember.MaxMP != 150 {
		t.Fatalf("member initial HP/MP mismatch: %+v", gotMember)
	}
	if gotMember.Gear == nil || gotMember.Gear.WeaponID != "weapon-01" || gotMember.Gear.ArmorID != "" {
		t.Fatalf("member gear mismatch: %+v", gotMember.Gear)
	}
	if gotMember.Defense >= 999 {
		t.Fatalf("member defense used post-join store value: %d", gotMember.Defense)
	}
}

func TestNormalRunPartyCapturesBattleRowsAtCreateAndJoin(t *testing.T) {
	f := newNormalFixture(t, 2)
	_, lobbyID := setupCapturedPartyMembers(t, f)
	leader, member := f.actors[0], f.actors[1]

	entry, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobbyID)
	if err != nil {
		t.Fatalf("EnterParty failed: %v", err)
	}
	r, err := f.store.FindRun(context.Background(), entry.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPartyActors(t, r, leader.ID, member.ID)
}

func TestNormalRunPartyCapturedBattleRowStartRetry(t *testing.T) {
	f := newNormalFixture(t, 2)
	adm, lobbyID := setupCapturedPartyMembers(t, f)
	leader, member := f.actors[0], f.actors[1]

	// Lose SQL commit ack and fail Valkey cleanup on first EnterParty attempt.
	f.store.loseCommit.Store(true)
	adm.fail.Store(true)
	first, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobbyID)
	if err == nil || first.Version != 1 {
		t.Fatalf("expected cleanup error with committed version 1, got receipt=%+v err=%v", first, err)
	}

	// Delete Valkey lobby to simulate expiry before retry, then restart service and retry.
	adm.fail.Store(false)
	savedAdm, err := f.store.FindAdmission(context.Background(), first.Command.RunID)
	if err != nil || savedAdm == nil {
		t.Fatalf("FindAdmission failed: %+v %v", savedAdm, err)
	}
	if err := adm.CleanupLobby(context.Background(), *savedAdm); err != nil {
		t.Fatal(err)
	}
	f.restart(t)

	retried, err := f.svc.EnterParty(context.Background(), leader.PlayerID, leader.ID, lobbyID)
	if err != nil {
		t.Fatalf("EnterParty retry failed: %v", err)
	}
	if !reflect.DeepEqual(first, retried) {
		t.Fatalf("retry receipt mismatch: first=%+v retried=%+v", first, retried)
	}
	r, err := f.store.FindRun(context.Background(), retried.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	assertCapturedPartyActors(t, r, leader.ID, member.ID)
}

func TestNormalRunPartyLegacyLobbyFallback(t *testing.T) {
	f := newNormalFixture(t, 2)
	ctx := context.Background()
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
	// Legacy party service without ParticipantBuilder simulates pre-cutover lobbies in Valkey without BattleActor.
	legacySvc, err := party.NewService(repo, f.chars, f.cfg.Stages)
	if err != nil {
		t.Fatal(err)
	}
	adm := &normalPartyAdmission{service: legacySvc, repo: repo}
	f.cfg.Admission = adm
	f.restart(t)
	legacySvc.SetLobbyBoundary(f.svc.WithWaitingLobby)

	leader, member := f.actors[0], f.actors[1]
	p, err := legacySvc.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Name: "legacy-lobby", StageID: adventure.StarterAdventure, Mode: "normal", Speed: 3, MaxMembers: 4})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		state := party.LobbyState{Party: p.Party, Members: []party.Member{{CharacterID: leader.ID}, {CharacterID: member.ID}}}
		if err := repo.CleanupParty(context.Background(), state); err != nil {
			t.Error(err)
		}
	})
	if _, err := legacySvc.JoinParty(ctx, p.Party.ID, member.ID, ""); err != nil {
		t.Fatal(err)
	}

	// Verify stored lobby members have no BattleActor, and equip an item before EnterParty.
	members, err := repo.GetMembers(ctx, p.Party.ID)
	if err != nil || len(members) != 2 || members[0].BattleActor != nil || members[1].BattleActor != nil {
		t.Fatalf("expected legacy lobby without BattleActor, got %+v err=%v", members, err)
	}
	inv, err := coreinventory.New(leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	w1, err := coreitem.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := inv.Add(w1); err != nil {
		t.Fatal(err)
	}
	if err := f.invs.Save(ctx, inv); err != nil {
		t.Fatal(err)
	}
	eq, err := coreequipment.New(leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	eq.Slots[coreitem.SlotMainHand] = w1.ID
	if err := f.equips.Save(ctx, eq); err != nil {
		t.Fatal(err)
	}

	entry, err := f.svc.EnterParty(ctx, leader.PlayerID, leader.ID, p.Party.ID)
	if err != nil {
		t.Fatalf("EnterParty legacy fallback failed: %v", err)
	}
	r, err := f.store.FindRun(ctx, entry.Command.RunID)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Battle.Actors) != 2 || r.Battle.Actors[0].Gear == nil || r.Battle.Actors[0].Gear.WeaponID != "weapon-01" {
		t.Fatalf("expected legacy fallback to build actors at start, got %+v", r.Battle.Actors)
	}
}
