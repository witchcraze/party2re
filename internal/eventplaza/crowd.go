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

func crowdGuestIdentity(slot int) int {
	if slot == 64 {
		return 60
	}
	return slot
}

func crowdGuestName(slot int) string {
	return fmt.Sprintf("Celebration guest %02d", 1+crowdGuestIdentity(slot))
}

func crowdPropIdentity(slot int) int {
	if slot == 24 {
		return 20
	}
	if slot > 24 {
		return slot - 1
	}
	return slot
}

func crowdPropName(slot int) string {
	return fmt.Sprintf("Celebration decoration %02d", 1+crowdPropIdentity(slot))
}

// RecordVictoryCrowd appends the temporary guests and props after a successful sealing.
func (s *Service) RecordVictoryCrowd(ctx context.Context) error {
	if s.presenceTracker == nil {
		return nil
	}
	now := s.clock.Now()
	guests, props := 2+s.rng.Intn(4), 1+s.rng.Intn(2)
	crowd := make([]Presence, 0, guests+props)
	// Generic identities replace legacy names/assets; 65/32 candidate slots map to
	// 64/31 identities with duplicate slots (60/64 and 20/24) preserving legacy weights.
	for _, group := range []struct {
		nameGen func(int) string
		count   int
		slots   int
	}{
		{crowdGuestName, guests, 65},
		{crowdPropName, props, 32},
	} {
		for i := 0; i < group.count; i++ {
			marker := now.Add(time.Duration(3600+s.rng.Intn(3600)) * time.Second)
			crowd = append(crowd, Presence{Name: group.nameGen(s.rng.Intn(group.slots)), Marker: marker})
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
