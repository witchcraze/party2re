package blacksmith

import (
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/witchcraze/party2re/internal/validation"
)

var (
	ErrNameWhitespace    = errors.New("name contains invalid whitespace")
	ErrNameForbiddenChar = errors.New("name contains forbidden characters (,;\"'&<>)")
	ErrNameAtSymbol      = errors.New("name contains forbidden full-width at symbol (＠)")
	ErrNameTooLong       = errors.New("name cannot exceed 20 characters")
	ErrEmptyName         = errors.New("name cannot be empty")
)

// ValidateCustomName applies the legacy nazukeru name rules (blacksmith.cgi:60-71, 80-91).
func ValidateCustomName(name string) error {
	// Perl-false "0" renames nothing either; legacy shows the prompt (blacksmith.cgi:59, 79).
	if name == "" || name == "0" {
		return ErrEmptyName
	}
	// Perl \s on the byte string plus the ideographic space.
	if strings.ContainsAny(name, " \t\n\r\f\v　") {
		return ErrNameWhitespace
	}
	if strings.ContainsAny(name, validation.MarkupChars) {
		return ErrNameForbiddenChar
	}
	if strings.Contains(name, validation.FullwidthAt) {
		return ErrNameAtSymbol
	}
	if utf8.RuneCountInString(name) > 20 {
		return ErrNameTooLong
	}
	return nil
}
