package guild

import (
	"context"
	"errors"
	"slices"
	"strings"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/economy"
)

// errRosterChanged reports membership that changed between unlocked discovery and
// its locks; the departure is retried with a fresh lock set.
var errRosterChanged = errors.New("guild roster changed during departure")

const maxLeaveAttempts = 50

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
	return s.depart(ctx, guildID, []string{characterID})
}

// RemoveFromGuilds removes characters from their active guilds upon admin action or deletion
// (lib/system.cgi:delete_guild_member), applying Leave's succession and dissolution rules in
// ascending character ID order. Characters without an active guild are skipped. Inside an
// ambient transaction every change, including dissolution news, commits with the caller.
func (s *Service) RemoveFromGuilds(ctx context.Context, characterIDs ...string) error {
	var ids []string
	for _, id := range characterIDs {
		if id = strings.TrimSpace(id); id != "" {
			ids = append(ids, id)
		}
	}
	// Normalize departure order; departLocked separately orders the full lock set.
	slices.Sort(ids)
	ids = slices.Compact(ids)
	if len(ids) == 0 {
		return nil
	}
	return s.depart(ctx, "", ids)
}

// depart retries departLocked until it observes a stable roster. guildID is the
// explicit Leave target; empty means each character's active guild.
func (s *Service) depart(ctx context.Context, guildID string, ids []string) error {
	for attempt := 0; ; attempt++ {
		if err := ctx.Err(); err != nil {
			return err
		}
		if attempt >= maxLeaveAttempts {
			return errors.New("guild leave exceeded maximum retry attempts")
		}
		err := s.runInTx(ctx, func(txCtx context.Context) error {
			return s.departLocked(txCtx, guildID, ids)
		})
		if !errors.Is(err, errRosterChanged) {
			return err
		}
	}
}

// departLocked locks the departing characters and every member of their guilds in
// ascending ID order (Rank 2), then the guilds in ascending ID order (Rank 7), and
// applies each departure under those locks. Membership changes lock the character
// first, so a locked candidate's eligibility cannot change during evaluation.
func (s *Service) departLocked(ctx context.Context, guildID string, ids []string) error {
	guildOf := make(map[string]string, len(ids))
	lockIDs := slices.Clone(ids)
	var guildIDs []string
	for _, id := range ids {
		gid := guildID
		if gid == "" {
			g, _, err := s.repo.GetGuildByCharacter(ctx, id)
			if errors.Is(err, ErrCharacterNotInGuild) || errors.Is(err, ErrGuildNotFound) {
				continue
			}
			if err != nil {
				return err
			}
			gid = g.ID
		}
		_, members, err := s.repo.GetGuild(ctx, gid)
		if err != nil {
			if guildID == "" && errors.Is(err, ErrGuildNotFound) {
				return errRosterChanged
			}
			return err
		}
		guildOf[id] = gid
		guildIDs = append(guildIDs, gid)
		for _, m := range members {
			lockIDs = append(lockIDs, m.CharacterID)
		}
	}

	locked := make(map[string]bool, len(lockIDs))
	if err := economy.LockInOrder(ctx, lockIDs, func(ctx context.Context, id string) error {
		err := s.repo.LockCharacterForUpdate(ctx, id)
		if err != nil && !errors.Is(err, ErrCharacterNotFound) {
			return err
		}
		locked[id] = err == nil
		return nil
	}); err != nil {
		return err
	}
	if err := economy.LockInOrder(ctx, guildIDs, func(ctx context.Context, gid string) error {
		if _, _, err := s.repo.GetGuildForUpdate(ctx, gid); err != nil {
			if guildID == "" && errors.Is(err, ErrGuildNotFound) {
				return errRosterChanged
			}
			return err
		}
		return nil
	}); err != nil {
		return err
	}

	if guildID == "" {
		// Joining or creating a guild locks the character, so membership is now stable.
		for _, id := range ids {
			g, _, err := s.repo.GetGuildByCharacter(ctx, id)
			if err != nil && !errors.Is(err, ErrCharacterNotInGuild) && !errors.Is(err, ErrGuildNotFound) {
				return err
			}
			if g.ID != guildOf[id] {
				return errRosterChanged
			}
		}
	}

	for _, id := range ids {
		gid, ok := guildOf[id]
		if !ok {
			continue
		}
		if err := s.departOne(ctx, gid, id, locked, guildID != ""); err != nil {
			return err
		}
	}
	return nil
}

// departOne removes one character from a guild whose members are already locked.
// A leader hands over to the first eligible member, titles containing "ギルマス"
// first and then roster order; without one the guild is disbanded.
func (s *Service) departOne(ctx context.Context, guildID, characterID string, locked map[string]bool, strict bool) error {
	g, members, err := s.repo.GetGuildForUpdate(ctx, guildID)
	if err != nil {
		if !strict && errors.Is(err, ErrGuildNotFound) {
			return nil
		}
		return err
	}
	var self *Member
	var titled, others []Member
	for i := range members {
		switch {
		case members[i].CharacterID == characterID:
			self = &members[i]
		case strings.Contains(members[i].Title, "ギルマス"):
			titled = append(titled, members[i])
		default:
			others = append(others, members[i])
		}
	}
	if self == nil {
		if strict {
			return ErrCharacterNotInGuild
		}
		return nil
	}
	if strict && !self.IsPending {
		// Only an explicit ＠だったい is a milestone; recorded while still a member so the entry shows the guild.
		if err := s.memories.Record(ctx, characterID, corecharacter.MemoryGuildLeave, g.Name+"から脱退する"); err != nil {
			return err
		}
	}
	candidates := slices.Concat(titled, others)
	if len(candidates) == 0 {
		return s.disband(ctx, g)
	}
	if self.Role != RoleLeader {
		return s.removeMember(ctx, guildID, characterID)
	}
	if g.LeaderCharacterID != "" && g.LeaderCharacterID != characterID {
		return ErrUnauthorized
	}

	for _, c := range candidates {
		ok, exists := locked[c.CharacterID]
		if !exists {
			return errRosterChanged
		}
		if !ok {
			continue
		}
		if c.IsPending {
			active, _, err := s.repo.GetGuildByCharacter(ctx, c.CharacterID)
			if err != nil && !errors.Is(err, ErrCharacterNotInGuild) {
				return err
			}
			if err == nil && active.ID != guildID {
				continue
			}
		}
		if err := s.repo.TransferLeadership(ctx, guildID, characterID, c.CharacterID); err != nil {
			if errors.Is(err, ErrCharacterAlreadyInGuild) {
				continue
			}
			return err
		}
		return s.removeMember(ctx, guildID, characterID)
	}
	return s.disband(ctx, g)
}

func (s *Service) removeMember(ctx context.Context, guildID, characterID string) error {
	if err := s.repo.RemoveMember(ctx, guildID, characterID); err != nil {
		return err
	}
	s.touchActive(ctx, guildID)
	return nil
}

// disband dissolves the guild and publishes its news through the same transaction context.
func (s *Service) disband(ctx context.Context, g Guild) error {
	if err := s.repo.DisbandGuild(ctx, g.ID); err != nil {
		return err
	}
	s.publishDissolutionNews(ctx, g.Name, s.nowFunc())
	return nil
}
