package main

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/witchcraze/party2re/internal/api/http"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreplayer "github.com/witchcraze/party2re/internal/core/player"
)

type logoutPlayers struct {
	http.PlayerService
	loggedOut bool
}

func (p *logoutPlayers) Authenticate(_ context.Context, sessionID string) (coreplayer.Player, error) {
	if sessionID != "s" {
		return coreplayer.Player{}, errors.New("unknown session")
	}
	return coreplayer.Player{ID: "p"}, nil
}

func (p *logoutPlayers) Logout(context.Context, string) error { p.loggedOut = true; return nil }

type logoutCharacters struct{}

func (logoutCharacters) FindByPlayerID(_ context.Context, playerID string) ([]corecharacter.Character, error) {
	if playerID != "p" {
		return nil, nil
	}
	return []corecharacter.Character{{ID: "a"}, {ID: "b"}}, nil
}

type leaveRecorder struct {
	left []string
	err  error
}

func (l *leaveRecorder) Leave(_ context.Context, id string) error {
	l.left = append(l.left, id)
	return l.err
}

func TestLogoutLeavesEveryCharacterAndAlwaysLogsOut(t *testing.T) {
	players, members := &logoutPlayers{}, &leaveRecorder{}
	s := placeMemberLogout{PlayerService: players, characters: logoutCharacters{}, members: members}
	if err := s.Logout(context.Background(), "s"); err != nil || !players.loggedOut || !slices.Equal(members.left, []string{"a", "b"}) {
		t.Fatalf("logout: %v, logged out %v, left %v", err, players.loggedOut, members.left)
	}
	// A failed removal or an unknown session never blocks logout.
	players, members = &logoutPlayers{}, &leaveRecorder{err: errors.New("down")}
	s = placeMemberLogout{PlayerService: players, characters: logoutCharacters{}, members: members}
	if err := s.Logout(context.Background(), "s"); err != nil || !players.loggedOut {
		t.Fatalf("failed leave blocked logout: %v %v", err, players.loggedOut)
	}
	players, members = &logoutPlayers{}, &leaveRecorder{}
	s = placeMemberLogout{PlayerService: players, characters: logoutCharacters{}, members: members}
	if err := s.Logout(context.Background(), "x"); err != nil || !players.loggedOut || len(members.left) != 0 {
		t.Fatalf("unknown session: %v %v %v", err, players.loggedOut, members.left)
	}
}
