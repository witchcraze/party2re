// Package placelog stores the short shared log of each place (legacy
// write_comment, lib/system.cgi:660-676). It is display context only: it has no
// gameplay authority and losing it on a Valkey failure is accepted.
// Contract: docs/architecture/place-log.md.
package placelog

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/valkey-io/valkey-go"
)

const (
	// DefaultLimit is the legacy $max_log (tracked config.cgi).
	DefaultLimit = 30
	// DefaultTTL expires logs of places nobody writes to, such as idle homes.
	DefaultTTL = 7 * 24 * time.Hour

	keyPrefix = "party2:placelog:"
)

// SpeakerKind distinguishes player rows from NPC rows.
type SpeakerKind string

const (
	SpeakerCharacter SpeakerKind = "character"
	SpeakerNPC       SpeakerKind = "npc"
)

// Place identifies one log. Fixed facilities use KindFacility with their scene
// destination; owner-scoped places (home, store, guild) use the owner ID.
type Place struct {
	Kind string
	ID   string
}

const KindFacility = "facility"

// Entry is one log row. Name and Color are snapshots at write time and Text is
// already rendered; rows are never re-rendered.
type Entry struct {
	OccurredAt  time.Time   `json:"occurred_at"`
	SpeakerKind SpeakerKind `json:"speaker_kind"`
	// SpeakerCharacterID is empty for NPC rows.
	SpeakerCharacterID string `json:"speaker_character_id,omitempty"`
	Name               string `json:"name"`
	Color              string `json:"color"`
	Text               string `json:"text"`
	// RecipientCharacterID marks a private (whisper) row.
	RecipientCharacterID string `json:"recipient_character_id,omitempty"`
}

// VisibleTo reports whether viewerID may see the row: public rows to everyone,
// private rows only to their sender and recipient (lib/system.cgi:618-633).
func (e Entry) VisibleTo(viewerID string) bool {
	if e.RecipientCharacterID == "" {
		return true
	}
	return viewerID != "" && (viewerID == e.RecipientCharacterID || viewerID == e.SpeakerCharacterID)
}

// Config holds the retention settings.
type Config struct {
	Limit int
	TTL   time.Duration
}

// Log is the Valkey Master place log.
type Log struct {
	client valkey.Client
	cfg    Config
}

//go:embed lua/append.lua
var appendLua string

var appendScript = valkey.NewLuaScript(appendLua)

// New returns a Log; non-positive settings fall back to the defaults.
func New(client valkey.Client, cfg Config) *Log {
	if cfg.Limit <= 0 {
		cfg.Limit = DefaultLimit
	}
	if cfg.TTL < time.Second {
		cfg.TTL = DefaultTTL
	}
	return &Log{client: client, cfg: cfg}
}

func key(p Place) (string, error) {
	if strings.TrimSpace(p.Kind) == "" || strings.TrimSpace(p.ID) == "" || strings.Contains(p.Kind, ":") {
		return "", fmt.Errorf("placelog: invalid place %q/%q", p.Kind, p.ID)
	}
	return keyPrefix + p.Kind + ":" + p.ID, nil
}

// Append adds entries in order (the last one becomes the newest), trims the log
// to the limit and refreshes its TTL in one atomic step.
func (l *Log) Append(ctx context.Context, p Place, entries ...Entry) error {
	k, err := key(p)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	args := []string{strconv.Itoa(l.cfg.Limit), strconv.FormatInt(int64(l.cfg.TTL/time.Second), 10)}
	for _, e := range entries {
		raw, err := json.Marshal(e)
		if err != nil {
			return err
		}
		args = append(args, string(raw))
	}
	return appendScript.Exec(ctx, l.client, []string{k}, args).Error()
}

// Read returns the rows viewerID may see, newest first. A missing log is empty;
// a store or decode failure is an error, never an empty success.
func (l *Log) Read(ctx context.Context, p Place, viewerID string) ([]Entry, error) {
	k, err := key(p)
	if err != nil {
		return nil, err
	}
	raws, err := l.client.Do(ctx, l.client.B().Lrange().Key(k).Start(0).Stop(int64(l.cfg.Limit-1)).Build()).AsStrSlice()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return nil, err
	}
	out := make([]Entry, 0, len(raws))
	for _, raw := range raws {
		var e Entry
		if err := json.Unmarshal([]byte(raw), &e); err != nil {
			return nil, fmt.Errorf("placelog: decode %s: %w", k, err)
		}
		if e.VisibleTo(viewerID) {
			out = append(out, e)
		}
	}
	return out, nil
}
