package dungeon_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

func seedPreUpdateWipedOutValkey(
	t *testing.T,
	ctx context.Context,
	store dungeon.ActiveExpeditionStore,
	run dungeon.ActiveExpedition,
) {
	t.Helper()
	if err := store.SaveActiveExpedition(ctx, run); err != nil {
		t.Fatalf("seed pre-update wiped_out snapshot: %v", err)
	}
}

func TestLegacyWipedOutBuffer_ContinuationAndRetry(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		for _, resumeVia := range []string{"escape", "fight"} {
			t.Run(kind+"/"+resumeVia, func(t *testing.T) {
				ctx := context.Background()
				store := &failedCleanupStore{ActiveExpeditionStore: encounterStore(t, kind)}
				lead := createTestChar(id.New(), 10, 100, 30, 20)
				ally := createTestChar(id.New(), 8, 80, 20, 15)
				repo := &errorInjectingDungeonRepo{
					mockDungeonRepo:       newMockDungeonRepo(),
					errFinalizeExpedition: errors.New("SQL unavailable"),
				}
				repo.savedChars[lead.ID] = lead
				repo.savedChars[ally.ID] = ally
				now := time.Now().UTC().Truncate(time.Millisecond)
				exp := dungeon.ActiveExpedition{
					ID:               id.New(),
					CharacterID:      lead.ID,
					PartyID:          id.New(),
					DungeonID:        dungeon.NativeMap0ID,
					Revision:         4,
					PosX:             2,
					PosY:             3,
					CurrentHP:        0,
					Round:            7,
					RoundLimit:       20,
					AccumulatedExp:   90,
					AccumulatedGold:  150,
					AccumulatedItems: []string{"item-101"},
					Status:           dungeon.StatusWipedOut,
					StartedAt:        now,
					UpdatedAt:        now,
					Members: []dungeon.ExpeditionMember{
						{CharacterID: lead.ID, Name: lead.Name, CurrentHP: 0, MaxHP: 100, AccumulatedExp: 90, AccumulatedGold: 150},
						{CharacterID: ally.ID, Name: ally.Name, CurrentHP: 0, MaxHP: 80, AccumulatedExp: 90, AccumulatedGold: 150},
					},
				}
				seedPreUpdateWipedOutValkey(t, ctx, store, exp)
				t.Cleanup(func() {
					if err := store.ActiveExpeditionStore.DeleteActiveExpedition(ctx, lead.ID, exp.ID); err != nil {
						t.Error(err)
					}
				})

				svc, err := dungeon.NewService(
					repo,
					&mockCharRepo{chars: map[string]corecharacter.Character{lead.ID: lead, ally.ID: ally}},
					corebattle.Engine{},
					dungeon.WithActiveExpeditionStore(store),
				)
				if err != nil {
					t.Fatal(err)
				}

				obsLead, err := svc.ObserveExpedition(ctx, lead.ID)
				if err != nil || obsLead == nil || obsLead.Status != dungeon.StatusWipedOut || obsLead.Map != nil {
					t.Fatalf("leader observation: %+v err=%v", obsLead, err)
				}
				if obsLead.Controls.CanMove || !obsLead.Controls.CanFight || !obsLead.Controls.CanEscape {
					t.Fatalf("leader controls: %+v", obsLead.Controls)
				}

				obsAlly, err := svc.ObserveExpedition(ctx, ally.ID)
				if err != nil || obsAlly == nil || obsAlly.Status != dungeon.StatusWipedOut || obsAlly.Map != nil {
					t.Fatalf("member observation: %+v err=%v", obsAlly, err)
				}
				if obsAlly.Controls.CanMove || obsAlly.Controls.CanFight || obsAlly.Controls.CanEscape {
					t.Fatalf("member controls must stay empty: %+v", obsAlly.Controls)
				}

				if _, err := svc.Move(ctx, lead.ID, dungeon.DirectionEast); !errors.Is(err, dungeon.ErrNoActiveExpedition) {
					t.Fatalf("move on wiped_out: got %v want ErrNoActiveExpedition", err)
				}
				if _, err := svc.Escape(ctx, ally.ID); !errors.Is(err, dungeon.ErrForbidden) {
					t.Fatalf("member escape: got %v want ErrForbidden", err)
				}
				if _, err := svc.Fight(ctx, ally.ID); !errors.Is(err, dungeon.ErrForbidden) {
					t.Fatalf("member fight: got %v want ErrForbidden", err)
				}
				if _, err := svc.StartExpedition(ctx, lead.ID, dungeon.NativeMap0ID); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
					t.Fatalf("start while frozen: got %v want ErrActiveExpeditionExists", err)
				}

				resume := func() (dungeon.ExpeditionStepResult, error) {
					if resumeVia == "fight" {
						return svc.Fight(ctx, lead.ID)
					}
					return svc.Escape(ctx, lead.ID)
				}

				// 1. Interrupted SQL finalization preserves the buffer for retry.
				if _, err := resume(); !errors.Is(err, repo.errFinalizeExpedition) {
					t.Fatalf("expected SQL failure, got %v", err)
				}
				for _, actor := range []string{lead.ID, ally.ID} {
					still, err := store.GetActiveExpedition(ctx, actor)
					if err != nil || still == nil || still.ID != exp.ID || still.Status != dungeon.StatusWipedOut {
						t.Fatalf("buffer lost after SQL failure for %s: %+v err=%v", actor, still, err)
					}
				}

				// 2. SQL commits while cleanup fails: buffer remains, second retry clears it idempotently.
				repo.errFinalizeExpedition = nil
				store.fail = true
				res, err := resume()
				if err != nil || !res.IsFinished || res.EventType != dungeon.EventWipeout || res.Expedition.Status != dungeon.StatusWipedOut || res.ExpEarned != 0 || res.GoldFound != 0 {
					t.Fatalf("resumed settlement result: %+v err=%v", res, err)
				}
				if lingering, err := store.GetActiveExpedition(ctx, lead.ID); err != nil || lingering == nil {
					t.Fatalf("expected buffer retained on cleanup failure: %+v err=%v", lingering, err)
				}

				store.fail = false
				resRetry, err := resume()
				if err != nil || !resRetry.IsFinished || resRetry.EventType != dungeon.EventWipeout || resRetry.Expedition.Status != dungeon.StatusWipedOut {
					t.Fatalf("cleanup retry result: %+v err=%v", resRetry, err)
				}

				for _, actor := range []string{lead.ID, ally.ID} {
					if got, err := store.GetActiveExpedition(ctx, actor); err != nil || got != nil {
						t.Fatalf("occupancy not released for %s: %+v err=%v", actor, got, err)
					}
				}
				h := repo.histories[lead.ID]
				if len(h) != 1 || h[0].ID != exp.ID || h[0].Outcome != dungeon.StatusWipedOut || h[0].ExpReward != 0 || h[0].GoldReward != 0 || h[0].ItemsRewardCount != 0 {
					t.Fatalf("wrong/duplicate history: %+v", h)
				}
				if len(repo.awardedItems[lead.ID]) != 0 || repo.savedChars[lead.ID].Money != lead.Money || repo.savedChars[ally.ID].Money != ally.Money {
					t.Fatalf("unbanked rewards must be forfeited: items=%v lead=%+v ally=%+v", repo.awardedItems[lead.ID], repo.savedChars[lead.ID], repo.savedChars[ally.ID])
				}
			})
		}
	}
}

func TestDungeonIntegration_LegacyWipedOutSettlement_RealSQLAndValkey(t *testing.T) {
	db := database.OpenTestDB(t)
	client := testutil.OpenValkey(t)

	store, err := dungeon.NewValkeyExpeditionRepository(client)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	setupParty := func(t *testing.T, ctx context.Context, leadMoney, allyMoney int) (corecharacter.Character, corecharacter.Character, coreitem.Instance, coreitem.Instance) {
		t.Helper()
		lead, err := database.CreateTestCharacterWithFunds(ctx, db, "Legacy wipeout lead", leadMoney)
		if err != nil {
			t.Fatal(err)
		}
		ally, err := database.CreateTestCharacterWithFunds(ctx, db, "Legacy wipeout ally", allyMoney)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			cleanupCtx := context.Background()
			cleanupDungeonValkey(t, client, lead.ID)
			_ = client.Do(cleanupCtx, client.B().Del().Key("party2:dungeon:{char:"+ally.ID+"}:member").Build()).Error()
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM dungeon_expedition_history WHERE character_id IN (?, ?)", lead.ID, ally.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM dungeon_active_expeditions WHERE character_id IN (?, ?)", lead.ID, ally.ID)
			_ = chars.Delete(cleanupCtx, lead.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", lead.PlayerID)
			_ = chars.Delete(cleanupCtx, ally.ID)
			_, _ = db.ExecContext(cleanupCtx, "DELETE FROM players WHERE id = ?", ally.PlayerID)
		})

		for _, c := range []*corecharacter.Character{&lead, &ally} {
			c.Level = 10
			c.Stats.MaxHP, c.Stats.HP = 100, 100
			c.Stats.MaxMP, c.Stats.MP = 40, 40
			if err := chars.Update(ctx, *c); err != nil {
				t.Fatal(err)
			}
		}

		leadItem, err := coreitem.NewInstance("potion", 2)
		if err != nil {
			t.Fatal(err)
		}
		allyItem, err := coreitem.NewInstance("ether", 3)
		if err != nil {
			t.Fatal(err)
		}
		for _, pair := range []struct {
			charID string
			inst   coreitem.Instance
		}{{lead.ID, leadItem}, {ally.ID, allyItem}} {
			inv, err := coreinventory.New(pair.charID)
			if err != nil {
				t.Fatal(err)
			}
			if err := inv.Add(pair.inst); err != nil {
				t.Fatal(err)
			}
			if err := invs.Save(ctx, inv); err != nil {
				t.Fatal(err)
			}
		}
		return lead, ally, leadItem, allyItem
	}

	assertItemQty := func(t *testing.T, ctx context.Context, charID, instID string, wantQty int) {
		t.Helper()
		inv, err := invs.FindByCharacterID(ctx, charID)
		if err != nil {
			t.Fatal(err)
		}
		gotQty := 0
		for _, it := range inv.Items {
			if it.ID == instID {
				gotQty = it.Quantity
			}
		}
		if gotQty != wantQty {
			t.Fatalf("char %s item %s qty=%d, want %d (items=%+v)", charID, instID, gotQty, wantQty, inv.Items)
		}
	}

	t.Run("absent_history_claim_finalizes_once_and_releases_occupancy", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		lead, ally, leadItem, allyItem := setupParty(t, ctx, 500, 300)

		now := time.Now().UTC().Truncate(time.Millisecond)
		oldRun := dungeon.ActiveExpedition{
			ID:               id.New(),
			CharacterID:      lead.ID,
			PartyID:          id.New(),
			DungeonID:        dungeon.NativeMap0ID,
			Revision:         5,
			PosX:             1,
			PosY:             2,
			CurrentHP:        0,
			Round:            8,
			RoundLimit:       20,
			AccumulatedExp:   120,
			AccumulatedGold:  250,
			AccumulatedItems: []string{"potion"},
			Status:           dungeon.StatusWipedOut,
			StartedAt:        now,
			UpdatedAt:        now,
			Members: []dungeon.ExpeditionMember{
				{
					CharacterID:     lead.ID,
					Name:            lead.Name,
					CurrentHP:       0,
					MaxHP:           100,
					CurrentMP:       12,
					MaxMP:           40,
					AccumulatedExp:  120,
					AccumulatedGold: 250,
					ConsumedItems:   []corebattle.ConsumedItem{{ID: "potion", InstanceID: leadItem.ID, Quantity: 1}},
				},
				{
					CharacterID:     ally.ID,
					Name:            ally.Name,
					CurrentHP:       0,
					MaxHP:           100,
					AccumulatedExp:  120,
					AccumulatedGold: 250,
					ConsumedItems:   []corebattle.ConsumedItem{{ID: "ether", InstanceID: allyItem.ID, Quantity: 1}},
				},
			},
		}
		// Seed a raw pre-update JSON payload for the ally without current_mp to verify legacy MP preservation too.
		if err := store.SaveActiveExpedition(ctx, oldRun); err != nil {
			t.Fatal(err)
		}
		// Explicitly rewrite members JSON so lead has current_mp=12 and ally omits current_mp (pre-MP legacy).
		membersJSON := fmt.Sprintf(
			`[{"character_id":%q,"name":%q,"current_hp":0,"max_hp":100,"current_mp":12,"max_mp":40,"accumulated_exp":120,"accumulated_gold":250,"consumed_items":[{"id":"potion","instance_id":%q,"quantity":1}]},{"character_id":%q,"name":%q,"current_hp":0,"max_hp":100,"accumulated_exp":120,"accumulated_gold":250,"consumed_items":[{"id":"ether","instance_id":%q,"quantity":1}]}]`,
			lead.ID, lead.Name, leadItem.ID, ally.ID, ally.Name, allyItem.ID,
		)
		sKey := "party2:dungeon:{char:" + lead.ID + "}:state"
		if err := client.Do(ctx, client.B().Hset().Key(sKey).FieldValue().FieldValue("members", membersJSON).Build()).Error(); err != nil {
			t.Fatal(err)
		}

		injectRepo := &errorInjectingDungeonRepoCompat{Repository: repo, errFinalize: errors.New("sql timeout")}
		flakyStore := &failOnceDeleteStore{inner: store}
		svc, err := dungeon.NewService(injectRepo, chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(flakyStore))
		if err != nil {
			t.Fatal(err)
		}

		if _, err := svc.StartExpedition(ctx, lead.ID, dungeon.NativeMap0ID); !errors.Is(err, dungeon.ErrActiveExpeditionExists) {
			t.Fatalf("start must reject while wiped_out buffer exists: %v", err)
		}
		obs, err := svc.ObserveExpedition(ctx, lead.ID)
		if err != nil || obs == nil || obs.Status != dungeon.StatusWipedOut || !obs.Controls.CanEscape || !obs.Controls.CanFight || obs.Controls.CanMove {
			t.Fatalf("unexpected observation: %+v err=%v", obs, err)
		}

		// Interrupted SQL finalization leaves Valkey buffer and member ref intact.
		if _, err := svc.Escape(ctx, lead.ID); !errors.Is(err, injectRepo.errFinalize) {
			t.Fatalf("expected injected SQL failure, got %v", err)
		}
		if got, err := store.GetActiveExpedition(ctx, ally.ID); err != nil || got == nil || got.ID != oldRun.ID {
			t.Fatalf("member occupancy lost on SQL failure: %+v err=%v", got, err)
		}

		// SQL commits while Valkey cleanup fails once; retry via Fight clears Valkey without re-applying SQL effects.
		injectRepo.errFinalize = nil
		flakyStore.failCalls.Store(1)
		res1, err := svc.Escape(ctx, lead.ID)
		if err != nil || !res1.IsFinished || res1.EventType != dungeon.EventWipeout {
			t.Fatalf("first escape after SQL recovery: %+v err=%v", res1, err)
		}
		if got, err := store.GetActiveExpedition(ctx, lead.ID); err != nil || got == nil {
			t.Fatalf("expected Valkey buffer to remain after cleanup error: %+v err=%v", got, err)
		}

		res2, err := svc.Fight(ctx, lead.ID)
		if err != nil || !res2.IsFinished || res2.EventType != dungeon.EventWipeout {
			t.Fatalf("retry via Fight: %+v err=%v", res2, err)
		}

		for _, actor := range []string{lead.ID, ally.ID} {
			if got, err := store.GetActiveExpedition(ctx, actor); err != nil || got != nil {
				t.Fatalf("occupancy remained for %s: %+v err=%v", actor, got, err)
			}
		}
		if n, err := client.Do(ctx, client.B().Exists().Key("party2:dungeon:{char:"+ally.ID+"}:member").Build()).AsInt64(); err != nil || n != 0 {
			t.Fatalf("member ref remained: %d err=%v", n, err)
		}

		histories, err := repo.GetHistory(ctx, lead.ID, 10)
		if err != nil || len(histories) != 1 || histories[0].ID != oldRun.ID || histories[0].Outcome != dungeon.StatusWipedOut ||
			histories[0].ExpReward != 0 || histories[0].GoldReward != 0 || histories[0].ItemsRewardCount != 0 {
			t.Fatalf("unexpected history: %+v err=%v", histories, err)
		}

		reloadedLead, err := chars.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		reloadedAlly, err := chars.FindByID(ctx, ally.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.HP != 0 || reloadedLead.Stats.MP != 12 || reloadedLead.Money != 500 || reloadedLead.Experience != 0 {
			t.Fatalf("unexpected leader after wipeout: %+v", reloadedLead)
		}
		if reloadedAlly.Stats.HP != 0 || reloadedAlly.Stats.MP != 40 || reloadedAlly.Money != 300 || reloadedAlly.Experience != 0 {
			t.Fatalf("unexpected ally after wipeout: %+v", reloadedAlly)
		}
		assertItemQty(t, ctx, lead.ID, leadItem.ID, 1)
		assertItemQty(t, ctx, ally.ID, allyItem.ID, 2)

		// Occupancy is released so the leader can start a fresh expedition.
		fresh, err := svc.StartExpedition(ctx, lead.ID, dungeon.NativeMap0ID)
		if err != nil || fresh == nil || fresh.Status != dungeon.StatusExploring || fresh.ID == oldRun.ID {
			t.Fatalf("expected new expedition after cleanup, got %+v err=%v", fresh, err)
		}
	})

	t.Run("committed_history_claim_clears_only_old_run_and_preserves_replacement", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		lead, ally, leadItem, allyItem := setupParty(t, ctx, 600, 400)

		now := time.Now().UTC().Truncate(time.Millisecond)
		oldRun := dungeon.ActiveExpedition{
			ID:               id.New(),
			CharacterID:      lead.ID,
			PartyID:          id.New(),
			DungeonID:        dungeon.NativeMap0ID,
			Revision:         3,
			CurrentHP:        0,
			AccumulatedExp:   55,
			AccumulatedGold:  88,
			AccumulatedItems: []string{"potion"},
			Status:           dungeon.StatusWipedOut,
			StartedAt:        now,
			UpdatedAt:        now,
			Members: []dungeon.ExpeditionMember{
				{
					CharacterID:   lead.ID,
					Name:          lead.Name,
					CurrentHP:     0,
					MaxHP:         100,
					CurrentMP:     10,
					MaxMP:         40,
					ConsumedItems: []corebattle.ConsumedItem{{ID: "potion", InstanceID: leadItem.ID, Quantity: 1}},
				},
				{
					CharacterID:   ally.ID,
					Name:          ally.Name,
					CurrentHP:     0,
					MaxHP:         100,
					CurrentMP:     15,
					MaxMP:         40,
					ConsumedItems: []corebattle.ConsumedItem{{ID: "ether", InstanceID: allyItem.ID, Quantity: 1}},
				},
			},
		}
		// Pre-commit the SQL claim as if SQL succeeded before Valkey cleanup crashed.
		preHistory := dungeon.DungeonExpeditionHistory{
			ID:          oldRun.ID,
			CharacterID: lead.ID,
			DungeonID:   oldRun.DungeonID,
			Outcome:     dungeon.StatusWipedOut,
			CreatedAt:   now,
		}
		if err := repo.FinalizeExpedition(ctx, preHistory, &lead, nil, oldRun.Members...); err != nil {
			t.Fatal(err)
		}
		// Simulate subsequent character healing/wallet change after SQL claim already committed.
		if _, err := db.ExecContext(ctx, "UPDATE characters SET hp = 75, money = 777 WHERE id = ?", lead.ID); err != nil {
			t.Fatal(err)
		}

		// Subcase A: retry against committed claim clears the lingering old Valkey keys without touching SQL again.
		if err := store.SaveActiveExpedition(ctx, oldRun); err != nil {
			t.Fatal(err)
		}
		svc, err := dungeon.NewService(repo, chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		res, err := svc.Escape(ctx, lead.ID)
		if err != nil || !res.IsFinished || res.EventType != dungeon.EventWipeout {
			t.Fatalf("retry against committed history: %+v err=%v", res, err)
		}
		for _, actor := range []string{lead.ID, ally.ID} {
			if got, err := store.GetActiveExpedition(ctx, actor); err != nil || got != nil {
				t.Fatalf("old buffer not cleared for %s: %+v err=%v", actor, got, err)
			}
		}
		reloadedLead, err := chars.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloadedLead.Stats.HP != 75 || reloadedLead.Money != 777 {
			t.Fatalf("retry overwrote post-claim character state: %+v", reloadedLead)
		}
		assertItemQty(t, ctx, lead.ID, leadItem.ID, 1)
		assertItemQty(t, ctx, ally.ID, allyItem.ID, 2)

		// Subcase B: a delayed cleanup of the old wiped_out run preserves a later run with a different ID.
		if err := store.SaveActiveExpedition(ctx, oldRun); err != nil {
			t.Fatal(err)
		}
		nextRun := oldRun
		nextRun.ID = id.New()
		nextRun.Status = dungeon.StatusExploring
		nextRun.CurrentHP = 75
		nextRun.AccumulatedGold = 42
		nextRun.ConsumedEvents = []string{}
		nextRun.Members = append([]dungeon.ExpeditionMember(nil), oldRun.Members...)
		nextRun.Members[0].CurrentHP = 75
		nextRun.Members[1].CurrentHP = 50
		nextRun.Members = dungeon.DecodeMembers(dungeon.EncodeMembers(nextRun.Members))
		paused := &pausedFinalizeRepository{Repository: repo, entered: make(chan struct{}), resume: make(chan struct{})}
		pausedSvc, err := dungeon.NewService(paused, chars, corebattle.Engine{}, dungeon.WithActiveExpeditionStore(store))
		if err != nil {
			t.Fatal(err)
		}
		errs := testutil.RunRace(func() error {
			_, err := pausedSvc.Escape(context.WithValue(ctx, staleEscapeKey{}, true), lead.ID)
			return err
		}, func() error {
			defer close(paused.resume)
			select {
			case <-paused.entered:
			case <-ctx.Done():
				return ctx.Err()
			}
			// Replace the Valkey keys and SQL fallback with the newer run before old cleanup runs.
			if err := store.SaveActiveExpedition(ctx, nextRun); err != nil {
				return err
			}
			return repo.SaveActiveExpedition(ctx, nextRun)
		})
		for _, err := range errs {
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, actor := range []string{lead.ID, ally.ID} {
			got, err := store.GetActiveExpedition(ctx, actor)
			if err != nil || !reflect.DeepEqual(got, &nextRun) {
				t.Fatalf("delayed old cleanup altered newer run for %s: got=%+v want=%+v err=%v", actor, got, nextRun, err)
			}
		}
		histories, err := repo.GetHistory(ctx, lead.ID, 10)
		if err != nil || len(histories) != 1 || histories[0].ID != oldRun.ID {
			t.Fatalf("history changed unexpectedly: %+v err=%v", histories, err)
		}
	})

	t.Run("contrast_new_all_fallen_run_stays_exploring_and_escapes_with_stock", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		lead, _, _, _ := setupParty(t, ctx, 500, 300)

		lossEngine := &customPartyBattleEngine{resolvePartyFn: func(corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
			return corebattle.PartyBattleResult{
				Outcome:     corebattle.OutcomeDefeat,
				WinnerSide:  "enemies",
				RemainingHP: map[string]int{lead.ID: 0, "slime": 30},
			}, nil
		}}
		custom := dungeon.Dungeon{
			ID:         "dg-contrast-all-fallen",
			Name:       "Contrast Fallen Dungeon",
			RoundLimit: 20,
			Variants: []dungeon.Floor{{
				Width:    2,
				Height:   1,
				Grid:     []string{"S0"},
				Monsters: []dungeon.DungeonMonster{{ID: "slime", Name: "Slime", HP: 30, Attack: 10, ExpReward: 15, GoldReward: 25}},
			}},
		}
		svc, err := dungeon.NewService(repo, chars, lossEngine, dungeon.WithActiveExpeditionStore(store), dungeon.WithInventoryProvider(invs), dungeon.WithCustomDungeons([]dungeon.Dungeon{custom}))
		if err != nil {
			t.Fatal(err)
		}

		exp, err := svc.StartExpedition(ctx, lead.ID, custom.ID)
		if err != nil {
			t.Fatal(err)
		}
		exp.AccumulatedExp = 40
		exp.AccumulatedGold = 125
		exp.AccumulatedItems = []string{"potion"}
		exp.Encounter = &dungeon.ActiveEncounter{
			Monster:   custom.Variants[0].Monsters[0],
			Enemies:   []corebattle.Participant{corebattle.MustNewParticipant("slime", 30, 10, 0)},
			EventType: dungeon.EventBattle,
			X:         1,
		}
		if err := store.SaveActiveExpedition(ctx, *exp); err != nil {
			t.Fatal(err)
		}

		fightRes, err := svc.Fight(ctx, lead.ID)
		if err != nil {
			t.Fatalf("expected defeat fight to keep expedition exploring: %v", err)
		}
		if fightRes.IsFinished || fightRes.Expedition.Status != dungeon.StatusExploring {
			t.Fatalf("new all-fallen run must stay exploring, got %+v", fightRes)
		}
		obs, err := svc.ObserveExpedition(ctx, lead.ID)
		if err != nil || obs == nil || obs.Status != dungeon.StatusExploring || obs.Controls.CanMove || obs.Controls.CanFight || !obs.Controls.CanEscape {
			t.Fatalf("unexpected observation after all-fallen defeat: %+v err=%v", obs, err)
		}
		if _, err := svc.Fight(ctx, lead.ID); !errors.Is(err, dungeon.ErrActorFallen) {
			t.Fatalf("expected ErrActorFallen when retrying fight while fallen: %v", err)
		}

		escRes, err := svc.Escape(ctx, lead.ID)
		if err != nil || !escRes.IsFinished || escRes.EventType != dungeon.EventEscape || escRes.Expedition.Status != dungeon.StatusEscaped {
			t.Fatalf("unexpected escape result: %+v err=%v", escRes, err)
		}
		if escRes.ExpEarned != 40 || escRes.GoldFound != 125 {
			t.Fatalf("expected stock earned once, got %+v", escRes)
		}
		histories, err := repo.GetHistory(ctx, lead.ID, 10)
		if err != nil || len(histories) != 1 || histories[0].Outcome != dungeon.StatusEscaped || histories[0].ExpReward != 40 || histories[0].GoldReward != 125 || histories[0].ItemsRewardCount != 1 {
			t.Fatalf("unexpected history: %+v err=%v", histories, err)
		}
		reloaded, err := chars.FindByID(ctx, lead.ID)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Stats.HP != 0 || reloaded.Money != 625 || reloaded.Experience != 40 {
			t.Fatalf("unexpected character after escaped all-fallen run: %+v", reloaded)
		}
	})
}

type errorInjectingDungeonRepoCompat struct {
	dungeon.Repository
	errFinalize error
}

func (r *errorInjectingDungeonRepoCompat) FinalizeExpedition(
	ctx context.Context,
	history dungeon.DungeonExpeditionHistory,
	character *corecharacter.Character,
	rewardItems []coreitem.Instance,
	members ...dungeon.ExpeditionMember,
) error {
	if r.errFinalize != nil {
		return r.errFinalize
	}
	return r.Repository.FinalizeExpedition(ctx, history, character, rewardItems, members...)
}
