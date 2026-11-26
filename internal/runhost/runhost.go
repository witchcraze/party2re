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
	"github.com/witchcraze/party2re/internal/depot"
	"github.com/witchcraze/party2re/internal/economy"
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
	ErrRoomFull             = errors.New("run has no free member slot")
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
	// Teams builds every combat frame from colors as legacy read_member does:
	// each other team's living member is the requester's enemy. A room with
	// no encounter (PvP) sets it; Enemies then never decides a frame.
	Teams bool `json:"teams,omitempty"`
	// Retained keeps the run, its enemies and its round after its last member
	// leaves: a system room (vs_king) outlives its members until it is deleted.
	Retained bool `json:"retained,omitempty"`
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
	// Name is the quest name players gave the room (legacy $p_name).
	Name string `json:"name,omitempty"`
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
	Inspections  []Inspection               `json:"inspections,omitempty"`
}

// Inspection is one inspection body's facts in execution order: the
// requester's operation, then a responding NPC's.
type Inspection struct {
	ActorID string                      `json:"actor_id"`
	EntryID string                      `json:"entry_id"`
	Facts   []corebattle.InspectionFact `json:"facts"`
}

func appendInspections(dst []Inspection, actor string, results []corebattle.OrdinaryToolResult) []Inspection {
	for _, r := range results {
		if len(r.Facts) > 0 {
			dst = append(dst, Inspection{ActorID: actor, EntryID: r.ID, Facts: r.Facts})
		}
	}
	return dst
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

// RetainedEarnings is an unpaid unconditional balance left in a recipient
// ledger by a timed-out run after its room was disposed, or by a Dejon removal.
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

// SystemStore keeps system-opened runs. A slot is a run's unique live
// identity; disposal releases it.
type SystemStore[R any] interface {
	// OpenRun writes a new run under its run lock. It fails with ErrConflict
	// while another live run holds slot.
	OpenRun(ctx context.Context, run R, slot string) error
	// FindSlot returns the live run holding slot, or "".
	FindSlot(ctx context.Context, slot string) (string, error)
	// JoinRun enrolls actor into the locked run, replacing a recipient row the
	// actor left by exit or Dejon and adding that row's unreleased earnings to
	// recipient; it returns the stored recipient. It fails with ErrBusy when
	// actor is enrolled elsewhere. Lock Order: after the actor's character row.
	JoinRun(ctx context.Context, runID, actor string, order int, recipient Recipient) (Recipient, error)
}

// Store keeps receipts and recipient dispositions beyond active state
// disposal. WithRunForUpdate must start before any ambient character transaction.
type Store[R any] interface {
	RetainedEarningsStore
	SystemStore[R]
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

// Inventories locks party inventories before the requester's asset session
// and saves a changed tool hand inside it.
type Inventories interface {
	FindByCharacterID(context.Context, string) (coreinventory.Inventory, error)
	FindByCharacterIDForUpdate(context.Context, string) (coreinventory.Inventory, error)
	Save(context.Context, coreinventory.Inventory) error
}

type Blessings interface {
	GetActiveBlessing(context.Context, string) (string, error)
}

// JobAssetDepots locks and saves the requesting player's depot inside the
// ambient Command transaction, after the economy runner's inventory lock.
type JobAssetDepots interface {
	FindByCharacterIDForUpdate(context.Context, string) (depot.Depot, error)
	Save(context.Context, depot.Depot) error
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
	// Open fixes the room after member checks, once every member's actor is
	// built. release undoes an entry claim when the enrollment does not commit.
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

// SystemRoom is Room's part for a room the system opens with no members,
// that characters join while its battle runs (quest.cgi add_member, type 6)
// and that can outlive its members.
type SystemRoom[R any] interface {
	// Join checks c against the room's join rules and applies the join's own
	// effects to c; the host saves c in the join transaction.
	Join(r *R, c *corecharacter.Character) error
	// Retain reports whether r survives its last member's exit.
	Retain(r *R) bool
	// Banish applies the room's effects to a member a room NPC banished
	// (vs_king.cgi dejon); the host removes the member without settlement.
	Banish(r *R, c *corecharacter.Character) error
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
	Inspections     []Inspection
	// Settled writes the room's own records after settlement (Rank 8).
	Settled func(context.Context) error
	// Committed runs once after this command's own commit, never on replay
	// or an unconfirmed commit (best-effort side effects outside SQL).
	Committed func(context.Context)
}

type Config[R any] struct {
	// Kind namespaces run identities derived from admission generations.
	Kind string
	// MaxMembers caps an admitted roster; zero is legacy $max_party (4).
	MaxMembers  int
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
	// CombatRoom is Room's part of explicit combat; nil keeps the host defaults.
	CombatRoom CombatRoom[R]
	// SystemRoom enables Open, Join and retained empty runs; nil disables them.
	SystemRoom SystemRoom[R]
	Blessings  Blessings
	// Unsealed runs after a command's stage19 treasure creation commits, never
	// on receipt replay; it receives the run name and the party, leader first.
	Unsealed func(ctx context.Context, name string, unsealers []corebattle.LifetimeActor)

	// The requester's asset session for combat callbacks (job50/job78/job86);
	// without a runner, combat runs without one. JobAssetEquipment keeps
	// equipment_slots on a changed tool hand; explicit test adapters may omit it.
	JobAssetRunner    economy.TransactionRunner
	JobAssetHelpers   func(context.Context) ([]int, error)
	JobAssetDepots    JobAssetDepots
	JobAssetEquipment depot.EquipmentRepository
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

func (s *Service[R, P]) maxMembers() int {
	if s.cfg.MaxMembers > 0 {
		return s.cfg.MaxMembers
	}
	return 4
}

func (s *Service[R, P]) SetExitEffects(apply func(context.Context, string) error) {
	s.cfg.ExitEffects = apply
}

func (s *Service[R, P]) SetBlessings(provider Blessings) { s.cfg.Blessings = provider }

// ID derives a run identity from an admission generation.
func (s *Service[R, P]) ID(generation string) string {
	h := sha256.Sum256([]byte(s.cfg.Kind + ":" + generation))
	return hex.EncodeToString(h[:16])
}

type commandKey struct{}
type admissionContext struct{}

// commandContext marks ctx as running inside runID's command transaction.
func commandContext(ctx context.Context, runID string) context.Context {
	return context.WithValue(ctx, commandKey{}, runID)
}

// inCommand reports whether ctx runs inside runID's command transaction.
func inCommand(ctx context.Context, runID string) bool {
	return ctx.Value(commandKey{}) == runID
}

// CheckMutation protects conflicting service mutations without taking a run
// lock after character locks. Native commands carry the already-held run identity.
func (s *Service[R, P]) CheckMutation(ctx context.Context, actor string) error {
	run, err := s.cfg.Store.FindActorRun(ctx, actor)
	if err != nil {
		return err
	}
	if run != nil && !inCommand(ctx, run.RunID) {
		return ErrBusy
	}
	return nil
}
