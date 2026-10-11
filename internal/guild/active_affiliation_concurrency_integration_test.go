package guild_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/testutil"
)

type activeAffiliationFixture struct {
	db      *sql.DB
	repo    *database.GuildRepository
	service *guild.Service
}

func newActiveAffiliationFixture(t *testing.T) *activeAffiliationFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})
	repo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := database.NewTransactionProvider(db)
	svc, err := guild.NewService(repo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}
	return &activeAffiliationFixture{db: db, repo: repo, service: svc}
}

func (f *activeAffiliationFixture) createCharacter(t *testing.T, prefix string, money int) corecharacter.Character {
	t.Helper()
	ctx := context.Background()
	char, err := database.CreateTestCharacter(ctx, f.db, prefix+"_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	if money > 0 {
		_, err := f.db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", money, char.ID)
		if err != nil {
			t.Fatal(err)
		}
		char.Money = money
	}
	t.Cleanup(func() {
		cleanupCtx := context.Background()
		_, _ = f.db.ExecContext(cleanupCtx, "DELETE FROM guild_members WHERE character_id = ?", char.ID)
		_, _ = f.db.ExecContext(cleanupCtx, "DELETE FROM character_letters WHERE recipient_character_id = ? OR sender_character_id = ?", char.ID, char.ID)
		chars, _ := database.NewCharacterRepository(f.db)
		if chars != nil {
			_ = chars.Delete(cleanupCtx, char.ID)
		}
		players, _ := database.NewPlayerRepository(f.db)
		if players != nil {
			_ = players.Delete(cleanupCtx, char.PlayerID)
		}
	})
	return char
}

func (f *activeAffiliationFixture) countActiveAffiliations(t *testing.T, charID string) int {
	t.Helper()
	var count int
	err := f.db.QueryRow("SELECT COUNT(*) FROM guild_members WHERE character_id = ? AND is_pending = FALSE", charID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *activeAffiliationFixture) countGuildsLed(t *testing.T, charID string) int {
	t.Helper()
	var count int
	err := f.db.QueryRow("SELECT COUNT(*) FROM guilds WHERE leader_character_id = ?", charID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func (f *activeAffiliationFixture) getCharacterMoney(t *testing.T, charID string) int {
	t.Helper()
	var money int
	err := f.db.QueryRow("SELECT money FROM characters WHERE id = ?", charID).Scan(&money)
	if err != nil {
		t.Fatal(err)
	}
	return money
}

func TestGuildActiveAffiliation_ConcurrentCreate(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	initialMoney := 20000
	creator := f.createCharacter(t, "AffilCreate", initialMoney)

	name1 := "AffilG1_" + id.New()[:8]
	name2 := "AffilG2_" + id.New()[:8]

	err1, err2 := testutil.RunRace2(
		func() error {
			_, _, _, err := f.service.Create(ctx, creator.ID, name1)
			return err
		},
		func() error {
			_, _, _, err := f.service.Create(ctx, creator.ID, name2)
			return err
		},
	)

	// Exactly one must succeed, and the other must return ErrCharacterAlreadyInGuild
	var successCount, alreadyInGuildCount int
	for _, err := range []error{err1, err2} {
		if err == nil {
			successCount++
		} else if errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			alreadyInGuildCount++
		} else {
			t.Errorf("unexpected error from Create: %v", err)
		}
	}
	if successCount != 1 || alreadyInGuildCount != 1 {
		t.Fatalf("expected 1 success and 1 ErrCharacterAlreadyInGuild, got %d successes and %d alreadyInGuild (err1=%v, err2=%v)",
			successCount, alreadyInGuildCount, err1, err2)
	}

	// Invariant: active affiliations must be exactly 1
	if activeCount := f.countActiveAffiliations(t, creator.ID); activeCount != 1 {
		t.Errorf("active affiliation count = %d, want 1", activeCount)
	}

	// Invariant: exactly 1 guild created
	if guildsLed := f.countGuildsLed(t, creator.ID); guildsLed != 1 {
		t.Errorf("guilds led = %d, want 1", guildsLed)
	}

	// Invariant: exactly one creation fee deducted (5,000)
	expectedMoney := initialMoney - guild.CreationFee
	if currentMoney := f.getCharacterMoney(t, creator.ID); currentMoney != expectedMoney {
		t.Errorf("character money = %d, want %d (double deduction bug)", currentMoney, expectedMoney)
	}
}

func TestGuildActiveAffiliation_ConcurrentApproval(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	leader1 := f.createCharacter(t, "AppLead1", 20000)
	leader2 := f.createCharacter(t, "AppLead2", 20000)
	applicant := f.createCharacter(t, "AppTarget", 5000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "AppG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	g2, _, _, err := f.service.Create(ctx, leader2.ID, "AppG2_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// Applicant submits pending applications to both guilds
	if err := f.service.ApplyToJoin(ctx, g1.ID, applicant.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, g2.ID, applicant.ID); err != nil {
		t.Fatal(err)
	}

	// Both leaders approve the applicant concurrently
	err1, err2 := testutil.RunRace2(
		func() error {
			return f.service.ApproveApplication(ctx, g1.ID, leader1.ID, applicant.ID, "騎士")
		},
		func() error {
			return f.service.ApproveApplication(ctx, g2.ID, leader2.ID, applicant.ID, "魔導士")
		},
	)

	var successCount, alreadyInGuildCount int
	for _, err := range []error{err1, err2} {
		if err == nil {
			successCount++
		} else if errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			alreadyInGuildCount++
		} else {
			t.Errorf("unexpected error from ApproveApplication: %v", err)
		}
	}
	if successCount != 1 || alreadyInGuildCount != 1 {
		t.Fatalf("expected 1 success and 1 ErrCharacterAlreadyInGuild, got %d successes and %d alreadyInGuild (err1=%v, err2=%v)",
			successCount, alreadyInGuildCount, err1, err2)
	}

	// Invariant: active affiliations must be exactly 1
	if activeCount := f.countActiveAffiliations(t, applicant.ID); activeCount != 1 {
		t.Errorf("active affiliation count = %d, want 1", activeCount)
	}
}

func TestGuildActiveAffiliation_CreateVsApproval(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	leader1 := f.createCharacter(t, "CvALead", 20000)
	target := f.createCharacter(t, "CvATarget", 20000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "CvAG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// Target applies to G1
	if err := f.service.ApplyToJoin(ctx, g1.ID, target.ID); err != nil {
		t.Fatal(err)
	}

	newGuildName := "CvANew_" + id.New()[:8]

	// Target creates a new guild while Leader1 approves Target concurrently
	err1, err2 := testutil.RunRace2(
		func() error {
			_, _, _, err := f.service.Create(ctx, target.ID, newGuildName)
			return err
		},
		func() error {
			return f.service.ApproveApplication(ctx, g1.ID, leader1.ID, target.ID, "戦士")
		},
	)

	var successCount, alreadyInGuildCount int
	for _, err := range []error{err1, err2} {
		if err == nil {
			successCount++
		} else if errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			alreadyInGuildCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successCount != 1 || alreadyInGuildCount != 1 {
		t.Fatalf("expected 1 success and 1 ErrCharacterAlreadyInGuild, got %d successes and %d alreadyInGuild (err1=%v, err2=%v)",
			successCount, alreadyInGuildCount, err1, err2)
	}

	// Invariant: active affiliations must be exactly 1
	if activeCount := f.countActiveAffiliations(t, target.ID); activeCount != 1 {
		t.Errorf("active affiliation count = %d, want 1", activeCount)
	}

	// Invariant: if Create won, money is 15000; if Approve won, money is 20000
	currentMoney := f.getCharacterMoney(t, target.ID)
	if err1 == nil {
		// Create won
		if currentMoney != 15000 {
			t.Errorf("Create won: expected money 15000, got %d", currentMoney)
		}
		if guildsLed := f.countGuildsLed(t, target.ID); guildsLed != 1 {
			t.Errorf("Create won: expected 1 guild led, got %d", guildsLed)
		}
	} else {
		// Approve won
		if currentMoney != 20000 {
			t.Errorf("Approve won: expected money 20000, got %d", currentMoney)
		}
		if guildsLed := f.countGuildsLed(t, target.ID); guildsLed != 0 {
			t.Errorf("Approve won: expected 0 guilds led, got %d", guildsLed)
		}
	}
}

func TestGuildActiveAffiliation_JoinVsCreate(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	leader := f.createCharacter(t, "JvCLead", 20000)
	target := f.createCharacter(t, "JvCTarget", 20000)

	g1, _, _, err := f.service.Create(ctx, leader.ID, "JvCG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	newGuildName := "JvCNew_" + id.New()[:8]

	// Target joins G1 directly while concurrently creating a new guild
	err1, err2 := testutil.RunRace2(
		func() error {
			_, err := f.service.Join(ctx, g1.ID, target.ID)
			return err
		},
		func() error {
			_, _, _, err := f.service.Create(ctx, target.ID, newGuildName)
			return err
		},
	)

	var successCount, alreadyInGuildCount int
	for _, err := range []error{err1, err2} {
		if err == nil {
			successCount++
		} else if errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			alreadyInGuildCount++
		} else {
			t.Errorf("unexpected error: %v", err)
		}
	}
	if successCount != 1 || alreadyInGuildCount != 1 {
		t.Fatalf("expected 1 success and 1 ErrCharacterAlreadyInGuild, got %d successes and %d alreadyInGuild (err1=%v, err2=%v)",
			successCount, alreadyInGuildCount, err1, err2)
	}

	// Invariant: active affiliations must be exactly 1
	if activeCount := f.countActiveAffiliations(t, target.ID); activeCount != 1 {
		t.Errorf("active affiliation count = %d, want 1", activeCount)
	}
}

func TestGuildActiveAffiliation_PendingSuccessionDisband(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leader1 := f.createCharacter(t, "SDLead1", 20000)
	leader2 := f.createCharacter(t, "SDLead2", 20000)
	applicant := f.createCharacter(t, "SDApp", 5000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "SDG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	g2, _, _, err := f.service.Create(ctx, leader2.ID, "SDG2_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// Applicant applies to both G1 and G2
	if err := f.service.ApplyToJoin(ctx, g1.ID, applicant.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, g2.ID, applicant.ID); err != nil {
		t.Fatal(err)
	}

	// Leader2 approves applicant in G2 -> applicant is now active in G2
	if err := f.service.ApproveApplication(ctx, g2.ID, leader2.ID, applicant.ID, "精鋭"); err != nil {
		t.Fatal(err)
	}
	if count := f.countActiveAffiliations(t, applicant.ID); count != 1 {
		t.Fatalf("expected 1 active affiliation after approval, got %d", count)
	}

	// Now Leader1 leaves G1. The only other member in G1 is applicant (who is active in G2).
	// G1 cannot promote applicant to leader, so G1 must be disbanded cleanly!
	if err := f.service.Leave(ctx, g1.ID, leader1.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	// G1 must be disbanded (not found)
	if _, err := f.service.Get(ctx, g1.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected G1 to be disbanded (ErrGuildNotFound), got %v", err)
	}

	// Applicant must still have exactly 1 active affiliation (in G2)
	if count := f.countActiveAffiliations(t, applicant.ID); count != 1 {
		t.Errorf("expected applicant active affiliation count = 1, got %d", count)
	}
	activeG, _, err := f.service.GetByCharacter(ctx, applicant.ID)
	if err != nil {
		t.Fatalf("GetByCharacter failed: %v", err)
	}
	if activeG.ID != g2.ID {
		t.Errorf("applicant active guild = %s, want %s (G2)", activeG.ID, g2.ID)
	}
}

func TestGuildActiveAffiliation_PendingSuccessionFallback(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx := context.Background()

	leader1 := f.createCharacter(t, "SFLead1", 20000)
	leader2 := f.createCharacter(t, "SFLead2", 20000)
	appDisqualified := f.createCharacter(t, "SFDisq", 5000)
	appEligible := f.createCharacter(t, "SFElig", 5000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "SFG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	g2, _, _, err := f.service.Create(ctx, leader2.ID, "SFG2_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// Both applicants apply to G1
	if err := f.service.ApplyToJoin(ctx, g1.ID, appDisqualified.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApplyToJoin(ctx, g1.ID, appEligible.ID); err != nil {
		t.Fatal(err)
	}

	// appDisqualified also applies to G2 and gets approved in G2
	if err := f.service.ApplyToJoin(ctx, g2.ID, appDisqualified.ID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.ApproveApplication(ctx, g2.ID, leader2.ID, appDisqualified.ID, "戦士"); err != nil {
		t.Fatal(err)
	}

	// Now Leader1 leaves G1.
	// appDisqualified is already active in G2 and cannot succeed.
	// appEligible should be chosen as successor and formalized!
	if err := f.service.Leave(ctx, g1.ID, leader1.ID); err != nil {
		t.Fatalf("Leave failed: %v", err)
	}

	// G1 must survive with appEligible as leader
	g1Detail, err := f.service.Get(ctx, g1.ID)
	if err != nil {
		t.Fatalf("expected G1 to survive: %v", err)
	}
	if g1Detail.Guild.LeaderCharacterID != appEligible.ID {
		t.Errorf("G1 leader = %s, want %s (appEligible)", g1Detail.Guild.LeaderCharacterID, appEligible.ID)
	}

	// Both characters must have exactly 1 active affiliation
	if count := f.countActiveAffiliations(t, appDisqualified.ID); count != 1 {
		t.Errorf("appDisqualified active count = %d, want 1", count)
	}
	if count := f.countActiveAffiliations(t, appEligible.ID); count != 1 {
		t.Errorf("appEligible active count = %d, want 1", count)
	}
}
