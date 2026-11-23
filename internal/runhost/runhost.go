// Package runhost is the mode-neutral run host shared by every room library: the
// durable snapshot, enrollment, receipts, recipients, idle cleanup, individual
// exit, treasure inspection and the per-command transaction. A room library
// (Adventure stages, Challenge, ...) supplies only its own rules through Room.
package runhost

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/witchcraze/party2re/internal/battle"
	corebattle "github.com/witchcraze/party2re/internal/core/battle"
	corecharacter "github.com/witchcraze/party2re/internal/core/character"
	coreinventory "github.com/witchcraze/party2re/internal/core/inventory"
	"github.com/witchcraze/party2re/internal/core/job"
	"github.com/witchcraze/party2re/internal/core/random"
)

var (
	ErrConflict             = errors.New("run command conflicts with committed state")
	ErrForbidden            = errors.New("run actor is not authorized")
	ErrBusy                 = errors.New("character has unfinished run work")
	ErrUnavailable          = errors.New("run command implementation unavailable")
	ErrCooldown             = errors.New("run actor interval has not elapsed")
	ErrRecovery             = errors.New("run requires recovery")
	ErrNotFound             = errors.New("run not found")
	ErrNoParticipants       = errors.New("run requires at least 1 participant")
	ErrNotParticipant       = errors.New("character is not a participant in this run")
	ErrCharacterUnconscious = errors.New("character is unconscious (HP <= 0) and cannot enter a run")
	ErrCharacterExhausted   = errors.New("character is exhausted (tired >= 100) and must rest")
	ErrInvalidAction        = errors.New("invalid action for the current room state")
	ErrActorUnconscious     = errors.New("unconscious character cannot open treasure")
	ErrNoUnopenedTreasure   = errors.New("no unopened treasure boxes remaining")
	ErrTreasureLimitReached = errors.New("character has reached treasure inspection limit")
)

// State is the host-owned part of a durable run. A room library's snapshot
// embeds it, so the library's fields persist beside it in one flat document.
// Character records and executable callbacks never belong in this snapshot.
type State struct {
	ID        string    `json:"id"`
	Version   int       `json:"version"`
	Encoding  int       `json:"encoding"`
	Kind      string    `json:"kind"`
	Admission Admission `json:"admission"`
	LeaderID  string    `json:"leader_id"`
	Roster    []string  `json:"roster"`
	// Round is legacy $round: every mode's progress counter (floor, Lv.).
	Round        int                      `json:"round"`
	Enemies      []string                 `json:"enemies"`
	Battle       corebattle.LifetimeState `json:"battle"`
	LastActions  map[string]time.Time     `json:"last_actions"`
	Recipients   map[string]Recipient     `json:"recipients"`
	Boxes        []TreasureBox            `json:"boxes"`
	StartedAt    time.Time                `json:"started_at"`
	LastActivity time.Time                `json:"last_activity"`
	TimedOut     bool                     `json:"timed_out"`
}

// Snapshot is a room library's run type: a pointer exposing its embedded State.
type Snapshot[R any] interface {
	*R
	Host() *State
}

// Admission is frozen under Party's roster lock. Generation is the lobby
// identity for Party admission and the owned request identity for solo
// admission. StageID, Season and Mode are lobby parameters whose meaning the
// room library owns.
type Admission struct {
	Generation   string                     `json:"generation"`
	PartyID      string                     `json:"party_id"`
	ActorID      string                     `json:"actor_id"`
	CharacterIDs []string                   `json:"character_ids"`
	Actors       []corebattle.LifetimeActor `json:"actors,omitempty"`
	StageID      string                     `json:"stage_id"`
	Season       string                     `json:"season"`
	Mode         string                     `json:"mode"`
	Speed        int                        `json:"speed"`
}

type Earnings struct {
	EXP      int `json:"experience"`
	Gold     int `json:"gold"`
	Crystals int `json:"crystals"`
}

type Recipient struct {
	PlayerID     string    `json:"player_id"`
	Pending      Earnings  `json:"pending"`
	Conditional  Earnings  `json:"conditional"`
	Applied      Earnings  `json:"applied"`
	Inspected    int       `json:"inspected"`
	Disposition  string    `json:"disposition"`
	PenaltyUntil time.Time `json:"penalty_until"`
	TimerPending bool      `json:"timer_pending"`
}

type Command struct {
	PlayerID string `json:"player_id"`
	ActorID  string `json:"actor_id"`
	RunID    string `json:"run_id"`
	ID       string `json:"id"`
	Version  int    `json:"version"`
	Action   string `json:"action"`
	TargetID string `json:"target_id"`
	Input    string `json:"input"`
}

// Receipt contains only the executing actor's result, including after exit.
type Receipt struct {
	Command      Command                    `json:"command"`
	Version      int                        `json:"version"`
	Round        int                        `json:"round"`
	Disposition  string                     `json:"disposition"`
	Box          *TreasureBox               `json:"box,omitempty"`
	Recipient    Recipient                  `json:"recipient"`
	MonsterTames []battle.MonsterTameResult `json:"monster_tames,omitempty"`
}

type ActorRun struct {
	RunID        string
	PartyID      string
	Version      int
	Round        int
	LeaderID     string
	Role         string
	IdleDue      bool
	LastActivity time.Time
	Recipient    Recipient
}

// RetainedEarnings is an unpaid unconditional balance that a timed-out run left
// in its recipient ledger after the active room was disposed.
type RetainedEarnings struct {
	RunID     string
	Recipient Recipient
}

// RetainedEarningsStore owns the claim path for RetainedEarnings. The read
// precedes settlement; the write runs after it (Rank 8), locks the row and fails
// with ErrConflict unless its unpaid balance equals the credited one.
type RetainedEarningsStore interface {
	FindRetainedEarnings(context.Context, string) ([]RetainedEarnings, error)
	SaveRetainedRecipient(context.Context, string, string, Earnings, Recipient) error
}

// Store keeps receipts and recipient dispositions beyond active state
// disposal. WithRunForUpdate must start before any ambient character transaction.
type Store[R any] interface {
	RetainedEarningsStore
	WithRunForUpdate(context.Context, string, bool, func(context.Context) error) error
	FindRun(context.Context, string) (*R, error)
	SaveRun(context.Context, R, int) error
	FindRunReceipt(context.Context, string, string) (*Receipt, error)
	SaveRunReceipt(context.Context, Receipt) error
	EnrollRun(context.Context, State) error
	SaveRunRecipient(context.Context, string, string, Recipient) error
	FindActorRun(context.Context, string) (*ActorRun, error)
	FindAdmission(context.Context, string) (*Admission, error)
	FindPenalty(context.Context, string) (*ActorRun, error)
}

type Characters interface {
	FindByID(context.Context, string) (corecharacter.Character, error)
	FindByIDForUpdate(context.Context, string) (corecharacter.Character, error)
	Update(context.Context, corecharacter.Character) error
}

type Inventories interface {
	FindByCharacterID(context.Context, string) (coreinventory.Inventory, error)
}

type ActorBuilder interface {
	BuildLifetimeActor(context.Context, string, string) (corebattle.LifetimeActor, error)
}

type Settler interface {
	ApplyPostBattleResult(context.Context, battle.ApplyPostBattleRequest) (battle.ApplyPostBattleResponse, error)
}

type Clock interface{ Now() time.Time }

// Lobby serializes live waiting-roster validation, but never owns SQL asset
// locks. Roster validation runs under the matching SQL handoff lock.
type Lobby interface {
	WithRoster(context.Context, string, string, func(context.Context, Admission) error) error
	CleanupLobby(context.Context, Admission) error
	CheckSoloAdmission(context.Context, string) error
}

// Penalty projects a committed absolute deadline. SQL ownership remains held
// until this succeeds; Home must independently enforce the durable deadline.
type Penalty interface {
	ProjectRunPenalty(context.Context, string, time.Time) error
}

// Room is a room library: the mode rules the host consults (legacy vs_*.cgi
// and the $m{lib} branches of shared routines). Hooks run inside the host's
// command transaction after the run and character locks.
type Room[R any] interface {
	// Prepare validates and normalizes the lobby parameters before enrollment.
	Prepare(*Admission) error
	// AdmitMember checks one locked member against the room's entry rules.
	AdmitMember(ctx context.Context, r *R, id string) error
	// Open fixes the room after member checks. release undoes an entry claim
	// when the enrollment does not commit.
	Open(ctx context.Context, r *R) (release func(context.Context) error, err error)
	// Actions are the room's own commands (add_battle_action), run by Act.
	Actions() []string
	Act(ctx context.Context, r *R, cmd Command) (Effects, error)
	// Exit returns the room's exit bonus and fatigue for actor.
	Exit(r *R, actor string) (Earnings, int, error)
	// Record writes actor's history at exit or timeout.
	Record(ctx context.Context, r *R, actor, disposition string) error
	// Conditional reports whether new earnings are held until the room converts them.
	Conditional(r *R) bool
}

// Combat executes an explicit player operation and its NPC response, and the
// stock-level operation. It must admit the action before effects, use the
// ambient transaction and return a rejection rather than absorb it, so that
// Command saves neither the run nor a receipt.
type Combat[R any] interface {
	Execute(ctx context.Context, r *R, cmd Command) (Effects, error)
	StockLevel(ctx context.Context, r *R, cmd Command, requester corecharacter.Character, now time.Time) error
}

// Effects are one command's new earnings, consumption and settlement facts.
type Effects struct {
	Earnings        map[string]Earnings
	Consumed        map[string][]corebattle.ConsumedItem
	RoomActivity    bool
	ActorActivity   bool
	ReleaseStock    bool
	UnsealDemonKing bool
	Defeats         []battle.ExplicitDefeat
}

type Config[R any] struct {
	// Kind namespaces run identities derived from admission generations.
	Kind        string
	Store       Store[R]
	Characters  Characters
	Inventories Inventories
	Builder     ActorBuilder
	Settler     Settler
	Jobs        job.DefinitionProvider
	Lobby       Lobby
	Penalty     Penalty
	Clock       Clock
	RNG         random.Generator
	CheckEntry  func(context.Context, string) error
	ExitEffects func(context.Context, string) error
	Room        Room[R]
	Combat      Combat[R]
}

type Service[R any, P Snapshot[R]] struct{ cfg Config[R] }

func NewService[R any, P Snapshot[R]](cfg Config[R]) (*Service[R, P], error) {
	if cfg.Kind == "" || cfg.Store == nil || cfg.Characters == nil || cfg.Builder == nil || cfg.Settler == nil || cfg.Clock == nil || cfg.Penalty == nil || cfg.Room == nil {
		return nil, ErrUnavailable
	}
	if cfg.RNG == nil {
		cfg.RNG = random.Default()
	}
	return &Service[R, P]{cfg: cfg}, nil
}

func (s *Service[R, P]) SetExitEffects(apply func(context.Context, string) error) {
	s.cfg.ExitEffects = apply
}

// ID derives a run identity from an admission generation.
func (s *Service[R, P]) ID(generation string) string {
	h := sha256.Sum256([]byte(s.cfg.Kind + ":" + generation))
	return hex.EncodeToString(h[:16])
}

type commandContext struct{}
type admissionContext struct{}

// CommandContext marks ctx as running inside runID's command transaction.
func CommandContext(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, commandContext{}, runID)
}

// InCommand reports whether ctx runs inside runID's command transaction.
func InCommand(ctx context.Context, runID string) bool {
	return ctx.Value(commandContext{}) == runID
}

// CheckMutation protects conflicting service mutations without taking a run
// lock after character locks. Native commands carry the already-held run identity.
func (s *Service[R, P]) CheckMutation(ctx context.Context, actor string) error {
	run, err := s.cfg.Store.FindActorRun(ctx, actor)
	if err != nil {
		return err
	}
	if run != nil && !InCommand(ctx, run.RunID) {
		return ErrBusy
	}
	return nil
}
