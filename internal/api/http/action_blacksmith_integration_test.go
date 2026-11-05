package http

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/blacksmith"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/testutil"
)

type blacksmithGatewayCharacters struct {
	*database.CharacterRepository
	fail bool
}

func (r *blacksmithGatewayCharacters) Update(ctx context.Context, value corecharacter.Character) error {
	if r.fail {
		return errors.New("private character save failure")
	}
	return r.CharacterRepository.Update(ctx, value)
}

type blacksmithDBFixture struct {
	actor   corecharacter.Character
	chars   *blacksmithGatewayCharacters
	inv     *database.InventoryRepository
	equip   *database.EquipmentRepository
	storage *database.BlacksmithRepository
	router  http.Handler
}

func newBlacksmithDBFixture(t *testing.T, ctx context.Context) *blacksmithDBFixture {
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
	inv, err := database.NewInventoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	equip, err := database.NewEquipmentRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	storage, err := database.NewBlacksmithRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	actor, err := database.CreateTestCharacterWithFunds(ctx, db, "Blacksmith Gateway", 0)
	if err != nil {
		t.Fatal(err)
	}
	actor.Crystal = 500
	if err := chars.Update(ctx, actor); err != nil {
		t.Fatal(err)
	}
	catalog, err := item.InitialCatalog()
	if err != nil {
		t.Fatal(err)
	}
	owned, err := inv.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	weapon, err := item.NewInstance("weapon-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := owned.Add(weapon); err != nil {
		t.Fatal(err)
	}
	if err := inv.Save(ctx, owned); err != nil {
		t.Fatal(err)
	}
	equipped, err := equip.FindByCharacterID(ctx, actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	def, err := catalog.FindByID(weapon.DefinitionID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := equipped.Equip(&owned, def, weapon.ID); err != nil {
		t.Fatal(err)
	}
	if err := equip.Save(ctx, equipped); err != nil {
		t.Fatal(err)
	}
	f := &blacksmithDBFixture{actor: actor, chars: &blacksmithGatewayCharacters{CharacterRepository: chars}, inv: inv, equip: equip, storage: storage}
	service, err := blacksmith.NewService(f.chars, inv, catalog, blacksmith.WithEquipmentRepository(equip), blacksmith.WithStorageRepository(storage), blacksmith.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}
	characters, err := character.NewService(chars)
	if err != nil {
		t.Fatal(err)
	}
	pc := playercontext.NewService(chars, emptyActionsReader{}, timer.NewService(nil))
	h, err := NewHandler(expansionDBPlayer{owner: actor.PlayerID}, characters, &struct{ AdventureService }{}, &struct{ ShopService }{}, WithBlacksmith(service), WithPlayerContext(pc))
	if err != nil {
		t.Fatal(err)
	}
	f.router = h.Router()
	return f
}

type blacksmithDBSnapshot struct {
	char   corecharacter.Character
	inv    coreinventory.Inventory
	equip  coreequipment.Equipment
	stored []blacksmith.Deposit
}

func (f *blacksmithDBFixture) snapshot(t *testing.T, ctx context.Context) blacksmithDBSnapshot {
	t.Helper()
	c, err := f.chars.FindByID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	i, err := f.inv.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	e, err := f.equip.FindByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	s, err := f.storage.ListByCharacterID(ctx, f.actor.ID)
	if err != nil {
		t.Fatal(err)
	}
	return blacksmithDBSnapshot{c, i, e, s}
}

func TestBlacksmithGatewayPersistence(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlacksmithDBFixture(t, ctx)
	for _, input := range []struct{ action, params string }{
		{"blacksmith_name", `{"target":"weapon","name":"名剣"}`},
		{"blacksmith_seal", `{"seal_id":2}`},
		{"blacksmith_deposit", `{}`},
		{"blacksmith_withdraw", `{"slot":1}`},
	} {
		status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody(input.action, input.params), ctx)
		if status != 200 || string(got["success"]) != "true" {
			t.Fatalf("%s: %d %s", input.action, status, got)
		}
		state := f.snapshot(t, ctx)
		if state.char.Money != 0 {
			t.Fatal("blacksmith touched wallet gold")
		}
		if input.action == "blacksmith_deposit" {
			if len(state.stored) != 1 || len(state.inv.Items) != 0 || len(state.equip.Slots) != 0 || state.char.WeaponSeal != 0 || state.char.WeaponCustomName != "" || state.stored[0].SealID != 2 || state.stored[0].CustomName != "名剣" {
				t.Fatalf("deposit did not preserve equipment facts: %+v", state)
			}
		}
	}
	state := f.snapshot(t, ctx)
	if state.char.Crystal != 0 || state.char.WeaponSeal != 2 || state.char.WeaponCustomName != "名剣" || len(state.stored) != 0 || len(state.inv.Items) != 1 || state.inv.Items[0].DefinitionID != "weapon-01" || state.equip.Slots[item.SlotMainHand] != state.inv.Items[0].ID {
		t.Fatalf("round trip lost assets: %+v", state)
	}
	status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody("blacksmith_seal", `{"seal_id":1}`), ctx)
	if status != 400 {
		t.Fatalf("crystal validation bypassed: %d %s", status, got)
	}
	assertGatewayError(t, got, "BLACKSMITH_INSUFFICIENT_CRYSTALS")
	if after := f.snapshot(t, ctx); !reflect.DeepEqual(state, after) {
		t.Fatal("rejection mutated assets")
	}
	status, got = gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody("blacksmith_name", `{"target":"weapon","name":""}`), ctx)
	if status != 200 || f.snapshot(t, ctx).char.WeaponCustomName != "" {
		t.Fatalf("explicit name reset failed: %d %s", status, got)
	}
}

func TestBlacksmithGatewayRollback(t *testing.T) {
	for _, input := range blacksmithInputs {
		t.Run(input.action, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			f := newBlacksmithDBFixture(t, ctx)
			params := input.params
			if input.action == "blacksmith_name" {
				params = `{"target":"weapon","name":"名剣"}`
			}
			if input.action == "blacksmith_withdraw" {
				status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody("blacksmith_deposit", `{}`), ctx)
				if status != 200 {
					t.Fatalf("setup deposit: %d %s", status, got)
				}
				params = `{"slot":1}`
			}
			before := f.snapshot(t, ctx)
			f.chars.fail = true
			status, got := gatewayRequest(t, f.router, f.actor.ID, "session", "application/json", saleGatewayBody(input.action, params), ctx)
			if status != 500 {
				t.Fatalf("save failure hidden: %d %s", status, got)
			}
			assertGatewayError(t, got, "EXECUTION_FAILED")
			if _, exists := got["context"]; exists {
				t.Fatal("unknown outcome refreshed")
			}
			if after := f.snapshot(t, ctx); !reflect.DeepEqual(before, after) {
				t.Fatalf("transaction failed to roll back: before=%+v after=%+v", before, after)
			}
		})
	}
}

func TestBlacksmithGatewayConcurrentAssets(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	f := newBlacksmithDBFixture(t, ctx)
	for _, input := range []struct{ action, params string }{
		{"blacksmith_deposit", `{}`}, {"blacksmith_withdraw", `{"slot":1}`}, {"blacksmith_seal", `{"seal_id":2}`},
	} {
		statuses := make([]int, 2)
		request := func(index int) func() error {
			return func() error {
				r := httptest.NewRequest(http.MethodPost, "/api/v1/characters/"+f.actor.ID+"/actions", strings.NewReader(saleGatewayBody(input.action, input.params))).WithContext(ctx)
				r.Header.Set("Authorization", "Bearer session")
				r.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				f.router.ServeHTTP(w, r)
				statuses[index] = w.Code
				if w.Code != 200 && w.Code != 400 {
					return fmt.Errorf("%s: %d %s", input.action, w.Code, w.Body.String())
				}
				return nil
			}
		}
		for _, err := range testutil.RunRace(request(0), request(1)) {
			if err != nil {
				t.Fatal(err)
			}
		}
		if !((statuses[0] == 200 && statuses[1] == 400) || (statuses[0] == 400 && statuses[1] == 200)) {
			t.Fatalf("duplicate mutation: %s %v", input.action, statuses)
		}
		state := f.snapshot(t, ctx)
		if len(state.inv.Items)+len(state.stored) != 1 || state.char.Money != 0 {
			t.Fatalf("asset conservation failed: %+v", state)
		}
		if input.action == "blacksmith_seal" && (state.char.Crystal != 0 || state.char.WeaponSeal != 2) {
			t.Fatalf("crystals not conserved: %+v", state)
		}
	}
}
