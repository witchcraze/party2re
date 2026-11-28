package validation

import (
	"errors"
	"fmt"
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
	ErrInternalWhitespace  = errors.New("input contains disallowed whitespace")
	ErrWhitespace          = ErrInternalWhitespace
	ErrInvalidStarterJob   = errors.New("starter job must be between 1 and 12 (job-01 to job-12)")
	ErrInvalidGender       = errors.New("gender must be 'm' or 'f'")
)

// Legacy prohibited-character sets. Each field's rule pairs one with its own
// whitespace, length and error checks; features use these instead of their own sets.
const (
	// MarkupChars: , ; " ' & < > (lib/guild.cgi:123, lib/farm.cgi:279, lib/store.cgi:678, lib/blacksmith.cgi:65).
	MarkupChars = ",;\"'&<>"
	// TitleChars adds ASCII @ (lib/guild.cgi:173, lib/photo.cgi:123, lib/join_guild.cgi:247, new_entry.cgi:111).
	TitleChars = MarkupChars + "@"
	// NameChars adds \ and / (lib/quest.cgi:559, lib/casino.cgi:291, lib/store.cgi:300, lib/name_change.cgi:122).
	NameChars = TitleChars + "\\/"
	// FullwidthAt is the ＠ that named fields reject alongside their set.
	FullwidthAt = "＠"
	// TagChars: ; < > (lib/profile.cgi:121, lib/custom_skill.cgi:149,156).
	TagChars = ";<>"
)

// StripLegacyText removes CR, LF and the combining-mark runs deleted by
// legacy decode_pt (party2/lib/system.cgi:708,711-713) before any field rule runs.
func StripLegacyText(s string) string {
	hasRemoved := false
	for _, r := range s {
		if isLegacyStrippedRune(r) {
			hasRemoved = true
			break
		}
	}
	if !hasRemoved {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if !isLegacyStrippedRune(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func isLegacyStrippedRune(r rune) bool {
	return r == '\r' || r == '\n' ||
		(r >= 0x0300 && r <= 0x036F) ||
		(r >= 0x0E34 && r <= 0x0E3A) ||
		(r >= 0x0E47 && r <= 0x0E4E)
}

// SanitizeText returns the Unicode NFC normalized and whitespace-trimmed version of the input.
func SanitizeText(s string) string {
	return norm.NFC.String(strings.TrimSpace(s))
}

// ValidateSingleLine validates that the input string is a valid single-line text within maxRunes.
// It strips legacy decode_pt characters, normalizes via NFC, trims whitespace, and rejects control
// characters, zero-width characters, bidi overrides, and excessive combining marks (Zalgo text).
func ValidateSingleLine(input string, maxRunes int) (string, error) {
	sanitized := SanitizeText(StripLegacyText(input))
	if sanitized == "" {
		return "", ErrEmpty
	}
	if utf8.RuneCountInString(sanitized) > maxRunes {
		return "", ErrTooLong
	}

	consecutiveCombining := 0
	for _, r := range sanitized {
		if err := checkSafeRune(r, &consecutiveCombining); err != nil {
			return "", err
		}
	}

	return sanitized, nil
}

// ValidatePlainLine applies ValidateSingleLine and rejects MarkupChars.
func ValidatePlainLine(input string, maxRunes int) (string, error) {
	sanitized, err := ValidateSingleLine(input, maxRunes)
	if err != nil {
		return "", err
	}
	if strings.ContainsAny(sanitized, MarkupChars) {
		return "", ErrProhibitedCharacter
	}
	return sanitized, nil
}

// ValidateLegacyComment validates chat-style body text like the legacy comment field:
// decode_pt removals apply, and maxLength applies to the length after the legacy HTML escaping,
// whatever form the caller stores. It otherwise applies the ValidateSingleLine checks and returns
// the normalized text.
func ValidateLegacyComment(input string, maxLength int) (string, error) {
	sanitized, err := ValidateSingleLine(input, maxLength)
	if err != nil {
		return "", err
	}
	length := 0
	for _, r := range sanitized {
		switch r {
		case '&', ';', ',':
			length += 5
		case '<', '>':
			length += 4
		case '"':
			length += 6
		default:
			length++
		}
	}
	if length > maxLength {
		return "", ErrTooLong
	}
	return sanitized, nil
}

// ValidateVerbatimLine validates text that is stored exactly as given: no trimming,
// normalization or repair, and empty input is allowed. It applies the shared
// safety checks plus malformed UTF-8 and Unicode line/paragraph separators.
func ValidateVerbatimLine(input string, maxRunes int) error {
	if !utf8.ValidString(input) {
		return ErrControlCharacter
	}
	if utf8.RuneCountInString(input) > maxRunes {
		return ErrTooLong
	}
	consecutiveCombining := 0
	for _, r := range input {
		if r == ' ' || r == ' ' {
			return ErrControlCharacter
		}
		if err := checkSafeRune(r, &consecutiveCombining); err != nil {
			return err
		}
	}
	return nil
}

// CheckText rejects control, zero-width, bidi-override and Zalgo characters
// without normalizing the text.
func CheckText(s string) error {
	consecutiveCombining := 0
	for _, r := range s {
		if err := checkSafeRune(r, &consecutiveCombining); err != nil {
			return err
		}
	}
	return nil
}

// MaxQuestNameLen is the legacy $max_title limit for quest/party names (quest.cgi:18).
const MaxQuestNameLen = 50

// validateName applies legacy identifier name constraints (decode_pt stripping, NFC,
// no prohibited symbols, no ASCII/fullwidth whitespace, safety checks, and maxRunes).
func validateName(name string, maxRunes int, prohibited string) (string, error) {
	name = StripLegacyText(name)
	if name == "" {
		return "", ErrEmpty
	}
	normalized := norm.NFC.String(name)
	if normalized == "" {
		return "", ErrEmpty
	}
	if strings.ContainsAny(normalized, prohibited) {
		return "", ErrProhibitedCharacter
	}

	consecutiveCombining := 0
	for _, r := range normalized {
		if unicode.IsSpace(r) || r == '\u3000' {
			return "", ErrInternalWhitespace
		}
		if err := checkSafeRune(r, &consecutiveCombining); err != nil {
			return "", err
		}
	}

	if utf8.RuneCountInString(normalized) > maxRunes {
		return "", ErrTooLong
	}

	return normalized, nil
}

// ValidateCharacterName validates new character names (and recipient lookups) according to legacy Party2 rules and modern safety checks:
// - Max 8 runes (legacy: new_entry.cgi:114)
// - No whitespace (\s or \u3000) permitted anywhere in the name (surrounding or internal) (legacy: new_entry.cgi:113)
// - No prohibited symbols ([,;\"\'&<>@＠]); \ and / are permitted on creation
// - decode_pt CR/LF and combining-mark stripping, Unicode NFC normalization, and no control/zero-width/bidi/Zalgo characters
func ValidateCharacterName(name string) (string, error) {
	return validateName(name, 8, TitleChars+FullwidthAt)
}

// ValidateCharacterRename validates Naming Hall character renames according to legacy Party2 rules and modern safety checks:
// - Max 8 runes (legacy: name_change.cgi:125)
// - No whitespace (\s or \u3000) permitted anywhere in the name (legacy: name_change.cgi:124)
// - No prohibited symbols ([,;\"\'&<>\\\/@＠]) (legacy: name_change.cgi:122)
// - decode_pt CR/LF and combining-mark stripping, Unicode NFC normalization, and no control/zero-width/bidi/Zalgo characters
func ValidateCharacterRename(name string) (string, error) {
	return validateName(name, 8, NameChars+FullwidthAt)
}

// ValidateGuildName validates guild names according to legacy Party2 rules and modern safety checks:
// - Max 16 runes (legacy: join_guild.cgi:26 $max_guild_name = 16; system.cgi:1723-1729 get_utf_length)
// - No ASCII whitespace (\s) or Japanese fullwidth spaces (\u3000) (internal or surrounding)
// - No prohibited symbols ([,;\"\'&<>@＠]); \ and / are permitted (legacy: join_guild.cgi:247-248)
// - decode_pt CR/LF and combining-mark stripping, Unicode NFC normalization, and no control/zero-width/bidi/Zalgo characters
func ValidateGuildName(name string) (string, error) {
	return validateName(name, 16, TitleChars+FullwidthAt)
}

// ValidateQuestName validates quest/party names according to legacy Party2 rules (quest.cgi:18,554–561)
// and shared safety checks (decode_pt stripping, NFC, max 50 runes, no whitespace, no prohibited symbols).
func ValidateQuestName(name string) (string, error) {
	return validateName(name, MaxQuestNameLen, NameChars+FullwidthAt)
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

// checkSafeRune is the shared safety predicate: it rejects control, invisible,
// bidi-override and excessive combining characters. combining carries the run
// of consecutive combining marks across calls.
func checkSafeRune(r rune, combining *int) error {
	if isC0OrC1Control(r) {
		return ErrControlCharacter
	}
	if isZeroWidth(r) {
		return ErrZeroWidth
	}
	if isBidiOverride(r) {
		return ErrBidiOverride
	}
	if isCombiningMark(r) {
		*combining++
		if *combining > 2 {
			return ErrZalgo
		}
	} else {
		*combining = 0
	}
	return nil
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
