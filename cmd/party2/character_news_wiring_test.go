package main

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
)

func TestCharacterJoiningNewsProductionWiring(t *testing.T) {
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
	if err := core.initPlayerAndChar(soc.guildRepo, soc.guild, econ.fleamarketRepo, soc.notification, referrals.DeliverReferral, 0, logging.Nop()); err != nil {
		t.Fatal(err)
	}
	player, err := database.CreateTestPlayer(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	name := "J_" + id.New()[:6]
	title := "冒険者 " + name + " が参加しました"
	t.Cleanup(func() {
		if _, err := db.ExecContext(ctx, "DELETE FROM news_articles WHERE title = ?", title); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM character_depots WHERE character_id IN (SELECT id FROM characters WHERE player_id = ?)", player.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM characters WHERE player_id = ?", player.ID); err != nil {
			t.Error(err)
		}
		if _, err := db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", player.ID); err != nil {
			t.Error(err)
		}
	})
	created, err := core.charService.Create(ctx, player.ID, name)
	if err != nil {
		t.Fatal(err)
	}
	var category, content, author string
	if err := db.QueryRowContext(ctx, "SELECT category, content, author FROM news_articles WHERE title = ?", title).Scan(&category, &content, &author); err != nil {
		t.Fatal(err)
	}
	if category != "character" || author != "System" || !strings.Contains(content, created.ID) {
		t.Fatalf("production news: category=%q author=%q content=%q", category, author, content)
	}
}
