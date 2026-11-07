package home

import (
	"context"
	"fmt"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/core/timer"
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
// news) and appearance. The non-transactional appearance store is written last
// so a database failure rolls back before it is touched.
func (s *Service) applyPersonalityBook(ctx context.Context, char *corecharacter.Character) (string, error) {
	if s.personality == nil {
		return "", fmt.Errorf("%w: %sはここでは使えません", ErrCannotUseHere, personalityBookName)
	}
	if err := s.personality.SetLoginMessage(ctx, char.PlayerID, char.ID, personalityBookMessage); err != nil {
		return "", err
	}
	if err := s.personality.SetPersonality(ctx, char.ID, personalityBookTrait); err != nil {
		return "", err
	}
	if s.costumeApplier != nil {
		// shortcut: a placeholder job-08 appearance until production job art exists (IMG-004).
		icon := "assets/images/placeholders/job/job-default.svg#job-08-" + appearanceGender(char.Gender)
		expiresAt := timer.NextMidnightJST(s.nowFunc()).UTC()
		if err := s.costumeApplier.ApplyCostume(ctx, char.ID, personalityBookItemNo, personalityBookName, icon, expiresAt); err != nil {
			return "", err
		}
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
