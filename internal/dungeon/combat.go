package dungeon

import (
	"context"
	"slices"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	corerandom "github.com/witchcraze/party2re/internal/core/random"
)

// encounterOutcome is one battle's effect on the run: per-member HP/MP deltas,
// consumed items and the leader's HP delta mirrored by the run-level HP.
type encounterOutcome struct {
	result   corebattle.Result
	field    *corebattle.FieldState
	won      bool
	memberHP map[string]int
	memberMP map[string]int
	consumed map[string][]corebattle.ConsumedItem
	leaderHP int
}

func (s *Service) resolveEncounter(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	enemies []corebattle.Participant,
	allyStatus string,
) (encounterOutcome, error) {
	allies, err := s.buildCombatAllies(ctx, exp, char, allyStatus)
	if err != nil {
		return encounterOutcome{}, err
	}
	if len(allies) == 0 {
		return encounterOutcome{result: corebattle.Result{Outcome: corebattle.OutcomeDefeat}, memberHP: make(map[string]int), leaderHP: -exp.CurrentHP}, nil
	}
	initialMP := make(map[string]int, len(allies))
	for _, a := range allies {
		initialMP[a.ID] = a.MP
	}
	var initialField *corebattle.FieldState
	if exp.Encounter != nil {
		initialField = exp.Encounter.Field
	}
	partyRes, err := s.battleEngine.ResolvePartyBattle(corebattle.PartyBattleRequest{
		Allies:       allies,
		InitialField: initialField,
		Enemies:      enemies,
		RNG:          adaptRandomSource(s.rng),
	})
	if err != nil {
		return encounterOutcome{}, err
	}
	won := partyRes.Outcome == corebattle.OutcomeWin && partyRes.WinnerSide == "allies"
	winnerID, loserID := enemies[0].ID, char.ID
	if won {
		winnerID, loserID = char.ID, enemies[0].ID
	}
	out := encounterOutcome{
		result: corebattle.Result{
			Outcome:         partyRes.Outcome,
			WinnerID:        winnerID,
			LoserID:         loserID,
			Turns:           partyRes.Turns,
			RemainingHP:     partyRes.RemainingHP,
			RemainingMP:     partyRes.RemainingMP,
			RemainingCMP:    partyRes.RemainingCMP,
			RemainingStatus: partyRes.RemainingStatus,
			ConsumedItems:   partyRes.ConsumedItems,
			Logs:            partyRes.Logs,
		},
		field:    partyRes.FinalField,
		won:      won,
		memberHP: make(map[string]int),
		memberMP: make(map[string]int),
		consumed: make(map[string][]corebattle.ConsumedItem),
	}

	if len(exp.Members) > 0 {
		for _, m := range exp.Members {
			if m.CurrentHP <= 0 {
				continue
			}
			if remHP, ok := partyRes.RemainingHP[m.CharacterID]; ok {
				out.memberHP[m.CharacterID] = remHP - m.CurrentHP
			} else if !won {
				out.memberHP[m.CharacterID] = -m.CurrentHP
			}
			if remMP, ok := partyRes.RemainingMP[m.CharacterID]; ok {
				out.memberMP[m.CharacterID] = remMP - m.CurrentMP
			} else if initMP, ok := initialMP[m.CharacterID]; ok && !m.HasCurrentMP() {
				out.memberMP[m.CharacterID] = initMP - m.CurrentMP
			}
			if items := partyRes.ConsumedItems[m.CharacterID]; len(items) > 0 {
				out.consumed[m.CharacterID] = items
			}
		}
	} else {
		hpDelta := 0
		if remHP, ok := partyRes.RemainingHP[char.ID]; ok {
			hpDelta = remHP - exp.CurrentHP
		} else if len(partyRes.Logs) > 0 {
			if remHP, ok := partyRes.Logs[len(partyRes.Logs)-1].RemainingHP[char.ID]; ok {
				hpDelta = remHP - exp.CurrentHP
			}
		} else if !won {
			hpDelta = -exp.CurrentHP
		}
		out.memberHP[char.ID] = hpDelta
	}

	if d, ok := out.memberHP[char.ID]; ok {
		out.leaderHP = d
	} else if !won {
		out.leaderHP = -exp.CurrentHP
	}
	return out, nil
}

func (s *Service) resolveMonsterCombat(
	ctx context.Context,
	exp *ActiveExpedition,
	char *corecharacter.Character,
	monsters []DungeonMonster,
	eventType TileEventType,
	newX, newY int,
	eventKey string,
	allyStatus string,
	revived map[string]int,
	// carry holds the event's map state and floor change, saved with the combat step.
	carry StepParams,
	memberDamages map[string]int,
) (ExpeditionStepResult, error) {
	rounds, stayX, stayY := exp.combatProgress(newX, newY)
	var enemies []corebattle.Participant
	if exp.Encounter != nil {
		enemies = exp.Encounter.Enemies
	} else {
		enemies = participants(monsters)
	}
	living := slices.DeleteFunc(slices.Clone(enemies), func(e corebattle.Participant) bool { return e.HP <= 0 })
	out, err := s.resolveEncounter(ctx, exp, char, living, allyStatus)
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	for id, d := range revived {
		out.memberHP[id] += d
	}
	out.leaderHP += revived[char.ID]

	if out.won {
		// shortcut: the whole group's rewards land at victory until #1957 credits each defeat.
		var expReward, goldReward int
		for _, m := range monsters {
			expReward += m.ExpReward
			goldReward += m.GoldReward
		}
		memberExpDeltas := make(map[string]int, len(exp.Members))
		memberGoldDeltas := make(map[string]int, len(exp.Members))
		for _, m := range exp.Members {
			if remHP, ok := out.result.RemainingHP[m.CharacterID]; ok && remHP > 0 {
				memberExpDeltas[m.CharacterID] = expReward
				memberGoldDeltas[m.CharacterID] = goldReward
			}
		}
		leaderExpDelta := memberExpDeltas[char.ID]
		leaderGoldDelta := memberGoldDeltas[char.ID]
		if len(exp.Members) <= 1 && out.leaderHP+exp.CurrentHP > 0 {
			leaderExpDelta = expReward
			leaderGoldDelta = goldReward
		}

		stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
			ExpectedExpeditionID: exp.ID,
			ExpectedRevision:     &exp.Revision,
			Combat:               true,
			NewX:                 newX,
			NewY:                 newY,
			HPDelta:              out.leaderHP,
			MemberHPDeltas:       out.memberHP,
			MemberMPDeltas:       out.memberMP,
			MemberExpDeltas:      memberExpDeltas,
			MemberGoldDeltas:     memberGoldDeltas,
			ConsumedItems:        out.consumed,
			RoundDelta:           rounds,
			EventKey:             eventKey,
			MapState:             carry.MapState,
			MapVariant:           carry.MapVariant,
			RoundLimit:           carry.RoundLimit,
			ExpDelta:             leaderExpDelta,
			GoldDelta:            leaderGoldDelta,
			Now:                  time.Now().UTC(),
		})
		if err != nil {
			return ExpeditionStepResult{}, err
		}

		return ExpeditionStepResult{
			Expedition:    stepRes.Expedition,
			EventType:     eventType,
			BattleResult:  &out.result,
			MemberDamages: memberDamages,
			ExpEarned:     leaderExpDelta,
			GoldFound:     leaderGoldDelta,
			Monsters:      monsters,
			CombatOutcome: corebattle.OutcomeWin,
		}, nil
	}

	// Draw or defeat: surviving enemies stay. An all-fallen party remains in the
	// run, where only Escape is left (legacy never ends a quest because allies fell).
	// After a mutual knockout no enemy survives, so no encounter is kept.
	pending := pendingEncounter(exp, monsters, enemies, eventType, newX, newY, out.result, out.field)
	stepRes, err := s.activeStore.Step(ctx, char.ID, StepParams{
		ExpectedExpeditionID: exp.ID,
		ExpectedRevision:     &exp.Revision,
		Combat:               true,
		NewX:                 stayX,
		NewY:                 stayY,
		Encounter:            pending,
		HPDelta:              out.leaderHP,
		MemberHPDeltas:       out.memberHP,
		MemberMPDeltas:       out.memberMP,
		ConsumedItems:        out.consumed,
		RoundDelta:           rounds,
		EventKey:             eventKey,
		MapState:             carry.MapState,
		MapVariant:           carry.MapVariant,
		RoundLimit:           carry.RoundLimit,
		Now:                  time.Now().UTC(),
	})
	if err != nil {
		return ExpeditionStepResult{}, err
	}
	return ExpeditionStepResult{
		Expedition:    stepRes.Expedition,
		EventType:     eventType,
		BattleResult:  &out.result,
		MemberDamages: memberDamages,
		Monsters:      monsters,
		CombatOutcome: out.result.Outcome,
	}, nil
}

// combatProgress: the round advances only on the move itself, and the party
// already stands on the destination (vs_dungeon.cgi _susumu).
func (exp *ActiveExpedition) combatProgress(newX, newY int) (rounds, stayX, stayY int) {
	if exp.Encounter != nil {
		return 0, newX, newY
	}
	return 1, newX, newY
}

func buildFallbackParticipantWithHP(char corecharacter.Character, currentHP int) corebattle.Participant {
	hp := currentHP
	if hp <= 0 {
		hp = char.Stats.HP
		if hp <= 0 && char.Stats.MaxHP > 0 {
			hp = char.Stats.MaxHP
		}
	}
	p := corebattle.MustNewParticipant(char.ID, hp, char.Stats.Attack, char.Stats.Defense)
	p.Name = char.Name
	p.MaxHP = char.Stats.MaxHP
	p.MP = char.Stats.MP
	p.MaxMP = char.Stats.MaxMP
	p.Agility = char.Stats.Agility
	return p
}

type randomSourceAdapter struct {
	src RandomSource
}

func (a randomSourceAdapter) Intn(n int) int {
	return a.src.Intn(n)
}

func (a randomSourceAdapter) IntN(n int) int {
	return a.src.Intn(n)
}

func (a randomSourceAdapter) Float64() float64 {
	return a.src.Float64()
}

func adaptRandomSource(src RandomSource) corerandom.Generator {
	if src == nil {
		return nil
	}
	if gen, ok := src.(corerandom.Generator); ok {
		return gen
	}
	return randomSourceAdapter{src: src}
}
