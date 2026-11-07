package home

import (
	"context"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// Legacy item 058 (lib/_data.cgi:929–955).
const (
	personalityBookName    = "エッチな本"
	personalityBookItemNo  = 58
	personalityBookTrait   = "ムッツリスケベ"
	personalityBookMessage = "性格：" + personalityBookTrait
)

// PersonalityRecorder writes the character-owned login message and profile
// personality inside the caller's transaction.
type PersonalityRecorder interface {
	SetLoginMessage(ctx context.Context, playerID, characterID, message string) error
	// SetPersonality preserves other profile fields and publishes the public announcement.
	SetPersonality(ctx context.Context, characterID, personality string) error
}

// SetPersonalityRecorder registers the cross-domain personality writer.
func (s *Service) SetPersonalityRecorder(r PersonalityRecorder) { s.personality = r }

// applyPersonalityBook changes the login message, profile personality (with its
// news) and appearance inside the item transaction.
func (s *Service) applyPersonalityBook(ctx context.Context, char *corecharacter.Character, look *appearanceUndo) (string, error) {
	if s.personality == nil {
		return "", fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, personalityBookName)
	}
	if err := s.personality.SetLoginMessage(ctx, char.PlayerID, char.ID, personalityBookMessage); err != nil {
		return "", err
	}
	if err := s.personality.SetPersonality(ctx, char.ID, personalityBookTrait); err != nil {
		return "", err
	}
	// shortcut: placeholder job-08 art until production job images exist (IMG-006).
	icon := "assets/images/placeholders/job/appearance.svg#job-08-" + appearanceGender(char.Gender)
	if err := s.wearAppearance(ctx, char.ID, personalityBookItemNo, personalityBookName, icon, look); err != nil {
		return "", err
	}
	return fmt.Sprintf("%sはエッチな本をじっくり読んだ………%sの性格がムッツリスケベになった！", char.Name, char.Name), nil
}

// appearanceGender matches the costume gender branch.
func appearanceGender(gender string) string {
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(gender)), "f") || gender == "2" {
		return "f"
	}
	return "m"
}
