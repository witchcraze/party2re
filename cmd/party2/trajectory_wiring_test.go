package main

import (
	"context"
	"os"
	"slices"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
)

// The production service groups write their milestones to the one trajectory
// store, in the transaction that still shows the state each entry describes.
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
	created.Money, created.GodVisit = 100000, corecharacter.GodVisitHeaven
	if err := core.charRepo.Update(ctx, created); err != nil {
		t.Fatal(err)
	}
	g, _, _, err := soc.guild.Create(ctx, created.ID, guildName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := misc.god.GrantWish(ctx, created.ID, god.WishMoney); err != nil {
		t.Fatal(err)
	}
	if err := soc.guild.Disband(ctx, g.ID, created.ID); err != nil {
		t.Fatal(err)
	}

	memories, err := core.charService.Trajectory(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, m := range slices.Backward(memories) {
		got = append(got, string(m.Kind)+"@"+m.GuildName)
	}
	// Founding and disbanding both show the guild; birth precedes it.
	want := []string{"birth@", "guild_found@" + guildName, "wish@" + guildName, "guild_disband@" + guildName}
	if !slices.Equal(got, want) {
		t.Fatalf("trajectory = %q, want %q", got, want)
	}
}
