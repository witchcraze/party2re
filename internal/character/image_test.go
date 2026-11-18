package character

import (
	"context"
	"errors"
	"testing"
)

type fixedImageUnlocks ImageUnlocks

func (u *fixedImageUnlocks) ImageUnlocks(context.Context, string) (ImageUnlocks, error) {
	return ImageUnlocks(*u), nil
}

// custom_image.cgi write_exe: aura always, avatar only after its unlock, and
// the bio and comment written elsewhere are kept.
func TestService_UpdateImage(t *testing.T) {
	ctx := context.Background()
	const oldAvatar = "https://i.imgur.com/old12.gif"
	for _, tc := range []struct {
		name       string
		unlocks    ImageUnlocks
		aura       int
		avatarID   string
		wantErr    error
		wantAvatar string
	}{
		{"image locked", ImageUnlocks{}, 1, "", ErrImageLocked, oldAvatar},
		{"aura only keeps avatar before its unlock", ImageUnlocks{Image: true}, 8, "", nil, oldAvatar},
		{"avatar input before its unlock", ImageUnlocks{Image: true}, 1, "abcde", ErrAvatarLocked, oldAvatar},
		{"avatar set", ImageUnlocks{Image: true, Avatar: true}, 0, "Ab3de12345", nil, "https://i.imgur.com/Ab3de12345.gif"},
		{"avatar cleared", ImageUnlocks{Image: true, Avatar: true}, 2, "", nil, ""},
		{"avatar ID too short", ImageUnlocks{Image: true, Avatar: true}, 2, "abcd", ErrInvalidAvatarURL, oldAvatar},
		{"avatar ID path", ImageUnlocks{Image: true, Avatar: true}, 2, "abcde.gif?x", ErrInvalidAvatarURL, oldAvatar},
		{"aura above range", ImageUnlocks{Image: true, Avatar: true}, 9, "", ErrInvalidAuraEffect, oldAvatar},
		{"aura below range", ImageUnlocks{Image: true, Avatar: true}, -1, "", ErrInvalidAuraEffect, oldAvatar},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := newMockRepository()
			unlocks := fixedImageUnlocks(tc.unlocks)
			service, _ := NewService(repo, WithImageUnlocks(&unlocks))
			char, _ := service.Create(ctx, "player-1", "Hero")
			repo.profiles[char.ID] = Profile{CharacterID: char.ID, Comment: "c", AvatarURL: oldAvatar, AuraEffect: 3, BioData: map[string]string{"hobby": "h"}}

			_, err := service.UpdateImage(ctx, char.ID, tc.aura, tc.avatarID)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			stored := repo.profiles[char.ID]
			wantAura := 3
			if tc.wantErr == nil {
				wantAura = tc.aura
			}
			if stored.AvatarURL != tc.wantAvatar || stored.AuraEffect != wantAura || stored.Comment != "c" || stored.BioData["hobby"] != "h" {
				t.Fatalf("stored %+v", stored)
			}
			view, err := service.GetImage(ctx, char.ID)
			if err != nil || view.AvatarURL != tc.wantAvatar || view.AuraEffect != wantAura || view.AvatarUnlocked != tc.unlocks.Avatar {
				t.Fatalf("GetImage = %+v, %v", view, err)
			}
		})
	}
}
