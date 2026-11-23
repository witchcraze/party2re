package adventure

import (
	"context"
	"time"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/core/random"
	"github.com/witchcraze/party2re/internal/runhost"
)

// Choices are vs_monster's add_battle_action entries: しらべる on a treasure
// floor, then すすむ whenever no enemy or unopened chest remains. advance
// rejects what susumu rejects (first advance, finished quest).
func (stageRoom) Choices(r *NormalRun, _ string) []runhost.Choice {
	var choices []runhost.Choice
	if isTreasureFloor(r.Round) {
		choices = append(choices, runhost.Choice{Action: "chest", Source: "しらべる"})
	}
	if !r.LivingEnemies() {
		choices = append(choices, runhost.Choice{Action: "advance", Source: "すすむ", Check: func(string) error {
			// Only advance's unsupported EX data is gated; susumu's rejections execute.
			if r.HasEX() && (r.Round == EXBossFloor && len(r.Stage.exTreasureItems()) == 0 || r.Round == TreasureRoomFloor && (r.Stage.Ordinary == nil || len(r.Stage.Ordinary.EXRoster) == 0)) {
				return runhost.ErrUnavailable
			}
			return nil
		}})
	}
	return choices
}

// Responds is false in treasure rooms (vs_monster.cgi:38–41).
func (stageRoom) Responds(r *NormalRun) bool { return !isTreasureFloor(r.Round) }

// Combat binds the stage's travel tools, job78 room and defeat modifiers.
// Travel and room transitions stay operation-local; Settle records the
// EX marker and floor clear credit.
func (room stageRoom) Combat(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext, stored bool) (runhost.RoomCombat, error) {
	s := room.s
	held := &combatTravelTransition{}
	jobRoom, encounter, err := s.prepareJobEncounters(ctx, r, actor, reset)
	if err != nil {
		return runhost.RoomCombat{}, err
	}
	// A later replacement wins: held, then stored, then job78's room.
	transitions := []*combatTravelTransition{held}
	var storedTravel *corebattle.StoredToolTravel
	if stored {
		var transition *combatTravelTransition
		storedTravel, transition = s.prepareStoredToolTravel(ctx, r, actor, reset)
		transitions = append(transitions, transition)
	}
	transitions = append(transitions, encounter)
	unlocked := false
	return runhost.RoomCombat{
		NormalStage:  true,
		JobRoom:      jobRoom,
		Tools:        s.heldTravelTools(ctx, r, actor, reset, held),
		StoredTravel: storedTravel,
		Summons: func() runhost.Summons {
			summons := runhost.Summons{EnemyPool: r.EncounterPool, AppearancePositions: r.AppearancePositions, Hard: r.Mode == CrawlModeHard}
			if t := r.Stage.Ordinary; t != nil {
				summons.EXPool, summons.EXXPool = s.payloads(t.EXRoster), s.payloads(t.EXXRoster)
				// Stage25's top-level boss-branch draw; shortcut: stage15's
				// reread draws and pool rebuild belong to #2691.
				if len(t.BossRosters) > 1 {
					summons.ReloadDraws = 1
				}
			}
			return summons
		},
		Habitat: func() string { return r.Stage.Name },
		Scale: func(e runhost.Earnings) runhost.Earnings {
			if r.Mode == CrawlModeCalm {
				e.EXP, e.Gold, e.Crystals = e.EXP/2, e.Gold/2, e.Crystals/2
			}
			return e
		},
		Credited: func(identity string, rng random.Generator) {
			unlocked = rollRoyalBeeEX(r.Stage, identity, rng) || unlocked
		},
		Respond: func(now time.Time) string { return combinedTransition(transitions).beginNPCResponse(r, now) },
		Settle: func(defeated bool) (bool, error) {
			r.EXUnlocked = r.EXUnlocked || unlocked
			if defeated && r.EncounterFloor >= 1 && r.EncounterFloor <= EXBossFloor && !isTreasureFloor(r.EncounterFloor) {
				r.FloorsCleared = max(r.FloorsCleared, r.EncounterFloor)
				if r.EncounterFloor == BossFloor {
					r.StageCleared = true
				}
			}
			return combinedTransition(transitions).UnsealDemonKing, nil
		},
	}, nil
}

// heldTravelTools binds the finite travel callbacks for a held tool. Execute
// mutates only the locked run; the host adopts the result.
func (s *NormalRunService) heldTravelTools(ctx context.Context, r *NormalRun, actor string, reset *corebattle.EquipmentResetContext, transition *combatTravelTransition) map[string]corebattle.CallbackDefinition {
	tools := make(map[string]corebattle.CallbackDefinition, len(travelTools))
	for _, tool := range travelTools {
		input := func(f *corebattle.CallbackContext) (combatTravelInput, error) {
			target := ""
			if len(f.Arguments) > 0 {
				target = f.Arguments[0]
			}
			return travelInput(tool, target)
		}
		tools[tool] = corebattle.CallbackDefinition{
			Validate: func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext) error {
				value, err := input(f)
				if err != nil {
					return err
				}
				return s.validateCombatTravel(r, actor, tool, value, reset)
			},
			Execute: func(_ *corebattle.LifetimeState, f *corebattle.CallbackContext, _ random.Generator) error {
				value, err := input(f)
				if err != nil {
					return err
				}
				t, err := s.applyCombatTravel(ctx, r, actor, tool, value, reset)
				*transition = t
				return err
			},
		}
	}
	return tools
}

// combinedTransition is the operation's room change: a replacement carries
// the old responders, and any genuine stage19 treasure creation unseals once.
func combinedTransition(transitions []*combatTravelTransition) combatTravelTransition {
	var t combatTravelTransition
	unseal := false
	for _, each := range transitions {
		if each.replaced {
			t = *each
		}
		unseal = unseal || each.UnsealDemonKing
	}
	t.ResponseSuppressed, t.UnsealDemonKing = false, unseal
	return t
}

// payloads resolves stage table IDs; a missing entry leaves the pool empty so
// its reader rejects instead of using a partial table.
func (s *NormalRunService) payloads(ids []string) []corebattle.LifetimeActor {
	pool := make([]corebattle.LifetimeActor, 0, len(ids))
	for _, id := range ids {
		m, err := s.cfg.Monsters.FindByID(id)
		if err != nil {
			return nil
		}
		pool = append(pool, m.NPCPayload())
	}
	return pool
}
