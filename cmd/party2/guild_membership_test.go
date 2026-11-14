package main

import (
	"context"
	"errors"
	"testing"

	"github.com/witchcraze/party2re/internal/guild"
)

// A failed lookup must not read as "not in a guild": that would let a member rename (#2215).
func TestGuildMembership(t *testing.T) {
	down := errors.New("lookup down")
	for _, tc := range []struct {
		name    string
		err     error
		want    bool
		wantErr error
	}{
		{"member", nil, true, nil},
		{"not a member", guild.ErrCharacterNotInGuild, false, nil},
		{"lookup failure", down, false, down},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := guildMembership(func(context.Context, string) (guild.Guild, guild.Member, error) {
				return guild.Guild{}, guild.Member{}, tc.err
			})(context.Background(), "c")
			if got != tc.want || !errors.Is(err, tc.wantErr) {
				t.Fatalf("got %v, %v; want %v, %v", got, err, tc.want, tc.wantErr)
			}
		})
	}
}
