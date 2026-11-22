package battle

import (
	"errors"
	"reflect"
	"testing"
)

func automaticFixture() (LifetimeState, AutomaticRevivalContext) {
	s, c := revivalFixture()
	reset := func(id string) error {
		a := s.actor(id)
		a.ResetEffects(LifetimeReset{Attack: 10, Defense: 20, Agility: 30})
		return nil
	}
	return s, AutomaticRevivalContext{TargetContext: c.TargetContext, Reset: reset}
}

func TestAutomaticRevivalPrecedenceFloorsAndRepeatedDeath(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		tmp                      TemporaryEffect
		abilities, items         []string
		mp                       int
		tension                  float64
		wantHP, wantMP, transfer int
		draws                    []float64
	}{
		{"temporary before pharaoh", TemporaryRevival, []string{"pharaoh"}, []string{"item-193"}, 80, 1, 10, 80, 0, []float64{0}},
		{"temporary upper", TemporaryRevival, nil, nil, 0, 1, 20, 0, 0, []float64{.999}},
		{"pharaoh", "", []string{"pharaoh"}, []string{"item-193"}, 27, 1, 101, 14, 0, nil},
		{"pharaoh strict threshold", "", []string{"pharaoh"}, nil, 25, 1, 100, 25, 50, nil},
		{"shield below8", "", nil, []string{"item-161"}, 40, 7.99, 100, 40, 50, nil},
		{"shield at8", "", nil, []string{"item-161"}, 40, 8, 50, 40, 50, nil},
		{"amulet before curse", "", nil, []string{"item-193", "item-260"}, 40, 1, 50, 40, 50, nil},
		{"curse", "", nil, []string{"item-260"}, 40, 1, 30, 40, 50, nil},
		{"already cursed", "", []string{"cursed"}, []string{"item-260"}, 40, 1, 100, 40, 50, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, c := automaticFixture()
			a := s.actor("self")
			a.Temporary, a.Abilities, a.ItemDefinitionIDs, a.MP, a.Tension = tc.tmp, tc.abilities, tc.items, tc.mp, tc.tension
			a.Attack = 100
			s.actor("ally").Gear = &BattleGearStats{WeaponID: "weapon-37", ArmorID: "armor-38"}
			s.actor("ally").Attack = 0
			rng := &targetRNG{t: t, draws: tc.draws}
			r, err := s.ApplyAutomaticRevival(&c, "self", rng)
			if err != nil || a.HP != tc.wantHP || a.MP != tc.wantMP || s.actor("ally").Attack != tc.transfer {
				t.Fatal(s, r, err)
			}
			rng.done()
			if r.Prevented && (a.Temporary != "" || a.Tension != 1) {
				t.Fatal("missing reset", s)
			}
			if tc.name == "amulet before curse" {
				if a.Status != StatusDofuu || hasItem(a.ItemDefinitionIDs, "item-193") || len(a.ConsumedItems) != 1 {
					t.Fatal(s)
				}
				s = reloadLifetime(t, s)
				c.Reset = func(id string) error { s.actor(id).ResetEffects(LifetimeReset{}); return nil }
				if r, err = s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t}); err != nil || !r.Prevented || s.actor("self").HP != 30 || len(s.actor("self").ConsumedItems) != 1 {
					t.Fatal(s, r, err)
				}
				if r, err = s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t}); err != nil || r.Prevented {
					t.Fatal(s, r, err)
				}
			}
			if tc.name == "curse" && (a.Attack != 310 || a.Defense != 320 || a.Agility != 330 || !hasAbility(a.Abilities, "cursed")) {
				t.Fatal(s)
			}
		})
	}
}

func TestAutomaticRevivalResetBeforeConsumptionAndNoTransferOnEarlyReturn(t *testing.T) {
	s, c := automaticFixture()
	a := s.actor("self")
	a.ItemDefinitionIDs = []string{"item-193"}
	a.Gear.AccessoryID = "item-193"
	c.Reset = func(id string) error {
		if !hasItem(s.actor(id).ItemDefinitionIDs, "item-193") {
			t.Fatal("consumed before reset")
		}
		s.actor(id).ResetEffects(LifetimeReset{Attack: 999, Temporary: TemporaryDefense})
		return nil
	}
	if r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t}); err != nil || !r.Prevented || a.Gear.AccessoryID != "" || a.Attack != 999 || a.Temporary != TemporaryDefense {
		t.Fatal(s, r, err)
	}
	for _, maxHP := range []int{0, 1} {
		s, c := automaticFixture()
		s.actor("self").MaxHP = maxHP
		s.actor("self").Temporary = TemporaryRevival
		if r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t, draws: []float64{0}}); err != nil || !r.Prevented || s.actor("self").HP != 0 {
			t.Fatal(s, r, err)
		}
	}
}

func TestAutomaticRevivalAdmissionAndErrorPropagation(t *testing.T) {
	s, c := automaticFixture()
	s.actor("self").Temporary = TemporaryRevival
	c.Reset = nil
	before := reloadLifetime(t, s)
	if _, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t}); !errors.Is(err, ErrUnsupportedEffect) || !reflect.DeepEqual(before, s) {
		t.Fatal(s, err)
	}
	failure := errors.New("reset failure")
	c.Reset = func(string) error { return failure }
	if _, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t, draws: []float64{0}}); !errors.Is(err, failure) {
		t.Fatal(err)
	}
}

func TestAutomaticRevivalDeathTransferIncludesFallenAndHostileHolders(t *testing.T) {
	s, c := automaticFixture()
	s.actor("self").Attack = 101
	for _, id := range []string{"fallen", "enemy"} {
		s.actor(id).Gear = &BattleGearStats{WeaponID: "weapon-37", ArmorID: "armor-38"}
		s.actor(id).Attack = 970
	}
	if r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t}); err != nil || r.Prevented || s.actor("fallen").Attack != 999 || s.actor("enemy").Attack != 999 {
		t.Fatal(s, r, err)
	}
}

func TestAutomaticRevivalAmuletClearsCombatHandWithoutTouchingUnknownsOrOthers(t *testing.T) {
	t.Run("known hand and gear", func(t *testing.T) {
		s, c := automaticFixture()
		self := s.actor("self")
		self.HeldToolID = new("item-193")
		self.Gear = &BattleGearStats{WeaponID: "weapon-01", ArmorID: "armor-01", AccessoryID: "item-193"}
		self.ItemDefinitionIDs = []string{"weapon-01", "armor-01", "item-193"}
		self.Abilities = []string{"dokuro_amulet"}

		ally := s.actor("ally")
		ally.HeldToolID = new("item-169")
		ally.Gear = &BattleGearStats{WeaponID: "weapon-02", ArmorID: "armor-02", AccessoryID: "item-169"}
		ally.ItemDefinitionIDs = []string{"weapon-02", "armor-02", "item-169"}

		resetSawPreConsumption := false
		c.Reset = func(id string) error {
			a := s.actor(id)
			resetSawPreConsumption = hasItem(a.ItemDefinitionIDs, "item-193") &&
				a.Gear != nil && a.Gear.AccessoryID == "item-193" &&
				a.HeldToolID != nil && *a.HeldToolID == "item-193"
			a.ResetEffects(LifetimeReset{Attack: 10, Defense: 20, Agility: 30})
			return nil
		}

		r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t})
		if err != nil || !r.Prevented || !resetSawPreConsumption {
			t.Fatal(s, r, err, resetSawPreConsumption)
		}
		for _, state := range []LifetimeState{s, reloadLifetime(t, s)} {
			got := state.actor("self")
			if got.HP != 50 || got.Status != StatusDofuu ||
				got.HeldToolID == nil || *got.HeldToolID != "" ||
				got.Gear == nil || got.Gear.AccessoryID != "" ||
				got.Gear.WeaponID != "weapon-01" || got.Gear.ArmorID != "armor-01" ||
				!reflect.DeepEqual(got.ItemDefinitionIDs, []string{"weapon-01", "armor-01"}) ||
				hasAbility(got.Abilities, "dokuro_amulet") ||
				!reflect.DeepEqual(got.ConsumedItems, []ConsumedItem{{ID: "item-193", Quantity: 1}}) {
				t.Fatalf("amulet revival left stale hand or mutated gear: %+v gear=%+v", got, got.Gear)
			}
			other := state.actor("ally")
			if other.HeldToolID == nil || *other.HeldToolID != "item-169" ||
				other.Gear == nil || other.Gear.AccessoryID != "item-169" ||
				!reflect.DeepEqual(other.ItemDefinitionIDs, []string{"weapon-02", "armor-02", "item-169"}) {
				t.Fatalf("unrelated actor mutated: %+v gear=%+v", other, other.Gear)
			}
		}
	})

	t.Run("unknown hand and gear stay unknown", func(t *testing.T) {
		s, c := automaticFixture()
		self := s.actor("self")
		self.HeldToolID = nil
		self.Gear = nil
		self.ItemDefinitionIDs = []string{"weapon-01", "item-193"}
		self.Abilities = []string{"dokuro_amulet"}

		r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t})
		if err != nil || !r.Prevented {
			t.Fatal(s, r, err)
		}
		if self.HeldToolID != nil || self.Gear != nil ||
			!reflect.DeepEqual(self.ItemDefinitionIDs, []string{"weapon-01"}) ||
			hasAbility(self.Abilities, "dokuro_amulet") ||
			!reflect.DeepEqual(self.ConsumedItems, []ConsumedItem{{ID: "item-193", Quantity: 1}}) {
			t.Fatalf("fabricated unknown hand/gear: %+v", self)
		}
	})

	t.Run("unrelated held hand preserved", func(t *testing.T) {
		s, c := automaticFixture()
		self := s.actor("self")
		self.HeldToolID = new("item-169")
		self.Gear = &BattleGearStats{AccessoryID: "item-169"}
		self.ItemDefinitionIDs = []string{"item-169", "item-193"}

		r, err := s.ApplyAutomaticRevival(&c, "self", &targetRNG{t: t})
		if err != nil || !r.Prevented {
			t.Fatal(s, r, err)
		}
		if self.HeldToolID == nil || *self.HeldToolID != "item-169" ||
			self.Gear.AccessoryID != "item-169" ||
			!reflect.DeepEqual(self.ItemDefinitionIDs, []string{"item-169"}) {
			t.Fatalf("cleared unrelated hand: %+v gear=%+v", self, self.Gear)
		}
	})
}
