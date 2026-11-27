package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/collection"
	"github.com/witchcraze/party2re/internal/home"
)

// TestGameplayScenario_HomeHousingAndMailbox builds a house, visits it from a
// second account and exchanges a letter between the two homes.
func TestGameplayScenario_HomeHousingAndMailbox(t *testing.T) {
	// A town admits at most ten houses (_town.cgi:58); keep the count our own.
	a := newIsolatedScenarioApp(t)
	ctx := context.Background()
	_, ownerName, ownerToken, owner := a.registerAccount(true)
	_, visitorName, visitorToken, visitor := a.registerAccount(true)

	// Fixture: new_entry.cgi:184 starts at 200G, below a house. Fund the owner
	// for the first weapon (20G) and a メケメケ村 house (town1.cgi:17, 500G).
	const funds, weaponCost, housePrice, cycleDays = 600, 20, 500, 5
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET money = ? WHERE id = ?`, funds, owner); err != nil {
		t.Fatal(err)
	}

	visit := func(token, actor, destination string) scenarioContext {
		t.Helper()
		a.elapseActionWait(actor)
		c := a.context(token, actor)
		for c.Scene.LocationID != "town" {
			c = a.offered(token, actor, c, "scene_back", nil, http.StatusOK).Context
		}
		return a.enter(token, actor, c, destination)
	}
	// act issues a command the actor's current observation offers.
	act := func(token, actor, action string, params map[string]any) scenarioOutcome {
		t.Helper()
		a.elapseActionWait(actor)
		return a.offered(token, actor, a.context(token, actor), action, params, http.StatusOK)
	}
	sub := func(token, actor string, c scenarioContext, destination string) scenarioContext {
		t.Helper()
		a.elapseActionWait(actor)
		return a.enter(token, actor, c, destination)
	}
	back := func(token, actor string) scenarioContext {
		t.Helper()
		a.elapseActionWait(actor)
		return a.offered(token, actor, a.context(token, actor), "scene_back", nil, http.StatusOK).Context
	}

	// The owner's purchase records weapon-01 in the owner's collection only.
	shop := visit(ownerToken, owner, "shop_weapon")
	if len(shop.Scene.Data.Items) == 0 || shop.Scene.Data.Items[0].RetailPrice != weaponCost {
		t.Fatalf("expected the first weapon at %dG: %+v", weaponCost, shop.Scene.Data.Items)
	}
	weapon := shop.Scene.Data.Items[0].ID
	act(ownerToken, owner, "shop_purchase", shopPurchaseParams(shop))

	// ＠たてる (_town.cgi:47-83): pay the price and own a house for cycle days.
	district := visit(ownerToken, owner, "town1")
	terms := district.Scene.Data.Town
	if terms.Price != housePrice || terms.CycleDays != cycleDays || len(terms.HouseStyles) == 0 || len(district.Scene.Data.Houses) != 0 {
		t.Fatalf("district before building: %+v houses=%+v", terms, district.Scene.Data.Houses)
	}
	style := terms.HouseStyles[0]
	start := time.Now().UTC()
	built := act(ownerToken, owner, "house_build", map[string]any{"house_style": style})
	end := time.Now().UTC()
	var house home.TownHouse
	if err := json.Unmarshal(built.Result, &house); err != nil {
		t.Fatal(err)
	}
	if house.CharacterID != owner || house.OwnerName != ownerName || house.TownID != terms.ID || house.HouseStyle != style ||
		house.ExpiresAt.Before(start.Add(cycleDays*24*time.Hour-time.Second)) || house.ExpiresAt.After(end.Add(cycleDays*24*time.Hour+time.Second)) {
		t.Fatalf("built house: %+v", house)
	}
	if built.Context.Character.Gold != funds-weaponCost-housePrice {
		t.Fatalf("gold after building: %d", built.Context.Character.Gold)
	}
	var storedGold int
	var storedTown, storedStyle string
	var storedExpiry time.Time
	if err := a.db.QueryRowContext(ctx, `SELECT c.money, h.town_id, h.house_style, h.expires_at FROM characters c JOIN character_homes h ON h.character_id = c.id WHERE c.id = ?`, owner).
		Scan(&storedGold, &storedTown, &storedStyle, &storedExpiry); err != nil {
		t.Fatal(err)
	}
	if storedGold != funds-weaponCost-housePrice || storedTown != terms.ID || storedStyle != style || !storedExpiry.Equal(house.ExpiresAt.Truncate(time.Microsecond)) {
		t.Fatalf("stored house: gold=%d town=%s style=%s expires=%v", storedGold, storedTown, storedStyle, storedExpiry)
	}

	// The visitor finds the owner's house in the district and visits it.
	seen := visit(visitorToken, visitor, "town1")
	if len(seen.Scene.Data.Houses) != 1 || seen.Scene.Data.Houses[0].CharacterID != owner || seen.Scene.Data.Houses[0].OwnerName != ownerName {
		t.Fatalf("visitor's district: %+v", seen.Scene.Data.Houses)
	}
	target := seen.Scene.Data.Houses[0].CharacterID
	visit(visitorToken, visitor, "home")
	visited := act(visitorToken, visitor, "scene_select", map[string]any{"target_kind": "home", "target_id": target}).Context
	view := visited.Scene.Data.View
	if view.Owner.ID != owner || view.IsOwner || view.Private != nil || view.Home.TownID != terms.ID {
		t.Fatalf("visited home: %+v", view)
	}
	// Visitors get only ＠ねる and the record views (home.cgi:26-42).
	for _, d := range visited.Scene.Data.Destinations {
		if !slices.Contains([]string{"home_collection", "home_monster_book", "home_job_master", "home_profile", "home_adventure_record"}, d.ID) {
			t.Fatalf("visitor offered owner destination %s", d.ID)
		}
	}
	if visited.offers("home_letter_send") || visited.offers("home_color") {
		t.Fatalf("visitor offered owner commands: %+v", visited.AvailableActions)
	}
	// The collection scene's "items" section is not the shared Items shape.
	weapons := func(token, actor string, from scenarioContext) (ownerID string, discovered bool) {
		t.Helper()
		var params json.RawMessage
		for _, d := range from.Scene.Data.Destinations {
			if d.ID == "home_collection" {
				params = d.EnterParams
			}
		}
		if params == nil {
			t.Fatalf("home_collection is not offered: %+v", from.Scene.Data.Destinations)
		}
		var out struct {
			Context struct {
				Scene struct {
					Data struct {
						OwnerID string `json:"owner_id"`
						Weapons struct {
							Entries []collection.ItemCollectionEntry `json:"entries"`
						} `json:"weapons"`
					} `json:"data"`
				} `json:"scene"`
			} `json:"context"`
		}
		a.elapseActionWait(actor)
		body := `{"action":"scene_enter","params":` + string(params) + `}`
		a.call(token, http.MethodPost, "/api/v1/characters/"+actor+"/actions", body, http.StatusOK, &out)
		data := out.Context.Scene.Data
		return data.OwnerID, slices.ContainsFunc(data.Weapons.Entries, func(e collection.ItemCollectionEntry) bool { return e.ItemID == weapon })
	}
	if id, ok := weapons(visitorToken, visitor, visited); id != owner || !ok {
		t.Fatalf("visited collection: owner=%s weapon=%v", id, ok)
	}
	visited = a.command(visitorToken, visitor, "scene_back", map[string]any{}, http.StatusOK).Context
	monsters := sub(visitorToken, visitor, visited, "home_monster_book")
	if monsters.Scene.Data.OwnerID != owner {
		t.Fatalf("visited monster book owner: %s", monsters.Scene.Data.OwnerID)
	}
	jobs := sub(visitorToken, visitor, back(visitorToken, visitor), "home_job_master")
	if jobs.Scene.Data.Mastery.CharacterID != owner {
		t.Fatalf("visited job master owner: %s", jobs.Scene.Data.Mastery.CharacterID)
	}
	if id, ok := weapons(visitorToken, visitor, visit(visitorToken, visitor, "home")); id != visitor || ok {
		t.Fatalf("visitor's own collection: owner=%s weapon=%v", id, ok)
	}
	// Leave the collection: the shared observation cannot decode it.
	a.command(visitorToken, visitor, "scene_back", map[string]any{}, http.StatusOK)

	// ＠かすたむすきる: the owner's update is observed back in the owner's view.
	skills := sub(ownerToken, owner, visit(ownerToken, owner, "home"), "home_custom_skill")
	if skills.Scene.Data.CustomSkill != nil {
		t.Fatalf("custom skill before setting: %+v", skills.Scene.Data.CustomSkill)
	}
	set := act(ownerToken, owner, "custom_skill_set", map[string]any{"name": "ひっさつ", "comment": "いくぞ"})
	if got := set.Context.Scene.Data.CustomSkill; got == nil || got.CharacterID != owner || got.Name != "ひっさつ" || got.Comment != "いくぞ" {
		t.Fatalf("custom skill after setting: %+v", got)
	}
	a.assertRowCount(`SELECT COUNT(*) FROM character_custom_skills WHERE character_id = ? AND skill_name = 'ひっさつ' AND skill_comment = 'いくぞ'`, owner, 1)

	// ＠からー then ＠てがみをかく: the letter carries the sender's colour (system.cgi send_letter).
	homeView := back(ownerToken, owner)
	if len(homeView.Scene.Data.ColorSamples) == 0 {
		t.Fatalf("owner's home lacks colour samples: %+v", homeView.Scene.Data)
	}
	color := homeView.Scene.Data.ColorSamples[0].Color
	// home.cgi color keeps the code as written (#2935).
	if got := act(ownerToken, owner, "home_color", map[string]any{"color": color}).Context.Scene.Data.View.Owner.Color; got != color {
		t.Fatalf("owner colour %q after choosing %q", got, color)
	}
	letterParams := map[string]any{"recipient_name": visitorName, "content": "あそびにきてね"}
	rejected := a.command(ownerToken, owner, "home_letter_send", letterParams, http.StatusConflict)
	if rejected.Error.Code != "ACTION_WAIT" {
		t.Fatalf("letter during action wait: %+v", rejected)
	}
	a.assertRowCount(`SELECT COUNT(*) FROM character_letters WHERE sender_character_id = ?`, owner, 0)
	sent := act(ownerToken, owner, "home_letter_send", letterParams)
	var letter home.Letter
	if err := json.Unmarshal(sent.Result, &letter); err != nil {
		t.Fatal(err)
	}
	if letter.ID == "" || letter.SenderCharacterID != owner || letter.RecipientCharacterID != visitor || letter.Color != color || letter.IsRead {
		t.Fatalf("sent letter: %+v", letter)
	}
	assertLetter := func(isRead, deletedBySender, deletedByRecipient bool) {
		t.Helper()
		var sender, recipient, storedColor, content string
		var read, readAt, bySender, byRecipient bool
		if err := a.db.QueryRowContext(ctx, `SELECT sender_character_id, recipient_character_id, color, content, is_read, read_at IS NOT NULL, is_deleted_by_sender, is_deleted_by_recipient FROM character_letters WHERE id = ?`, letter.ID).
			Scan(&sender, &recipient, &storedColor, &content, &read, &readAt, &bySender, &byRecipient); err != nil {
			t.Fatal(err)
		}
		if sender != owner || recipient != visitor || storedColor != color || content != "あそびにきてね" ||
			read != isRead || readAt != isRead || bySender != deletedBySender || byRecipient != deletedByRecipient {
			t.Fatalf("stored letter: %s→%s %s %q read=%v/%v deleted=%v/%v", sender, recipient, storedColor, content, read, readAt, bySender, byRecipient)
		}
	}
	assertLetter(false, false, false)
	mailbox := func(token, actor, destination string) []home.Letter {
		t.Helper()
		return sub(token, actor, visit(token, actor, "home"), destination).Scene.Data.Letters
	}
	ids := func(letters []home.Letter) []string {
		out := []string{}
		for _, l := range letters {
			out = append(out, l.ID)
		}
		return out
	}
	assertMailboxes := func(step string, ownerOut, visitorIn []string) {
		t.Helper()
		a.resetRateLimits(ownerToken, visitorToken)
		got := [4][]string{ids(mailbox(ownerToken, owner, "home_inbox")), ids(mailbox(ownerToken, owner, "home_outbox")),
			ids(mailbox(visitorToken, visitor, "home_inbox")), ids(mailbox(visitorToken, visitor, "home_outbox"))}
		want := [4][]string{{}, ownerOut, visitorIn, {}}
		for i := range got {
			if !slices.Equal(got[i], want[i]) {
				t.Fatalf("%s: mailboxes owner in/out, visitor in/out = %v; want %v", step, got, want)
			}
		}
	}
	assertMailboxes("sent", []string{letter.ID}, []string{letter.ID})

	// The recipient's own home announces the unread letter until ＠てがみをよむ.
	if v := visit(visitorToken, visitor, "home").Scene.Data.View; v.Private == nil || v.Private.UnreadLetterCount != 1 {
		t.Fatalf("recipient home before reading: %+v", v.Private)
	}
	inbox := sub(visitorToken, visitor, visit(visitorToken, visitor, "home"), "home_inbox")
	if got := inbox.Scene.Data.Letters; len(got) != 1 || got[0].SenderName != ownerName || got[0].Color != color || got[0].IsRead {
		t.Fatalf("recipient inbox: %+v", got)
	}
	read := act(visitorToken, visitor, "home_letter_read", map[string]any{"letter_id": inbox.Scene.Data.Letters[0].ID})
	if got := read.Context.Scene.Data.Letters; len(got) != 1 || !got[0].IsRead {
		t.Fatalf("inbox after reading: %+v", got)
	}
	assertLetter(true, false, false)
	if v := visit(visitorToken, visitor, "home").Scene.Data.View; v.Private == nil || v.Private.UnreadLetterCount != 0 {
		t.Fatalf("recipient home after reading: %+v", v.Private)
	}

	// Deleting from the inbox leaves the sender's outbox copy.
	inbox = sub(visitorToken, visitor, visit(visitorToken, visitor, "home"), "home_inbox")
	deleted := act(visitorToken, visitor, "home_letter_delete", map[string]any{"letter_id": inbox.Scene.Data.Letters[0].ID})
	if len(deleted.Context.Scene.Data.Letters) != 0 {
		t.Fatalf("inbox after deleting: %+v", deleted.Context.Scene.Data.Letters)
	}
	assertLetter(true, false, true)
	assertMailboxes("recipient deleted", []string{letter.ID}, []string{})
}
