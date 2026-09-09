package town

import (
	"errors"
	"strings"
)

var (
	ErrTownNotFound      = errors.New("town not found")
	ErrInvalidHouseStyle = errors.New("invalid house style for town")
)

// Town represents configuration and metadata for a game town.
type Town struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Price       int      `json:"price"`
	CycleDays   int      `json:"cycle_days"`
	MaxHouses   int      `json:"max_houses"`
	HouseStyles []string `json:"house_styles"`
}

var towns = map[string]Town{
	"town1": {
		ID:        "town1",
		Name:      "メケメケ村",
		Price:     500,
		CycleDays: 5,
		MaxHouses: 10,
		HouseStyles: []string{
			"001", "002", "003", "004",
		},
	},
	"town2": {
		ID:        "town2",
		Name:      "キノコ町",
		Price:     1500,
		CycleDays: 10,
		MaxHouses: 10,
		HouseStyles: []string{
			"005", "006", "007", "008", "009", "010", "011", "012",
		},
	},
	"town3": {
		ID:        "town3",
		Name:      "スライム町",
		Price:     3000,
		CycleDays: 15,
		MaxHouses: 10,
		HouseStyles: []string{
			"013", "014", "015", "016", "017", "018", "019", "020",
		},
	},
	"town4": {
		ID:        "town4",
		Name:      "ガイア国",
		Price:     5000,
		CycleDays: 20,
		MaxHouses: 10,
		HouseStyles: []string{
			"021", "022", "023", "024", "025", "026", "027", "028",
		},
	},
}

// GetTown returns the Town metadata for the specified ID.
func GetTown(id string) (Town, bool) {
	t, ok := towns[strings.TrimSpace(id)]
	return t, ok
}

// AllTowns returns a list of all configured towns in standard order.
func AllTowns() []Town {
	return []Town{
		towns["town1"],
		towns["town2"],
		towns["town3"],
		towns["town4"],
	}
}

// IsValidHouseStyle verifies if the given houseStyle is permitted in townID.
func IsValidHouseStyle(townID, houseStyle string) bool {
	t, ok := GetTown(townID)
	if !ok {
		return false
	}
	cleanStyle := strings.TrimSpace(houseStyle)
	for _, style := range t.HouseStyles {
		if style == cleanStyle {
			return true
		}
	}
	return false
}
