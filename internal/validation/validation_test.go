package validation_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/validation"
)

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
			name:       "contains newline in single line",
			input:      "Line1\nLine2",
			maxRunes:   20,
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "contains carriage return in single line",
			input:      "Line1\rLine2",
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
			name:       "zalgo text with more than 2 consecutive combining marks",
			input:      "Z\u0300\u0301\u0302algo",
			maxRunes:   20,
			wantErr:    validation.ErrZalgo,
			wantOutput: "",
		},
		{
			name:       "valid combining marks up to 2",
			input:      "Z\u0300\u0301algo",
			maxRunes:   20,
			wantErr:    nil,
			wantOutput: "Z\u0300\u0301algo",
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

func TestValidateMultiLine(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		maxRunes   int
		wantErr    error
		wantOutput string
	}{
		{
			name:       "allows newlines",
			input:      "Hello\nWorld\r\nAgain",
			maxRunes:   50,
			wantErr:    nil,
			wantOutput: "Hello\nWorld\r\nAgain",
		},
		{
			name:       "rejects C0 control characters other than newline and cr",
			input:      "Hello\x07World",
			maxRunes:   50,
			wantErr:    validation.ErrControlCharacter,
			wantOutput: "",
		},
		{
			name:       "rejects zalgo text",
			input:      "Hello\nZ\u0300\u0301\u0302algo",
			maxRunes:   50,
			wantErr:    validation.ErrZalgo,
			wantOutput: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := validation.ValidateMultiLine(tc.input, tc.maxRunes)
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("ValidateMultiLine error = %v; want %v", err, tc.wantErr)
				}
			} else {
				if err != nil {
					t.Fatalf("ValidateMultiLine unexpected error = %v", err)
				}
				if got != tc.wantOutput {
					t.Errorf("ValidateMultiLine = %q; want %q", got, tc.wantOutput)
				}
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
			name:       "valid standard name",
			input:      "  Merino  ",
			wantErr:    nil,
			wantOutput: "Merino",
		},
		{
			name:       "valid 32-character name",
			input:      strings.Repeat("あ", 32),
			wantErr:    nil,
			wantOutput: strings.Repeat("あ", 32),
		},
		{
			name:       "33-character name exceeds limit",
			input:      strings.Repeat("あ", 33),
			wantErr:    validation.ErrTooLong,
			wantOutput: "",
		},
		{
			name:       "empty name",
			input:      "   ",
			wantErr:    validation.ErrEmpty,
			wantOutput: "",
		},
		{
			name:       "contains internal whitespace",
			input:      "Alice Bob",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "contains fullwidth whitespace",
			input:      "アリス　ボブ",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
		},
		{
			name:       "contains prohibited comma",
			input:      "Alice,Bob",
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
			name:       "contains prohibited angle brackets",
			input:      "<script>",
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
			name:       "contains zalgo marks",
			input:      "Z\u0300\u0301\u0302algo",
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
			name:       "valid 32-character name",
			input:      strings.Repeat("あ", 32),
			wantErr:    nil,
			wantOutput: strings.Repeat("あ", 32),
		},
		{
			name:       "nfc normalization converts decomposed characters",
			input:      "Ka\u0301",
			wantErr:    nil,
			wantOutput: "Ká",
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
			name:       "newline rejected",
			input:      "Alice\nBob",
			wantErr:    validation.ErrInternalWhitespace,
			wantOutput: "",
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
			name:       "contains prohibited backslash",
			input:      `Alice\Bob`,
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
		},
		{
			name:       "contains prohibited slash",
			input:      "Alice/Bob",
			wantErr:    validation.ErrProhibitedCharacter,
			wantOutput: "",
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
			name:       "contains zalgo marks",
			input:      "Z\u0300\u0301\u0302algo",
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
