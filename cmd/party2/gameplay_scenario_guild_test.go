package main

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
)

// TestGameplayScenario_GuildMembership founds a guild, approves a second
// account's application, calls out from the room, then expels the member.
func TestGameplayScenario_GuildMembership(t *testing.T) {
	// Guild listing, color uniqueness and dissolution news are server-wide.
	a := newIsolatedScenarioApp(t)
	ctx := context.Background()
	_, masterName, masterToken, master := a.registerAccount(true)
	_, applicantName, applicantToken, applicant := a.registerAccount(true)
	gname := "g" + id.New()[:7]

	// Fixture: the founding fee is 5000 G (join_guild.cgi:17).
	if _, err := a.db.ExecContext(ctx, `UPDATE characters SET money = 5100 WHERE id = ?`, master); err != nil {
		t.Fatal(err)
	}

	type guildView struct {
		Kind string `json:"kind"`
		// Members are the place's present characters; Data.View.Members the roster.
		Members []struct {
			Name string `json:"name"`
		} `json:"members"`
		Data struct {
			Guilds []struct {
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"guilds"`
			View struct {
				Members []struct {
					CharacterID string `json:"character_id"`
					Name        string `json:"name"`
					Title       string `json:"title"`
					Pending     bool   `json:"pending"`
				} `json:"members"`
			} `json:"view"`
		} `json:"data"`
	}
	// look decodes the guild facts locally to keep them out of the shared scenarioContext (#2772).
	look := func(token, char string) (scenarioContext, guildView) {
		t.Helper()
		var body json.RawMessage
		a.call(token, http.MethodGet, "/api/v1/characters/"+char+"/context", "", http.StatusOK, &body)
		var c scenarioContext
		var v struct {
			Scene guildView `json:"scene"`
		}
		if err := json.Unmarshal(body, &c); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(body, &v); err != nil {
			t.Fatal(err)
		}
		return c, v.Scene
	}
	roster := func(v guildView) []string {
		var out []string
		for _, m := range v.Data.View.Members {
			out = append(out, m.Name+"＠"+m.Title)
		}
		return out
	}
	// guildEntry is the observed ＠ぎるど destination, or nil when it is not offered.
	guildEntry := func(c scenarioContext) json.RawMessage {
		for _, d := range c.Scene.Data.Destinations {
			if d.ID == "guild" {
				return d.EnterParams
			}
		}
		return nil
	}

	type stored struct {
		masterGold, applicantGold, points int
		leader                            string
		rows                              string // character:title:pending in roster order; same-second rows keep join order (#2955)
	}
	load := func() stored {
		t.Helper()
		var s stored
		if err := a.db.QueryRowContext(ctx, `SELECT (SELECT money FROM characters WHERE id = ?), (SELECT money FROM characters WHERE id = ?),
			g.points, g.leader_character_id, (SELECT GROUP_CONCAT(CONCAT(character_id, ':', title, ':', is_pending) ORDER BY joined_at, character_id)
			FROM guild_members WHERE guild_id = g.id) FROM guilds g WHERE g.name = ?`, master, applicant, gname).
			Scan(&s.masterGold, &s.applicantGold, &s.points, &s.leader, &s.rows); err != nil {
			t.Fatal(err)
		}
		return s
	}
	column := func(query string, args ...any) []string {
		t.Helper()
		rows, err := a.db.QueryContext(ctx, query, args...)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []string
		for rows.Next() {
			var s string
			if err := rows.Scan(&s); err != nil {
				t.Fatal(err)
			}
			out = append(out, s)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	// letters lists both characters' mail as sender>recipient:content, sorted.
	letters := func() []string {
		t.Helper()
		got := column(`SELECT CONCAT(sender_character_id, '>', recipient_character_id, ':', content) FROM character_letters
			WHERE sender_character_id IN (?, ?) OR recipient_character_id IN (?, ?)`, master, applicant, master, applicant)
		slices.Sort(got)
		return got
	}
	memories := func() []string {
		t.Helper()
		return column(`SELECT CONCAT(character_id, ':', kind, ':', content) FROM character_memories
			WHERE character_id IN (?, ?) AND kind LIKE 'guild%' ORDER BY id`, master, applicant)
	}
	assertMail := func(wantLetters, wantMemories []string) {
		t.Helper()
		slices.Sort(wantLetters)
		if got := letters(); !slices.Equal(got, wantLetters) {
			t.Fatalf("letters %q; want %q", got, wantLetters)
		}
		if got := memories(); !slices.Equal(got, wantMemories) {
			t.Fatalf("memories %q; want %q", got, wantMemories)
		}
	}

	// ＠つくる (join_guild.cgi:tsukuru) charges 5000 G, makes the founder ギルマス and moves it into the room.
	var startGold int
	if err := a.db.QueryRowContext(ctx, `SELECT money FROM characters WHERE id = ?`, applicant).Scan(&startGold); err != nil {
		t.Fatal(err)
	}
	association := a.enter(masterToken, master, a.context(masterToken, master), "join_guild")
	created := a.offered(masterToken, master, association, "guild_create", map[string]any{"name": gname}, http.StatusOK)
	if created.Context.Scene.LocationID != "guild" {
		t.Fatalf("founder not moved to the room: %s", created.Context.Scene.LocationID)
	}
	want := stored{masterGold: 100, applicantGold: startGold, leader: master, rows: master + ":ギルマス:0"}
	if got := load(); got != want {
		t.Fatalf("after founding: %+v; want %+v", got, want)
	}
	wantLetters := []string{}
	wantMemories := []string{master + ":guild_found:新ギルド" + gname + "を設立！"}
	assertMail(wantLetters, wantMemories)

	// ＠さんか (join_guild.cgi:sanka) adds a pending row and writes to the master.
	town, _ := look(applicantToken, applicant)
	if guildEntry(town) != nil {
		t.Fatal("unaffiliated character offered the guild room")
	}
	list := a.enter(applicantToken, applicant, town, "join_guild")
	_, listed := look(applicantToken, applicant)
	var guildID string
	for _, g := range listed.Data.Guilds {
		if g.Name == gname {
			guildID = g.ID
		}
	}
	if guildID == "" {
		t.Fatalf("%s is not listed: %+v", gname, listed.Data.Guilds)
	}
	a.elapseActionWait(applicant)
	selected := a.offered(applicantToken, applicant, list, "scene_select", map[string]any{"target_id": guildID}, http.StatusOK).Context
	a.elapseActionWait(applicant)
	a.offered(applicantToken, applicant, selected, "guild_apply", nil, http.StatusOK)
	want.rows += "," + applicant + ":参加申請中:1"
	if got := load(); got != want {
		t.Fatalf("after applying: %+v; want %+v", got, want)
	}
	wantLetters = append(wantLetters, applicant+">"+master+":【＋参加申請＋】"+gname+" 入団希望者 "+applicantName)
	assertMail(wantLetters, wantMemories)

	// ＠あたえる (guild.cgi:ataeru) with a title approves the applicant observed in the roster.
	a.elapseActionWait(master)
	room, view := look(masterToken, master)
	if r := roster(view); !slices.Equal(r, []string{masterName + "＠ギルマス", applicantName + "＠参加申請中"}) || !view.Data.View.Members[1].Pending {
		t.Fatalf("roster before approval: %q %+v", r, view.Data.View.Members)
	}
	a.offered(masterToken, master, room, "guild_assign", map[string]any{"member_id": view.Data.View.Members[1].CharacterID, "title": "団員"}, http.StatusOK)
	want.rows = master + ":ギルマス:0," + applicant + ":団員:0"
	if got := load(); got != want {
		t.Fatalf("after approval: %+v; want %+v", got, want)
	}
	wantLetters = append(wantLetters, master+">"+applicant+":【＋参加許可証＋】"+gname+" (ギルマス "+masterName+") から参加許可をもらいました")
	wantMemories = append(wantMemories, applicant+":guild_join:"+gname+"ギルドに加入")
	assertMail(wantLetters, wantMemories)

	// ＠ぎるど (system.cgi:set_action) now leads the member to the room, where only
	// ＠めんばー and ＠よびかける are its own (guild.cgi:25-36).
	a.elapseActionWait(applicant)
	// Back from the selected guild returns to the list, then to town.
	list = a.offered(applicantToken, applicant, selected, "scene_back", nil, http.StatusOK).Context
	a.elapseActionWait(applicant)
	town = a.offered(applicantToken, applicant, list, "scene_back", nil, http.StatusOK).Context
	a.elapseActionWait(applicant)
	enterGuild := guildEntry(town)
	if enterGuild == nil {
		t.Fatalf("member not offered the guild room: %+v", town.Scene.Data.Destinations)
	}
	a.enter(applicantToken, applicant, town, "guild")
	memberRoom, memberView := look(applicantToken, applicant)
	if r := roster(memberView); !slices.Equal(r, []string{masterName + "＠ギルマス", applicantName + "＠団員"}) ||
		!memberRoom.offers("guild_callout") || memberRoom.offers("guild_assign") || memberRoom.offers("guild_color") || memberRoom.offers("guild_message") {
		t.Fatalf("member room: %q %+v", r, memberRoom.AvailableActions)
	}

	// ＠よびかける (guild.cgi:yobikakeru) writes to every roster row and earns 1 point.
	// The master's newest letter is the application, so the 30-second letter wait
	// (home.md#letter-settings) applies; it is measured from the store, so age it there.
	if _, err := a.db.ExecContext(ctx, `UPDATE character_letters SET created_at = created_at - INTERVAL 30 SECOND
		WHERE sender_character_id IN (?, ?) OR recipient_character_id IN (?, ?)`, master, applicant, master, applicant); err != nil {
		t.Fatal(err)
	}
	a.elapseActionWait(applicant)
	a.offered(applicantToken, applicant, memberRoom, "guild_callout", map[string]any{"message": "よろしく"}, http.StatusOK)
	want.points = 1
	if got := load(); got != want {
		t.Fatalf("after the callout: %+v; want %+v", got, want)
	}
	wantLetters = append(wantLetters, applicant+">"+master+":よろしく", applicant+">"+applicant+":よろしく")
	assertMail(wantLetters, wantMemories)
	// Both members stand in the same place: the guild's own room.
	present := func(v guildView) []string {
		var out []string
		for _, m := range v.Members {
			out = append(out, m.Name)
		}
		slices.Sort(out)
		return out
	}
	both := []string{masterName, applicantName}
	slices.Sort(both)
	_, masterView := look(masterToken, master)
	if got := present(masterView); !slices.Equal(got, both) {
		t.Fatalf("room members seen by the master: %q; want %q", got, both)
	}

	// ＠あたえる>追放 removes the member and writes to it, without a memory (guild.cgi:241-245).
	a.elapseActionWait(master)
	room, view = look(masterToken, master)
	a.offered(masterToken, master, room, "guild_assign", map[string]any{"member_id": view.Data.View.Members[1].CharacterID, "title": "追放"}, http.StatusOK)
	want.rows = master + ":ギルマス:0"
	if got := load(); got != want {
		t.Fatalf("after expulsion: %+v; want %+v", got, want)
	}
	wantLetters = append(wantLetters, master+">"+applicant+":【＋追放＋】"+gname+" (ギルマス "+masterName+") から追放されました")
	assertMail(wantLetters, wantMemories)

	// The expelled character's room is gone; it may only go back, and the room stays closed.
	a.elapseActionWait(applicant)
	stale, staleView := look(applicantToken, applicant)
	if staleView.Kind != "selection_unavailable" || stale.offers("guild_callout") || !stale.offers("scene_back") {
		t.Fatalf("expelled member's room: %s %+v", staleView.Kind, stale.AvailableActions)
	}
	town = a.offered(applicantToken, applicant, stale, "scene_back", nil, http.StatusOK).Context
	if town.Scene.LocationID != "town" || guildEntry(town) != nil {
		t.Fatalf("expelled member's town: %s %+v", town.Scene.LocationID, town.Scene.Data.Destinations)
	}
	a.elapseActionWait(applicant)
	denied := a.command(applicantToken, applicant, "scene_enter", enterGuild, http.StatusForbidden)
	if denied.Error.Code != "SCENE_ACCESS_DENIED" || denied.Context.Scene.LocationID != "town" {
		t.Fatalf("expelled member entered the room: %+v", denied)
	}

	a.elapseActionWait(master)
	a.restart()
	for range 2 {
		if got := load(); got != want {
			t.Fatalf("after restart: %+v; want %+v", got, want)
		}
		assertMail(wantLetters, wantMemories)
		if c, _ := look(masterToken, master); c.Scene.LocationID != "guild" || !c.offers("guild_assign") {
			t.Fatalf("master after restart: %s %+v", c.Scene.LocationID, c.AvailableActions)
		}
		if c, _ := look(applicantToken, applicant); c.Scene.LocationID != "town" || guildEntry(c) != nil {
			t.Fatalf("expelled member after restart: %s", c.Scene.LocationID)
		}
	}
}

func TestGameplayScenario_GuildClearsStaleApplication(t *testing.T) {
	a := newIsolatedScenarioApp(t)
	ctx := context.Background()
	_, _, masterToken, master := a.registerAccount(true)
	if _, err := a.db.ExecContext(ctx, "UPDATE characters SET money = 5100 WHERE id = ?", master); err != nil {
		t.Fatal(err)
	}
	association := a.enter(masterToken, master, a.context(masterToken, master), "join_guild")
	a.offered(masterToken, master, association, "guild_create", map[string]any{"name": "g" + id.New()[:7]}, http.StatusOK)
	repo, err := database.NewGuildRepository(a.db)
	if err != nil {
		t.Fatal(err)
	}
	first, _, err := repo.GetGuildByCharacter(ctx, master)
	if err != nil {
		t.Fatal(err)
	}
	other, _, err := database.CreateTestGuildWithLeader(ctx, a.db, "g"+id.New()[:7], 0)
	if err != nil {
		t.Fatal(err)
	}

	for _, title := range []string{"団員", guild.RemovalTitle} {
		t.Run(title, func(t *testing.T) {
			a := *a
			a.t = t
			applicant, err := database.CreateTestCharacter(ctx, a.db, "App"+id.New()[:7])
			if err != nil {
				t.Fatal(err)
			}
			if _, err := repo.AddMember(ctx, guild.Member{GuildID: first.ID, CharacterID: applicant.ID, Role: guild.RoleMember, IsPending: true}); err != nil {
				t.Fatal(err)
			}
			if _, err := repo.AddMember(ctx, guild.Member{GuildID: other.ID, CharacterID: applicant.ID, Role: guild.RoleMember, Title: "斥候"}); err != nil {
				t.Fatal(err)
			}

			a.elapseActionWait(master)
			room := a.context(masterToken, master)
			got := a.offered(masterToken, master, room, "guild_assign", map[string]any{"member_id": applicant.ID, "title": title}, http.StatusOK)
			var reply struct {
				Outcome string `json:"outcome"`
				Message string `json:"message"`
			}
			if err := json.Unmarshal(got.Result, &reply); err != nil {
				t.Fatal(err)
			}
			if reply.Outcome != "stale_application_removed" || reply.Message != applicant.Name+"はすでに別のギルドに加入しています" || got.Context.Scene.LocationID != "guild" {
				t.Fatalf("stale application reply = %+v, scene = %q", reply, got.Context.Scene.LocationID)
			}

			var observed struct {
				Scene struct {
					Data struct {
						View struct {
							Members []struct {
								CharacterID string `json:"character_id"`
							} `json:"members"`
						} `json:"view"`
					} `json:"data"`
				} `json:"scene"`
			}
			a.call(masterToken, http.MethodGet, "/api/v1/characters/"+master+"/context", "", http.StatusOK, &observed)
			if members := observed.Scene.Data.View.Members; len(members) != 1 || members[0].CharacterID != master {
				t.Fatalf("refreshed roster = %+v, want only leader", members)
			}
			activeGuild, member, err := repo.GetGuildByCharacter(ctx, applicant.ID)
			if err != nil || activeGuild.ID != other.ID || member.Title != "斥候" {
				t.Fatalf("existing affiliation = %q / %+v, err = %v", activeGuild.ID, member, err)
			}
			var letters, memories int
			if err := a.db.QueryRowContext(ctx, `SELECT
				(SELECT COUNT(*) FROM character_letters WHERE recipient_character_id = ?),
				(SELECT COUNT(*) FROM character_memories WHERE character_id = ? AND kind = 'guild_join')`, applicant.ID, applicant.ID).Scan(&letters, &memories); err != nil {
				t.Fatal(err)
			}
			if letters != 0 || memories != 0 {
				t.Fatalf("stale application wrote %d letters and %d join memories", letters, memories)
			}
		})
	}
}
