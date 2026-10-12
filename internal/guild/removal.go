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

var (
	errCandidateIneligible = errors.New("candidate ineligible for guild leadership")
	errRosterChanged       = errors.New("guild roster changed during departure evaluation")
)

// leaveInternal executes the departure workflow for a character leaving a guild.
// If the character is not the leader, they are removed directly.
// If the character is the leader:
//  1. Candidate successors are evaluated (title containing "ギルマス" prioritized first, then roster order).
//  2. For each candidate, acquires Rank 2 lock on candidate character followed by Rank 7 guild lock,
//     re-verifies active affiliation under lock, and atomically transfers leadership and removes old leader.
//  3. If all candidate attempts fail or no candidates exist, disbands the guild and publishes dissolution news.
//
// Roster state is verified inside locked transactions to avoid TOCTOU races against concurrent joins/leaves.
func (s *Service) leaveInternal(ctx context.Context, guildID string, characterID string, ignoreMissing bool) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	characterID = strings.TrimSpace(characterID)
	if characterID == "" {
		if ignoreMissing {
			return nil
		}
		return ErrCharacterNotFound
	}

	ineligible := make(map[string]bool)
	const maxLeaveAttempts = 50
	retries := 0

	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		if retries >= maxLeaveAttempts {
			return errors.New("guild leave exceeded maximum retry attempts")
		}

		_, members, err := s.repo.GetGuild(ctx, guildID)
		if err != nil {
			if ignoreMissing && (errors.Is(err, ErrGuildNotFound) || errors.Is(err, ErrCharacterNotInGuild)) {
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
			if ignoreMissing {
				return nil
			}
			return ErrCharacterNotInGuild
		}

		// 1. Non-leader departure workflow under Rank 7 lock
		if currentMember.Role != RoleLeader {
			var (
				disbanded          bool
				disbandedGuildName string
			)
			err := s.runInTx(ctx, func(txCtx context.Context) error {
				lockedGuild, lockedMembers, err := s.repo.GetGuildForUpdate(txCtx, guildID)
				if err != nil {
					return err
				}
				var m *Member
				for i := range lockedMembers {
					if lockedMembers[i].CharacterID == characterID {
						m = &lockedMembers[i]
						break
					}
				}
				if m == nil {
					if ignoreMissing {
						return nil
					}
					return ErrCharacterNotInGuild
				}
				if m.Role == RoleLeader {
					return errRosterChanged
				}

				var remainingLocked int
				for _, lm := range lockedMembers {
					if lm.CharacterID != characterID {
						remainingLocked++
					}
				}
				if remainingLocked == 0 {
					if err := s.repo.DisbandGuild(txCtx, guildID); err != nil {
						return err
					}
					disbanded = true
					disbandedGuildName = lockedGuild.Name
					return nil
				}

				if err := s.repo.RemoveMember(txCtx, guildID, characterID); err != nil {
					return err
				}
				s.touchActive(txCtx, guildID)
				return nil
			})
			if err == nil {
				if disbanded {
					s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
				}
				return nil
			}
			if errors.Is(err, errRosterChanged) {
				retries++
				continue
			}
			return err
		}

		// 2. Leader departure workflow: evaluate untried candidates in rank order
		var candidates []Member
		for _, m := range members {
			if m.CharacterID != characterID && !ineligible[m.CharacterID] {
				if strings.Contains(m.Title, "ギルマス") {
					candidates = append(candidates, m)
				}
			}
		}
		for _, m := range members {
			if m.CharacterID != characterID && !ineligible[m.CharacterID] {
				if !strings.Contains(m.Title, "ギルマス") {
					candidates = append(candidates, m)
				}
			}
		}

		if len(candidates) > 0 {
			candID := candidates[0].CharacterID
			var (
				disbanded          bool
				disbandedGuildName string
			)
			tErr := s.runInTx(ctx, func(txCtx context.Context) error {
				// Rank 2: Lock candidate character
				if err := s.repo.LockCharacterForUpdate(txCtx, candID); err != nil {
					if errors.Is(err, ErrCharacterNotFound) {
						return errCandidateIneligible
					}
					return err
				}

				// Rank 7: Lock guild
				lockedGuild, lockedMembers, err := s.repo.GetGuildForUpdate(txCtx, guildID)
				if err != nil {
					return err
				}

				var leaderInLocked *Member
				for i := range lockedMembers {
					if lockedMembers[i].CharacterID == characterID {
						leaderInLocked = &lockedMembers[i]
						break
					}
				}
				if leaderInLocked == nil {
					if ignoreMissing {
						return nil
					}
					return ErrCharacterNotInGuild
				}

				var remainingLocked int
				for _, lm := range lockedMembers {
					if lm.CharacterID != characterID {
						remainingLocked++
					}
				}
				if remainingLocked == 0 {
					if err := s.repo.DisbandGuild(txCtx, guildID); err != nil {
						return err
					}
					disbanded = true
					disbandedGuildName = lockedGuild.Name
					return nil
				}

				if leaderInLocked.Role != RoleLeader {
					if err := s.repo.RemoveMember(txCtx, guildID, characterID); err != nil {
						return err
					}
					s.touchActive(txCtx, guildID)
					return nil
				}
				if lockedGuild.LeaderCharacterID != "" && lockedGuild.LeaderCharacterID != characterID {
					return ErrUnauthorized
				}

				var candInLocked *Member
				for i := range lockedMembers {
					if lockedMembers[i].CharacterID == candID {
						candInLocked = &lockedMembers[i]
						break
					}
				}
				if candInLocked == nil {
					return errCandidateIneligible
				}

				if candInLocked.IsPending {
					activeGuild, _, err := s.repo.GetGuildByCharacter(txCtx, candID)
					if err == nil {
						if activeGuild.ID != guildID {
							return errCandidateIneligible
						}
					} else if !errors.Is(err, ErrCharacterNotInGuild) {
						return err
					}
				}

				if err := s.repo.TransferLeadership(txCtx, guildID, characterID, candID); err != nil {
					if errors.Is(err, ErrCharacterAlreadyInGuild) {
						return errCandidateIneligible
					}
					return err
				}

				if err := s.repo.RemoveMember(txCtx, guildID, characterID); err != nil {
					return err
				}
				s.touchActive(txCtx, guildID)
				return nil
			})

			if tErr == nil {
				if disbanded {
					s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
				}
				return nil
			}
			if errors.Is(tErr, errCandidateIneligible) {
				ineligible[candID] = true
				continue
			}
			if errors.Is(tErr, errRosterChanged) {
				retries++
				continue
			}
			return tErr
		}

		// 3. Disband workflow when no untried candidates remain
		var (
			disbanded          bool
			disbandedGuildName string
		)
		err = s.runInTx(ctx, func(txCtx context.Context) error {
			lockedGuild, lockedMembers, err := s.repo.GetGuildForUpdate(txCtx, guildID)
			if err != nil {
				return err
			}

			var leaderInLocked *Member
			for i := range lockedMembers {
				if lockedMembers[i].CharacterID == characterID {
					leaderInLocked = &lockedMembers[i]
					break
				}
			}
			if leaderInLocked == nil {
				if ignoreMissing {
					return nil
				}
				return ErrCharacterNotInGuild
			}

			var remainingLocked int
			for _, lm := range lockedMembers {
				if lm.CharacterID != characterID {
					remainingLocked++
				}
			}

			if leaderInLocked.Role != RoleLeader {
				if remainingLocked == 0 {
					if err := s.repo.DisbandGuild(txCtx, guildID); err != nil {
						return err
					}
					disbanded = true
					disbandedGuildName = lockedGuild.Name
					return nil
				}
				if err := s.repo.RemoveMember(txCtx, guildID, characterID); err != nil {
					return err
				}
				s.touchActive(txCtx, guildID)
				return nil
			}
			if lockedGuild.LeaderCharacterID != "" && lockedGuild.LeaderCharacterID != characterID {
				return ErrUnauthorized
			}

			// Verify if any remaining member in lockedMembers is NOT in ineligible map!
			for _, lm := range lockedMembers {
				if lm.CharacterID != characterID && !ineligible[lm.CharacterID] {
					return errRosterChanged
				}
			}

			if err := s.repo.DisbandGuild(txCtx, guildID); err != nil {
				return err
			}
			disbanded = true
			disbandedGuildName = lockedGuild.Name
			return nil
		})

		if err == nil {
			if disbanded {
				s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
			}
			return nil
		}
		if errors.Is(err, errRosterChanged) {
			retries++
			continue
		}
		return err
	}
}

// Leave removes a character from the guild (join_guild.cgi:dattai).
// If the character is the sole member, the guild is disbanded and dissolution news is published.
// If the character is the leader and other members remain, leadership is transferred before removal.
func (s *Service) Leave(ctx context.Context, guildID string, characterID string) error {
	return s.leaveInternal(ctx, guildID, characterID, false)
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
	g, _, err := s.repo.GetGuildByCharacter(ctx, characterID)
	if err != nil {
		if errors.Is(err, ErrCharacterNotInGuild) || errors.Is(err, ErrGuildNotFound) {
			return nil
		}
		return err
	}
	return s.leaveInternal(ctx, g.ID, characterID, true)
}
