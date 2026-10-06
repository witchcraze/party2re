package database

import (
	"context"
	"database/sql"
	"os"
	"strings"
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

func cleanupTestCharacter(t *testing.T, db *sql.DB, char corecharacter.Character) {
	t.Helper()
	ctx := context.Background()
	_, _ = db.ExecContext(ctx, "DELETE FROM character_profiles WHERE character_id = ?", char.ID)
	_, _ = db.ExecContext(ctx, "DELETE FROM characters WHERE id = ?", char.ID)
	if char.PlayerID != "" {
		_, _ = db.ExecContext(ctx, "DELETE FROM players WHERE id = ?", char.PlayerID)
	}
}

// TestCharacterService_ConcurrentProfilePatch_DisjointFields reproduces the interleaving
// where two services update disjoint fields (Comment vs AvatarURL) concurrently.
// It verifies that Service 2 blocks on the character row lock while Service 1 is in-flight,
// and after Service 1 commits, Service 2 applies its change over the committed state,
// preserving both new fields as well as existing bio data.
func TestCharacterService_ConcurrentProfilePatch_DisjointFields(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfDisjointTest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupTestCharacter(t, db, char)
	})

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	// 1. Persist initial profile: AvatarURL and BioData
	initialAvatar := "https://example.com/old.png"
	initialBio := map[string]string{"key": "saved"}
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

	interceptingRepo := &interceptingProfileRepo{
		ProfileRepository: charRepo,
		afterGetProfile: func(ctx context.Context, charID string) {
			select {
			case svc1ReadDone <- struct{}{}:
			default:
			}
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

	newComment := "New comment from svc1"
	newAvatar := "https://example.com/new.png"

	svc1ErrChan := make(chan error, 1)
	go func() {
		_, err := svc1.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			Comment: &newComment,
		})
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
		_, err := svc2.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			AvatarURL: &newAvatar,
		})
		svc2ErrChan <- err
	}()

	// Assert that svc2 is blocked waiting on the lock held by svc1
	select {
	case err := <-svc2ErrChan:
		t.Fatalf("svc2 finished unexpectedly while svc1 was holding character lock, err=%v", err)
	case <-time.After(100 * time.Millisecond):
		// Expected: svc2 is blocked on row lock in MariaDB
	}

	// Resume svc1 so it can save and commit its transaction
	close(svc1Resume)

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

	// Assert persisted state in MariaDB: both new comment and new avatar are retained!
	finalProf, err := charRepo.GetProfile(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}

	if finalProf.Comment != newComment {
		t.Errorf("comment mismatch: got %q, want %q", finalProf.Comment, newComment)
	}
	if finalProf.AvatarURL != newAvatar {
		t.Errorf("avatar URL mismatch: got %q, want %q", finalProf.AvatarURL, newAvatar)
	}
	if finalProf.BioData["key"] != "saved" {
		t.Errorf("bio data mismatch: got %v, want %v", finalProf.BioData, initialBio)
	}
}

// TestCharacterService_ConcurrentFirstProfileCreation tests concurrent profile creation
// when the character has no existing profile row in character_profiles.
// It verifies that the row lock on characters serializes first-profile creation.
func TestCharacterService_ConcurrentFirstProfileCreation(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfFirstTest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupTestCharacter(t, db, char)
	})

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

	interceptingRepo := &interceptingProfileRepo{
		ProfileRepository: charRepo,
		afterGetProfile: func(ctx context.Context, charID string) {
			select {
			case svc1ReadDone <- struct{}{}:
			default:
			}
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

	c1 := "First creation comment"
	aura2 := 5

	svc1ErrChan := make(chan error, 1)
	go func() {
		_, err := svc1.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			Comment: &c1,
		})
		svc1ErrChan <- err
	}()

	select {
	case <-svc1ReadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1 to read profile")
	}

	svc2ErrChan := make(chan error, 1)
	go func() {
		_, err := svc2.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			AuraEffect: &aura2,
		})
		svc2ErrChan <- err
	}()

	select {
	case err := <-svc2ErrChan:
		t.Fatalf("svc2 finished unexpectedly while svc1 was holding lock, err=%v", err)
	case <-time.After(100 * time.Millisecond):
		// Expected: svc2 is blocked on characters row lock
	}

	close(svc1Resume)

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

	if finalProf.Comment != c1 {
		t.Errorf("comment mismatch: got %q, want %q", finalProf.Comment, c1)
	}
	if finalProf.AuraEffect != aura2 {
		t.Errorf("aura effect mismatch: got %d, want %d", finalProf.AuraEffect, aura2)
	}
}

// TestCharacterService_ConcurrentAllOptionalFields verifies that Comment, AvatarURL,
// AuraEffect, and BioData are all preserved across concurrent partial updates.
func TestCharacterService_ConcurrentAllOptionalFields(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfAllFieldsTest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupTestCharacter(t, db, char)
	})

	charRepo, err := NewCharacterRepository(db)
	if err != nil {
		t.Fatal(err)
	}
	txProvider := NewTransactionProvider(db)

	svc1ReadDone := make(chan struct{})
	svc1Resume := make(chan struct{})

	interceptingRepo := &interceptingProfileRepo{
		ProfileRepository: charRepo,
		afterGetProfile: func(ctx context.Context, charID string) {
			select {
			case svc1ReadDone <- struct{}{}:
			default:
			}
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

	// Service 1 sets Comment and BioData
	comment1 := "Comprehensive comment"
	bio1 := map[string]string{"hobby": "fishing", "food": "ramen"}

	// Service 2 sets AvatarURL and AuraEffect
	avatar2 := "https://example.com/avatar_all.png"
	aura2 := 7

	svc1ErrChan := make(chan error, 1)
	go func() {
		_, err := svc1.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			Comment: &comment1,
			BioData: bio1,
		})
		svc1ErrChan <- err
	}()

	select {
	case <-svc1ReadDone:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for svc1 read")
	}

	svc2ErrChan := make(chan error, 1)
	go func() {
		_, err := svc2.UpdateProfile(context.Background(), char.ID, character.UpdateProfileRequest{
			AvatarURL:  &avatar2,
			AuraEffect: &aura2,
		})
		svc2ErrChan <- err
	}()

	select {
	case err := <-svc2ErrChan:
		t.Fatalf("svc2 completed prematurely: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	close(svc1Resume)

	if err := <-svc1ErrChan; err != nil {
		t.Fatalf("svc1 failed: %v", err)
	}
	if err := <-svc2ErrChan; err != nil {
		t.Fatalf("svc2 failed: %v", err)
	}

	finalProf, err := charRepo.GetProfile(ctx, char.ID)
	if err != nil {
		t.Fatalf("GetProfile failed: %v", err)
	}

	if finalProf.Comment != comment1 {
		t.Errorf("comment mismatch: got %q, want %q", finalProf.Comment, comment1)
	}
	if finalProf.AvatarURL != avatar2 {
		t.Errorf("avatar URL mismatch: got %q, want %q", finalProf.AvatarURL, avatar2)
	}
	if finalProf.AuraEffect != aura2 {
		t.Errorf("aura mismatch: got %d, want %d", finalProf.AuraEffect, aura2)
	}
	if finalProf.BioData["hobby"] != "fishing" || finalProf.BioData["food"] != "ramen" {
		t.Errorf("bio data mismatch: got %v", finalProf.BioData)
	}
}

// TestCharacterService_ProfileUpdate_StorageErrorAndCorruptJSON verifies that
// failed profile reads (e.g. corrupt JSON) cause zero saves and do not overwrite
// existing stored data with default empty profiles.
func TestCharacterService_ProfileUpdate_StorageErrorAndCorruptJSON(t *testing.T) {
	if os.Getenv("PARTY2_DB_DSN") == "" {
		t.Skip("PARTY2_DB_DSN is not configured")
	}

	db, err := OpenFromEnvironment()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	ctx := context.Background()
	char, err := CreateTestCharacter(ctx, db, "ProfCorruptTest")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanupTestCharacter(t, db, char)
	})

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
	initialComment := "Before corruption"
	initialAvatar := "https://example.com/intact.png"
	_, err = svc.UpdateProfile(ctx, char.ID, character.UpdateProfileRequest{
		Comment:   &initialComment,
		AvatarURL: &initialAvatar,
	})
	if err != nil {
		t.Fatalf("initial UpdateProfile failed: %v", err)
	}

	// 2. Inject valid JSON with non-string values directly into database.
	// MariaDB's JSON column validates syntax, but Go's map[string]string unmarshaling fails.
	corruptRaw := `{"unsupported_number": 12345}`
	_, err = db.ExecContext(ctx, "UPDATE character_profiles SET bio_data = ? WHERE character_id = ?", corruptRaw, char.ID)
	if err != nil {
		t.Fatalf("injecting incompatible JSON failed: %v", err)
	}

	// 3. Attempt UpdateProfile: must fail without overwriting existing row with defaults
	newComment := "Attempted overwrite comment"
	_, err = svc.UpdateProfile(ctx, char.ID, character.UpdateProfileRequest{
		Comment: &newComment,
	})
	if err == nil {
		t.Fatal("expected UpdateProfile to fail on incompatible bio_data JSON, got nil")
	}

	// Verify the database row was NOT overwritten
	var savedComment, savedBioData string
	err = db.QueryRowContext(ctx, "SELECT comment, bio_data FROM character_profiles WHERE character_id = ?", char.ID).Scan(&savedComment, &savedBioData)
	if err != nil {
		t.Fatalf("reading back profile row failed: %v", err)
	}

	if savedComment != initialComment {
		t.Errorf("comment was overwritten on error: got %q, want %q", savedComment, initialComment)
	}
	if !strings.Contains(savedBioData, "12345") {
		t.Errorf("incompatible bio_data was lost or reset: got %q", savedBioData)
	}
}
