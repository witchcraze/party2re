package main

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/database"
	"github.com/witchcraze/party2re/internal/testutil"
)

func TestCharacterNameEqualityAndRetry(t *testing.T) {
	f := newReferralFixture(t)
	ctx := context.Background()
	for _, name := range []string{"Hero", "hero", "Héro", "HERO", "é"} {
		created, err := f.create(ctx, name, "")
		if err != nil || created.Name != name {
			t.Fatalf("distinct exact name %q: created=%+v err=%v", name, created, err)
		}
	}
	for _, name := range []string{"Hero", "e\u0301"} {
		if _, err := f.create(ctx, name, ""); !errors.Is(err, character.ErrNameAlreadyTaken) {
			t.Fatalf("duplicate/NFC-equivalent name %q: %v", name, err)
		}
	}
	f.assertCounts(t, 5, 0)
	// Direct duplicate primary keys and unrelated storage failures retain their errors.
	if err := f.core.charRepo.Save(ctx, f.referrer); err == nil || errors.Is(err, character.ErrNameAlreadyTaken) {
		t.Fatalf("primary-key duplicate mistaken for name conflict: %v", err)
	}
	// The shared fixture factory still produces distinct characters for repeated labels.
	first, err := database.CreateTestCharacter(ctx, f.db, "Fixture")
	if err != nil {
		t.Fatal(err)
	}
	second, err := database.CreateTestCharacter(ctx, f.db, "Fixture")
	if err != nil || first.Name == second.Name || first.ID == second.ID {
		t.Fatalf("repeated fixture label: first=%+v second=%+v err=%v", first, second, err)
	}
}

func TestCharacterNameAssignmentRaces(t *testing.T) {
	for _, scenario := range []string{"creation_creation", "creation_rename", "rename_rename"} {
		t.Run(scenario, func(t *testing.T) {
			f := newReferralFixture(t)
			ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
			defer cancel()
			var renamers []corecharacter.Character
			var operations []func() error
			for i := 0; i < 2; i++ {
				rename := scenario == "rename_rename" || (scenario == "creation_rename" && i == 1)
				if rename {
					original, err := database.CreateTestCharacterWithFunds(ctx, f.db, "Renamer", 900000)
					if err != nil {
						t.Fatal(err)
					}
					renamers = append(renamers, original)
					operations = append(operations, func() error {
						_, err := f.core.charService.ChangeName(ctx, original.ID, "RaceName")
						return err
					})
				} else {
					operations = append(operations, func() error {
						_, err := f.create(ctx, "RaceName", "")
						return err
					})
				}
			}
			successes := 0
			for _, err := range testutil.RunRace(operations...) {
				if err == nil {
					successes++
				} else if !errors.Is(err, character.ErrNameAlreadyTaken) {
					t.Fatalf("unexpected name assignment error: %v", err)
				}
			}
			if successes != 1 {
				t.Fatalf("successful name assignments=%d, want 1", successes)
			}
			var count int
			if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters WHERE name = 'RaceName'").Scan(&count); err != nil || count != 1 {
				t.Fatalf("target name owners=%d err=%v", count, err)
			}
			if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM news_articles").Scan(&count); err != nil || count != 1 {
				t.Fatalf("failed assignment published news: articles=%d err=%v", count, err)
			}
			for _, original := range renamers {
				current, err := f.core.charRepo.FindByID(ctx, original.ID)
				if err != nil {
					t.Fatal(err)
				}
				wantMoney := original.Money
				if current.Name == "RaceName" {
					wantMoney -= character.NameChangeCost
				} else if current.Name != original.Name {
					t.Fatalf("failed rename changed name: %+v", current)
				}
				if current.Money != wantMoney {
					t.Fatalf("rename currency conservation: got=%d want=%d", current.Money, wantMoney)
				}
			}
			var characterCount, depotCount int
			if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM characters").Scan(&characterCount); err != nil {
				t.Fatal(err)
			}
			if err := f.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_depots").Scan(&depotCount); err != nil || depotCount != characterCount {
				t.Fatalf("character/depot conservation: characters=%d depots=%d err=%v", characterCount, depotCount, err)
			}
		})
	}
}
