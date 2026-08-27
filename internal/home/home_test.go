package home

import (
	"strings"
	"testing"
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
			content:              strings.Repeat("a", MaxLetterContentLength+1),
			expectErr:            ErrContentTooLong,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateLetter(tt.senderCharacterID, tt.recipientCharacterID, tt.content)
			if tt.expectErr != nil {
				if err != tt.expectErr {
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
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidatePhrase(tt.phrase)
			if tt.expectErr != nil {
				if err != tt.expectErr {
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
