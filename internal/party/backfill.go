package party

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/witchcraze/party2re/internal/adventure"
	"github.com/witchcraze/party2re/internal/core/battle"
)

// AdventureLogReader retrieves party adventure logs for backfill.
type AdventureLogReader interface {
	GetAdventureLogsByPartyID(ctx context.Context, partyID string) ([]PartyAdventureLog, error)
}

// BackfillPartyAdventureLogs backfills past party_adventure_logs into individual character adventures.
// For normal stages, character IDs are reconstructed from details_json (allies_survived + allies_fallen)
// and saved with is_cleared = (outcome == "WIN" / battle.OutcomeWin).
// For stage-26, past party logs lack runtime seasonal variant metadata, so they are saved with
// seasonal_sub_id = 0 (unclassified season).
func BackfillPartyAdventureLogs(
	ctx context.Context,
	logRepo AdventureLogReader,
	advRepo AdventureRepository,
	partyID string,
) (int, error) {
	if logRepo == nil || advRepo == nil {
		return 0, nil
	}
	logs, err := logRepo.GetAdventureLogsByPartyID(ctx, partyID)
	if err != nil {
		return 0, err
	}

	backfilled := 0
	for _, log := range logs {
		if log.DetailsJSON == "" {
			continue
		}
		var battleRes battle.PartyBattleResult
		if err := json.Unmarshal([]byte(log.DetailsJSON), &battleRes); err != nil {
			continue
		}

		charIDs := make([]string, 0, len(battleRes.AlliesSurvived)+len(battleRes.AlliesFallen))
		seen := make(map[string]bool)
		for _, id := range append(battleRes.AlliesSurvived, battleRes.AlliesFallen...) {
			if id != "" && !seen[id] {
				seen[id] = true
				charIDs = append(charIDs, id)
			}
		}

		isCleared := log.Outcome == string(battle.OutcomeWin)
		outcome := battle.Outcome(log.Outcome)

		for _, cID := range charIDs {
			advIDHash := sha256.Sum256([]byte(fmt.Sprintf("party-backfill:%s:%s", log.ID, cID)))
			advID := hex.EncodeToString(advIDHash[:16])

			winnerID := ""
			if outcome == battle.OutcomeWin {
				winnerID = cID
			}

			adv := adventure.Adventure{
				ID:               advID,
				CharacterID:      cID,
				Type:             log.StageID,
				StageID:          log.StageID,
				StartedAt:        log.CreatedAt,
				FloorsCleared:    10,
				IsCleared:        isCleared,
				PartySize:        len(charIDs),
				ExperienceReward: log.TotalEXP,
				Resolved:         true,
				BattleResult: battle.Result{
					Outcome:  outcome,
					WinnerID: winnerID,
					Turns:    log.Turns,
					Reward: battle.Reward{
						Experience: log.TotalEXP,
						Currency:   log.TotalGold,
					},
				},
			}
			if err := advRepo.Save(ctx, adv); err != nil {
				return backfilled, fmt.Errorf("backfill adventure record for %s: %w", cID, err)
			}
			backfilled++
		}
	}
	return backfilled, nil
}
