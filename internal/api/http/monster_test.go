package http_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apihttp "github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
	"github.com/witchcraze/party2re/internal/monster"
)

type stubMonsterService struct {
	getSummaryFn     func(ctx context.Context, characterID, locationFilter string) (monster.MonsterBoxSummary, error)
	tameMonsterFn    func(ctx context.Context, characterID, monsterID, customName string) (monster.MonsterInstance, error)
	bringToHomeFn    func(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	depositToBoxFn   func(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error)
	renameFn         func(ctx context.Context, characterID, instanceID, newName string) (monster.MonsterInstance, error)
	sendMonsterFn    func(ctx context.Context, senderCharID, recipientCharID, instanceID string) error
	releaseMonsterFn func(ctx context.Context, characterID, instanceID string) error
	getDialogueFn    func() monster.Dialogue
}

func (s *stubMonsterService) GetSummary(ctx context.Context, characterID, locationFilter string) (monster.MonsterBoxSummary, error) {
	if s.getSummaryFn != nil {
		return s.getSummaryFn(ctx, characterID, locationFilter)
	}
	return monster.MonsterBoxSummary{}, nil
}

func (s *stubMonsterService) TameMonster(ctx context.Context, characterID, monsterID, customName string) (monster.MonsterInstance, error) {
	if s.tameMonsterFn != nil {
		return s.tameMonsterFn(ctx, characterID, monsterID, customName)
	}
	return monster.MonsterInstance{}, nil
}

func (s *stubMonsterService) BringToHome(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error) {
	if s.bringToHomeFn != nil {
		return s.bringToHomeFn(ctx, characterID, instanceID)
	}
	return monster.MonsterInstance{}, nil
}

func (s *stubMonsterService) DepositToBox(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error) {
	if s.depositToBoxFn != nil {
		return s.depositToBoxFn(ctx, characterID, instanceID)
	}
	return monster.MonsterInstance{}, nil
}

func (s *stubMonsterService) Rename(ctx context.Context, characterID, instanceID, newName string) (monster.MonsterInstance, error) {
	if s.renameFn != nil {
		return s.renameFn(ctx, characterID, instanceID, newName)
	}
	return monster.MonsterInstance{}, nil
}

func (s *stubMonsterService) SendMonster(ctx context.Context, senderCharID, recipientCharID, instanceID string) error {
	if s.sendMonsterFn != nil {
		return s.sendMonsterFn(ctx, senderCharID, recipientCharID, instanceID)
	}
	return nil
}

func (s *stubMonsterService) ReleaseMonster(ctx context.Context, characterID, instanceID string) error {
	if s.releaseMonsterFn != nil {
		return s.releaseMonsterFn(ctx, characterID, instanceID)
	}
	return nil
}

func (s *stubMonsterService) GetDialogue() monster.Dialogue {
	if s.getDialogueFn != nil {
		return s.getDialogueFn()
	}
	return monster.Dialogue{NPCName: "@モンジィ", Title: "モンスターじいさん"}
}

func TestMonsterEndpoints(t *testing.T) {
	player := coreplayer.Player{ID: "p1", Username: "hero"}
	char := corecharacter.Character{ID: "c1", PlayerID: "p1", Name: "Hero"}

	pService := &stubPlayerService{
		authenticateFn: alwaysAuthPlayer(player),
	}
	cService := &stubCharacterService{
		getFn: func(_ context.Context, id string) (corecharacter.Character, error) {
			if id == "c1" {
				return char, nil
			}
			return corecharacter.Character{}, corecharacter.ErrNotFound
		},
	}
	mService := &stubMonsterService{
		getDialogueFn: func() monster.Dialogue {
			return monster.Dialogue{NPCName: "@モンジィ", Title: "モンスターじいさん", Phrases: []string{"hello"}}
		},
		getSummaryFn: func(ctx context.Context, characterID, locationFilter string) (monster.MonsterBoxSummary, error) {
			return monster.MonsterBoxSummary{
				BoxCount:     1,
				BoxCapacity:  50,
				HomeCount:    1,
				HomeCapacity: 8,
				Monsters: []monster.MonsterInstance{
					{ID: "m1", CharacterID: characterID, MonsterID: "slime", CustomName: "スラりん", Location: monster.LocationBox},
				},
			}, nil
		},
		tameMonsterFn: func(ctx context.Context, characterID, monsterID, customName string) (monster.MonsterInstance, error) {
			return monster.MonsterInstance{ID: "m2", CharacterID: characterID, MonsterID: monsterID, CustomName: customName, Location: monster.LocationBox}, nil
		},
		bringToHomeFn: func(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error) {
			return monster.MonsterInstance{ID: instanceID, CharacterID: characterID, MonsterID: "slime", CustomName: "スラりん", Location: monster.LocationHome}, nil
		},
		depositToBoxFn: func(ctx context.Context, characterID, instanceID string) (monster.MonsterInstance, error) {
			return monster.MonsterInstance{ID: instanceID, CharacterID: characterID, MonsterID: "slime", CustomName: "スラりん", Location: monster.LocationBox}, nil
		},
		renameFn: func(ctx context.Context, characterID, instanceID, newName string) (monster.MonsterInstance, error) {
			return monster.MonsterInstance{ID: instanceID, CharacterID: characterID, MonsterID: "slime", CustomName: newName, Location: monster.LocationBox}, nil
		},
	}

	handler, err := apihttp.NewHandler(
		pService,
		cService,
		&stubAdventureService{},
		&stubShopService{},
		apihttp.WithMonster(mService),
	)
	if err != nil {
		t.Fatalf("apihttp.New failed: %v", err)
	}

	server := httptest.NewServer(handler.Router())
	defer server.Close()

	// 1. GET /monster/dialogue (public)
	{
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/monster/dialogue", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /monster/dialogue failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var d monster.Dialogue
		_ = json.NewDecoder(resp.Body).Decode(&d)
		resp.Body.Close()
		if d.NPCName != "@モンジィ" {
			t.Errorf("expected @モンジィ, got %s", d.NPCName)
		}
	}

	// 2. GET /characters/c1/monsters (authenticated)
	{
		req, _ := http.NewRequest(http.MethodGet, server.URL+"/characters/c1/monsters?location=box", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("GET /characters/c1/monsters failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var summary monster.MonsterBoxSummary
		_ = json.NewDecoder(resp.Body).Decode(&summary)
		resp.Body.Close()
		if summary.BoxCount != 1 || len(summary.Monsters) != 1 {
			t.Errorf("unexpected summary: %+v", summary)
		}
	}

	// 3. POST /characters/c1/monsters/tame
	{
		payload := []byte(`{"monster_id":"slime","custom_name":"スラりん"}`)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/tame", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST /characters/c1/monsters/tame failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var inst monster.MonsterInstance
		_ = json.NewDecoder(resp.Body).Decode(&inst)
		resp.Body.Close()
		if inst.CustomName != "スラりん" {
			t.Errorf("expected custom_name スラりん, got %s", inst.CustomName)
		}
	}

	// 4. POST /characters/c1/monsters/m1/bring-home
	{
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/m1/bring-home", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST bring-home failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var inst monster.MonsterInstance
		_ = json.NewDecoder(resp.Body).Decode(&inst)
		resp.Body.Close()
		if inst.Location != monster.LocationHome {
			t.Errorf("expected LocationHome, got %s", inst.Location)
		}
	}

	// 5. POST /characters/c1/monsters/m1/deposit
	{
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/m1/deposit", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST deposit failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var inst monster.MonsterInstance
		_ = json.NewDecoder(resp.Body).Decode(&inst)
		resp.Body.Close()
		if inst.Location != monster.LocationBox {
			t.Errorf("expected LocationBox, got %s", inst.Location)
		}
	}

	// 6. POST /characters/c1/monsters/m1/rename
	{
		payload := []byte(`{"custom_name":"スラきち"}`)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/m1/rename", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST rename failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var inst monster.MonsterInstance
		_ = json.NewDecoder(resp.Body).Decode(&inst)
		resp.Body.Close()
		if inst.CustomName != "スラきち" {
			t.Errorf("expected custom_name スラきち, got %s", inst.CustomName)
		}
	}

	// 7. POST /characters/c1/monsters/m1/send
	{
		payload := []byte(`{"recipient_character_id":"c2"}`)
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/m1/send", bytes.NewReader(payload))
		req.Header.Set("Authorization", "Bearer valid-token")
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST send failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// 8. POST /characters/c1/monsters/m1/release
	{
		req, _ := http.NewRequest(http.MethodPost, server.URL+"/characters/c1/monsters/m1/release", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("POST release failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}

	// 9. IDOR protection: accessing with another player's token
	{
		otherPlayer := coreplayer.Player{ID: "p2", Username: "other"}
		pServiceOther := &stubPlayerService{
			authenticateFn: alwaysAuthPlayer(otherPlayer),
		}
		hOther, _ := apihttp.NewHandler(
			pServiceOther,
			cService,
			&stubAdventureService{},
			&stubShopService{},
			apihttp.WithMonster(mService),
		)
		sOther := httptest.NewServer(hOther.Router())
		defer sOther.Close()

		req, _ := http.NewRequest(http.MethodGet, sOther.URL+"/characters/c1/monsters", nil)
		req.Header.Set("Authorization", "Bearer other-token")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("IDOR request failed: %v", err)
		}
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 Forbidden for IDOR attempt, got %d", resp.StatusCode)
		}
		resp.Body.Close()
	}
}
