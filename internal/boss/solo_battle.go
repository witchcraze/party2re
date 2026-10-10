package boss

import (
	"context"
	"fmt"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

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
	startTime := time.Now().UTC()
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
		char.AddTired(20)

		var p corebattle.Participant
		if s.participantBuilder != nil {
			var err error
			p, err = s.participantBuilder.BuildParticipant(ctx, char.ID)
			if err != nil {
				return err
			}
		} else {
			p = buildFallbackParticipant(char)
		}
		allies := []corebattle.Participant{p}
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
			remMP := -1
			if mp, hasMP := battleRes.RemainingMP[char.ID]; hasMP && mp >= 0 {
				remMP = mp
			}
			char.ApplyCombatSurvival(remHP, remMP, remHP <= 0)
		} else if remMP, ok := battleRes.RemainingMP[char.ID]; ok && remMP >= 0 {
			char.Stats.MP = remMP
			char.Stats.ClampVitality()
		}

		// Dejon banishment: +30% Tired ($m{tired} += 30)
		if battleRes.BanishedIDs != nil && battleRes.BanishedIDs[char.ID] {
			banishedList = append(banishedList, char.ID)
			char.AddTired(30)
		}

		outcome := battleRes.Outcome
		isWin := (outcome == corebattle.OutcomeWin)
		var rewardItemID string
		var newsMsg string
		var rewardCrystals int
		banquetHeld := false
		heroCountGained := 0

		if isWin {
			heroCountGained = 1

			candidates := append([]string(nil), stage.TreasureItemIDs...)
			orb := getDayOfWeekOrb(s.now(), s.rng)
			candidates = append(candidates, orb)
			rewardItemID = s.pickTreasure(candidates)

			elapsed := time.Since(startTime)
			if elapsed < 0 {
				elapsed = 0
			}
			rewardCrystals = calculateTotalCrystals(stage, []corecharacter.Character{char}, elapsed)

			char.HeroCount++
			if err := char.AddMoney(battleRes.TotalReward.Currency); err != nil {
				return fmt.Errorf("failed to add boss currency reward: %w", err)
			}
			if battleRes.TotalReward.Experience > 0 {
				_, _ = progression.ApplyExperience(&char, battleRes.TotalReward.Experience)
			}
			if rewardCrystals > 0 {
				if err := char.AddCrystal(rewardCrystals); err != nil {
					return fmt.Errorf("failed to add boss crystal reward: %w", err)
				}
			}

			newsMsg = fmt.Sprintf("勇者%sが%sを封印する", char.Name, stage.Name)
			if s.newsPub != nil {
				_ = s.newsPub.PublishNews(txCtx, "boss", newsMsg, newsMsg, "System", s.now())
			}

			if s.monsterRecorder != nil {
				if stage.ID == "king99" {
					hp := char.Stats.MaxHP * 50
					if hp <= 0 {
						hp = 5000
					}
					mp := char.Stats.MaxMP * 50
					if mp <= 0 {
						mp = 500
					}
					at := char.Stats.Attack * 2
					df := char.Stats.Defense * 2
					ag := char.Stats.Agility * 2
					strong := int(float64(hp) + float64(mp) + float64(at) + float64(df)*0.5 + float64(ag))
					exp := (char.Level + char.JobLevel) * 30
					gold := int(float64(char.Level)*0.5) * 30
					rec := DefeatedMonsterRecord{
						MonsterID:        "king99-clone",
						MonsterName:      "影",
						Habitat:          "封印戦",
						Icon:             "king99-clone",
						Strong:           strong,
						HP:               hp,
						MP:               mp,
						Attack:           at,
						Defense:          df,
						Agility:          ag,
						ExperienceReward: exp,
						GoldReward:       gold,
					}
					_ = s.monsterRecorder.RecordMonsterDefeat(txCtx, char.ID, rec)
				} else {
					for i, b := range stage.Bosses {
						monID := parseBossMonsterID(b.Icon, fmt.Sprintf("%s-boss-%d", stage.ID, i))
						hp := b.MaxHP
						if hp <= 0 {
							hp = b.HP
						}
						mp := b.MaxMP
						if mp <= 0 {
							mp = b.MP
						}
						strong := int(float64(hp) + float64(mp) + float64(b.Attack) + float64(b.Defense)*0.5 + float64(b.Agility))
						rec := DefeatedMonsterRecord{
							MonsterID:        monID,
							MonsterName:      b.Name,
							Habitat:          "封印戦",
							Icon:             b.Icon,
							Strong:           strong,
							HP:               hp,
							MP:               mp,
							Attack:           b.Attack,
							Defense:          b.Defense,
							Agility:          b.Agility,
							ExperienceReward: b.GetExp,
							GoldReward:       b.GetMoney,
						}
						_ = s.monsterRecorder.RecordMonsterDefeat(txCtx, char.ID, rec)
					}
				}
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

		now := s.now()
		var rewardItemInst *coreitem.Instance
		if rewardItemID != "" {
			rewardItemInst = &coreitem.Instance{
				ID:           id.New(),
				DefinitionID: rewardItemID,
				Quantity:     1,
			}
		}

		stageTier := stageTierFromID(stage.ID)
		rec, err := s.repo.GetOrCreateRecord(txCtx, char.ID)
		if err != nil {
			return fmt.Errorf("get or create boss record: %w", err)
		}
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
			RewardCrystals:    rewardCrystals,
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
