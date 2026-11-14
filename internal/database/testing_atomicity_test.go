package database

import (
	"context"
	"database/sql"
	"errors"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/guild"
)

func factoryIsolatedDB(t *testing.T, extraTables ...string) *sql.DB {
	t.Helper()
	db, _ := playerIsolatedDB(t)
	source, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer source.Close()
	for _, table := range extraTables {
		var name, ddl string
		if err := source.QueryRow("SHOW CREATE TABLE `"+table+"`").Scan(&name, &ddl); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ddl); err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func characterFactoryDB(t *testing.T) *sql.DB {
	t.Helper()
	return factoryIsolatedDB(t, "characters", "character_depots")
}

func guildFactoryDB(t *testing.T) *sql.DB {
	t.Helper()
	return factoryIsolatedDB(t, "characters", "character_depots", "guilds", "guild_members")
}

func assertTableRows(t *testing.T, ctx context.Context, db *sql.DB, want int, tables ...string) {
	t.Helper()
	for _, table := range tables {
		var got int
		if err := ExecutorFromContext(ctx, db).QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("%s rows = %d, want %d", table, got, want)
		}
	}
}

func assertFactoryRows(t *testing.T, ctx context.Context, db *sql.DB, want int) {
	t.Helper()
	assertTableRows(t, ctx, db, want, "players", "characters", "character_depots")
}

func TestCharacterFactoryRequiresDatabase(t *testing.T) {
	c, err := CreateTestCharacterWithFunds(context.Background(), nil, "NoDB", 1)
	if err == nil || c.ID != "" || c.PlayerID != "" {
		t.Fatalf("nil DB returned character=%+v error=%v", c, err)
	}
}

func TestCharacterFactoryRollsBackSaveFailures(t *testing.T) {
	for _, table := range []string{"characters", "character_depots"} {
		t.Run(table, func(t *testing.T) {
			db := characterFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			sentinel, err := CreateTestCharacterWithFunds(ctx, db, "Sentinel", 123)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_factory BEFORE INSERT ON `+table+`
				FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected factory failure'`); err != nil {
				t.Fatal(err)
			}
			for _, ambient := range []bool{false, true} {
				create := func(callCtx context.Context) error {
					c, err := CreateTestCharacterWithFunds(callCtx, db, "Failure", 7777)
					if c.ID != "" || c.PlayerID != "" {
						t.Errorf("failed factory returned partial character: %+v", c)
					}
					return err
				}
				if ambient {
					err = RunInTx(ctx, db, create)
				} else {
					err = create(ctx)
				}
				var injected *mysql.MySQLError
				if !errors.As(err, &injected) || injected.Number != 1644 || injected.Message != "injected factory failure" {
					t.Fatalf("ambient=%v: got %v, want original injected SQL error", ambient, err)
				}
				assertFactoryRows(t, ctx, db, 1)
				var owner string
				var money int
				if err := db.QueryRowContext(ctx, `SELECT c.player_id, c.money FROM characters c
					JOIN players p ON p.id = c.player_id
					JOIN character_depots d ON d.character_id = c.id
					WHERE c.id = ?`, sentinel.ID).Scan(&owner, &money); err != nil {
					t.Fatal(err)
				}
				if owner != sentinel.PlayerID || money != sentinel.Money {
					t.Fatal("unrelated sentinel changed")
				}
			}
		})
	}
}

func TestCharacterFactoryPreservesNameRetries(t *testing.T) {
	db := characterFactoryDB(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	a, err := CreateTestCharacterWithFunds(ctx, db, "Factory", 123)
	if err != nil {
		t.Fatal(err)
	}
	b, err := CreateTestCharacterWithFunds(ctx, db, a.Name, 7777)
	if err != nil {
		t.Fatal(err)
	}
	if a.Name != "Factory" || b.Name == a.Name || b.Money != 7777 || a.PlayerID == b.PlayerID {
		t.Fatalf("unexpected fixtures: %+v, %+v", a, b)
	}
	assertFactoryRows(t, ctx, db, 2)
	// Session variables survive rollback, so they independently count failed attempts.
	db.SetMaxOpenConns(1)
	db.SetConnMaxLifetime(0)
	db.SetConnMaxIdleTime(0)
	if _, err := db.ExecContext(ctx, `CREATE TRIGGER exhaust_factory_names BEFORE INSERT ON characters
		FOR EACH ROW BEGIN
		SET @fixture_attempts = COALESCE(@fixture_attempts, 0) + 1;
		SIGNAL SQLSTATE '23000' SET MYSQL_ERRNO = 1062, MESSAGE_TEXT = 'Duplicate entry for key ''uq_characters_name''';
		END`); err != nil {
		t.Fatal(err)
	}
	if _, err := CreateTestCharacterWithFunds(ctx, db, "Retry", 1); !errors.Is(err, character.ErrNameAlreadyTaken) {
		t.Fatalf("exhausted retries = %v, want name conflict", err)
	}
	var attempts int
	if err := db.QueryRowContext(ctx, `SELECT @fixture_attempts`).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if attempts != 3 {
		t.Fatalf("save attempts = %d, want 3", attempts)
	}
	assertFactoryRows(t, ctx, db, 2)
}

func TestCharacterFactoryPreservesAmbientTransaction(t *testing.T) {
	for _, mode := range []string{"rollback", "commit"} {
		t.Run(mode, func(t *testing.T) {
			commit := mode == "commit"
			db := characterFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			rollback := errors.New("outer rollback")
			var c corecharacter.Character
			err := RunInTx(ctx, db, func(txCtx context.Context) error {
				var err error
				c, err = CreateTestCharacterWithFunds(txCtx, db, "Ambient", 456)
				if err != nil {
					return err
				}
				assertFactoryRows(t, txCtx, db, 1)
				if !commit {
					return rollback
				}
				return nil
			})
			wantRows := 0
			if commit {
				wantRows = 1
				if err != nil {
					t.Fatal(err)
				}
				var money int
				if err := db.QueryRowContext(ctx, `SELECT money FROM characters WHERE id = ?`, c.ID).Scan(&money); err != nil || money != 456 {
					t.Fatalf("committed character money=%d error=%v", money, err)
				}
			} else if !errors.Is(err, rollback) {
				t.Fatalf("outer rollback = %v", err)
			}
			assertFactoryRows(t, ctx, db, wantRows)
		})
	}
}

func assertGuildFactoryRows(t *testing.T, ctx context.Context, db *sql.DB, wantLeaderAssets, wantGuilds int) {
	t.Helper()
	assertTableRows(t, ctx, db, wantLeaderAssets, "players", "characters", "character_depots")
	assertTableRows(t, ctx, db, wantGuilds, "guilds", "guild_members")
}

func assertGuildFixturesUnchanged(t *testing.T, ctx context.Context, db *sql.DB, sentinel corecharacter.Character, existingGuild guild.Guild, existingLeader corecharacter.Character) {
	t.Helper()
	var owner string
	var money int
	if err := db.QueryRowContext(ctx, `SELECT c.player_id, c.money FROM characters c
		JOIN players p ON p.id = c.player_id
		JOIN character_depots d ON d.character_id = c.id
		WHERE c.id = ?`, sentinel.ID).Scan(&owner, &money); err != nil {
		t.Fatal(err)
	}
	if owner != sentinel.PlayerID || money != sentinel.Money {
		t.Fatalf("unrelated sentinel changed: got owner=%s money=%d, want owner=%s money=%d", owner, money, sentinel.PlayerID, sentinel.Money)
	}

	var guildName, leaderID, memberRole string
	var leaderMoney int
	if err := db.QueryRowContext(ctx, `SELECT g.name, g.leader_character_id, gm.role, c.money
		FROM guilds g
		JOIN guild_members gm ON gm.guild_id = g.id AND gm.character_id = g.leader_character_id
		JOIN characters c ON c.id = g.leader_character_id
		JOIN players p ON p.id = c.player_id
		JOIN character_depots d ON d.character_id = c.id
		WHERE g.id = ?`, existingGuild.ID).Scan(&guildName, &leaderID, &memberRole, &leaderMoney); err != nil {
		t.Fatal(err)
	}
	if guildName != existingGuild.Name || leaderID != existingLeader.ID || memberRole != string(guild.RoleLeader) || leaderMoney != existingLeader.Money {
		t.Fatalf("existing guild changed: got name=%s leader=%s role=%s money=%d", guildName, leaderID, memberRole, leaderMoney)
	}
}

func TestGuildFactoryRequiresDatabase(t *testing.T) {
	g, c, err := CreateTestGuildWithLeader(context.Background(), nil, "NoDB", 1)
	if err == nil || g.ID != "" || c.ID != "" || c.PlayerID != "" {
		t.Fatalf("nil DB returned guild=%+v leader=%+v error=%v", g, c, err)
	}
}

func TestGuildFactoryRollsBackFailures(t *testing.T) {
	cases := []struct {
		name      string
		guildName string
		setup     func(t *testing.T, ctx context.Context, db *sql.DB)
		checkErr  func(t *testing.T, ambient bool, err error)
	}{
		{
			name:      "duplicate_guild_name",
			guildName: "DuplicateGuild",
			checkErr: func(t *testing.T, ambient bool, err error) {
				if !errors.Is(err, guild.ErrGuildNameTaken) {
					t.Fatalf("ambient=%v: got %v, want %v", ambient, err, guild.ErrGuildNameTaken)
				}
			},
		},
		{
			name:      "guild_members_insert_failure",
			guildName: "FailMember",
			setup: func(t *testing.T, ctx context.Context, db *sql.DB) {
				if _, err := db.ExecContext(ctx, `CREATE TRIGGER fail_guild_member_factory BEFORE INSERT ON guild_members
					FOR EACH ROW SIGNAL SQLSTATE '45000' SET MESSAGE_TEXT = 'injected member failure'`); err != nil {
					t.Fatal(err)
				}
			},
			checkErr: func(t *testing.T, ambient bool, err error) {
				var injected *mysql.MySQLError
				if !errors.As(err, &injected) || injected.Number != 1644 || injected.Message != "injected member failure" {
					t.Fatalf("ambient=%v: got %v, want original injected SQL error", ambient, err)
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := guildFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()

			sentinel, err := CreateTestCharacterWithFunds(ctx, db, "Sentinel", 123)
			if err != nil {
				t.Fatal(err)
			}
			existingGuild, existingLeader, err := CreateTestGuildWithLeader(ctx, db, "DuplicateGuild", 15000)
			if err != nil {
				t.Fatal(err)
			}
			if tc.setup != nil {
				tc.setup(t, ctx, db)
			}

			for _, ambient := range []bool{false, true} {
				create := func(callCtx context.Context) error {
					g, c, err := CreateTestGuildWithLeader(callCtx, db, tc.guildName, 7777)
					if g.ID != "" || g.Name != "" || c.ID != "" || c.PlayerID != "" {
						t.Errorf("failed guild factory returned partial result: guild=%+v leader=%+v", g, c)
					}
					return err
				}
				if ambient {
					err = RunInTx(ctx, db, create)
				} else {
					err = create(ctx)
				}
				tc.checkErr(t, ambient, err)
				assertGuildFactoryRows(t, ctx, db, 2, 1)
				assertGuildFixturesUnchanged(t, ctx, db, sentinel, existingGuild, existingLeader)
			}
		})
	}
}

func TestGuildFactoryPreservesBehaviorAndAmbientTransaction(t *testing.T) {
	t.Run("explicit_name_and_leader_retry", func(t *testing.T) {
		db := guildFactoryDB(t)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()

		g1, l1, err := CreateTestGuildWithLeader(ctx, db, "AlphaGuild", 1111)
		if err != nil {
			t.Fatal(err)
		}
		// AlphaGui2_Ldr truncates to "AlphaGui" just like AlphaGuild_Ldr, exercising character name retry.
		g2, l2, err := CreateTestGuildWithLeader(ctx, db, "AlphaGui2", 2222)
		if err != nil {
			t.Fatal(err)
		}
		if g1.Name != "AlphaGuild" || g2.Name != "AlphaGui2" || l1.Name != "AlphaGui" || l2.Name == l1.Name || l2.Money != 2222 {
			t.Fatalf("unexpected guilds/leaders: g1=%+v l1=%+v g2=%+v l2=%+v", g1, l1, g2, l2)
		}
		assertGuildFactoryRows(t, ctx, db, 2, 2)
	})

	for _, mode := range []string{"rollback", "commit"} {
		t.Run("ambient_"+mode, func(t *testing.T) {
			commit := mode == "commit"
			db := guildFactoryDB(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			rollback := errors.New("outer rollback")
			var g guild.Guild
			var l corecharacter.Character
			err := RunInTx(ctx, db, func(txCtx context.Context) error {
				var err error
				g, l, err = CreateTestGuildWithLeader(txCtx, db, "AmbientGuild", 456)
				if err != nil {
					return err
				}
				assertGuildFactoryRows(t, txCtx, db, 1, 1)
				if !commit {
					return rollback
				}
				return nil
			})
			wantRows := 0
			if commit {
				wantRows = 1
				if err != nil {
					t.Fatal(err)
				}
				var gName string
				var money int
				if err := db.QueryRowContext(ctx, `SELECT g.name, c.money FROM guilds g
					JOIN characters c ON c.id = g.leader_character_id
					WHERE g.id = ? AND c.id = ?`, g.ID, l.ID).Scan(&gName, &money); err != nil || gName != "AmbientGuild" || money != 456 {
					t.Fatalf("committed guild name=%q money=%d error=%v", gName, money, err)
				}
			} else if !errors.Is(err, rollback) {
				t.Fatalf("outer rollback = %v", err)
			}
			assertGuildFactoryRows(t, ctx, db, wantRows, wantRows)
		})
	}
}
