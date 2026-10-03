package park_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/park"
	"github.com/witchcraze/party2re/internal/validation"
)

func TestSanitizeContent(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "Plain text",
			input:    "こんにちは、みなさん！",
			expected: "こんにちは、みなさん！",
		},
		{
			name:     "HTML tags escaped",
			input:    "<script>alert('xss')</script>",
			expected: "&lt;script&gt;alert(&#39;xss&#39;)&lt;/script&gt;",
		},
		{
			name:     "Special characters",
			input:    "A & B < C > D \"quotes\"",
			expected: "A &amp; B &lt; C &gt; D &#34;quotes&#34;",
		},
		{
			name:     "Whitespace trimmed",
			input:    "   hello world   ",
			expected: "hello world",
		},
		{
			name:     "NFD to NFC normalized",
			input:    "Ka\u0301llout",
			expected: "Kállout",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			actual := park.SanitizeContent(tc.input)
			if actual != tc.expected {
				t.Fatalf("expected %q, got %q", tc.expected, actual)
			}
		})
	}
}

func TestValidatePost(t *testing.T) {
	t.Run("Valid post", func(t *testing.T) {
		got, err := park.ValidatePost("char123", "Hello", "#000000", "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "Hello" {
			t.Errorf("expected Hello, got %q", got)
		}
	})

	t.Run("Valid multiline post", func(t *testing.T) {
		got, err := park.ValidatePost("char123", "Line 1\nLine 2\r\nLine 3", "#000000", "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		expected := "Line 1\nLine 2\r\nLine 3"
		if got != expected {
			t.Errorf("expected %q, got %q", expected, got)
		}
	})

	t.Run("Normalizes NFD to NFC", func(t *testing.T) {
		// "Ka\u0301llout" (NFD for Kállout)
		got, err := park.ValidatePost("char123", "Ka\u0301llout", "#000000", "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
		if got != "Kállout" {
			t.Errorf("expected NFC Kállout, got %q", got)
		}
	})

	t.Run("Empty character ID", func(t *testing.T) {
		_, err := park.ValidatePost("", "Hello", "#000000", "")
		if !errors.Is(err, park.ErrInvalidCharacterID) {
			t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
		}
	})

	t.Run("Empty content", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "   ", "#000000", "")
		if !errors.Is(err, park.ErrEmptyContent) {
			t.Fatalf("expected ErrEmptyContent, got %v", err)
		}
	})

	t.Run("Content too long (>200 chars)", func(t *testing.T) {
		longContent := strings.Repeat("あ", 201)
		_, err := park.ValidatePost("char123", longContent, "#000000", "")
		if !errors.Is(err, park.ErrContentTooLong) {
			t.Fatalf("expected ErrContentTooLong, got %v", err)
		}
	})

	t.Run("Control characters rejected", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "Hello\x00World", "#000000", "")
		if !errors.Is(err, validation.ErrControlCharacter) {
			t.Fatalf("expected ErrControlCharacter, got %v", err)
		}
	})

	t.Run("Zero-width space rejected", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "Hello\u200BWorld", "#000000", "")
		if !errors.Is(err, validation.ErrZeroWidth) {
			t.Fatalf("expected ErrZeroWidth, got %v", err)
		}
	})

	t.Run("Bidi override rejected", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "Hello\u202EWorld", "#000000", "")
		if !errors.Is(err, validation.ErrBidiOverride) {
			t.Fatalf("expected ErrBidiOverride, got %v", err)
		}
	})

	t.Run("Zalgo text rejected", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "Z\u0300\u0301\u0302algo", "#000000", "")
		if !errors.Is(err, validation.ErrZalgo) {
			t.Fatalf("expected ErrZalgo, got %v", err)
		}
	})

	t.Run("Invalid color format", func(t *testing.T) {
		_, err := park.ValidatePost("char123", "Hello", "invalid-color-123456789012345678901234567890", "")
		if !errors.Is(err, park.ErrInvalidColor) {
			t.Fatalf("expected ErrInvalidColor, got %v", err)
		}
	})
}

func TestTownGirlNPC_Talk(t *testing.T) {
	npc := park.NewTownGirlNPC(random.NewDeterministic(42))
	line := npc.Talk("勇者", "勇者1号")
	if line == "" {
		t.Fatalf("expected non-empty dialogue line")
	}
	if !strings.Contains(line, "勇者1号") && !strings.Contains(line, "勇者") && !strings.Contains(line, "天気") && !strings.Contains(line, "元気") && !strings.Contains(line, "どこ") && !strings.Contains(line, "占い") && !strings.Contains(line, "夕飯") {
		t.Fatalf("unexpected dialogue line: %q", line)
	}
}

func TestTownGirlNPC_Divinate(t *testing.T) {
	npc := park.NewTownGirlNPC(random.NewDeterministic(42))
	result := npc.Divinate("勇者1号")
	if result.Fortune == "" {
		t.Fatalf("expected non-empty fortune")
	}
	if result.LuckyColor == "" {
		t.Fatalf("expected non-empty lucky color")
	}
	if !strings.Contains(result.Message, "勇者1号") || !strings.Contains(result.Message, result.Fortune) || !strings.Contains(result.Message, result.LuckyColor) {
		t.Fatalf("expected message to contain name, fortune, and lucky color, got %q", result.Message)
	}
}

func TestTownGirlNPC_Inspect(t *testing.T) {
	npc := park.NewTownGirlNPC(random.NewDeterministic(42))
	line := npc.Inspect()
	if line == "" {
		t.Fatalf("expected non-empty inspect line")
	}
}
