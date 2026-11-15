package skill

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/item"
)

// exe_alacarte updates the current hand used by book discovery and _has_need_item
// (ff97fc5:_skill.cgi:136–145,10456–10460; _battle.cgi:1747–1752).
func TestStoredToolExchangeBookIdentity(t *testing.T) {
	for _, mode := range []string{"exchange", "reload", "already holding new book"} {
		t.Run(mode, func(t *testing.T) {
			s, f, d := storedFixture("item-175", corebattle.StoredTool{InstanceID: "next", DefinitionID: "item-175", Category: item.UsageCategoryCombatPassive})
			a := &s.Actors[0]
			old := "item-169"
			if mode == "already holding new book" {
				old = "item-175"
			}
			*a.HeldToolID, *f.Assets.HeldToolID = old, old
			a.MP, a.Defense, a.BaseStats.Defense = 100, 80, 80
			a.Gear.WeaponID, a.Gear.ArmorID, a.Gear.AccessoryID = "weapon-01", "armor-01", old
			a.Gear.WeaponAttack = new(12)
			a.Gear.ArmorDefense, a.Gear.WeaponWeight, a.Gear.ArmorWeight = 7, 3, 2
			a.ItemDefinitionIDs = []string{"weapon-01", "armor-01", old}
			a.Temporary, a.Extended, a.ExtendedTurns = corebattle.TemporaryRegeneration, corebattle.ExtendedIncense, 3
			want := a.Clone()
			*want.HeldToolID, want.Gear.AccessoryID = "item-175", "item-175"
			want.ItemDefinitionIDs[2] = "item-175"
			if mode != "already holding new book" {
				runStored(t, &s, &f, d)
			}
			if mode == "reload" {
				raw, err := json.Marshal(s)
				if err != nil {
					t.Fatal(err)
				}
				s = corebattle.LifetimeState{}
				if err := json.Unmarshal(raw, &s); err != nil {
					t.Fatal(err)
				}
			}
			if !reflect.DeepEqual(s.Actors[0], want) || *f.Assets.HeldToolID != "item-175" {
				t.Fatalf("exchange changed unrelated state or left stale identity: got %+v, want %+v", s.Actors[0], want)
			}
			// Rebuild operation-local registration for discovery and execution.
			book := &corebattle.BookContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: f.TargetContext}}}
			var books BattleDefinitions
			books.RegisterBooks(book)
			if _, err := books.BookAction(&s, book, "book-169", noArithmeticDraws{t}); !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal("old book discoverable", err)
			}
			if _, err := books.BookAction(&s, book, "book-175", noArithmeticDraws{t}); err != nil {
				t.Fatal("new book rejected", err)
			}
			executeBook(t, &s, book, books, "book-175", []string{"self"})
			if s.Actors[0].Defense != 112 || s.Actors[0].HP != 500 || s.Actors[0].MP != 100 {
				t.Fatal("new book body", s.Actors[0])
			}
		})
	}
}

func TestStoredToolExchangePassiveResetAndExtension(t *testing.T) {
	for _, next := range []string{"item-149", "item-163"} {
		t.Run(next, func(t *testing.T) {
			old := "item-163"
			if next == old {
				old = "item-149"
			}
			s, f, d := storedFixture(next, corebattle.StoredTool{InstanceID: "next", DefinitionID: next, Category: item.UsageCategoryCombatPassive})
			a := &s.Actors[0]
			*a.HeldToolID, *f.Assets.HeldToolID = old, old
			a.Gear.AccessoryID, a.ItemDefinitionIDs = old, []string{old}
			a.Tension = 1
			runStored(t, &s, &f, d)
			ext := &corebattle.EquippedExtensionContext{DamageContext: corebattle.DamageContext{EffectContext: corebattle.EffectContext{TargetContext: f.TargetContext}}}
			_, err := s.ValidateEquippedExtension(ext, "extension-163", noArithmeticDraws{t})
			if next == "item-163" && err != nil || next != "item-163" && !errors.Is(err, corebattle.ErrUnsupportedEffect) {
				t.Fatal("extension identity", next, err)
			}
			reset := &corebattle.EquipmentResetContext{TargetContext: f.TargetContext}
			if err := s.ApplyEquipmentReset(reset, "self", noArithmeticDraws{t}); err != nil {
				t.Fatal(err)
			}
			// Passive149 adds 3 hit; passive163 establishes magic reflection.
			if next == "item-149" && (a.Hit != 98 || a.Temporary != "") || next == "item-163" && (a.Hit != 95 || a.Temporary != corebattle.TemporaryReflectMagic) {
				t.Fatal("reset used old accessory", a)
			}
		})
	}
}

func TestJobAssetHandChangesKeepEquippedIdentity(t *testing.T) {
	for _, tc := range []struct {
		name, held, want string
		scavenge, npc    bool
	}{
		{"enhanced consumption", "item-001", "", false, false},
		{"enhanced category2", "item-018", "", false, false},
		{"enhanced retained", "item-006", "item-006", false, false},
		{"purchase", "", "item-001", false, false},
		{"NPC purchase", "", "item-001", false, true},
		{"scavenging", "", "item-001", true, false},
		{"NPC scavenging", "", "item-001", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, f, d := enhancedFixture(tc.held)
			a := &s.Actors[0]
			a.NPC = tc.npc
			a.Gear.WeaponID, a.Gear.ArmorID, a.Gear.AccessoryID = "weapon-01", "armor-01", tc.held
			a.ItemDefinitionIDs = []string{"weapon-01", "armor-01"}
			if tc.held != "" {
				a.ItemDefinitionIDs = append(a.ItemDefinitionIDs, tc.held)
			}
			if tc.npc {
				*f.Assets.HeldToolID = "item-169"
			}
			if tc.scavenge {
				executeJobAsset(t, &s, &f, d, "job-78-entry-5", 0, 0, 0)
			} else {
				runEnhanced(t, &s, &f, d, 0)
			}
			want := []string{"weapon-01", "armor-01"}
			if tc.want != "" {
				want = append(want, tc.want)
			}
			if *a.HeldToolID != tc.want || a.Gear.AccessoryID != tc.want || !reflect.DeepEqual(a.ItemDefinitionIDs, want) {
				t.Fatal("inconsistent hand", *a.HeldToolID, a.Gear, a.ItemDefinitionIDs)
			}
			durable := tc.want
			if tc.npc {
				durable = "item-169"
			}
			if *f.Assets.HeldToolID != durable {
				t.Fatal("durable hand", *f.Assets.HeldToolID, durable)
			}
		})
	}
}
