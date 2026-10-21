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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
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

func TestGuildActiveAffiliation_AssignCustomRoleConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader1 := f.createCharacter(t, "ARLead1", 20000)
	leader2 := f.createCharacter(t, "ARLead2", 20000)
	applicant := f.createCharacter(t, "ARTarget", 5000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "ARG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	g2, _, _, err := f.service.Create(ctx, leader2.ID, "ARG2_"+id.New()[:8])
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

	// Concurrent AssignCustomRole on pending applicant vs ApproveApplication (or AssignCustomRole on other guild)
	err1, err2 := testutil.RunRace2(
		func() error {
			return f.service.AssignCustomRole(ctx, g1.ID, leader1.ID, applicant.ID, "親衛隊長")
		},
		func() error {
			return f.service.AssignCustomRole(ctx, g2.ID, leader2.ID, applicant.ID, "魔導将軍")
		},
	)

	var successCount, alreadyInGuildCount int
	for _, err := range []error{err1, err2} {
		if err == nil {
			successCount++
		} else if errors.Is(err, guild.ErrCharacterAlreadyInGuild) {
			alreadyInGuildCount++
		} else {
			t.Errorf("unexpected error from AssignCustomRole: %v", err)
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

func TestGuildActiveAffiliation_LeaveSuccessionVsApprovalConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader1 := f.createCharacter(t, "LSLead1", 20000)
	leader2 := f.createCharacter(t, "LSLead2", 20000)
	applicant := f.createCharacter(t, "LSTarget", 5000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "LSG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	g2, _, _, err := f.service.Create(ctx, leader2.ID, "LSG2_"+id.New()[:8])
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

	// Leader1 leaves G1 (attempting to promote applicant as sole remaining member)
	// while Leader2 concurrently approves applicant in G2!
	errLeave, errApprove := testutil.RunRace2(
		func() error {
			return f.service.Leave(ctx, g1.ID, leader1.ID)
		},
		func() error {
			return f.service.ApproveApplication(ctx, g2.ID, leader2.ID, applicant.ID, "精鋭")
		},
	)

	// Leave should always succeed (either promoting applicant to leader, or disbanding G1 if applicant is in G2).
	if errLeave != nil {
		t.Fatalf("unexpected error from Leave: %v", errLeave)
	}

	// ApproveApplication either succeeds (if it won race), or fails with ErrCharacterAlreadyInGuild (if Leave promoted applicant first).
	if errApprove != nil && !errors.Is(errApprove, guild.ErrCharacterAlreadyInGuild) {
		t.Fatalf("unexpected error from ApproveApplication: %v", errApprove)
	}

	// Invariant: active affiliations for applicant must be exactly 1 under all race outcomes
	if activeCount := f.countActiveAffiliations(t, applicant.ID); activeCount != 1 {
		t.Errorf("active affiliation count for applicant = %d, want 1", activeCount)
	}

	// If ApproveApplication won, applicant is in G2 and G1 is disbanded.
	// If Leave won, applicant is leader of G1 and ApproveApplication failed.
	if errApprove == nil {
		// Approve won: G2 active, G1 disbanded
		activeG, _, err := f.service.GetByCharacter(ctx, applicant.ID)
		if err != nil {
			t.Fatalf("GetByCharacter failed: %v", err)
		}
		if activeG.ID != g2.ID {
			t.Errorf("applicant active guild = %s, want %s (G2)", activeG.ID, g2.ID)
		}
		if _, err := f.service.Get(ctx, g1.ID); !errors.Is(err, guild.ErrGuildNotFound) {
			t.Errorf("expected G1 to be disbanded, got err: %v", err)
		}
	} else {
		// Leave won: applicant is leader of G1
		g1Detail, err := f.service.Get(ctx, g1.ID)
		if err != nil {
			t.Fatalf("expected G1 to survive: %v", err)
		}
		if g1Detail.Guild.LeaderCharacterID != applicant.ID {
			t.Errorf("G1 leader = %s, want %s", g1Detail.Guild.LeaderCharacterID, applicant.ID)
		}
	}
}

func TestGuildActiveAffiliation_LeaveSuccessionVsCreateConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader1 := f.createCharacter(t, "LCLead1", 20000)
	successor := f.createCharacter(t, "LCTarget", 20000)

	g1, _, _, err := f.service.Create(ctx, leader1.ID, "LCG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	// Successor submits pending application to G1
	if err := f.service.ApplyToJoin(ctx, g1.ID, successor.ID); err != nil {
		t.Fatal(err)
	}

	newGuildName := "LCNew_" + id.New()[:8]

	// Leader1 leaves G1 (attempting to promote successor) while successor concurrently calls Create!
	errLeave, errCreate := testutil.RunRace2(
		func() error {
			return f.service.Leave(ctx, g1.ID, leader1.ID)
		},
		func() error {
			_, _, _, err := f.service.Create(ctx, successor.ID, newGuildName)
			return err
		},
	)

	if errLeave != nil {
		t.Fatalf("unexpected error from Leave: %v", errLeave)
	}
	if errCreate != nil && !errors.Is(errCreate, guild.ErrCharacterAlreadyInGuild) {
		t.Fatalf("unexpected error from Create: %v", errCreate)
	}

	// Invariant: active affiliations for successor must be exactly 1 under all race outcomes
	if activeCount := f.countActiveAffiliations(t, successor.ID); activeCount != 1 {
		t.Errorf("active affiliation count for successor = %d, want 1", activeCount)
	}

	// Exactly 1 guild is led by successor
	if guildsLed := f.countGuildsLed(t, successor.ID); guildsLed != 1 {
		t.Errorf("guilds led by successor = %d, want 1", guildsLed)
	}
}

func TestGuildActiveAffiliation_SoleLeaderLeaveVsJoinConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader := f.createCharacter(t, "LJLead", 20000)
	joiner := f.createCharacter(t, "LJJoiner", 5000)

	g1, _, _, err := f.service.Create(ctx, leader.ID, "LJG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	errLeave, errJoin := testutil.RunRace2(
		func() error {
			return f.service.Leave(ctx, g1.ID, leader.ID)
		},
		func() error {
			_, err := f.service.Join(ctx, g1.ID, joiner.ID)
			return err
		},
	)

	if errLeave != nil {
		t.Fatalf("unexpected error from Leave: %v", errLeave)
	}

	if errJoin != nil && !errors.Is(errJoin, guild.ErrGuildNotFound) {
		t.Fatalf("unexpected error from Join: %v", errJoin)
	}

	if errJoin == nil {
		// Join succeeded: G1 MUST NOT be disbanded!
		// Invariant: joiner must be active member and leader of G1!
		if activeCount := f.countActiveAffiliations(t, joiner.ID); activeCount != 1 {
			t.Errorf("active affiliation count for joiner = %d, want 1", activeCount)
		}
		g1Detail, err := f.service.Get(ctx, g1.ID)
		if err != nil {
			t.Fatalf("expected G1 to survive when Join succeeded, got error: %v", err)
		}
		if g1Detail.Guild.LeaderCharacterID != joiner.ID {
			t.Errorf("G1 leader = %s, want %s (joiner)", g1Detail.Guild.LeaderCharacterID, joiner.ID)
		}
	} else {
		// Join failed because G1 was disbanded first:
		// G1 must be disbanded, joiner must have 0 affiliations.
		if activeCount := f.countActiveAffiliations(t, joiner.ID); activeCount != 0 {
			t.Errorf("active affiliation count for joiner = %d, want 0", activeCount)
		}
		if _, err := f.service.Get(ctx, g1.ID); !errors.Is(err, guild.ErrGuildNotFound) {
			t.Errorf("expected G1 to be disbanded, got err: %v", err)
		}
	}

	// Leader must have 0 active affiliations in both cases
	if activeCount := f.countActiveAffiliations(t, leader.ID); activeCount != 0 {
		t.Errorf("active affiliation count for leader = %d, want 0", activeCount)
	}
}

func TestGuildActiveAffiliation_LeaderLeaveVsCandidateDepartureConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader := f.createCharacter(t, "LLCLead", 20000)
	member := f.createCharacter(t, "LLCMem", 5000)

	g1, _, _, err := f.service.Create(ctx, leader.ID, "LLCG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Join(ctx, g1.ID, member.ID); err != nil {
		t.Fatal(err)
	}

	// Both leader and member attempt to leave concurrently
	errLeave1, errLeave2 := testutil.RunRace2(
		func() error {
			return f.service.Leave(ctx, g1.ID, leader.ID)
		},
		func() error {
			return f.service.Leave(ctx, g1.ID, member.ID)
		},
	)

	// Both leaves must succeed without deadlock
	if errLeave1 != nil {
		t.Fatalf("unexpected error from leader Leave: %v", errLeave1)
	}
	if errLeave2 != nil {
		t.Fatalf("unexpected error from member Leave: %v", errLeave2)
	}

	// Guild must be disbanded cleanly (since both departed)
	if _, err := f.service.Get(ctx, g1.ID); !errors.Is(err, guild.ErrGuildNotFound) {
		t.Errorf("expected G1 to be disbanded after all members departed, got err: %v", err)
	}

	// Both characters must have 0 active affiliations
	if activeCount := f.countActiveAffiliations(t, leader.ID); activeCount != 0 {
		t.Errorf("active affiliation count for leader = %d, want 0", activeCount)
	}
	if activeCount := f.countActiveAffiliations(t, member.ID); activeCount != 0 {
		t.Errorf("active affiliation count for member = %d, want 0", activeCount)
	}
}

func TestGuildActiveAffiliation_LeaderLeaveVsCandidateRosterChangesConcurrency(t *testing.T) {
	f := newActiveAffiliationFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	leader := f.createCharacter(t, "LRCLead", 20000)
	candidate1 := f.createCharacter(t, "LRCCand1", 5000)
	candidate2 := f.createCharacter(t, "LRCCand2", 5000)

	g1, _, _, err := f.service.Create(ctx, leader.ID, "LRCG1_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}

	if _, err := f.service.Join(ctx, g1.ID, candidate1.ID); err != nil {
		t.Fatal(err)
	}

	// 3 concurrent operations:
	// 1: Leader leaves G1
	// 2: Candidate1 leaves G1
	// 3: Candidate2 joins G1
	errs := testutil.RunRace(
		func() error {
			return f.service.Leave(ctx, g1.ID, leader.ID)
		},
		func() error {
			return f.service.Leave(ctx, g1.ID, candidate1.ID)
		},
		func() error {
			_, err := f.service.Join(ctx, g1.ID, candidate2.ID)
			return err
		},
	)

	errLeaveLeader := errs[0]
	errLeaveCand1 := errs[1]
	errJoinCand2 := errs[2]

	if errLeaveLeader != nil {
		t.Fatalf("unexpected error from leader Leave: %v", errLeaveLeader)
	}
	if errLeaveCand1 != nil {
		t.Fatalf("unexpected error from candidate1 Leave: %v", errLeaveCand1)
	}
	if errJoinCand2 != nil && !errors.Is(errJoinCand2, guild.ErrGuildNotFound) {
		t.Fatalf("unexpected error from candidate2 Join: %v", errJoinCand2)
	}

	// Invariants:
	// Leader and Candidate1 must always have 0 affiliations
	if c := f.countActiveAffiliations(t, leader.ID); c != 0 {
		t.Errorf("active affiliation count for leader = %d, want 0", c)
	}
	if c := f.countActiveAffiliations(t, candidate1.ID); c != 0 {
		t.Errorf("active affiliation count for candidate1 = %d, want 0", c)
	}

	// If Candidate2 joined successfully:
	if errJoinCand2 == nil {
		// Guild must survive and Candidate2 must be active member and leader
		if c := f.countActiveAffiliations(t, candidate2.ID); c != 1 {
			t.Errorf("active affiliation count for candidate2 = %d, want 1", c)
		}
		detail, err := f.service.Get(ctx, g1.ID)
		if err != nil {
			t.Fatalf("expected G1 to exist, got err: %v", err)
		}
		if detail.Guild.LeaderCharacterID != candidate2.ID {
			t.Errorf("G1 leader = %s, want %s (candidate2)", detail.Guild.LeaderCharacterID, candidate2.ID)
		}
	} else {
		// Candidate2 join failed because G1 was disbanded:
		if c := f.countActiveAffiliations(t, candidate2.ID); c != 0 {
			t.Errorf("active affiliation count for candidate2 = %d, want 0", c)
		}
		if _, err := f.service.Get(ctx, g1.ID); !errors.Is(err, guild.ErrGuildNotFound) {
			t.Errorf("expected G1 to be disbanded, got err: %v", err)
		}
	}

	// Assert no orphaned members exist for this guild
	var orphanCount int
	err = f.db.QueryRow("SELECT COUNT(*) FROM guild_members WHERE guild_id = ? AND character_id NOT IN (?, ?, ?)",
		g1.ID, leader.ID, candidate1.ID, candidate2.ID).Scan(&orphanCount)
	if err != nil {
		t.Fatal(err)
	}
	if orphanCount != 0 {
		t.Errorf("found %d orphaned members in G1", orphanCount)
	}
}
