package park

import (
	"context"
	"time"
)

type Repository interface {
	CreatePost(ctx context.Context, post Post) error
	GetRecentPosts(ctx context.Context, limit int, offset int) ([]Post, int, error)
	GetRecentPostsByCursor(ctx context.Context, limit int, beforeTime time.Time, beforeID string) ([]Post, error)
	GetLatestPostTimeByCharacter(ctx context.Context, characterID string) (time.Time, error)
}
