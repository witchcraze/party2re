package god_test

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/witchcraze/party2re/internal/casino"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/god"
	"github.com/witchcraze/party2re/internal/guild"
	"github.com/witchcraze/party2re/internal/testutil"
)

var errCasinoSave = errors.New("casino save failed")

type failingCasino struct{}

func (failingCasino) AdjustCoins(context.Context, string, int64) (casino.Account, error) {
	return casino.Account{}, errCasinoSave
}

func TestGod_GrantWish_Admission(t *testing.T) {
	ctx := context.Background()
	for _, tc := range []struct {
		name    string
		visit   string
		wish    string
		casino  god.CasinoRepository
		wantErr error
		// wantVisit is the stored visit afterwards; wantMoney the stored gold.
		wantVisit string
		wantMoney int
	}{
		{"no visit", "", god.WishMoney, nil, god.ErrNotAdmitted, "", 0},
		// The visit decides the realm, so an Underworld wish is unknown in Heaven.
		{"other realm wish", corecharacter.GodVisitHeaven, god.WishExpandDepot, nil, god.ErrWishNotFound, corecharacter.GodVisitHeaven, 0},
		{"granted ends visit", corecharacter.GodVisitHeaven, god.WishMoney, nil, nil, "", 100000},
		{"unmet requirement keeps visit", corecharacter.GodVisitHeaven, god.WishGuildRank, nil, god.ErrWishRequirement, corecharacter.GodVisitHeaven, 0},
		{"lover keeps visit", corecharacter.GodVisitHeaven, god.WishLover, nil, nil, corecharacter.GodVisitHeaven, 0},
		{"dependency failure keeps visit", corecharacter.GodVisitHeaven, god.WishCasinoCoins, failingCasino{}, errCasinoSave, corecharacter.GodVisitHeaven, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockCharacterRepo()
			repo.characters["c"] = corecharacter.Character{ID: "c", GodVisit: tc.visit}
			opts := []god.Option{}
			if tc.casino != nil {
				opts = append(opts, god.WithCasinoRepository(tc.casino))
			}
			svc, err := god.NewService(repo, opts...)
			if err != nil {
				t.Fatal(err)
			}
			_, err = svc.GrantWish(ctx, "c", tc.wish)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if c := repo.characters["c"]; c.GodVisit != tc.wantVisit || c.Money != tc.wantMoney {
				t.Fatalf("stored visit=%q money=%d, want %q/%d", c.GodVisit, c.Money, tc.wantVisit, tc.wantMoney)
			}
			if tc.wantErr == nil && tc.wantVisit == "" {
				if _, err := svc.GrantWish(ctx, "c", tc.wish); !errors.Is(err, god.ErrNotAdmitted) {
					t.Fatalf("second wish err = %v, want ErrNotAdmitted", err)
				}
			}
		})
	}
}

func TestGod_GrantWish_Integration_ConcurrentDoubleSubmit(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}
	db, err := database.OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	ctx := context.Background()
	c, err := database.CreateTestCharacter(ctx, db, "GodDouble")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), "DELETE FROM characters WHERE id = ?", c.ID)
		_, _ = db.ExecContext(context.Background(), "DELETE FROM players WHERE id = ?", c.PlayerID)
	})
	repo, err := database.NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	c.GodVisit = corecharacter.GodVisitHeaven
	if err := repo.Update(ctx, c); err != nil {
		t.Fatal(err)
	}
	svc, err := god.NewService(repo, god.WithTransactionProvider(database.NewTransactionProvider(db)))
	if err != nil {
		t.Fatal(err)
	}

	wish := func() error {
		_, err := svc.GrantWish(ctx, c.ID, god.WishMoney)
		return err
	}
	errs := testutil.RunRace(wish, wish)
	granted := 0
	for _, err := range errs {
		switch {
		case err == nil:
			granted++
		case !errors.Is(err, god.ErrNotAdmitted):
			t.Fatalf("unexpected error: %v", err)
		}
	}
	after, err := repo.FindByID(ctx, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	if granted != 1 || after.Money != c.Money+100000 || after.GodVisit != "" {
		t.Fatalf("granted=%d money=%d visit=%q, want 1, %d, empty", granted, after.Money, after.GodVisit, c.Money+100000)
	}
}

func TestGod_ForfeitVisit(t *testing.T) {
	repo := newMockCharacterRepo()
	repo.characters["c"] = corecharacter.Character{ID: "c", GodVisit: corecharacter.GodVisitUnderworld, Money: 7}
	svc, err := god.NewService(repo)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 { // a second forfeit is a no-op
		if err := svc.ForfeitVisit(context.Background(), "c"); err != nil {
			t.Fatal(err)
		}
	}
	if c := repo.characters["c"]; c.GodVisit != "" || c.Money != 7 {
		t.Fatalf("stored %+v, want only the visit cleared", c)
	}
	if _, err := svc.GrantWish(context.Background(), "c", god.WishExpandDepot); !errors.Is(err, god.ErrNotAdmitted) {
		t.Fatalf("wish after forfeit: %v", err)
	}
}

// failingGuilds fails every membership read, unlike "not in a guild".
type failingGuilds struct{ mockGuildRepo }

var errGuildRead = errors.New("guild read failed")

func (failingGuilds) GetGuildByCharacter(context.Context, string) (guild.Guild, guild.Member, error) {
	return guild.Guild{}, guild.Member{}, errGuildRead
}

func TestGod_GuildReadFailureIsNotNoGuild(t *testing.T) {
	ctx := context.Background()
	repo := newMockCharacterRepo()
	repo.characters["c"] = corecharacter.Character{ID: "c", Level: 99, GodVisit: corecharacter.GodVisitHeaven}
	svc, err := god.NewService(repo, god.WithGuildRepository(&failingGuilds{}))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GetWishes(ctx, "c", god.RealmHeaven); !errors.Is(err, errGuildRead) {
		t.Fatalf("GetWishes err = %v", err)
	}
	for _, wish := range []string{god.WishGuildRank, god.WishGuildGorgeous} {
		if _, err := svc.GrantWish(ctx, "c", wish); !errors.Is(err, errGuildRead) || errors.Is(err, god.ErrWishRequirement) {
			t.Fatalf("%s err = %v", wish, err)
		}
		if repo.characters["c"].GodVisit != corecharacter.GodVisitHeaven {
			t.Fatal("failed grant ended the visit")
		}
	}
}
