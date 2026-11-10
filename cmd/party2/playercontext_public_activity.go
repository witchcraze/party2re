package main

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/witchcraze/party2re/internal/gvg"
	"github.com/witchcraze/party2re/internal/party"
	"github.com/witchcraze/party2re/internal/pvp"
)

// publicActivity uses viewer-independent public reads, never the owned context query.
func (r contextActivityReaders) publicActivity(ctx context.Context, actor string) (bool, error) {
	active, err := r.normal.HasActiveRun(ctx, actor)
	if err != nil {
		return false, fmt.Errorf("public adventure activity: %w", err)
	}
	p, member, err := r.party.GetActiveParty(ctx, actor)
	if err != nil && !errors.Is(err, party.ErrNotFound) {
		return false, fmt.Errorf("public party activity: %w", err)
	}
	if err == nil && p.Status != party.StatusDisbanded {
		if member.CharacterID != actor || member.PartyID != p.ID {
			return false, errors.New("public party membership mismatch")
		}
		active = true
	}
	pv, err := r.pvp.GetCharacterRoom(ctx, actor)
	if err != nil && !errors.Is(err, pvp.ErrRoomNotFound) {
		return false, fmt.Errorf("public pvp activity: %w", err)
	}
	if err == nil && pv.Room.Status != pvp.StatusDisbanded {
		if !slices.ContainsFunc(pv.Members, func(m pvp.RoomMember) bool { return m.CharacterID == actor }) {
			return false, errors.New("public pvp membership mismatch")
		}
		active = true
	} else {
		claim, err := r.pvp.GetCharacterClaim(ctx, actor)
		if err != nil {
			return false, fmt.Errorf("public pvp claim: %w", err)
		}
		active = active || claim != ""
	}
	gv, err := r.gvg.GetCharacterRoom(ctx, actor)
	if err != nil && !errors.Is(err, gvg.ErrRoomNotFound) {
		return false, fmt.Errorf("public gvg activity: %w", err)
	}
	if err == nil && gv.Room.Status != gvg.StatusDisbanded {
		if !slices.ContainsFunc(gv.Members, func(m gvg.GvGMember) bool { return m.CharacterID == actor }) {
			return false, errors.New("public gvg membership mismatch")
		}
		active = true
	}
	exp, err := r.dungeon.GetActiveExpedition(ctx, actor)
	if err != nil {
		return false, fmt.Errorf("public dungeon activity: %w", err)
	}
	if exp != nil {
		if !exp.ContainsCharacter(actor) {
			return false, errors.New("public dungeon membership mismatch")
		}
		active = true
	}
	session, err := r.challenge.GetActiveSession(ctx, actor)
	if err != nil {
		return false, fmt.Errorf("public challenge activity: %w", err)
	}
	if session != nil {
		if !session.ContainsCharacter(actor) {
			return false, errors.New("public challenge membership mismatch")
		}
		active = true
	}
	casino, err := r.casino.HasCharacterRoom(ctx, actor)
	if err != nil {
		return false, fmt.Errorf("public casino activity: %w", err)
	}
	return active || casino, nil
}
