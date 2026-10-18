package eventplaza

import (
	"context"
	"fmt"
	"time"
)

// Presence keeps a display identity and a time marker, which may be in the future.
type Presence struct {
	Name   string
	Marker time.Time
}

// PresenceSnapshot separates merchant selection from the subsequent display cleanup.
type PresenceSnapshot struct {
	Stored int
	Active int
}

// RecordVictoryCrowd appends the temporary guests and props after a successful sealing.
func (s *Service) RecordVictoryCrowd(ctx context.Context) error {
	if s.presenceTracker == nil {
		return nil
	}
	now := s.clock.Now()
	guests, props := 2+s.rng.Intn(4), 1+s.rng.Intn(2)
	crowd := make([]Presence, 0, guests+props)
	// Generic identities replace legacy names/assets; repeated draws remain duplicates.
	for _, group := range []struct {
		name        string
		count, pool int
	}{
		{"Celebration guest", guests, 64}, {"Celebration decoration", props, 32},
	} {
		for i := 0; i < group.count; i++ {
			marker := now.Add(time.Duration(3600+s.rng.Intn(3600)) * time.Second)
			name := fmt.Sprintf("%s %02d", group.name, 1+s.rng.Intn(group.pool))
			crowd = append(crowd, Presence{Name: name, Marker: marker})
		}
	}
	return s.presenceTracker.AppendCrowd(ctx, crowd)
}

func observeCrowd(rows []Presence, now time.Time) ([]Presence, PresenceSnapshot) {
	snapshot := PresenceSnapshot{Stored: len(rows)}
	if tier, _, _ := CalculateMerchantTier(snapshot.Stored); tier > 0 {
		rows = append(rows, Presence{Name: "merchant:traveling", Marker: now})
	}
	cutoff := now.Unix() - int64(PresenceWindow/time.Second)
	seen := make(map[string]bool, len(rows))
	active := make([]Presence, 0, len(rows))
	for _, row := range rows {
		if row.Marker.Unix() < cutoff || seen[row.Name] {
			continue
		}
		seen[row.Name] = true
		active = append(active, row)
	}
	snapshot.Active = len(active)
	return active, snapshot
}
