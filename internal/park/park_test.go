package park_test

import (
	"math/rand"
	"strings"
	"testing"

	"github.com/witchcraze/party2re/internal/park"
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
		err := park.ValidatePost("char123", "Hello", "#000000", "")
		if err != nil {
			t.Fatalf("expected nil err, got %v", err)
		}
	})

	t.Run("Empty character ID", func(t *testing.T) {
		err := park.ValidatePost("", "Hello", "#000000", "")
		if err != park.ErrInvalidCharacterID {
			t.Fatalf("expected ErrInvalidCharacterID, got %v", err)
		}
	})

	t.Run("Empty content", func(t *testing.T) {
		err := park.ValidatePost("char123", "   ", "#000000", "")
		if err != park.ErrEmptyContent {
			t.Fatalf("expected ErrEmptyContent, got %v", err)
		}
	})

	t.Run("Content too long (>200 chars)", func(t *testing.T) {
		longContent := strings.Repeat("あ", 201)
		err := park.ValidatePost("char123", longContent, "#000000", "")
		if err != park.ErrContentTooLong {
			t.Fatalf("expected ErrContentTooLong, got %v", err)
		}
	})

	t.Run("Invalid color format", func(t *testing.T) {
		err := park.ValidatePost("char123", "Hello", "invalid-color-123456789012345678901234567890", "")
		if err != park.ErrInvalidColor {
			t.Fatalf("expected ErrInvalidColor, got %v", err)
		}
	})
}

func TestTownGirlNPC_Talk(t *testing.T) {
	npc := park.NewTownGirlNPC(rand.New(rand.NewSource(42)))
	line := npc.Talk("勇者", "勇者1号")
	if line == "" {
		t.Fatalf("expected non-empty dialogue line")
	}
	if !strings.Contains(line, "勇者1号") && !strings.Contains(line, "勇者") && !strings.Contains(line, "天気") && !strings.Contains(line, "元気") && !strings.Contains(line, "どこ") && !strings.Contains(line, "占い") && !strings.Contains(line, "夕飯") {
		t.Fatalf("unexpected dialogue line: %q", line)
	}
}

func TestTownGirlNPC_Divinate(t *testing.T) {
	npc := park.NewTownGirlNPC(rand.New(rand.NewSource(42)))
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
	npc := park.NewTownGirlNPC(rand.New(rand.NewSource(42)))
	line := npc.Inspect()
	if line == "" {
		t.Fatalf("expected non-empty inspect line")
	}
}
