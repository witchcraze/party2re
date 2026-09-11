package store

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	StorePrice         = 50000
	StoreCycleDays     = 90
	MaxTownStores      = 10
	NameChangePrice    = 5000
	MaxInteriorCount   = 5
	InteriorPrice      = 1000
	BaseMaxListings    = 10
	MaxStoreNameLen    = 8
	MaxInteriorNameLen = 8

	SaleTypeGold   = "gold"
	SaleTypeBarter = "barter"
)

var (
	ErrStoreNotFound        = errors.New("store not found")
	ErrAlreadyOwnsStore     = errors.New("character already owns a store")
	ErrTownMaxStoresReached = errors.New("maximum stores reached in this town")
	ErrInvalidTownID        = errors.New("invalid town id")
	ErrInvalidHouseStyle    = errors.New("invalid house style for town")
	ErrInsufficientFunds    = errors.New("insufficient funds")
	ErrMaxListingsReached   = errors.New("maximum store listings reached")
	ErrInvalidPrice         = errors.New("price must be between 1 and 999,999 gold")
	ErrItemNotFound         = errors.New("item not found in depot")
	ErrStoreExpired         = errors.New("store has expired")
	ErrCannotBuyOwnItem     = errors.New("cannot purchase from your own store")
	ErrCannotTradeOwnItem   = errors.New("cannot trade with your own store")
	ErrDepotFull            = errors.New("depot is full")
	ErrListingNotFound      = errors.New("listing not found")
	ErrTradeItemMissing     = errors.New("required barter item not found in depot")
	ErrMaxInteriorsReached  = errors.New("maximum interiors reached")
	ErrInvalidInterior      = errors.New("invalid interior furniture")
	ErrInteriorNotFound     = errors.New("interior not found")
	ErrInvalidStoreName     = errors.New("invalid store name")
	ErrStoreNameTaken       = errors.New("store name already exists")
	ErrInvalidWallpaper     = errors.New("invalid wallpaper")
	ErrInvalidInteriorName  = errors.New("invalid interior name")
	ErrCharacterNotFound    = errors.New("character not found")
)

var ValidFurnitures = map[string]bool{
	"001": true, "002": true, "003": true, "010": true, "011": true,
	"014": true, "015": true, "016": true, "017": true, "018": true,
	"019": true, "020": true, "021": true, "022": true, "023": true,
}

var WallpaperPrices = map[string]int{
	"none":       0,
	"farm":       1000,
	"lot":        1000,
	"sp_change":  1500,
	"exile":      1500,
	"depot":      1500,
	"item":       2000,
	"medal":      2000,
	"bar":        2500,
	"casino":     2500,
	"goods":      2500,
	"armor":      3000,
	"job_change": 3000,
	"park":       3500,
	"auction":    4000,
	"weapon":     5000,
	"event":      5000,
	"stage0":     6000,
	"stage1":     6500,
	"stage2":     7000,
	"stage3":     7500,
	"stage4":     8000,
	"stage5":     8500,
	"stage6":     9000,
	"stage7":     9500,
	"stage8":     10000,
	"stage9":     10500,
}

type Store struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	TownID      string    `json:"town_id"`
	StoreName   string    `json:"store_name"`
	HouseStyle  string    `json:"house_style"`
	Wallpaper   string    `json:"wallpaper"`
	ExpiresAt   time.Time `json:"expires_at"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s Store) IsActive(now time.Time) bool {
	return now.Before(s.ExpiresAt)
}

type Sale struct {
	ID               string    `json:"id"`
	StoreID          string    `json:"store_id"`
	CharacterID      string    `json:"character_id"`
	SlotNumber       int       `json:"slot_number"`
	ItemDefinitionID string    `json:"item_definition_id"`
	ItemName         string    `json:"item_name"`
	Quantity         int       `json:"quantity"`
	EnhancementLevel int       `json:"enhancement_level"`
	SaleType         string    `json:"sale_type"`
	Price            int       `json:"price"`
	WishItemName     string    `json:"wish_item_name"`
	CreatedAt        time.Time `json:"created_at"`
}

type Interior struct {
	ID          string    `json:"id"`
	StoreID     string    `json:"store_id"`
	CharacterID string    `json:"character_id"`
	FurnitureID string    `json:"furniture_id"`
	Name        string    `json:"name"`
	SlotIndex   int       `json:"slot_index"`
	CreatedAt   time.Time `json:"created_at"`
}

// MaxListings calculates the listing capacity according to legacy formula:
// 10 + overStore * 2 (10..20)
func MaxListings(overStore int) int {
	if overStore < 0 {
		overStore = 0
	}
	if overStore > 5 {
		overStore = 5
	}
	return BaseMaxListings + overStore*2
}

// ValidateStoreName validates a custom store name according to legacy store.cgi:kanban rules:
// - Max 8 characters
// - No spaces or empty
// - No forbidden characters: , ; " ' & < > \ / @ ＠
func ValidateStoreName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ErrInvalidStoreName
	}
	if utf8.RuneCountInString(trimmed) > MaxStoreNameLen {
		return ErrInvalidStoreName
	}
	if strings.ContainsAny(trimmed, " ,;\"'&<>/\\@　") || strings.Contains(trimmed, "＠") {
		return ErrInvalidStoreName
	}
	return nil
}

// ValidateInteriorName validates custom interior name according to legacy store.cgi:nazukeru rules:
// - Max 8 characters
// - No forbidden characters: , ; " ' & < > @ ＠ and spaces
func ValidateInteriorName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return ErrInvalidInteriorName
	}
	if utf8.RuneCountInString(trimmed) > MaxInteriorNameLen {
		return ErrInvalidInteriorName
	}
	if strings.ContainsAny(trimmed, " ,;\"'&<>@　") || strings.Contains(trimmed, "＠") {
		return ErrInvalidInteriorName
	}
	return nil
}
