package character_test

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreequipment "github.com/witchcraze/party2re/internal/core/equipment"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
)

type inspectFakes struct {
	chars   map[string]corecharacter.Character
	equips  map[string]coreequipment.Equipment
	invs    map[string]coreinventory.Inventory
	charErr error
	eqErr   error
	invErr  error
}

func (f *inspectFakes) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if f.charErr != nil {
		return corecharacter.Character{}, f.charErr
	}
	if c, ok := f.chars[id]; ok {
		return c, nil
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

func (f *inspectFakes) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	for _, c := range f.chars {
		if c.Name == name {
			return c, nil
		}
	}
	return corecharacter.Character{}, corecharacter.ErrNotFound
}

type inspectEquipment struct{ *inspectFakes }

func (f inspectEquipment) FindByCharacterID(_ context.Context, id string) (coreequipment.Equipment, error) {
	if f.eqErr != nil {
		return coreequipment.Equipment{}, f.eqErr
	}
	if e, ok := f.equips[id]; ok {
		return e, nil
	}
	return coreequipment.New(id)
}

type inspectInventory struct{ *inspectFakes }

func (f inspectInventory) FindByCharacterID(_ context.Context, id string) (coreinventory.Inventory, error) {
	if f.invErr != nil {
		return coreinventory.Inventory{}, f.invErr
	}
	if i, ok := f.invs[id]; ok {
		return i, nil
	}
	return coreinventory.New(id)
}

type inspectItems map[string]string

func (m inspectItems) FindByID(id string) (coreitem.Definition, error) {
	if name, ok := m[id]; ok {
		return coreitem.Definition{ID: id, Name: name}, nil
	}
	return coreitem.Definition{}, errors.New("not found")
}

func newInspector(f *inspectFakes) character.Inspector {
	return character.Inspector{Characters: f, Equipment: inspectEquipment{f}, Inventory: inspectInventory{f},
		Items: inspectItems{"sword-1": "鋼の剣", "armor-1": "鉄の鎧"}}
}

func equipBob(t *testing.T, f *inspectFakes, items []coreitem.Instance, slots map[coreitem.Slot]string) {
	t.Helper()
	inv, _ := coreinventory.New("char-2")
	for _, it := range items {
		if err := inv.Add(it); err != nil {
			t.Fatal(err)
		}
	}
	eq, _ := coreequipment.New("char-2")
	for slot, id := range slots {
		eq.Slots[slot] = id
	}
	f.invs["char-2"], f.equips["char-2"] = inv, eq
}

func TestInspect(t *testing.T) {
	ctx := context.Background()
	f := &inspectFakes{chars: map[string]corecharacter.Character{
		"char-1": {ID: "char-1", Name: "Alice", Money: 1},
		"char-2": {ID: "char-2", Name: "Bob", Gender: "m", Level: 25, JobID: "warrior", SP: 4, OldJobID: "mage", OldSP: 2,
			Tired: 30, Money: 12345, Stats: corecharacter.Stats{MaxHP: 90, MaxMP: 20, Attack: 11, Defense: 12, Agility: 13},
			WeaponCustomName: "Bobの剣"},
	}, equips: map[string]coreequipment.Equipment{}, invs: map[string]coreinventory.Inventory{}}
	items := []coreitem.Instance{
		{ID: "armor-bob", DefinitionID: "armor-1", Quantity: 1, EnhancementLevel: 2},
		{ID: "sword-bob", DefinitionID: "sword-1", Quantity: 1, EnhancementLevel: 5},
	}
	equipBob(t, f, items, map[coreitem.Slot]string{coreitem.SlotBody: "armor-bob"})
	svc := newInspector(f)

	// Armor is shown without a weapon, unlike legacy's "if $p{wea}" copy defect.
	res, err := svc.Inspect(ctx, "char-1", "char-2", "")
	if err != nil {
		t.Fatal(err)
	}
	want := character.InspectResult{
		CharacterID: "char-2", Name: "Bob", Gender: "m", Level: 25, JobID: "warrior", SP: 4, OldJobID: "mage", OldSP: 2,
		Tired: 30, MaxHP: 90, MaxMP: 20, Attack: 11, Defense: 12, Agility: 13,
		Armor: &character.EquippedItemInfo{InstanceID: "armor-bob", DefinitionID: "armor-1", Name: "鉄の鎧", EnhancementLevel: 2},
	}
	if !reflect.DeepEqual(res, want) {
		t.Fatalf("got %+v, want %+v", res, want)
	}

	equipBob(t, f, items, map[coreitem.Slot]string{coreitem.SlotBody: "armor-bob", coreitem.SlotMainHand: "sword-bob"})
	res, err = svc.Inspect(ctx, "char-1", "", "Bob")
	if err != nil || res.CharacterID != "char-2" || res.Weapon == nil || res.Weapon.Name != "鋼の剣" || res.Weapon.CustomName != "Bobの剣" || res.Weapon.EnhancementLevel != 5 {
		t.Fatalf("by name: %+v %v", res, err)
	}

	// Omitted or missing targets fall back to the inspector (legacy shiraberu).
	for _, target := range [][2]string{{"", ""}, {"nonexistent", ""}, {"", "NonExistent"}, {"char-1", ""}} {
		res, err := svc.Inspect(ctx, "char-1", target[0], target[1])
		if err != nil || res.CharacterID != "char-1" || !res.Self {
			t.Fatalf("%v: %+v %v", target, res, err)
		}
	}
}

func TestInspect_PreservesReadFailures(t *testing.T) {
	storage := errors.New("storage down")
	for name, inject := range map[string]func(*inspectFakes){
		"character": func(f *inspectFakes) { f.charErr = storage },
		"equipment": func(f *inspectFakes) { f.eqErr = storage },
		"inventory": func(f *inspectFakes) { f.invErr = storage },
	} {
		t.Run(name, func(t *testing.T) {
			f := &inspectFakes{chars: map[string]corecharacter.Character{"char-2": {ID: "char-2", Name: "Bob"}}}
			inject(f)
			if _, err := newInspector(f).Inspect(context.Background(), "char-1", "char-2", ""); !errors.Is(err, storage) {
				t.Fatalf("got %v, want storage error", err)
			}
		})
	}
}

func TestInspect_AccessoryFallbackName(t *testing.T) {
	f := &inspectFakes{chars: map[string]corecharacter.Character{"char-2": {ID: "char-2", Name: "Bob"}},
		equips: map[string]coreequipment.Equipment{}, invs: map[string]coreinventory.Inventory{}}
	equipBob(t, f, []coreitem.Instance{{ID: "acc-1", DefinitionID: "unknown-ring", Quantity: 1}},
		map[coreitem.Slot]string{coreitem.SlotAccessory: "acc-1"})
	res, err := newInspector(f).Inspect(context.Background(), "char-1", "char-2", "")
	if err != nil || res.Accessory == nil || res.Accessory.Name != "unknown-ring" {
		t.Fatalf("got %+v %v", res.Accessory, err)
	}
}
