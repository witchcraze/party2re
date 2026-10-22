package challenge_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sync/atomic"
	"testing"

	valkey "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/tavern"
	"github.com/witchcraze/party2re/internal/testutil"
)

type protocolEngine struct {
	defeat       atomic.Bool
	calls        atomic.Int64
	consumeBonus atomic.Bool
}

func (e *protocolEngine) Resolve(corebattle.Request) (corebattle.Result, error) {
	return corebattle.Result{}, errors.New("party adapter required")
}
func (e *protocolEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.calls.Add(1)
	r := corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies", RemainingHP: map[string]int{}, RemainingMP: map[string]int{}, ConsumedItems: map[string][]corebattle.ConsumedItem{}}
	if e.defeat.Load() {
		r.WinnerSide = "enemies"
	}
	for _, a := range req.Allies {
		r.RemainingHP[a.ID] = a.HP - 10
		r.RemainingMP[a.ID] = max(0, a.MP-2)
		if e.defeat.Load() {
			r.RemainingHP[a.ID] = 0
		}
		if a.MP == 40 {
			r.ConsumedItems[a.ID] = []corebattle.ConsumedItem{{ID: "item-193", Quantity: 1}}
			if e.consumeBonus.Load() {
				r.ConsumedItems[a.ID] = append(r.ConsumedItems[a.ID], corebattle.ConsumedItem{ID: "item-105", Quantity: 1})
			}
		}
	}
	return r, nil
}

type protocolFixture struct {
	db       *sql.DB
	client   valkey.Client
	repo     *database.ChallengeRepository
	chars    *database.CharacterRepository
	store    *challenge.ValkeySessionRepository
	svc      *challenge.Service
	engine   *protocolEngine
	fullness *tavern.Service
	adapter  *battle.Service
	actors   []corecharacter.Character
}

func newProtocolFixture(t *testing.T, n int) *protocolFixture {
	t.Helper()
	f := &protocolFixture{db: participantSettlementDB(t), client: openTestValkey(t), engine: &protocolEngine{}}
	t.Cleanup(f.client.Close)
	var err error
	f.chars, err = database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.repo, err = database.NewChallengeRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.store, err = challenge.NewValkeySessionRepository(f.client)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < n; i++ {
		c, err := database.CreateTestCharacter(ctx, f.db, "Run "+id.New()[:8])
		if err != nil {
			t.Fatal(err)
		}
		c.Level, c.Experience, c.Money, c.Tired = 30, 5000, 100, 0
		c.Stats.HP, c.Stats.MaxHP, c.Stats.MP, c.Stats.MaxMP = 200, 200, 40, 40
		if err := f.chars.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		f.actors = append(f.actors, c)
		for _, def := range []string{"item-193", "item-105"} {
			instance := id.New()
			if _, err := f.db.ExecContext(ctx, `INSERT INTO inventory_items (id,character_id,definition_id,quantity,enhancement_level) VALUES (?,?,?,1,0)`, instance, c.ID, def); err != nil {
				t.Fatal(err)
			}
			if def == "item-105" {
				if _, err := f.db.ExecContext(ctx, `INSERT INTO equipment_slots (character_id,slot,instance_id) VALUES (?,'accessory',?)`, c.ID, instance); err != nil {
					t.Fatal(err)
				}
			}
		}
		if _, err := f.db.ExecContext(ctx, `INSERT INTO tavern_character_status (character_id,is_full,total_meals_eaten,total_gold_spent) VALUES (?,TRUE,7,1234)`, c.ID); err != nil {
			t.Fatal(err)
		}
	}
	inv, err := database.NewInventoryRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(f.chars), battle.WithInventoryRepository(inv), battle.WithEquipmentRepository(equip))
	tavernRepo, err := database.NewTavernRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := tavern.LoadDefaultCatalog()
	if err != nil {
		t.Fatal(err)
	}
	fullness, err := tavern.NewService(catalog, tavernRepo, f.chars, database.NewTransactionProvider(f.db))
	if err != nil {
		t.Fatal(err)
	}
	f.fullness, f.adapter = fullness, adapter
	f.svc, err = challenge.NewService(f.repo, f.chars, f.engine, challenge.WithActiveSessionStore(f.store), challenge.WithAdmission(approvedRoster{}), challenge.WithParticipantBuilder(adapter), challenge.WithFullnessResetter(fullness), challenge.WithCustomTiers(map[string]challenge.ChallengeTier{"test": {ID: "test", MaxParticipants: 4, BaseMonster: challenge.ChallengeMonster{Name: "Test", BaseHP: 1, BaseExp: 20, BaseGold: 5}}}))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, c := range f.actors {
			if err := clearRun(context.Background(), f.store, c.ID); err != nil {
				t.Error(err)
			}
		}
	})
	return f
}

func (f *protocolFixture) start(t *testing.T) *challenge.ChallengeSession {
	t.Helper()
	ctx := context.Background()
	var run *challenge.ChallengeSession
	var err error
	if len(f.actors) == 1 {
		run, err = f.svc.StartSession(ctx, f.actors[0].ID, "test")
	} else {
		ids := make([]string, len(f.actors))
		for i, c := range f.actors {
			ids[i] = c.ID
		}
		run, err = f.svc.StartPartySession(ctx, ids[0], ids, "test", "Party", "")
	}
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (f *protocolFixture) advance(t *testing.T, run *challenge.ChallengeSession) *challenge.ChallengeSession {
	t.Helper()
	_, next, err := f.svc.AdvanceRound(context.Background(), run.CharacterID, run.ID, run.Revision)
	if err != nil {
		t.Fatal(err)
	}
	return next
}

func (f *protocolFixture) assertSettled(t *testing.T, actor int, rounds, attempted int) {
	t.Helper()
	ctx := context.Background()
	original := f.actors[actor]
	got, err := f.chars.FindByID(ctx, original.ID)
	if err != nil {
		t.Fatal(err)
	}
	hp := 200 - 10*rounds
	mp := 40 - 2*rounds
	if attempted > rounds {
		hp = 1
		mp -= 2
	}
	exp, gold := 0, 0
	for r := 1; r <= rounds; r++ {
		exp += int(float64(20)*(1+.1*float64(r)) + .5)
		gold += int(float64(5)*(1+.1*float64(r)) + .5)
	}
	if got.Stats.HP != hp || got.Stats.MP != mp || got.Experience != original.Experience+exp+20*attempted || got.Money != original.Money+gold || got.Tired != attempted*3+1 {
		t.Fatalf("settlement rounds=%d attempted=%d: %+v", rounds, attempted, got)
	}
	var full bool
	var meals, spent int
	if err := f.db.QueryRowContext(ctx, `SELECT is_full,total_meals_eaten,total_gold_spent FROM tavern_character_status WHERE character_id=?`, original.ID).Scan(&full, &meals, &spent); err != nil {
		t.Fatal(err)
	}
	if full || meals != 7 || spent != 1234 {
		t.Fatalf("meal history changed: %v %d %d", full, meals, spent)
	}
	var consumables int
	if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM inventory_items WHERE character_id=? AND definition_id='item-193'`, original.ID).Scan(&consumables); err != nil {
		t.Fatal(err)
	}
	want := 1
	if attempted > 0 {
		want = 0
	}
	if consumables != want {
		t.Fatalf("consumables=%d want=%d", consumables, want)
	}
	record, err := f.repo.FindRecord(ctx, original.ID, "test")
	if err != nil || record == nil || record.TotalAttempts != 1 || record.HighestRound != rounds {
		t.Fatalf("record: %+v %v", record, err)
	}
}

func TestRunProtocolIndividualSettlementIntegration(t *testing.T) {
	for _, n := range []int{1, 3} {
		for _, wins := range []int{0, 1, 3} {
			t.Run(fmt.Sprintf("party%d/wins%d", n, wins), func(t *testing.T) {
				f := newProtocolFixture(t, n)
				ctx := context.Background()
				run := f.start(t)
				for r := 0; r < wins; r++ {
					run = f.advance(t, run)
				}
				before := append([]challenge.ChallengeMember(nil), run.Members...)
				first, err := f.svc.ExitSession(ctx, f.actors[0].ID, run.ID, run.Revision)
				if err != nil {
					t.Fatal(err)
				}
				f.assertSettled(t, 0, wins, wins)
				for i := 1; i < n; i++ {
					got, err := f.chars.FindByID(ctx, f.actors[i].ID)
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(got, f.actors[i]) {
						t.Fatalf("survivor persisted by another exit: %+v", got)
					}
				}
				if n > 1 {
					if first.Session.CharacterID != f.actors[1].ID || !reflect.DeepEqual(first.Session.Members, before[1:]) {
						t.Fatalf("handoff lost state: %+v", first.Session)
					}
					// The former leader's new run cannot overwrite the original canonical body.
					replacement, err := f.svc.StartSession(ctx, f.actors[0].ID, "test")
					if err != nil {
						t.Fatal(err)
					}
					if replacement.ID == run.ID {
						t.Fatal("run identity reused")
					}
					_, err = f.svc.ExitSession(ctx, f.actors[0].ID, run.ID, run.Revision)
					if err != nil {
						t.Fatal(err)
					}
					active, err := f.svc.GetActiveSession(ctx, f.actors[0].ID)
					if err != nil || active.ID != replacement.ID {
						t.Fatalf("late replay removed replacement: %+v %v", active, err)
					}
					run = f.advance(t, &first.Session)
					for i := 1; i < n; i++ {
						out, err := f.svc.ExitSession(ctx, f.actors[i].ID, run.ID, run.Revision)
						if err != nil {
							t.Fatal(err)
						}
						run = &out.Session
						f.assertSettled(t, i, wins+1, wins+1)
					}
				}
			})
		}
	}
}

func TestRunProtocolConcurrentCommandsIntegration(t *testing.T) {
	for _, mode := range []string{"same actor", "different actors", "advance", "defeat"} {
		t.Run(mode, func(t *testing.T) {
			f := newProtocolFixture(t, 2)
			ctx := context.Background()
			run := f.advance(t, f.start(t))
			rev := run.Revision
			if mode == "defeat" {
				f.engine.defeat.Store(true)
			}
			out := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, operation int) error {
				actor := f.actors[0].ID
				if mode == "different actors" && worker%2 == 1 {
					actor = f.actors[1].ID
				}
				var err error
				if (mode == "advance" || mode == "defeat") && worker%2 == 1 {
					_, _, err = f.svc.AdvanceRound(ctx, actor, run.ID, rev)
				} else {
					_, err = f.svc.ExitSession(ctx, actor, run.ID, rev)
				}
				if errors.Is(err, challenge.ErrRevisionConflict) || errors.Is(err, challenge.ErrForbidden) || errors.Is(err, challenge.ErrSessionNotActive) {
					return nil
				}
				return err
			})
			if out.Failures != 0 {
				t.Fatalf("failures=%d", out.Failures)
			}
			for _, actor := range f.actors {
				active, err := f.svc.GetActiveSession(ctx, actor.ID)
				if err != nil {
					t.Fatal(err)
				}
				if active != nil {
					if _, err := f.svc.ExitSession(ctx, actor.ID, run.ID, active.Revision); err != nil {
						t.Fatal(err)
					}
				}
			}
			var settled, records int
			if err := f.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM challenge_participants WHERE run_id=? AND settled=TRUE`, run.ID).Scan(&settled); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRowContext(ctx, `SELECT SUM(total_attempts) FROM character_challenge_records`).Scan(&records); err != nil {
				t.Fatal(err)
			}
			if settled != records || settled != 2 {
				t.Fatalf("payout duplicated or lost: settled=%d attempts=%d", settled, records)
			}
			for _, original := range f.actors {
				var data []byte
				if err := f.db.QueryRowContext(ctx, `SELECT settlement_json FROM challenge_participants WHERE run_id=? AND character_id=?`, run.ID, original.ID).Scan(&data); err != nil {
					t.Fatal(err)
				}
				var receipt challenge.ChallengeMember
				if err := json.Unmarshal(data, &receipt); err != nil {
					t.Fatal(err)
				}
				got, err := f.chars.FindByID(ctx, original.ID)
				if err != nil {
					t.Fatal(err)
				}
				if got.Money != original.Money+receipt.AccumulatedGold+receipt.ExitBonusGold || got.Experience != original.Experience+receipt.AccumulatedExp+receipt.ExitBonusExp || got.Stats.HP != max(1, receipt.CharacterCurrentHP) || got.Stats.MP != *receipt.CharacterCurrentMP {
					t.Fatalf("asset conservation failed: receipt=%+v character=%+v", receipt, got)
				}
			}
		})
	}
}

// A committed pending intent is tested against real SQL and Valkey. Failures are
// injected at real storage boundaries without changing any shared test database.
func TestRunProtocolPublicationRecoveryIntegration(t *testing.T) {
	f := newProtocolFixture(t, 2)
	ctx := context.Background()
	run := f.advance(t, f.start(t))
	actor := f.actors[0].ID
	// Ref deletion must fail after the canonical body was installed.
	ref := fmt.Sprintf("party2:challenge:{char:%s}:member", actor)
	if err := f.client.Do(ctx, f.client.B().Del().Key(ref).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	if err := f.client.Do(ctx, f.client.B().Hset().Key(ref).FieldValue().FieldValue("wrong", "type").Build()).Error(); err != nil {
		t.Fatal(err)
	}
	result, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
	if result == nil || !errors.Is(err, challenge.ErrRecoveryRequired) {
		t.Fatalf("expected committed pending result: %+v %v", result, err)
	}
	f.assertSettled(t, 0, 1, 1)
	for _, c := range f.actors {
		view, err := f.svc.GetActiveSession(ctx, c.ID)
		if err != nil || view == nil || !view.RecoveryPending {
			t.Fatalf("pending visibility: %+v %v", view, err)
		}
		if _, err := f.svc.StartSession(ctx, c.ID, "test"); !errors.Is(err, challenge.ErrActiveSessionExists) {
			t.Fatalf("pending claim admitted another run: %v", err)
		}
	}
	tr, err := f.repo.FindTransition(ctx, run.ID)
	if err != nil || !tr.Pending {
		t.Fatalf("GET acknowledged publication: %+v %v", tr, err)
	}
	if err := f.client.Do(ctx, f.client.B().Del().Key(ref).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	retry, err := f.svc.ExitSession(ctx, actor, run.ID, run.Revision)
	if err != nil || !reflect.DeepEqual(result, retry) {
		t.Fatalf("recovery replay: %+v %v", retry, err)
	}
	f.assertSettled(t, 0, 1, 1)
	// Loss after acknowledged publication remains owned activity, never idle.
	skey := fmt.Sprintf("party2:challenge:{run:%s}:session", run.ID)
	if err := f.client.Do(ctx, f.client.B().Del().Key(skey).Build()).Error(); err != nil {
		t.Fatal(err)
	}
	view, err := f.svc.GetActiveSession(ctx, f.actors[1].ID)
	if err != nil || !view.RecoveryPending {
		t.Fatalf("missing live state treated as idle: %+v %v", view, err)
	}
	if _, err := f.svc.ExitSession(ctx, f.actors[1].ID, run.ID, result.Session.Revision); !errors.Is(err, challenge.ErrRecoveryRequired) {
		t.Fatalf("missing state was invented: %v", err)
	}
	// Remove only known references during teardown after this intentional loss.
	for _, c := range f.actors {
		key := fmt.Sprintf("party2:challenge:{char:%s}:member", c.ID)
		if err := f.client.Do(ctx, f.client.B().Del().Key(key).Build()).Error(); err != nil {
			t.Fatal(err)
		}
	}
}
