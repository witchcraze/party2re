package database

import "context"

func (r *ParkRepository) SetTrimFnForTesting(fn func(ctx context.Context, logLimit int) error) {
	r.trimFn = fn
}

func IsDeadlockForTesting(err error) bool {
	return isDeadlock(err)
}
