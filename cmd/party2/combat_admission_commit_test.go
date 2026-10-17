package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/party"
	vk "github.com/witchcraze/party2re/internal/valkey"
)

// commitFault wraps the real transaction provider. "abort" makes the real
// transaction roll back after the start callback published its Valkey state;
// "lost-ack" lets the real transaction commit and then reports an error, as
// when only the commit acknowledgement is lost. Both inject the failure at the
// provider boundary: they do not exercise a real MariaDB commit outage.
type commitFault struct {
	inner interface {
		RunInTx(context.Context, func(context.Context) error) error
	}
	mode string
	err  error
}

func (f commitFault) RunInTx(ctx context.Context, fn func(context.Context) error) error {
	if f.mode == "abort" {
		return f.inner.RunInTx(ctx, func(ctx context.Context) error {
			if err := fn(ctx); err != nil {
				return err
			}
			return f.err
		})
	}
	if err := f.inner.RunInTx(ctx, fn); err != nil {
		return err
	}
	return f.err
}

func TestCombatAdmissionCommitFailureProduction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx := context.Background()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	core, err := newCoreServices(db, client)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, client, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	combat, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	chars := make([]corecharacter.Character, 2)
	for i := range chars {
		char, err := database.CreateTestCharacterWithFunds(ctx, db, "Commit_"+id.New()[:8], 1000)
		if err != nil {
			t.Fatal(err)
		}
		char.Level = 15
		char.Stats.HP = 200
		char.Stats.MaxHP = 200
		if err := core.charRepo.Update(ctx, char); err != nil {
			t.Fatal(err)
		}
		chars[i] = char
		t.Cleanup(func() {
			if err := core.charRepo.Delete(ctx, char.ID); err != nil {
				t.Error(err)
			}
			if err := core.playerRepo.Delete(ctx, char.PlayerID); err != nil {
				t.Error(err)
			}
		})
	}
	leader, member := chars[0], chars[1]
	// Registered after the character cleanup so it runs first: sessions reference characters.
	ds, _ := dungeon.NewValkeyExpeditionRepository(client)
	cs, _ := challenge.NewValkeySessionRepository(client)
	t.Cleanup(func() {
		for _, c := range chars {
			if _, err := db.ExecContext(ctx, "DELETE FROM challenge_sessions WHERE character_id = ?", c.ID); err != nil {
				t.Error(err)
			}
			if err := ds.DeleteActiveExpedition(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := cs.DeleteActiveSession(ctx, c.ID); err != nil {
				t.Error(err)
			}
		}
	})
	lobby, err := combat.party.CreateParty(ctx, leader.ID, party.CreatePartyRequest{Name: "Commit fault", StageID: "stage-00"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := combat.party.DisbandParty(ctx, lobby.Party.ID, leader.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := combat.party.JoinParty(ctx, lobby.Party.ID, member.ID, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := combat.party.SetReady(ctx, lobby.Party.ID, member.ID, true); err != nil {
		t.Fatal(err)
	}
	errInjected := errors.New("injected commit failure")

	busy := func(ctx context.Context, actor string) error {
		exp, err := combat.dungeon.GetActiveExpedition(ctx, actor)
		if err != nil {
			return err
		}
		if exp != nil {
			return dungeon.ErrActiveExpeditionExists
		}
		session, err := combat.challenge.GetActiveSession(ctx, actor)
		if err != nil {
			return err
		}
		if session != nil {
			return challenge.ErrActiveSessionExists
		}
		return nil
	}
	useAdmission := func(mode string) {
		adm := combatAdmission{party: combat.party, characters: core.charRepo, busy: busy,
			tx: commitFault{inner: core.txProvider, mode: mode, err: errInjected}}
		combat.dungeon.ConfigureAdmission(adm)
		combat.challenge.ConfigureAdmission(adm)
	}
	sqlRows := func(characterID string) int {
		var n int
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM challenge_sessions WHERE character_id = ?", characterID).Scan(&n); err != nil {
			t.Fatal(err)
		}
		return n
	}

	t.Run("challenge solo rolled back", func(t *testing.T) {
		useAdmission("abort")
		_, err := combat.challenge.StartSession(ctx, leader.ID, "1")
		if !errors.Is(err, errInjected) || errors.Is(err, challenge.ErrStartOutcomeUnknown) {
			t.Fatalf("confirmed abort must return the commit failure: %v", err)
		}
		if run, err := combat.challenge.GetActiveSession(ctx, leader.ID); err != nil || run != nil || sqlRows(leader.ID) != 0 {
			t.Fatalf("aborted start left state: run=%+v err=%v rows=%d", run, err, sqlRows(leader.ID))
		}
		useAdmission("")
		run, err := combat.challenge.StartSession(ctx, leader.ID, "1")
		if err != nil || run == nil {
			t.Fatalf("retry after abort: %+v %v", run, err)
		}
		if err := cs.DeleteActiveSession(ctx, leader.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("challenge solo lost acknowledgement", func(t *testing.T) {
		useAdmission("lost-ack")
		before := sqlRows(leader.ID)
		run, err := combat.challenge.StartSession(ctx, leader.ID, "1")
		if err != nil || run == nil || sqlRows(leader.ID) != before+1 {
			t.Fatalf("committed start must resolve to its run: %+v %v rows=%d", run, err, sqlRows(leader.ID))
		}
		if active, err := combat.challenge.GetActiveSession(ctx, leader.ID); err != nil || active == nil || active.ID != run.ID {
			t.Fatalf("run not preserved: %+v %v", active, err)
		}
		if _, err := combat.challenge.StartSession(ctx, leader.ID, "1"); !errors.Is(err, challenge.ErrActiveSessionExists) {
			t.Fatalf("no duplicate run: %v", err)
		}
		if err := cs.DeleteActiveSession(ctx, leader.ID); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("challenge party rolled back", func(t *testing.T) {
		useAdmission("abort")
		before := sqlRows(leader.ID)
		if _, err := combat.challenge.StartPartySession(ctx, leader.ID, nil, "1", "Faulty", "#FFFFFF"); !errors.Is(err, errInjected) {
			t.Fatalf("expected the injected commit failure: %v", err)
		}
		for _, c := range chars {
			if run, err := combat.challenge.GetActiveSession(ctx, c.ID); err != nil || run != nil {
				t.Fatalf("%s still busy after abort: %+v %v", c.ID, run, err)
			}
		}
		if sqlRows(leader.ID) != before {
			t.Fatal("aborted start row persisted")
		}
	})
	t.Run("dungeon party lost acknowledgement", func(t *testing.T) {
		useAdmission("lost-ack")
		run, err := combat.dungeon.StartPartyExpedition(ctx, leader.ID, nil, "dungeon-01", lobby.Party.ID)
		if err != nil || run == nil {
			t.Fatalf("published run must be returned: %+v %v", run, err)
		}
		if active, err := combat.dungeon.GetActiveExpedition(ctx, member.ID); err != nil || active == nil || active.ID != run.ID {
			t.Fatalf("member lost the run: %+v %v", active, err)
		}
		if _, err := combat.dungeon.StartPartyExpedition(ctx, leader.ID, nil, "dungeon-01", lobby.Party.ID); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
			t.Fatalf("no duplicate run: %v", err)
		}
		if err := ds.DeleteActiveExpedition(ctx, leader.ID); err != nil {
			t.Fatal(err)
		}
	})
}
