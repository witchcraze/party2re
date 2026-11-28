package boss

import (
	"context"
	"fmt"
	"log/slog"
	"reflect"
	"slices"
	"strings"

	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	"github.com/witchcraze/party2re/internal/runhost"
)

// sealedRound is the round ＠ふういん enters (vs_king.cgi:55): the treasure round.
const sealedRound = 2

// sealingRoom is the sealing room library (vs_king.cgi) the run host consults
// inside its command transaction.
type sealingRoom struct{ s *RunService }

// Prepare and AdmitMember refuse lobby admission: characters enter a
// sealing room only by joining its running battle.
func (sealingRoom) Prepare(*runhost.Admission) error { return runhost.ErrUnavailable }
func (sealingRoom) AdmitMember(context.Context, *Run, string) error {
	return runhost.ErrUnavailable
}
func (sealingRoom) Open(context.Context, *Run) (func(context.Context) error, error) {
	return nil, runhost.ErrUnavailable
}

func (sealingRoom) Actions() []string { return []string{"seal"} }

// Act is ＠ふういん (vs_king.cgi fuuin): with every enemy defeated, the
// requester's party gains Hero Count, the news and celebration follow and the
// room enters the treasure round with fresh chests and no field.
func (room sealingRoom) Act(ctx context.Context, r *Run, cmd runhost.Command) (runhost.Effects, error) {
	if r.Round >= sealedRound {
		return runhost.Effects{}, ErrSealed
	}
	if r.LivingEnemies() {
		return runhost.Effects{}, ErrEnemiesRemain
	}
	s := room.s
	stage, ok := s.byID[r.Admission.StageID]
	actor := r.Actor(cmd.ActorID)
	if !ok || actor == nil {
		return runhost.Effects{}, runhost.ErrConflict
	}
	r.Round++
	var names []string
	for _, a := range partys(r, actor.TeamID) {
		names = append(names, a.Name)
		if !slices.Contains(r.Roster, a.ID) {
			continue
		}
		c, err := s.cfg.Characters.FindByID(ctx, a.ID)
		if err != nil {
			return runhost.Effects{}, err
		}
		c.HeroCount++
		if err := s.cfg.Characters.Update(ctx, c); err != nil {
			return runhost.Effects{}, err
		}
	}
	if err := room.addTreasure(ctx, r, stage, actor); err != nil {
		return runhost.Effects{}, err
	}
	now := s.cfg.Clock.Now()
	// delete_all_field(1) removes the temporary field only (_battle.cgi:1560–1566).
	r.Battle.Fields.Temporary, r.Battle.Fields.Turns = "", 0
	msg := fmt.Sprintf("勇者%sが%sを封印する", strings.Join(names, "、"), stage.Name)
	fx := runhost.Effects{}
	if s.cfg.News != nil {
		fx.Settled = func(ctx context.Context) error { return s.cfg.News.PublishNews(ctx, "boss", msg, msg, "System", now) }
	}
	if s.banquet != nil {
		fx.Committed = func(ctx context.Context) {
			if err := s.banquet(ctx); err != nil {
				slog.WarnContext(ctx, "sealing celebration failed", "error_type", reflect.TypeOf(err).String())
			}
		}
	}
	return fx, nil
}

// partys is @partys: actor team's members in member order, fallen included.
func partys(r *Run, team string) []corebattle.LifetimeActor {
	var out []corebattle.LifetimeActor
	for _, a := range r.Battle.Actors {
		if a.TeamID == team && !a.Banished && !a.RosterDetached {
			out = append(out, a)
		}
	}
	return out
}

// addTreasure is add_treasure (_npc_action.cgi:498–549) as ＠ふういん and
// job78's treasure retry call it: it replaces the chests with ones drawn from
// the stage's @treasures plus the weekday orb, counted from the requester's
// @partys and Chapel blessing.
func (room sealingRoom) addTreasure(ctx context.Context, r *Run, stage BossStage, actor *corebattle.LifetimeActor) error {
	s := room.s
	party := partys(r, actor.TeamID)
	fallen := 0
	for _, a := range party {
		if a.HP <= 0 {
			fallen++
		}
	}
	blessing := ""
	if s.cfg.Blessings != nil {
		var err error
		if blessing, err = s.cfg.Blessings.GetActiveBlessing(ctx, actor.ID); err != nil {
			return fmt.Errorf("treasure blessing: %w", err)
		}
	}
	count := runhost.TreasureCount(0, len(party), fallen, runhost.IsTreasureBlessing(blessing), s.cfg.RNG.Intn)
	r.Boxes = runhost.GenerateTreasureBoxes(count, stage.Treasures, s.cfg.Clock.Now(), s.cfg.RNG.Intn)
	return nil
}

// Exit is nigeru's sealing part; sealing rooms have no calm mode.
func (room sealingRoom) Exit(r *Run, actor string) (runhost.Earnings, int, error) {
	return r.DefaultExit(actor, room.s.cfg.RNG)
}

// Record is a no-op: nigeru prints no result screen for type 6.
func (sealingRoom) Record(context.Context, *Run, string, string) error { return nil }

func (sealingRoom) Conditional(*Run) bool { return false }

// Join is add_member's type 6 branch (quest.cgi:928–1077): only before
// sealing, below the stage's player capacity and under its join condition;
// joining adds 20 Tired.
func (room sealingRoom) Join(r *Run, c *corecharacter.Character) error {
	stage, ok := room.s.byID[r.Admission.StageID]
	switch {
	case !ok:
		return runhost.ErrConflict
	case r.Round >= sealedRound:
		return ErrSealed
	case len(r.Roster) >= stage.MaxMembers:
		return runhost.ErrRoomFull
	}
	if err := corecharacter.ValidateNeedJoin(stage.NeedJoin, *c); err != nil {
		return fmt.Errorf("%w: %v", ErrNeedJoinNotMet, err)
	}
	c.AddTired(20)
	return nil
}

// Retain is nigeru's last-exit check (_battle.cgi:1331): the room survives
// while its leader boss lives. A defeated NPC leaves the roster at save
// (_battle.cgi:523), so a missing leader counts as defeated.
func (sealingRoom) Retain(r *Run) bool {
	a := r.Actor(r.Leader)
	return a != nil && a.HP > 0
}

// Banish is dejon's effect on the removed player's data (vs_king.cgi:80–98).
func (sealingRoom) Banish(_ *Run, c *corecharacter.Character) error {
	c.AddTired(30)
	return nil
}

// Choices are add_battle_action's しらべる in the treasure round and ふういん
// once no enemy remains (vs_king.cgi:28–38).
func (sealingRoom) Choices(r *Run, _ string) []runhost.Choice {
	switch {
	case r.Round == sealedRound:
		return []runhost.Choice{{Action: "chest", Source: "しらべる"}}
	case !r.LivingEnemies():
		return []runhost.Choice{{Action: "seal", Source: "ふういん"}}
	}
	return nil
}

// Responds is false in the treasure round (vs_king.cgi:30).
func (sealingRoom) Responds(r *Run) bool { return r.Round != sealedRound }

// Combat binds the vs_king branches: crystals scaled by the unseal clock
// (_battle.cgi:152–170), the room's NPC skills こうげき and デジョン, and
// job78's treasure retry through add_treasure (_skill.cgi:6214–6218).
func (room sealingRoom) Combat(ctx context.Context, r *Run, actor string, _ *corebattle.EquipmentResetContext, _ bool) (runhost.RoomCombat, error) {
	s := room.s
	unsealedAt, err := s.cfg.UnsealClock.GetUnsealedAt(ctx)
	if err != nil {
		return runhost.RoomCombat{}, fmt.Errorf("read unseal clock: %w", err)
	}
	now := s.cfg.Clock.Now()
	stage, ok := s.byID[r.Admission.StageID]
	// vs_king.cgi:19–23 loads the stage file on every request, drawing its pool.
	pool := stage.summonPool(s.cfg.RNG)
	jobRoom := runhost.JobRoom{Validate: func(id string) error {
		if id != "job-78-entry-6" || !ok || r.Actor(actor) == nil {
			return corebattle.ErrUnsupportedEffect
		}
		return nil
	}}
	jobRoom.Apply = func(id string) error {
		if err := jobRoom.Validate(id); err != nil {
			return err
		}
		return room.addTreasure(ctx, r, stage, r.Actor(actor))
	}
	return runhost.RoomCombat{
		JobRoom: jobRoom,
		Habitat: func() string { return stage.Name },
		Summons: func() runhost.Summons { return runhost.Summons{EnemyPool: pool} },
		Scale: func(e runhost.Earnings) runhost.Earnings {
			if unsealedAt != nil {
				e.Crystals = scaleCrystals(e.Crystals, now.Sub(*unsealedAt))
			}
			return e
		},
		NPCSkills:  npcSkills,
		NPCEntries: map[string]corebattle.CallbackDefinition{dejonEntry: dejon},
	}, nil
}
