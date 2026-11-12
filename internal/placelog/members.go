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

// DefaultMemberTTL is the legacy $limit_member_time (tracked config.cgi).
const DefaultMemberTTL = 300 * time.Second

const memberPrefix = "party2:placemember:"

// Member is a character's public appearance as of its latest refresh.
type Member struct {
	CharacterID string `json:"character_id"`
	Name        string `json:"name"`
	Color       string `json:"color"`
	IconURL     string `json:"icon_url"`
	AuraEffect  int    `json:"aura_effect"`
}

// Members is the per-place member list (legacy read_member/leave_member,
// lib/system.cgi:470-520,960-975). Like the log it is display context only.
type Members struct {
	client valkey.Client
	ttl    time.Duration
}

//go:embed lua/refresh_member.lua
var refreshMemberLua string

var refreshMemberScript = valkey.NewLuaScript(refreshMemberLua)

// NewMembers returns a member store; a TTL under one second falls back to the default.
func NewMembers(client valkey.Client, ttl time.Duration) *Members {
	if ttl < time.Second {
		ttl = DefaultMemberTTL
	}
	return &Members{client: client, ttl: ttl}
}

// memberKeys share one hash tag so the refresh script stays in one slot:
// joined (arrival order), expiry (expires_at seconds) and profile (JSON).
func memberKeys(p Place) ([3]string, error) {
	if _, err := key(p); err != nil {
		return [3]string{}, err
	}
	base := memberPrefix + "{" + p.Kind + ":" + p.ID + "}:"
	return [3]string{base + "joined", base + "expiry", base + "profile"}, nil
}

func characterKey(characterID string) string { return memberPrefix + "character:" + characterID }

// Refresh lists m in place p until now+TTL. A character is listed in one place
// at a time, so an entry left in another place is removed. The place pointer is
// swapped after the entry is written: a concurrent Leave or Refresh elsewhere
// then never leaves an entry that no pointer can reach.
func (s *Members) Refresh(ctx context.Context, p Place, m Member, now time.Time) error {
	keys, err := memberKeys(p)
	if err != nil {
		return err
	}
	if strings.TrimSpace(m.CharacterID) == "" {
		return errors.New("placelog: member without character ID")
	}
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	ttl := int64(s.ttl / time.Second)
	if err := refreshMemberScript.Exec(ctx, s.client, keys[:], []string{
		strconv.FormatInt(now.Unix(), 10), strconv.FormatInt(now.Unix()+ttl, 10), strconv.FormatInt(ttl, 10), m.CharacterID, string(raw),
	}).Error(); err != nil {
		return err
	}
	place := p.Kind + ":" + p.ID
	previous, err := s.client.Do(ctx, s.client.B().Set().Key(characterKey(m.CharacterID)).Value(place).Get().ExSeconds(ttl).Build()).ToString()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return err
	}
	if previous != "" && previous != place {
		return s.remove(ctx, previous, m.CharacterID)
	}
	return nil
}

// Leave removes the character from the place it is listed in, if any.
func (s *Members) Leave(ctx context.Context, characterID string) error {
	place, err := s.client.Do(ctx, s.client.B().Getdel().Key(characterKey(characterID)).Build()).ToString()
	if errors.Is(err, valkey.Nil) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.remove(ctx, place, characterID)
}

func (s *Members) remove(ctx context.Context, place, characterID string) error {
	kind, id, _ := strings.Cut(place, ":")
	keys, err := memberKeys(Place{Kind: kind, ID: id})
	if err != nil {
		return err
	}
	for _, r := range s.client.DoMulti(ctx,
		s.client.B().Zrem().Key(keys[0]).Member(characterID).Build(),
		s.client.B().Zrem().Key(keys[1]).Member(characterID).Build(),
		s.client.B().Hdel().Key(keys[2]).Field(characterID).Build(),
	) {
		if err := r.Error(); err != nil {
			return err
		}
	}
	return nil
}

// Read returns the members listed at now in arrival order. It never writes:
// expired entries are skipped here and purged by the next refresh. A store or
// decode failure is an error, never an empty success.
func (s *Members) Read(ctx context.Context, p Place, now time.Time) ([]Member, error) {
	keys, err := memberKeys(p)
	if err != nil {
		return nil, err
	}
	res := s.client.DoMulti(ctx,
		s.client.B().Zrange().Key(keys[0]).Min("0").Max("-1").Build(),
		s.client.B().Zrange().Key(keys[1]).Min(strconv.FormatInt(now.Unix(), 10)).Max("+inf").Byscore().Build(),
		s.client.B().Hgetall().Key(keys[2]).Build(),
	)
	joined, err := res[0].AsStrSlice()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return nil, err
	}
	live, err := res[1].AsStrSlice()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return nil, err
	}
	profiles, err := res[2].AsStrMap()
	if err != nil && !errors.Is(err, valkey.Nil) {
		return nil, err
	}
	alive := make(map[string]bool, len(live))
	for _, id := range live {
		alive[id] = true
	}
	out := make([]Member, 0, len(live))
	for _, id := range joined {
		raw, ok := profiles[id]
		if !alive[id] || !ok { // a concurrent refresh or leave between the reads
			continue
		}
		var m Member
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			return nil, fmt.Errorf("placelog: decode member %s: %w", id, err)
		}
		out = append(out, m)
	}
	return out, nil
}
