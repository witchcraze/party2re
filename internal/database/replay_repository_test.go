package database_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/replay"
)

func TestReplayRepository(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	// Pruning must only delete this test's rows, even when other packages use the same DB.
	db.SetMaxOpenConns(1)
	var table, ddl string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE battle_replays").Scan(&table, &ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, strings.Replace(ddl, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC()
	replayID := fmt.Sprintf("rep_%016x", now.UnixNano())
	r := replay.BattleReplay{
		Provenance: replay.ProvenancePersonal, ArchiveRecipientID: "charA",
		ID:            replayID,
		CombatType:    replay.CombatTypePvP,
		InitiatorID:   "charA",
		InitiatorName: "Hero A",
		OpponentID:    "charB",
		OpponentName:  "Hero B",
		Outcome:       corebattle.OutcomeWin,
		WinnerID:      "charA",
		LoserID:       "charB",
		TotalTurns:    3,
		InitialParticipants: []replay.ParticipantSnapshot{
			{ID: "charA", Name: "Hero A", MaxHP: 100, Attack: 30, Defense: 10, Level: 10},
			{ID: "charB", Name: "Hero B", MaxHP: 80, Attack: 20, Defense: 8, Level: 9},
		},
		TurnLogs: []corebattle.TurnLog{
			{
				Turn:        1,
				ActorID:     "charA",
				ActionName:  "こうげき",
				TargetID:    "charB",
				DamageDealt: 22,
				Message:     "Hero A の攻撃！ 22ダメージ！",
				RemainingHP: map[string]int{"charA": 100, "charB": 58},
			},
		},
		CreatedAt: now,
	}

	// 1. Save
	if err := repo.Save(ctx, r); err != nil {
		t.Fatalf("Save replay failed: %v", err)
	}

	// 2. FindByID
	fetched, err := repo.FindByID(ctx, replayID)
	if err != nil {
		t.Fatalf("FindByID failed: %v", err)
	}
	if fetched.ID != replayID || fetched.CombatType != replay.CombatTypePvP || len(fetched.TurnLogs) != 1 {
		t.Errorf("unexpected fetched replay: %#v", fetched)
	}

	// 3. FindByCharacter
	headers, err := repo.FindByCharacter(ctx, "charA", "", 10)
	if err != nil || len(headers) == 0 {
		t.Fatalf("FindByCharacter failed: %v, len=%d", err, len(headers))
	}
	if headers[0].ID != replayID || headers[0].InitiatorName != "Hero A" {
		t.Errorf("unexpected header: %#v", headers[0])
	}

	// 4. FindRecent
	recent, err := repo.FindRecent(ctx, replay.CombatTypePvP, 10)
	if err != nil || len(recent) != 0 {
		t.Fatalf("FindRecent failed: %v, len=%d", err, len(recent))
	}

	// 5. DeleteOlderThan
	deleted, err := repo.DeleteOlderThan(ctx, now.Add(time.Minute))
	if err != nil || deleted == 0 {
		t.Errorf("expected at least 1 deleted replay, got %d (err: %v)", deleted, err)
	}
}

func TestReplayRepository_ArchiveVisibilityAndCursorPredicates(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	db.SetMaxOpenConns(1)
	var table, ddl string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE battle_replays").Scan(&table, &ddl); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, strings.Replace(ddl, "CREATE TABLE", "CREATE TEMPORARY TABLE", 1)); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewReplayRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	var visible []string
	for i, suffix := range []string{"1", "2", "3", "4", "5", "6-other", "7-unknown", "8-global", "9-second", "10-case"} {
		rep := replay.BattleReplay{
			ID:                  "vis_" + suffix,
			Provenance:          replay.ProvenancePersonal,
			ArchiveRecipientID:  "char-owner",
			CombatType:          replay.CombatTypeAdventure,
			InitiatorID:         "char-owner",
			InitiatorName:       "Owner",
			OpponentID:          "char-other",
			OpponentName:        "Other",
			Outcome:             corebattle.OutcomeWin,
			InitialParticipants: []replay.ParticipantSnapshot{{ID: "char-owner", Attack: 50}},
			TurnLogs:            []corebattle.TurnLog{{Turn: 1, Message: "raw turn"}},
			CreatedAt:           now,
		}
		switch i {
		case 5:
			rep.ArchiveRecipientID = "char-other"
		case 6:
			rep.Provenance = ""
		case 7:
			rep.Provenance = "global"
		case 8:
			rep.ArchiveRecipientID = "char-second"
		case 9:
			rep.Provenance = "Personal_Archive"
		default:
			visible = append([]string{rep.ID}, visible...)
		}
		if err := repo.Save(ctx, rep); err != nil {
			t.Fatal(err)
		}
	}

	// An old writer omitting archive columns preserves default empty provenance/recipient.
	const migratedID = "vis_migrated"
	if _, err := db.ExecContext(ctx, `INSERT INTO battle_replays
		(id, combat_type, initiator_id, initiator_name, opponent_id, opponent_name, outcome, initial_participants_json, turn_logs_json, created_at)
		VALUES (?, 'adventure', 'char-owner', 'Owner', 'char-other', 'Other', 'win', '[]', '[]', ?)`, migratedID, now); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.FindByID(ctx, migratedID)
	if err != nil || stored.Provenance != "" || stored.ArchiveRecipientID != "" {
		t.Fatalf("historical migration defaults mismatch: %+v %v", stored, err)
	}

	// FindPersonalRecipient validates provenance without decoding JSON payloads.
	if _, err := db.ExecContext(ctx, `UPDATE battle_replays SET initial_participants_json = '"invalid snapshot"' WHERE id = 'vis_6-other'`); err != nil {
		t.Fatal(err)
	}
	recipient, err := repo.FindPersonalRecipient(ctx, "vis_6-other")
	if err != nil || recipient != "char-other" {
		t.Fatalf("FindPersonalRecipient on malformed payload: recipient=%q err=%v", recipient, err)
	}
	for _, hiddenID := range []string{"vis_7-unknown", "vis_8-global", "vis_10-case", migratedID, "vis_absent"} {
		if _, err := repo.FindPersonalRecipient(ctx, hiddenID); !errors.Is(err, replay.ErrReplayNotFound) {
			t.Fatalf("FindPersonalRecipient(%q): got err=%v want ErrReplayNotFound", hiddenID, err)
		}
	}

	for _, filter := range []string{"", replay.CombatTypeAdventure, replay.CombatTypePvP} {
		want := visible
		if filter == replay.CombatTypePvP {
			want = nil
		}
		headers, err := repo.FindByCharacter(ctx, "char-owner", filter, 20)
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, h := range headers {
			got = append(got, h.ID)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("FindByCharacter(filter=%q): got %v want %v", filter, got, want)
		}

		for _, boundary := range []struct {
			at time.Time
			id string
		}{{}, {now, visible[1]}, {now.Add(time.Second), ""}, {time.Time{}, visible[1]}} {
			rows, err := repo.FindByCharacterByCursor(ctx, "char-owner", filter, 20, boundary.at, boundary.id)
			if err != nil {
				t.Fatal(err)
			}
			for _, h := range rows {
				allowed := false
				for _, id := range want {
					if h.ID == id {
						allowed = true
						break
					}
				}
				if !allowed {
					t.Fatalf("cursor query leaked non-owned/hidden header: %s (filter=%q boundary=%+v)", h.ID, filter, boundary)
				}
			}
		}
	}

	for _, filter := range []string{"", replay.CombatTypeAdventure} {
		rows, err := repo.FindRecent(ctx, filter, 1)
		if err != nil || len(rows) != 0 {
			t.Fatalf("FindRecent(filter=%q) leaked rows: %+v %v", filter, rows, err)
		}
		for _, boundary := range []struct {
			at time.Time
			id string
		}{{}, {now, visible[0]}, {now, ""}, {time.Time{}, visible[0]}} {
			rows, err := repo.FindRecentByCursor(ctx, filter, 1, boundary.at, boundary.id)
			if err != nil || len(rows) != 0 {
				t.Fatalf("FindRecentByCursor(filter=%q, boundary=%+v) leaked rows: %+v %v", filter, boundary, rows, err)
			}
		}
	}
}
