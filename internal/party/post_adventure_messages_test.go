package party

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func TestPartyAdventure_PostAdventureMessages(t *testing.T) {
	for _, tt := range []struct {
		name          string
		leaderMessage string
		leaderErr     error
	}{
		{"delivered", "酒場からスープが届きました！ HPが20回復した！", nil},
		{"insufficient funds", "酒場から出前が届きましたが、お金が足りませんでした", nil},
		{"no reservation", "", nil},
		{"settlement error", "uncommitted delivery", errors.New("delivery unavailable")},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			svc, repo, chars, _, _ := setupPartyAdventureTestService(t, fixedBattleEngine{outcome: corebattle.OutcomeWin})
			leader := corecharacter.Character{ID: "leader", Name: "Leader", Level: 10, Stats: corecharacter.Stats{HP: 100, MaxHP: 100}}
			ally := leader
			ally.ID, ally.Name = "ally", "Ally"
			partyID := createReadyParty(ctx, t, svc, repo, chars, leader, ally)
			calls := map[string]int{}
			svc.SetPostAdventureHook(func(_ context.Context, id string) (string, error) {
				calls[id]++
				if id == leader.ID {
					return tt.leaderMessage, tt.leaderErr
				}
				return "酒場からパンが届きました！", nil
			})
			result, err := svc.StartPartyAdventure(ctx, partyID, leader.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := map[string]string{ally.ID: "酒場からパンが届きました！"}
			if tt.leaderMessage != "" && tt.leaderErr == nil {
				want[leader.ID] = tt.leaderMessage
			}
			if !reflect.DeepEqual(result.PostAdventureMessages, want) || calls[leader.ID] != 1 || calls[ally.ID] != 1 {
				t.Fatalf("messages=%v want=%v calls=%v", result.PostAdventureMessages, want, calls)
			}
			data, err := json.Marshal(result)
			if err != nil {
				t.Fatal(err)
			}
			var decoded PartyAdventureResult
			if err := json.Unmarshal(data, &decoded); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(decoded.PostAdventureMessages, want) {
				t.Fatalf("lost messages: %s", data)
			}
		})
	}
}

func TestPartyAdventureResult_OmitsEmptyPostAdventureMessages(t *testing.T) {
	data, err := json.Marshal(PartyAdventureResult{})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "post_adventure_messages") {
		t.Fatalf("empty messages must be omitted: %s", data)
	}
}
