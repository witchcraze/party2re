package park

import (
	"context"
	"errors"
	mrand "math/rand"
	"strings"
	"time"

	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/ratelimit"
)

type CharacterReader interface {
	FindByID(ctx context.Context, id string) (corecharacter.Character, error)
}

type Limiter interface {
	Allow(ctx context.Context, key string, limit int64, window time.Duration) (ratelimit.Result, error)
}

type Service struct {
	repo         Repository
	charReader   CharacterReader
	npc          *TownGirlNPC
	rateLimitDur time.Duration
	limiter      Limiter
	nowFunc      func() time.Time
}

type ServiceOption func(*Service)

func WithRateLimit(d time.Duration) ServiceOption {
	return func(s *Service) {
		s.rateLimitDur = d
	}
}

func WithRateLimiter(limiter Limiter) ServiceOption {
	return func(s *Service) {
		s.limiter = limiter
	}
}

func WithNowFunc(fn func() time.Time) ServiceOption {
	return func(s *Service) {
		s.nowFunc = fn
	}
}

func WithNPCRNG(rng *mrand.Rand) ServiceOption {
	return func(s *Service) {
		s.npc = NewTownGirlNPC(rng)
	}
}

func NewService(repo Repository, charReader CharacterReader, opts ...ServiceOption) (*Service, error) {
	if repo == nil {
		return nil, errors.New("repository is required")
	}
	if charReader == nil {
		return nil, errors.New("character reader is required")
	}
	s := &Service{
		repo:         repo,
		charReader:   charReader,
		npc:          NewTownGirlNPC(nil),
		rateLimitDur: 3 * time.Second,
		nowFunc:      func() time.Time { return time.Now().UTC() },
	}
	for _, opt := range opts {
		opt(s)
	}
	return s, nil
}

// PostMessage creates a new public message in the park.
func (s *Service) PostMessage(ctx context.Context, characterID, content, color, recipient string) (Post, error) {
	if err := ValidatePost(characterID, content, color, recipient); err != nil {
		return Post{}, err
	}

	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return Post{}, ErrCharacterNotFound
		}
		return Post{}, err
	}

	now := s.nowFunc().UTC()
	if s.rateLimitDur > 0 {
		if s.limiter != nil {
			res, err := s.limiter.Allow(ctx, "park:post:"+characterID, 1, s.rateLimitDur)
			if err == nil && !res.Allowed {
				return Post{}, ErrRateLimited
			}
		} else {
			latestTime, err := s.repo.GetLatestPostTimeByCharacter(ctx, characterID)
			if err != nil {
				// return error if repo fails
				return Post{}, err
			}
			if !latestTime.IsZero() {
				if now.Sub(latestTime.UTC()) < s.rateLimitDur {
					return Post{}, ErrRateLimited
				}
			}
		}
	}

	cleanContent := SanitizeContent(content)
	cleanRecipient := strings.TrimSpace(recipient)
	if len(cleanRecipient) > 64 {
		cleanRecipient = cleanRecipient[:64]
	}

	if color == "" {
		color = DefaultColor
	}

	post := Post{
		ID:            id.New(),
		CharacterID:   char.ID,
		CharacterName: char.Name,
		Content:       cleanContent,
		Color:         color,
		RecipientName: cleanRecipient,
		CreatedAt:     now,
	}

	if err := s.repo.CreatePost(ctx, post); err != nil {
		return Post{}, err
	}

	return post, nil
}

// GetRecentPosts retrieves recent board posts with pagination.
func (s *Service) GetRecentPosts(ctx context.Context, limit, offset int) ([]Post, int, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	return s.repo.GetRecentPosts(ctx, limit, offset)
}

// GetRecentPostsByCursor retrieves recent board posts using keyset / cursor-based pagination.
func (s *Service) GetRecentPostsByCursor(ctx context.Context, limit int, cursor string) (pagination.CursorPage[Post], error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}

	var beforeTime time.Time
	var beforeID string
	var err error

	if cursor != "" {
		beforeTime, beforeID, err = pagination.DecodeCursor(cursor)
		if err != nil {
			beforeID, _ = pagination.DecodeIDCursor(cursor)
		}
	}

	fetchLimit := limit + 1
	items, err := s.repo.GetRecentPostsByCursor(ctx, fetchLimit, beforeTime, beforeID)
	if err != nil {
		return pagination.CursorPage[Post]{}, err
	}

	hasMore := false
	if len(items) > limit {
		hasMore = true
		items = items[:limit]
	}

	var nextCursor string
	if hasMore && len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = pagination.EncodeCursor(last.CreatedAt, last.ID)
	}

	return pagination.NewCursorPage(items, nextCursor, cursor, limit, hasMore), nil
}

// TalkToNPC generates a dialogue line for the character talking to the town girl NPC.
func (s *Service) TalkToNPC(ctx context.Context, characterID string) (string, error) {
	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return "", ErrCharacterNotFound
		}
		return "", err
	}
	return s.npc.Talk(char.JobID, char.Name), nil
}

// Divinate requests a fortune and lucky color divination from the town girl NPC.
func (s *Service) Divinate(ctx context.Context, characterID string) (DivinationResult, error) {
	char, err := s.charReader.FindByID(ctx, characterID)
	if err != nil {
		if errors.Is(err, corecharacter.ErrNotFound) {
			return DivinationResult{}, ErrCharacterNotFound
		}
		return DivinationResult{}, err
	}
	return s.npc.Divinate(char.Name), nil
}

// InspectNPC generates the inspection dialog when investigating the town girl NPC.
func (s *Service) InspectNPC() string {
	return s.npc.Inspect()
}
