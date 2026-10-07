package playercontext

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/witchcraze/party2re/internal/core/scheduling"
)

var (
	ErrInvalidSelection        = errors.New("invalid navigation selection")
	ErrSelectionNotFound       = errors.New("navigation subject not found")
	ErrNavigationForbidden     = errors.New("navigation actor is not owned")
	ErrNavigationUnavailable   = errors.New("navigation is unavailable during recovery or unfinished work")
	ErrNavigationNotConfigured = errors.New("navigation is not configured")
	navigationID               = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)
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
}

type NavigationObservation struct {
	Selection   Selection `json:"selection"`
	Unavailable bool      `json:"unavailable"`
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
	SubjectAvailable        func(ctx context.Context, actorID, targetID string) (bool, error)
}

type PageParams struct {
	Destination string `json:"destination"`
	Offset      int    `json:"offset"`
	Limit       int    `json:"limit"`
}

func (s *Service) NavigationConfigured() bool { return s.navigation != nil }

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

func validSelection(n Selection) bool {
	return navigationID.MatchString(n.Destination) && n.Offset >= 0 && n.Offset <= 1000000 && n.Limit >= 0 && n.Limit <= 100 &&
		(n.Subject == (Subject{}) || (navigationID.MatchString(n.Subject.Kind) && navigationID.MatchString(n.Subject.ID))) &&
		(n.Offset == 0 || n.Limit > 0)
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

func (s *Service) observeNavigation(ctx context.Context, actorID string) (*NavigationObservation, error) {
	if s.navigation == nil {
		return nil, nil
	}
	n, err := s.selection(ctx, actorID)
	if err != nil {
		return nil, err
	}
	d, ok := s.scenes[n.Destination]
	observation := &NavigationObservation{Selection: n, Unavailable: !ok}
	if n.Subject != (Subject{}) {
		if !ok || d.SubjectKind != n.Subject.Kind || d.SubjectAvailable == nil {
			observation.Unavailable = true
		} else {
			available, err := d.SubjectAvailable(ctx, actorID, n.Subject.ID)
			if err != nil {
				return nil, fmt.Errorf("read selected subject: %w", err)
			}
			observation.Unavailable = !available
		}
	}
	return observation, nil
}

// navigate rechecks owned actor and recovery/work guards at the service boundary.
// One SET replaces the whole record; concurrent clients use the last SET.
func (s *Service) navigate(ctx context.Context, playerID, actorID string, transition func(Selection) (Selection, error)) (Selection, error) {
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
	if r.Snapshot.Sleeping || slices.ContainsFunc(r.Snapshot.OngoingActions, func(a scheduling.ScheduledAction) bool {
		return a.State == scheduling.StatePending || a.State == scheduling.StateProcessing
	}) {
		return Selection{}, ErrNavigationUnavailable
	}
	n, err := transition(r.Navigation.Selection)
	if err != nil {
		return Selection{}, err
	}
	if !validSelection(n) {
		return Selection{}, ErrInvalidSelection
	}
	if err := s.navigation.Save(ctx, actorID, n); err != nil {
		return Selection{}, fmt.Errorf("save navigation: %w", err)
	}
	return n, nil
}

func (s *Service) Enter(ctx context.Context, playerID, actorID, destination string) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(Selection) (Selection, error) {
		if _, ok := s.scenes[destination]; !ok {
			return Selection{}, ErrInvalidSelection
		}
		return Selection{Destination: destination}, nil
	})
}

func (s *Service) Select(ctx context.Context, playerID, actorID string, subject Subject) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(n Selection) (Selection, error) {
		d, ok := s.scenes[n.Destination]
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
	return s.navigate(ctx, playerID, actorID, func(n Selection) (Selection, error) {
		d, ok := s.scenes[n.Destination]
		if !ok || !d.Pageable || p.Destination != n.Destination || n.Subject != (Subject{}) {
			return Selection{}, ErrInvalidSelection
		}
		if p.Limit == 0 {
			p.Limit = 20
		}
		n.Offset, n.Limit = p.Offset, p.Limit
		return n, nil
	})
}

func (s *Service) Back(ctx context.Context, playerID, actorID string) (Selection, error) {
	return s.navigate(ctx, playerID, actorID, func(n Selection) (Selection, error) {
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
