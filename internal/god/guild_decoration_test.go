package god_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/testutil"
)

type decorationFixture struct {
	db     *sql.DB
	ctx    context.Context
	g      guild.Guild
	leader corecharacter.Character
	chars  *database.CharacterRepository
	repo   *database.GuildRepository
	guilds *guild.Service
	wishes *god.Service
}

func newDecorationFixture(t *testing.T) decorationFixture {
	t.Helper()
	f := decorationFixture{db: database.CreateTestDatabase(t,
		"players", "characters", "character_depots", "guilds", "guild_members", "guild_decorations")}
	var cancel context.CancelFunc
	f.ctx, cancel = context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)
	var err error
	f.g, f.leader, err = database.CreateTestGuildWithLeader(f.ctx, f.db, "Medals", 10000)
	if err != nil {
		t.Fatal(err)
	}
	f.chars, err = database.NewCharacterRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.repo, err = database.NewGuildRepository(f.db)
	if err != nil {
		t.Fatal(err)
	}
	f.guilds, err = guild.NewService(f.repo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}
	f.wishes, err = god.NewService(f.chars, god.WithGuildRepository(f.guilds),
		god.WithTransactionProvider(database.NewTransactionProvider(f.db)))
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f decorationFixture) admit(characterID string) error {
	_, err := f.db.ExecContext(f.ctx, "UPDATE characters SET god_visit = ? WHERE id = ?", corecharacter.GodVisitHeaven, characterID)
	return err
}

func TestGodGuildDecoration_PersistenceAndMembership(t *testing.T) {
	f := newDecorationFixture(t)
	for range 2 {
		if err := f.admit(f.leader.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.wishes.GrantWish(f.ctx, f.leader.ID, god.WishGuildGorgeous); err != nil {
			t.Fatal(err)
		}
	}
	detail, err := f.guilds.Get(f.ctx, f.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Guild.Bgimg != "god.gif" || len(detail.Decorations) != 2 {
		t.Fatalf("expected background and two medals, got %+v", detail)
	}
	if detail.Decorations[0].ID == detail.Decorations[1].ID {
		t.Fatal("repeated wishes must append separate medals")
	}
	for _, d := range detail.Decorations {
		if d.Kind != guild.GoldMedal || d.Icon != "assets/images/placeholders/guild/gold-medal.svg" || d.CreatedAt.IsZero() {
			t.Fatalf("unexpected decoration: %+v", d)
		}
	}
	if len(detail.Members) != 1 || detail.Members[0].Role != guild.RoleLeader || detail.Guild.LeaderCharacterID != f.leader.ID {
		t.Fatalf("decorations changed membership/authority: %+v", detail)
	}
	stored, err := f.chars.FindByID(f.ctx, f.leader.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stored.GodVisit != "" {
		t.Fatal("granted wish did not consume visit")
	}

	outsider, err := database.CreateTestCharacter(f.ctx, f.db, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, pending := range []bool{false, true} {
		if pending {
			if _, err := f.repo.AddMember(f.ctx, guild.Member{GuildID: f.g.ID, CharacterID: outsider.ID,
				Role: guild.RoleMember, IsPending: true, JoinedAt: time.Now().UTC()}); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.admit(outsider.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.wishes.GrantWish(f.ctx, outsider.ID, god.WishGuildGorgeous); !errors.Is(err, god.ErrWishRequirement) {
			t.Fatalf("unaffiliated/pending wish: %v", err)
		}
		c, err := f.chars.FindByID(f.ctx, outsider.ID)
		if err != nil {
			t.Fatal(err)
		}
		if c.GodVisit != corecharacter.GodVisitHeaven {
			t.Fatal("rejected wish consumed visit")
		}
	}
	other, _, err := database.CreateTestGuildWithLeader(f.ctx, f.db, "OtherMedals", 10000)
	if err != nil {
		t.Fatal(err)
	}
	otherDetail, err := f.guilds.Get(f.ctx, other.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(otherDetail.Decorations) != 0 {
		t.Fatal("decorations leaked to another guild")
	}
	if err := f.guilds.Disband(f.ctx, f.g.ID, f.leader.ID); err != nil {
		t.Fatal(err)
	}
	decorations, err := f.repo.ListDecorations(f.ctx, f.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(decorations) != 0 {
		t.Fatal("disband left orphan decorations")
	}
}

type failingDecorationRepo struct {
	*database.GuildRepository
	err error
}

func (r failingDecorationRepo) AddDecoration(context.Context, string, guild.Decoration) error {
	return r.err
}

func TestGodGuildDecoration_Rollback(t *testing.T) {
	f := newDecorationFixture(t)
	sentinel := errors.New("decoration transaction failure")
	for _, stage := range []string{"decoration save", "memory after character save"} {
		t.Run(stage, func(t *testing.T) {
			var repo guild.Repository = f.repo
			if stage == "decoration save" {
				repo = failingDecorationRepo{f.repo, sentinel}
			}
			guilds, err := guild.NewService(repo, guild.WithTransactionProvider(database.NewTransactionProvider(f.db)))
			if err != nil {
				t.Fatal(err)
			}
			opts := []god.Option{god.WithGuildRepository(guilds), god.WithTransactionProvider(database.NewTransactionProvider(f.db))}
			if stage != "decoration save" {
				opts = append(opts, god.WithMemoryRecorder(func(context.Context, string, corecharacter.MemoryKind, string) error { return sentinel }))
			}
			wishes, err := god.NewService(f.chars, opts...)
			if err != nil {
				t.Fatal(err)
			}
			if err := f.admit(f.leader.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := wishes.GrantWish(f.ctx, f.leader.ID, god.WishGuildGorgeous); !errors.Is(err, sentinel) {
				t.Fatalf("got %v", err)
			}
			detail, err := f.guilds.Get(f.ctx, f.g.ID)
			if err != nil {
				t.Fatal(err)
			}
			stored, err := f.chars.FindByID(f.ctx, f.leader.ID)
			if err != nil {
				t.Fatal(err)
			}
			if detail.Guild.Bgimg != f.g.Bgimg || len(detail.Decorations) != 0 || stored.GodVisit != corecharacter.GodVisitHeaven {
				t.Fatalf("partial wish committed: %+v, visit=%q", detail, stored.GodVisit)
			}
		})
	}
}

func TestGodGuildDecoration_ConcurrentWishes(t *testing.T) {
	f := newDecorationFixture(t)
	if err := f.admit(f.leader.ID); err != nil {
		t.Fatal(err)
	}
	wish := func() error {
		_, err := f.wishes.GrantWish(f.ctx, f.leader.ID, god.WishGuildGorgeous)
		return err
	}
	successes := 0
	for _, err := range testutil.RunRace(wish, wish) {
		if err == nil {
			successes++
		} else if !errors.Is(err, god.ErrNotAdmitted) {
			t.Fatal(err)
		}
	}
	if successes != 1 {
		t.Fatalf("double submit granted %d wishes", successes)
	}

	cfg := testutil.GetStressConfig()
	characters := make([]corecharacter.Character, cfg.Workers)
	for i := range characters {
		c, err := database.CreateTestCharacter(f.ctx, f.db, "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.guilds.Join(f.ctx, f.g.ID, c.ID); err != nil {
			t.Fatal(err)
		}
		characters[i] = c
	}
	res := testutil.RunConcurrentStressTest(t, cfg, func(worker, _ int) error {
		characterID := characters[worker].ID
		if err := f.admit(characterID); err != nil {
			return err
		}
		if _, err := f.wishes.GrantWish(f.ctx, characterID, god.WishGuildGorgeous); err != nil {
			return err
		}
		if _, err := f.wishes.GrantWish(f.ctx, characterID, god.WishGuildGorgeous); !errors.Is(err, god.ErrNotAdmitted) {
			return fmt.Errorf("repeat wish: %v", err)
		}
		return nil
	})
	if res.Failures != 0 {
		t.Fatalf("failed wishes: %+v", res)
	}
	detail, err := f.guilds.Get(f.ctx, f.g.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(detail.Decorations) != int(res.Successes)+1 || len(detail.Members) != cfg.Workers+1 {
		t.Fatalf("medals=%d members=%d; want %d/%d", len(detail.Decorations), len(detail.Members), res.Successes+1, cfg.Workers+1)
	}
	for _, c := range characters {
		after, err := f.chars.FindByID(f.ctx, c.ID)
		if err != nil {
			t.Fatal(err)
		}
		if after.GodVisit != "" || after.Money != c.Money {
			t.Fatalf("visit or money changed unexpectedly: %+v", after)
		}
	}
}
