package contest

import (
	"context"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrPhotoNotFound              = errors.New("photo not found")
	ErrMaxPhotosReached           = errors.New("photo capacity reached (maximum 20 photos allowed)")
	ErrInvalidTitle               = errors.New("invalid photo or contest title")
	ErrTitleTooLong               = errors.New("title exceeds maximum length of 40 characters")
	ErrConsecutiveEntryDisallowed = errors.New("cannot enter consecutively while having an active entry in the current contest")
	ErrAlreadyEntered             = errors.New("character already has an entry in this contest round")
	ErrDuplicateTitle             = errors.New("an entry with the same title already exists in this contest round")
	ErrAlreadyVoted               = errors.New("character has already voted in this contest round")
	ErrSelfVoteDisallowed         = errors.New("cannot vote for your own contest entry")
	ErrContestNotActive           = errors.New("contest is not currently active for voting")
	ErrContestNotFound            = errors.New("contest round not found")
	ErrEntryNotFound              = errors.New("contest entry not found")
	ErrForbidden                  = errors.New("forbidden: resource belongs to another character")
	ErrCharacterNotFound          = errors.New("character not found")
	ErrContestNotReadyToSettle    = errors.New("contest round is still active and has not reached its end time")
	ErrCommentTooLong             = errors.New("comment exceeds maximum length of 100 characters")
)

const (
	MaxPhotosPerCharacter = 20
	MinEntriesForContest  = 5
	ContestCycleDays      = 10
	MaxTitleLength        = 40
	MaxCommentLength      = 100

	StatusPreparing = "preparing"
	StatusActive    = "active"
	StatusSettled   = "settled"

	VoterBonusSmallMedals = 1
)

// Prize defines rewards awarded to top contest winners.
type Prize struct {
	Gold        int `json:"gold"`
	SmallMedals int `json:"small_medals"`
	GuildPoints int `json:"guild_points"`
}

var (
	PrizeFirst  = Prize{Gold: 15000, SmallMedals: 10, GuildPoints: 700}
	PrizeSecond = Prize{Gold: 7000, SmallMedals: 6, GuildPoints: 300}
	PrizeThird  = Prize{Gold: 3000, SmallMedals: 3, GuildPoints: 100}
)

// Photo represents a saved screenshot / captured moment for a character.
type Photo struct {
	ID          string    `json:"id"`
	CharacterID string    `json:"character_id"`
	Title       string    `json:"title"`
	Location    string    `json:"location"`
	ImageURL    string    `json:"image_url"`
	Caption     string    `json:"caption"`
	Metadata    string    `json:"metadata,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ContestRound represents a single photo contest season / round.
type ContestRound struct {
	Round     int       `json:"round"`
	Status    string    `json:"status"` // "preparing", "active", "settled"
	StartTime time.Time `json:"start_time"`
	EndTime   time.Time `json:"end_time"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// ContestEntry represents a photo submitted into a contest round.
type ContestEntry struct {
	ID            string    `json:"id"`
	Round         int       `json:"round"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	GuildName     string    `json:"guild_name,omitempty"`
	Title         string    `json:"title"`
	PhotoID       string    `json:"photo_id"`
	ImageURL      string    `json:"image_url"`
	Caption       string    `json:"caption"`
	Votes         int       `json:"votes"`
	Ranking       int       `json:"ranking"`
	Comments      []string  `json:"comments,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

// ContestVote records a character's vote for an entry in a contest round.
type ContestVote struct {
	ID                 string    `json:"id"`
	Round              int       `json:"round"`
	EntryID            string    `json:"entry_id"`
	VoterCharacterID   string    `json:"voter_character_id"`
	VoterCharacterName string    `json:"voter_character_name"`
	Comment            string    `json:"comment,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// ContestLegend represents a 1st-place winning entry permanently archived in the Hall of Fame.
type ContestLegend struct {
	Round         int       `json:"round"`
	EntryID       string    `json:"entry_id"`
	Title         string    `json:"title"`
	CharacterID   string    `json:"character_id"`
	CharacterName string    `json:"character_name"`
	GuildName     string    `json:"guild_name,omitempty"`
	Votes         int       `json:"votes"`
	ImageURL      string    `json:"image_url"`
	Caption       string    `json:"caption"`
	SettledAt     time.Time `json:"settled_at"`
}

// Dialogue represents NPC @ワコール dialogue in the Photo Contest venue.
type Dialogue struct {
	NPCName  string   `json:"npc_name"`
	Title    string   `json:"title"`
	Greeting string   `json:"greeting"`
	Phrases  []string `json:"phrases"`
}

// ContestOverview summarizes the state of the active, preparing, and past contest rounds.
type ContestOverview struct {
	ActiveRound    *ContestRound  `json:"active_round,omitempty"`
	PreparingRound *ContestRound  `json:"preparing_round,omitempty"`
	ActiveEntries  []ContestEntry `json:"active_entries,omitempty"`
	EntryCount     int            `json:"entry_count"`
	MinEntries     int            `json:"min_entries"`
	IsPostponed    bool           `json:"is_postponed"`
	Dialogue       Dialogue       `json:"dialogue"`
}

// SettlementResult summarizes the result of concluding a contest round.
type SettlementResult struct {
	Round             int            `json:"round"`
	Rankings          []ContestEntry `json:"rankings,omitempty"`
	WinnerLegend      *ContestLegend `json:"winner_legend,omitempty"`
	PrizesDistributed bool           `json:"prizes_distributed"`
	Postponed         bool           `json:"postponed"`
	ExtendedUntil     time.Time      `json:"extended_until,omitempty"`
	VotersRewarded    int            `json:"voters_rewarded"`
	Message           string         `json:"message"`
}

// ValidateTitle checks title length and character constraints.
func ValidateTitle(title string) error {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return ErrInvalidTitle
	}
	if utf8.RuneCountInString(trimmed) > MaxTitleLength {
		return ErrTitleTooLong
	}
	// Check forbidden characters: , ; " ' & < > @ ＠ and full-width space
	for _, r := range trimmed {
		switch r {
		case ',', ';', '"', '\'', '&', '<', '>', '@', '＠', '\t', '\n', '\r':
			return ErrInvalidTitle
		}
	}
	return nil
}

// ValidateComment checks comment length and character constraints.
func ValidateComment(comment string) error {
	trimmed := strings.TrimSpace(comment)
	if utf8.RuneCountInString(trimmed) > MaxCommentLength {
		return ErrCommentTooLong
	}
	return nil
}

// CharacterRepository defines character data operations for Contest service.
type CharacterRepository interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
	FindByIDForUpdate(ctx context.Context, id string) (corecharacter.Character, error)
	Update(ctx context.Context, char corecharacter.Character) error
}

// GuildService defines guild operations for awarding guild points/exp.
type GuildService interface {
	AddGuildExp(ctx context.Context, characterID string, exp int64) error
}

// NewsPublisher defines interface for publishing announcements.
type NewsPublisher interface {
	PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error
}

type NewsPublisherFunc func(ctx context.Context, category, title, content, author string, publishedAt time.Time) error

func (f NewsPublisherFunc) PublishNews(ctx context.Context, category, title, content, author string, publishedAt time.Time) error {
	return f(ctx, category, title, content, author, publishedAt)
}

// ContestRepository defines database storage operations for photo contest.
type ContestRepository interface {
	// Photos
	SavePhoto(ctx context.Context, photo Photo) error
	FindPhotoByID(ctx context.Context, id string) (Photo, error)
	FindPhotoByIDForUpdate(ctx context.Context, id string) (Photo, error)
	ListPhotosByCharacterID(ctx context.Context, characterID string) ([]Photo, error)
	CountPhotosByCharacterID(ctx context.Context, characterID string) (int, error)
	DeletePhoto(ctx context.Context, id string) error

	// Rounds
	GetRoundByNumber(ctx context.Context, round int) (ContestRound, error)
	GetRoundByNumberForUpdate(ctx context.Context, round int) (ContestRound, error)
	GetActiveRound(ctx context.Context) (ContestRound, error)
	GetActiveRoundForUpdate(ctx context.Context) (ContestRound, error)
	GetPreparingRound(ctx context.Context) (ContestRound, error)
	GetPreparingRoundForUpdate(ctx context.Context) (ContestRound, error)
	GetLatestSettledRound(ctx context.Context) (ContestRound, error)
	SaveRound(ctx context.Context, round ContestRound) error

	// Entries
	SaveEntry(ctx context.Context, entry ContestEntry) error
	FindEntryByID(ctx context.Context, id string) (ContestEntry, error)
	FindEntryByIDForUpdate(ctx context.Context, id string) (ContestEntry, error)
	FindEntryByRoundAndCharacter(ctx context.Context, round int, characterID string) (ContestEntry, error)
	FindEntryByRoundAndTitle(ctx context.Context, round int, title string) (ContestEntry, error)
	ListEntriesByRound(ctx context.Context, round int) ([]ContestEntry, error)
	CountEntriesByRound(ctx context.Context, round int) (int, error)

	// Votes
	SaveVote(ctx context.Context, vote ContestVote) error
	HasVotedInRound(ctx context.Context, round int, voterCharacterID string) (bool, error)
	ListVotesByRound(ctx context.Context, round int) ([]ContestVote, error)
	ListVotesByEntryID(ctx context.Context, entryID string) ([]ContestVote, error)
	IncrementEntryVotes(ctx context.Context, entryID string) error

	// Legends
	SaveLegend(ctx context.Context, legend ContestLegend) error
	ListLegends(ctx context.Context, limit, offset int) ([]ContestLegend, error)
}

// TransactionProvider executes functions inside an atomic database transaction.
type TransactionProvider interface {
	RunInTx(ctx context.Context, fn func(txCtx context.Context) error) error
}
