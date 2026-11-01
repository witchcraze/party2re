package dungeon

import (
	"context"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

func (s *Service) buildCombatAllies(ctx context.Context, exp *ActiveExpedition, char *corecharacter.Character) ([]corebattle.Participant, error) {
	if len(exp.Members) == 0 {
		var playerPart corebattle.Participant
		var err error
		if s.participantBuilder != nil {
			playerPart, err = s.participantBuilder.BuildParticipantWithCurrentHP(ctx, char.ID, exp.CurrentHP)
			if err != nil {
				return nil, err
			}
		} else {
			playerPart = buildFallbackParticipantWithHP(*char, exp.CurrentHP)
		}
		exp.Encounter.restoreParticipant(&playerPart)
		playerPart.TeamID = "allies"
		return []corebattle.Participant{playerPart}, nil
	}

	allies := make([]corebattle.Participant, 0, len(exp.Members))
	for i := range exp.Members {
		m := &exp.Members[i]
		if m.CurrentHP <= 0 {
			continue
		}
		var p corebattle.Participant
		var err error
		if s.participantBuilder != nil {
			p, err = s.participantBuilder.BuildParticipantWithState(ctx, m.CharacterID, m.CurrentHP, &m.CurrentMP, m.ConsumedItems)
		} else {
			var memberChar corecharacter.Character
			if m.CharacterID == char.ID {
				memberChar = *char
			} else {
				memberChar, err = s.characterRepo.FindByID(ctx, m.CharacterID)
			}
			if err == nil {
				p = buildFallbackParticipantWithHP(memberChar, m.CurrentHP)
				p.MP = m.CurrentMP
				p.MaxMP = m.MaxMP
			}
		}
		if err != nil {
			return nil, err
		}
		exp.Encounter.restoreParticipant(&p)
		p.TeamID = "allies"
		allies = append(allies, p)
	}
	return allies, nil
}
