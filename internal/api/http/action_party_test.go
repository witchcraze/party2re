package http

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/pagination"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/runhost"
)

type partyLobbyStub struct {
	calls   []string
	err     error
	parties []party.PartySummary
}

func (s *partyLobbyStub) CreateParty(_ context.Context, leader string, req party.CreatePartyRequest) (party.PartyDetail, error) {
	s.calls = append(s.calls, fmt.Sprintf("create/%s/%s/%s/%d/%d/%s/%s", leader, req.Name, req.StageID, req.Speed, req.MaxMembers, req.NeedJoin, req.Password))
	return party.PartyDetail{Party: party.Party{ID: "new"}}, s.err
}

func (s *partyLobbyStub) JoinParty(_ context.Context, partyID, actor, password string) (party.PartyDetail, error) {
	s.calls = append(s.calls, "join/"+partyID+"/"+actor+"/"+password)
	return party.PartyDetail{Party: party.Party{ID: partyID}}, s.err
}

func (s *partyLobbyStub) GetPartySummary(_ context.Context, id string) (party.PartySummary, error) {
	for _, p := range s.parties {
		if p.ID == id {
			return p, nil
		}
	}
	return party.PartySummary{}, party.ErrNotFound
}

func (s *partyLobbyStub) ListParties(_ context.Context, status string, limit, offset int) (pagination.Page[party.PartySummary], error) {
	if status != party.StatusRecruiting {
		return pagination.Page[party.PartySummary]{}, errors.New("quest scene must list recruiting lobbies")
	}
	end := min(offset+limit, len(s.parties))
	return pagination.NewPage(s.parties[min(offset, end):end], len(s.parties), limit, offset), nil
}

func (s *partyLobbyStub) LeaveParty(_ context.Context, partyID, actor string) error {
	s.calls = append(s.calls, "leave/"+partyID+"/"+actor)
	return s.err
}

func (s *partyLobbyStub) KickMember(_ context.Context, partyID, leader, target string) error {
	s.calls = append(s.calls, "kick/"+partyID+"/"+leader+"/"+target)
	return s.err
}

func TestPartyLobbyGateway(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		facts            []string
		err              error
		status           int
	}{
		{"leave", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, nil, 200},
		{"kick", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_leave", "party_kick"}, nil, 200},
		{"kick not offered", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "", []string{"party_leave"}, nil, 409},
		{"missing party", `{"action":"party_leave","params":{}}`, "", []string{"party_leave"}, nil, 400},
		{"unknown param", `{"action":"party_leave","params":{"party_id":"p1","ready":true}}`, "", []string{"party_leave"}, nil, 400},
		{"lobby gone", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, party.ErrNotFound, 404},
		{"not member", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, party.ErrCharacterNotInParty, 422},
		{"not leader", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_kick"}, party.ErrNotPartyLeader, 403},
		{"kick self", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"hero"}}`, "kick/p1/hero/hero", []string{"party_kick"}, party.ErrCannotKickSelf, 422},
		{"started", `{"action":"party_kick","params":{"party_id":"p1","target_character_id":"ally"}}`, "kick/p1/hero/ally", []string{"party_kick"}, runhost.ErrBusy, 409},
		{"store down", `{"action":"party_leave","params":{"party_id":"p1"}}`, "leave/p1/hero", []string{"party_leave"}, errors.New("down"), 500},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newGatewayFixture(t)
			stub := &partyLobbyStub{err: tc.err}
			pc := playercontext.NewService(f, f, timer.NewService(nil), playercontext.WithActivities(func(context.Context, string, string) ([]playercontext.Activity, error) {
				return []playercontext.Activity{{Kind: "party", ID: "p1", Phase: party.StatusRecruiting, Role: "leader", Actions: tc.facts}}, nil
			}))
			router := f.router("rescue_request", timer.NewService(nil), WithParty(stub), WithPlayerContext(pc))
			status, raw := gatewayRequest(t, router, "hero", "session", "application/json", tc.body, nil)
			if status != tc.status {
				t.Fatalf("status %d %s", status, raw)
			}
			if got := len(stub.calls); (tc.want == "" && got != 0) || (tc.want != "" && (got != 1 || stub.calls[0] != tc.want)) {
				t.Fatalf("calls %v, want %q", stub.calls, tc.want)
			}
		})
	}
}
