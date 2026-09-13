package blacksmith

import (
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	ErrNameWhitespace    = errors.New("name contains invalid whitespace")
	ErrNameForbiddenChar = errors.New("name contains forbidden characters (,;\"'&<>)")
	ErrNameAtSymbol      = errors.New("name contains forbidden at symbol (@, ＠)")
	ErrNameTooLong       = errors.New("name cannot exceed 20 characters")
	ErrEmptyName         = errors.New("name cannot be empty")
)

// ValidateCustomName validates custom names for weapons and armors according to legacy Party2 rules (blacksmith.cgi:60-72, 80-92).
func ValidateCustomName(name string) error {
	if name == "" {
		return ErrEmptyName
	}
	if strings.ContainsAny(name, " \t\n\r") || strings.Contains(name, "　") {
		return ErrNameWhitespace
	}
	if strings.ContainsAny(name, ",;\"'&<>") {
		return ErrNameForbiddenChar
	}
	if strings.ContainsAny(name, "@＠") {
		return ErrNameAtSymbol
	}
	if utf8.RuneCountInString(name) > 20 {
		return ErrNameTooLong
	}
	return nil
}
