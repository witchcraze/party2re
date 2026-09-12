package boss

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

// StartSealingBattle starts and resolves an authentic 4-player Party Sealing Battle (vs_king.cgi).
func (s *Service) StartSealingBattle(ctx context.Context, partyID, leaderCharID string) (SealingBattleResult, error) {
	if s.partyRepo == nil {
		return SealingBattleResult{}, errors.New("party repository not configured")
	}

	var result SealingBattleResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		// 1. Lock party
		p, err := s.partyRepo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return ErrPartyNotFound
		}
		if p.LeaderCharacterID != leaderCharID {
			return ErrNotPartyLeader
		}
		if p.Status != party.StatusRecruiting {
			return ErrPartyNotRecruiting
		}

		// 2. Fetch members
		members, err := s.partyRepo.GetMembers(txCtx, partyID)
		if err != nil || len(members) == 0 {
			return ErrPartyNotReady
		}
		for _, m := range members {
			if !m.ReadyState {
				return ErrPartyNotReady
			}
		}

		// 3. Resolve stage
		stage, ok := s.stageMap[p.StageID]
		if !ok {
			return ErrBossNotFound
		}

		// 4. Lock all character rows in ascending order (Rank 2)
		charIDs := make([]string, len(members))
		for i, m := range members {
			charIDs[i] = m.CharacterID
		}
		sort.Strings(charIDs)

		chars := make(map[string]corecharacter.Character, len(charIDs))
		for _, cID := range charIDs {
			c, err := s.characterRepo.FindByIDForUpdate(txCtx, cID)
			if err != nil {
				return ErrCharacterNotFound
			}
			if c.Stats.HP <= 0 {
				return ErrCharacterUnconscious
			}
			if c.Tired >= 100 {
				return ErrCharacterExhausted
			}
			if err := party.ValidateNeedJoin(stage.NeedJoin, c); err != nil {
				return ErrNeedJoinNotMet
			}
			chars[cID] = c
		}

		// 5. Entry fatigue cost: each participant gets +20% Tired ($m{tired} += 20)
		for _, cID := range charIDs {
			c := chars[cID]
			c.Tired += 20
			if c.Tired > 100 {
				c.Tired = 100
			}
			chars[cID] = c
		}

		// 6. Build ally participants
		allies := make([]corebattle.Participant, 0, len(members))
		orderedChars := make([]corecharacter.Character, 0, len(members))
		for _, m := range members {
			c := chars[m.CharacterID]
			allies = append(allies, corebattle.NewParticipantFromCharacter(c))
			orderedChars = append(orderedChars, c)
		}

		// 7. Build boss participants
		bossParticipants := s.buildBossParticipants(stage, orderedChars)

		// 8. Resolve Battle
		req := corebattle.PartyBattleRequest{
			Allies:  allies,
			Enemies: bossParticipants,
			VictoryReward: corebattle.Reward{
				Experience: s.calculateTotalExp(stage, orderedChars),
				Currency:   s.calculateTotalGold(stage, orderedChars),
			},
		}

		partyEngine, ok := s.battleEngine.(corebattle.PartyBattleResolver)
		var battleRes corebattle.PartyBattleResult
		if ok {
			res, err := partyEngine.ResolvePartyBattle(req)
			if err != nil {
				return fmt.Errorf("party battle resolution failed: %w", err)
			}
			battleRes = res
		} else {
			res, err := corebattle.Engine{}.ResolvePartyBattle(req)
			if err != nil {
				return fmt.Errorf("party battle resolution failed: %w", err)
			}
			battleRes = res
		}

		// 9. Process Banished combatants (Dejon) and HP/MP
		banishedList := make([]string, 0)
		for _, cID := range charIDs {
			c := chars[cID]
			if remHP, ok := battleRes.RemainingHP[cID]; ok {
				if remHP <= 0 {
					c.Stats.HP = 1
				} else {
					c.Stats.HP = remHP
					if c.Stats.MaxHP > 0 && c.Stats.HP > c.Stats.MaxHP {
						c.Stats.HP = c.Stats.MaxHP
					}
				}
			}
			if remMP, ok := battleRes.RemainingMP[cID]; ok && remMP >= 0 {
				c.Stats.MP = remMP
				if c.Stats.MaxMP > 0 && c.Stats.MP > c.Stats.MaxMP {
					c.Stats.MP = c.Stats.MaxMP
				}
			}

			// Dejon banishment: +30% Tired ($m{tired} += 30)
			if battleRes.BanishedIDs != nil && battleRes.BanishedIDs[cID] {
				banishedList = append(banishedList, cID)
				c.Tired += 30
				if c.Tired > 100 {
					c.Tired = 100
				}
			}
			chars[cID] = c
		}

		// 10. Victory & Resealing (@ふういん)
		outcome := battleRes.Outcome
		isWin := (outcome == corebattle.OutcomeWin)
		var rewardItemID string
		var newsMsg string
		banquetHeld := false
		heroCountGained := 0

		if isWin {
			heroCountGained = 1
			if len(stage.TreasureItemIDs) > 0 {
				rewardItemID = s.pickTreasure(stage.TreasureItemIDs)
			}

			heroNames := make([]string, 0, len(members))
			for _, m := range members {
				c := chars[m.CharacterID]
				c.HeroCount++
				_ = c.AddMoney(battleRes.TotalReward.Currency)
				if battleRes.TotalReward.Experience > 0 {
					_, _ = progression.ApplyExperience(&c, battleRes.TotalReward.Experience)
				}
				chars[m.CharacterID] = c
				heroNames = append(heroNames, c.Name)
			}

			newsMsg = fmt.Sprintf("勇者%sが%sを封印する", strings.Join(heroNames, "、"), stage.Name)
			if s.newsPub != nil {
				_ = s.newsPub.PublishNews(txCtx, "boss", newsMsg, newsMsg, "System", time.Now().UTC())
			}

			leaderChar := chars[leaderCharID]
			stageTier := stageTierFromID(stage.ID)
			if s.victoryBanquetHook != nil {
				_ = s.victoryBanquetHook(txCtx, stage.ID, stage.Name, leaderChar.ID, leaderChar.Name, stageTier)
				banquetHeld = true
			}
			if s.victoryHook != nil {
				_ = s.victoryHook(txCtx, leaderChar.ID, stage.ID, stageTier)
			}
		}

		// 11. Persist updated characters in DB
		for _, cID := range charIDs {
			if err := s.characterRepo.Update(txCtx, chars[cID]); err != nil {
				return err
			}
		}

		// 12. Record challenge history & records
		now := time.Now().UTC()
		var rewardItemInst *coreitem.Instance
		if rewardItemID != "" {
			rewardItemInst = &coreitem.Instance{
				ID:           id.New(),
				DefinitionID: rewardItemID,
				Quantity:     1,
			}
		}

		stageTier := stageTierFromID(stage.ID)
		for _, cID := range charIDs {
			rec, _ := s.repo.GetOrCreateRecord(txCtx, cID)
			isFirstClear := false
			if isWin {
				rec.TotalBossDefeats++
				if rec.FirstClearedAt == nil {
					rec.FirstClearedAt = &now
				}
				if stageTier > rec.HighestTierCleared {
					rec.HighestTierCleared = stageTier
					isFirstClear = true
				}
			}
			rec.LastChallengedAt = &now

			hist := BossChallengeHistory{
				ID:           id.New(),
				CharacterID:  cID,
				BossID:       stage.ID,
				Tier:         stageTier,
				Outcome:      outcome,
				Turns:        battleRes.Turns,
				RewardExp:    battleRes.TotalReward.Experience,
				RewardGold:   battleRes.TotalReward.Currency,
				RewardItemID: rewardItemID,
				IsFirstClear: isFirstClear,
				CreatedAt:    now,
			}
			if err := s.repo.RecordChallenge(txCtx, hist, rec, chars[cID], rewardItemInst); err != nil {
				return fmt.Errorf("failed to record challenge for %s: %w", cID, err)
			}
		}

		// 13. Disband party lobby
		_ = s.partyRepo.DeleteParty(txCtx, partyID)

		result = SealingBattleResult{
			StageID:           stage.ID,
			StageName:         stage.Name,
			Outcome:           outcome,
			Turns:             battleRes.Turns,
			RewardExp:         battleRes.TotalReward.Experience,
			RewardGold:        battleRes.TotalReward.Currency,
			RewardItemID:      rewardItemID,
			HeroCountGained:   heroCountGained,
			BanishedMemberIDs: banishedList,
			NewsMessage:       newsMsg,
			BanquetHeld:       banquetHeld,
			BattleResult:      battleRes,
		}
		return nil
	})

	return result, err
}

// ChallengeBoss executes a sealing battle encounter for a single character against the specified King stage.
func (s *Service) ChallengeBoss(ctx context.Context, characterID, bossID string) (SealingBattleResult, error) {
	if characterID == "" {
		return SealingBattleResult{}, ErrCharacterNotFound
	}
	if bossID == "" {
		return SealingBattleResult{}, ErrInvalidBossID
	}

	stage, ok := s.stageMap[bossID]
	if !ok {
		return SealingBattleResult{}, ErrBossNotFound
	}

	var result SealingBattleResult
	err := s.runInTx(ctx, func(txCtx context.Context) error {
		char, err := s.characterRepo.FindByIDForUpdate(txCtx, characterID)
		if err != nil {
			return ErrCharacterNotFound
		}
		if char.Stats.HP <= 0 {
			return ErrCharacterUnconscious
		}
		if char.Tired >= 100 {
			return ErrCharacterExhausted
		}
		if err := party.ValidateNeedJoin(stage.NeedJoin, char); err != nil {
			return ErrNeedJoinNotMet
		}

		// Entry fatigue cost: +20% Tired ($m{tired} += 20)
		char.Tired += 20
		if char.Tired > 100 {
			char.Tired = 100
		}

		allies := []corebattle.Participant{
			corebattle.NewParticipantFromCharacter(char),
		}
		orderedChars := []corecharacter.Character{char}
		bossParticipants := s.buildBossParticipants(stage, orderedChars)

		req := corebattle.PartyBattleRequest{
			Allies:  allies,
			Enemies: bossParticipants,
			VictoryReward: corebattle.Reward{
				Experience: s.calculateTotalExp(stage, orderedChars),
				Currency:   s.calculateTotalGold(stage, orderedChars),
			},
		}

		partyEngine, ok := s.battleEngine.(corebattle.PartyBattleResolver)
		var battleRes corebattle.PartyBattleResult
		if ok {
			res, err := partyEngine.ResolvePartyBattle(req)
			if err != nil {
				return fmt.Errorf("battle resolution failed: %w", err)
			}
			battleRes = res
		} else {
			res, err := corebattle.Engine{}.ResolvePartyBattle(req)
			if err != nil {
				return fmt.Errorf("battle resolution failed: %w", err)
			}
			battleRes = res
		}

		banishedList := make([]string, 0)
		if remHP, ok := battleRes.RemainingHP[char.ID]; ok {
			if remHP <= 0 {
				char.Stats.HP = 1
			} else {
				char.Stats.HP = remHP
				if char.Stats.MaxHP > 0 && char.Stats.HP > char.Stats.MaxHP {
					char.Stats.HP = char.Stats.MaxHP
				}
			}
		}
		if remMP, ok := battleRes.RemainingMP[char.ID]; ok && remMP >= 0 {
			char.Stats.MP = remMP
			if char.Stats.MaxMP > 0 && char.Stats.MP > char.Stats.MaxMP {
				char.Stats.MP = char.Stats.MaxMP
			}
		}

		// Dejon banishment: +30% Tired ($m{tired} += 30)
		if battleRes.BanishedIDs != nil && battleRes.BanishedIDs[char.ID] {
			banishedList = append(banishedList, char.ID)
			char.Tired += 30
			if char.Tired > 100 {
				char.Tired = 100
			}
		}

		outcome := battleRes.Outcome
		isWin := (outcome == corebattle.OutcomeWin)
		var rewardItemID string
		var newsMsg string
		banquetHeld := false
		heroCountGained := 0

		if isWin {
			heroCountGained = 1
			if len(stage.TreasureItemIDs) > 0 {
				rewardItemID = s.pickTreasure(stage.TreasureItemIDs)
			}
			char.HeroCount++
			_ = char.AddMoney(battleRes.TotalReward.Currency)
			if battleRes.TotalReward.Experience > 0 {
				_, _ = progression.ApplyExperience(&char, battleRes.TotalReward.Experience)
			}

			newsMsg = fmt.Sprintf("勇者%sが%sを封印する", char.Name, stage.Name)
			if s.newsPub != nil {
				_ = s.newsPub.PublishNews(txCtx, "boss", newsMsg, newsMsg, "System", time.Now().UTC())
			}

			stageTier := stageTierFromID(stage.ID)
			if s.victoryBanquetHook != nil {
				_ = s.victoryBanquetHook(txCtx, stage.ID, stage.Name, char.ID, char.Name, stageTier)
				banquetHeld = true
			}
			if s.victoryHook != nil {
				_ = s.victoryHook(txCtx, char.ID, stage.ID, stageTier)
			}
		}

		if err := s.characterRepo.Update(txCtx, char); err != nil {
			return err
		}

		now := time.Now().UTC()
		var rewardItemInst *coreitem.Instance
		if rewardItemID != "" {
			rewardItemInst = &coreitem.Instance{
				ID:           id.New(),
				DefinitionID: rewardItemID,
				Quantity:     1,
			}
		}

		stageTier := stageTierFromID(stage.ID)
		rec, _ := s.repo.GetOrCreateRecord(txCtx, char.ID)
		isFirstClear := false
		if isWin {
			rec.TotalBossDefeats++
			if rec.FirstClearedAt == nil {
				rec.FirstClearedAt = &now
			}
			if stageTier > rec.HighestTierCleared {
				rec.HighestTierCleared = stageTier
				isFirstClear = true
			}
		}
		rec.LastChallengedAt = &now

		hist := BossChallengeHistory{
			ID:           id.New(),
			CharacterID:  char.ID,
			BossID:       stage.ID,
			Tier:         stageTier,
			Outcome:      outcome,
			Turns:        battleRes.Turns,
			RewardExp:    battleRes.TotalReward.Experience,
			RewardGold:   battleRes.TotalReward.Currency,
			RewardItemID: rewardItemID,
			IsFirstClear: isFirstClear,
			CreatedAt:    now,
		}
		if err := s.repo.RecordChallenge(txCtx, hist, rec, char, rewardItemInst); err != nil {
			return fmt.Errorf("failed to record challenge: %w", err)
		}

		result = SealingBattleResult{
			StageID:           stage.ID,
			StageName:         stage.Name,
			Outcome:           outcome,
			Turns:             battleRes.Turns,
			RewardExp:         battleRes.TotalReward.Experience,
			RewardGold:        battleRes.TotalReward.Currency,
			RewardItemID:      rewardItemID,
			HeroCountGained:   heroCountGained,
			BanishedMemberIDs: banishedList,
			NewsMessage:       newsMsg,
			BanquetHeld:       banquetHeld,
			BattleResult:      battleRes,
		}
		return nil
	})

	return result, err
}
