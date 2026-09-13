package character

import (
	"errors"
)

const (
	// MaxCrystal is the maximum crystal currency (刻印晶) a character can hold (legacy: 999,999).
	MaxCrystal = 999_999
)

var (
	// ErrInsufficientCrystals indicates the character does not possess enough crystal currency.
	ErrInsufficientCrystals = errors.New("insufficient crystals for seal operation")
)

// AddCrystal adds crystal currency (刻印晶) clamped to MaxCrystal (999,999).
func (c *Character) AddCrystal(amount int) error {
	if c == nil {
		return ErrNotFound
	}
	if amount < 0 {
		return ErrInvalidAmount
	}
	c.Crystal += amount
	if c.Crystal > MaxCrystal {
		c.Crystal = MaxCrystal
	}
	return nil
}

// DeductCrystal subtracts crystal currency (刻印晶), returning ErrInsufficientCrystals if balance is insufficient.
func (c *Character) DeductCrystal(amount int) error {
	if c == nil {
		return ErrNotFound
	}
	if amount < 0 {
		return ErrInvalidAmount
	}
	if c.Crystal < amount {
		return ErrInsufficientCrystals
	}
	c.Crystal -= amount
	return nil
}

// ClearWeaponCustomization resets the weapon seal and custom weapon name.
func (c *Character) ClearWeaponCustomization() {
	if c == nil {
		return
	}
	c.WeaponSeal = 0
	c.WeaponCustomName = ""
}

// ClearArmorCustomization resets the custom armor name.
func (c *Character) ClearArmorCustomization() {
	if c == nil {
		return
	}
	c.ArmorCustomName = ""
}
