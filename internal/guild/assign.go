package guild

import (
	"context"
	"errors"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// AssignOutcome is the roster change made by the guild leader.
type AssignOutcome string

const (
	AssignApproved                AssignOutcome = "approved"
	AssignRetitled                AssignOutcome = "retitled"
	AssignRejected                AssignOutcome = "rejected"
	AssignExpelled                AssignOutcome = "expelled"
	AssignStaleApplicationRemoved AssignOutcome = "stale_application_removed"
)

// Assign applies the current guild leader's title or removal command.
// Transaction: RunInTx covers authorization, affiliation and roster changes.
// Lock Order: characters(2) -> guilds(7) -> guild_members(7).
func (s *Service) Assign(ctx context.Context, guildID, leaderID, targetID, title string) (AssignOutcome, error) {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return "", ErrInvalidGuildID
	}
	leaderID = strings.TrimSpace(leaderID)
	targetID = strings.TrimSpace(targetID)
	if leaderID == "" || targetID == "" {
		return "", ErrCharacterNotFound
	}
	removing := title == RemovalTitle
	if !removing {
		normalized, err := ValidateRoleTitle(title)
		if err != nil {
			return "", err
		}
		title = normalized
	}

	var outcome AssignOutcome
	var guildName string
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		if err := s.repo.LockCharacterForUpdate(txCtx, targetID); err != nil {
			if errors.Is(err, ErrCharacterNotFound) {
				return ErrTargetNotMember
			}
			return err
		}
		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}
		if err := authorizeCurrentLeader(lockedGuild, members, leaderID); err != nil {
			return err
		}
		var target *Member
		for i := range members {
			if members[i].CharacterID == targetID {
				target = &members[i]
				break
			}
		}
		if target == nil {
			return ErrTargetNotMember
		}
		if target.Role == RoleLeader {
			if removing {
				return ErrCannotKickLeader
			}
			return ErrCannotAssignToLeader
		}

		switch {
		case target.IsPending:
			outcome = AssignApproved
			if removing {
				outcome = AssignRejected
			}
			// The legacy roster rewrite discards the application before either branch.
			activeGuild, _, err := s.repo.GetGuildByCharacter(txCtx, targetID)
			switch {
			case err == nil && activeGuild.ID != guildID:
				outcome = AssignStaleApplicationRemoved
			case err == nil:
				return ErrMemberNotPending
			case !errors.Is(err, ErrCharacterNotInGuild):
				return err
			}
		case removing:
			outcome = AssignExpelled
		default:
			outcome = AssignRetitled
		}

		switch outcome {
		case AssignStaleApplicationRemoved, AssignRejected, AssignExpelled:
			if err := s.repo.RemoveMember(txCtx, guildID, targetID); err != nil {
				return err
			}
		case AssignApproved:
			if err := s.repo.ApproveMember(txCtx, guildID, targetID, title); err != nil {
				return err
			}
			if err := s.memories.Record(txCtx, targetID, corecharacter.MemoryGuildJoin, lockedGuild.Name+"ギルドに加入"); err != nil {
				return err
			}
		case AssignRetitled:
			if err := s.repo.AssignCustomRole(txCtx, guildID, targetID, title); err != nil {
				return err
			}
		}
		s.touchActive(txCtx, guildID)
		guildName = lockedGuild.Name
		return nil
	})
	if err != nil {
		return "", err
	}
	if outcome == AssignRetitled || outcome == AssignStaleApplicationRemoved || s.letterSender == nil || s.charReader == nil {
		return outcome, nil
	}

	// The incoming transaction, if present, rolls the letter back with the roster change.
	leader, leaderErr := s.charReader.FindByID(ctx, leaderID)
	_, targetErr := s.charReader.FindByID(ctx, targetID)
	if leaderErr == nil && targetErr == nil {
		sender := guildName + " (ギルマス " + leader.Name + ")"
		var content string
		switch outcome {
		case AssignApproved:
			content = "【＋参加許可証＋】" + sender + " から参加許可をもらいました"
		case AssignRejected:
			content = "【＋不合格＋】残念ながら " + sender + " から参加を拒否されました"
		case AssignExpelled:
			content = "【＋追放＋】" + sender + " から追放されました"
		}
		//lint:ignore error-swallow existing best-effort membership notification
		_ = s.letterSender.SendLetter(ctx, leaderID, targetID, content)
	}
	return outcome, nil
}
