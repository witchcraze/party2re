package pvp

import (
	"context"
	"slices"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	"github.com/witchcraze/party2re/internal/runhost"
)

// matchRoom is the PvP room library (vs_player.cgi) the run host consults
// inside its command transaction.
type matchRoom struct{ s *RunService }

// Prepare keeps only the lobby's speed, stage and name.
func (matchRoom) Prepare(a *runhost.Admission) error {
	a.Season, a.Mode = "", ""
	return nil
}

// AdmitMember is a no-op: members passed the join checks in the lobby and
// the first かいし checks none again.
func (matchRoom) AdmitMember(context.Context, *Run, string) error { return nil }

// Open adopts the lobby room committed under the handoff lock: target wins,
// bet escrow and team colors. Then it starts round 1 as かいし does.
func (room matchRoom) Open(ctx context.Context, r *Run) (func(context.Context) error, error) {
	t, err := room.s.cfg.Lobby.transitions.FindTransition(ctx, r.Admission.PartyID)
	if err != nil {
		return nil, err
	}
	if t.Pending || t.Detail.Room.Status != StatusRecruiting || len(t.Detail.Members) != len(r.Roster) {
		return nil, ErrTransitionConflict
	}
	r.Teams, r.TargetWins, r.Pool, r.Wins = true, t.Detail.Room.TargetWins, t.Detail.Room.PrizePool, map[string]int{}
	// vs_player doubles created field durations (_battle.cgi:1615) and still
	// exercises named-field tails with npc_turn disabled (_battle.cgi:1718).
	r.Battle.Fields.DoubleTurns, r.Battle.Fields.ExerciseWithoutResponse = true, true
	for _, m := range t.Detail.Members {
		a := r.Actor(m.CharacterID)
		if a == nil {
			return nil, ErrTransitionConflict
		}
		a.TeamID = m.TeamColor
	}
	// The leader's first かいし is its action (_battle.cgi:1176).
	r.LastActions[r.Admission.ActorID] = room.s.cfg.Clock.Now()
	return nil, room.s.startRound(ctx, r, r.Admission.ActorID)
}

func (matchRoom) Actions() []string { return []string{"start"} }

func (room matchRoom) Act(ctx context.Context, r *Run, cmd runhost.Command) (runhost.Effects, error) {
	return room.s.kaishi(ctx, r, cmd.ActorID)
}

// Exit is nigeru's PvP part. The bet stays in the pool: only a lobby departure
// before round 1 refunds it (_battle.cgi:1229–1233).
func (room matchRoom) Exit(r *Run, actor string) (runhost.Earnings, int, error) {
	return r.DefaultExit(actor, room.s.cfg.RNG)
}

// Record is a no-op: nigeru writes history only for quest types 1–3.
func (matchRoom) Record(context.Context, *Run, string, string) error { return nil }

func (matchRoom) Conditional(*Run) bool { return false }

// Choices is add_battle_action's かいし, offered in every round (vs_player.cgi:40–41).
func (matchRoom) Choices(*Run, string) []runhost.Choice {
	return []runhost.Choice{{Action: "start", Source: "かいし"}}
}

// Responds is false: vs_player disables npc_turn (vs_player.cgi:219).
func (matchRoom) Responds(*Run) bool { return false }

// Combat sets legacy $win, which every PvP room has.
func (matchRoom) Combat(context.Context, *Run, string, *corebattle.EquipmentResetContext, bool) (runhost.RoomCombat, error) {
	return runhost.RoomCombat{Win: true}, nil
}

// kaishi is a later かいし (vs_player.cgi:94–183). Its checks run in legacy
// order; a fallen member may call it (_battle.cgi:659–662).
func (s *RunService) kaishi(ctx context.Context, r *Run, actor string) (runhost.Effects, error) {
	if r.Paid {
		return runhost.Effects{}, ErrMatchFinished
	}
	// Checked before the tally, so round 11 is played but never counted.
	if r.Round > 10 {
		return runhost.Effects{}, ErrRoundLimit
	}
	alive := map[string]bool{}
	for _, a := range inRoom(r) {
		if a.HP > 0 {
			alive[a.TeamID] = true
		}
	}
	switch len(alive) {
	case 0: // draw: no tally
	case 1:
		for color := range alive {
			r.Wins[color]++
			if r.Wins[color] >= r.TargetWins {
				return s.giveBet(r, color), nil
			}
		}
	default:
		return runhost.Effects{}, ErrNotDecided
	}
	return runhost.Effects{}, s.startRound(ctx, r, actor)
}

// startRound is かいし's round start (vs_player.cgi:165–182). The waits it
// gives other members only drive their countdown display, never the action gate.
func (s *RunService) startRound(ctx context.Context, r *Run, actor string) error {
	requester, err := s.cfg.Characters.FindByID(ctx, actor)
	if err != nil {
		return err
	}
	reset, err := r.ResetContext(actor, &requester.Level)
	if err != nil {
		return err
	}
	if err := battle.ResetAll(&r.Battle, &reset, s.cfg.RNG); err != nil {
		return err
	}
	r.Battle.Fields.ResetFloor()
	shuffle(r, s.cfg.RNG.Intn)
	for i := range r.Battle.Actors {
		r.Battle.Actors[i].HP = r.Battle.Actors[i].MaxHP
	}
	r.Round++
	return nil
}

// shuffle orders the room by color, the colors drawn in random order (_battle.cgi:1433–1457).
func shuffle(r *Run, intn func(int) int) {
	var colors []string
	for _, a := range r.Battle.Actors {
		if !slices.Contains(colors, a.TeamID) {
			colors = append(colors, a.TeamID)
		}
	}
	rank := map[string]int{}
	for i := 0; len(colors) > 0; i++ {
		k := intn(len(colors))
		rank[colors[k]] = i
		colors = slices.Delete(colors, k, k+1)
	}
	slices.SortStableFunc(r.Battle.Actors, func(a, b corebattle.LifetimeActor) int { return rank[a.TeamID] - rank[b.TeamID] })
	slices.SortStableFunc(r.Roster, func(a, b string) int { return rank[r.Actor(a).TeamID] - rank[r.Actor(b).TeamID] })
}

// giveBet pays int(pool / n) to each roster member of the winning color
// (vs_player.cgi:186–216). n counts every room member of that color, so an
// allied summon dilutes the share without receiving one; the remainder is lost.
func (s *RunService) giveBet(r *Run, color string) runhost.Effects {
	var n int
	var winners []string
	for _, a := range inRoom(r) {
		if a.TeamID != color {
			continue
		}
		n++
		if slices.Contains(r.Roster, a.ID) {
			winners = append(winners, a.ID)
		}
	}
	share := r.Pool / max(n, 1)
	name := r.Admission.Name
	r.Pool, r.Paid = 0, true
	return runhost.Effects{Settled: func(ctx context.Context) error {
		for _, id := range winners {
			if err := s.cfg.Prizes.HoldPrize(ctx, id, name, share); err != nil {
				return err
			}
		}
		return nil
	}}
}

// inRoom is legacy @members: the actors still in the room's roster arrays.
func inRoom(r *Run) []corebattle.LifetimeActor {
	return slices.DeleteFunc(slices.Clone(r.Battle.Actors), func(a corebattle.LifetimeActor) bool { return a.Banished || a.RosterDetached })
}
