package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	"github.com/witchcraze/party2re/internal/contest"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type stubContestService struct {
	getDialogueFn       func() contest.Dialogue
	getOverviewFn       func(ctx context.Context) (contest.ContestOverview, error)
	savePhotoFn         func(ctx context.Context, characterID, title, location, imageURL, caption, metadata string) (contest.Photo, error)
	listPhotosFn        func(ctx context.Context, characterID string) ([]contest.Photo, error)
	deletePhotoFn       func(ctx context.Context, characterID, photoID string) error
	enterContestFn      func(ctx context.Context, characterID, photoID, title string) (contest.ContestEntry, error)
	voteFn              func(ctx context.Context, voterCharacterID, entryID, comment string) (contest.ContestVote, error)
	getCurrentEntriesFn func(ctx context.Context) ([]contest.ContestEntry, error)
	getPastResultsFn    func(ctx context.Context) (*contest.ContestRound, []contest.ContestEntry, error)
	getLegendsFn        func(ctx context.Context, limit, offset int) ([]contest.ContestLegend, error)
	settleContestFn     func(ctx context.Context, force bool) (contest.SettlementResult, error)
}

func (s *stubContestService) GetDialogue() contest.Dialogue {
	if s.getDialogueFn != nil {
		return s.getDialogueFn()
	}
	return contest.Dialogue{NPCName: "@ワコール"}
}

func (s *stubContestService) GetOverview(ctx context.Context) (contest.ContestOverview, error) {
	if s.getOverviewFn != nil {
		return s.getOverviewFn(ctx)
	}
	return contest.ContestOverview{}, nil
}

func (s *stubContestService) SavePhoto(ctx context.Context, characterID, title, location, imageURL, caption, metadata string) (contest.Photo, error) {
	if s.savePhotoFn != nil {
		return s.savePhotoFn(ctx, characterID, title, location, imageURL, caption, metadata)
	}
	return contest.Photo{ID: "photo-1", CharacterID: characterID, Title: title}, nil
}

func (s *stubContestService) ListPhotos(ctx context.Context, characterID string) ([]contest.Photo, error) {
	if s.listPhotosFn != nil {
		return s.listPhotosFn(ctx, characterID)
	}
	return []contest.Photo{}, nil
}

func (s *stubContestService) DeletePhoto(ctx context.Context, characterID, photoID string) error {
	if s.deletePhotoFn != nil {
		return s.deletePhotoFn(ctx, characterID, photoID)
	}
	return nil
}

func (s *stubContestService) EnterContest(ctx context.Context, characterID, photoID, title string) (contest.ContestEntry, error) {
	if s.enterContestFn != nil {
		return s.enterContestFn(ctx, characterID, photoID, title)
	}
	return contest.ContestEntry{ID: "entry-1", CharacterID: characterID, Title: title}, nil
}

func (s *stubContestService) Vote(ctx context.Context, voterCharacterID, entryID, comment string) (contest.ContestVote, error) {
	if s.voteFn != nil {
		return s.voteFn(ctx, voterCharacterID, entryID, comment)
	}
	return contest.ContestVote{ID: "vote-1", EntryID: entryID, VoterCharacterID: voterCharacterID}, nil
}

func (s *stubContestService) GetCurrentEntries(ctx context.Context) ([]contest.ContestEntry, error) {
	if s.getCurrentEntriesFn != nil {
		return s.getCurrentEntriesFn(ctx)
	}
	return []contest.ContestEntry{}, nil
}

func (s *stubContestService) GetPastResults(ctx context.Context) (*contest.ContestRound, []contest.ContestEntry, error) {
	if s.getPastResultsFn != nil {
		return s.getPastResultsFn(ctx)
	}
	return nil, []contest.ContestEntry{}, nil
}

func (s *stubContestService) GetLegends(ctx context.Context, limit, offset int) ([]contest.ContestLegend, error) {
	if s.getLegendsFn != nil {
		return s.getLegendsFn(ctx, limit, offset)
	}
	return []contest.ContestLegend{}, nil
}

func (s *stubContestService) SettleContest(ctx context.Context, force bool) (contest.SettlementResult, error) {
	if s.settleContestFn != nil {
		return s.settleContestFn(ctx, force)
	}
	return contest.SettlementResult{}, nil
}

func TestContestPublicEndpoints(t *testing.T) {
	stub := &stubContestService{
		getDialogueFn: func() contest.Dialogue {
			return contest.Dialogue{NPCName: "@ワコール", Title: "フォトコン会場"}
		},
		getOverviewFn: func(ctx context.Context) (contest.ContestOverview, error) {
			return contest.ContestOverview{MinEntries: 5}, nil
		},
		getCurrentEntriesFn: func(ctx context.Context) ([]contest.ContestEntry, error) {
			return []contest.ContestEntry{{ID: "e1", Title: "Entry 1"}}, nil
		},
		getPastResultsFn: func(ctx context.Context) (*contest.ContestRound, []contest.ContestEntry, error) {
			round := contest.ContestRound{Round: 1, Status: contest.StatusSettled}
			return &round, []contest.ContestEntry{{ID: "e1", Title: "Past Winner", Ranking: 1}}, nil
		},
		getLegendsFn: func(ctx context.Context, limit, offset int) ([]contest.ContestLegend, error) {
			return []contest.ContestLegend{{Round: 1, Title: "Legendary"}}, nil
		},
	}

	pService := &stubPlayerService{}
	cService := &stubCharacterService{}
	aService := &stubAdventureService{}
	sService := &stubShopService{}

	handler, err := apihttp.NewHandler(pService, cService, aService, sService, apihttp.WithContest(stub))
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	// 1. GET /contest/venue
	resp, err := http.Get(server.URL + "/contest/venue")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /contest/venue failed: resp=%v, err=%v", resp, err)
	}

	// 2. GET /contest/current
	resp, err = http.Get(server.URL + "/contest/current")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /contest/current failed: resp=%v, err=%v", resp, err)
	}

	// 3. GET /contest/past
	resp, err = http.Get(server.URL + "/contest/past")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /contest/past failed: resp=%v, err=%v", resp, err)
	}

	// 4. GET /contest/legends
	resp, err = http.Get(server.URL + "/contest/legends")
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /contest/legends failed: resp=%v, err=%v", resp, err)
	}
}

func TestContestCharacterEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "player-1", Username: "user1"}
	char := corecharacter.Character{ID: "char-1", PlayerID: player.ID, Name: "Hero"}

	playerSvc := &stubPlayerService{
		authenticateFn: func(ctx context.Context, sessionID string) (coreplayer.Player, error) {
			if sessionID == "valid-session" {
				return player, nil
			}
			return coreplayer.Player{}, coreplayer.ErrInvalidSession
		},
	}

	charSvc := &stubCharacterService{
		getFn: func(ctx context.Context, id string) (corecharacter.Character, error) {
			if id == char.ID {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}

	stub := &stubContestService{
		listPhotosFn: func(ctx context.Context, characterID string) ([]contest.Photo, error) {
			return []contest.Photo{{ID: "p1", Title: "Photo 1", CharacterID: characterID}}, nil
		},
		savePhotoFn: func(ctx context.Context, characterID, title, location, imageURL, caption, metadata string) (contest.Photo, error) {
			return contest.Photo{ID: "p-new", CharacterID: characterID, Title: title}, nil
		},
		deletePhotoFn: func(ctx context.Context, characterID, photoID string) error {
			if photoID == "forbidden-photo" {
				return contest.ErrForbidden
			}
			return nil
		},
		enterContestFn: func(ctx context.Context, characterID, photoID, title string) (contest.ContestEntry, error) {
			if title == "Consecutive" {
				return contest.ContestEntry{}, contest.ErrConsecutiveEntryDisallowed
			}
			return contest.ContestEntry{ID: "e-new", CharacterID: characterID, Title: title}, nil
		},
		voteFn: func(ctx context.Context, voterCharacterID, entryID, comment string) (contest.ContestVote, error) {
			if entryID == "own-entry" {
				return contest.ContestVote{}, contest.ErrSelfVoteDisallowed
			}
			return contest.ContestVote{ID: "v-new", EntryID: entryID, VoterCharacterID: voterCharacterID}, nil
		},
	}

	handler, err := apihttp.NewHandler(playerSvc, charSvc, &stubAdventureService{}, &stubShopService{}, apihttp.WithContest(stub))
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	client := &http.Client{}

	// 1. GET /characters/{id}/photos - Unauthenticated
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/characters/char-1/photos", nil)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 Unauthorized without session, got %d", resp.StatusCode)
	}

	// 2. GET /characters/{id}/photos - Authenticated
	req, _ = http.NewRequest(http.MethodGet, server.URL+"/characters/char-1/photos", nil)
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 3. POST /characters/{id}/photos - Save Photo
	body, _ := json.Marshal(map[string]string{
		"title":     "New Sunset",
		"location":  "Beach",
		"image_url": "http://example.com/sunset.png",
		"caption":   "Peaceful ocean",
	})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/photos", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	// 4. DELETE /characters/{id}/photos/{photoId} - Delete Photo
	req, _ = http.NewRequest(http.MethodDelete, server.URL+"/characters/char-1/photos/p1", nil)
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 5. POST /characters/{id}/contest/enter - Enter Contest
	entryBody, _ := json.Marshal(map[string]string{
		"photo_id": "p1",
		"title":    "Sunset Entry",
	})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/contest/enter", bytes.NewReader(entryBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201 Created, got %d", resp.StatusCode)
	}

	// 6. POST /characters/{id}/contest/vote - Vote
	voteBody, _ := json.Marshal(map[string]string{
		"entry_id": "e1",
		"comment":  "Wonderful!",
	})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/characters/char-1/contest/vote", bytes.NewReader(voteBody))
	req.Header.Set("Authorization", "Bearer valid-session")
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}

	// 7. POST /contest/settle - Admin Auth Required
	settleBody, _ := json.Marshal(map[string]bool{"force": true})
	req, _ = http.NewRequest(http.MethodPost, server.URL+"/contest/settle", bytes.NewReader(settleBody))
	resp, err = client.Do(req)
	if err != nil || resp.StatusCode != http.StatusForbidden {
		t.Fatalf("expected 403 Forbidden for settle without admin key, got %d", resp.StatusCode)
	}
}

func TestSettleContestWithAdminKey(t *testing.T) {
	adminKey := "test-admin-secret-key"
	stub := &stubContestService{
		settleContestFn: func(ctx context.Context, force bool) (contest.SettlementResult, error) {
			return contest.SettlementResult{
				Round:             1,
				PrizesDistributed: true,
				Message:           "Settled",
			}, nil
		},
	}

	handler, err := apihttp.NewHandler(&stubPlayerService{}, &stubCharacterService{}, &stubAdventureService{}, &stubShopService{},
		apihttp.WithAdminAPIKey(adminKey),
		apihttp.WithContest(stub),
	)
	if err != nil {
		t.Fatalf("NewHandler failed: %v", err)
	}
	server := httptest.NewServer(handler.Router())
	defer server.Close()

	client := &http.Client{}
	settleBody, _ := json.Marshal(map[string]bool{"force": true})

	req, _ := http.NewRequest(http.MethodPost, server.URL+"/contest/settle", bytes.NewReader(settleBody))
	req.Header.Set("X-Admin-Key", adminKey)
	resp, err := client.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 OK with admin key, got %d (err: %v)", resp.StatusCode, err)
	}
}
