package eventplaza

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
	"github.com/witchcraze/party2re/internal/core/random"
)

const ValkeyPresenceKey = "party2:eventplaza:presence"

// ValkeyPresenceTracker owns ordered presence records scored by their time markers.
// WATCH protects snapshot selection and cleanup against concurrent crowd/heartbeat writes.
type ValkeyPresenceTracker struct {
	client valkey.Client
	key    string
}

func NewValkeyPresenceTracker(client valkey.Client) *ValkeyPresenceTracker {
	return &ValkeyPresenceTracker{client: client, key: ValkeyPresenceKey}
}

func (t *ValkeyPresenceTracker) RecordPresence(ctx context.Context, displayName string, at time.Time) error {
	_, err := t.update(ctx, func(rows []Presence) ([]Presence, PresenceSnapshot) {
		for i := range rows {
			if rows[i].Name == displayName {
				rows[i].Marker = at
				return rows, PresenceSnapshot{}
			}
		}
		return append(rows, Presence{Name: displayName, Marker: at}), PresenceSnapshot{}
	})
	return err
}

func (t *ValkeyPresenceTracker) AppendCrowd(ctx context.Context, crowd []Presence) error {
	_, err := t.update(ctx, func(rows []Presence) ([]Presence, PresenceSnapshot) {
		return append(rows, crowd...), PresenceSnapshot{}
	})
	return err
}

func (t *ValkeyPresenceTracker) Observe(ctx context.Context, now time.Time) (PresenceSnapshot, error) {
	return t.update(ctx, func(rows []Presence) ([]Presence, PresenceSnapshot) {
		return observeCrowd(rows, now)
	})
}

func (t *ValkeyPresenceTracker) update(ctx context.Context, change func([]Presence) ([]Presence, PresenceSnapshot)) (PresenceSnapshot, error) {
	if t.client == nil {
		return PresenceSnapshot{}, nil
	}
	var snapshot PresenceSnapshot
	err := t.client.Dedicated(func(client valkey.DedicatedClient) error {
		defer func() {
			// A cancelled request must not return a still-watched socket to the pool.
			if err := client.Do(ctx, client.B().Unwatch().Build()).Error(); err != nil {
				client.Close()
			}
		}()
		for attempt := 0; attempt < 16; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := client.Do(ctx, client.B().Watch().Key(t.key).Build()).Error(); err != nil {
				return err
			}
			values, err := client.Do(ctx, client.B().Zrange().Key(t.key).Min("0").Max("-1").Withscores().Build()).AsZScores()
			if err != nil {
				return err
			}
			// Member prefixes preserve insertion order independently of the marker scores.
			slices.SortStableFunc(values, func(a, b valkey.ZScore) int { return strings.Compare(a.Member, b.Member) })
			rows := make([]Presence, 0, len(values))
			for _, value := range values {
				name := value.Member
				if strings.HasPrefix(name, "v1:") {
					parts := strings.SplitN(name, ":", 3)
					if len(parts) != 3 {
						return fmt.Errorf("invalid plaza presence member %q", name)
					}
					name = parts[2]
				}
				rows = append(rows, Presence{Name: name, Marker: time.Unix(int64(value.Score), 0).UTC()})
			}
			rows, snapshot = change(rows)
			commands := []valkey.Completed{client.B().Multi().Build(), client.B().Del().Key(t.key).Build()}
			if len(rows) > 0 {
				add := client.B().Zadd().Key(t.key).ScoreMember()
				for i, row := range rows {
					member := fmt.Sprintf("v1:%020d:%s", i, row.Name)
					add = add.ScoreMember(float64(row.Marker.Unix()), member)
				}
				commands = append(commands, add.Build())
			}
			commands = append(commands, client.B().Exec().Build())
			results := client.DoMulti(ctx, commands...)
			for _, result := range results[:len(results)-1] {
				if err := result.Error(); err != nil {
					return err
				}
			}
			committed, err := results[len(results)-1].ToArray()
			if valkey.IsValkeyNil(err) {
				// Back off contending writers instead of exhausting retries in a tight loop.
				delay := time.Duration(1+random.Intn(1<<min(attempt, 8))) * time.Millisecond
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(delay):
				}
				continue
			}
			if err != nil {
				return err
			}
			for _, result := range committed {
				if err := result.Error(); err != nil {
					return err
				}
			}
			return nil
		}
		return errors.New("plaza presence changed concurrently; retry request")
	})
	if err != nil {
		return PresenceSnapshot{}, err
	}
	return snapshot, nil
}
