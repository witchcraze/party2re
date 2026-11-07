package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	"github.com/witchcraze/party2re/internal/character"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/testutil"
)

type referralFixture struct {
	db       *sql.DB
	core     *coreServices
	depots   *database.DepotRepository
	home     *home.Service
	referrer corecharacter.Character
	playerID string
}

type referralRunBuilder struct{}

func (referralRunBuilder) BuildLifetimeActor(context.Context, string, string) (corebattle.LifetimeActor, error) {
	return corebattle.LifetimeActor{}, nil
}

type referralRunSettler struct{}

func (referralRunSettler) ApplyPostBattleResult(context.Context, battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error) {
	return battle.ApplyPostBattleResponse{}, nil
}

type referralRunPenalty struct{}

func (referralRunPenalty) ProjectRunPenalty(context.Context, string, time.Time) error { return nil }

func newReferralFixture(t *testing.T) referralFixture {
	t.Helper()
	db := helperWiringIsolatedDB(t)
	source, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Close(); err != nil {
			t.Error(err)
		}
	}()
	copyTableDefinitions(t, source, db, []string{"news_articles", "character_delivery_notices", "fleamarket_listings"})
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	depots, err := database.NewDepotRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	homes, err := home.NewService(homeRepo, core.charRepo)
	if err != nil {
		t.Fatal(err)
	}
	newsRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	news, err := notification.NewService(newsRepo, newsRepo)
	if err != nil {
		t.Fatal(err)
	}
	adapter := referralDeliveryAdapter{depots: depots, home: homes, items: core.itemCatalog}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	fleaRepo, err := database.NewFleaMarketRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := core.initPlayerAndChar(guildRepo, nil, fleaRepo, news, adapter.DeliverReferral, 0, logging.Nop()); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	referrer, err := database.CreateTestCharacterWithFunds(ctx, db, "Referral recipient", 1000)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	return referralFixture{db, core, depots, homes, referrer, player.ID}
}

func (f referralFixture) create(ctx context.Context, name, referrerID string) (corecharacter.Character, error) {
	return f.core.charService.CreateWithOptions(ctx, f.playerID, name, character.CreationOptions{
		JobID: "job-01", Gender: "m", ReferrerCharacterID: referrerID,
	})
}

func (f referralFixture) assertCounts(t *testing.T, creations, rewards int) {
	t.Helper()
	ctx := context.Background()
	for _, check := range []struct {
		table string
		want  int
	}{
		{"characters", 1 + creations}, {"character_depots", 1 + creations}, {"news_articles", creations},
		{"depot_items", rewards}, {"character_delivery_notices", rewards},
	} {
		var count int
		if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+check.table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != check.want {
			t.Fatalf("%s count=%d, want %d", check.table, count, check.want)
		}
	}
	char, err := f.core.charRepo.FindByID(ctx, f.referrer.ID)
	if err != nil || char.SmallMedals != f.referrer.SmallMedals || char.Money != f.referrer.Money {
		t.Fatalf("referrer currency changed: character=%+v err=%v", char, err)
	}
}

func TestRegistrationReferralLifecycle(t *testing.T) {
	for _, scenario := range []string{"eligible", "ambient_commit", "no_referral", "nonexistent", "missing_depot", "same_account", "full", "overfull", "existing_medal"} {
		t.Run(scenario, func(t *testing.T) {
			f := newReferralFixture(t)
			ctx := context.Background()
			target, want := f.referrer.ID, 1
			switch scenario {
			case "no_referral":
				target, want = "", 0
			case "nonexistent":
				target, want = "nonexistent-character", 0
			case "same_account":
				f.playerID = f.referrer.PlayerID
			case "missing_depot":
				want = 0
				if _, err := f.db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id = ?", f.referrer.ID); err != nil {
					t.Fatal(err)
				}
			}
			originalSlots := 0
			if scenario == "full" || scenario == "overfull" || scenario == "existing_medal" {
				dep, err := f.depots.FindByCharacterID(ctx, f.referrer.ID)
				if err != nil {
					t.Fatal(err)
				}
				originalSlots = dep.Capacity
				if scenario == "overfull" {
					originalSlots++
				}
				for i := 0; i < originalSlots; i++ {
					definition := "weapon-01"
					if scenario == "existing_medal" && i == 0 {
						definition = "item-023"
					}
					instance, err := item.NewInstance(definition, 1)
					if err != nil {
						t.Fatal(err)
					}
					dep.AppendDelivery(instance)
				}
				if err := f.depots.Save(ctx, dep); err != nil {
					t.Fatal(err)
				}
			}
			var created corecharacter.Character
			var err error
			if scenario == "ambient_commit" {
				err = database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
					created, err = f.create(txCtx, "Joining", target)
					if err == nil {
						f.assertCounts(t, 0, 0)
					}
					return err
				})
			} else {
				created, err = f.create(ctx, "Joining", target)
			}
			if err != nil {
				t.Fatal(err)
			}
			if created.SmallMedals != 0 || created.Money != 200 || created.PlayerID != f.playerID {
				t.Fatalf("joining character=%+v", created)
			}
			if _, err := f.create(ctx, "Joining", target); !errors.Is(err, character.ErrNameAlreadyTaken) {
				t.Fatalf("retry must fail without a second reward: %v", err)
			}
			if scenario != "missing_depot" {
				dep, err := f.depots.FindByCharacterID(ctx, f.referrer.ID)
				if err != nil || len(dep.Items) != originalSlots+want || dep.Capacity != depot.DefaultDepotCapacity {
					t.Fatalf("depot=%+v err=%v", dep, err)
				}
				if want == 1 {
					last := dep.Items[len(dep.Items)-1]
					if last.DefinitionID != "item-023" || last.Quantity != 1 || last.EnhancementLevel != 0 {
						t.Fatalf("referral item=%+v", last)
					}
				}
			}
			notices, err := f.home.ListDeliveryNotices(ctx, f.referrer.ID, false)
			if err != nil || len(notices) != want {
				t.Fatalf("notices=%+v err=%v", notices, err)
			}
			if want == 1 && (notices[0].NoticeType != "referral_registration" || !strings.Contains(notices[0].Message, created.Name) || !strings.Contains(notices[0].Message, created.ID) || !strings.Contains(notices[0].Message, "紹介加入")) {
				t.Fatalf("notice does not identify referred registration: %+v", notices[0])
			}
			if originalSlots == 0 && scenario != "missing_depot" {
				f.assertCounts(t, 1, want)
			}
		})
	}
}

func activeReferralRun(t *testing.T, f referralFixture) (*database.NormalAdventureRepository, *adventure.NormalRun) {
	t.Helper()
	ctx := context.Background()
	runRepo, err := database.NewNormalAdventureRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	history, err := database.NewAdventureRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	normalService, err := adventure.NewNormalRunService(adventure.NormalRunConfig{
		Store: runRepo, Characters: f.core.charRepo, Inventories: f.core.invRepo,
		Builder: referralRunBuilder{}, Settler: referralRunSettler{}, History: history,
		Stages: stages, Monsters: monsters, Clock: adventure.RealClock{}, Penalty: referralRunPenalty{},
	})
	if err != nil {
		t.Fatal(err)
	}
	startedAt := time.Now().UTC()
	run := adventure.NormalRun{
		ID: id.New(), Version: 1, Encoding: 1,
		Admission: adventure.NormalAdmission{
			ActorID: f.referrer.ID, CharacterIDs: []string{f.referrer.ID},
			StageID: adventure.StarterAdventure, Mode: "normal", Speed: 3,
		},
		Stage: adventure.Stage{ID: adventure.StarterAdventure}, Mode: "normal",
		LeaderID: f.referrer.ID, Roster: []string{f.referrer.ID},
		Recipients: map[string]adventure.RunRecipient{
			f.referrer.ID: {PlayerID: f.referrer.PlayerID},
		},
		StartedAt: startedAt, LastActivity: startedAt,
	}
	if err := runRepo.WithRunForUpdate(ctx, run.ID, true, func(runCtx context.Context) error {
		if err := runRepo.SaveRun(runCtx, run, 0); err != nil {
			return err
		}
		return runRepo.EnrollRun(runCtx, run)
	}); err != nil {
		t.Fatal(err)
	}
	if active, err := runRepo.FindActorRun(ctx, f.referrer.ID); err != nil || active == nil || active.RunID != run.ID {
		t.Fatalf("expected SQL enrollment in active run %q, got %+v (err=%v)", run.ID, active, err)
	}
	f.core.normalGuard = normalService.CheckMutation
	return runRepo, &run
}

func TestRegistrationReferralToActiveRunRecipient(t *testing.T) {
	for _, failDelivery := range []bool{false, true} {
		name := "success_preserves_run"
		if failDelivery {
			name = "delivery_failure_rolls_back_registration_and_reward"
		}
		t.Run(name, func(t *testing.T) {
			f := newReferralFixture(t)
			ctx := context.Background()
			runRepo, before := activeReferralRun(t, f)
			if err := f.core.normalGuard(ctx, f.referrer.ID); !errors.Is(err, adventure.ErrRunBusy) {
				t.Fatalf("real NormalRunService guard = %v, want %v", err, adventure.ErrRunBusy)
			}
			if failDelivery {
				if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER fail_active_referral BEFORE INSERT ON character_delivery_notices FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected active referral failure'"); err != nil {
					t.Fatal(err)
				}
				defer func() {
					if _, err := f.db.ExecContext(ctx, "DROP TRIGGER fail_active_referral"); err != nil {
						t.Error(err)
					}
				}()
			}

			created, err := f.create(ctx, "Joining", f.referrer.ID)
			if failDelivery {
				if err == nil {
					t.Fatal("expected referral delivery to fail")
				}
				f.assertCounts(t, 0, 0)
			} else {
				if err != nil {
					t.Fatal(err)
				}
				if created.PlayerID != f.playerID {
					t.Fatalf("created character belongs to %q, want %q", created.PlayerID, f.playerID)
				}
				f.assertCounts(t, 1, 1)
			}

			after, err := runRepo.FindRun(ctx, before.ID)
			if err != nil || after == nil || !reflect.DeepEqual(before, after) {
				t.Fatalf("referral changed the recipient's active run: before=%+v after=%+v err=%v", before, after, err)
			}
			active, err := runRepo.FindActorRun(ctx, f.referrer.ID)
			if err != nil || active == nil || active.RunID != before.ID {
				t.Fatalf("referral changed SQL run enrollment: active=%+v err=%v", active, err)
			}
		})
	}
}

func TestRegistrationReferralAtomicity(t *testing.T) {
	for _, stage := range []string{"characters", "character_depots", "depot_items", "character_delivery_notices", "news_articles", "ambient_rollback", "commit_failure"} {
		t.Run(stage, func(t *testing.T) {
			f := newReferralFixture(t)
			ctx := context.Background()
			trigger := stage != "ambient_rollback" && stage != "commit_failure"
			if trigger {
				if _, err := f.db.ExecContext(ctx, "CREATE TRIGGER fail_referral BEFORE INSERT ON "+stage+" FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected referral failure'"); err != nil {
					t.Fatal(err)
				}
			}
			rollback := errors.New("caller rollback")
			var err error
			if trigger {
				_, err = f.create(ctx, "Joining", f.referrer.ID)
			} else {
				err = database.RunInTx(ctx, f.db, func(txCtx context.Context) error {
					if _, err := f.create(txCtx, "Joining", f.referrer.ID); err != nil {
						return err
					}
					// Neither referral delivery nor joining news is visible before commit.
					f.assertCounts(t, 0, 0)
					if stage == "commit_failure" {
						return database.TxFromContext(txCtx).Rollback()
					}
					return rollback
				})
			}
			if err == nil {
				t.Fatal("expected creation failure")
			}
			if stage == "commit_failure" && !errors.Is(err, sql.ErrTxDone) {
				t.Fatalf("commit error=%v", err)
			}
			if stage == "ambient_rollback" && !errors.Is(err, rollback) {
				t.Fatalf("rollback error=%v", err)
			}
			f.assertCounts(t, 0, 0)
			if trigger {
				if _, err := f.db.ExecContext(ctx, "DROP TRIGGER fail_referral"); err != nil {
					t.Fatal(err)
				}
			}
			if _, err := f.create(ctx, "Joining", f.referrer.ID); err != nil {
				t.Fatal(err)
			}
			f.assertCounts(t, 1, 1)
		})
	}
}

func TestRegistrationReferralConcurrentRetry(t *testing.T) {
	f := newReferralFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	// Both workers retry the same name; only one registration can commit.
	results := testutil.RunRace(
		func() error { _, err := f.create(ctx, "Joining", f.referrer.ID); return err },
		func() error { _, err := f.create(ctx, "Joining", f.referrer.ID); return err },
	)
	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if !errors.Is(err, character.ErrNameAlreadyTaken) {
			t.Fatalf("unexpected concurrent retry error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("committed registrations=%d, want 1", successes)
	}
	f.assertCounts(t, 1, 1)
	// Distinct registrations contend for one referrer and must all append once.
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		_, err := f.create(ctx, fmt.Sprintf("R%02d%02d", worker, op), f.referrer.ID)
		return err
	})
	if result.Failures != 0 {
		t.Fatalf("concurrent registrations: %+v", result)
	}
	f.assertCounts(t, 1+int(result.Successes), 1+int(result.Successes))
}
