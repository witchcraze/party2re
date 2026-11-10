package economy

import (
	"context"
	"slices"
)

// LockInOrder calls lock once per distinct ID in ascending order without changing ids.
// IDs are opaque, including empty strings; lock owns validation and not-found policy.
// The caller owns the transaction and must acquire later-rank locks after this returns.
func LockInOrder(ctx context.Context, ids []string, lock func(context.Context, string) error) error {
	ordered := slices.Clone(ids)
	slices.Sort(ordered)
	for _, id := range slices.Compact(ordered) {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := lock(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
