package party_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

// Script the crawl outcome and bind owned SQL instances to native tool
// consumption intents. Automatic revival still runs through the batch engine.
type consumptionFloorEngine struct {
	corebattle.Engine
	t           *testing.T
	calls, last int
	defeat      bool
	items       map[string][]coreitem.Instance
}

func (e *consumptionFloorEngine) ResolvePartyBattle(req corebattle.PartyBattleRequest) (corebattle.PartyBattleResult, error) {
	e.t.Helper()
	e.calls++
	res := corebattle.PartyBattleResult{
		Outcome: corebattle.OutcomeWin, Turns: 1,
		TotalReward:   corebattle.Reward{Experience: 1, Currency: 2, Crystals: 1},
		RemainingHP:   make(map[string]int),
		RemainingMP:   make(map[string]int),
		ConsumedItems: make(map[string][]corebattle.ConsumedItem),
	}
	for _, ally := range req.Allies {
		res.RemainingHP[ally.ID], res.RemainingMP[ally.ID] = 50, 10
		if e.calls != 1 && e.calls != e.last {
			continue
		}
		index := 0
		if e.calls == e.last {
			index = 1
		}
		inst := e.items[ally.ID][index]
		if !slices.Contains(ally.ItemDefinitionIDs, inst.DefinitionID) {
			e.t.Fatalf("floor %d: %s is not carrying %s", e.calls, ally.ID, inst.DefinitionID)
		}
		toolActor := corebattle.LifetimeActor{Participant: ally, Tension: 1, Hit: 95}
		toolActor.MP, toolActor.MaxMP = 0, 100
		state := corebattle.LifetimeState{Actors: []corebattle.LifetimeActor{toolActor}}
		tool := corebattle.OrdinaryToolContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{
			TargetContext: corebattle.TargetContext{ActorID: ally.ID, PartyIDs: []string{ally.ID}},
		}}, Arguments: []string{ally.ID}}
		if err := state.ApplyOrdinaryTool(&tool, "tool-12", random.NewDeterministic(1)); err != nil {
			return res, err
		}
		if len(tool.Results) != 1 || !tool.Results[0].Consume || state.Actors[0].MP == 0 || state.Actors[0].HP != ally.HP {
			e.t.Fatalf("floor %d: native MP tool = %+v, actor=%+v", e.calls, tool.Results, state.Actors[0])
		}
		res.ConsumedItems[ally.ID] = []corebattle.ConsumedItem{{ID: inst.DefinitionID, InstanceID: inst.ID, Quantity: 1}}
		if e.calls == 1 {
			actor := ally
			actor.HP, actor.MaxHP, actor.Attack, actor.Defense, actor.Agility = 1, 100, 1000, 0, 1
			actor.ActionItems = nil
			enemy := corebattle.MustNewParticipant("enemy", 5, 2, 0)
			enemy.Agility = 1000
			revived, err := e.Engine.ResolvePartyBattle(corebattle.PartyBattleRequest{
				Allies: []corebattle.Participant{actor}, Enemies: []corebattle.Participant{enemy}, RNG: random.NewDeterministic(1),
			})
			if err != nil {
				return res, err
			}
			if got := revived.ConsumedItems[ally.ID]; len(got) != 1 || got[0].ID != battle.ItemDokuroAmulet || got[0].Quantity != 1 {
				e.t.Fatalf("actual revival consumption = %+v", revived.ConsumedItems)
			}
			res.ConsumedItems[ally.ID] = append(res.ConsumedItems[ally.ID], revived.ConsumedItems[ally.ID]...)
		}
	}
	if e.defeat && e.calls == e.last {
		res.Outcome = corebattle.OutcomeDefeat
		for owner := range res.RemainingHP {
			res.RemainingHP[owner] = 0
		}
	}
	return res, nil
}

type consumptionAdventureHistory struct {
	adventure.Repository
	afterSave func(context.Context) error
}

func (r *consumptionAdventureHistory) Save(ctx context.Context, a adventure.Adventure) error {
	if err := r.Repository.Save(ctx, a); err != nil {
		return err
	}
	return r.afterSave(ctx)
}

type consumptionPartyHistory struct {
	party.AdventureLogRepository
	afterSave func(context.Context) error
}

func (r *consumptionPartyHistory) SaveAdventureLog(ctx context.Context, log party.PartyAdventureLog) error {
	if err := r.AdventureLogRepository.SaveAdventureLog(ctx, log); err != nil {
		return err
	}
	return r.afterSave(ctx)
}

func TestAdventureConsumptionSQLIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	})
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	invs, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equips, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	advs, err := database.NewAdventureRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	logs, err := database.NewPartyRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	tx := database.NewTransactionProvider(db)
	runner, err := economy.NewService(chars, invs, economy.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	adapter := battle.NewService(battle.WithCharacterRepository(chars), battle.WithInventoryRepository(invs),
		battle.WithEquipmentRepository(equips), battle.WithTransactionProvider(tx), battle.WithTransactionRunner(runner))
	stages, err := adventure.InitialStageCatalog()
	if err != nil {
		t.Fatal(err)
	}
	monsters, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatal(err)
	}
	for _, members := range []int{1, 2} {
		for _, tc := range []struct {
			name, mode string
			defeat     bool
		}{
			{"victory", adventure.CrawlModeNormal, false},
			{"defeat-earned", adventure.CrawlModeNormal, true},
			{"defeat-no-reward", adventure.CrawlModeTry, true},
		} {
			t.Run(fmt.Sprintf("members=%d/%s", members, tc.name), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
				defer cancel()
				var originals []corecharacter.Character
				items := make(map[string][]coreitem.Instance)
				for i := 0; i <= members; i++ {
					c, err := database.CreateTestCharacterWithFunds(ctx, db, "Consume", 100)
					if err != nil {
						t.Fatal(err)
					}
					c.Level = 10
					c.Stats.HP, c.Stats.MaxHP, c.Stats.MP, c.Stats.MaxMP = 100, 100, 30, 30
					if err := chars.Update(ctx, c); err != nil {
						t.Fatal(err)
					}
					originals = append(originals, c)
					// MP tools in separate stacks expose definition-only or wrong-owner
					// debits. Their native callbacks produce consumption intents.
					items[c.ID] = []coreitem.Instance{
						{ID: id.New(), DefinitionID: "item-012", Quantity: 3},
						{ID: id.New(), DefinitionID: "item-012", Quantity: 4},
						{ID: id.New(), DefinitionID: "item-012", Quantity: 2},
						{ID: id.New(), DefinitionID: battle.ItemDokuroAmulet, Quantity: 1},
					}
					if _, err := database.CreateTestInventoryWithItems(ctx, db, c.ID, items[c.ID]); err != nil {
						t.Fatal(err)
					}
					if err := equips.Save(ctx, coreequipment.Equipment{CharacterID: c.ID, Slots: map[coreitem.Slot]string{
						coreitem.SlotAccessory: items[c.ID][3].ID,
					}}); err != nil {
						t.Fatal(err)
					}
				}
				engine := &consumptionFloorEngine{t: t, items: items, last: adventure.BossFloor, defeat: tc.defeat}
				if tc.defeat {
					engine.last = 3
				}
				partyID := id.New()
				// Read through SQL repositories, using the ambient transaction only at the fault boundary.
				assertAssets := func(readCtx context.Context, committed bool) {
					t.Helper()
					for i, before := range originals {
						applied := committed && i < members
						inv, err := invs.FindByCharacterID(readCtx, before.ID)
						if err != nil {
							t.Fatal(err)
						}
						want := slices.Clone(items[before.ID])
						if applied {
							want[0].Quantity--
							want[1].Quantity--
							want = want[:3]
						}
						slices.SortFunc(want, func(a, b coreitem.Instance) int { return strings.Compare(a.ID, b.ID) })
						if inv.CharacterID != before.ID || !reflect.DeepEqual(inv.Items, want) {
							t.Fatalf("owner %s inventory = %+v, want %+v", before.ID, inv.Items, want)
						}
						equip, err := equips.FindByCharacterID(readCtx, before.ID)
						if err != nil {
							t.Fatal(err)
						}
						wantSlots := map[coreitem.Slot]string{}
						if !applied {
							wantSlots[coreitem.SlotAccessory] = items[before.ID][3].ID
						}
						if !reflect.DeepEqual(equip.Slots, wantSlots) {
							t.Fatalf("owner %s equipment = %+v, want %+v", before.ID, equip.Slots, wantSlots)
						}
						after, err := chars.FindByID(readCtx, before.ID)
						if err != nil {
							t.Fatal(err)
						}
						if !applied {
							if !reflect.DeepEqual(after, before) {
								t.Fatalf("owner %s character changed without commit: before=%+v after=%+v", before.ID, before, after)
							}
							continue
						}
						wins := engine.last
						if tc.defeat {
							wins--
						}
						if tc.mode == adventure.CrawlModeTry {
							wins = 0
						}
						hp := 50
						if tc.defeat {
							hp = 1
						}
						if after.Money != before.Money+2*wins || after.Experience != before.Experience+wins || after.Crystal != before.Crystal+wins || after.Stats.HP != hp || after.Stats.MP != 10 || after.Tired == before.Tired {
							t.Fatalf("owner %s settlement: %+v (credited floors=%d)", before.ID, after, wins)
						}
					}
				}
				assertHistory := func(readCtx context.Context, count int) {
					t.Helper()
					var got int
					var err error
					if members == 1 {
						err = database.ExecutorFromContext(readCtx, db).QueryRowContext(readCtx, "SELECT COUNT(*) FROM adventures WHERE character_id = ?", originals[0].ID).Scan(&got)
					} else {
						err = database.ExecutorFromContext(readCtx, db).QueryRowContext(readCtx, "SELECT COUNT(*) FROM party_adventure_logs WHERE party_id = ?", partyID).Scan(&got)
					}
					if err != nil {
						t.Fatal(err)
					}
					if got != count {
						t.Fatalf("SQL history count = %d, want %d", got, count)
					}
				}
				injected := errors.New("history failure after settlement and SQL insert")
				fail, saves := true, 0
				afterSave := func(txCtx context.Context) error {
					saves++
					assertAssets(txCtx, true)
					assertHistory(txCtx, 1)
					if fail {
						return injected
					}
					return nil
				}
				var run func() (corebattle.Outcome, error)
				if members == 1 {
					history := &consumptionAdventureHistory{Repository: advs, afterSave: afterSave}
					svc, err := adventure.NewServiceWithCatalogs(history, chars, invs, stages, monsters, engine, nil, nil, adventure.RealClock{},
						adventure.WithParticipantBuilder(adapter), adventure.WithPostBattleSettler(adapter), adventure.WithTransactionProvider(tx))
					if err != nil {
						t.Fatal(err)
					}
					run = func() (corebattle.Outcome, error) {
						r, err := svc.ExecuteCrawl(ctx, adventure.DungeonCrawlRequest{CharacterIDs: []string{originals[0].ID}, StageID: adventure.StarterAdventure, Mode: tc.mode, Rng: func(int) int { return 0 }})
						return r.Outcome, err
					}
				} else {
					history := &consumptionPartyHistory{AdventureLogRepository: logs, afterSave: afterSave}
					// Lobby state is outside this SQL contract; use its existing in-memory adapter.
					lobby := party.NewValkeyRepository(nil, party.WithDurableLogRepository(history))
					if err := lobby.SaveParty(ctx, party.Party{ID: partyID, LeaderCharacterID: originals[0].ID, StageID: adventure.StarterAdventure, Mode: tc.mode, Status: party.StatusRecruiting}); err != nil {
						t.Fatal(err)
					}
					for i, c := range originals[:members] {
						if err := lobby.AddMember(ctx, party.Member{PartyID: partyID, CharacterID: c.ID, IsLeader: i == 0, ReadyState: true}); err != nil {
							t.Fatal(err)
						}
					}
					svc, err := party.NewService(lobby, chars, invs, stages, monsters, engine,
						party.WithParticipantBuilder(adapter), party.WithPostBattleSettler(adapter), party.WithTransactionProvider(tx), party.WithRNG(func(int) int { return 0 }))
					if err != nil {
						t.Fatal(err)
					}
					run = func() (corebattle.Outcome, error) {
						r, err := svc.StartPartyAdventure(ctx, partyID, originals[0].ID)
						return corebattle.Outcome(r.Outcome), err
					}
				}
				// Keep comparison snapshots in exactly the form returned by the SQL adapter.
				for i, c := range originals {
					originals[i], err = chars.FindByID(ctx, c.ID)
					if err != nil {
						t.Fatal(err)
					}
				}
				assertAssets(ctx, false)
				assertHistory(ctx, 0)
				if _, err := run(); !errors.Is(err, injected) {
					t.Fatalf("expected injected history error, got %v", err)
				}
				if saves != 1 || engine.calls != engine.last {
					t.Fatalf("failure before intended boundary: saves=%d floors=%d", saves, engine.calls)
				}
				assertAssets(ctx, false)
				assertHistory(ctx, 0)
				fail, engine.calls = false, 0
				outcome, err := run()
				if err != nil {
					t.Fatal(err)
				}
				wantOutcome := corebattle.OutcomeWin
				if tc.defeat {
					wantOutcome = corebattle.OutcomeDefeat
				}
				if outcome != wantOutcome || saves != 2 || engine.calls != engine.last {
					t.Fatalf("retry outcome=%s saves=%d floors=%d", outcome, saves, engine.calls)
				}
				assertAssets(ctx, true)
				assertHistory(ctx, 1)
			})
		}
	}
}
