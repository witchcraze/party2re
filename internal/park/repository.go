package park

import (
	"context"
	"time"
)

type Repository interface {
	CreatePost(ctx context.Context, post Post, logLimit int) error
	GetRecentPosts(ctx context.Context, limit int, offset int) ([]Post, int, error)
	GetRecentPostsByCursor(ctx context.Context, limit int, beforeTime time.Time, beforeID string) ([]Post, error)
}
