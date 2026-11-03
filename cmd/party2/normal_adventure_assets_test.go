package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/auction"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/store"
	"github.com/witchcraze/party2re/internal/testutil"
	vk "github.com/witchcraze/party2re/internal/valkey"
	"github.com/witchcraze/party2re/internal/wishingwell"
)

func TestNormalRunAssetMutationProduction(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" || os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("requires MariaDB and Valkey")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	client, err := vk.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
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
	misc, err := newMiscServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the later production Store reconfiguration too.
	econ.initStore(core, soc.guild, soc.timer, client)
	combat.normal.SetExitEffects(func(context.Context, string) error { return nil })
	chars := make([]corecharacter.Character, 2)
	for i := range chars {
		c, err := database.CreateTestCharacterWithFunds(ctx, db, "Assets_"+id.New()[:8], 10000)
		if err != nil {
			t.Fatal(err)
		}
		c.Level, c.JobLevel, c.SP = 15, 7, 10
		c.Stats.HP, c.Stats.MaxHP = 200, 200
		if err := core.charRepo.Update(ctx, c); err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestInventoryWithItems(ctx, db, c.ID, []coreitem.Instance{{ID: id.New(), DefinitionID: "item-001", Quantity: 2}}); err != nil {
			t.Fatal(err)
		}
		if _, err := database.CreateTestDepot(ctx, db, c.ID, 0, []coreitem.Instance{{ID: id.New(), DefinitionID: "item-002", Quantity: 1}}); err != nil {
			t.Fatal(err)
		}
		chars[i] = c
	}
	defer func() {
		for _, c := range chars {
			if err := core.charRepo.Delete(ctx, c.ID); err != nil {
				t.Error(err)
			}
			if err := core.playerRepo.Delete(ctx, c.PlayerID); err != nil {
				t.Error(err)
			}
		}
	}()
	sort.Slice(chars, func(i, j int) bool { return chars[i].ID < chars[j].ID })
	for i := range chars {
		t.Run(fmt.Sprintf("actor-lock-position-%d", i), func(t *testing.T) {
			actor, peer := chars[i], chars[1-i]
			held, err := core.invRepo.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(held.Items) == 0 {
				if _, _, err := core.economy.GrantItem(ctx, actor.ID, "item-001", 2); err != nil {
					t.Fatal(err)
				}
			}
			dep, err := econ.depotRepo.FindByCharacterID(ctx, peer.ID)
			if err != nil || len(dep.Items) == 0 {
				t.Fatal(dep, err)
			}
			listing, err := econ.fleamarket.CreateListing(ctx, peer.ID, dep.Items[0].ID, 10, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			// Persist a pre-existing offer without depending on unrelated town admission.
			st := store.Store{ID: id.New(), CharacterID: peer.ID, TownID: "town-01", StoreName: "Store_" + id.New()[:8], HouseStyle: "wood", ExpiresAt: time.Now().Add(time.Hour), CreatedAt: time.Now(), UpdatedAt: time.Now()}
			if err := econ.storeRepo.SaveStore(ctx, st); err != nil {
				t.Fatal(err)
			}
			sale := store.Sale{ID: id.New(), StoreID: st.ID, CharacterID: peer.ID, SlotNumber: 1, ItemDefinitionID: "item-001", ItemName: "薬草", Quantity: 1, SaleType: store.SaleTypeGold, Price: 10, CreatedAt: time.Now()}
			if err := econ.storeRepo.SaveSale(ctx, sale); err != nil {
				t.Fatal(err)
			}
			barter := sale
			barter.ID, barter.SlotNumber, barter.SaleType = id.New(), 2, store.SaleTypeBarter
			if err := econ.storeRepo.SaveSale(ctx, barter); err != nil {
				t.Fatal(err)
			}
			actorDepot, err := econ.depotRepo.FindByCharacterID(ctx, actor.ID)
			if err != nil || len(actorDepot.Items) == 0 {
				t.Fatal(actorDepot, err)
			}
			ownListing, err := econ.fleamarket.CreateListing(ctx, actor.ID, actorDepot.Items[0].ID, 10, time.Now())
			if err != nil {
				t.Fatal(err)
			}
			entry, err := combat.normal.EnterSolo(ctx, actor.PlayerID, actor.ID, id.New(), adventure.StarterAdventure, "normal", 3)
			if err != nil {
				t.Fatal(err)
			}
			inv, err := core.invRepo.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			before, err := core.charRepo.FindByID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			checks := []struct {
				name string
				run  func() error
			}{
				{"exchange", func() error {
					_, err := core.economy.Exchange(ctx, economy.ExchangeRequest{CharacterID: actor.ID, DeductGold: 1, GrantDefinitionID: "item-001", GrantQuantity: 1})
					return err
				}},
				{"consume", func() error {
					_, err := core.economy.ConsumeItemInstance(ctx, actor.ID, inv.Items[0].ID, 1)
					return err
				}},
				{"consume-definition", func() error { _, err := core.economy.ConsumeItemDefinition(ctx, actor.ID, "item-001", 1); return err }},
				{"shop", func() error { _, err := econ.shop.Purchase(ctx, actor.ID, "item-001", 1); return err }},
				{"secretshop", func() error {
					_, err := misc.secretshop.PurchaseItem(ctx, actor.ID, "secret_item_herbal_root", 1)
					return err
				}},
				{"secretshop-depot", func() error {
					_, err := misc.secretshop.PurchaseItem(ctx, actor.ID, "secret_item_herbal_root", 2)
					return err
				}},
				{"blackmarket-offer", func() error { _, err := misc.blackmarket.SacrificeItem(ctx, actor.ID, inv.Items[0].ID); return err }},
				{"blackmarket-prize", func() error { _, err := misc.blackmarket.TradePrize(ctx, actor.ID, "bm_prize_087"); return err }},
				{"bank-deposit", func() error { _, err := econ.bank.Deposit(ctx, actor.ID, 1); return err }},
				{"bank-withdraw", func() error { _, err := econ.bank.Withdraw(ctx, actor.ID, 1); return err }},
				{"depot", func() error { _, err := econ.depot.DepositItem(ctx, actor.ID, inv.Items[0].ID); return err }},
				{"depot-send", func() error { _, err := econ.depot.SendMoney(ctx, actor.ID, peer.ID, 1); return err }},
				{"auction-send", func() error {
					_, err := econ.auction.Send(ctx, auction.SendRequest{SenderCharacterID: actor.ID, TargetCharacterID: peer.ID, Gold: 1})
					return err
				}},
				{"economy-send", func() error { _, _, err := core.economy.TransferGold(ctx, actor.ID, peer.ID, 1); return err }},
				{"fleamarket-buy", func() error {
					_, err := econ.fleamarket.PurchaseListing(ctx, actor.ID, listing.ID, time.Now())
					return err
				}},
				{"fleamarket-list", func() error {
					_, err := econ.fleamarket.CreateListing(ctx, actor.ID, "missing", 10, time.Now())
					return err
				}},
				{"store-buy", func() error { return econ.store.BuyItem(ctx, actor.ID, sale.ID) }},
				{"store-trade", func() error { return econ.store.TradeItem(ctx, actor.ID, barter.ID, "missing") }},
				{"store-list", func() error { _, err := econ.store.ListGoldItem(ctx, actor.ID, "missing", 10); return err }},
				{"gem-buy", func() error { _, err := econ.gemStore.BuyGem(ctx, actor.ID, "gem_atk_1"); return err }},
				{"gem-send", func() error { _, err := econ.gemStore.SendGem(ctx, actor.ID, peer.ID, "gem_atk_1"); return err }},
				{"monster-release", func() error { return misc.monster.ReleaseMonster(ctx, actor.ID, "missing") }},
				{"monster-rename", func() error { _, err := misc.monster.Rename(ctx, actor.ID, "missing", "Pet"); return err }},
				{"monster-send", func() error { return misc.monster.SendMonster(ctx, actor.ID, peer.ID, "missing") }},
				{"blacksmith", func() error { _, err := econ.blacksmith.ApplySeal(ctx, actor.ID, 1); return err }},
				{"alchemy", func() error { _, err := econ.alchemy.Synthesize(ctx, actor.ID, "recipe-001"); return err }},
				{"lottery", func() error { _, _, _, err := misc.lottery.PlayRaffle(ctx, actor.ID); return err }},
				{"casino-slot", func() error { _, _, err := misc.casino.SpinSlot(ctx, actor.ID, 1); return err }},
				{"casino-prize", func() error { _, err := misc.casino.ExchangePrize(ctx, actor.ID, 100, 1); return err }},
				{"takarakuji", func() error {
					// Even round creation must roll back when buyer admission fails.
					return core.txProvider.RunInTx(ctx, func(ctx context.Context) error {
						_, err := misc.lottery.BuyTakarakujiTicket(ctx, actor.ID)
						return err
					})
				}},
				{"guild-create", func() error { _, _, _, err := soc.guild.Create(ctx, actor.ID, "Guild_"+id.New()[:8]); return err }},
				{"guild-mark", func() error { _, err := soc.guild.ChangeMark(ctx, "missing", actor.ID, "star"); return err }},
				{"guild-wallpaper", func() error { _, err := soc.guild.ChangeWallpaper(ctx, "missing", actor.ID, "farm.gif"); return err }},
				{"customskill", func() error {
					_, err := combat.customSkill.SetCustomSkill(ctx, actor.ID, "Test", "", [3]string{"gem_atk_1"})
					return err
				}},
				{"medal-exchange", func() error { _, _, err := misc.medal.Claim(ctx, actor.ID, "armor-32"); return err }},
				{"plantation", func() error { _, err := econ.plantation.Sow(ctx, actor.ID, "red"); return err }},
				{"depot-send-item", func() error { _, err := econ.depot.SendItem(ctx, actor.ID, peer.ID, inv.Items[0].ID); return err }},
				{"wishingwell", func() error {
					_, err := misc.wishingwell.Exchange(ctx, wishingwell.ExchangeRequest{CharacterID: actor.ID, Stat: "hp", SP: 1})
					return err
				}},
			}
			for _, check := range checks {
				t.Run(check.name, func(t *testing.T) {
					if err := check.run(); !errors.Is(err, adventure.ErrRunBusy) {
						t.Fatalf("expected ownership rejection, got %v", err)
					}
				})
			}
			after, err := core.charRepo.FindByID(ctx, actor.ID)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("rejected operations changed character", after, err)
			}
			afterInv, err := core.invRepo.FindByCharacterID(ctx, actor.ID)
			if err != nil || !reflect.DeepEqual(inv, afterInv) {
				t.Fatal("rejected operations changed inventory", afterInv, err)
			}
			// Public reward delivery and incoming transfers retain ordinary repositories.
			if _, err := core.economy.AddGold(ctx, actor.ID, 3); err != nil {
				t.Fatal(err)
			}
			if _, err := econ.depot.SendMoney(ctx, peer.ID, actor.ID, 5); err != nil {
				t.Fatal(err)
			}
			if _, err := econ.auction.Send(ctx, auction.SendRequest{SenderCharacterID: peer.ID, TargetCharacterID: actor.ID, Gold: 7}); err != nil {
				t.Fatal(err)
			}
			if _, err := econ.fleamarket.PurchaseListing(ctx, peer.ID, ownListing.ID, time.Now()); err != nil {
				t.Fatal("passive seller proceeds rejected", err)
			}
			peerInv, err := core.invRepo.FindByCharacterID(ctx, peer.ID)
			if err != nil || len(peerInv.Items) != 1 {
				t.Fatal(peerInv, err)
			}
			incoming := peerInv.Items[0]
			if _, err := econ.depot.SendItem(ctx, peer.ID, actor.ID, incoming.ID); err != nil {
				t.Fatal("incoming item rejected", err)
			}
			if err := (monsterTamerAdapter{svc: misc.monster}).TameMonster(ctx, actor.ID, "monster-001", "Gift"); err != nil {
				t.Fatal("passive capture reward rejected", err)
			}
			// Native exit still works through the same guarded Economy port.
			if _, err := combat.normal.Command(ctx, adventure.RunCommand{PlayerID: actor.PlayerID, ActorID: actor.ID, RunID: entry.Command.RunID, ID: "exit", Version: 1, Action: "exit"}); err != nil {
				t.Fatal(err)
			}
			after, err = core.charRepo.FindByID(ctx, actor.ID)
			if err != nil || after.Money != before.Money+25 {
				t.Fatal("exit lost incoming credits", after, err)
			}
			creditedDepot, err := econ.depotRepo.FindByCharacterID(ctx, actor.ID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, item := range creditedDepot.Items {
				if item.ID == incoming.ID && item.Quantity == incoming.Quantity {
					found = true
				}
			}
			if !found {
				t.Fatal("exit lost incoming item", creditedDepot)
			}
			captures, err := misc.monsterRepo.ListByCharacterID(ctx, actor.ID)
			if err != nil || len(captures) != 1 || captures[0].CustomName != "Gift" {
				t.Fatal("exit lost passive capture reward", captures, err)
			}
			if _, err := econ.bank.Deposit(ctx, actor.ID, 1); err != nil {
				t.Fatal("ownership was not released", err)
			}
			if _, err := misc.secretshop.PurchaseItem(ctx, actor.ID, "secret_item_herbal_root", 1); err != nil {
				t.Fatal("purchase remained blocked after exit", err)
			}
			// Restore the peer's listed item for the next lock-order case.
			if _, err := econ.fleamarket.CancelListing(ctx, peer.ID, listing.ID); err != nil {
				t.Fatal(err)
			}
		})
	}
	// A live recipient can receive concurrent credits but cannot send assets.
	actor, peer := chars[0], chars[1]
	inv, err := core.invRepo.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Items) > 0 {
		if _, err := econ.depot.DepositItem(ctx, actor.ID, inv.Items[0].ID); err != nil {
			t.Fatal(err)
		}
	}
	entry, err := combat.normal.EnterSolo(ctx, actor.PlayerID, actor.ID, id.New(), adventure.StarterAdventure, "normal", 3)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := misc.secretshop.PurchaseItem(ctx, actor.ID, "secret_item_herbal_root", 1); !errors.Is(err, adventure.ErrRunBusy) {
		t.Fatal("empty hand purchase bypassed enrollment", err)
	}
	_, granted, err := core.economy.GrantItem(ctx, actor.ID, "item-002", 1)
	if err != nil {
		t.Fatal("passive item reward rejected", err)
	}
	beforeActor, err := core.charRepo.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforePeer, err := core.charRepo.FindByID(ctx, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	result := testutil.RunConcurrentStressTest(t, testutil.GetStressConfig(), func(worker, op int) error {
		if (worker+op)%2 == 0 {
			_, err := econ.depot.SendMoney(ctx, actor.ID, peer.ID, 1)
			if !errors.Is(err, adventure.ErrRunBusy) {
				return fmt.Errorf("outgoing transfer: %v", err)
			}
			return nil
		}
		_, err := econ.depot.SendMoney(ctx, peer.ID, actor.ID, 1)
		return err
	})
	if result.Failures != 0 {
		t.Fatal(result)
	}
	afterActor, err := core.charRepo.FindByID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterPeer, err := core.charRepo.FindByID(ctx, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterActor.Money+afterPeer.Money != beforeActor.Money+beforePeer.Money || afterActor.Money-beforeActor.Money != int(result.TotalOps/2) {
		t.Fatal("credits/debits not conserved", beforeActor.Money, beforePeer.Money, afterActor.Money, afterPeer.Money)
	}
	if _, err := combat.normal.Command(ctx, adventure.RunCommand{PlayerID: actor.PlayerID, ActorID: actor.ID, RunID: entry.Command.RunID, ID: "exit", Version: 1, Action: "exit"}); err != nil {
		t.Fatal(err)
	}
	finalInv, err := core.invRepo.FindByCharacterID(ctx, actor.ID)
	if err != nil || len(finalInv.Items) != 1 || finalInv.Items[0].ID != granted.ID {
		t.Fatal("exit lost passive item reward", finalInv, err)
	}

}
