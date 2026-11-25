package adventure

import (
	"context"
	"slices"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
)

// UnsealHook runs after a stage19 treasure creation commits (make_vs_king).
// unsealers is the unsealing party, leader first; partyName is its quest name.
type UnsealHook func(ctx context.Context, partyName string, unsealers []corebattle.LifetimeActor)

// SetUnsealHook sets the hook the batch crawl calls after an unseal commits.
func (s *Service) SetUnsealHook(hook UnsealHook) {
	s.unsealHook = hook
}

// unsealers is the crawl party at its treasure creation, leader first, with
// each character's jobs and battle-row rewards.
func (s *CrawlSession) unsealers() []corebattle.LifetimeActor {
	chars := make(map[string]corecharacter.Character, len(s.Characters))
	for _, c := range s.Characters {
		chars[c.ID] = c
	}
	party := make([]corebattle.LifetimeActor, 0, len(s.Participants))
	for _, p := range s.Participants {
		a := corebattle.LifetimeActor{Participant: p}
		if c, ok := chars[p.ID]; ok {
			a.Jobs = corebattle.BattleJobs{JobID: c.JobID, SP: c.SP, OldJobID: c.OldJobID, OldSP: c.OldSP}
			a.ExperienceReward, a.GoldReward = c.BattleRowRewards()
		}
		if p.ID == s.LeaderID {
			party = slices.Insert(party, 0, a)
		} else {
			party = append(party, a)
		}
	}
	return party
}
