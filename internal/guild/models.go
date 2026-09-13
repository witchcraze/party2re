package guild

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

type Role string

const (
	RoleLeader Role = "leader"
	RoleMember Role = "member"
)

func (r Role) Valid() bool {
	switch r {
	case RoleLeader, RoleMember:
		return true
	default:
		return false
	}
}

const (
	CreationFee               = 5000 // Gold required to create a guild (5,000G in join_guild.cgi)
	MaxNameLength             = 32   // Maximum characters for guild name
	MaxNoticeLength           = 200  // Maximum characters for guild notice (200 in guild.cgi)
	MaxRoleTitleWidth         = 12   // Maximum visual width for custom role title (6 full-width / 12 half-width characters)
	DefaultTitleLeader        = "ギルマス"
	DefaultTitlePending       = "参加申請中"
	DefaultColor              = "#FFFFFF"
	NPCColor                  = "#FF69B4"
	DefaultMark               = "0"
	MarkChangeFee             = 3000                // Gold required to change guild mark (3,000G in join_guild.cgi)
	InactivityDisbandDuration = 20 * 24 * time.Hour // 20 days of inactivity before automatic disbandment (join_guild.cgi:29)
)

var (
	ErrInvalidGuildID               = errors.New("invalid guild ID")
	ErrInvalidGuildName             = errors.New("guild name must be between 1 and 32 characters")
	ErrNoticeTooLong                = errors.New("guild notice exceeds maximum allowed length")
	ErrGuildNotFound                = errors.New("guild not found")
	ErrGuildNameTaken               = errors.New("guild name is already taken")
	ErrCharacterNotFound            = errors.New("character not found")
	ErrCharacterAlreadyInGuild      = errors.New("character is already a member of a guild")
	ErrCharacterNotInGuild          = errors.New("character is not a member of this guild")
	ErrInsufficientFunds            = errors.New("character does not have enough gold")
	ErrUnauthorized                 = errors.New("unauthorized to perform this action in the guild")
	ErrCannotKickLeader             = errors.New("cannot kick the guild leader")
	ErrLeaderCannotLeaveWithMembers = errors.New("guild leader cannot leave while other members remain; transfer leadership or disband")
	ErrTargetNotMember              = errors.New("target character is not a member of the guild")
	ErrCannotAssignToLeader         = errors.New("cannot assign role title to the guild leader")
	ErrInvalidRoleTitle             = errors.New("invalid role title")
	ErrReservedRoleTitle            = errors.New("reserved role title cannot be used")
	ErrRoleTitleTooLong             = errors.New("role title exceeds maximum allowed length of 6 full-width or 12 half-width characters")
	ErrInvalidColorFormat           = errors.New("invalid color format, expected #RRGGBB hex code")
	ErrColorTaken                   = errors.New("guild color is already taken or reserved")
	ErrApplicationAlreadyPending    = errors.New("character already has a pending application")
	ErrApplicationNotFound          = errors.New("application not found")
	ErrMemberNotPending             = errors.New("target member is not pending approval")
	ErrMemberIsPending              = errors.New("member is pending approval")
	ErrEmptyCalloutMessage          = errors.New("callout message cannot be empty")
	ErrCalloutMessageTooLong        = errors.New("callout message exceeds maximum allowed length")
	ErrInvalidWallpaper             = errors.New("invalid wallpaper")
	ErrInvalidMark                  = errors.New("invalid guild mark")
)

var hexColorRegex = regexp.MustCompile(`^#[0-9A-F]{6}$`)

// CalculateTitleWidth returns the visual display width of the string:
// ASCII runes (<= 127) count as 1, non-ASCII runes (e.g. full-width Japanese characters) count as 2.
func CalculateTitleWidth(s string) int {
	width := 0
	for _, r := range s {
		if r > 127 {
			width += 2
		} else {
			width += 1
		}
	}
	return width
}

// ValidateRoleTitle validates custom role titles according to legacy specifications (guild.cgi:ataeru):
// - Must not be empty
// - Must not contain whitespace (half-width or full-width)
// - Must not contain invalid characters: , ; " ' & < > @ ＠
// - Must not be reserved titles: "参加申請中" or "ギルマス"
// - Visual width must be <= 12 (6 full-width / 12 half-width characters)
func ValidateRoleTitle(title string) error {
	if title == "" {
		return ErrInvalidRoleTitle
	}
	if strings.ContainsAny(title, " \t\r\n\u3000") {
		return ErrInvalidRoleTitle
	}
	if strings.ContainsAny(title, ",;\"'&<>@＠") {
		return ErrInvalidRoleTitle
	}
	if title == "参加申請中" || title == "ギルマス" {
		return ErrReservedRoleTitle
	}
	if CalculateTitleWidth(title) > MaxRoleTitleWidth {
		return ErrRoleTitleTooLong
	}
	return nil
}

// ValidateColorFormat validates that a hex color string is in #RRGGBB format and normalizes to uppercase.
func ValidateColorFormat(color string) (string, error) {
	upper := strings.ToUpper(strings.TrimSpace(color))
	if !hexColorRegex.MatchString(upper) {
		return "", ErrInvalidColorFormat
	}
	return upper, nil
}

type Guild struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	LeaderCharacterID string    `json:"leader_character_id"`
	Points            int64     `json:"points"` // Guild Points (gpoint)
	Notice            string    `json:"notice"`
	Color             string    `json:"color"`
	Bgimg             string    `json:"bgimg"`
	Mark              string    `json:"mark"`
	LastActiveAt      time.Time `json:"last_active_at"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Member struct {
	GuildID     string    `json:"guild_id"`
	CharacterID string    `json:"character_id"`
	Role        Role      `json:"role"`
	Title       string    `json:"title"`
	IsPending   bool      `json:"is_pending"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Detail struct {
	Guild   Guild    `json:"guild"`
	Members []Member `json:"members"`
}

// WallpaperPrices defines the complete legacy wallpaper catalog and pricing in Gold (_data.cgi:330-388).
var WallpaperPrices = map[string]int{
	"none.gif":       0,
	"farm.gif":       1000,
	"lot.gif":        1000,
	"sp_change.gif":  1500,
	"exile.gif":      1500,
	"depot.gif":      1500,
	"item.gif":       2000,
	"medal.gif":      2000,
	"bar.gif":        2500,
	"casino.gif":     2500,
	"goods.gif":      2500,
	"armor.gif":      3000,
	"job_change.gif": 3000,
	"park.gif":       3500,
	"auction.gif":    4000,
	"weapon.gif":     5000,
	"event.gif":      5000,
	"stage0.gif":     6000,
	"stage1.gif":     6500,
	"stage2.gif":     7000,
	"stage3.gif":     7500,
	"stage4.gif":     8000,
	"stage5.gif":     8500,
	"stage6.gif":     9000,
	"stage7.gif":     9500,
	"stage8.gif":     10000,
	"stage9.gif":     10500,
	"stage10.gif":    11000,
	"stage11.gif":    11500,
	"stage12.gif":    12000,
	"stage13.gif":    12500,
	"stage14.gif":    13000,
	"stage15.gif":    20000,
	"stage16.gif":    22000,
	"stage17.gif":    24000,
	"stage18.gif":    25000,
	"stage19.gif":    30000,
	"stage20.gif":    50000,
	"stage22.gif":    10000,
	"stage23.gif":    10000,
	"stage24.gif":    10000,
	"stage25.gif":    10000,
	"stage26_1.gif":  10000,
	"stage26_2.gif":  10000,
	"stage26_3.gif":  10000,
	"stage26_4.gif":  10000,
	"map1.gif":       3000,
	"map2.gif":       4500,
	"map3.gif":       5000,
	"map14.gif":      10000,
}

// LookupWallpaperPrice validates and retrieves the normalized wallpaper filename and price.
// Accepts base names (e.g. "farm") or full file names (e.g. "farm.gif").
func LookupWallpaperPrice(target string) (string, int, error) {
	trimmed := strings.TrimSpace(target)
	if trimmed == "" {
		return "", 0, ErrInvalidWallpaper
	}
	if !strings.HasSuffix(trimmed, ".gif") {
		trimmed = trimmed + ".gif"
	}
	price, ok := WallpaperPrices[trimmed]
	if !ok {
		return "", 0, ErrInvalidWallpaper
	}
	return trimmed, price, nil
}
