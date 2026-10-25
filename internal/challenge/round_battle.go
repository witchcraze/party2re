package challenge

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) resolveRound(ctx context.Context, session *ChallengeSession, leader corecharacter.Character, enemies ...corebattle.Participant) (corebattle.Result, bool, error) {
	// Older solo snapshots have no roster. Upgrade on the next computed round.
	if len(session.Members) == 0 {
		session.Members = []ChallengeMember{{CharacterID: leader.ID, CharacterName: leader.Name,
			JobID: leader.JobID, Level: leader.Level, CharacterCurrentHP: session.CharacterCurrentHP,
			MaxHP: leader.Stats.MaxHP, MaxMP: leader.Stats.MaxMP, Attack: leader.Stats.Attack,
			Defense: leader.Stats.Defense, Agility: leader.Stats.Agility}}
	}
	allies := make([]corebattle.Participant, 0, len(session.Members))
	for i := range session.Members {
		m := &session.Members[i]
		if m.CharacterCurrentMP == nil {
			// Pre-upgrade buffers have only a shared reward ledger.
			m.AccumulatedExp, m.AccumulatedGold = session.AccumulatedExp, session.AccumulatedGold
			if m.CharacterCurrentHP <= 0 {
				char, err := s.charRepo.FindByID(ctx, m.CharacterID)
				if err != nil {
					return corebattle.Result{}, false, err
				}
				m.CharacterCurrentMP = &char.Stats.MP
			}
		}
		if m.CharacterCurrentHP <= 0 {
			continue
		}
		var p corebattle.Participant
		var err error
		if s.participantBuilder != nil {
			p, err = s.participantBuilder.BuildParticipantWithState(ctx, m.CharacterID, m.CharacterCurrentHP, m.CharacterCurrentMP, m.ConsumedItems)
		} else {
			var char corecharacter.Character
			char, err = s.charRepo.FindByID(ctx, m.CharacterID)
			if err == nil {
				p = buildFallbackParticipantWithHP(char, m.CharacterCurrentHP)
			}
		}
		if err != nil {
			return corebattle.Result{}, false, err
		}
		if m.CharacterCurrentMP != nil {
			p.MP = *m.CharacterCurrentMP
		}
		mp := p.MP
		m.CharacterCurrentMP = &mp
		allies = append(allies, p)
	}
	res := corebattle.PartyBattleResult{}
	var display corebattle.Result
	if len(allies) == 0 || len(enemies) == 0 {
		res.RemainingHP = map[string]int{}
	} else if resolver, ok := s.battleEngine.(corebattle.PartyBattleResolver); ok {
		var err error
		res, err = resolver.ResolvePartyBattle(corebattle.PartyBattleRequest{Allies: allies, Enemies: enemies, RNG: s.rng})
		if err != nil {
			return display, false, err
		}
		display = corebattle.Result{Outcome: res.Outcome, WinnerID: leader.ID, LoserID: enemies[0].ID, Turns: res.Turns, Logs: res.Logs}
	} else {
		// A two-combatant resolver cannot safely execute a party roster.
		if len(session.Members) > 1 {
			return display, false, corebattle.ErrInvalidRequest
		}
		var err error
		display, err = s.battleEngine.Resolve(corebattle.Request{Participants: []corebattle.Participant{allies[0], enemies[0]}, RNG: s.rng})
		if err != nil {
			return display, false, err
		}
		res.Outcome = display.Outcome
		if display.Outcome == corebattle.OutcomeWin && display.WinnerID == allies[0].ID {
			res.WinnerSide = "allies"
		}
		if len(display.Logs) > 0 {
			res.RemainingHP = display.Logs[len(display.Logs)-1].RemainingHP
		}
	}
	won := res.Outcome == corebattle.OutcomeWin && res.WinnerSide == "allies"
	for i := range session.Members {
		m := &session.Members[i]
		if hp, ok := res.RemainingHP[m.CharacterID]; ok {
			m.CharacterCurrentHP = max(0, hp)
			if m.MaxHP > 0 {
				m.CharacterCurrentHP = min(m.CharacterCurrentHP, m.MaxHP)
			}
		} else if !won {
			m.CharacterCurrentHP = 0
		}
		if mp, ok := res.RemainingMP[m.CharacterID]; ok {
			mp = max(0, mp)
			if m.MaxMP > 0 {
				mp = min(mp, m.MaxMP)
			}
			m.CharacterCurrentMP = &mp
		}
		m.ConsumedItems = append(slices.Clone(m.ConsumedItems), res.ConsumedItems[m.CharacterID]...)
		if m.CharacterID == leader.ID {
			session.CharacterCurrentHP = m.CharacterCurrentHP
		}
	}
	return display, won, nil
}
