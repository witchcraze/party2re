package database

import (
	"context"
	"database/sql"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/witchcraze/party2re/internal/character"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type interceptingProfileRepo struct {
	character.ProfileRepository
	afterGetProfile func(ctx context.Context, charID string)
}

func (r *interceptingProfileRepo) GetProfile(ctx context.Context, characterID string) (character.Profile, error) {
	p, err := r.ProfileRepository.GetProfile(ctx, characterID)
	if r.afterGetProfile != nil {
		r.afterGetProfile(ctx, characterID)
	}
	return p, err
}

// TestCharacterService_ConcurrentProfilePatch_DisjointFields reproduces the interleaving
// where the profile form and the personality writer update the profile concurrently.
// It verifies that Service 2 blocks on the character row lock while Service 1 is in-flight,
// and after Service 1 commits, Service 2 applies its change over the committed state,
// preserving both new fields as well as existing bio data.
func TestCharacterService_ConcurrentProfilePatch_DisjointFields(t *testing.T) {
	db := OpenTestDB(t)

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfDisjointTest")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, char)

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	// 1. Persist initial profile: AvatarURL and BioData
	initialAvatar := "https://i.imgur.com/AvDTest.gif"
	initialBio := map[string]string{"hobby": "saved"}
	err = charRepo.SaveProfile(ctx, character.Profile{
		CharacterID: char.ID,
		AvatarURL:   initialAvatar,
		BioData:     initialBio,
		UpdatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("setup SaveProfile failed: %v", err)
	}

	// Channels to coordinate interleaving
	svc1ReadDone := make(chan struct{})
	svc1Resume := make(chan struct{})
	var readOnce sync.Once
	var resumeOnce sync.Once
	closeResume := func() {
		resumeOnce.Do(func() {
			close(svc1Resume)
		})
	}
	t.Cleanup(closeResume)

	interceptingRepo := &interceptingProfileRepo{
		ProfileRepository: charRepo,
		afterGetProfile: func(ctx context.Context, charID string) {
			readOnce.Do(func() {
				close(svc1ReadDone)
			})
			<-svc1Resume
		},
	}

	svc1, err := character.NewService(charRepo,
		character.WithProfileRepository(interceptingRepo),
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	svc2, err := character.NewService(charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	svc1ErrChan := make(chan error, 1)
	go func() {
		_, err := svc1.UpdateProfile(context.Background(), char.ID, map[string]string{"hobby": "new"})
		svc1ErrChan <- err
	}()

	// Wait until svc1 has executed GetProfile and is paused holding character lock in tx
	select {
	case <-svc1ReadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1 to read profile")
	}

	// Launch svc2 UpdateProfile concurrently
	svc2ErrChan := make(chan error, 1)
	go func() {
		svc2ErrChan <- svc2.SetPersonality(context.Background(), char.ID, "x")
	}()

	// Assert that svc2 is blocked waiting on the lock held by svc1
	select {
	case err := <-svc2ErrChan:
		t.Fatalf("svc2 finished unexpectedly while svc1 was holding character lock, err=%v", err)
	case <-time.After(100 * time.Millisecond):
		// Expected: svc2 is blocked on row lock in MariaDB
	}

	// Resume svc1 so it can save and commit its transaction
	closeResume()

	// Wait for svc1 to finish
	select {
	case err := <-svc1ErrChan:
		if err != nil {
			t.Fatalf("svc1 UpdateProfile failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1 to complete")
	}

	// Now svc2 unblocks, applies its update, and completes
	select {
	case err := <-svc2ErrChan:
		if err != nil {
			t.Fatalf("svc2 UpdateProfile failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc2 to complete after svc1 commit")
	}

	// Both writes and the untouched avatar are retained.
	finalProf, err := charRepo.GetProfile(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}
	if finalProf.AvatarURL != initialAvatar || finalProf.BioData["hobby"] != "new" || finalProf.BioData[character.PersonalityBioKey] != "x" {
		t.Errorf("lost update: %+v", finalProf)
	}
}

// TestCharacterService_ConcurrentFirstProfileCreation tests concurrent profile creation
// when the character has no existing profile row in character_profiles.
// It verifies that the row lock on characters serializes first-profile creation.
func TestCharacterService_ConcurrentFirstProfileCreation(t *testing.T) {
	db := OpenTestDB(t)

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfFirstTest")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, char)

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	// Verify no profile exists yet
	var count int
	err = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM character_profiles WHERE character_id = ?", char.ID).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected 0 profiles for new character, got %d", count)
	}

	svc1ReadDone := make(chan struct{})
	svc1Resume := make(chan struct{})
	var readOnce sync.Once
	var resumeOnce sync.Once
	closeResume := func() {
		resumeOnce.Do(func() {
			close(svc1Resume)
		})
	}
	t.Cleanup(closeResume)

	interceptingRepo := &interceptingProfileRepo{
		ProfileRepository: charRepo,
		afterGetProfile: func(ctx context.Context, charID string) {
			readOnce.Do(func() {
				close(svc1ReadDone)
			})
			<-svc1Resume
		},
	}

	svc1, err := character.NewService(charRepo,
		character.WithProfileRepository(interceptingRepo),
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	svc2, err := character.NewService(charRepo,
		character.WithTransactionProvider(txProvider),
	)
	if err != nil {
		t.Fatal(err)
	}

	svc1ErrChan := make(chan error, 1)
	go func() {
		_, err := svc1.UpdateProfile(context.Background(), char.ID, map[string]string{"hobby": "first"})
		svc1ErrChan <- err
	}()

	select {
	case <-svc1ReadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1 to read profile")
	}

	svc2ErrChan := make(chan error, 1)
	go func() {
		svc2ErrChan <- svc2.SetPersonality(context.Background(), char.ID, "x")
	}()

	select {
	case err := <-svc2ErrChan:
		t.Fatalf("svc2 finished unexpectedly while svc1 was holding lock, err=%v", err)
	case <-time.After(100 * time.Millisecond):
		// Expected: svc2 is blocked on characters row lock
	}

	closeResume()

	select {
	case err := <-svc1ErrChan:
		if err != nil {
			t.Fatalf("svc1 failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1")
	}

	select {
	case err := <-svc2ErrChan:
		if err != nil {
			t.Fatalf("svc2 failed: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc2")
	}

	finalProf, err := charRepo.GetProfile(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}

	if finalProf.BioData["hobby"] != "first" || finalProf.BioData[character.PersonalityBioKey] != "x" {
		t.Errorf("lost update: %+v", finalProf)
	}
}

// TestCharacterService_ProfileUpdate_StorageErrorAndCorruptJSON verifies that
// failed profile reads (e.g. corrupt JSON) cause zero saves and do not overwrite
// existing stored data with default empty profiles.
func TestCharacterService_ProfileUpdate_StorageErrorAndCorruptJSON(t *testing.T) {
	db := OpenTestDB(t)

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfCorruptTest")
	if err != nil {
		t.Fatal(err)
	}
	RegisterCharacterCleanup(t, db, char)

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	svc, err := character.NewService(charRepo, character.WithTransactionProvider(txProvider))
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial valid profile
	initialAvatar := "https://i.imgur.com/AbCd1.gif"
	if err := charRepo.SaveProfile(ctx, character.Profile{CharacterID: char.ID, AvatarURL: initialAvatar, UpdatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("initial SaveProfile failed: %v", err)
	}

	// 2. Inject valid JSON with non-string values directly into database.
	// MariaDB's JSON column validates syntax, but Go's map[string]string unmarshaling fails.
	corruptRaw := `{"unsupported_number": 12345}`
	_, err = db.ExecContext(ctx, "UPDATE character_profiles SET bio_data = ? WHERE character_id = ?", corruptRaw, char.ID)
	if err != nil {
		t.Fatalf("injecting incompatible JSON failed: %v", err)
	}

	// 3. Attempt UpdateProfile: must fail without overwriting existing row with defaults
	_, err = svc.UpdateProfile(ctx, char.ID, map[string]string{"hobby": "overwrite"})
	if err == nil {
		t.Fatal("expected UpdateProfile to fail on incompatible bio_data JSON, got nil")
	}

	// Verify the database row was NOT overwritten
	var savedAvatar, savedBioData string
	err = db.QueryRowContext(ctx, "SELECT avatar_url, bio_data FROM character_profiles WHERE character_id = ?", char.ID).Scan(&savedAvatar, &savedBioData)
	if err != nil {
		t.Fatalf("reading back profile row failed: %v", err)
	}

	if savedAvatar != initialAvatar {
		t.Errorf("avatar_url was overwritten on error: got %q, want %q", savedAvatar, initialAvatar)
	}
	if !strings.Contains(savedBioData, "12345") {
		t.Errorf("incompatible bio_data was lost or reset: got %q", savedBioData)
	}
}

func TestCharacterProfileFixtureCleanup(t *testing.T) {
	observer := OpenTestDB(t)
	ctx := context.Background()

	obsRepo, err := NewCharacterRepository(observer)
	if err != nil {
		t.Fatalf("NewCharacterRepository observer: %v", err)
	}

	unrelated, err := CreateTestCharacter(ctx, observer, "UnrelProf")
	if err != nil {
		t.Fatalf("CreateTestCharacter unrelated: %v", err)
	}
	RegisterCharacterCleanup(t, observer, unrelated)

	now := time.Now().UTC().Truncate(time.Second)
	if err := obsRepo.SaveProfile(ctx, character.Profile{
		CharacterID: unrelated.ID,
		AvatarURL:   "https://i.imgur.com/UnRel1.gif",
		UpdatedAt:   now,
	}); err != nil {
		t.Fatalf("SaveProfile unrelated: %v", err)
	}

	seed := func(t *testing.T, db *sql.DB, charID string) {
		t.Helper()
		repo, err := NewCharacterRepository(db)
		if err != nil {
			t.Fatalf("NewCharacterRepository: %v", err)
		}
		if err := repo.SaveProfile(ctx, character.Profile{
			CharacterID: charID,
			AvatarURL:   "https://i.imgur.com/OwnEd1.gif",
			UpdatedAt:   now,
		}); err != nil {
			t.Fatalf("SaveProfile owned: %v", err)
		}
	}

	var normalChar, earlyChar corecharacter.Character
	t.Run("normal scope end", func(t *testing.T) {
		db := OpenTestDB(t)
		var err error
		normalChar, err = CreateTestCharacter(ctx, db, "ProfNorm")
		if err != nil {
			t.Fatalf("CreateTestCharacter normal: %v", err)
		}
		RegisterCharacterCleanup(t, db, normalChar)
		seed(t, db, normalChar.ID)
	})

	t.Run("early return path", func(t *testing.T) {
		db := OpenTestDB(t)
		var err error
		earlyChar, err = CreateTestCharacter(ctx, db, "ProfEarly")
		if err != nil {
			t.Fatalf("CreateTestCharacter early: %v", err)
		}
		RegisterCharacterCleanup(t, db, earlyChar)
		seed(t, db, earlyChar.ID)
		return
	})

	assertRowCount := func(query, arg string, want int) {
		t.Helper()
		assertFixtureRowCount(t, ctx, observer, query, arg, want)
	}

	for _, c := range []corecharacter.Character{normalChar, earlyChar} {
		assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM character_profiles WHERE character_id = ?", c.ID, 0)
		assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", c.PlayerID, 0)
	}

	assertRowCount("SELECT COUNT(*) FROM characters WHERE id = ?", unrelated.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM character_depots WHERE character_id = ?", unrelated.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM character_profiles WHERE character_id = ?", unrelated.ID, 1)
	assertRowCount("SELECT COUNT(*) FROM players WHERE id = ?", unrelated.PlayerID, 1)
}
