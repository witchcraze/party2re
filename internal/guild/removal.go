package guild

import (
	"context"
	"errors"
	"strings"
)

// removeAuthorizedMember shares the locked removal workflow for expulsion and rejection.
// pendingOnly restricts rejection to applicants; Kick selects the notification from
// the target's state under the same lock, without re-entering an unlocked workflow.
func (s *Service) removeAuthorizedMember(ctx context.Context, guildID, leaderID, targetID string, pendingOnly bool) error {
	var g Guild
	var pending bool
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}
		var requester, target *Member
		for i := range members {
			if members[i].CharacterID == leaderID {
				requester = &members[i]
			}
			if members[i].CharacterID == targetID {
				target = &members[i]
			}
		}
		if lockedGuild.LeaderCharacterID != leaderID || requester == nil || requester.Role != RoleLeader || requester.IsPending {
			return ErrUnauthorized
		}
		if target == nil {
			return ErrTargetNotMember
		}
		if target.Role == RoleLeader {
			if pendingOnly {
				return ErrMemberNotPending
			}
			return ErrCannotKickLeader
		}
		if pendingOnly && !target.IsPending {
			return ErrMemberNotPending
		}
		if err := s.repo.RemoveMember(txCtx, guildID, targetID); err != nil {
			return err
		}
		s.touchActive(txCtx, guildID)
		g, pending = lockedGuild, target.IsPending
		return nil
	})
	if err != nil {
		return err
	}

	// Production letters persist through the incoming ambient context. An outer
	// transaction therefore rolls them back along with the membership mutation.
	if s.letterSender != nil && s.charReader != nil {
		leader, errL := s.charReader.FindByID(ctx, leaderID)
		target, errT := s.charReader.FindByID(ctx, targetID)
		if errL == nil && errT == nil {
			content := "【＋追放＋】" + g.Name + " (ギルマス " + leader.Name + ") から追放されました"
			if pending {
				content = "【＋不合格＋】残念ながら " + g.Name + " (ギルマス " + leader.Name + ") から参加を拒否されました"
			}
			//lint:ignore error-swallow existing best-effort membership notification
			_ = s.letterSender.SendLetter(ctx, leaderID, leader.Name, targetID, target.Name, content, leader.Color)
		}
	}
	return nil
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
		var (
			transferred bool
			lastErr     error
		)
		candidates := make([]Member, 0, len(remaining))
		for _, m := range remaining {
			if strings.Contains(m.Title, "ギルマス") {
				candidates = append(candidates, m)
			}
		}
		for _, m := range remaining {
			if !strings.Contains(m.Title, "ギルマス") {
				candidates = append(candidates, m)
			}
		}

		for _, cand := range candidates {
			err := s.repo.TransferLeadership(ctx, g.ID, characterID, cand.CharacterID)
			if err == nil {
				transferred = true
				break
			}
			if errors.Is(err, ErrCharacterAlreadyInGuild) {
				lastErr = err
				continue
			}
			return false, err
		}

		if !transferred {
			if lastErr != nil && errors.Is(lastErr, ErrCharacterAlreadyInGuild) {
				if err := s.repo.DisbandGuild(ctx, g.ID); err != nil {
					return false, err
				}
				return true, nil
			}
			return false, lastErr
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
