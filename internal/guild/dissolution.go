package guild

import (
	"context"
	"errors"
	"strings"
	"time"
)

func (s *Service) publishDissolutionNews(ctx context.Context, guildName string, publishedAt time.Time) {
	if s.newsPub != nil {
		msg := "ギルド『" + guildName + "』が解散しました"
		//lint:ignore error-swallow best-effort news publication for guild dissolution
		_ = s.newsPub.PublishNews(ctx, "guild", msg, msg, "System", publishedAt)
	}
}

// Disband allows the guild master to disband the guild (join_guild.cgi:kaisan).
// Transaction: RunInTx (authorization under Rank-7 lock and dissolution; news follows success).
// Lock Order: guilds(7) -> guild_members(7).
func (s *Service) Disband(ctx context.Context, guildID string, leaderCharID string) error {
	guildID = strings.TrimSpace(guildID)
	if guildID == "" {
		return ErrInvalidGuildID
	}
	leaderCharID = strings.TrimSpace(leaderCharID)
	if leaderCharID == "" {
		return ErrCharacterNotFound
	}

	var disbandedGuildName string
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		lockedGuild, members, err := s.repo.GetGuildForUpdate(txCtx, guildID)
		if err != nil {
			return err
		}

		if err := authorizeCurrentLeader(lockedGuild, members, leaderCharID); err != nil {
			return err
		}

		if err := s.repo.DisbandGuild(txCtx, guildID); err != nil {
			return err
		}
		disbandedGuildName = lockedGuild.Name
		return nil
	})
	if err != nil {
		return err
	}
	s.publishDissolutionNews(ctx, disbandedGuildName, s.nowFunc())
	return nil
}

// DisbandInactiveGuilds inspects guilds with no member activity for >= 20 days and disbands them (join_guild.cgi:check_dead_guild).
func (s *Service) DisbandInactiveGuilds(ctx context.Context, now time.Time, limit int) (int, []string, error) {
	cutoff := now.Add(-InactivityDisbandDuration)
	inactive, err := s.repo.ListInactiveGuilds(ctx, cutoff, limit)
	if err != nil {
		return 0, nil, err
	}

	var disbanded []string
	for _, candidate := range inactive {
		var disbandedName string
		disbandedThis := false
		err := s.runInTx(ctx, func(txCtx context.Context) error {
			g, _, err := s.repo.GetGuildForUpdate(txCtx, candidate.ID)
			if err != nil {
				if errors.Is(err, ErrGuildNotFound) {
					// Guild was already deleted or disbanded concurrently.
					return nil
				}
				return err
			}

			// Recheck inactivity under Rank-7 lock. If the guild was touched
			// or updated at or after cutoff, it has reactivated and must be kept.
			if !g.LastActiveAt.Before(cutoff) {
				return nil
			}

			if err := s.repo.DisbandGuild(txCtx, candidate.ID); err != nil {
				if errors.Is(err, ErrGuildNotFound) {
					// Guild was already deleted or disbanded concurrently.
					return nil
				}
				return err
			}
			disbandedThis = true
			disbandedName = g.Name
			return nil
		})
		if err != nil {
			return len(disbanded), disbanded, err
		}
		if disbandedThis {
			disbanded = append(disbanded, candidate.ID)
			s.publishDissolutionNews(ctx, disbandedName, now)
		}
	}

	return len(disbanded), disbanded, nil
}
