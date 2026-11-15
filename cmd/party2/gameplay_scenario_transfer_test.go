package main

import (
	"context"
	"net"
	"net/http"
	"testing"
)

func TestGameplayScenario_TwoAccountTransfers(t *testing.T) {
	// The market's server-wide listing ceiling must not depend on unrelated tests.
	a := newIsolatedScenarioApp(t)
	senderToken, sender := a.register()
	recipientToken, recipient := a.register()
	tokens := [2]string{senderToken, recipientToken}
	heroes := [2]string{sender, recipient}

	// new_entry.cgi:184 starts each wallet at 200G. _data.cgi's first weapon
	// costs 10G, doubled by weapon.cgi:61. depot.cgi:355-368 sends without a fee.
	const startingGold, weaponCost, remittance, listingPrice = 200, 20, 40, 60
	const weapon = "weapon-01"
	type assets struct{ gold, deposit, held, stored, hand, listed int }
	want := [2]assets{{gold: startingGold}, {gold: startingGold}}
	wantGold, wantItems := 2*startingGold, 0
	ip, _, err := net.SplitHostPort(a.remoteAddr)
	if err != nil {
		t.Fatal(err)
	}

	visit := func(actor int, destination string) scenarioContext {
		t.Helper()
		a.elapseActionWait(heroes[actor])
		c := a.context(tokens[actor], heroes[actor])
		if c.Scene.LocationID != "town" {
			c = a.offered(tokens[actor], heroes[actor], c, "scene_back", map[string]any{}, http.StatusOK).Context
		}
		return a.enter(tokens[actor], heroes[actor], c, destination)
	}
	act := func(actor int, c scenarioContext, action string, params map[string]any) scenarioOutcome {
		t.Helper()
		a.elapseActionWait(heroes[actor])
		return a.offered(tokens[actor], heroes[actor], c, action, params, http.StatusOK)
	}
	// Observe every asset through its owner's session. Hands and market escrow
	// count too: moving a unit out of the Depot must not hide a duplicate or loss.
	check := func(step string) [2]scenarioContext {
		t.Helper()
		// A journey spans HTTP rate-limit windows; expire only its own buckets.
		for _, key := range []string{"http:ip:" + ip, "http:auth:" + senderToken, "http:auth:" + recipientToken} {
			if err := a.valkey.Do(context.Background(), a.valkey.B().Expire().Key("party2:ratelimit:"+key).Seconds(0).Build()).Error(); err != nil {
				t.Fatal(err)
			}
		}
		var depots [2]scenarioContext
		gold, items := 0, 0
		for actor := range heroes {
			bank := visit(actor, "bank")
			got := assets{gold: bank.Character.Gold, deposit: int(bank.Scene.Data.Deposit)}
			shop := visit(actor, "shop_weapon")
			if offer := shop.Scene.Data.SaleOffer; offer != nil {
				if offer.ItemDefinitionID != weapon || offer.ItemInstanceID == "" {
					t.Fatalf("%s actor %d: unexpected hand %+v", step, actor, offer)
				}
				got.hand++
			}
			market := visit(actor, "fleamarket")
			for _, listing := range market.Scene.Data.OwnedListings {
				if listing.ItemID != weapon || listing.Price != listingPrice || listing.SellerCharacterID != heroes[actor] || listing.Status != "active" {
					t.Fatalf("%s actor %d: unexpected escrow %+v", step, actor, listing)
				}
				got.listed++
			}
			d := visit(actor, "depot")
			depots[actor] = d
			for _, r := range d.Scene.Data.PendingRemittances {
				got.held += r.Amount
			}
			if d.Scene.Data.ItemCount != len(d.Scene.Data.Items) {
				t.Fatalf("%s actor %d: Depot count disagrees with contents: %+v", step, actor, d.Scene.Data)
			}
			for _, item := range d.Scene.Data.Items {
				if item.DefinitionID != weapon || item.Quantity != 1 || item.ID == "" {
					t.Fatalf("%s actor %d: unexpected stored item %+v", step, actor, item)
				}
				got.stored += item.Quantity
			}
			if got != want[actor] {
				t.Fatalf("%s actor %d: assets %+v, want %+v", step, actor, got, want[actor])
			}
			gold += got.gold + got.deposit + got.held
			items += got.stored + got.hand + got.listed
		}
		if gold != wantGold || items != wantItems {
			t.Fatalf("%s: conservation: gold=%d items=%d, want gold=%d items=%d", step, gold, items, wantGold, wantItems)
		}
		return depots
	}
	check("registered")
	for actor, amount := range []int{20, 30} {
		act(actor, visit(actor, "bank"), "bank_deposit", map[string]any{"amount": amount})
		want[actor].gold -= amount
		want[actor].deposit += amount
		check("bank deposit")
	}

	shop := visit(0, "shop_weapon")
	if len(shop.Scene.Data.Items) == 0 || shop.Scene.Data.Items[0].ID != weapon || shop.Scene.Data.Items[0].RetailPrice != weaponCost {
		t.Fatalf("expected the legacy first weapon at %dG: %+v", weaponCost, shop.Scene.Data.Items)
	}
	bought := act(0, shop, "shop_purchase", shopPurchaseParams(shop))
	if bought.Context.Scene.Data.SaleOffer == nil {
		t.Fatal("purchase did not expose the held item")
	}
	itemID := bought.Context.Scene.Data.SaleOffer.ItemInstanceID
	want[0].gold -= weaponCost
	want[0].hand = 1
	wantGold -= weaponCost
	wantItems = 1
	d := check("weapon purchase")

	act(0, d[0], "depot_deposit", map[string]any{"item_id": itemID})
	want[0].hand, want[0].stored = 0, 1
	d = check("item deposited")
	// okuru sends from the hand (depot.cgi:371-405), so withdraw before sending.
	itemID = d[0].Scene.Data.Items[0].ID
	act(0, d[0], "depot_withdraw", map[string]any{"item_id": itemID})
	want[0].hand, want[0].stored = 1, 0
	d = check("item withdrawn for sending")
	act(0, d[0], "depot_send_item", map[string]any{"item_id": itemID, "recipient_character_id": recipient})
	want[0].hand, want[1].stored = 0, 1
	d = check("item sent once")

	act(0, d[0], "depot_send_money", map[string]any{"amount": remittance, "recipient_character_id": recipient})
	want[0].gold -= remittance
	want[1].held = remittance
	d = check("money held for recipient")
	// The sender's real session cannot collect on behalf of the recipient.
	a.call(senderToken, http.MethodPost, "/api/v1/characters/"+recipient+"/actions", `{"action":"depot_withdraw","params":{}}`, http.StatusForbidden, nil)
	d = check("cross-account command rejected without mutation")
	act(1, d[1], "depot_withdraw", map[string]any{})
	want[1].held = 0
	want[1].gold += remittance
	d = check("recipient collected money without withdrawing item")

	// The recipient now sells the delivered unit back to the original sender.
	itemID = d[1].Scene.Data.Items[0].ID
	act(1, visit(1, "fleamarket"), "fleamarket_list", map[string]any{"item_id": itemID, "price": listingPrice})
	want[1].stored, want[1].listed = 0, 1
	check("item in market escrow")
	market := visit(0, "fleamarket")
	if len(market.Scene.Data.Listings) != 1 || market.Scene.Data.Listings[0].SellerCharacterID != recipient {
		t.Fatalf("buyer cannot observe recipient's listing: %+v", market.Scene.Data.Listings)
	}
	act(0, market, "fleamarket_purchase", map[string]any{"listing_id": market.Scene.Data.Listings[0].ID})
	want[0].gold -= listingPrice
	want[1].held = listingPrice
	want[1].listed, want[0].stored = 0, 1
	check("market purchase held seller proceeds and delivered once")
	act(1, visit(1, "depot"), "depot_withdraw", map[string]any{})
	want[1].held = 0
	want[1].gold += listingPrice
	check("seller collected flea market remittance")
	a.restart()
	check("restart preserved both sessions and assets")
	check("repeated observations did not collect or duplicate assets")
}
