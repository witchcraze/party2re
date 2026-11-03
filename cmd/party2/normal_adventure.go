package main

import (
	"context"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/timer"
	"github.com/witchcraze/party2re/internal/party"
)

type normalAdmissionAdapter struct {
	service *party.Service
	store   *party.ValkeyRepository
}

func (a normalAdmissionAdapter) CheckSoloAdmission(ctx context.Context, actor string) error {
	_, _, err := a.service.GetActiveParty(ctx, actor)
	if errors.Is(err, party.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	return adventure.ErrRunBusy
}

func (a normalAdmissionAdapter) WithNormalRoster(ctx context.Context, actor, partyID string, start func(context.Context, adventure.NormalAdmission) error) error {
	return a.service.WithNormalRoster(ctx, actor, partyID, func(ctx context.Context, p party.Party, ids []string) error {
		return start(ctx, adventure.NormalAdmission{Generation: "party:" + p.ID, PartyID: p.ID, ActorID: actor, CharacterIDs: ids, StageID: p.StageID, Season: p.Season, Mode: p.Mode, Speed: p.Speed})
	})
}

func (a normalAdmissionAdapter) CleanupNormalLobby(ctx context.Context, snapshot adventure.NormalAdmission) error {
	state := party.LobbyState{Party: party.Party{ID: snapshot.PartyID, LeaderCharacterID: snapshot.ActorID}}
	for _, id := range snapshot.CharacterIDs {
		state.Members = append(state.Members, party.Member{PartyID: snapshot.PartyID, CharacterID: id})
	}
	return a.store.CleanupParty(ctx, state)
}

type normalPenaltyAdapter struct {
	timers interface {
		SetLockUntil(context.Context, string, string, time.Time) error
	}
}

func (a normalPenaltyAdapter) ProjectRunPenalty(ctx context.Context, actor string, until time.Time) error {
	return a.timers.SetLockUntil(ctx, timer.CategorySleep, actor, until)
}
