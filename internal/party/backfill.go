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

// AdventureFinder provides read lookup to verify if an adventure record already exists.
type AdventureFinder interface {
	FindByID(ctx context.Context, id string) (adventure.Adventure, error)
}

// BackfillPartyAdventureLogs backfills past party_adventure_logs into individual character adventures.
//
// Run Identity Parity:
// Uses deterministic adventure ID "party:<log.ID>:<cID>", matching live party crawl recording in
// StartPartyAdventure. If a run has already been recorded by live crawl or a previous backfill pass,
// existing records and their settled rewards are preserved without double-counting.
//
// Participant Reconstruction Constraints:
// Historical logs only store details_json from the final floor encounter (Floor 10 boss or defeat floor).
// Participants are reconstructed from AlliesSurvived + AlliesFallen. Participants who fell on earlier
// floors (Floors 1..9) and were absent from the final battle cannot be reconstructed from historical logs.
// Speculative participant guessing is avoided to preserve data integrity.
//
// Stage 26 Seasonal Variant:
// Past party logs lack persisted seasonal variant metadata, so Stage 26 records are saved with
// seasonal_sub_id = 0 (unclassified).
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

	finder, _ := advRepo.(AdventureFinder)

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
			advIDHash := sha256.Sum256([]byte(fmt.Sprintf("party:%s:%s", log.ID, cID)))
			advID := hex.EncodeToString(advIDHash[:16])

			if finder != nil {
				if existing, err := finder.FindByID(ctx, advID); err == nil && existing.ID != "" {
					// Already recorded (live crawl or previous backfill pass); preserve existing record and settled rewards
					continue
				}
			}

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
