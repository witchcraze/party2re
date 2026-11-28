package validation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/validation"
)

func TestStripLegacyText(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"unchanged ascii", "Hello, World!", "Hello, World!"},
		{"deletes CR and LF", "a\r\nb\nc\rd", "abcd"},
		{"deletes combining diacritics U+0300-U+036F", "e\u0300\u0301\u036f", "e"},
		{"deletes Thai marks U+0E34-U+0E3A and U+0E47-U+0E4E", "ก\u0E34\u0E3A\u0E47\u0E4Eข", "กข"},
		{"preserves precomposed accent and dakuten", "éか\u3099", "éか\u3099"},
		{"preserves tab and other controls", "a\tb\u20D0", "a\tb\u20D0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := validation.StripLegacyText(tc.input); got != tc.want {
				t.Errorf("StripLegacyText(%q) = %q; want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "trims whitespace",
			input:    "   hello world   ",
			expected: "hello world",
		},
		{
			name:     "nfc normalization for decomposed e and acute accent",
			input:    "e\u0301",
			expected: "é",
		},
		{
			name:     "nfc normalization for decomposed japanese dakuten",
			input:    "か\u3099",
			expected: "が",
		},
		{
			name:     "empty string",
			input:    "   \t\n  ",
			expected: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := validation.SanitizeText(tc.input)
			if got != tc.expected {
				t.Errorf("SanitizeText(%q) = %q; want %q", tc.input, got, tc.expected)
			}
		})
	}
}

func TestValidateSingleLine(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		maxRunes   int
		wantErr    error
		wantOutput string
	}{
		{
			name:       "valid ascii string",
			input:      "  ValidName123  ",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "ValidName123",
		},
		{
			name:       "valid unicode string",
			input:      "  勇者アリス  ",
			maxRunes:   10,
			wantErr:    nil,
			wantOutput: "勇者アリス",
		},
		{
			name:       "empty string",
			input:      "   ",
			maxRunes:   10,
			wantErr:    validation.ErrEmpty,
			wantOutput: "",
		},
		{
			name:       "exceeds max runes",
			input:      "123456",
			maxRunes:   5,
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "strips newline in single line as decode_pt does",
			input:      "Line1\nLine2",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "Line1Line2",
		},
		{
			name:       "strips carriage return in single line as decode_pt does",
			input:      "Line1\rLine2",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "Line1Line2",
		},
		{
			name:       "contains tab control character",
			input:      "Line1\tLine2",
			maxRunes:   20,
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "contains null byte",
			input:      "Hello\x00World",
			maxRunes:   20,
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "contains zero-width space",
			input:      "Invisible\u200BSpace",
			maxRunes:   20,
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains zero-width non-joiner",
			input:      "Invisible\u200CChar",
			maxRunes:   20,
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains byte order mark BOM",
			input:      "BOM\uFEFFText",
			maxRunes:   20,
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains RLO bidi override",
			input:      "Hello\u202EWorld",
			maxRunes:   20,
			wantErr:    validation.ErrBidiOverride,
			wantOutput: "",
		},
		{
			name:       "contains LRO bidi override",
			input:      "Hello\u202DWorld",
			maxRunes:   20,
			wantErr:    validation.ErrBidiOverride,
			wantOutput: "",
		},
		{
			name:       "contains bidi isolate",
			input:      "Hello\u2066World",
			maxRunes:   20,
			wantErr:    validation.ErrBidiOverride,
			wantOutput: "",
		},
		{
			name:       "strips decode_pt combining marks",
			input:      "Z\u0300\u0301\u0302algo",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "Zalgo",
		},
		{
			name:       "zalgo text with more than 2 consecutive combining marks outside decode_pt",
			input:      "Z\u20D0\u20D1\u20D2algo",
			maxRunes:   20,
			wantErr:    validation.ErrZalgo,
			wantOutput: "",
		},
		{
			name:       "valid combining marks up to 2 outside decode_pt",
			input:      "Z\u20D0\u20D1algo",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "Z\u20D0\u20D1algo",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateSingleLine(tc.input, tc.maxRunes)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateSingleLine(%q, %d) error = %v; want %v", tc.input, tc.maxRunes, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateSingleLine(%q, %d) unexpected error = %v", tc.input, tc.maxRunes, err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateSingleLine(%q, %d) = %q; want %q", tc.input, tc.maxRunes, got, tc.wantOutput)
				}
			}
		})
	}
}

func TestCheckText(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr error
	}{
		{"accepts plain text unchanged", " a,;<>＠ ", nil},
		{"rejects LF", "a\nb", validation.ErrControlCharacter},
		{"rejects CR", "a\rb", validation.ErrControlCharacter},
		{"rejects tab", "a\tb", validation.ErrControlCharacter},
		{"rejects other controls", "a\x07b", validation.ErrControlCharacter},
		{"rejects C1 control", "a\u0085b", validation.ErrControlCharacter},
		{"rejects zero-width", "a\u200bb", validation.ErrZeroWidth},
		{"rejects bidi override", "a\u202eb", validation.ErrBidiOverride},
		{"rejects zalgo", "Z\u20D0\u20D1\u20D2", validation.ErrZalgo},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := validation.CheckText(tc.input); !errors.Is(err, tc.wantErr) {
				t.Fatalf("CheckText(%q) = %v; want %v", tc.input, err, tc.wantErr)
			}
		})
	}
}

func TestValidateCharacterName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    error
		wantOutput string
	}{
		{
			name:       "valid 1-character ascii name",
			input:      "A",
			wantErr:    nil,
			wantOutput: "A",
		},
		{
			name:       "valid 1-character japanese name",
			input:      "あ",
			wantErr:    nil,
			wantOutput: "あ",
		},
		{
			name:       "valid 8-character ascii name",
			input:      "12345678",
			wantErr:    nil,
			wantOutput: "12345678",
		},
		{
			name:       "valid 8-character japanese name",
			input:      "あいうえおかきく",
			wantErr:    nil,
			wantOutput: "あいうえおかきく",
		},
		{
			name:       "9-character ascii name exceeds limit",
			input:      "123456789",
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "9-character japanese name exceeds limit",
			input:      "あいうえおかきくけ",
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "33-character name exceeds limit",
			input:      strings.Repeat("あ", 33),
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "empty name",
			input:      "",
			wantErr:    validation.ErrEmpty,
			wantOutput: "",
		},
		{
			name:       "whitespace only name",
			input:      "   ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "leading whitespace rejected",
			input:      " Merino",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "trailing whitespace rejected",
			input:      "Merino ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "surrounding whitespace rejected",
			input:      "  Merino  ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "leading fullwidth whitespace rejected",
			input:      "　Merino",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "trailing fullwidth whitespace rejected",
			input:      "Merino　",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "contains internal whitespace",
			input:      "A Bob",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "contains fullwidth whitespace",
			input:      "アリ　ボブ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "contains internal tab",
			input:      "A\tB",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "strips internal newline as decode_pt does",
			input:      "A\nB",
			wantErr:    nil,
			wantOutput: "AB",
		},
		{
			name:       "strips decode_pt combining marks before NFC",
			input:      "Ka\u0301",
			wantErr:    nil,
			wantOutput: "Ka",
		},
		{
			name:       "nfc normalization converts decomposed dakuten",
			input:      "か\u3099",
			wantErr:    nil,
			wantOutput: "が",
		},
		{
			name:       "contains prohibited comma",
			input:      "Alice,B",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited at sign",
			input:      "@Alice",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited fullwidth at sign",
			input:      "＠Alice",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited quotes",
			input:      `"Alice"`,
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited single quote",
			input:      "'Alice'",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited semicolon",
			input:      "Alice;",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited ampersand",
			input:      "A&B",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "accepts backslash on creation",
			input:      "A\\B",
			wantErr:    nil,
			wantOutput: "A\\B",
		},
		{
			name:       "accepts slash on creation",
			input:      "A/B",
			wantErr:    nil,
			wantOutput: "A/B",
		},
		{
			name:       "contains prohibited angle brackets",
			input:      "<Alice>",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains zero-width character",
			input:      "A\u200Blice",
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains bidi override",
			input:      "A\u202Elice",
			wantErr:    validation.ErrBidiOverride,
			wantOutput: "",
		},
		{
			name:       "contains zalgo marks outside decode_pt",
			input:      "Z\u20D0\u20D1\u20D2al",
			wantErr:    validation.ErrZalgo,
			wantOutput: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateCharacterName(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateCharacterName(%q) error = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateCharacterName(%q) unexpected error = %v", tc.input, err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateCharacterName(%q) = %q; want %q", tc.input, got, tc.wantOutput)
				}
			}
		})
	}
}

func TestValidateCharacterRename(t *testing.T) {
	if _, err := validation.ValidateCharacterRename("A\\B"); !errors.Is(err, validation.ErrProhibitedCharacter) {
		t.Fatalf("ValidateCharacterRename backslash = %v; want ErrProhibitedCharacter", err)
	}
	if _, err := validation.ValidateCharacterRename("A/B"); !errors.Is(err, validation.ErrProhibitedCharacter) {
		t.Fatalf("ValidateCharacterRename slash = %v; want ErrProhibitedCharacter", err)
	}
	if got, err := validation.ValidateCharacterRename("A\nB\u0301"); err != nil || got != "AB" {
		t.Fatalf("ValidateCharacterRename stripped = %q, %v; want %q, nil", got, err, "AB")
	}
}

func TestValidateStarterJob(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantJob string
		wantErr error
	}{
		{name: "numeric 1", input: "1", wantJob: "job-01", wantErr: nil},
		{name: "numeric 12", input: "12", wantJob: "job-12", wantErr: nil},
		{name: "canonical job-01", input: "job-01", wantJob: "job-01", wantErr: nil},
		{name: "canonical job-12", input: "job-12", wantJob: "job-12", wantErr: nil},
		{name: "numeric 0 disallowed", input: "0", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "numeric 13 disallowed", input: "13", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "job-00 disallowed", input: "job-00", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "job-13 disallowed", input: "job-13", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "fictional starter disallowed", input: "starter", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "empty string disallowed", input: "", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
		{name: "whitespace disallowed", input: "   ", wantJob: "", wantErr: validation.ErrInvalidStarterJob},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateStarterJob(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateStarterJob(%q) error = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateStarterJob(%q) unexpected error = %v", tc.input, err)
				}
				if got != tc.wantJob {
					t.Errorf("ValidateStarterJob(%q) = %q; want %q", tc.input, got, tc.wantJob)
				}
			}
		})
	}
}

func TestValidateGender(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantGender string
		wantErr    error
	}{
		{name: "m", input: "m", wantGender: "m", wantErr: nil},
		{name: "male", input: "male", wantGender: "m", wantErr: nil},
		{name: "japanese male", input: "男", wantGender: "m", wantErr: nil},
		{name: "f", input: "f", wantGender: "f", wantErr: nil},
		{name: "female", input: "female", wantGender: "f", wantErr: nil},
		{name: "japanese female", input: "女", wantGender: "f", wantErr: nil},
		{name: "unspecified disallowed", input: "unspecified", wantGender: "", wantErr: validation.ErrInvalidGender},
		{name: "other disallowed", input: "other", wantGender: "", wantErr: validation.ErrInvalidGender},
		{name: "japanese other disallowed", input: "その他", wantGender: "", wantErr: validation.ErrInvalidGender},
		{name: "empty disallowed", input: "", wantGender: "", wantErr: validation.ErrInvalidGender},
		{name: "whitespace disallowed", input: "   ", wantGender: "", wantErr: validation.ErrInvalidGender},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateGender(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateGender(%q) error = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateGender(%q) unexpected error = %v", tc.input, err)
				}
				if got != tc.wantGender {
					t.Errorf("ValidateGender(%q) = %q; want %q", tc.input, got, tc.wantGender)
				}
			}
		})
	}
}

func TestValidateGuildName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    error
		wantOutput string
	}{
		{
			name:       "valid standard name",
			input:      "Knights",
			wantErr:    nil,
			wantOutput: "Knights",
		},
		{
			name:       "valid japanese name",
			input:      "勇者のギルド",
			wantErr:    nil,
			wantOutput: "勇者のギルド",
		},
		{
			name:       "valid 16-character ascii name",
			input:      strings.Repeat("a", 16),
			wantErr:    nil,
			wantOutput: strings.Repeat("a", 16),
		},
		{
			name:       "valid 16-character japanese name",
			input:      strings.Repeat("あ", 16),
			wantErr:    nil,
			wantOutput: strings.Repeat("あ", 16),
		},
		{
			name:       "strips decode_pt combining marks",
			input:      "Ka\u0301",
			wantErr:    nil,
			wantOutput: "Ka",
		},
		{
			name:       "nfc normalization converts decomposed dakuten",
			input:      "か\u3099",
			wantErr:    nil,
			wantOutput: "が",
		},
		{
			name:       "17-character ascii name exceeds limit",
			input:      strings.Repeat("a", 17),
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "17-character japanese name exceeds limit",
			input:      strings.Repeat("あ", 17),
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "empty name",
			input:      "",
			wantErr:    validation.ErrEmpty,
			wantOutput: "",
		},
		{
			name:       "whitespace only name",
			input:      "   ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "leading whitespace rejected",
			input:      " Knights",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "trailing whitespace rejected",
			input:      "Knights ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "internal ascii whitespace rejected",
			input:      "Alice Bob",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "japanese fullwidth space rejected",
			input:      "アリス　ボブ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "tab whitespace rejected",
			input:      "Alice\tBob",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "strips newline as decode_pt does",
			input:      "Alice\nBob",
			wantErr:    nil,
			wantOutput: "AliceBob",
		},
		{
			name:       "contains prohibited comma",
			input:      "Alice,Bob",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited semicolon",
			input:      "Alice;Bob",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited double quote",
			input:      `"Alice"`,
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited single quote",
			input:      "'Alice'",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited ampersand",
			input:      "Alice&Bob",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited less than",
			input:      "<Alice>",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited greater than",
			input:      "Alice>Bob",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited at sign",
			input:      "@Alice",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited fullwidth at sign",
			input:      "＠Alice",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "accepts backslash",
			input:      `Alice\Bob`,
			wantErr:    nil,
			wantOutput: `Alice\Bob`,
		},
		{
			name:       "accepts slash",
			input:      "Alice/Bob",
			wantErr:    nil,
			wantOutput: "Alice/Bob",
		},
		{
			name:       "contains c0 control character",
			input:      "Alice\x00Bob",
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "contains c1 control character",
			input:      "Alice\u0080Bob",
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "contains zero-width space",
			input:      "Alice\u200BBob",
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains zero-width non-joiner",
			input:      "Alice\u200CBob",
			wantErr:    validation.ErrZeroWidth,
			wantOutput: "",
		},
		{
			name:       "contains bidi override",
			input:      "Alice\u202EBob",
			wantErr:    validation.ErrBidiOverride,
			wantOutput: "",
		},
		{
			name:       "contains zalgo marks outside decode_pt",
			input:      "Z\u20D0\u20D1\u20D2algo",
			wantErr:    validation.ErrZalgo,
			wantOutput: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateGuildName(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateGuildName(%q) error = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateGuildName(%q) unexpected error = %v", tc.input, err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateGuildName(%q) = %q; want %q", tc.input, got, tc.wantOutput)
				}
			}
		})
	}
}

func TestValidateQuestName(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantErr    error
		wantOutput string
	}{
		{name: "valid 50 runes", input: strings.Repeat("あ", 50), wantOutput: strings.Repeat("あ", 50)},
		{name: "51 runes exceeds limit", input: strings.Repeat("あ", 51), wantErr: validation.ErrTooLong},
		{name: "empty name", input: "", wantErr: validation.ErrEmpty},
		{name: "ascii whitespace rejected", input: "solo quest", wantErr: validation.ErrInternalWhitespace},
		{name: "fullwidth whitespace rejected", input: "冒険　隊", wantErr: validation.ErrInternalWhitespace},
		{name: "prohibited at sign rejected", input: "quest@1", wantErr: validation.ErrProhibitedCharacter},
		{name: "prohibited fullwidth at sign rejected", input: "＠冒険", wantErr: validation.ErrProhibitedCharacter},
		{name: "decode_pt marks stripped", input: "Ka\u0301", wantOutput: "Ka"},
		{name: "nfc normalized", input: "か\u3099", wantOutput: "が"},
		{name: "backslash rejected", input: `a\b`, wantErr: validation.ErrProhibitedCharacter},
		{name: "slash rejected", input: "a/b", wantErr: validation.ErrProhibitedCharacter},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateQuestName(tc.input)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateQuestName(%q) error = %v; want %v", tc.input, err, tc.wantErr)
				}
			} else if err != nil || got != tc.wantOutput {
				t.Fatalf("ValidateQuestName(%q) = %q, %v; want %q, nil", tc.input, got, err, tc.wantOutput)
			}
		})
	}
}

func TestValidateLegacyComment(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		max     int
		want    string
		wantErr error
	}{
		{"plain text at the limit", "あいう", 3, "あいう", nil},
		{"plain text over the limit", "あいう", 2, "", validation.ErrTooLong},
		{"ampersand is 5", "&", 5, "&", nil},
		{"ampersand over", "&", 4, "", validation.ErrTooLong},
		{"semicolon is 5", ";", 5, ";", nil},
		{"semicolon over", ";", 4, "", validation.ErrTooLong},
		{"comma is 5", ",", 5, ",", nil},
		{"comma over", ",", 4, "", validation.ErrTooLong},
		{"less-than is 4", "<", 4, "<", nil},
		{"less-than over", "<", 3, "", validation.ErrTooLong},
		{"greater-than is 4", ">", 4, ">", nil},
		{"greater-than over", ">", 3, "", validation.ErrTooLong},
		{"double quote is 6", "\"", 6, "\"", nil},
		{"double quote over", "\"", 5, "", validation.ErrTooLong},
		{"single quote is 1", "'", 1, "'", nil},
		{"line breaks are removed and not counted", "1\r\n2\n3\r4", 4, "1234", nil},
		{"only line breaks is empty", "\r\n\n", 10, "", validation.ErrEmpty},
		{"control character", "a\x00b", 10, "", validation.ErrControlCharacter},
		{"zero width", "a\u200Bb", 10, "", validation.ErrZeroWidth},
		{"bidi override", "a\u202Eb", 10, "", validation.ErrBidiOverride},
		{"decode_pt combining marks are removed", "e\u0301\u0E34\u0E47", 10, "e", nil},
		{"zalgo", "Z\u20D0\u20D1\u20D2", 10, "", validation.ErrZalgo},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validation.ValidateLegacyComment(tt.input, tt.max)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestValidateVerbatimLine(t *testing.T) {
	ok := []string{
		"",
		"  前後の空白　も保持 ",
		`<script>alert("x")</script> & ' ; DROP TABLE x;--`,
		"café à́",                      // decomposed accents, two marks
		strings.Repeat("a", 60),          // ASCII boundary
		strings.Repeat("あ", 60),          // 180 UTF-8 bytes
		strings.Repeat("\U0001F600", 60), // supplementary plane, 240 bytes
	}
	for _, in := range ok {
		if err := validation.ValidateVerbatimLine(in, 60); err != nil {
			t.Errorf("%q: unexpected %v", in, err)
		}
	}
	bad := map[string]error{
		strings.Repeat("a", 61):          validation.ErrTooLong,
		strings.Repeat("あ", 61):          validation.ErrTooLong,
		strings.Repeat("\U0001F600", 61): validation.ErrTooLong,
		"bad\xffutf8":                    validation.ErrControlCharacter,
		"tab\there":                      validation.ErrControlCharacter,
		"line\nbreak":                    validation.ErrControlCharacter,
		"cr\rhere":                       validation.ErrControlCharacter,
		"c1\u0085":                       validation.ErrControlCharacter,
		"ls ":                            validation.ErrControlCharacter,
		"ps ":                            validation.ErrControlCharacter,
		"zw​":                            validation.ErrZeroWidth,
		"bidi‮":                          validation.ErrBidiOverride,
		"zalgo à́̂":                     validation.ErrZalgo,
	}
	for in, want := range bad {
		if err := validation.ValidateVerbatimLine(in, 60); !errors.Is(err, want) {
			t.Errorf("%q: got %v, want %v", in, err, want)
		}
	}
}

func TestValidatePlainLine(t *testing.T) {
	if got, err := validation.ValidatePlainLine(" ようこそ！ ", 10); err != nil || got != "ようこそ！" {
		t.Fatalf("plain line = %q, %v", got, err)
	}
	for _, in := range []string{"a,b", "a;b", `a"b`, "a'b", "a&b", "a<b", "a>b"} {
		if _, err := validation.ValidatePlainLine(in, 10); !errors.Is(err, validation.ErrProhibitedCharacter) {
			t.Fatalf("%q: %v", in, err)
		}
	}
	if _, err := validation.ValidatePlainLine("", 10); !errors.Is(err, validation.ErrEmpty) {
		t.Fatalf("empty: %v", err)
	}
}
