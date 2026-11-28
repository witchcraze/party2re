package guild

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/validation"
)

// ApplyToJoin registers an applicant in the guild with is_pending = true (join_guild.cgi:sanka).
// Sends an application notification letter to the guild master.
// Transaction: RunInTx checks affiliation and the roster and inserts the application under locks.
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) ApplyToJoin(ctx context.Context, guildID string, applicantID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	applicantID = strings.TrimSpace(applicantID)
	if applicantID == "" {
		return ErrCharacterNotFound
	}

	var g Guild
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, applicantID); err != nil {
			return err
		}
		// Verify applicant is not already in an active guild (join_guild.cgi:sanka)
		if _, _, err := s.repo.GetGuildByCharacter(txCtx, applicantID); err == nil {
			return ErrCharacterAlreadyInGuild
		} else if !errors.Is(err, ErrCharacterNotInGuild) {
			return err
		}
		var members []Member
		var err error
		if g, members, err = s.repo.GetGuildForUpdate(txCtx, guildID); err != nil {
			return err
		}
		// Verify applicant does not already have a pending application or membership in this guild (join_guild.cgi:sanka)
		for i := range members {
			if members[i].CharacterID == applicantID {
				if members[i].IsPending {
					return ErrApplicationAlreadyPending
				}
				return ErrCharacterAlreadyInGuild
			}
		}
		if _, err := s.repo.AddMember(txCtx, Member{
			GuildID:     guildID,
			CharacterID: applicantID,
			Role:        RoleMember,
			Title:       DefaultTitlePending,
			IsPending:   true,
			JoinedAt:    s.nowFunc().UTC(),
		}); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
	if err != nil {
		return err
	}

	// Send application letter to the Guild Master (join_guild.cgi:216-219)
	if s.letterSender != nil && s.charReader != nil {
		applicantChar, errA := s.charReader.FindByID(ctx, applicantID)
		_, errL := s.charReader.FindByID(ctx, g.LeaderCharacterID)
		if errA == nil && errL == nil {
			content := "【＋参加申請＋】" + g.Name + " 入団希望者 " + applicantChar.Name
			_ = s.letterSender.SendLetter(ctx, applicantID, g.LeaderCharacterID, content)
		}
	}

	return nil
}

// BroadcastCallout sends a broadcast message to all guild members (including pending applicants) and awards +1 Guild Point (guild.cgi:yobikakeru).
func (s *Service) BroadcastCallout(ctx context.Context, guildID string, senderID string, message string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	senderID = strings.TrimSpace(senderID)
	if senderID == "" {
		return ErrCharacterNotFound
	}

	sanitizedMessage, err := validation.ValidateSingleLine(message, MaxNoticeLength)
	if err != nil {
		if errors.Is(err, validation.ErrEmpty) {
			return ErrEmptyCalloutMessage
		}
		if errors.Is(err, validation.ErrTooLong) {
			return ErrCalloutMessageTooLong
		}
		return err
	}

	_, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	var senderMember *Member
	for i := range members {
		if members[i].CharacterID == senderID {
			senderMember = &members[i]
			break
		}
	}
	if senderMember == nil || senderMember.IsPending {
		return ErrUnauthorized
	}

	// Deliver letter to each member (including pending applicants in roster order)
	if s.letterSender != nil {
		for _, m := range members {
			if err := s.letterSender.SendLetter(ctx, senderID, m.CharacterID, sanitizedMessage); err != nil {
				return err
			}
		}
	}

	// +1 Guild Point (guild.cgi:63)
	//lint:ignore error-swallow best-effort guild activity tracking
	_ = s.repo.AddPoints(ctx, guildID, 1)
	s.touchActive(ctx, guildID)
	return nil
}

// ChangeMark changes the guild's icon mark for a 3,000G fee paid by the guild master (join_guild.cgi:mark).
// Transaction: RunInTx (Rank 2 paying character lock, Rank 7 guild authorization and wallet deduction; rollback on stale leader).
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) ChangeMark(ctx context.Context, guildID string, leaderID string, mark string) (corecharacter.Character, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return corecharacter.Character{}, ErrInvalidGuildID
	}
	leaderID = strings.TrimSpace(leaderID)
	if leaderID == "" {
		return corecharacter.Character{}, ErrCharacterNotFound
	}
	trimmedMark := strings.TrimSpace(mark)
	if err := ValidateMark(trimmedMark); err != nil {
		return corecharacter.Character{}, err
	}

	var updatedChar corecharacter.Character
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, leaderID); err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, leaderID); err != nil {
			return err
		}

		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		if err := authorizeCurrentLeader(lockedGuild, members, leaderID); err != nil {
			return err
		}

		updatedChar, err = s.repo.UpdateMark(txCtx, guildID, trimmedMark, MarkChangeFee, leaderID)
		if err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return updatedChar, nil
}

// ChangeWallpaper changes the guild's background wallpaper for the catalog price paid by the guild master (join_guild.cgi:kabegami).
// Transaction: RunInTx (Rank 2 paying character lock, Rank 7 guild authorization and wallet deduction; rollback on stale leader).
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) ChangeWallpaper(ctx context.Context, guildID string, leaderID string, wallpaper string) (corecharacter.Character, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return corecharacter.Character{}, ErrInvalidGuildID
	}
	leaderID = strings.TrimSpace(leaderID)
	if leaderID == "" {
		return corecharacter.Character{}, ErrCharacterNotFound
	}

	normalized, price, err := LookupWallpaperPrice(wallpaper)
	if err != nil {
		return corecharacter.Character{}, err
	}

	var updatedChar corecharacter.Character
	err = s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, leaderID); err != nil {
			return err
		}

		if err := s.checkMutation(txCtx, leaderID); err != nil {
			return err
		}

		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		if err := authorizeCurrentLeader(lockedGuild, members, leaderID); err != nil {
			return err
		}

		updatedChar, err = s.repo.UpdateWallpaper(txCtx, guildID, normalized, price, leaderID)
		if err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		return nil
	})
	if err != nil {
		return corecharacter.Character{}, err
	}
	return updatedChar, nil
}
