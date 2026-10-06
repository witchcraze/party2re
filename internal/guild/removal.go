package guild

import "context"

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
