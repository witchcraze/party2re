package town

import (
	"testing"
)

func TestTownsMetadata(t *testing.T) {
	towns := AllTowns()
	if len(towns) != 4 {
		t.Fatalf("expected 4 towns, got %d", len(towns))
	}

	t1, ok := GetTown("town1")
	if !ok || t1.Name != "メケメケ村" || t1.Price != 500 || t1.CycleDays != 5 || t1.MaxHouses != 10 {
		t.Errorf("unexpected town1: %+v", t1)
	}

	t2, ok := GetTown("town2")
	if !ok || t2.Name != "キノコ町" || t2.Price != 1500 || t2.CycleDays != 10 || t2.MaxHouses != 10 {
		t.Errorf("unexpected town2: %+v", t2)
	}

	t3, ok := GetTown("town3")
	if !ok || t3.Name != "スライム町" || t3.Price != 3000 || t3.CycleDays != 15 || t3.MaxHouses != 10 {
		t.Errorf("unexpected town3: %+v", t3)
	}

	t4, ok := GetTown("town4")
	if !ok || t4.Name != "ガイア国" || t4.Price != 5000 || t4.CycleDays != 20 || t4.MaxHouses != 10 {
		t.Errorf("unexpected town4: %+v", t4)
	}

	if _, ok := GetTown("unknown"); ok {
		t.Errorf("expected GetTown(unknown) to return false")
	}
}

func TestIsValidHouseStyle(t *testing.T) {
	if !IsValidHouseStyle("town1", "001") {
		t.Errorf("expected 001 to be valid for town1")
	}
	if !IsValidHouseStyle("town1", "004") {
		t.Errorf("expected 004 to be valid for town1")
	}
	if IsValidHouseStyle("town1", "005") {
		t.Errorf("expected 005 to be invalid for town1")
	}
	if !IsValidHouseStyle("town4", "028") {
		t.Errorf("expected 028 to be valid for town4")
	}
	if IsValidHouseStyle("town99", "001") {
		t.Errorf("expected unknown town to be invalid")
	}
}
