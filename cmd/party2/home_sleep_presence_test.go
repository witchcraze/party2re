package main

import (
	"context"
	"testing"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/logging"
)

// TestHomeSleepUsesProductionPresenceCount drives Sleep through newSocServices with
// real presence publications; observation prunes table-wide, so the DB is dedicated.
func TestHomeSleepUsesProductionPresenceCount(t *testing.T) {
	db := database.CreateTestDatabase(t, "players", "characters", "character_depots", "depot_items", "guilds", "guild_members",
		"character_profiles", "character_presence_snapshots", "character_jobs", "character_job_masteries", "inventory_items", "equipment_slots")
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
	newChar := func() corecharacter.Character {
		c, err := database.CreateTestCharacter(ctx, db, "")
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	enter := func(c corecharacter.Character) {
		if err := core.presence.Enter(ctx, c.PlayerID, c.ID, nil); err != nil {
			t.Fatal(err)
		}
	}
	addVisible := func(n int) {
		for range n {
			enter(newChar())
		}
	}
	sleep := func(c corecharacter.Character, want int) {
		t.Helper()
		res, err := soc.home.Sleep(ctx, c.ID, "")
		if err != nil || res.DurationSeconds != want {
			t.Fatalf("Sleep = %+v, %v; want %ds", res, err, want)
		}
	}

	addVisible(19)
	sleep(newChar(), 300) // the unpublished actor is not added to 19 others

	actor := newChar()
	enter(actor)
	enter(actor) // repeated entries do not multiply one character
	sleep(actor, 600)

	addVisible(9) // 29 visible: the actor and 28 others
	sleep(newChar(), 600)
	addVisible(1)
	sleep(newChar(), 900)
}
