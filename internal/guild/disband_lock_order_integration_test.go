package guild_test

import (
	"context"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
)

// Disbanding writes the leader's trajectory entry, whose foreign key locks the
// leader's character row. A character action holds that row (Rank 2) and then
// takes the guild (Rank 7), so Disband must take the character first too.
func TestGuildDisband_LocksLeaderBeforeGuild_RealDB(t *testing.T) {
	db := guildIsolatedDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	memories, err := database.NewCharacterMemoryRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	service, err := guild.NewService(guildRepo,
		guild.WithTransactionProvider(database.NewTransactionProvider(db)),
		guild.WithMemoryRecorder(memories.AppendMemory))
	if err != nil {
		t.Fatal(err)
	}
	leader, err := database.CreateTestCharacterWithFunds(ctx, db, "LockLdr", guild.CreationFee)
	if err != nil {
		t.Fatal(err)
	}
	g, _, _, err := service.Create(ctx, leader.ID, "LockOrder")
	if err != nil {
		t.Fatal(err)
	}

	// The holder is a character action: it owns the leader's row and will want the guild.
	holder, err := db.Conn(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	var holderThread int64
	if err := holder.QueryRowContext(ctx, "SELECT CONNECTION_ID()").Scan(&holderThread); err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{"SET SESSION innodb_lock_wait_timeout = 10", "BEGIN"} {
		if _, err := holder.ExecContext(ctx, stmt); err != nil {
			t.Fatal(err)
		}
	}
	defer func() { _, _ = holder.ExecContext(context.Background(), "ROLLBACK") }()
	if _, err := holder.ExecContext(ctx, "SELECT id FROM characters WHERE id = ? FOR UPDATE", leader.ID); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- service.Disband(ctx, g.ID, leader.ID) }()

	// Wait until Disband is stuck on a statement; this dedicated database has no other activity.
	for blocked := 0; blocked == 0; {
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.PROCESSLIST
			WHERE DB = DATABASE() AND ID NOT IN (?, CONNECTION_ID()) AND INFO IS NOT NULL AND TIME_MS >= 300`,
			holderThread).Scan(&blocked); err != nil {
			t.Fatal(err)
		}
		select {
		case err := <-done:
			t.Fatalf("Disband finished while the leader's character row was held: %v", err)
		case <-time.After(50 * time.Millisecond):
		}
	}

	if _, err := holder.ExecContext(ctx, "SELECT id FROM guilds WHERE id = ? FOR UPDATE", g.ID); err != nil {
		t.Fatalf("the character holder could not take the guild; Disband holds it out of rank order: %v", err)
	}
	if _, err := holder.ExecContext(ctx, "COMMIT"); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Disband after the holder released: %v", err)
	}
	got, err := memories.ListMemories(ctx, leader.ID, 1)
	if err != nil || len(got) != 1 || got[0].GuildName != "LockOrder" {
		t.Fatalf("disband entry = %+v, %v", got, err)
	}
}
