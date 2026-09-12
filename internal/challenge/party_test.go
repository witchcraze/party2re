package challenge_test

import (
	"context"
	"testing"

	"github.com/witchcraze/party2re/internal/challenge"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func createPartyTestChar(id string, level int, hp int, atk int, def int) corecharacter.Character {
	return corecharacter.Character{
		ID:         id,
		Level:      level,
		Experience: 5000,
		Stats: corecharacter.Stats{
			HP:      hp,
			MaxHP:   hp,
			Attack:  atk,
			Defense: def,
			Agility: 50,
		},
	}
}

func TestStartPartyChallengeSession(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}

	c1 := createPartyTestChar("lead", 15, 200, 50, 30)
	c1.Name = "挑戦勇者"
	c1.JobID = "hero"
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 15, 180, 45, 25)
	c2.Name = "挑戦戦士"
	c2.JobID = "warrior"
	charRepo.chars[c2.ID] = c2

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{})
	if err != nil {
		t.Fatalf("failed to create challenge service: %v", err)
	}
	ctx := context.Background()

	// 1. Success starting party session
	sess, err := svc.StartPartySession(ctx, "lead", []string{"lead", "c2"}, "novice", "栄光の騎士団", "#00FF00")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}
	if sess == nil {
		t.Fatal("expected non-nil session")
	}
	if len(sess.Members) != 2 {
		t.Fatalf("expected 2 members, got %d", len(sess.Members))
	}
	if sess.PartyName != "栄光の騎士団" {
		t.Errorf("expected party name 栄光の騎士団, got %s", sess.PartyName)
	}
	if sess.PartyColor != "#00FF00" {
		t.Errorf("expected party color #00FF00, got %s", sess.PartyColor)
	}
}

func TestPartyAdvanceRoundAndHallOfFame(t *testing.T) {
	repo := newMockChallengeRepo()
	charRepo := &mockCharRepo{chars: make(map[string]corecharacter.Character)}

	c1 := createPartyTestChar("lead", 30, 500, 100, 80)
	c1.Name = "勇者アリス"
	c1.JobID = "paladin"
	c1.Stats.MP = 50
	c1.Stats.MaxMP = 50
	charRepo.chars[c1.ID] = c1

	c2 := createPartyTestChar("c2", 30, 400, 90, 70)
	c2.Name = "魔法使いボブ"
	c2.JobID = "wizard"
	c2.Stats.MP = 100
	c2.Stats.MaxMP = 100
	charRepo.chars[c2.ID] = c2

	svc, err := challenge.NewService(repo, charRepo, &corebattle.Engine{})
	if err != nil {
		t.Fatalf("failed to create service: %v", err)
	}
	ctx := context.Background()

	sess, err := svc.StartPartySession(ctx, "lead", []string{"lead", "c2"}, "novice", "ドリームチーム", "#FF0000")
	if err != nil {
		t.Fatalf("StartPartySession failed: %v", err)
	}

	// Advance round 1
	roundRes, updatedSess, err := svc.AdvanceRound(ctx, "lead", sess.ID)
	if err != nil {
		t.Fatalf("AdvanceRound failed: %v", err)
	}
	if !roundRes.Won {
		t.Fatalf("expected round 1 to be won by strong party")
	}
	if updatedSess.CurrentRound != 2 {
		t.Errorf("expected session round to advance to 2, got %d", updatedSess.CurrentRound)
	}

	// Check 20% MaxHP recovery
	for _, m := range updatedSess.Members {
		if m.CharacterCurrentHP <= 0 {
			t.Errorf("expected surviving member %s to have positive HP", m.CharacterID)
		}
	}

	// Check Hall of Fame record
	hof, err := svc.GetHallOfFame(ctx, "novice")
	if err != nil {
		t.Fatalf("GetHallOfFame failed: %v", err)
	}
	if hof == nil {
		t.Fatal("expected Hall of Fame entry to be created for new highest round")
	}
	if hof.HighestRound != 1 {
		t.Errorf("expected Hall of Fame highest round 1, got %d", hof.HighestRound)
	}
	if hof.PartyName != "ドリームチーム" {
		t.Errorf("expected Hall of Fame party name ドリームチーム, got %s", hof.PartyName)
	}
	if len(hof.Members) != 2 {
		t.Fatalf("expected 2 members in Hall of Fame, got %d", len(hof.Members))
	}
	if hof.Members[0].CharacterName != "勇者アリス" || hof.Members[0].JobID != "paladin" {
		t.Errorf("unexpected member 0 in Hall of Fame: %+v", hof.Members[0])
	}
	if hof.Members[1].CharacterName != "魔法使いボブ" || hof.Members[1].JobID != "wizard" {
		t.Errorf("unexpected member 1 in Hall of Fame: %+v", hof.Members[1])
	}
}
