package player_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/home"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/notification"
	"github.com/witchcraze/party2re/internal/player"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errInjected = errors.New("injected deletion failure")

// failingCharacterRepo injects failures into the real repository.
type failingCharacterRepo struct {
	*database.CharacterRepository
	failDelete  string // character ID whose deletion fails after its SQL cleanup ran
	failListing bool
}

func (r *failingCharacterRepo) Delete(ctx context.Context, id string) error {
	if err := r.CharacterRepository.Delete(ctx, id); err != nil {
		return err
	}
	if id == r.failDelete {
		return errInjected
	}
	return nil
}

func (r *failingCharacterRepo) FindByPlayerID(ctx context.Context, playerID string) ([]corecharacter.Character, error) {
	if r.failListing {
		return nil, errInjected
	}
	return r.CharacterRepository.FindByPlayerID(ctx, playerID)
}

type failingPlayerRepo struct {
	*database.PlayerRepository
	failDelete bool
}

func (r *failingPlayerRepo) Delete(ctx context.Context, id string) error {
	if err := r.PlayerRepository.Delete(ctx, id); err != nil {
		return err
	}
	if r.failDelete {
		return errInjected
	}
	return nil
}

type deletionFixture struct {
	db       *sql.DB
	chars    *failingCharacterRepo
	players  *failingPlayerRepo
	guilds   *guild.Service
	charSvc  *character.Service
	accounts *player.Service
	tokens   *database.PlayerAPITokenRepository
}

func newDeletionFixture(t *testing.T) *deletionFixture {
	t.Helper()
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	tx := database.NewTransactionProvider(db)
	f := &deletionFixture{db: db}
	charRepo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.chars = &failingCharacterRepo{CharacterRepository: charRepo}
	playerRepo, err := database.NewPlayerRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	f.players = &failingPlayerRepo{PlayerRepository: playerRepo}
	if f.tokens, err = database.NewPlayerAPITokenRepository(db); err != nil {
		t.Fatal(err)
	}
	guildRepo, err := database.NewGuildRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	notifRepo, err := database.NewNotificationRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	news, err := notification.NewService(notifRepo, notifRepo)
	if err != nil {
		t.Fatal(err)
	}
	f.guilds, err = guild.NewService(guildRepo, guild.WithTransactionProvider(tx),
		guild.WithNewsPublisher(newsFunc(func(ctx context.Context, cat, title, content, author string, at time.Time) error {
			_, err := news.PublishNews(ctx, cat, title, content, author, at)
			return err
		})))
	if err != nil {
		t.Fatal(err)
	}
	f.charSvc, err = character.NewService(f.chars, character.WithTransactionProvider(tx),
		character.WithCleanupHook(character.CleanupHookFunc(f.guilds.RemoveFromGuilds)))
	if err != nil {
		t.Fatal(err)
	}
	f.accounts, err = player.NewService(f.players, player.NewValkeySessionRepository(nil),
		player.WithCharacterService(f.charSvc), player.WithAPITokenRepository(f.tokens), player.WithTransactionProvider(tx))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

type newsFunc func(ctx context.Context, cat, title, content, author string, at time.Time) error

func (f newsFunc) PublishNews(ctx context.Context, cat, title, content, author string, at time.Time) error {
	return f(ctx, cat, title, content, author, at)
}

// character creates a funded character, optionally owned by an existing player.
func (f *deletionFixture) character(t *testing.T, playerID string) corecharacter.Character {
	t.Helper()
	ctx := context.Background()
	c, err := database.CreateTestCharacterWithFunds(ctx, f.db, "Del", 100000)
	if err != nil {
		t.Fatal(err)
	}
	if playerID != "" {
		if _, err := f.db.ExecContext(ctx, `UPDATE characters SET player_id = ? WHERE id = ?`, playerID, c.ID); err != nil {
			t.Fatal(err)
		}
		c.PlayerID = playerID
	}
	it, err := coreitem.NewInstance("potion-01", 1)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.CreateTestInventoryWithItems(ctx, f.db, c.ID, []coreitem.Instance{it}); err != nil {
		t.Fatal(err)
	}
	return c
}

func (f *deletionFixture) createGuild(t *testing.T, leader corecharacter.Character, members ...corecharacter.Character) guild.Guild {
	t.Helper()
	ctx := context.Background()
	g, _, _, err := f.guilds.Create(ctx, leader.ID, "Del_"+id.New()[:8])
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range members {
		if err := f.guilds.ApplyToJoin(ctx, g.ID, m.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.guilds.ApproveApplication(ctx, g.ID, leader.ID, m.ID, "member"); err != nil {
			t.Fatal(err)
		}
	}
	return g
}

func (f *deletionFixture) count(t *testing.T, query string, args ...any) int {
	t.Helper()
	var n int
	if err := f.db.QueryRowContext(context.Background(), query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (f *deletionFixture) leader(t *testing.T, guildID string) string {
	t.Helper()
	var leader string
	err := f.db.QueryRowContext(context.Background(), `SELECT leader_character_id FROM guilds WHERE id = ?`, guildID).Scan(&leader)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return leader
}

func (f *deletionFixture) dissolutionNews(t *testing.T, g guild.Guild) int {
	return f.count(t, `SELECT COUNT(*) FROM news_articles WHERE title = ?`, "ギルド『"+g.Name+"』が解散しました")
}

// assertIntact verifies characters, their inventory and guild memberships are unchanged.
func (f *deletionFixture) assertIntact(t *testing.T, chars ...corecharacter.Character) {
	t.Helper()
	for _, c := range chars {
		if f.count(t, `SELECT COUNT(*) FROM characters WHERE id = ?`, c.ID) != 1 ||
			f.count(t, `SELECT COUNT(*) FROM inventory_items WHERE character_id = ?`, c.ID) == 0 {
			t.Fatalf("character %s or its inventory was not restored", c.ID)
		}
	}
}

// A failure anywhere in account deletion leaves the account, every character,
// their assets, guild leadership and API tokens as they were (#1095).
func TestAccountDeletionRollsBackOnFailure(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name   string
		inject func(f *deletionFixture, second string)
	}{
		{"second character fails", func(f *deletionFixture, second string) { f.chars.failDelete = second }},
		{"player delete fails", func(f *deletionFixture, _ string) { f.players.failDelete = true }},
		{"character listing fails", func(f *deletionFixture, _ string) { f.chars.failListing = true }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := newDeletionFixture(t)
			owner, err := database.CreateTestPlayer(ctx, f.db)
			if err != nil {
				t.Fatal(err)
			}
			a, b := f.character(t, owner.ID), f.character(t, owner.ID)
			other := f.character(t, "")
			solo := f.createGuild(t, a)          // A alone: deletion would disband it
			shared := f.createGuild(t, b, other) // B leads: deletion would hand over to other
			if _, _, err := f.accounts.CreateAPIToken(ctx, owner.ID, "keep", nil); err != nil {
				t.Fatal(err)
			}
			tc.inject(f, max(a.ID, b.ID))

			if err := f.accounts.DeleteAccount(ctx, owner.ID, ""); !errors.Is(err, errInjected) {
				t.Fatalf("DeleteAccount() = %v, want injected failure", err)
			}
			f.assertIntact(t, a, b)
			if f.count(t, `SELECT COUNT(*) FROM players WHERE id = ?`, owner.ID) != 1 ||
				f.count(t, `SELECT COUNT(*) FROM player_api_tokens WHERE player_id = ?`, owner.ID) != 1 {
				t.Fatal("player or API token was not restored")
			}
			if f.leader(t, solo.ID) != a.ID || f.leader(t, shared.ID) != b.ID || f.dissolutionNews(t, solo) != 0 {
				t.Fatal("guild state or dissolution news survived the rollback")
			}
		})
	}
}

// Direct character deletion rolls back guild succession when SQL cleanup fails.
func TestCharacterDeletionRollsBackGuildDeparture(t *testing.T) {
	f := newDeletionFixture(t)
	ctx := context.Background()
	leader, member := f.character(t, ""), f.character(t, "")
	solo := f.character(t, "")
	shared := f.createGuild(t, leader, member)
	alone := f.createGuild(t, solo)

	for _, c := range []corecharacter.Character{leader, solo} {
		f.chars.failDelete = c.ID
		if err := f.charSvc.Delete(ctx, c.PlayerID, c.ID); !errors.Is(err, errInjected) {
			t.Fatalf("Delete(%s) = %v, want injected failure", c.ID, err)
		}
	}
	f.assertIntact(t, leader, solo)
	if f.leader(t, shared.ID) != leader.ID || f.leader(t, alone.ID) != solo.ID || f.dissolutionNews(t, alone) != 0 {
		t.Fatal("guild departure survived the rolled-back deletion")
	}
}

// Successful account deletion hands over or dissolves guilds once and stops authentication.
func TestAccountDeletionSucceedsAcrossSharedGuild(t *testing.T) {
	f := newDeletionFixture(t)
	ctx := context.Background()
	owner, err := database.CreateTestPlayer(ctx, f.db)
	if err != nil {
		t.Fatal(err)
	}
	a, b := f.character(t, owner.ID), f.character(t, owner.ID)
	other, solo := f.character(t, ""), f.character(t, owner.ID)
	shared := f.createGuild(t, a, b, other) // both owned characters leave; other inherits
	alone := f.createGuild(t, solo)
	_, token, err := f.accounts.CreateAPIToken(ctx, owner.ID, "gone", nil)
	if err != nil {
		t.Fatal(err)
	}

	if err := f.accounts.DeleteAccount(ctx, owner.ID, ""); err != nil {
		t.Fatal(err)
	}
	if n := f.count(t, `SELECT COUNT(*) FROM characters WHERE player_id = ?`, owner.ID); n != 0 {
		t.Fatalf("%d characters survived", n)
	}
	if f.leader(t, shared.ID) != other.ID || f.count(t, `SELECT COUNT(*) FROM guild_members WHERE guild_id = ?`, shared.ID) != 1 {
		t.Fatal("shared guild did not pass to the surviving member")
	}
	if f.leader(t, alone.ID) != "" || f.dissolutionNews(t, alone) != 1 {
		t.Fatal("sole guild was not dissolved exactly once")
	}
	if _, err := f.accounts.Authenticate(ctx, token); err == nil {
		t.Fatal("API token still authenticates after account deletion")
	}
}

// Account deletion and a guildmate's departure lock characters before guilds in
// ascending order, so they never deadlock and leave a consistent guild.
func TestAccountDeletionRacingGuildDeparture(t *testing.T) {
	f := newDeletionFixture(t)
	ctx := context.Background()
	cfg := testutil.GetStressConfig()
	for round := 0; round < cfg.Workers/3+1; round++ {
		owner, err := database.CreateTestPlayer(ctx, f.db)
		if err != nil {
			t.Fatal(err)
		}
		a, b := f.character(t, owner.ID), f.character(t, owner.ID)
		x, y := f.character(t, ""), f.character(t, "")
		g := f.createGuild(t, a, x, b, y)

		errDelete, errLeave := testutil.RunRace2(
			func() error { return f.accounts.DeleteAccount(ctx, owner.ID, "") },
			func() error { return f.guilds.Leave(ctx, g.ID, x.ID) },
		)
		if errDelete != nil || errLeave != nil {
			t.Fatalf("round %d: delete=%v leave=%v", round, errDelete, errLeave)
		}
		if f.leader(t, g.ID) != y.ID || f.count(t, `SELECT COUNT(*) FROM guild_members WHERE guild_id = ?`, g.ID) != 1 {
			t.Fatalf("round %d: guild should be left to its only survivor", round)
		}
		if err := f.charSvc.Delete(ctx, "", x.ID); err != nil {
			t.Fatal(err)
		}
		if err := f.charSvc.Delete(ctx, "", y.ID); err != nil {
			t.Fatal(err)
		}
	}
}

// Concurrent deletion of two characters with mutual letters updates shared letter
// rows in ascending ID order and purges both copies (#2017).
func TestMutualCharacterDeletionDoesNotDeadlock(t *testing.T) {
	f := newDeletionFixture(t)
	ctx := context.Background()
	homes, err := database.NewHomeRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 10; round++ {
		a, b := f.character(t, ""), f.character(t, "")
		var letters []string
		base := time.Now().UTC().Add(-time.Hour)
		for i := range 20 {
			from, to := a, b
			if i%2 == 1 {
				from, to = b, a
			}
			l := home.Letter{ID: id.New(), SenderCharacterID: from.ID, SenderName: from.Name,
				RecipientCharacterID: to.ID, RecipientName: to.Name, Content: "mutual", Color: "#000000",
				CreatedAt: base.Add(time.Duration(i) * time.Minute)}
			if err := homes.CreateLetter(ctx, l, 100); err != nil {
				t.Fatal(err)
			}
			letters = append(letters, l.ID)
		}
		errA, errB := testutil.RunRace2(
			func() error { return f.charSvc.Delete(ctx, a.PlayerID, a.ID) },
			func() error { return f.charSvc.Delete(ctx, b.PlayerID, b.ID) },
		)
		if errA != nil || errB != nil {
			t.Fatalf("round %d: A=%v B=%v", round, errA, errB)
		}
		for _, l := range letters {
			if f.count(t, `SELECT COUNT(*) FROM character_letters WHERE id = ?`, l) != 0 {
				t.Fatalf("round %d: letter %s survived both owners", round, l)
			}
		}
	}
}
