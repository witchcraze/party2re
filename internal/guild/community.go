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
func (s *Service) ApplyToJoin(ctx context.Context, guildID string, applicantID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	applicantID = strings.TrimSpace(applicantID)
	if applicantID == "" {
		return ErrCharacterNotFound
	}

	// Verify applicant is not already in a guild or has an existing pending application
	if _, existingMember, err := s.repo.GetGuildByCharacter(ctx, applicantID); err == nil {
		if existingMember.IsPending {
			return ErrApplicationAlreadyPending
		}
		return ErrCharacterAlreadyInGuild
	}

	g, _, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}

	m := Member{
		GuildID:     guildID,
		CharacterID: applicantID,
		Role:        RoleMember,
		Title:       DefaultTitlePending,
		IsPending:   true,
		JoinedAt:    s.nowFunc().UTC(),
	}

	if _, err := s.repo.AddMember(ctx, m); err != nil {
		return err
	}
	s.touchActive(ctx, guildID)

	// Send application letter to the Guild Master (join_guild.cgi:216-219)
	if s.letterSender != nil && s.charReader != nil {
		applicantChar, errA := s.charReader.FindByID(ctx, applicantID)
		leaderChar, errL := s.charReader.FindByID(ctx, g.LeaderCharacterID)
		if errA == nil && errL == nil {
			content := "【＋参加申請＋】" + g.Name + " 入団希望者 " + applicantChar.Name
			_ = s.letterSender.SendLetter(ctx, applicantID, applicantChar.Name, g.LeaderCharacterID, leaderChar.Name, content, applicantChar.Color)
		}
	}

	return nil
}

// ApproveApplication approves a pending applicant, assigns their role title, and clears is_pending (guild.cgi:ataeru).
// Sends an acceptance letter to the applicant.
func (s *Service) ApproveApplication(ctx context.Context, guildID string, leaderID string, applicantID string, title string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	leaderID = strings.TrimSpace(leaderID)
	applicantID = strings.TrimSpace(applicantID)
	if leaderID == "" || applicantID == "" {
		return ErrCharacterNotFound
	}

	normalizedTitle, err := ValidateRoleTitle(title)
	if err != nil {
		return err
	}

	g, members, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return err
	}
	if g.LeaderCharacterID != leaderID {
		return ErrUnauthorized
	}

	var target *Member
	for i := range members {
		if members[i].CharacterID == applicantID {
			target = &members[i]
			break
		}
	}
	if target == nil {
		return ErrTargetNotMember
	}
	if !target.IsPending {
		return ErrMemberNotPending
	}

	if err := s.repo.ApproveMember(ctx, guildID, applicantID, normalizedTitle); err != nil {
		return err
	}
	s.touchActive(ctx, guildID)

	// Send acceptance letter to applicant (guild.cgi:236-237)
	if s.letterSender != nil && s.charReader != nil {
		leaderChar, errL := s.charReader.FindByID(ctx, leaderID)
		applicantChar, errA := s.charReader.FindByID(ctx, applicantID)
		if errL == nil && errA == nil {
			content := "【＋参加許可証＋】" + g.Name + " (ギルマス " + leaderChar.Name + ") から参加許可をもらいました"
			_ = s.letterSender.SendLetter(ctx, leaderID, leaderChar.Name, applicantID, applicantChar.Name, content, leaderChar.Color)
		}
	}

	return nil
}

// RejectApplication rejects a pending applicant and removes them from the guild (guild.cgi:ataeru/tsuihou).
// Sends a rejection letter after successful removal, using the incoming ambient context.
// Transaction: RunInTx (authorization, pending-state validation and removal).
// Lock Order: guilds(7) -> guild_members(7).
func (s *Service) RejectApplication(ctx context.Context, guildID string, leaderID string, applicantID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	leaderID = strings.TrimSpace(leaderID)
	applicantID = strings.TrimSpace(applicantID)
	if leaderID == "" || applicantID == "" {
		return ErrCharacterNotFound
	}

	return s.removeAuthorizedMember(ctx, guildID, leaderID, applicantID, true)
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

	senderName := senderID
	senderColor := "#000000"
	if s.charReader != nil {
		if sc, err := s.charReader.FindByID(ctx, senderID); err == nil {
			senderName = sc.Name
			if sc.Color != "" {
				senderColor = sc.Color
			}
		}
	}

	// Deliver letter to each member (including pending applicants in roster order)
	if s.letterSender != nil {
		for _, m := range members {
			recName := m.CharacterID
			if s.charReader != nil {
				if rc, err := s.charReader.FindByID(ctx, m.CharacterID); err == nil {
					recName = rc.Name
				}
			}
			if err := s.letterSender.SendLetter(ctx, senderID, senderName, m.CharacterID, recName, sanitizedMessage, senderColor); err != nil {
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
	if trimmedMark == "" {
		return corecharacter.Character{}, ErrInvalidMark
	}

	g, _, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if g.LeaderCharacterID != leaderID {
		return corecharacter.Character{}, ErrUnauthorized
	}

	return s.repo.UpdateMark(ctx, guildID, trimmedMark, MarkChangeFee, leaderID)
}

// ChangeWallpaper changes the guild's background wallpaper for the catalog price paid by the guild master (join_guild.cgi:kabegami).
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

	g, _, err := s.repo.GetGuild(ctx, guildID)
	if err != nil {
		return corecharacter.Character{}, err
	}
	if g.LeaderCharacterID != leaderID {
		return corecharacter.Character{}, ErrUnauthorized
	}

	return s.repo.UpdateWallpaper(ctx, guildID, normalized, price, leaderID)
}

func (s *Service) removeMemberInternal(ctx context.Context, g Guild, members []Member, characterID string, role Role) (bool, error) {
	var remaining []Member
	for _, m := range members {
		if m.CharacterID != characterID {
			remaining = append(remaining, m)
		}
	}

	if len(remaining) == 0 {
		if err := s.repo.DisbandGuild(ctx, g.ID); err != nil {
			return false, err
		}
		return true, nil
	}

	if role == RoleLeader {
		successorID := remaining[0].CharacterID
		for _, m := range remaining {
			if strings.Contains(m.Title, "ギルマス") {
				successorID = m.CharacterID
				break
			}
		}
		if err := s.repo.TransferLeadership(ctx, g.ID, characterID, successorID); err != nil {
			return false, err
		}
	}

	if err := s.repo.RemoveMember(ctx, g.ID, characterID); err != nil {
		return false, err
	}
	s.touchActive(ctx, g.ID)
	return false, nil
}

// Leave removes a character from the guild (join_guild.cgi:dattai).
// If the character is the sole member, the guild is disbanded and dissolution news is published.
// If the character is the leader and other members remain, leadership is transferred before removal.
func (s *Service) Leave(ctx context.Context, guildID string, characterID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return ErrCharacterNotFound
	}

	var (
		disbanded          bool
		disbandedGuildName string
	)
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		g, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		var currentMember *Member
		for i := range members {
			if members[i].CharacterID == characterID {
				currentMember = &members[i]
				break
			}
		}
		if currentMember == nil {
			return ErrCharacterNotInGuild
		}

		var dErr error
		disbanded, dErr = s.removeMemberInternal(txCtx, g, members, characterID, currentMember.Role)
		if dErr != nil {
			return dErr
		}
		if disbanded {
			disbandedGuildName = g.Name
		}
		return nil
	})
	if err != nil {
		return err
	}

	if disbanded {
		s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
	}
	return nil
}

// RemoveCharacterFromGuild removes a character from their guild upon admin action or deletion (lib/system.cgi:delete_guild_member).
// If the character is not in a guild, it returns nil.
// If the character is the sole member, the guild is disbanded and dissolution news is published.
// If the character is the leader and other members exist, leadership is transferred before removal.
func (s *Service) RemoveCharacterFromGuild(ctx context.Context, characterID string) error {
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		return nil
	}

	var (
		disbanded          bool
		disbandedGuildName string
	)
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		g, _, err := s.repo.GetGuildByCharacter(txCtx, characterID)
		if err != nil {
			if errors.Is(err, ErrCharacterNotInGuild) || errors.Is(err, ErrGuildNotFound) {
				return nil
			}
			return err
		}

		gLocked, members, err := s.repo.GetGuildForUpdate(txCtx, g.ID)
		if err != nil {
			if errors.Is(err, ErrGuildNotFound) {
				return nil
			}
			return err
		}

		var currentMember *Member
		for i := range members {
			if members[i].CharacterID == characterID {
				currentMember = &members[i]
				break
			}
		}
		if currentMember == nil {
			return nil
		}

		var dErr error
		disbanded, dErr = s.removeMemberInternal(txCtx, gLocked, members, characterID, currentMember.Role)
		if dErr != nil {
			return dErr
		}
		if disbanded {
			disbandedGuildName = gLocked.Name
		}
		return nil
	})
	if err != nil {
		return err
	}

	if disbanded {
		s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
	}
	return nil
}
