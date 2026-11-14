package main

import (
	"context"
	"database/sql"
	"os"
	"reflect"
	"testing"
	"time"

	valkeygo "github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/alchemy"
	"github.com/witchcraze/party2re/internal/chapel"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/costume"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/logging"
	"github.com/witchcraze/party2re/internal/monster"
	"github.com/witchcraze/party2re/internal/valkey"
)

func newHomePetCharacter(t *testing.T) (*sql.DB, corecharacter.Character) {
	t.Helper()
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
	chars, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	players, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	char, err := database.CreateTestCharacter(context.Background(), db, "")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx := context.Background()
		if err := chars.Delete(ctx, char.ID); err != nil {
			t.Error(err)
			return
		}
		if err := players.Delete(ctx, char.PlayerID); err != nil {
			t.Error(err)
		}
	})
	return db, char
}

type hookWiringFixture struct {
	db   *sql.DB
	char corecharacter.Character
	core *coreServices
	econ *econServices
	soc  *socServices
	misc *miscServices
	cmbt *cmbtServices
}

func newHookWiringFixture(t *testing.T, client valkeygo.Client) hookWiringFixture {
	t.Helper()
	db, char := newHomePetCharacter(t)
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
	// Recurring schedules scan shared rounds; their registration has separate tests.
	soc.sched = nil
	misc, err := newMiscServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	cmbt, err := newCmbtServices(db, core, soc, econ, client)
	if err != nil {
		t.Fatal(err)
	}
	econ.initStore(core, soc.guildRepo, soc.timer, client, storeDelivery{econ.depotRepo, soc.home, core.itemCatalog})
	if err := wireHooks(core, cmbt, econ, misc, soc); err != nil {
		t.Fatal(err)
	}
	return hookWiringFixture{db, char, core, econ, soc, misc, cmbt}
}

func TestHomePetProductionWiring(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	ctx := context.Background()
	f.char.GodVisit = corecharacter.GodVisitHeaven
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.god.GrantWish(ctx, f.char.ID, god.WishOrtega); err != nil {
		t.Fatal(err)
	}
	members, err := f.misc.homeMemberRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil || len(members) != 1 {
		t.Fatalf("wish companion = %+v, %v", members, err)
	}
	pet := monster.MonsterInstance{ID: id.New(), CharacterID: f.char.ID, MonsterID: "slime", CustomName: "Pet", Location: monster.LocationHome, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
	if err := f.misc.monsterRepo.Save(ctx, pet); err != nil {
		t.Fatal(err)
	}
	view, err := f.soc.home.GetHomeView(ctx, f.char.ID, f.char.ID, f.char.PlayerID)
	if err != nil {
		t.Fatal(err)
	}
	want := []home.HomePet{
		{ID: pet.ID, MonsterID: pet.MonsterID, CustomName: pet.CustomName},
		{ID: members[0].ID, MonsterID: members[0].Icon, CustomName: members[0].Name},
	}
	if !reflect.DeepEqual(view.ResidentPets, want) {
		t.Fatalf("production pets = %+v, want %+v", view.ResidentPets, want)
	}
	// This port is also installed by newMiscServices, independently of the Home reader.
	if _, err := f.misc.monster.DepositToBox(ctx, f.char.ID, members[0].ID); err != nil {
		t.Fatal(err)
	}
	remaining, err := f.misc.homeMemberRepo.FindByCharacterID(ctx, f.char.ID)
	if err != nil || len(remaining) != 0 {
		t.Fatalf("companion store connection = %+v, %v", remaining, err)
	}
}

func TestHomePetFixtureCleanup(t *testing.T) {
	observer, sentinel := newHomePetCharacter(t)
	ctx := context.Background()
	for _, early := range []bool{false, true} {
		var owned corecharacter.Character
		t.Run(map[bool]string{false: "complete", true: "early return"}[early], func(t *testing.T) {
			db, char := newHomePetCharacter(t)
			owned = char
			members, err := database.NewHomeMemberRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := members.AddMember(ctx, god.HomeMember{ID: id.New(), CharacterID: char.ID, IsNPC: true, Name: "Companion", Icon: "companion", CreatedAt: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
			monsters, err := database.NewMonsterRepository(db)
			if err != nil {
				t.Fatal(err)
			}
			if err := monsters.Save(ctx, monster.MonsterInstance{ID: id.New(), CharacterID: char.ID, MonsterID: "slime", CustomName: "Pet", Location: monster.LocationHome, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
			if early {
				return
			}
			if err := db.PingContext(ctx); err != nil {
				t.Fatal(err)
			}
		})
		for _, target := range []struct{ table, key, value string }{
			{"players", "id", owned.PlayerID}, {"characters", "id", owned.ID},
			{"character_depots", "character_id", owned.ID}, {"home_members", "character_id", owned.ID}, {"character_monsters", "character_id", owned.ID},
		} {
			var count int
			if err := observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+target.table+" WHERE "+target.key+" = ?", target.value).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Errorf("%s: %d owned rows survived cleanup", target.table, count)
			}
		}
		var count int
		if err := observer.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters c JOIN players p ON p.id = c.player_id JOIN character_depots d ON d.character_id = c.id WHERE c.id = ?", sentinel.ID).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Fatal("cleanup changed the unrelated sentinel")
		}
	}
}

func TestHomeWakeProductionHooks(t *testing.T) {
	if os.Getenv("PARTY2_VALKEY_ADDR") == "" {
		t.Skip("PARTY2_VALKEY_ADDR is not configured")
	}
	client, err := valkey.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.Close)
	f := newHookWiringFixture(t, client)
	ctx := context.Background()
	f.char.PendingWake = true
	if err := f.core.charRepo.Update(ctx, f.char); err != nil {
		t.Fatal(err)
	}
	if _, err := f.db.ExecContext(ctx, "INSERT INTO tavern_character_status (character_id, is_full) VALUES (?, TRUE)", f.char.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.misc.chapel.SelectBlessing(ctx, f.char.ID, chapel.BlessingGold); err != nil {
		t.Fatal(err)
	}
	repo, err := database.NewAlchemyRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.SaveSynthesisState(ctx, alchemy.Synthesis{CharacterID: f.char.ID, RecipeID: "recipe-001", State: alchemy.StateOngoing}); err != nil {
		t.Fatal(err)
	}
	costumes := costume.NewValkeyCostumeRepository(client)
	if err := costumes.SaveActiveCostume(ctx, costume.ActiveCostume{CharacterID: f.char.ID, ItemNo: 1, ItemName: "Costume", Icon: "costume", RentedAt: time.Now().UTC(), ExpiresAt: time.Now().Add(time.Hour)}, time.Hour); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := costumes.ClearActiveCostume(ctx, f.char.ID); err != nil {
			t.Error(err)
		}
	})
	if _, err := f.soc.home.Wake(ctx, f.char.ID); err != nil {
		t.Fatal(err)
	}
	status, err := f.misc.tavern.GetStatus(ctx, f.char.ID)
	if err != nil || status.IsFull {
		t.Fatalf("fullness hook = %+v, %v", status, err)
	}
	blessing, err := f.misc.chapel.GetBlessing(ctx, f.char.ID)
	if err != nil || blessing.ActiveBlessing != chapel.BlessingNone {
		t.Fatalf("chapel hook = %+v, %v", blessing, err)
	}
	state, err := f.econ.alchemy.GetStatus(ctx, f.char.ID)
	if err != nil || state.State != alchemy.StateCompleted {
		t.Fatalf("alchemy hook = %+v, %v", state, err)
	}
	active, err := costumes.GetActiveCostume(ctx, f.char.ID)
	if err != nil || active != nil {
		t.Fatalf("costume hook = %+v, %v", active, err)
	}
}

func TestHomeRecipeLearnerProductionWiring(t *testing.T) {
	f := newHookWiringFixture(t, nil)
	ctx := context.Background()
	scroll, err := item.NewInstance("item-127", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, f.db, f.char.ID, []item.Instance{scroll}); err != nil {
		t.Fatal(err)
	}
	if _, err := f.soc.home.UseHomeItem(ctx, f.char.ID, scroll.ID, "inventory"); err != nil {
		t.Fatal(err)
	}
	compendium, err := f.econ.alchemy.GetCompendium(ctx, f.char.ID)
	if err != nil || compendium.LearnedCount != 1 {
		t.Fatalf("recipe connection = %+v, %v", compendium, err)
	}
}
