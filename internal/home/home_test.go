package home

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/validation"
)

func TestValidateLetter(t *testing.T) {
	tests := []struct {
		name                 string
		senderCharacterID    string
		recipientCharacterID string
		content              string
		expectErr            error
	}{
		{
			name:                 "valid letter",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello! Let's adventure together.",
			expectErr:            nil,
		},
		{
			name:                 "valid multiline letter",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello!\nLet's adventure together.\r\nSee you tomorrow!",
			expectErr:            nil,
		},
		{
			name:                 "empty sender",
			senderCharacterID:    "",
			recipientCharacterID: "char-2",
			content:              "Hello!",
			expectErr:            ErrInvalidSender,
		},
		{
			name:                 "empty recipient",
			senderCharacterID:    "char-1",
			recipientCharacterID: "",
			content:              "Hello!",
			expectErr:            ErrInvalidRecipient,
		},
		{
			name:                 "send to self",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-1",
			content:              "Hello self!",
			expectErr:            ErrCannotSendToSelf,
		},
		{
			name:                 "empty content",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "   ",
			expectErr:            ErrEmptyContent,
		},
		{
			name:                 "content too long",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              strings.Repeat("a", DefaultLetterMaxLength+1),
			expectErr:            ErrContentTooLong,
		},
		{
			name:                 "content with disallowed control character",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello\x00World",
			expectErr:            validation.ErrControlCharacter,
		},
		{
			name:                 "content with zero-width character",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello\u200BWorld",
			expectErr:            validation.ErrZeroWidth,
		},
		{
			name:                 "content with bidi override",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello\u202EWorld",
			expectErr:            validation.ErrBidiOverride,
		},
		{
			name:                 "content with zalgo diacritics",
			senderCharacterID:    "char-1",
			recipientCharacterID: "char-2",
			content:              "Hello Z\u0300\u0301\u0302algo",
			expectErr:            validation.ErrZalgo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLetter(tt.senderCharacterID, tt.recipientCharacterID, tt.content, DefaultLetterMaxLength)
			if tt.expectErr != nil {
				if !errors.Is(err, tt.expectErr) {
					t.Fatalf("expected error %v, got %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestValidatePhrase(t *testing.T) {
	tests := []struct {
		name      string
		phrase    string
		expectErr error
	}{
		{
			name:      "valid phrase",
			phrase:    "おかえりなさい！ご主人様！",
			expectErr: nil,
		},
		{
			name:      "empty phrase",
			phrase:    "   ",
			expectErr: ErrEmptyPhrase,
		},
		{
			name:      "phrase too long",
			phrase:    strings.Repeat("a", MaxPhraseLength+1),
			expectErr: ErrPhraseTooLong,
		},
		{
			name:      "phrase with newline disallowed in single line",
			phrase:    "Hello\nWorld",
			expectErr: validation.ErrControlCharacter,
		},
		{
			name:      "phrase with control character",
			phrase:    "Hello\x07World",
			expectErr: validation.ErrControlCharacter,
		},
		{
			name:      "phrase with zero-width character",
			phrase:    "Hello\uFEFFWorld",
			expectErr: validation.ErrZeroWidth,
		},
		{
			name:      "phrase with bidi override",
			phrase:    "Hello\u2066World",
			expectErr: validation.ErrBidiOverride,
		},
		{
			name:      "phrase with zalgo diacritics",
			phrase:    "Z\u0300\u0301\u0302algo",
			expectErr: validation.ErrZalgo,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhrase(tt.phrase)
			if tt.expectErr != nil {
				if !errors.Is(err, tt.expectErr) {
					t.Fatalf("expected error %v, got %v", tt.expectErr, err)
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func TestIsLetterThrottled(t *testing.T) {
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 900_000_000, time.UTC)
	tests := []struct {
		name         string
		latestSender string
		now          time.Time
		want         bool
	}{
		{"same second", "a", t0, true},
		{"29 seconds", "a", t0.Add(29 * time.Second), true},
		{"30 whole seconds with a sub-second difference", "a", t0.Add(29*time.Second + 200*time.Millisecond), false},
		{"30 seconds", "a", t0.Add(30 * time.Second), false},
		{"another latest sender", "b", t0, false},
		{"latest letter in the future", "a", t0.Add(-time.Hour), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsLetterThrottled("a", tt.latestSender, t0, tt.now); got != tt.want {
				t.Fatalf("expected %v, got %v", tt.want, got)
			}
		})
	}
}
