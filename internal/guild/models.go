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
	CreationFee        = 5000 // Gold required to create a guild (5,000G in join_guild.cgi)
	MaxNameLength      = 32   // Maximum characters for guild name
	MaxNoticeLength    = 200  // Maximum characters for guild notice (200 in guild.cgi)
	MaxRoleTitleWidth  = 12   // Maximum visual width for custom role title (6 full-width / 12 half-width characters)
	DefaultTitleLeader = "ギルマス"
	DefaultColor       = "#FFFFFF"
	NPCColor           = "#FF69B4"
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
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

type Member struct {
	GuildID     string    `json:"guild_id"`
	CharacterID string    `json:"character_id"`
	Role        Role      `json:"role"`
	Title       string    `json:"title"`
	JoinedAt    time.Time `json:"joined_at"`
}

type Detail struct {
	Guild   Guild    `json:"guild"`
	Members []Member `json:"members"`
}
