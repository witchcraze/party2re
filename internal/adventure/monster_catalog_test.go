package adventure_test

import (
	"testing"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/item"
)

func TestMonsterCreation(t *testing.T) {
	m, err := adventure.NewMonster("m-1", "Slime", 10, 5, 6, 2, 4, 5, 4, []string{"item-001"})
	if err != nil {
		t.Fatalf("NewMonster() error = %v", err)
	}
	if m.ID != "m-1" || m.Name != "Slime" || m.HP != 10 || m.Attack != 6 {
		t.Errorf("NewMonster() = %#v", m)
	}
}

func TestMonsterCreationValidation(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		monsterName string
		hp          int
		attack      int
		defense     int
		agility     int
		expReward   int
		goldReward  int
	}{
		{name: "empty id", id: "", monsterName: "Slime", hp: 10, attack: 1, defense: 1, agility: 1},
		{name: "empty name", id: "m-1", monsterName: "", hp: 10, attack: 1, defense: 1, agility: 1},
		{name: "zero hp", id: "m-1", monsterName: "Slime", hp: 0, attack: 1, defense: 1, agility: 1},
		{name: "negative hp", id: "m-1", monsterName: "Slime", hp: -5, attack: 1, defense: 1, agility: 1},
		{name: "negative attack", id: "m-1", monsterName: "Slime", hp: 10, attack: -1, defense: 1, agility: 1},
		{name: "negative defense", id: "m-1", monsterName: "Slime", hp: 10, attack: 1, defense: -1, agility: 1},
		{name: "negative agility", id: "m-1", monsterName: "Slime", hp: 10, attack: 1, defense: 1, agility: -1},
		{name: "negative exp", id: "m-1", monsterName: "Slime", hp: 10, attack: 1, defense: 1, agility: 1, expReward: -1},
		{name: "negative gold", id: "m-1", monsterName: "Slime", hp: 10, attack: 1, defense: 1, agility: 1, goldReward: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := adventure.NewMonster(tt.id, tt.monsterName, tt.hp, 0, tt.attack, tt.defense, tt.agility, tt.expReward, tt.goldReward, nil)
			if err == nil {
				t.Errorf("NewMonster(%s) expected error, got nil", tt.name)
			}
		})
	}
}

func TestMonsterCatalogOperations(t *testing.T) {
	m1, _ := adventure.NewMonster("m-1", "Slime", 10, 5, 6, 2, 4, 5, 4, nil)
	m2, _ := adventure.NewMonster("m-2", "Goblin", 20, 0, 12, 5, 6, 12, 10, nil)

	cat, err := adventure.NewMonsterCatalog([]adventure.Monster{m1, m2})
	if err != nil {
		t.Fatalf("NewMonsterCatalog() error = %v", err)
	}

	found, err := cat.FindByID("m-1")
	if err != nil {
		t.Fatalf("FindByID(m-1) error = %v", err)
	}
	if found.Name != "Slime" {
		t.Errorf("FindByID(m-1).Name = %s, want Slime", found.Name)
	}

	if _, err := cat.FindByID("nonexistent"); err == nil {
		t.Error("FindByID(nonexistent) expected error, got nil")
	}

	monsters := cat.Monsters()
	if len(monsters) != 2 {
		t.Errorf("Monsters() count = %d, want 2", len(monsters))
	}
}

func TestInitialMonsterCatalogValid(t *testing.T) {
	cat, err := adventure.InitialMonsterCatalog()
	if err != nil {
		t.Fatalf("InitialMonsterCatalog() error = %v", err)
	}

	monsters := cat.Monsters()
	if len(monsters) == 0 {
		t.Fatal("InitialMonsterCatalog() returned empty list")
	}

	itemCat, err := item.InitialCatalog()
	if err != nil {
		t.Fatalf("item.InitialCatalog() error = %v", err)
	}

	seenIDs := make(map[string]bool)
	for _, m := range monsters {
		if seenIDs[m.ID] {
			t.Errorf("duplicate monster ID: %s", m.ID)
		}
		seenIDs[m.ID] = true

		if m.ID == "" {
			t.Errorf("monster has empty ID")
		}
		if m.Name == "" {
			t.Errorf("monster %s has empty Name", m.ID)
		}
		if m.HP <= 0 {
			t.Errorf("monster %s has non-positive HP: %d", m.ID, m.HP)
		}
		if m.Attack < 0 || m.Defense < 0 || m.Agility < 0 || m.ExperienceReward < 0 || m.GoldReward < 0 {
			t.Errorf("monster %s has negative stats/rewards: %#v", m.ID, m)
		}

		for _, dropID := range m.DropItemIDs {
			if _, err := itemCat.FindByID(dropID); err != nil {
				t.Errorf("monster %s references unknown drop item %s: %v", m.ID, dropID, err)
			}
		}
	}
}
