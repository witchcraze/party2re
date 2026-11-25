package boss

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreitem "github.com/witchcraze/party2re/internal/core/item"
	"github.com/witchcraze/party2re/internal/core/progression"
	"github.com/witchcraze/party2re/internal/economy"
	"github.com/witchcraze/party2re/internal/id"
	"github.com/witchcraze/party2re/internal/party"
)

// StartSealingBattle starts and resolves a sealing room's battle (vs_king.cgi)
// against its stored enemy roster. Any room member may start it.
//
// SQL room receipts serialize settlement and retain the committed result/roster.
// Valkey cleanup runs after commit and is retried without resolving battle again.
// Transaction: RunInTx (SQL settlement only; cleanup follows commit)
// Lock Order: boss_room_settlements(0) -> characters(2, ascending ID) -> inventory_items(3) -> depot_items(5)
func (s *Service) StartSealingBattle(ctx context.Context, partyID, actorID string) (SealingBattleResult, error) {
	if s.partyRepo == nil {
		return SealingBattleResult{}, errors.New("party repository not configured")
	}

	var result SealingBattleResult
	err := s.partyRepo.WithPartyAdventureLock(ctx, partyID, func(lockedCtx context.Context) error {
		var err error
		result, err = s.completeSealingBattle(lockedCtx, partyID, actorID)
		return err
	})
	return result, err
}

func (s *Service) settleSealingBattle(txCtx context.Context, partyID, actorID string, original **party.Party) (*SealingSettlement, string, error) {
	var receipt *SealingSettlement
	var slayerName string
	err := func() error {
		// 1. Lock party
		p, err := s.partyRepo.GetPartyForUpdate(txCtx, partyID)
		if err != nil {
			return ErrPartyNotFound
		}
		if p.Kind != party.KindSealing || len(p.Enemies) == 0 {
			return ErrBossNotFound
		}
		if p.Status != party.StatusRecruiting && p.Status != party.StatusInProgress {
			return ErrPartyNotRecruiting
		}
		// The SQL room lock proves that an interrupted in_progress room has no committed battle.
		p.Status = party.StatusRecruiting
		*original = &p

		// 2. Fetch members
		members, err := s.partyRepo.GetMembers(txCtx, partyID)
		if err != nil || len(members) == 0 {
			return ErrPartyNotReady
		}
		if !hasMember(members, actorID) {
			return ErrNotRoomMember
		}

		// 3. Resolve stage
		stage, ok := s.stageMap[p.StageID]
		if !ok {
			return ErrBossNotFound
		}
		if len(members) > stage.MaxMembers {
			return ErrPartyNotReady
		}

		// 4. Lock all character rows in ascending order (Rank 2)
		charIDs := make([]string, len(members))
		for i, m := range members {
			charIDs[i] = m.CharacterID
		}
		chars := make(map[string]corecharacter.Character, len(charIDs))
		var orderedIDs []string
		if err := economy.LockInOrder(txCtx, charIDs, func(txCtx context.Context, cID string) error {
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
			orderedIDs = append(orderedIDs, cID)
			return nil
		}); err != nil {
			return err
		}
		charIDs = orderedIDs

		// Hide the active room from ordinary admission; the SQL receipt guards settlement retries.
		claimed := p
		claimed.Status = party.StatusInProgress
		if err := s.partyRepo.UpdateParty(txCtx, claimed); err != nil {
			return err
		}

		// 6. Build ally participants
		allies := make([]corebattle.Participant, 0, len(members))
		for _, m := range members {
			c := chars[m.CharacterID]
			var p corebattle.Participant
			if s.participantBuilder != nil {
				var err error
				p, err = s.participantBuilder.BuildParticipant(txCtx, c.ID)
				if err != nil {
					return err
				}
			} else {
				p = buildFallbackParticipant(c)
			}
			allies = append(allies, p)
		}

		// 7. Resolve the battle against the room's stored roster
		exp, gold := rosterRewards(p.Enemies)
		req := corebattle.PartyBattleRequest{
			Allies:        allies,
			Enemies:       rosterParticipants(p.Enemies),
			VictoryReward: corebattle.Reward{Experience: exp, Currency: gold},
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
		var banishedList []string
		for _, cID := range charIDs {
			c := chars[cID]
			if remHP, ok := battleRes.RemainingHP[cID]; ok {
				remMP := -1
				if mp, hasMP := battleRes.RemainingMP[cID]; hasMP && mp >= 0 {
					remMP = mp
				}
				c.ApplyCombatSurvival(remHP, remMP, remHP <= 0)
			} else if remMP, ok := battleRes.RemainingMP[cID]; ok && remMP >= 0 {
				c.Stats.MP = remMP
				c.Stats.ClampVitality()
			}

			// Dejon banishment: +30% Tired ($m{tired} += 30)
			if battleRes.BanishedIDs != nil && battleRes.BanishedIDs[cID] {
				banishedList = append(banishedList, cID)
				c.AddTired(30)
			}
			chars[cID] = c
		}

		// 10. Victory & Resealing (@ふういん)
		outcome := battleRes.Outcome
		isWin := (outcome == corebattle.OutcomeWin)
		var rewardItemID string
		var newsMsg string
		var rewardCrystals int
		heroCountGained := 0

		if isWin {
			heroCountGained = 1

			// 10a. Day-of-week orb bonus added to treasure candidates
			candidates := append([]string(nil), stage.TreasureItemIDs...)
			orb := getDayOfWeekOrb(s.now(), s.rng)
			candidates = append(candidates, orb)
			rewardItemID = s.pickTreasure(candidates)

			// 10b. Elapsed time crystal reward with clear-time multiplier from persisted unseal clock
			var unsealedAt *time.Time
			if s.unsealClock != nil {
				var err error
				unsealedAt, err = s.unsealClock.GetUnsealedAt(txCtx)
				if err != nil {
					return fmt.Errorf("failed to read unseal clock: %w", err)
				}
			}
			rewardCrystals = rosterCrystals(p.Enemies, unsealedAt, s.now())

			heroNames := make([]string, 0, len(members))
			for _, m := range members {
				c := chars[m.CharacterID]
				c.HeroCount++
				if err := c.AddMoney(battleRes.TotalReward.Currency); err != nil {
					return fmt.Errorf("failed to add boss currency reward: %w", err)
				}
				if battleRes.TotalReward.Experience > 0 {
					if _, err := progression.ApplyExperience(&c, battleRes.TotalReward.Experience); err != nil {
						return fmt.Errorf("failed to add boss experience reward: %w", err)
					}
				}
				if rewardCrystals > 0 {
					if err := c.AddCrystal(rewardCrystals); err != nil {
						return fmt.Errorf("failed to add boss crystal reward: %w", err)
					}
				}
				chars[m.CharacterID] = c
				heroNames = append(heroNames, c.Name)
			}

			newsMsg = fmt.Sprintf("勇者%sが%sを封印する", strings.Join(heroNames, "、"), stage.Name)
			if s.newsPub != nil {
				if err := s.newsPub.PublishNews(txCtx, "boss", newsMsg, newsMsg, "System", s.now()); err != nil {
					return fmt.Errorf("failed to publish boss news: %w", err)
				}
			}

			for _, m := range members {
				if err := s.recordBossDefeats(txCtx, stage.ID, m.CharacterID, p.Enemies); err != nil {
					return err
				}
			}

			slayerName = chars[actorID].Name
		}

		// 11. Persist updated characters in DB
		for _, cID := range charIDs {
			if err := s.characterRepo.Update(txCtx, chars[cID]); err != nil {
				return err
			}
		}

		// 12. Deliver winning treasure and sync character state
		for _, cID := range charIDs {
			var rewardItemInst *coreitem.Instance
			if rewardItemID != "" {
				rewardItemInst = &coreitem.Instance{
					ID:           id.New(),
					DefinitionID: rewardItemID,
					Quantity:     1,
				}
			}
			if err := s.repo.SettleMember(txCtx, chars[cID], rewardItemInst); err != nil {
				return fmt.Errorf("failed to settle member %s: %w", cID, err)
			}
		}

		result := SealingBattleResult{
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
			BattleResult:      battleRes,
		}
		receipt = &SealingSettlement{Lobby: party.LobbyState{Party: p, Members: members}, Result: result}
		return s.settlementRepo.SaveSealingSettlement(txCtx, *receipt)
	}()

	return receipt, slayerName, err
}

func hasMember(members []party.Member, characterID string) bool {
	for _, m := range members {
		if m.CharacterID == characterID {
			return true
		}
	}
	return false
}
