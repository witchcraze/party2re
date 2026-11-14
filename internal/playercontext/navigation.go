package playercontext

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/core/character"
)

var (
	ErrInvalidSelection        = errors.New("invalid navigation selection")
	ErrSelectionNotFound       = errors.New("navigation subject not found")
	ErrSceneAccessDenied       = errors.New("character does not qualify for the selected scene")
	ErrNavigationForbidden     = errors.New("navigation actor is not owned")
	ErrNavigationUnavailable   = errors.New("navigation is unavailable during recovery or unfinished work")
	ErrNavigationNotConfigured = errors.New("navigation is not configured")
	navigationID               = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
	navigationCursor           = regexp.MustCompile(`^[a-zA-Z0-9_=-]{0,512}$`)
)

// Subject identifiers are validated by the selected facility's public read port.
type Subject struct {
	Kind string `json:"target_kind"`
	ID   string `json:"target_id"`
}

// Selection is the complete bounded record replaced by each successful write.
type Selection struct {
	Destination string  `json:"destination"`
	Subject     Subject `json:"subject"`
	Offset      int     `json:"offset"`
	Limit       int     `json:"limit"`
	Cursor      *string `json:"cursor,omitempty"`
}

type NavigationObservation struct {
	Selection   Selection `json:"selection"`
	Unavailable bool      `json:"unavailable"`
	// SubjectOwned reports that the actor owns the selected subject.
	SubjectOwned bool `json:"-"`
}

// NavigationRepository has no authority over feature activity or durable assets.
type NavigationRepository interface {
	Load(context.Context, string) (Selection, error)
	Save(context.Context, string, Selection) error
}

// SceneDefinition registers a parent and a single optional subject/collection.
// SubjectAvailable must be a read-only public adapter with propagated errors.
type SceneDefinition struct {
	ID, Parent, SubjectKind string
	Pageable                bool
	CursorPageable          bool
	// ParentOnly admits entry only from Parent: a place legacy reaches through
	// a facility action instead of ＠いどう.
	ParentOnly bool
	// CanEnter is a pure character qualification check; nil permits entry.
	CanEnter         func(character.Character) bool
	SubjectAvailable func(ctx context.Context, actorID, targetID string) (bool, error)
	// SubjectOwned is an optional read-only adapter telling whether the actor
	// owns an available subject; nil means subjects are never owned.
	SubjectOwned func(ctx context.Context, actorID, targetID string) (bool, error)
	// Actions are the facility commands the scene admits; non-nil SubjectActions
	// replace them while a subject is selected, and non-nil OwnedSubjectActions
	// replace those for a subject the actor owns. Set by AdmitSceneActions.
	Actions, SubjectActions, OwnedSubjectActions []string
}

type PageParams struct {
	Destination string  `json:"destination"`
	Offset      int     `json:"offset,omitempty"`
	Limit       int     `json:"limit"`
	Cursor      *string `json:"cursor,omitempty"`
}

func (s *Service) NavigationConfigured() bool { return s.navigation != nil }

// godVisitScenes are the places a recorded god visit stands in (legacy
// $m{lib} god/u_god). The visit, not the saved selection, decides the scene.
var godVisitScenes = map[string]string{character.GodVisitHeaven: "god", character.GodVisitUnderworld: "u_god"}

// WithGodVisitForfeit clears a recorded god visit when navigation leaves its
// scene, as legacy ＠いどう/＠まち/＠ほーむ replace $m{lib}.
func WithGodVisitForfeit(forfeit func(ctx context.Context, characterID string) error) func(*Service) {
	return func(s *Service) { s.forfeitGodVisit = forfeit }
}

// SceneDefinitions returns the closed registry in stable ID order for adapters.
func (s *Service) SceneDefinitions() []SceneDefinition {
	definitions := make([]SceneDefinition, 0, len(s.scenes))
	for _, d := range s.scenes {
		definitions = append(definitions, d)
	}
	slices.SortFunc(definitions, func(a, b SceneDefinition) int { return strings.Compare(a.ID, b.ID) })
	return definitions
}

// WithNavigation injects the existing Valkey record and closed scene registry.
func WithNavigation(store NavigationRepository, definitions ...SceneDefinition) func(*Service) {
	return func(s *Service) {
		s.navigation = store
		s.scenes = make(map[string]SceneDefinition, len(definitions))
		for _, d := range definitions {
			if !navigationID.MatchString(d.ID) || (d.ID == LocationTown && d.Parent != "") || (d.ID != LocationTown && !navigationID.MatchString(d.Parent)) || (d.SubjectKind == "") != (d.SubjectAvailable == nil) || (d.SubjectKind != "" && !navigationID.MatchString(d.SubjectKind)) {
				panic("invalid navigation scene registration: " + d.ID)
			}
			if _, exists := s.scenes[d.ID]; exists {
				panic("duplicate navigation scene: " + d.ID)
			}
			s.scenes[d.ID] = d
		}
		if _, ok := s.scenes[LocationTown]; !ok {
			panic("navigation requires town")
		}
		for _, d := range definitions {
			seen := map[string]bool{d.ID: true}
			for d.Parent != "" {
				parent, ok := s.scenes[d.Parent]
				if !ok || seen[d.Parent] {
					panic("invalid navigation parent: " + d.Parent)
				}
				seen[d.Parent] = true
				d = parent
			}
		}
	}
}

// AdmitSceneActions registers, during composition, the facility commands a
// scene admits. They must be location-gated entry actions outside navigation.
func (s *Service) AdmitSceneActions(sceneID string, actions, subjectActions, ownedSubjectActions []string) {
	d, ok := s.scenes[sceneID]
	if !ok {
		panic("scene actions for unregistered scene: " + sceneID)
	}
	for _, id := range slices.Concat(actions, subjectActions, ownedSubjectActions) {
		if def, known := GetAction(id); !known || !def.RequiredGates.Has(GateLocationCheck) || def.ActivityKind != "" || def.Category == "navigation" {
			panic("scene " + sceneID + " cannot admit action: " + id)
		}
	}
	d.Actions, d.SubjectActions, d.OwnedSubjectActions = actions, subjectActions, ownedSubjectActions
	s.scenes[sceneID] = d
}

// sceneActions is the single list of location-bound commands the observed
// scene admits. Discovery and execution both apply it through LocationGate.
func (s *Service) sceneActions(o *NavigationObservation) []string {
	d, ok := s.scenes[o.Selection.Destination]
	actions := []string{"scene_enter"}
	if ok && d.ID != LocationTown {
		actions = append(actions, "scene_back")
	}
	if o.Unavailable {
		return actions
	}
	if o.Selection.Subject != (Subject{}) {
		if o.SubjectOwned && d.OwnedSubjectActions != nil {
			return append(actions, d.OwnedSubjectActions...)
		}
		if d.SubjectActions != nil {
			return append(actions, d.SubjectActions...)
		}
		return append(actions, d.Actions...)
	}
	if d.SubjectKind != "" {
		actions = append(actions, "scene_select")
	}
	if d.Pageable {
		actions = append(actions, "scene_page")
	}
	return append(actions, d.Actions...)
}

// Enterable reports whether the observed actor may enter destination from the
// observed scene. Enter enforces it; discovery offers only destinations it admits.
func (s *Service) Enterable(r Result, destination string) error {
	d, ok := s.scenes[destination]
	if !ok {
		return ErrInvalidSelection
	}
	if d.CanEnter != nil && !d.CanEnter(r.Snapshot.Character) {
		return ErrSceneAccessDenied
	}
	if d.ParentOnly && (r.Navigation == nil || r.Navigation.Unavailable || r.Navigation.Selection.Destination != d.Parent) {
		return ErrSceneAccessDenied
	}
	return nil
}

func validSelection(n Selection) bool {
	return navigationID.MatchString(n.Destination) && n.Offset >= 0 && n.Offset <= 1000000 && n.Limit >= 0 && n.Limit <= 100 &&
		(n.Subject == (Subject{}) || (navigationID.MatchString(n.Subject.Kind) && navigationID.MatchString(n.Subject.ID))) &&
		(n.Offset == 0 || n.Limit > 0) && (n.Cursor == nil || (navigationCursor.MatchString(*n.Cursor) && n.Offset == 0 && n.Subject == (Subject{})))
}

func (s *Service) selection(ctx context.Context, actorID string) (Selection, error) {
	n, err := s.navigation.Load(ctx, actorID)
	if err != nil {
		return Selection{}, fmt.Errorf("load navigation: %w", err)
	}
	if n == (Selection{}) {
		n.Destination = LocationTown
	}
	if !validSelection(n) {
		return Selection{}, ErrInvalidSelection
	}
	return n, nil
}

func (s *Service) observeNavigation(ctx context.Context, actor character.Character) (*NavigationObservation, error) {
	if s.navigation == nil {
		return nil, nil
	}
	if scene, ok := godVisitScenes[actor.GodVisit]; ok {
		if _, registered := s.scenes[scene]; registered {
			return &NavigationObservation{Selection: Selection{Destination: scene}}, nil
		}
	}
	n, err := s.selection(ctx, actor.ID)
	if err != nil {
		return nil, err
	}
	d, ok := s.scenes[n.Destination]
	observation := &NavigationObservation{Selection: n, Unavailable: !ok}
	if d.CanEnter != nil && !d.CanEnter(actor) {
		observation.Unavailable = true
		return observation, nil
	}
	if n.Cursor != nil && !d.CursorPageable {
		observation.Unavailable = true
	}
	if n.Subject != (Subject{}) {
		if !ok || d.SubjectKind != n.Subject.Kind || d.SubjectAvailable == nil {
			observation.Unavailable = true
		} else {
			available, err := d.SubjectAvailable(ctx, actor.ID, n.Subject.ID)
			if err != nil {
				return nil, fmt.Errorf("read selected subject: %w", err)
			}
			observation.Unavailable = !available
			if available && d.SubjectOwned != nil {
				if observation.SubjectOwned, err = d.SubjectOwned(ctx, actor.ID, n.Subject.ID); err != nil {
					return nil, fmt.Errorf("read selected subject owner: %w", err)
				}
			}
		}
	}
	return observation, nil
}

// navigate rechecks owned actor and recovery/work guards at the service boundary.
// One SET replaces the whole record; concurrent clients use the last SET.
func (s *Service) navigate(ctx context.Context, playerID, actorID string, transition func(Result) (Selection, error)) (Selection, error) {
	if err := ctx.Err(); err != nil {
		return Selection{}, err
	}
	if s.navigation == nil {
		return Selection{}, ErrNavigationNotConfigured
	}
	if playerID == "" {
		return Selection{}, ErrNavigationForbidden
	}
	r, err := s.query(ctx, actorID, playerID)
	if err != nil {
		return Selection{}, err
	}
	if len(r.Snapshot.ActiveActivities()) > 0 {
		return Selection{}, ErrNavigationUnavailable
	}
	n, err := transition(r)
	if err != nil {
		return Selection{}, err
	}
	if !validSelection(n) {
		return Selection{}, ErrInvalidSelection
	}
	// Staying in the visit's scene keeps the saved selection to return to after the wish.
	if scene := godVisitScenes[r.Snapshot.Character.GodVisit]; scene != "" && n.Destination == scene {
		return n, nil
	}
	if err := s.navigation.Save(ctx, actorID, n); err != nil {
		return Selection{}, fmt.Errorf("save navigation: %w", err)
	}
	// Clear after the save: a failed clear leaves the visit, so the actor
	// still observes the god scene and may retry.
	if scene := r.Navigation.Selection.Destination; scene != n.Destination && scene == godVisitScenes[r.Snapshot.Character.GodVisit] {
		if s.forfeitGodVisit == nil {
			return Selection{}, ErrNavigationNotConfigured
		}
		if err := s.forfeitGodVisit(ctx, actorID); err != nil {
			return Selection{}, fmt.Errorf("forfeit god visit: %w", err)
		}
	}
	return n, nil
}

func (s *Service) Enter(ctx context.Context, playerID, actorID, destination string) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(r Result) (Selection, error) {
		if err := s.Enterable(r, destination); err != nil {
			return Selection{}, err
		}
		return Selection{Destination: destination}, nil
	})
}

func (s *Service) Select(ctx context.Context, playerID, actorID string, subject Subject) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(r Result) (Selection, error) {
		n := r.Navigation.Selection
		d, ok := s.scenes[n.Destination]
		if d.CanEnter != nil && !d.CanEnter(r.Snapshot.Character) {
			return Selection{}, ErrSceneAccessDenied
		}
		if !ok || d.SubjectAvailable == nil || d.SubjectKind != subject.Kind || !navigationID.MatchString(subject.ID) {
			return Selection{}, ErrInvalidSelection
		}
		available, err := d.SubjectAvailable(ctx, actorID, subject.ID)
		if err != nil {
			return Selection{}, err
		}
		if !available {
			return Selection{}, ErrSelectionNotFound
		}
		return Selection{Destination: n.Destination, Subject: subject}, nil
	})
}

func (s *Service) Page(ctx context.Context, playerID, actorID string, p PageParams) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(r Result) (Selection, error) {
		n := r.Navigation.Selection
		d, ok := s.scenes[n.Destination]
		if d.CanEnter != nil && !d.CanEnter(r.Snapshot.Character) {
			return Selection{}, ErrSceneAccessDenied
		}
		if !ok || !d.Pageable || p.Destination != n.Destination || n.Subject != (Subject{}) || (p.Cursor != nil && !d.CursorPageable) {
			return Selection{}, ErrInvalidSelection
		}
		if p.Limit == 0 {
			p.Limit = 20
		}
		n.Offset, n.Limit = p.Offset, p.Limit
		n.Cursor = p.Cursor
		return n, nil
	})
}

func (s *Service) Back(ctx context.Context, playerID, actorID string) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(r Result) (Selection, error) {
		n := r.Navigation.Selection
		if n.Subject != (Subject{}) {
			return Selection{Destination: n.Destination}, nil
		}
		d, ok := s.scenes[n.Destination]
		if !ok {
			return Selection{}, ErrInvalidSelection
		}
		if d.Parent == "" {
			return Selection{Destination: LocationTown}, nil
		}
		return Selection{Destination: d.Parent}, nil
	})
}
