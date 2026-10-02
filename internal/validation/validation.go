package validation

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrEmpty               = errors.New("input must not be empty or whitespace only")
	ErrTooLong             = errors.New("input exceeds maximum allowed length")
	ErrControlCharacter    = errors.New("input contains disallowed control characters")
	ErrZeroWidth           = errors.New("input contains invisible or zero-width formatting characters")
	ErrBidiOverride        = errors.New("input contains bidirectional override characters")
	ErrZalgo               = errors.New("input contains excessive combining diacritical marks")
	ErrProhibitedCharacter = errors.New("input contains prohibited characters")
	ErrInternalWhitespace  = errors.New("input contains disallowed internal whitespace")
	ErrInvalidStarterJob   = errors.New("starter job must be between 1 and 12 (job-01 to job-12)")
	ErrInvalidGender       = errors.New("gender must be 'm' or 'f'")
)

var prohibitedNameChars = regexp.MustCompile(`[,;\"\'&<>\\\/@＠]`)

// SanitizeText returns the Unicode NFC normalized and whitespace-trimmed version of the input.
func SanitizeText(s string) string {
	return norm.NFC.String(strings.TrimSpace(s))
}

// ValidateSingleLine validates that the input string is a valid single-line text within maxRunes.
// It normalizes via NFC, trims whitespace, and rejects control characters, zero-width characters,
// bidi overrides, and excessive combining marks (Zalgo text).
func ValidateSingleLine(input string, maxRunes int) (string, error) {
	sanitized := SanitizeText(input)
	if sanitized == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(sanitized) > maxRunes {
		return "", ErrTooLong
	}

	consecutiveCombining := 0
	for _, r := range sanitized {
		if isC0OrC1Control(r) {
			return "", ErrControlCharacter
		}
		if isZeroWidth(r) {
			return "", ErrZeroWidth
		}
		if isBidiOverride(r) {
			return "", ErrBidiOverride
		}
		if isCombiningMark(r) {
			consecutiveCombining++
			if consecutiveCombining > 2 {
				return "", ErrZalgo
			}
		} else {
			consecutiveCombining = 0
		}
	}

	return sanitized, nil
}

// ValidateMultiLine validates multi-line text input (such as comments or bio fields).
// It permits newlines (\n) and carriage returns (\r), while rejecting other control characters,
// zero-width characters, bidi overrides, and excessive combining marks.
func ValidateMultiLine(input string, maxRunes int) (string, error) {
	sanitized := SanitizeText(input)
	if sanitized == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(sanitized) > maxRunes {
		return "", ErrTooLong
	}

	consecutiveCombining := 0
	for _, r := range sanitized {
		if r == '\n' || r == '\r' {
			consecutiveCombining = 0
			continue
		}
		if isC0OrC1Control(r) {
			return "", ErrControlCharacter
		}
		if isZeroWidth(r) {
			return "", ErrZeroWidth
		}
		if isBidiOverride(r) {
			return "", ErrBidiOverride
		}
		if isCombiningMark(r) {
			consecutiveCombining++
			if consecutiveCombining > 2 {
				return "", ErrZalgo
			}
		} else {
			consecutiveCombining = 0
		}
	}

	return sanitized, nil
}

// ValidateCharacterName validates character names according to legacy Party2 rules and modern safety checks:
// - Max 32 runes
// - No internal spaces or Japanese fullwidth spaces
// - No prohibited symbols ([,;\"\'&<>\\\/@＠])
// - No control characters, zero-width characters, bidi overrides, or Zalgo marks
func ValidateCharacterName(name string) (string, error) {
	sanitized := SanitizeText(name)
	if sanitized == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(sanitized) > 32 {
		return "", ErrTooLong
	}
	if prohibitedNameChars.MatchString(sanitized) {
		return "", ErrProhibitedCharacter
	}

	consecutiveCombining := 0
	for _, r := range sanitized {
		if unicode.IsSpace(r) || r == '\u3000' {
			return "", ErrInternalWhitespace
		}
		if isC0OrC1Control(r) {
			return "", ErrControlCharacter
		}
		if isZeroWidth(r) {
			return "", ErrZeroWidth
		}
		if isBidiOverride(r) {
			return "", ErrBidiOverride
		}
		if isCombiningMark(r) {
			consecutiveCombining++
			if consecutiveCombining > 2 {
				return "", ErrZalgo
			}
		} else {
			consecutiveCombining = 0
		}
	}

	return sanitized, nil
}

// ValidateGuildName validates guild names according to legacy Party2 rules and modern safety checks:
// - Max 32 runes
// - No ASCII whitespace (\s) or Japanese fullwidth spaces (\u3000) (internal or surrounding)
// - No prohibited symbols ([,;\"\'&<>\\\/@＠])
// - No control characters, zero-width characters, bidi overrides, or Zalgo marks
// - Unicode NFC normalization
func ValidateGuildName(name string) (string, error) {
	if name == "" {
		return "", ErrEmpty
	}
	normalized := norm.NFC.String(name)
	if normalized == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(normalized) > 32 {
		return "", ErrTooLong
	}
	if prohibitedNameChars.MatchString(normalized) {
		return "", ErrProhibitedCharacter
	}

	consecutiveCombining := 0
	for _, r := range normalized {
		if unicode.IsSpace(r) || r == '\u3000' {
			return "", ErrInternalWhitespace
		}
		if isC0OrC1Control(r) {
			return "", ErrControlCharacter
		}
		if isZeroWidth(r) {
			return "", ErrZeroWidth
		}
		if isBidiOverride(r) {
			return "", ErrBidiOverride
		}
		if isCombiningMark(r) {
			consecutiveCombining++
			if consecutiveCombining > 2 {
				return "", ErrZalgo
			}
		} else {
			consecutiveCombining = 0
		}
	}

	return normalized, nil
}

// ValidateStarterJob verifies and normalizes starter job ID.
// Accepts numeric 1..12 or canonical job-01..job-12.
func ValidateStarterJob(jobID string) (string, error) {
	trimmed := strings.ToLower(strings.TrimSpace(jobID))
	if trimmed == "" {
		return "", ErrInvalidStarterJob
	}

	// Check if numeric 1..12
	if num, err := strconv.Atoi(trimmed); err == nil {
		if num >= 1 && num <= 12 {
			return fmt.Sprintf("job-%02d", num), nil
		}
		return "", ErrInvalidStarterJob
	}

	// Check if canonical format "job-01" .. "job-12"
	if strings.HasPrefix(trimmed, "job-") {
		numStr := strings.TrimPrefix(trimmed, "job-")
		if num, err := strconv.Atoi(numStr); err == nil {
			if num >= 1 && num <= 12 && len(numStr) == 2 {
				return trimmed, nil
			}
		}
	}

	return "", ErrInvalidStarterJob
}

// ValidateGender verifies and normalizes character gender.
// Rejects fictional values (unspecified, other).
func ValidateGender(gender string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(gender))
	switch normalized {
	case "m", "male", "男":
		return "m", nil
	case "f", "female", "女":
		return "f", nil
	default:
		return "", ErrInvalidGender
	}
}

func isC0OrC1Control(r rune) bool {
	return (r >= 0x00 && r <= 0x1F) || r == 0x7F || (r >= 0x80 && r <= 0x9F)
}

func isZeroWidth(r rune) bool {
	switch r {
	case '\u200B', // Zero-width space
		'\u200C', // Zero-width non-joiner
		'\u200D', // Zero-width joiner
		'\u2060', // Word joiner
		'\uFEFF': // Zero-width no-break space / BOM
		return true
	default:
		return false
	}
}

func isBidiOverride(r rune) bool {
	// LRE, RLE, PDF, LRO, RLO (\u202A - \u202E)
	if r >= '\u202A' && r <= '\u202E' {
		return true
	}
	// LRI, RLI, FSI, PDI (\u2066 - \u2069)
	if r >= '\u2066' && r <= '\u2069' {
		return true
	}
	return false
}

func isCombiningMark(r rune) bool {
	return unicode.Is(unicode.Mn, r) || unicode.Is(unicode.Me, r)
}
