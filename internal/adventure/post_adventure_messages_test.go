package adventure

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAdventure_PostAdventureMessages(t *testing.T) {
	for _, tt := range []struct {
		name    string
		message string
		err     error
	}{
		{"delivered", "酒場からスープが届きました！ HPが20回復した！", nil},
		{"insufficient funds", "酒場から出前が届きましたが、お金が足りませんでした", nil},
		{"no reservation", "", nil},
		{"settlement error", "uncommitted delivery", errors.New("delivery unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			svc, _, _, chars := newTestService(t)
			calls := 0
			svc.SetPostAdventureHook(func(_ context.Context, id string) (string, error) {
				calls++
				if id != chars.value.ID {
					t.Fatalf("unexpected participant %q", id)
				}
				return tt.message, tt.err
			})
			result, err := svc.StartStage(context.Background(), chars.value.ID, StarterAdventure)
			if err != nil || calls != 1 {
				t.Fatalf("StartStage err=%v hook calls=%d", err, calls)
			}
			want := tt.message
			if tt.err != nil {
				want = ""
			}
			if got := result.PostAdventureMessages[chars.value.ID]; got != want {
				t.Fatalf("message=%q, want %q", got, want)
			}
			if want == "" && result.PostAdventureMessages != nil {
				t.Fatalf("unexpected messages: %v", result.PostAdventureMessages)
			}
		})
	}
}

func TestDungeonCrawlResult_OmitsEmptyPostAdventureMessages(t *testing.T) {
	data, err := json.Marshal(DungeonCrawlResult{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "post_adventure_messages") {
		t.Fatalf("empty messages must be omitted: %s", data)
	}
}
