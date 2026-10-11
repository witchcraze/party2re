package http_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/character"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestReplayArchiveAuthorizationMariaDB(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
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
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	sessions := player.NewValkeySessionRepository(nil)
	identities, err := player.NewService(players, sessions)
	if err != nil {
		t.Fatal(err)
	}
	testGuild, guildOwner, err := database.CreateTestGuildWithLeader(ctx, db, "Replay "+id.New()[:8], 0)
	if err != nil {
		t.Fatal(err)
	}
	actors := map[string]corecharacter.Character{}
	tokens := map[string]string{}
	for _, role := range []string{"owner", "participant", "spectator", "guildmate", "unrelated", "second"} {
		actor := guildOwner
		if role != "owner" {
			actor, err = database.CreateTestCharacterWithFunds(ctx, db, "Replay "+role, 0)
			if err != nil {
				t.Fatal(err)
			}
		}
		if role == "second" {
			actor.PlayerID = actors["owner"].PlayerID
			if _, err := db.ExecContext(ctx, "UPDATE characters SET player_id = ? WHERE id = ?", actor.PlayerID, actor.ID); err != nil {
				t.Fatal(err)
			}
		}
		actors[role] = actor
		sess, err := coreplayer.NewSession(actor.PlayerID, time.Now().UTC(), time.Hour)
		if err != nil {
			t.Fatal(err)
		}
		if err := sessions.Save(ctx, sess); err != nil {
			t.Fatal(err)
		}
		tokens[role] = sess.ID
	}
	guilds, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := guilds.AddMember(ctx, guild.Member{GuildID: testGuild.ID, CharacterID: actors["guildmate"].ID, Role: guild.RoleMember, JoinedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	svc, err := replay.NewService(repo, identities, chars)
	if err != nil {
		t.Fatal(err)
	}
	charService, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	router := newTestHandler(t, identities, charService, &stubAdventureService{}, &stubShopService{}, apihttp.WithReplay(svc)).Router()
	prefix := id.New()
	now := time.Now().UTC().Truncate(time.Second)
	var visible []string
	for i, suffix := range []string{"1", "2", "3", "4", "5", "6-other", "7-unknown", "8-global", "9-second"} {
		rep := replay.BattleReplay{
			ID: prefix + suffix, Provenance: replay.ProvenancePersonal, ArchiveRecipientID: actors["owner"].ID,
			CombatType: replay.CombatTypeAdventure, InitiatorID: actors["owner"].ID, InitiatorName: "Owner",
			OpponentID: actors["participant"].ID, OpponentName: "Participant", Outcome: corebattle.OutcomeWin,
			InitialParticipants: []replay.ParticipantSnapshot{{ID: actors["owner"].ID, Attack: 999}},
			TurnLogs:            []corebattle.TurnLog{{Turn: 1, Message: "private raw turn"}}, CreatedAt: now,
		}
		switch i {
		case 5:
			rep.ArchiveRecipientID = actors["participant"].ID
		case 6:
			rep.Provenance = ""
		case 7:
			rep.Provenance = "global"
		case 8:
			rep.ArchiveRecipientID = actors["second"].ID
		default:
			visible = append([]string{rep.ID}, visible...)
		}
		if err := repo.Save(ctx, rep); err != nil {
			t.Fatal(err)
		}
	}
	// An old writer omitting all new columns retains its data and receives no inferred recipient.
	migratedID := prefix + "migrated"
	if _, err := db.ExecContext(ctx, `INSERT INTO battle_replays
		(id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name, outcome, initial_participants_json, turn_logs_json, created_at)
		VALUES (?, 'adventure', ?, 'Owner', ?, 'Participant', 'win', '[]', '[]', ?)`, migratedID, actors["owner"].ID, actors["participant"].ID, now); err != nil {
		t.Fatal(err)
	}
	// Malformed private payloads must not reveal existence through a decode failure to a non-owner.
	if _, err := db.ExecContext(ctx, `UPDATE battle_replays SET initial_participants_json = '"invalid snapshot"' WHERE id = ?`, prefix+"6-other"); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindByID(ctx, migratedID)
	if err != nil || stored.Provenance != "" || stored.ArchiveRecipientID != "" {
		t.Fatalf("historical preservation: %+v %v", stored, err)
	}

	request := func(path, auth string, status int) []byte {
		t.Helper()
		r := httptest.NewRequest(http.MethodGet, path, nil)
		if auth != "" {
			r.Header.Set("Authorization", auth)
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		if w.Code != status {
			t.Fatalf("%s: got %d want %d: %s", path, w.Code, status, w.Body.String())
		}
		return w.Body.Bytes()
	}
	personalID := visible[0]
	if _, err := svc.GetReplay(ctx, actors["owner"].ID, personalID); !errors.Is(err, coreplayer.ErrAuthentication) {
		t.Fatalf("character id spoofed a session: %v", err)
	}
	for _, role := range []string{"owner", "participant", "spectator", "guildmate", "unrelated", ""} {
		t.Run("detail/"+role, func(t *testing.T) {
			status := 404
			var auth string
			if role != "" {
				auth = "Bearer " + tokens[role]
			}
			if role == "owner" {
				status = 200
			}
			request("/replays/"+personalID, auth, status)
			_, err := svc.GetReplay(ctx, tokens[role], personalID)
			if status == 404 && !errors.Is(err, replay.ErrReplayNotFound) {
				t.Fatal(err)
			}
			if status == 200 && err != nil {
				t.Fatal(err)
			}
		})
	}
	for _, replayID := range []string{prefix + "6-other", prefix + "7-unknown", prefix + "8-global", migratedID, prefix + "absent"} {
		got := request("/replays/"+replayID, "Bearer "+tokens["owner"], 404)
		if string(got) != "{\"error\":\"battle replay not found\"}\n" {
			t.Fatalf("different denial: %s", got)
		}
	}
	for _, auth := range []string{"Bearer invalid", "Bearer ", "Basic invalid"} {
		request("/replays/"+personalID, auth, 401)
		request("/replays/absent", auth, 401)
	}
	request("/replays/"+prefix+"9-second", "Bearer "+tokens["owner"], 200)
	for _, role := range []string{"participant", "spectator", "guildmate", "unrelated"} {
		for _, cursor := range []string{"", "?cursor="} {
			request("/characters/"+actors["owner"].ID+"/replays"+cursor, "Bearer "+tokens[role], 403)
		}
		if _, err := svc.GetCharacterHistory(ctx, tokens[role], actors["owner"].ID, "", 10); !errors.Is(err, replay.ErrReplayNotFound) {
			t.Fatal(err)
		}
		if _, err := svc.GetCharacterHistoryByCursor(ctx, tokens[role], actors["owner"].ID, "", 10, ""); !errors.Is(err, replay.ErrReplayNotFound) {
			t.Fatal(err)
		}
	}
	for _, cursor := range []string{"", "?cursor="} {
		request("/characters/"+actors["owner"].ID+"/replays"+cursor, "", 401)
	}
	for _, filter := range []string{"", replay.CombatTypeAdventure, replay.CombatTypePvP} {
		want := visible
		if filter == replay.CombatTypePvP {
			want = nil
		}
		history, err := svc.GetCharacterHistory(ctx, tokens["owner"], actors["owner"].ID, filter, 20)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, h := range history {
			got = append(got, h.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("history: %v want %v", got, want)
		}
		// SQL keyset predicates, including the supported partial cursor forms, cannot select hidden rows.
		for _, boundary := range []struct {
			at time.Time
			id string
		}{{}, {now, visible[1]}, {now.Add(time.Second), ""}, {time.Time{}, visible[1]}} {
			rows, err := repo.FindByCharacterByCursor(ctx, actors["owner"].ID, filter, 20, boundary.at, boundary.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range rows {
				found := false
				for _, allowed := range want {
					if h.ID == allowed {
						found = true
					}
				}
				if !found {
					t.Fatalf("hidden header: %s", h.ID)
				}
			}
		}
	}
	var paged []string
	cursor := ""
	for n := 0; n < 4; n++ {
		body := request("/characters/"+actors["owner"].ID+"/replays?limit=2&cursor="+cursor, "Bearer "+tokens["owner"], 200)
		var page pagination.CursorPage[replay.ReplayHeader]
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		for _, h := range page.Items {
			paged = append(paged, h.ID)
		}
		if page.NextCursor != "" {
			_, nextID := pagination.DecodeCursorParts(page.NextCursor)
			if nextID != page.Items[len(page.Items)-1].ID {
				t.Fatal("cursor reveals a hidden id")
			}
		}
		if !page.HasMore {
			break
		}
		cursor = page.NextCursor
	}
	if !reflect.DeepEqual(paged, visible) {
		t.Fatalf("pages: %v want %v", paged, visible)
	}
	empty, err := svc.GetCharacterHistoryByCursor(ctx, tokens["owner"], actors["owner"].ID, "", 2, pagination.EncodeCursor(now, visible[len(visible)-1]))
	if err != nil || len(empty.Items) != 0 || empty.HasMore || empty.NextCursor != "" {
		t.Fatalf("empty page: %+v %v", empty, err)
	}
	for _, role := range []string{"", "owner", "participant"} {
		var auth string
		if role != "" {
			auth = "Bearer " + tokens[role]
		}
		if body := request("/replays/recent", auth, 200); string(body) != "[]\n" {
			t.Fatalf("public leak: %s", body)
		}
		body := request("/replays/recent?cursor=&limit=1", auth, 200)
		var page pagination.CursorPage[replay.ReplayHeader]
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		if len(page.Items) != 0 || page.HasMore || page.NextCursor != "" {
			t.Fatalf("public cursor leak: %s", body)
		}
	}
	for _, filter := range []string{"", replay.CombatTypeAdventure} {
		rows, err := repo.FindRecent(ctx, filter, 1)
		if err != nil || len(rows) != 0 {
			t.Fatalf("recent SQL: %+v %v", rows, err)
		}
		for _, boundary := range []struct {
			at time.Time
			id string
		}{{}, {now, personalID}, {now, ""}, {time.Time{}, personalID}} {
			rows, err := repo.FindRecentByCursor(ctx, filter, 1, boundary.at, boundary.id)
			if err != nil || len(rows) != 0 {
				t.Fatalf("recent cursor SQL: %+v %v", rows, err)
			}
		}
	}
}
