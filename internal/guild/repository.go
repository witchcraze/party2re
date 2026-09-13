package guild

import (
	"context"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// GuildReader defines read operations on guilds and their memberships.
type GuildReader interface {
	GetGuild(ctx context.Context, guildID string) (Guild, []Member, error)
	GetGuildByCharacter(ctx context.Context, characterID string) (Guild, Member, error)
	ListGuilds(ctx context.Context, offset, limit int) ([]Guild, error)
	IsColorTaken(ctx context.Context, color string, excludeGuildID string) (bool, error)
}

// GuildMemberWriter defines mutation operations on guild memberships and role titles.
type GuildMemberWriter interface {
	AddMember(ctx context.Context, member Member) (Member, error)
	RemoveMember(ctx context.Context, guildID string, characterID string) error
	TransferLeadership(ctx context.Context, guildID string, oldLeaderCharID string, newLeaderCharID string) error
	AssignCustomRole(ctx context.Context, guildID string, targetCharID string, title string) error
}

// GuildStateWriter defines mutation operations on guild entities, customization, and points.
type GuildStateWriter interface {
	CreateGuild(ctx context.Context, g Guild, creator Member, fee int) (Guild, Member, corecharacter.Character, error)
	UpdateNotice(ctx context.Context, guildID string, notice string) error
	UpdateColor(ctx context.Context, guildID string, color string) error
	AddPoints(ctx context.Context, guildID string, points int64) error
	AddGuildPoints(ctx context.Context, characterID string, points int) error
	UpdateBgimg(ctx context.Context, guildID string, bgimg string) error
	DisbandGuild(ctx context.Context, guildID string) error
}

// GuildWriter composes member and guild state mutation interfaces.
type GuildWriter interface {
	GuildMemberWriter
	GuildStateWriter
}

// Repository composes read and write operations for guild persistence.
type Repository interface {
	GuildReader
	GuildWriter
}
