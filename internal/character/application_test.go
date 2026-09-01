package character

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

type mockRepository struct {
	characters map[string]corecharacter.Character
	profiles   map[string]Profile
	err        error
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

func (r *mockRepository) GetProfile(_ context.Context, characterID string) (Profile, error) {
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

	char, err := service.Create(context.Background(), "player-1", "Hero")
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if char.Name != "Hero" || char.PlayerID != "player-1" {
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
	char, _ := service.Create(context.Background(), "player-1", "OldHero")
	char.Money = 1000000
	_ = repo.Update(context.Background(), char)

	// 2. Successful name change
	renamed, err := service.ChangeName(context.Background(), char.ID, "NewHero")
	if err != nil {
		t.Fatalf("ChangeName() failed: %v", err)
	}
	if renamed.Name != "NewHero" {
		t.Fatalf("expected name NewHero, got %s", renamed.Name)
	}
	if renamed.Money != 500000 {
		t.Fatalf("expected remaining money 500000, got %d", renamed.Money)
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
	_, _ = service.Create(context.Background(), "player-2", "TakenHero")
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
		Gender: "m",
	})
	char.Money = 20000
	_ = repo.Update(context.Background(), char)

	// 1. Successful change to female
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

	// 2. Same gender rejection
	if _, err := service.ChangeGender(context.Background(), char.ID, "female"); !errors.Is(err, ErrSameGender) {
		t.Fatalf("expected ErrSameGender, got %v", err)
	}

	// 3. Insufficient gold (< 10,000 G)
	updated.Money = 9999
	_ = repo.Update(context.Background(), updated)
	if _, err := service.ChangeGender(context.Background(), char.ID, "m"); !errors.Is(err, ErrInsufficientGold) {
		t.Fatalf("expected ErrInsufficientGold, got %v", err)
	}

	// 4. Invalid gender
	updated.Money = 20000
	_ = repo.Update(context.Background(), updated)
	if _, err := service.ChangeGender(context.Background(), char.ID, "invalid-gender"); !errors.Is(err, ErrInvalidGender) {
		t.Fatalf("expected ErrInvalidGender, got %v", err)
	}
}

func TestService_ProfileOperations(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	char, _ := service.Create(context.Background(), "player-1", "Hero")

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
	bio := map[string]string{
		"hobby":     "Fishing",
		"like_food": "Apple",
	}

	updatedProfile, err := service.UpdateProfile(context.Background(), char.ID, UpdateProfileRequest{
		Comment:   &comment,
		AvatarURL: &avatarURL,
		BioData:   bio,
	})
	if err != nil {
		t.Fatalf("UpdateProfile() failed: %v", err)
	}
	if updatedProfile.Comment != comment || updatedProfile.AvatarURL != avatarURL || updatedProfile.BioData["hobby"] != "Fishing" {
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
}

func TestService_UploadAvatar(t *testing.T) {
	repo := newMockRepository()
	service, _ := NewService(repo)

	char, _ := service.Create(context.Background(), "player-1", "Hero")

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
		char, err := service.Create(ctx, "player-123", "Hero")
		if err != nil {
			t.Fatalf("Create() error = %v", err)
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
		char, _ := service.Create(ctx, "player-owner", "OwnerHero")

		err := service.Delete(ctx, "player-intruder", char.ID)
		if !errors.Is(err, ErrForbidden) {
			t.Fatalf("expected ErrForbidden, got %v", err)
		}
	})

	t.Run("allows deletion when playerID is empty (system/admin delete)", func(t *testing.T) {
		repo := newMockRepository()
		service, _ := NewService(repo)
		char, _ := service.Create(ctx, "player-owner", "OwnerHero")

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
