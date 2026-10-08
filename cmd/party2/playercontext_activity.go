package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/witchcraze/party2re/internal/casino"
	"github.com/witchcraze/party2re/internal/challenge"
	"github.com/witchcraze/party2re/internal/dungeon"
	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/playercontext"
	"github.com/witchcraze/party2re/internal/pvp"
)

// Consumer ports use public services; private repositories stay feature-owned.
type partyActivityReader interface {
	GetActiveParty(context.Context, string) (party.Party, party.Member, error)
}
type pvpActivityReader interface {
	GetCharacterRoom(context.Context, string) (pvp.RoomDetail, error)
}
type gvgActivityReader interface {
	GetCharacterRoom(context.Context, string) (gvg.RoomDetail, error)
}
type dungeonActivityReader interface {
	GetActiveExpedition(context.Context, string) (*dungeon.ActiveExpedition, error)
}
type challengeActivityReader interface {
	GetActiveSession(context.Context, string) (*challenge.ChallengeSession, error)
}
type casinoActivityReader interface {
	GetCharacterRoomView(context.Context, string, string) (*casino.RoomView, error)
}

type contextActivityReaders struct {
	party     partyActivityReader
	pvp       pvpActivityReader
	gvg       gvgActivityReader
	dungeon   dungeonActivityReader
	challenge challengeActivityReader
	casino    casinoActivityReader
}

func (r contextActivityReaders) read(ctx context.Context, owner, actor string) ([]playercontext.Activity, error) {
	facts := []playercontext.Activity{}
	p, member, err := r.party.GetActiveParty(ctx, actor)
	if err != nil && !errors.Is(err, party.ErrNotFound) {
		return nil, fmt.Errorf("party activity: %w", err)
	}
	if err == nil && p.Status != party.StatusDisbanded {
		if member.CharacterID != actor || member.PartyID != p.ID {
			return nil, errors.New("party activity membership mismatch")
		}
		a := playercontext.Activity{Kind: "party", ID: p.ID, Phase: p.Status, Role: "member", Actions: []string{"party_leave"}}
		if member.IsLeader && p.LeaderCharacterID == actor {
			a.Role = "leader"
		}
		if p.Status == party.StatusRecruiting {
			a.Actions = append(a.Actions, "party_ready")
			if a.Role == "leader" {
				a.Actions = append(a.Actions, "party_start")
			}
		}
		facts = append(facts, a)
	}
	pv, err := r.pvp.GetCharacterRoom(ctx, actor)
	if err != nil && !errors.Is(err, pvp.ErrRoomNotFound) {
		return nil, fmt.Errorf("pvp activity: %w", err)
	}
	if err == nil && pv.Room.Status != pvp.StatusDisbanded {
		found := false
		for _, m := range pv.Members {
			if m.CharacterID == actor {
				found = true
			}
		}
		if !found {
			return nil, errors.New("pvp activity membership mismatch")
		}
		a := playercontext.Activity{Kind: "pvp", ID: pv.Room.ID, Phase: pv.Room.Status, Round: pv.Room.Round, Role: "member", Actions: []string{}}
		if pv.Room.LeaderCharacterID == actor {
			a.Role = "leader"
		}
		if pv.Room.Status != pvp.StatusInProgress {
			a.Actions = append(a.Actions, "pvp_leave")
		}
		if pv.Room.Status == pvp.StatusRecruiting && pv.Room.Round == 0 {
			a.Actions = append(a.Actions, "pvp_team")
			if a.Role == "leader" {
				a.Actions = append(a.Actions, "pvp_start")
			}
		}
		if a.Role == "leader" && pv.Room.Status == pvp.StatusInProgress {
			a.Actions = append(a.Actions, "pvp_advance")
		}
		facts = append(facts, a)
	}
	gv, err := r.gvg.GetCharacterRoom(ctx, actor)
	if err != nil && !errors.Is(err, gvg.ErrRoomNotFound) {
		return nil, fmt.Errorf("gvg activity: %w", err)
	}
	if err == nil && gv.Room.Status != gvg.StatusDisbanded {
		found := false
		for _, m := range gv.Members {
			if m.CharacterID == actor {
				found = true
			}
		}
		if !found {
			return nil, errors.New("gvg activity membership mismatch")
		}
		a := playercontext.Activity{Kind: "gvg", ID: gv.Room.ID, Phase: gv.Room.Status, Round: gv.Room.Round, Role: "member", Actions: []string{}}
		if gv.Room.LeaderCharacterID == actor {
			a.Role = "leader"
		}
		if a.Role == "leader" || gv.Room.Status == gvg.StatusRecruiting || gv.Room.Status == gvg.StatusCompleted {
			a.Actions = append(a.Actions, "gvg_leave")
		}
		if a.Role == "leader" && gv.Room.Status == gvg.StatusRecruiting && gv.Room.Round == 0 {
			a.Actions = append(a.Actions, "gvg_start")
		}
		if a.Role == "leader" && gv.Room.Status == gvg.StatusInProgress {
			a.Actions = append(a.Actions, "gvg_advance")
		}
		facts = append(facts, a)
	}
	exp, err := r.dungeon.GetActiveExpedition(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("dungeon activity: %w", err)
	}
	if exp != nil {
		if exp.CharacterID != actor {
			return nil, errors.New("dungeon activity ownership mismatch")
		}
		a := playercontext.Activity{Kind: "dungeon", ID: exp.ID, PartyID: exp.PartyID, Phase: string(exp.Status), Role: "actor", Round: exp.CurrentFloor, Actions: []string{}}
		if exp.Status == dungeon.StatusExploring {
			a.Actions = []string{"dungeon_move", "dungeon_escape"}
		}
		// Terminal buffers still require their owner to finish settlement.
		facts = append(facts, a)
	}
	session, err := r.challenge.GetActiveSession(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("challenge activity: %w", err)
	}
	if session != nil {
		if session.CharacterID != actor {
			return nil, errors.New("challenge activity ownership mismatch")
		}
		a := playercontext.Activity{Kind: "challenge", ID: session.ID, PartyID: session.PartyID, Phase: string(session.Status), Role: "actor", Round: session.CurrentRound, Actions: []string{}}
		if session.Status == challenge.StatusActive {
			a.Actions = []string{"challenge_advance"}
		}
		facts = append(facts, a)
	}
	view, err := r.casino.GetCharacterRoomView(ctx, owner, actor)
	if err != nil {
		return nil, fmt.Errorf("casino activity: %w", err)
	}
	if view != nil {
		controls, err := view.Controls(actor)
		if err != nil {
			return nil, err
		}
		a := playercontext.Activity{Kind: "casino", ID: view.Room.ID, Phase: string(view.Room.Status), Round: view.Room.Round, Role: controls.Role, Actions: []string{"casino_room_leave"}}
		if controls.CanStart {
			a.Actions = append(a.Actions, "casino_room_start", "casino_room_kick")
		}
		if len(controls.Actions) > 0 {
			a.Actions = append(a.Actions, "casino_room_action")
		}
		facts = append(facts, a)
	}
	return facts, nil
}
