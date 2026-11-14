package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
)

// Each production milestone hook reaches the shared trajectory store.
func TestTrajectoryProductionWiring(t *testing.T) {
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
	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	referrals := referralDeliveryAdapter{depots: econ.depotRepo, home: soc.home, items: core.itemCatalog}
	if err := core.initPlayerAndChar(soc.guildRepo, soc.guild, econ.fleamarketRepo, soc.notification, soc.exileRepo, referrals.DeliverReferral, 0, logging.Nop()); err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	name, guildName := "T_"+id.New()[:6], "TG"+id.New()[:6]
	t.Cleanup(func() {
		for _, q := range []struct {
			query string
			args  []any
		}{
			{"DELETE FROM news_articles WHERE title IN (?, ?)", []any{"冒険者 " + name + " が参加しました", "ギルド『" + guildName + "』が解散しました"}},
			{"DELETE FROM character_job_history WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", []any{player.ID}},
			{"DELETE FROM character_jobs WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", []any{player.ID}},
			{"DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", []any{player.ID}},
			{"DELETE FROM characters WHERE player_id = ?", []any{player.ID}},
			{"DELETE FROM players WHERE id = ?", []any{player.ID}},
		} {
			if _, err := db.ExecContext(ctx, q.query, q.args...); err != nil {
				t.Error(err)
			}
		}
	})

	created, err := core.charService.Create(ctx, player.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	assertTrajectoryKind(t, db, created.ID, corecharacter.MemoryBirth)
	created.Money, created.GodVisit = 100000, corecharacter.GodVisitHeaven
	if err := core.charRepo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	g, _, _, err := soc.guild.Create(ctx, created.ID, guildName)
	if err != nil {
		t.Fatal(err)
	}
	assertTrajectoryKind(t, db, created.ID, corecharacter.MemoryGuildFound)
	if _, err := misc.god.GrantWish(ctx, created.ID, god.WishMoney); err != nil {
		t.Fatal(err)
	}
	assertTrajectoryKind(t, db, created.ID, corecharacter.MemoryWish)
	if err := soc.guild.Disband(ctx, g.ID, created.ID); err != nil {
		t.Fatal(err)
	}

	assertTrajectoryKind(t, db, created.ID, corecharacter.MemoryGuildDisband)
	created, err = core.charRepo.FindByID(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	created.Level = 20
	if err := core.charRepo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	if _, _, err := misc.job.ChangeJob(ctx, created.ID, "job-02"); err != nil {
		t.Fatal(err)
	}
	assertTrajectoryKind(t, db, created.ID, corecharacter.MemoryJobChange)
}

func TestContestTrajectoryProductionWiring(t *testing.T) {
	// Settlement selects rounds globally, so it needs a dedicated database.
	db := wiringIsolatedDB(t, []string{
		"players", "characters", "character_depots", "guilds", "guild_members", "character_memories",
		"character_photos", "contest_rounds", "contest_entries", "contest_votes", "contest_legends", "news_articles",
	})
	ctx := context.Background()
	core, err := newCoreServices(db, nil)
	if err != nil {
		t.Fatal(err)
	}
	econ, err := newEconServices(db, core)
	if err != nil {
		t.Fatal(err)
	}
	soc, err := newSocServices(db, core, econ, nil, logging.Nop())
	if err != nil {
		t.Fatal(err)
	}
	misc, err := newMiscServices(db, core, soc, econ, nil)
	if err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewContestRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if err := repo.SaveRound(ctx, contest.ContestRound{
		Round: 1, Status: contest.StatusActive, StartTime: now.Add(-time.Hour), EndTime: now,
	}); err != nil {
		t.Fatal(err)
	}
	var winnerID string
	for i := 0; i < contest.MinEntriesForContest; i++ {
		char, err := database.CreateTestCharacter(ctx, db, "CT"+id.New()[:6])
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			winnerID = char.ID
		}
		if err := repo.SaveEntry(ctx, contest.ContestEntry{
			Round: 1, CharacterID: char.ID, CharacterName: char.Name,
			Title: fmt.Sprintf("entry-%d", i), Votes: contest.MinEntriesForContest - i,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := misc.contest.SettleContest(ctx, true); err != nil {
		t.Fatal(err)
	}
	assertTrajectoryKind(t, db, winnerID, corecharacter.MemoryContestPrize)
}

func assertTrajectoryKind(t *testing.T, db *sql.DB, characterID string, kind corecharacter.MemoryKind) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(context.Background(),
		"SELECT COUNT(*) FROM character_memories WHERE character_id = ? AND kind = ?", characterID, string(kind),
	).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("character %s: %s trajectory entries = %d, want 1", characterID, kind, count)
	}
}
