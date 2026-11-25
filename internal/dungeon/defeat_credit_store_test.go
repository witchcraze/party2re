package dungeon_test

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

// Both stores append credits and move the drained member's stock into them
// within the revision-checked step, and acknowledge settled credits by ID.
func TestCreditStore_StepDrainAndAck(t *testing.T) {
	for _, kind := range []string{"memory", "valkey"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			store := encounterStore(t, kind)
			lead, member := id.New(), id.New()
			run := sampleExpedition(id.New(), lead, "credits")
			run.Members = []dungeon.ExpeditionMember{
				{CharacterID: lead, CurrentHP: 10, AccumulatedExp: 5, AccumulatedGold: 6, AccumulatedCrystals: 1},
				{CharacterID: member, CurrentHP: 10},
			}
			if err := store.SaveActiveExpedition(ctx, run); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if err := store.DeleteActiveExpedition(context.Background(), lead, run.ID); err != nil {
					t.Error(err)
				}
			})
			first := dungeon.DefeatCredit{ID: "r0:" + lead, CharacterID: lead, Reward: corebattle.Reward{Experience: 9, Currency: 4, Crystals: 2},
				Defeats: []battle.ExplicitDefeat{{Monster: battle.DefeatedMonsterRecord{MonsterID: "slime", MonsterName: "スライム", Strong: 60}, StrongKill: true, Capture: true}}}
			out, err := store.Step(ctx, lead, dungeon.StepParams{
				ExpectedExpeditionID: run.ID, ExpectedRevision: &run.Revision, NewX: 1,
				Credits: []dungeon.DefeatCredit{first}, StockDrain: lead,
				MemberExpDeltas: map[string]int{member: 7}, MemberGoldDeltas: map[string]int{member: 8}, MemberCrystalDeltas: map[string]int{member: 1},
				ExpDelta: 3, GoldDelta: 2, Now: time.Now().UTC(),
			})
			if err != nil {
				t.Fatal(err)
			}
			check := func(step string, got dungeon.ActiveExpedition, credits []dungeon.DefeatCredit) {
				t.Helper()
				stock := func(m dungeon.ExpeditionMember) [3]int {
					return [3]int{m.AccumulatedExp, m.AccumulatedGold, m.AccumulatedCrystals}
				}
				if stock(got.Members[0]) != [3]int{} || stock(got.Members[1]) != [3]int{7, 8, 1} || got.AccumulatedExp != 3 || got.AccumulatedGold != 2 {
					t.Fatalf("%s: stock/record %+v", step, got)
				}
				if !reflect.DeepEqual(got.Credits, credits) {
					t.Fatalf("%s: credits %+v, want %+v", step, got.Credits, credits)
				}
			}
			check("step", out.Expedition, []dungeon.DefeatCredit{first})
			reloaded, err := store.GetActiveExpedition(ctx, lead)
			if err != nil {
				t.Fatal(err)
			}
			check("reload", *reloaded, []dungeon.DefeatCredit{first})

			second := dungeon.DefeatCredit{ID: "r1:" + member, CharacterID: member, Reward: corebattle.Reward{Currency: 1}}
			if _, err := store.Step(ctx, lead, dungeon.StepParams{ExpectedExpeditionID: run.ID, ExpectedRevision: &reloaded.Revision, NewX: 1, Credits: []dungeon.DefeatCredit{second}, Now: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
			for _, ack := range []struct {
				run  string
				ids  []string
				want []dungeon.DefeatCredit
			}{
				{"other-run", []string{first.ID, second.ID}, []dungeon.DefeatCredit{first, second}},
				{run.ID, []string{first.ID}, []dungeon.DefeatCredit{second}},
				{run.ID, []string{first.ID}, []dungeon.DefeatCredit{second}},
				{run.ID, []string{second.ID}, nil},
			} {
				if err := store.AckCredits(ctx, lead, ack.run, ack.ids); err != nil {
					t.Fatal(err)
				}
				got, err := store.GetActiveExpedition(ctx, lead)
				if err != nil {
					t.Fatal(err)
				}
				check("ack", *got, ack.want)
			}
		})
	}
}

// The killer's share reaches SQL during the run and survives it; another
// member's share waits in Valkey stock across a service reload and is granted
// once at Escape.
func TestDefeatCredits_SQLAndValkeyRun(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_DB_DSN and PARTY2_VALKEY_ADDR are required")
	}
	ctx := context.Background()
	db := database.OpenTestDB(t)
	store, err := dungeon.NewValkeyExpeditionRepository(testutil.OpenValkey(t))
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewDungeonRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	repo.SetPostBattleSettler(sqlPostBattle(t, db))
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	lead, err := database.CreateTestCharacterWithFunds(ctx, db, "Credit Lead "+id.New()[:8], 100)
	if err != nil {
		t.Fatal(err)
	}
	member, err := database.CreateTestCharacterWithFunds(ctx, db, "Credit Ally "+id.New()[:8], 100)
	if err != nil {
		t.Fatal(err)
	}
	database.RegisterCharacterCleanup(t, db, lead, member)

	monster := dungeon.DungeonMonster{ID: "slime", Name: "スライム", HP: 10, ExpReward: 3, GoldReward: 40}
	catalog := []dungeon.Dungeon{{ID: "credit-cave", RoundLimit: 21, Variants: []dungeon.Floor{{Width: 2, Height: 1, Grid: []string{"S0"}}}}}
	engine := &customPartyBattleEngine{resolvePartyFn: func(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
		return corebattle.PartyBattleResult{Outcome: corebattle.OutcomeWin, WinnerSide: "allies",
			RemainingHP:    map[string]int{lead.ID: lead.Stats.HP, member.ID: member.Stats.HP, "slime": 0},
			DefeatEarnings: []corebattle.DefeatEarnings{{ActorID: lead.ID, TargetID: "slime", RecipientIDs: []string{lead.ID, member.ID}}}}, nil
	}}
	newService := func() *dungeon.Service {
		svc, err := dungeon.NewService(repo, chars, engine, dungeon.WithActiveExpeditionStore(store), dungeon.WithCustomDungeons(catalog),
			dungeon.WithAdmission(approvedRoster{}), dungeon.WithRNG(&scriptedRNG{ints: []int{49}, floats: []float64{0.99}}))
		if err != nil {
			t.Fatal(err)
		}
		return svc
	}
	svc := newService()
	run, err := svc.StartPartyExpedition(ctx, lead.ID, []string{lead.ID, member.ID}, "credit-cave", "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := store.DeleteActiveExpedition(context.Background(), lead.ID, run.ID); err != nil {
			t.Error(err)
		}
	})
	run.Encounter = &dungeon.ActiveEncounter{Monsters: []dungeon.DungeonMonster{monster}, Enemies: []corebattle.Participant{corebattle.MustNewParticipant("slime", 10, 1, 0)}, EventType: dungeon.EventBattle, X: 1}
	if err := store.SaveActiveExpedition(ctx, *run); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Fight(ctx, lead.ID); err != nil {
		t.Fatal(err)
	}
	money := func(id string) int {
		t.Helper()
		c, err := chars.FindByID(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return c.Money
	}
	live, err := store.GetActiveExpedition(ctx, lead.ID)
	if err != nil {
		t.Fatal(err)
	}
	if money(lead.ID) != 140 || money(member.ID) != 100 || live.Members[1].AccumulatedGold != 40 || len(live.Credits) != 0 {
		t.Fatalf("killer credit/stock: lead=%d member=%d run=%+v", money(lead.ID), money(member.ID), live)
	}

	svc = newService() // the stock lives in Valkey, not in the service
	for i := 0; i < 2; i++ {
		_, err := svc.Escape(ctx, lead.ID)
		if i == 0 && err != nil || i == 1 && !errors.Is(err, dungeon.ErrNoActiveExpedition) {
			t.Fatalf("escape %d: %v", i, err)
		}
	}
	if money(lead.ID) != 140 || money(member.ID) != 140 {
		t.Fatalf("escape grants: lead=%d member=%d", money(lead.ID), money(member.ID))
	}
}
