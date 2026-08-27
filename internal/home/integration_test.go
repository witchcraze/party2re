package home_test

import (
	"context"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/home"
)

func TestHomeServiceIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()

	char1, err := database.CreateTestCharacter(ctx, db, "HomeIntegrationHero1")
	if err != nil {
		t.Fatal(err)
	}
	char2, err := database.CreateTestCharacter(ctx, db, "HomeIntegrationHero2")
	if err != nil {
		t.Fatal(err)
	}

	homeRepo, err := database.NewHomeRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	svc, err := home.NewService(homeRepo, charRepo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Visit & View
	view, err := svc.GetHomeView(ctx, char1.ID, char2.ID)
	if err != nil {
		t.Fatalf("GetHomeView failed: %v", err)
	}
	if view.Home.VisitorCount != 1 {
		t.Errorf("expected visitor count 1, got %d", view.Home.VisitorCount)
	}

	// 2. Custom settings
	updatedHome, err := svc.UpdateHome(ctx, char1.ID, "#123456", "Welcome all!", "ドラキー")
	if err != nil {
		t.Fatalf("UpdateHome failed: %v", err)
	}
	if updatedHome.CompanionName != "ドラキー" {
		t.Errorf("expected companion ドラキー, got %s", updatedHome.CompanionName)
	}

	// 3. Letters
	letter, err := svc.SendLetter(ctx, char2.ID, char1.ID, "Nice house!", "#00ff00")
	if err != nil {
		t.Fatalf("SendLetter failed: %v", err)
	}

	unread, err := svc.GetUnreadLetterCount(ctx, char1.ID)
	if err != nil || unread != 1 {
		t.Errorf("expected 1 unread, got %d, err=%v", unread, err)
	}

	err = svc.ReadLetter(ctx, letter.ID, char1.ID)
	if err != nil {
		t.Fatalf("ReadLetter failed: %v", err)
	}

	// 4. Companion phrases
	phrase, err := svc.TeachCompanionPhrase(ctx, char1.ID, "いらっしゃい！")
	if err != nil {
		t.Fatalf("TeachCompanionPhrase failed: %v", err)
	}

	talk, err := svc.TalkToCompanion(ctx, char1.ID)
	if err != nil || talk != "いらっしゃい！" {
		t.Errorf("expected 'いらっしゃい！', got %s, err=%v", talk, err)
	}

	err = svc.ForgetCompanionPhrase(ctx, phrase.ID, char1.ID)
	if err != nil {
		t.Fatalf("ForgetCompanionPhrase failed: %v", err)
	}
}
