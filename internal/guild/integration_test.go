package guild_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
)

func TestGuildServiceDatabaseIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := guild.NewService(guildRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Create test characters
	leaderChar, err := database.CreateTestCharacter(ctx, db, "SvcGuildLeader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	memberChar1, err := database.CreateTestCharacter(ctx, db, "SvcGuildMember1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 15000, memberChar1.ID); err != nil {
		t.Fatal(err)
	}

	memberChar2, err := database.CreateTestCharacter(ctx, db, "SvcGuildMember2")
	if err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("Integ_%s", leaderChar.ID[:8])

	// 1. Create Guild
	g, leaderM, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("service.Create failed: %v", err)
	}
	if g.Name != guildName || leaderM.Role != guild.RoleLeader || leaderM.Title != guild.DefaultTitleLeader {
		t.Errorf("unexpected guild/member: %+v, %+v", g, leaderM)
	}
	if g.Points != 0 || g.Color != guild.DefaultColor {
		t.Errorf("expected points 0, color #FFFFFF, got points %d, color %s", g.Points, g.Color)
	}

	// 2. Member1 joins
	m1, err := service.Join(ctx, g.ID, memberChar1.ID)
	if err != nil {
		t.Fatalf("service.Join failed: %v", err)
	}
	if m1.Role != guild.RoleMember || m1.Title != "" {
		t.Errorf("member1 role = %v, title = %q; want member, ''", m1.Role, m1.Title)
	}

	// 3. Leader assigns custom role title to Member1 (あたえる: 親衛隊長)
	if err := service.AssignCustomRole(ctx, g.ID, leaderChar.ID, memberChar1.ID, "親衛隊長"); err != nil {
		t.Fatalf("service.AssignCustomRole failed: %v", err)
	}
	_, m1Updated, err := service.GetByCharacter(ctx, memberChar1.ID)
	if err != nil || m1Updated.Title != "親衛隊長" {
		t.Fatalf("expected title '親衛隊長', got %q, err = %v", m1Updated.Title, err)
	}

	// 4. Leader updates guild color (からー)
	testColor := fmt.Sprintf("#%06X", (time.Now().UnixNano()/1000)%0xFFFFFF)
	if testColor == guild.DefaultColor || testColor == guild.NPCColor {
		testColor = "#FF3333"
	}
	if err := service.UpdateColor(ctx, g.ID, leaderChar.ID, testColor); err != nil {
		t.Fatalf("service.UpdateColor failed: %v", err)
	}
	detail, err := service.Get(ctx, g.ID)
	if err != nil || detail.Guild.Color != testColor {
		t.Fatalf("expected color %s, got %s, err = %v", testColor, detail.Guild.Color, err)
	}

	// 5. Member2 joins
	_, err = service.Join(ctx, g.ID, memberChar2.ID)
	if err != nil {
		t.Fatalf("service.Join (member2) failed: %v", err)
	}

	// 6. Leader updates notice
	if err := service.UpdateNotice(ctx, g.ID, leaderChar.ID, "Leader notice update"); err != nil {
		t.Fatalf("service.UpdateNotice failed: %v", err)
	}

	// 7. Leader kicks Member2
	if err := service.Kick(ctx, g.ID, leaderChar.ID, memberChar2.ID); err != nil {
		t.Fatalf("service.Kick failed: %v", err)
	}

	// 8. Add Guild Points dynamically
	if err := service.AddGuildPoints(ctx, memberChar1.ID, 100); err != nil {
		t.Fatalf("service.AddGuildPoints failed: %v", err)
	}
	gWithPoints, _, err := service.GetByCharacter(ctx, leaderChar.ID)
	if err != nil || gWithPoints.Points != 100 {
		t.Fatalf("expected points 100, got %d, err = %v", gWithPoints.Points, err)
	}

	// 9. Transfer leadership from Leader to Member1
	if err := service.TransferLeadership(ctx, g.ID, leaderChar.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.TransferLeadership failed: %v", err)
	}

	// 10. Former leader leaves guild
	if err := service.Leave(ctx, g.ID, leaderChar.ID); err != nil {
		t.Fatalf("service.Leave failed: %v", err)
	}

	// 11. Sole leader (Member1) leaves -> disbands guild
	if err := service.Leave(ctx, g.ID, memberChar1.ID); err != nil {
		t.Fatalf("service.Leave (sole leader disband) failed: %v", err)
	}

	// 12. Verify guild is deleted
	if _, err := service.Get(ctx, g.ID); err == nil {
		t.Error("expected error getting disbanded guild, got nil")
	}
}

func TestGuildService_PendingSuccessionIntegration(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	service, err := guild.NewService(guildRepo)
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := database.CreateTestCharacter(ctx, db, "SvcSuccLeader")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	applicantChar, err := database.CreateTestCharacter(ctx, db, "SvcSuccApplicant")
	if err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("IntegSucc_%s", leaderChar.ID[:8])

	// 1. Create Guild
	g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("service.Create failed: %v", err)
	}

	// 2. Applicant applies to join
	if err := service.ApplyToJoin(ctx, g.ID, applicantChar.ID); err != nil {
		t.Fatalf("service.ApplyToJoin failed: %v", err)
	}

	// 3. Leader leaves -> leadership transfers to applicant, guild preserved
	if err := service.Leave(ctx, g.ID, leaderChar.ID); err != nil {
		t.Fatalf("service.Leave (leader departure) failed: %v", err)
	}

	// 4. Verify guild is preserved and applicant is now leader
	gDetail, err := service.Get(ctx, g.ID)
	if err != nil {
		t.Fatalf("expected guild to be preserved, got error: %v", err)
	}
	if gDetail.Guild.LeaderCharacterID != applicantChar.ID {
		t.Errorf("leader = %q, want %q", gDetail.Guild.LeaderCharacterID, applicantChar.ID)
	}
	if len(gDetail.Members) != 1 {
		t.Fatalf("expected 1 remaining member, got %d", len(gDetail.Members))
	}
	newLeaderMember := gDetail.Members[0]
	if newLeaderMember.CharacterID != applicantChar.ID {
		t.Errorf("member = %q, want %q", newLeaderMember.CharacterID, applicantChar.ID)
	}
	if newLeaderMember.Role != guild.RoleLeader || newLeaderMember.Title != guild.DefaultTitleLeader || newLeaderMember.IsPending {
		t.Errorf("unexpected member state: role=%v, title=%q, isPending=%v", newLeaderMember.Role, newLeaderMember.Title, newLeaderMember.IsPending)
	}

	// 5. New leader leaves -> sole member left -> guild disbands
	if err := service.Leave(ctx, g.ID, applicantChar.ID); err != nil {
		t.Fatalf("service.Leave (sole leader departure) failed: %v", err)
	}

	if _, err := service.Get(ctx, g.ID); err == nil {
		t.Error("expected error getting disbanded guild, got nil")
	}
}

func TestGuildService_ConcurrentDepartureRace(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	txProvider := database.NewTransactionProvider(db)
	service, err := guild.NewService(guildRepo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	// Run multiple paired races to test concurrency interleavings
	const iterations = 5
	for iter := 0; iter < iterations; iter++ {
		leaderChar, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaceLdr_%d", iter))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
			t.Fatal(err)
		}

		applicantChar, err := database.CreateTestCharacter(ctx, db, fmt.Sprintf("RaceApp_%d", iter))
		if err != nil {
			t.Fatal(err)
		}

		guildName := fmt.Sprintf("RaceG_%s_%d", leaderChar.ID[:6], iter)
		g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
		if err != nil {
			t.Fatalf("Create failed: %v", err)
		}

		if err := service.ApplyToJoin(ctx, g.ID, applicantChar.ID); err != nil {
			t.Fatalf("ApplyToJoin failed: %v", err)
		}

		// Concurrently race leader leave and applicant leave
		err1, err2 := database.RunRace2(
			func() error {
				return service.Leave(ctx, g.ID, leaderChar.ID)
			},
			func() error {
				return service.Leave(ctx, g.ID, applicantChar.ID)
			},
		)

		if database.IsDeadlockError(err1) || database.IsDeadlockError(err2) {
			t.Fatalf("iter %d: DEADLOCK detected! err1=%v, err2=%v", iter, err1, err2)
		}

		// Invariant verification:
		// A surviving guild must have >= 1 member in guild_members, and its leader must be present in guild_members.
		// If 0 members remain, the guild MUST be disbanded (GetGuild returns ErrGuildNotFound).
		gDetail, err := service.Get(ctx, g.ID)
		if err == nil {
			// Guild survived
			if len(gDetail.Members) == 0 {
				t.Fatalf("iter %d: INVARIANT VIOLATION: surviving guild %s has 0 members in guild_members (empty guild pointing to leader %s)",
					iter, g.ID, gDetail.Guild.LeaderCharacterID)
			}
			var leaderInRoster bool
			for _, m := range gDetail.Members {
				if m.CharacterID == gDetail.Guild.LeaderCharacterID {
					leaderInRoster = true
					if m.Role != guild.RoleLeader {
						t.Errorf("iter %d: leader character %s has role %v, want RoleLeader", iter, m.CharacterID, m.Role)
					}
					break
				}
			}
			if !leaderInRoster {
				t.Fatalf("iter %d: INVARIANT VIOLATION: leader %s is absent from surviving guild roster",
					iter, gDetail.Guild.LeaderCharacterID)
			}
		} else if !errors.Is(err, guild.ErrGuildNotFound) {
			t.Fatalf("iter %d: unexpected Get error: %v", iter, err)
		}
	}
}

type interceptingGuildRepo struct {
	*database.GuildRepository
	onAfterGetGuildForUpdate func(ctx context.Context, guildID string)
}

func (r *interceptingGuildRepo) GetGuildForUpdate(ctx context.Context, guildID string) (guild.Guild, []guild.Member, error) {
	g, members, err := r.GuildRepository.GetGuildForUpdate(ctx, guildID)
	if err == nil && r.onAfterGetGuildForUpdate != nil {
		r.onAfterGetGuildForUpdate(ctx, guildID)
	}
	return g, members, err
}

func TestGuildService_DeterministicDepartureInterleaving(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	realRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}

	leaderLocked := make(chan struct{})
	resumeLeader := make(chan struct{})
	var leaderIntercepted atomic.Bool

	repo := &interceptingGuildRepo{
		GuildRepository: realRepo,
		onAfterGetGuildForUpdate: func(ctx context.Context, guildID string) {
			if leaderIntercepted.CompareAndSwap(false, true) {
				leaderLocked <- struct{}{}
				<-resumeLeader
			}
		},
	}

	txProvider := database.NewTransactionProvider(db)
	service, err := guild.NewService(repo, guild.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	leaderChar, err := database.CreateTestCharacter(ctx, db, "DetRaceLdr")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, "UPDATE characters SET money = ? WHERE id = ?", 20000, leaderChar.ID); err != nil {
		t.Fatal(err)
	}

	applicantChar, err := database.CreateTestCharacter(ctx, db, "DetRaceApp")
	if err != nil {
		t.Fatal(err)
	}

	guildName := fmt.Sprintf("DetRace_%s", leaderChar.ID[:8])
	g, _, _, err := service.Create(ctx, leaderChar.ID, guildName)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := service.ApplyToJoin(ctx, g.ID, applicantChar.ID); err != nil {
		t.Fatalf("ApplyToJoin failed: %v", err)
	}

	// 1. Leader starts Leave in background.
	// It will acquire SELECT ... FOR UPDATE on guilds and guild_members, then pause inside onAfterGetGuildForUpdate.
	leaderDone := make(chan error, 1)
	go func() {
		leaderDone <- service.Leave(ctx, g.ID, leaderChar.ID)
	}()

	// Wait until Leader holds the MariaDB row locks
	select {
	case <-leaderLocked:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for leader to acquire lock")
	}

	// 2. Applicant attempts Leave concurrently.
	// Applicant must block on GetGuildForUpdate because Leader holds exclusive row locks in the transaction.
	applicantDone := make(chan error, 1)
	go func() {
		applicantDone <- service.Leave(ctx, g.ID, applicantChar.ID)
	}()

	// Verify Applicant is blocked waiting for the row lock
	select {
	case res := <-applicantDone:
		t.Fatalf("Applicant Leave should have been blocked by Leader's row lock, but completed with %v", res)
	case <-time.After(150 * time.Millisecond):
		// Expected: Applicant is blocked on row lock
	}

	// 3. Resume Leader to complete transfer/removal and commit
	close(resumeLeader)

	// Leader finishes
	select {
	case lErr := <-leaderDone:
		if lErr != nil {
			t.Fatalf("leader Leave failed: %v", lErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for leader Leave to complete")
	}

	// Applicant now unblocks in MariaDB, reads updated guild state (Applicant is now leader), and leaves
	select {
	case aErr := <-applicantDone:
		if aErr != nil {
			t.Fatalf("applicant Leave failed: %v", aErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for applicant Leave to complete")
	}

	// Invariant verification:
	// Both departed -> guild must be cleanly disbanded, NOT left as an orphan pointing to the applicant!
	_, err = service.Get(ctx, g.ID)
	if !errors.Is(err, guild.ErrGuildNotFound) {
		t.Fatalf("expected guild to be disbanded (ErrGuildNotFound), got %v", err)
	}

	var memberCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM guild_members WHERE guild_id = ?", g.ID).Scan(&memberCount); err != nil {
		t.Fatal(err)
	}
	if memberCount != 0 {
		t.Fatalf("INVARIANT VIOLATION: expected 0 members in guild_members, got %d", memberCount)
	}

	var guildCount int
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM guilds WHERE id = ?", g.ID).Scan(&guildCount); err != nil {
		t.Fatal(err)
	}
	if guildCount != 0 {
		t.Fatalf("INVARIANT VIOLATION: expected 0 rows in guilds table, got %d (orphan guild exists!)", guildCount)
	}
}
