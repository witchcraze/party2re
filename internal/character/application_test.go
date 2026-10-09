package character

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corejob "github.com/witchcraze/party2re/internal/core/job"
)

type mockRepository struct {
	characters       map[string]corecharacter.Character
	profiles         map[string]Profile
	err              error
	getProfileErr    error
	saveProfileCalls int
}

func newMockRepository() *mockRepository {
	return &mockRepository{
		characters: make(map[string]corecharacter.Character),
		profiles:   make(map[string]Profile),
	}
}

func (r *mockRepository) Save(_ context.Context, value corecharacter.Character) error {
	if r.err != nil {
		return r.err
	}
	r.characters[value.ID] = value
	return nil
}

func (r *mockRepository) FindByID(_ context.Context, id string) (corecharacter.Character, error) {
	if r.err != nil {
		return corecharacter.Character{}, r.err
	}
	c, ok := r.characters[id]
	if !ok {
		return corecharacter.Character{}, ErrNotFound
	}
	return c, nil
}

func (r *mockRepository) FindByIDForUpdate(_ context.Context, id string) (corecharacter.Character, error) {
	return r.FindByID(context.Background(), id)
}

func (r *mockRepository) FindByName(_ context.Context, name string) (corecharacter.Character, error) {
	if r.err != nil {
		return corecharacter.Character{}, r.err
	}
	for _, c := range r.characters {
		if c.Name == name {
			return c, nil
		}
	}
	return corecharacter.Character{}, ErrNotFound
}

func (r *mockRepository) FindByNameForUpdate(_ context.Context, name string) (corecharacter.Character, error) {
	return r.FindByName(context.Background(), name)
}

func (r *mockRepository) FindByPlayerID(_ context.Context, playerID string) ([]corecharacter.Character, error) {
	if r.err != nil {
		return nil, r.err
	}
	var res []corecharacter.Character
	for _, c := range r.characters {
		if c.PlayerID == playerID {
			res = append(res, c)
		}
	}
	return res, nil
}

func (r *mockRepository) Update(_ context.Context, value corecharacter.Character) error {
	if r.err != nil {
		return r.err
	}
	r.characters[value.ID] = value
	return nil
}

func (r *mockRepository) GetProfile(ctx context.Context, characterID string) (Profile, error) {
	if err := ctx.Err(); err != nil {
		return Profile{}, err
	}
	if r.getProfileErr != nil {
		return Profile{}, r.getProfileErr
	}
	if r.err != nil {
		return Profile{}, r.err
	}
	p, ok := r.profiles[characterID]
	if !ok {
		return Profile{}, ErrNotFound
	}
	return p, nil
}

func (r *mockRepository) SaveProfile(_ context.Context, profile Profile) error {
	r.saveProfileCalls++
	if r.err != nil {
		return r.err
	}
	r.profiles[profile.CharacterID] = profile
	return nil
}

func (r *mockRepository) Delete(_ context.Context, id string) error {
	if r.err != nil {
		return r.err
	}
	if _, ok := r.characters[id]; !ok {
		return ErrNotFound
	}
	delete(r.characters, id)
	delete(r.profiles, id)
	return nil
}

type mockGuildChecker struct {
	inGuild bool
	err     error
}

func (m *mockGuildChecker) IsInGuild(_ context.Context, _ string) (bool, error) {
	return m.inGuild, m.err
}

type mockFleaChecker struct {
	hasListings bool
	err         error
}

func (m *mockFleaChecker) HasActiveListings(_ context.Context, _ string) (bool, error) {
	return m.hasListings, m.err
}

type mockNewsPublisher struct {
	published []string
}

func (m *mockNewsPublisher) PublishNews(_ context.Context, category, title, content, author string, publishedAt time.Time) error {
	m.published = append(m.published, title)
	return nil
}

func TestService_CreateAndGet(t *testing.T) {
	repo := newMockRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Missing or invalid job/gender rejection via CreateWithOptions
	if _, err := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{}); err == nil {
		t.Fatal("expected error when creating character with missing job and gender, got nil")
	}
	if _, err := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "starter", Gender: "m"}); err == nil {
		t.Fatal("expected error when creating character with fictional starter job, got nil")
	}
	if _, err := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "job-01", Gender: "unspecified"}); err == nil {
		t.Fatal("expected error when creating character with fictional unspecified gender, got nil")
	}

	// 2. Successful creation via Create using authentic defaults (job-01, m)
	defaultChar, err := service.Create(context.Background(), "player-default", "DefaultHero")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if defaultChar.JobID != "job-01" || defaultChar.Gender != "m" {
		t.Fatalf("expected job-01 and m, got %+v", defaultChar)
	}

	// 2. Successful creation with valid starter job and gender
	char, err := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	if err != nil {
		t.Fatalf("CreateWithOptions() error = %v", err)
	}
	if char.Name != "Hero" || char.PlayerID != "player-1" || char.JobID != "job-01" || char.Gender != "m" {
		t.Fatalf("unexpected char: %+v", char)
	}

	fetched, err := service.Get(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if fetched.ID != char.ID {
		t.Fatalf("fetched ID mismatch: %s != %s", fetched.ID, char.ID)
	}

	list, err := service.ListByPlayer(context.Background(), "player-1")
	if err != nil || len(list) != 1 {
		t.Fatalf("ListByPlayer() error = %v, len = %d", err, len(list))
	}
}

func TestService_ChangeName(t *testing.T) {
	repo := newMockRepository()
	news := &mockNewsPublisher{}
	guildChecker := &mockGuildChecker{}
	fleaChecker := &mockFleaChecker{}

	service, err := NewService(repo,
		WithNewsPublisher(news),
		WithGuildChecker(guildChecker),
		WithFleaMarketChecker(fleaChecker),
	)
	if err != nil {
		t.Fatal(err)
	}

	// 1. Setup rich character
	char, _ := service.CreateWithOptions(context.Background(), "player-1", "OldHero", CreationOptions{JobID: "job-01", Gender: "m"})
	char.Money = 600000
	_ = repo.Update(context.Background(), char)

	// 2. Successful name change
	renamed, err := service.ChangeName(context.Background(), char.ID, "NewHero")
	if err != nil {
		t.Fatalf("ChangeName() failed: %v", err)
	}
	if renamed.Name != "NewHero" {
		t.Fatalf("expected name NewHero, got %s", renamed.Name)
	}
	if renamed.Money != 100000 {
		t.Fatalf("expected remaining money 100000, got %d", renamed.Money)
	}
	if len(news.published) != 1 {
		t.Fatalf("expected news broadcast, got %d", len(news.published))
	}

	// 3. Same name rejection
	if _, err := service.ChangeName(context.Background(), char.ID, "NewHero"); !errors.Is(err, ErrSameName) {
		t.Fatalf("expected ErrSameName, got %v", err)
	}

	// 4. Insufficient gold (< 500,000 G)
	renamed.Money = 499999
	_ = repo.Update(context.Background(), renamed)
	if _, err := service.ChangeName(context.Background(), char.ID, "AnotherName"); !errors.Is(err, ErrInsufficientGold) {
		t.Fatalf("expected ErrInsufficientGold, got %v", err)
	}

	// 5. Reset gold & test taken name
	renamed.Money = 600000
	_ = repo.Update(context.Background(), renamed)
	_, _ = service.CreateWithOptions(context.Background(), "player-2", "TakenHero", CreationOptions{JobID: "job-01", Gender: "m"})
	if _, err := service.ChangeName(context.Background(), char.ID, "TakenHero"); !errors.Is(err, ErrNameAlreadyTaken) {
		t.Fatalf("expected ErrNameAlreadyTaken, got %v", err)
	}

	// 6. Guild membership block
	guildChecker.inGuild = true
	if _, err := service.ChangeName(context.Background(), char.ID, "ValidName"); !errors.Is(err, ErrInGuildDisallowed) {
		t.Fatalf("expected ErrInGuildDisallowed, got %v", err)
	}
	guildChecker.inGuild = false

	// 7. Active flea market listing block
	fleaChecker.hasListings = true
	if _, err := service.ChangeName(context.Background(), char.ID, "ValidName"); !errors.Is(err, ErrActiveMarketDisallowed) {
		t.Fatalf("expected ErrActiveMarketDisallowed, got %v", err)
	}
	fleaChecker.hasListings = false

	// 8. Invalid name formats
	invalidNames := []string{
		"",
		"   ",
		"Hero\u3000Name",        // Japanese space
		"Hero Name",             // Space
		"Hero,Name",             // Comma
		"Hero;Name",             // Semicolon
		"Hero\"Name",            // Quote
		"Hero'Name",             // Single quote
		"Hero&Name",             // Ampersand
		"Hero<Name",             // Less than
		"Hero>Name",             // Greater than
		"Hero\\Name",            // Backslash
		"Hero/Name",             // Slash
		"Hero@Name",             // At
		"Hero＠Name",             // Fullwidth at
		strings.Repeat("A", 33), // Too long
	}
	for _, inv := range invalidNames {
		if _, err := service.ChangeName(context.Background(), char.ID, inv); !errors.Is(err, ErrInvalidName) {
			t.Errorf("expected ErrInvalidName for %q, got %v", inv, err)
		}
	}
}

func TestService_ChangeGender(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	char, _ := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{
		JobID:  "job-01",
		Gender: "m",
	})
	char.Money = 20000
	_ = repo.Update(context.Background(), char)

	// 1. Successful change to female on gender-free job (job-01)
	updated, err := service.ChangeGender(context.Background(), char.ID, "f")
	if err != nil {
		t.Fatalf("ChangeGender() failed: %v", err)
	}
	if updated.Gender != "f" {
		t.Fatalf("expected gender f, got %s", updated.Gender)
	}
	if updated.Money != 10000 {
		t.Fatalf("expected money 10000, got %d", updated.Money)
	}

	// 2. Same gender rejection (f -> f)
	if _, err := service.ChangeGender(context.Background(), char.ID, "female"); !errors.Is(err, ErrSameGender) {
		t.Fatalf("expected ErrSameGender, got %v", err)
	}
	// Check money unchanged
	current, _ := repo.FindByID(context.Background(), char.ID)
	if current.Money != 10000 {
		t.Fatalf("expected money 10000 after ErrSameGender, got %d", current.Money)
	}

	// 3. Insufficient gold (< 10,000 G)
	updated.Money = 9999
	_ = repo.Update(context.Background(), updated)
	if _, err := service.ChangeGender(context.Background(), char.ID, "m"); !errors.Is(err, ErrInsufficientGold) {
		t.Fatalf("expected ErrInsufficientGold, got %v", err)
	}
	current, _ = repo.FindByID(context.Background(), char.ID)
	if current.Money != 9999 || current.Gender != "f" {
		t.Fatalf("expected money 9999 and gender f after ErrInsufficientGold, got money=%d gender=%s", current.Money, current.Gender)
	}

	// 4. Invalid gender
	updated.Money = 20000
	_ = repo.Update(context.Background(), updated)
	if _, err := service.ChangeGender(context.Background(), char.ID, "invalid-gender"); !errors.Is(err, ErrInvalidGender) {
		t.Fatalf("expected ErrInvalidGender, got %v", err)
	}
	current, _ = repo.FindByID(context.Background(), char.ID)
	if current.Money != 20000 || current.Gender != "f" {
		t.Fatalf("expected money 20000 and gender f after ErrInvalidGender, got money=%d gender=%s", current.Money, current.Gender)
	}

	// 5. Job gender restrictions: job-13 (吟遊詩人, male-only)
	job13Char, err := service.Create(context.Background(), "player-1", "Bard")
	if err != nil {
		t.Fatalf("failed to create Bard: %v", err)
	}
	job13Char.JobID = "job-13"
	job13Char.Gender = "m"
	job13Char.Money = 30000
	_ = repo.Update(context.Background(), job13Char)

	if _, err := service.ChangeGender(context.Background(), job13Char.ID, "f"); !errors.Is(err, ErrGenderIncompatibleWithJob) {
		t.Fatalf("expected ErrGenderIncompatibleWithJob for job-13 to f, got %v", err)
	}
	saved13, _ := repo.FindByID(context.Background(), job13Char.ID)
	if saved13.Money != 30000 || saved13.Gender != "m" {
		t.Fatalf("expected money 30000 and gender m preserved for job-13, got money=%d gender=%s", saved13.Money, saved13.Gender)
	}

	// 6. Job gender restrictions: job-14 (踊り子, female-only)
	job14Char, err := service.Create(context.Background(), "player-1", "Dancer")
	if err != nil {
		t.Fatalf("failed to create Dancer: %v", err)
	}
	job14Char.JobID = "job-14"
	job14Char.Gender = "f"
	job14Char.Money = 30000
	_ = repo.Update(context.Background(), job14Char)

	if _, err := service.ChangeGender(context.Background(), job14Char.ID, "m"); !errors.Is(err, ErrGenderIncompatibleWithJob) {
		t.Fatalf("expected ErrGenderIncompatibleWithJob for job-14 to m, got %v", err)
	}
	saved14, _ := repo.FindByID(context.Background(), job14Char.ID)
	if saved14.Money != 30000 || saved14.Gender != "f" {
		t.Fatalf("expected money 30000 and gender f preserved for job-14, got money=%d gender=%s", saved14.Money, saved14.Gender)
	}

	// 7. Job gender restrictions: job-15 (黒魔術師, male-only)
	job15Char, err := service.Create(context.Background(), "player-1", "BlackMage")
	if err != nil {
		t.Fatalf("failed to create BlackMage: %v", err)
	}
	job15Char.JobID = "job-15"
	job15Char.Gender = "m"
	job15Char.Money = 30000
	_ = repo.Update(context.Background(), job15Char)

	if _, err := service.ChangeGender(context.Background(), job15Char.ID, "f"); !errors.Is(err, ErrGenderIncompatibleWithJob) {
		t.Fatalf("expected ErrGenderIncompatibleWithJob for job-15 to f, got %v", err)
	}
	saved15, _ := repo.FindByID(context.Background(), job15Char.ID)
	if saved15.Money != 30000 || saved15.Gender != "m" {
		t.Fatalf("expected money 30000 and gender m preserved for job-15, got money=%d gender=%s", saved15.Money, saved15.Gender)
	}

	// 8. Job gender restrictions: job-16 (白魔術師, female-only)
	job16Char, err := service.Create(context.Background(), "player-1", "WhiteMage")
	if err != nil {
		t.Fatalf("failed to create WhiteMage: %v", err)
	}
	job16Char.JobID = "job-16"
	job16Char.Gender = "f"
	job16Char.Money = 30000
	_ = repo.Update(context.Background(), job16Char)

	if _, err := service.ChangeGender(context.Background(), job16Char.ID, "m"); !errors.Is(err, ErrGenderIncompatibleWithJob) {
		t.Fatalf("expected ErrGenderIncompatibleWithJob for job-16 to m, got %v", err)
	}
	saved16, _ := repo.FindByID(context.Background(), job16Char.ID)
	if saved16.Money != 30000 || saved16.Gender != "f" {
		t.Fatalf("expected money 30000 and gender f preserved for job-16, got money=%d gender=%s", saved16.Money, saved16.Gender)
	}

	// 9. Job-14 same gender rejection occurs before job check
	if _, err := service.ChangeGender(context.Background(), job14Char.ID, "f"); !errors.Is(err, ErrSameGender) {
		t.Fatalf("expected ErrSameGender for job-14 to f, got %v", err)
	}

	// 10. Job-14 insufficient gold rejection occurs before job check
	job14Char.Money = 5000
	_ = repo.Update(context.Background(), job14Char)
	if _, err := service.ChangeGender(context.Background(), job14Char.ID, "m"); !errors.Is(err, ErrInsufficientGold) {
		t.Fatalf("expected ErrInsufficientGold for job-14 with 5000G to m, got %v", err)
	}

	// 11. Custom WithJobDefinitionProvider option test
	customCatalog, _ := corejob.NewCatalog([]corejob.Definition{
		{ID: "custom-job", Name: "CustomJob", RequiredGender: "f"},
	})
	customService, _ := NewService(repo, WithJobDefinitionProvider(customCatalog))
	customChar, _ := customService.Create(context.Background(), "player-1", "CustomChar")
	customChar.JobID = "custom-job"
	customChar.Gender = "f"
	customChar.Money = 30000
	_ = repo.Update(context.Background(), customChar)

	if _, err := customService.ChangeGender(context.Background(), customChar.ID, "m"); !errors.Is(err, ErrGenderIncompatibleWithJob) {
		t.Fatalf("expected ErrGenderIncompatibleWithJob from custom job provider, got %v", err)
	}

	// 12. Repository / transaction update failure
	repo.err = errors.New("database update error")
	if _, err := service.ChangeGender(context.Background(), char.ID, "m"); err == nil {
		t.Fatal("expected error on repo failure, got nil")
	}
	repo.err = nil

	// Verify state unchanged after failed transaction
	afterTxFail, _ := repo.FindByID(context.Background(), char.ID)
	if afterTxFail.Money != 20000 || afterTxFail.Gender != "f" {
		t.Fatalf("expected money 20000 and gender f preserved after tx failure, got money=%d gender=%s", afterTxFail.Money, afterTxFail.Gender)
	}
}

func TestService_ProfileOperations(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	char, _ := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "job-01", Gender: "m"})

	// 1. Get default profile
	view, err := service.GetProfile(context.Background(), char.ID)
	if err != nil {
		t.Fatalf("GetProfile() failed: %v", err)
	}
	if view.Character.ID != char.ID {
		t.Fatalf("view character ID mismatch: %s != %s", view.Character.ID, char.ID)
	}

	// 2. Update profile
	comment := "I am a mighty adventurer."
	avatarURL := "https://example.com/avatar.png"
	auraEffect := 3
	bio := map[string]string{
		"hobby":     "Fishing",
		"like_food": "Apple",
	}

	updatedProfile, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment:    &comment,
		AvatarURL:  &avatarURL,
		AuraEffect: &auraEffect,
		BioData:    bio,
	})
	if err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if updatedProfile.Comment != comment || updatedProfile.AvatarURL != avatarURL || updatedProfile.AuraEffect != 3 || updatedProfile.BioData["hobby"] != "Fishing" {
		t.Fatalf("unexpected profile data: %+v", updatedProfile)
	}

	// 3. Validation errors
	tooLongComment := strings.Repeat("あ", 161)
	if _, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &tooLongComment,
	}); !errors.Is(err, ErrCommentTooLong) {
		t.Fatalf("expected ErrCommentTooLong, got %v", err)
	}

	invalidURL := "ftp://invalid-url.com/img.png"
	if _, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		AvatarURL: &invalidURL,
	}); !errors.Is(err, ErrInvalidAvatarURL) {
		t.Fatalf("expected ErrInvalidAvatarURL, got %v", err)
	}

	longKeyBio := map[string]string{
		strings.Repeat("k", 33): "value",
	}
	if _, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		BioData: longKeyBio,
	}); !errors.Is(err, ErrBioKeyTooLong) {
		t.Fatalf("expected ErrBioKeyTooLong, got %v", err)
	}

	// 4. Aura effect validation (0..8 valid, < 0 or > 8 invalid)
	validAuras := []int{0, 1, 8}
	for _, aura := range validAuras {
		val := aura
		prof, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
			AuraEffect: &val,
		})
		if err != nil || prof.AuraEffect != aura {
			t.Errorf("expected valid aura effect %d, got err %v", aura, err)
		}
	}

	invalidAuras := []int{-1, 9, 100}
	for _, aura := range invalidAuras {
		val := aura
		if _, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
			AuraEffect: &val,
		}); !errors.Is(err, ErrInvalidAuraEffect) {
			t.Errorf("expected ErrInvalidAuraEffect for %d, got %v", aura, err)
		}
	}
}

func TestService_UploadAvatar(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	char, _ := service.CreateWithOptions(context.Background(), "player-1", "Hero", CreationOptions{JobID: "job-01", Gender: "m"})

	// 1. Valid PNG upload
	pngHeader := []byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR\x00\x00\x00\x01\x00\x00\x00\x01\x08\x06\x00\x00\x00\x1f\x15c4")
	dataURI, err := service.UploadAvatar(context.Background(), char.ID, "avatar.png", "image/png", pngHeader)
	if err != nil {
		t.Fatalf("UploadAvatar() failed: %v", err)
	}
	if !strings.HasPrefix(dataURI, "data:image/png;base64,") {
		t.Fatalf("expected data:image/png URI, got %s", dataURI)
	}

	// 2. Empty data rejection
	if _, err := service.UploadAvatar(context.Background(), char.ID, "empty.png", "image/png", []byte{}); !errors.Is(err, ErrInvalidImageFormat) {
		t.Fatalf("expected ErrInvalidImageFormat, got %v", err)
	}

	// 3. Oversized data rejection (> 2 MB)
	hugeData := make([]byte, 2*1024*1024+1)
	if _, err := service.UploadAvatar(context.Background(), char.ID, "large.png", "image/png", hugeData); !errors.Is(err, ErrImageTooLarge) {
		t.Fatalf("expected ErrImageTooLarge, got %v", err)
	}
}

func TestService_GetNamingHallDialogue(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	dialogue := service.GetNamingHallDialogue()
	if dialogue.NPCName != "@マリナン" || dialogue.LocationTitle != "命名の館" || dialogue.NameChangeCost != 500000 || dialogue.GenderChangeCost != 10000 {
		t.Fatalf("unexpected dialogue: %+v", dialogue)
	}
}

func TestService_Delete(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully deletes character and invokes cleanup hooks", func(t *testing.T) {
		repo := newMockRepository()
		var hookRan bool
		hook := CleanupHookFunc(func(ctx context.Context, characterID string) error {
			hookRan = true
			return nil
		})

		service, _ := NewService(repo, WithCleanupHook(hook))
		char, err := service.CreateWithOptions(ctx, "player-123", "Hero", CreationOptions{JobID: "job-01", Gender: "m"})
		if err != nil {
			t.Fatalf("CreateWithOptions() error = %v", err)
		}

		if err := service.Delete(ctx, "player-123", char.ID); err != nil {
			t.Fatalf("Delete() error = %v", err)
		}

		if !hookRan {
			t.Errorf("expected cleanup hook to run")
		}

		if _, err := service.Get(ctx, char.ID); !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound after deletion, got %v", err)
		}
	})

	t.Run("rejects deletion by non-owner player", func(t *testing.T) {
		repo := newMockRepository()
		service, _ := NewService(repo)
		char, _ := service.CreateWithOptions(ctx, "player-owner", "OwnerHero", CreationOptions{JobID: "job-01", Gender: "m"})

		err := service.Delete(ctx, "player-intruder", char.ID)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("allows deletion when playerID is empty (system/admin delete)", func(t *testing.T) {
		repo := newMockRepository()
		service, _ := NewService(repo)
		char, _ := service.CreateWithOptions(ctx, "player-owner", "OwnerHero", CreationOptions{JobID: "job-01", Gender: "m"})

		if err := service.Delete(ctx, "", char.ID); err != nil {
			t.Fatalf("expected nil error for admin delete, got %v", err)
		}
	})

	t.Run("returns ErrNotFound for nonexistent character", func(t *testing.T) {
		repo := newMockRepository()
		service, _ := NewService(repo)

		if err := service.Delete(ctx, "player-123", "nonexistent-id"); !errors.Is(err, ErrNotFound) {
			t.Fatalf("expected ErrNotFound, got %v", err)
		}
	})
}

func TestService_Profile_ReadErrorPropagationAndPreservation(t *testing.T) {
	repo := newMockRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	char, err := service.Create(context.Background(), "player-1", "Hero")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial healthy profile with avatar and bio
	initialComment := "Initial comment"
	initialAvatar := "https://example.com/saved-avatar.png"
	initialBio := map[string]string{"key": "saved bio"}
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment:   &initialComment,
		AvatarURL: &initialAvatar,
		BioData:   initialBio,
	})
	if err != nil {
		t.Fatalf("initial UpdateProfile failed: %v", err)
	}

	// 2. GetProfile propagates storage errors
	storageErr := errors.New("database connection lost")
	repo.getProfileErr = storageErr
	_, err = service.GetProfile(context.Background(), char.ID)
	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storageErr from GetProfile, got %v", err)
	}

	// 3. GetProfile propagates cancellation errors
	repo.getProfileErr = nil
	canceledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = service.GetProfile(canceledCtx, char.ID)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled from GetProfile, got %v", err)
	}

	// 4. UpdateProfile fails when profile read fails, makes 0 SaveProfile calls, preserves existing data
	repo.saveProfileCalls = 0
	repo.getProfileErr = storageErr
	newComment := "New comment attempting to overwrite"
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &newComment,
	})
	if !errors.Is(err, storageErr) {
		t.Fatalf("expected storageErr from UpdateProfile, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Fatalf("expected 0 SaveProfile calls on read error, got %d", repo.saveProfileCalls)
	}

	// In storage, existing AvatarURL and BioData are still intact
	stored := repo.profiles[char.ID]
	if stored.AvatarURL != initialAvatar || stored.BioData["key"] != "saved bio" || stored.Comment != initialComment {
		t.Fatalf("stored profile was corrupted or overwritten on read error: %+v", stored)
	}

	// 5. Healthy partial update preserves saved AvatarURL and BioData
	repo.getProfileErr = nil
	updatedComment := "Healthy updated comment"
	updated, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &updatedComment,
	})
	if err != nil {
		t.Fatalf("healthy UpdateProfile failed: %v", err)
	}
	if updated.Comment != updatedComment {
		t.Errorf("comment not updated: %s", updated.Comment)
	}
	if updated.AvatarURL != initialAvatar {
		t.Errorf("avatar not preserved in return: %s", updated.AvatarURL)
	}
	if updated.BioData["key"] != "saved bio" {
		t.Errorf("bio data not preserved in return: %v", updated.BioData)
	}

	storedAfter := repo.profiles[char.ID]
	if storedAfter.AvatarURL != initialAvatar || storedAfter.BioData["key"] != "saved bio" || storedAfter.Comment != updatedComment {
		t.Fatalf("stored profile lost avatar or bio during partial update: %+v", storedAfter)
	}

	// 6. Genuinely absent profile returns default profile without error
	absentChar, err := service.Create(context.Background(), "player-1", "AbsentHero")
	if err != nil {
		t.Fatal(err)
	}
	absentView, err := service.GetProfile(context.Background(), absentChar.ID)
	if err != nil {
		t.Fatalf("expected nil error for absent profile, got %v", err)
	}
	if absentView.Profile.CharacterID != absentChar.ID || absentView.Profile.AvatarURL != "" {
		t.Fatalf("unexpected default profile for absent char: %+v", absentView.Profile)
	}
}

func TestService_UpdateProfile_ValidationFailureMakesZeroSaveCalls(t *testing.T) {
	repo := newMockRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	char, err := service.Create(context.Background(), "player-1", "Hero")
	if err != nil {
		t.Fatal(err)
	}

	// 1. Initial profile
	initComment := "Valid initial comment"
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &initComment,
	})
	if err != nil {
		t.Fatalf("setup UpdateProfile failed: %v", err)
	}
	repo.saveProfileCalls = 0

	// 2. Overlong comment
	longComment := strings.Repeat("a", MaxCommentLength+1)
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &longComment,
	})
	if !errors.Is(err, ErrCommentTooLong) {
		t.Fatalf("expected ErrCommentTooLong, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls on invalid comment, got %d", repo.saveProfileCalls)
	}

	// 3. Invalid avatar URL
	badAvatar := "ftp://example.com/avatar.png"
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		AvatarURL: &badAvatar,
	})
	if !errors.Is(err, ErrInvalidAvatarURL) {
		t.Fatalf("expected ErrInvalidAvatarURL, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls on invalid avatar, got %d", repo.saveProfileCalls)
	}

	// 4. Invalid aura effect
	badAura := 99
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		AuraEffect: &badAura,
	})
	if !errors.Is(err, ErrInvalidAuraEffect) {
		t.Fatalf("expected ErrInvalidAuraEffect, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls on invalid aura, got %d", repo.saveProfileCalls)
	}

	// 5. Invalid bio key
	longBioKey := map[string]string{strings.Repeat("k", MaxBioKeyLength+1): "val"}
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		BioData: longBioKey,
	})
	if !errors.Is(err, ErrBioKeyTooLong) {
		t.Fatalf("expected ErrBioKeyTooLong, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls on invalid bio key, got %d", repo.saveProfileCalls)
	}

	// 6. Invalid bio value
	longBioVal := map[string]string{"valid_key": strings.Repeat("v", MaxBioFieldLength+1)}
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		BioData: longBioVal,
	})
	if !errors.Is(err, ErrBioValueTooLong) {
		t.Fatalf("expected ErrBioValueTooLong, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls on invalid bio value, got %d", repo.saveProfileCalls)
	}

	// Ensure stored profile remains unchanged
	stored := repo.profiles[char.ID]
	if stored.Comment != initComment {
		t.Fatalf("stored profile was mutated after validation errors: %+v", stored)
	}
}

type mockTransactionProvider struct {
	runInTxCalls int
	fn           func(ctx context.Context, fn func(ctx context.Context) error) error
}

func (m *mockTransactionProvider) RunInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	m.runInTxCalls++
	if m.fn != nil {
		return m.fn(ctx, fn)
	}
	return fn(ctx)
}

func TestService_UpdateProfile_InvokesTransactionProvider(t *testing.T) {
	repo := newMockRepository()
	txProv := &mockTransactionProvider{}
	service, err := NewService(repo, WithTransactionProvider(txProv))
	if err != nil {
		t.Fatal(err)
	}

	char, err := service.Create(context.Background(), "player-1", "TxHero")
	if err != nil {
		t.Fatal(err)
	}

	comment := "Tx comment"
	_, err = service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment: &comment,
	})
	if err != nil {
		t.Fatalf("UpdateProfile failed: %v", err)
	}
	if txProv.runInTxCalls == 0 {
		t.Fatalf("expected RunInTx to be called, got %d", txProv.runInTxCalls)
	}
}

func TestService_UpdateProfile_NonexistentCharacterReturnsNotFound(t *testing.T) {
	repo := newMockRepository()
	service, err := NewService(repo)
	if err != nil {
		t.Fatal(err)
	}

	comment := "Any comment"
	_, err = service.UpdateProfile(context.Background(), "nonexistent-id", UpdateProfileRequest{
		Comment: &comment,
	})
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("expected ErrNotFound for nonexistent character, got %v", err)
	}
	if repo.saveProfileCalls != 0 {
		t.Errorf("expected 0 SaveProfile calls for nonexistent character, got %d", repo.saveProfileCalls)
	}
}
