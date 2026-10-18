package database_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/helperquest"
)

type helperHTTPPlayer struct{ apihttp.PlayerService }

func (helperHTTPPlayer) Authenticate(_ context.Context, sessionID string) (coreplayer.Player, error) {
	if sessionID != "helper-session" {
		return coreplayer.Player{}, errors.New("unauthorized")
	}
	return coreplayer.Player{ID: "helper-player"}, nil
}

type helperHTTPCharacter struct{ apihttp.CharacterService }

func (helperHTTPCharacter) Get(_ context.Context, characterID string) (corecharacter.Character, error) {
	return corecharacter.Character{ID: characterID, PlayerID: "helper-player"}, nil
}

func TestHelperHTTPBoardRenewal(t *testing.T) {
	for _, count := range []int{0, 2} {
		t.Run(fmt.Sprintf("expired=%d", count), func(t *testing.T) {
			f := newRenewalFixture(t)
			for i := range count {
				f.seed(t, fmt.Sprintf("expired-%d", i), f.now.Add(-time.Hour), "")
			}
			h, err := apihttp.NewHandler(helperHTTPPlayer{}, helperHTTPCharacter{}, struct{ apihttp.AdventureService }{}, struct{ apihttp.ShopService }{}, apihttp.WithHelper(f.service))
			if err != nil {
				t.Fatal(err)
			}
			router := h.Router()
			list := func() []helperquest.Quest {
				t.Helper()
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/helpers/quests", nil))
				if rec.Code != http.StatusOK {
					t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
				}
				var quests []helperquest.Quest
				if err := json.Unmarshal(rec.Body.Bytes(), &quests); err != nil {
					t.Fatal(err)
				}
				if quests == nil {
					t.Fatal("list must return an array, including for an empty board")
				}
				return quests
			}
			before := list()
			if len(before) != count || f.count(t) != count {
				t.Fatalf("GET must expose all expired requests without writes: listed=%d stored=%d want=%d", len(before), f.count(t), count)
			}
			questID := "unknown"
			if count > 0 {
				questID = before[0].ID
				for _, q := range before {
					if !q.ExpiresAt.Before(f.now) {
						t.Fatalf("GET renewed expired request: %+v", q)
					}
				}
			}
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/helpers/complete", strings.NewReader(fmt.Sprintf(`{"character_id":"helper-character","quest_id":%q}`, questID)))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer helper-session")
			router.ServeHTTP(rec, req)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("completion: %d %s", rec.Code, rec.Body.String())
			}
			wantErr := helperquest.ErrQuestNotFound
			if count > 0 {
				wantErr = helperquest.ErrQuestExpired
			}
			if !strings.Contains(rec.Body.String(), wantErr.Error()) {
				t.Fatalf("completion error: %s, want %v", rec.Body.String(), wantErr)
			}
			after := list()
			if len(after) != count || f.count(t) != count {
				t.Fatalf("one-for-one renewal: listed=%d stored=%d want=%d", len(after), f.count(t), count)
			}
			for _, q := range after {
				if !q.ExpiresAt.After(f.now) || q.CompletedAt != nil {
					t.Fatalf("expected fresh uncompleted request: %+v", q)
				}
			}
		})
	}
}

func TestHelperBoardListsUncompletedRegardlessOfDeadline(t *testing.T) {
	f := newRenewalFixture(t)
	f.seed(t, "expired", f.now.Add(-time.Hour), "")
	f.seed(t, "boundary", f.now, "")
	f.seed(t, "live", f.now.Add(time.Hour), "")
	f.seed(t, "done", f.now.Add(time.Hour), "someone")
	board, err := f.service.ListQuests(context.Background(), f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(board) != 3 || board[0].ID != "boundary" || board[1].ID != "expired" || board[2].ID != "live" {
		t.Fatalf("board = %+v, want boundary, expired, live in stable order", board)
	}
	active, err := f.service.GetActiveHelperItemIDs(context.Background(), f.now)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 1 {
		t.Fatalf("shop exclusion must still use active requests only: %v", active)
	}
	if f.count(t) != 4 {
		t.Fatalf("reads changed stored rows: %d", f.count(t))
	}
}
